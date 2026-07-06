package app

import (
	"strings"
	"testing"
)

func TestParseSubscription_ClashYAML(t *testing.T) {
	raw := `
proxies:
  - name: hk-vless
    type: vless
    server: hk.example.com
    port: 443
    uuid: 11111111-1111-1111-1111-111111111111
    sni: hk.example.com
  - name: hy2-node
    type: hysteria2
    server: hy2.example.com
    port: 8443
    password: pass123
`
	result := parseSubscription(raw)
	if len(result.nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d, warnings=%v", len(result.nodes), result.warnings)
	}
}

func TestParseSubscription_UnknownTypeFallback(t *testing.T) {
	raw := `
proxies:
  - name: unknown-a
    type: unknownproto
    server: x.example.com
    port: 8080
    username: u
    password: p
`
	result := parseSubscription(raw)
	if len(result.nodes) != 1 {
		t.Fatalf("expected 1 fallback node, got %d", len(result.nodes))
	}
	node := result.nodes[0]
	if node["compat_fallback"] != true {
		t.Fatalf("expected compat_fallback=true, got %v", node["compat_fallback"])
	}
	if node["compat_origin_type"] != "unknownproto" {
		t.Fatalf("expected compat_origin_type=unknownproto, got %v", node["compat_origin_type"])
	}
	if tag, _ := node["tag"].(string); tag == "" || !strings.HasPrefix(tag, "[fallback]") {
		t.Fatalf("expected fallback tag prefix, got %v", node["tag"])
	}
}
