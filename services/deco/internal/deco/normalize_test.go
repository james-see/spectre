package deco

import "testing"

func TestNormalizeClientsExcludesSpectreESP32(t *testing.T) {
	mesh := []MeshNode{{Name: "Living", MAC: "aa:bb:cc:dd:ee:01", Role: "master"}}
	raw := []map[string]any{
		{"name": "Phone", "mac": "11:22:33:44:55:66", "ip": "192.168.68.10", "online": true, "_deco_mac": "aa:bb:cc:dd:ee:01"},
		{"name": "ESP-A", "mac": "7c:4f:ad:12:34:56", "ip": "192.168.68.61", "online": true, "_deco_mac": "aa:bb:cc:dd:ee:01"},
		{"name": "ESP-B", "mac": "7C:4F:AD:aa:bb:cc", "ip": "192.168.68.71", "online": true, "_deco_mac": "aa:bb:cc:dd:ee:01"},
	}
	out := NormalizeClients(raw, mesh)
	if len(out) != 1 {
		t.Fatalf("want 1 client, got %d", len(out))
	}
	if out[0].MAC != "11:22:33:44:55:66" {
		t.Fatalf("unexpected mac %s", out[0].MAC)
	}
	if out[0].DecoName != "Living" {
		t.Fatalf("deco name=%q", out[0].DecoName)
	}
}

func TestNormalizeMeshMasterRole(t *testing.T) {
	devices := []map[string]any{
		{"mac": "aa:bb:cc:01", "role": "slave", "nickname": "hall", "device_ip": "192.168.68.2", "group_status": "connected"},
		{"mac": "aa:bb:cc:00", "role": "master", "custom_nickname": "Main", "device_ip": "192.168.68.1", "group_status": "connected"},
	}
	mesh := NormalizeMesh(devices)
	if len(mesh) != 2 {
		t.Fatalf("len=%d", len(mesh))
	}
	masters := 0
	for _, m := range mesh {
		if m.Role == "master" {
			masters++
			if m.Name != "Main" {
				t.Fatalf("master name=%q", m.Name)
			}
		}
	}
	if masters != 1 {
		t.Fatalf("masters=%d", masters)
	}
}
