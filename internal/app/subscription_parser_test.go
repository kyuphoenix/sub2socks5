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

func TestParseNodeLine_Hysteria2PreservesTLSAndPortHopping(t *testing.T) {
	node, err := parseNodeLine("hysteria2://secret@hy.example.com:443?sni=front.example.com&insecure=1&mport=20000-30000,40000#hy2-node")
	if err != nil {
		t.Fatalf("parse hysteria2 node: %v", err)
	}
	tls := getMap(node, "tls")
	if tls["enabled"] != true {
		t.Fatalf("hysteria2 TLS should be enabled, got %v", tls)
	}
	if got := mustStr(tls["server_name"]); got != "front.example.com" {
		t.Fatalf("hysteria2 SNI should be preserved, got %q", got)
	}
	if tls["insecure"] != true {
		t.Fatalf("hysteria2 insecure=1 should be preserved, got %v", tls["insecure"])
	}
	ports := getSlice(node, "server_ports")
	if len(ports) != 2 || mustStr(ports[0]) != "20000:30000" || mustStr(ports[1]) != "40000:40000" {
		t.Fatalf("unexpected hysteria2 server_ports: %#v", ports)
	}
}

func TestParseNodeLine_VLESSSupportsInsecureQuery(t *testing.T) {
	node, err := parseNodeLine("vless://11111111-1111-1111-1111-111111111111@vless.example.com:443?security=tls&sni=front.example.com&insecure=1&fp=chrome&type=tcp#vless-node")
	if err != nil {
		t.Fatalf("parse vless node: %v", err)
	}
	tls := getMap(node, "tls")
	if tls["insecure"] != true {
		t.Fatalf("vless insecure=1 should be preserved, got %v", tls["insecure"])
	}
	if got := mustStr(tls["server_name"]); got != "front.example.com" {
		t.Fatalf("vless SNI should be preserved, got %q", got)
	}
}
