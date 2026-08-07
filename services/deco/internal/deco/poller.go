package deco

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Poller refreshes mesh + clients on an interval and caches the last snapshot.
type Poller struct {
	client *Client
	every  time.Duration

	mu   sync.RWMutex
	snap Snapshot

	meshEvery  time.Duration
	lastMeshAt time.Time
	cachedMesh []MeshNode
	cachedMACs []string
	busy       atomic.Bool
}

func NewPoller(client *Client, every time.Duration) *Poller {
	if every < time.Second {
		every = 3 * time.Second
	}
	return &Poller{
		client:    client,
		every:     every,
		meshEvery: 30 * time.Second,
		snap: Snapshot{
			Mesh:    []MeshNode{},
			Clients: []LanClient{},
			Error:   "not yet polled",
		},
	}
}

func (p *Poller) Snapshot() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cp := p.snap
	cp.Mesh = append([]MeshNode(nil), p.snap.Mesh...)
	cp.Clients = append([]LanClient(nil), p.snap.Clients...)
	return cp
}

func (p *Poller) Start() {
	// First poll in background so HTTP bind is not blocked by Deco RTT.
	go func() {
		p.refresh()
		t := time.NewTicker(p.every)
		defer t.Stop()
		for range t.C {
			p.refresh()
		}
	}()
}

func (p *Poller) refresh() {
	if !p.busy.CompareAndSwap(false, true) {
		return // prior Deco poll still in flight
	}
	defer p.busy.Store(false)

	start := time.Now()
	needMesh := len(p.cachedMesh) == 0 || time.Since(p.lastMeshAt) >= p.meshEvery
	var mesh []MeshNode
	var macs []string
	if needMesh {
		devices, err := p.client.ListDevices()
		if err != nil {
			p.fail(err)
			return
		}
		mesh = NormalizeMesh(devices)
		macs = make([]string, 0, len(mesh))
		for _, m := range mesh {
			if m.MAC != "" {
				macs = append(macs, m.MAC)
			}
		}
		p.cachedMesh = mesh
		p.cachedMACs = macs
		p.lastMeshAt = time.Now()
	} else {
		mesh = p.cachedMesh
		macs = p.cachedMACs
	}

	clientsRaw, err := p.client.ListAllClients(macs)
	if err != nil {
		p.fail(err)
		return
	}
	clients := NormalizeClients(clientsRaw, mesh)

	p.mu.Lock()
	p.snap = Snapshot{
		Mesh:      append([]MeshNode(nil), mesh...),
		Clients:   clients,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	p.mu.Unlock()
	log.Printf("deco poll ok: mesh=%d clients=%d in %s", len(mesh), len(clients), time.Since(start).Round(time.Millisecond))
}

func (p *Poller) fail(err error) {
	log.Printf("deco poll error: %v", err)
	p.mu.Lock()
	// Fail closed: empty lists + error (keep last UpdatedAt if any).
	prev := p.snap.UpdatedAt
	p.snap = Snapshot{
		Mesh:      []MeshNode{},
		Clients:   []LanClient{},
		Error:     err.Error(),
		UpdatedAt: prev,
	}
	p.mu.Unlock()
}
