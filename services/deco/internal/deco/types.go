package deco

// MeshNode is one Deco AP in the mesh.
type MeshNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	Role   string `json:"role"` // master | satellite
	Online bool   `json:"online"`
}

// LanClient is a Wi‑Fi / Ethernet station associated with a Deco.
type LanClient struct {
	Name           string `json:"name"`
	MAC            string `json:"mac"`
	IP             string `json:"ip"`
	Online         bool   `json:"online"`
	Band           string `json:"band,omitempty"`
	ConnectionType string `json:"connection_type,omitempty"`
	DecoMAC        string `json:"deco_mac"`
	DecoName       string `json:"deco_name,omitempty"`
	UpRateKbps     *int   `json:"up_rate_kbps,omitempty"`
	DownRateKbps   *int   `json:"down_rate_kbps,omitempty"`
}

// Snapshot is the last successful (or fail-closed empty) poll result.
type Snapshot struct {
	Mesh      []MeshNode  `json:"mesh"`
	Clients   []LanClient `json:"clients"`
	Error     string      `json:"error,omitempty"`
	UpdatedAt string      `json:"updated_at,omitempty"`
}
