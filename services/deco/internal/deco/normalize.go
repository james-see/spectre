package deco

import (
	"fmt"
	"strings"
)

// Spectre ESP32 OUI — keep these on the CSI layer, not LAN client orbs.
const spectreESP32OUI = "7c:4f:ad"

func normMAC(mac string) string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	mac = strings.ReplaceAll(mac, "-", ":")
	return mac
}

func isSpectreESP32(mac string) bool {
	return strings.HasPrefix(normMAC(mac), spectreESP32OUI)
}

func strField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func isMasterDevice(d map[string]any) bool {
	if strings.EqualFold(strField(d, "role"), "master") {
		return true
	}
	switch v := d["master"].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	}
	return false
}

func decoOnline(d map[string]any) bool {
	if gs := strings.ToLower(strField(d, "group_status")); gs != "" {
		return gs == "connected"
	}
	return boolOnline(d)
}

func boolOnline(m map[string]any) bool {
	if v, ok := m["online"]; ok {
		switch t := v.(type) {
		case bool:
			return t
		case float64:
			return t != 0
		case string:
			s := strings.ToLower(t)
			return s == "online" || s == "true" || s == "1" || s == "connected"
		}
	}
	// client_list is typically only connected clients
	return true
}

func intPtrField(m map[string]any, keys ...string) *int {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			n := asInt(v)
			return &n
		}
	}
	return nil
}

// NormalizeMesh maps Deco device_list entries to MeshNode.
func NormalizeMesh(devices []map[string]any) []MeshNode {
	out := make([]MeshNode, 0, len(devices))
	for _, d := range devices {
		mac := normMAC(strField(d, "mac", "device_mac"))
		name := strField(d, "custom_nickname")
		if name == "" {
			name = humanizeNickname(strField(d, "nickname", "name", "device_name"))
		}
		if name == "" {
			name = mac
		}
		role := "satellite"
		if isMasterDevice(d) {
			role = "master"
		}
		ip := strField(d, "device_ip", "ip", "ip_addr", "inet_ip")
		id := strField(d, "device_id", "id")
		if id == "" {
			id = mac
		}
		out = append(out, MeshNode{
			ID:     id,
			Name:   name,
			MAC:    mac,
			IP:     ip,
			Role:   role,
			Online: decoOnline(d),
		})
	}
	masters := 0
	for _, n := range out {
		if n.Role == "master" {
			masters++
		}
	}
	if masters == 0 && len(out) > 0 {
		out[0].Role = "master"
	}
	if masters > 1 {
		seen := false
		for i := range out {
			if out[i].Role != "master" {
				continue
			}
			if !seen {
				seen = true
				continue
			}
			out[i].Role = "satellite"
		}
	}
	return out
}

func humanizeNickname(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return strings.Join(parts, " ")
}

// NormalizeClients maps client_list → LanClient, dropping Spectre ESP32 MACs.
func NormalizeClients(raw []map[string]any, mesh []MeshNode) []LanClient {
	decoNames := map[string]string{}
	for _, m := range mesh {
		decoNames[m.MAC] = m.Name
	}
	out := make([]LanClient, 0, len(raw))
	for _, c := range raw {
		mac := normMAC(strField(c, "mac", "client_mac"))
		if mac == "" || isSpectreESP32(mac) {
			continue
		}
		decoMAC := normMAC(strField(c, "_deco_mac", "device_mac", "deco_mac", "ap_mac", "parent_mac"))
		name := strField(c, "name", "hostname", "client_name")
		if name == "" {
			name = mac
		}
		band := strField(c, "band", "wifi_mode", "radio")
		conn := strField(c, "connection_type", "client_type", "type", "interface")
		if band == "" {
			band = conn
		}
		if strings.EqualFold(band, "undefined") {
			band = ""
		}
		if strings.EqualFold(conn, "undefined") {
			conn = ""
		}
		online := boolOnline(c)
		// Observatory only wants currently associated stations — Deco often
		// returns a large historical client_list.
		if !online {
			continue
		}
		out = append(out, LanClient{
			Name:           name,
			MAC:            mac,
			IP:             strField(c, "ip", "ip_addr", "inet_ip"),
			Online:         online,
			Band:           band,
			ConnectionType: conn,
			DecoMAC:        decoMAC,
			DecoName:       decoNames[decoMAC],
			UpRateKbps:     intPtrField(c, "up_speed", "up_rate", "uplink"),
			DownRateKbps:   intPtrField(c, "down_speed", "down_rate", "downlink"),
		})
	}
	return out
}
