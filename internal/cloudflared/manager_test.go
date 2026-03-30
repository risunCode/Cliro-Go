package cloudflared

import "testing"

func TestExtractTunnelURL_QuickTunnel(t *testing.T) {
	line := "INF https://gentle-breeze-demo.trycloudflare.com registered tunnel connection"
	if got := extractTunnelURL(line); got != "https://gentle-breeze-demo.trycloudflare.com" {
		t.Fatalf("quick tunnel url = %q", got)
	}
}

func TestExtractTunnelURL_NamedTunnel(t *testing.T) {
	line := `INF Updated to new configuration config="{\"ingress\":[{\"hostname\":\"api.example.com\",\"service\":\"http://localhost:8095\"}]}"`
	if got := extractTunnelURL(line); got != "https://api.example.com" {
		t.Fatalf("named tunnel url = %q", got)
	}
}
