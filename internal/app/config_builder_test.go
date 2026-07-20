package app

import "testing"

func TestBuildSingBoxConfig_URLTestGroupUsesNativeURLTest(t *testing.T) {
	cfg := map[string]any{
		"app":     map[string]any{"logLevel": "info"},
		"dns":     map[string]any{"strategy": "prefer_ipv4", "bootstrapServer": "1.1.1.1"},
		"routing": map[string]any{"routeFinal": "proxy"},
		"nodeRegistry": map[string]any{
			"manualNodes": []any{
				map[string]any{
					"type": "vless", "tag": "n1", "server": "a.example.com", "server_port": 443, "uuid": "11111111-1111-1111-1111-111111111111",
				},
			},
			"groups": []any{
				map[string]any{"tag": "urltest-1", "strategy": "urltest", "members": []any{"n1"}, "url": "https://cp.cloudflare.com/generate_204", "interval": "5m"},
			},
			"chains": []any{},
		},
		"ports": []any{
			map[string]any{"tag": "socks-urltest", "listen": "127.0.0.1", "port": 18081, "target": "urltest-1"},
			map[string]any{"tag": "socks-n", "listen": "127.0.0.1", "port": 18082, "target": "proxy"},
		},
	}
	sub := map[string]any{"nodes": []any{}}

	gen := buildSingBoxConfig(cfg, sub)
	outbounds, _ := gen["outbounds"].([]any)
	var group map[string]any
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if ok && mustStr(m["tag"]) == "urltest-1" {
			group = m
			break
		}
	}
	if group == nil {
		t.Fatalf("urltest group should be generated")
	}
	if got := mustStr(group["type"]); got != "urltest" {
		t.Fatalf("urltest group should use sing-box native urltest outbound, got %s", got)
	}
	if got := mustStr(group["url"]); got != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("urltest url should be preserved, got %s", got)
	}

	inbounds, _ := gen["inbounds"].([]any)
	if len(inbounds) < 2 {
		t.Fatalf("expected >=2 inbounds, got %d", len(inbounds))
	}
	check := map[string]int{}
	for _, item := range inbounds {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		check[mustStr(m["tag"])] = int(toFloat(m["listen_port"]))
	}
	if check["socks-urltest"] != 18081 {
		t.Fatalf("urltest target port should keep original port, got %d", check["socks-urltest"])
	}
	if check["socks-n"] != 18082 {
		t.Fatalf("other port should keep original port, got %d", check["socks-n"])
	}
}

func TestBuildSingBoxConfig_LegacyRotateUsesNativeURLTest(t *testing.T) {
	cfg := map[string]any{
		"app":     map[string]any{"logLevel": "info"},
		"dns":     map[string]any{"strategy": "prefer_ipv4", "bootstrapServer": "1.1.1.1"},
		"routing": map[string]any{"routeFinal": "proxy"},
		"nodeRegistry": map[string]any{
			"manualNodes": []any{
				map[string]any{
					"type": "vless", "tag": "n1", "server": "a.example.com", "server_port": 443, "uuid": "11111111-1111-1111-1111-111111111111",
				},
			},
			"groups": []any{
				map[string]any{"tag": "old-rotate", "strategy": "rotate", "members": []any{"n1"}},
			},
			"chains": []any{},
		},
		"ports": []any{
			map[string]any{"tag": "socks-old", "listen": "127.0.0.1", "port": 18081, "target": "old-rotate"},
		},
	}

	gen := buildSingBoxConfig(cfg, map[string]any{"nodes": []any{}})
	outbounds, _ := gen["outbounds"].([]any)
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if ok && mustStr(m["tag"]) == "old-rotate" {
			if got := mustStr(m["type"]); got != "urltest" {
				t.Fatalf("legacy rotate should be mapped to urltest, got %s", got)
			}
			return
		}
	}
	t.Fatalf("legacy rotate group should be generated")
}

func TestNormalizeAppConfig_MigratesLegacyGroupStrategies(t *testing.T) {
	cfg := map[string]any{
		"nodeRegistry": map[string]any{
			"groups": []any{
				map[string]any{"tag": "old-rotate", "strategy": "rotate"},
				map[string]any{"tag": "old-loadbalance", "strategy": "loadbalance"},
				map[string]any{"tag": "url-test-alias", "strategy": "url-test"},
			},
		},
		"runtimeState": map[string]any{"rotateGroups": map[string]any{"old-rotate": map[string]any{}}},
	}

	normalized, changed := normalizeAppConfig(cfg)
	if !changed {
		t.Fatalf("legacy strategy normalization should report changed")
	}
	groups := getSlice(getMap(normalized, "nodeRegistry"), "groups")
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if got := mustStr(group["strategy"]); got != "urltest" {
			t.Fatalf("legacy group %s should be rewritten to urltest, got %s", mustStr(group["tag"]), got)
		}
	}
	if _, exists := getMap(normalized, "runtimeState")["rotateGroups"]; exists {
		t.Fatalf("legacy runtime rotate state should be removed")
	}
}

func TestCollectOutbounds_BasicPresence(t *testing.T) {
	cfg := map[string]any{
		"nodeRegistry": map[string]any{
			"manualNodes": []any{
				map[string]any{"tag": "manual-a", "type": "vless"},
			},
			"groups": []any{
				map[string]any{"tag": "group-a", "strategy": "urltest"},
			},
			"chains": []any{
				map[string]any{"tag": "chain-a"},
			},
		},
	}
	sub := map[string]any{
		"nodes": []any{
			map[string]any{"tag": "sub-a", "type": "trojan"},
		},
	}

	out := collectOutbounds(cfg, sub)
	if len(out) == 0 {
		t.Fatalf("collectOutbounds should not be empty")
	}
	tags := map[string]bool{}
	for _, item := range out {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tags[mustStr(m["tag"])] = true
	}
	required := []string{"group-a", "chain-a", "manual-a", "sub-a", "proxy", "auto", "block"}
	for _, tag := range required {
		if !tags[tag] {
			t.Fatalf("expected outbound tag %s not found", tag)
		}
	}
}

func TestCollectOutbounds_ExcludesDisabledSubscriptionNodes(t *testing.T) {
	cfg := map[string]any{
		"nodeRegistry": map[string]any{
			"disabledSubscriptionTags": []any{"sub-disabled"},
		},
	}
	sub := map[string]any{
		"nodes": []any{
			map[string]any{"tag": "sub-active", "type": "trojan"},
			map[string]any{"tag": "sub-disabled", "type": "trojan"},
		},
	}

	out := collectOutbounds(cfg, sub)
	tags := map[string]bool{}
	for _, item := range out {
		m, ok := item.(map[string]any)
		if ok {
			tags[mustStr(m["tag"])] = true
		}
	}
	if !tags["sub-active"] {
		t.Fatalf("active subscription node should remain available")
	}
	if tags["sub-disabled"] {
		t.Fatalf("disabled subscription node should not be available")
	}
}

func TestBuildSingBoxConfig_ExcludesDisabledSubscriptionNodes(t *testing.T) {
	cfg := map[string]any{
		"app":     map[string]any{"logLevel": "info"},
		"dns":     map[string]any{"strategy": "prefer_ipv4", "bootstrapServer": "1.1.1.1"},
		"routing": map[string]any{"routeFinal": "proxy"},
		"nodeRegistry": map[string]any{
			"disabledSubscriptionTags": []any{"sub-disabled"},
			"manualNodes":              []any{},
			"groups":                   []any{},
			"chains":                   []any{},
		},
		"ports": []any{},
	}
	sub := map[string]any{
		"nodes": []any{
			map[string]any{"tag": "sub-active", "type": "trojan", "server": "active.example.com", "server_port": 443, "password": "secret"},
			map[string]any{"tag": "sub-disabled", "type": "trojan", "server": "disabled.example.com", "server_port": 443, "password": "secret"},
		},
	}

	generated := buildSingBoxConfig(cfg, sub)
	tags := map[string]bool{}
	for _, item := range getSlice(generated, "outbounds") {
		m, ok := item.(map[string]any)
		if ok {
			tags[mustStr(m["tag"])] = true
		}
	}
	if !tags["sub-active"] {
		t.Fatalf("active subscription node should remain in sing-box config")
	}
	if tags["sub-disabled"] {
		t.Fatalf("disabled subscription node should not remain in sing-box config")
	}
}
