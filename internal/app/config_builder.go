package app

import (
	"encoding/json"
	"fmt"
	"strings"
)

func toAnySliceString(in []string) []any {
	out := make([]any, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	return out
}

func normalizeOutboundForSingBox(node map[string]any) map[string]any {
	if node == nil {
		return nil
	}
	cloned := cloneMap(node)
	t := mustStr(cloned["type"])
	if t == "" || mustStr(cloned["tag"]) == "" {
		return nil
	}

	if t == "hysteria2" || t == "tuic" {
		tls, _ := cloned["tls"].(map[string]any)
		if tls == nil {
			tls = map[string]any{}
		}
		tls["enabled"] = true
		if strings.TrimSpace(mustStr(tls["server_name"])) == "" {
			tls["server_name"] = mustStr(cloned["server"])
		}
		if _, ok := tls["insecure"]; !ok {
			tls["insecure"] = false
		}
		if _, ok := tls["alpn"]; !ok {
			tls["alpn"] = []any{"h3"}
		}
		cloned["tls"] = tls
	}

	if t == "vless" || t == "trojan" || t == "vmess" || t == "hysteria2" || t == "tuic" || t == "shadowsocks" || t == "socks" {
		if strings.TrimSpace(mustStr(cloned["server"])) == "" || int(toFloat(cloned["server_port"])) <= 0 {
			return nil
		}
	}

	if t == "vless" && strings.TrimSpace(mustStr(cloned["uuid"])) == "" {
		return nil
	}
	if t == "trojan" && strings.TrimSpace(mustStr(cloned["password"])) == "" {
		return nil
	}
	if t == "hysteria2" && strings.TrimSpace(mustStr(cloned["password"])) == "" {
		return nil
	}
	if t == "tuic" && (strings.TrimSpace(mustStr(cloned["uuid"])) == "" || strings.TrimSpace(mustStr(cloned["password"])) == "") {
		return nil
	}
	if t == "shadowsocks" && (strings.TrimSpace(mustStr(cloned["method"])) == "" || strings.TrimSpace(mustStr(cloned["password"])) == "") {
		return nil
	}

	return cloned
}

func cloneMap(in map[string]any) map[string]any {
	b, err := json.Marshal(in)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if json.Unmarshal(b, &out) != nil {
		return map[string]any{}
	}
	return out
}

func collectOutbounds(cfg, sub map[string]any) []any {
	nr := getMap(cfg, "nodeRegistry")
	disabledSubscriptionTags := toStringSet(getSlice(nr, "disabledSubscriptionTags"))
	groups := []any{}
	for _, g := range getSlice(nr, "groups") {
		if m, ok := g.(map[string]any); ok {
			groups = append(groups, map[string]any{"tag": mustStr(m["tag"]), "type": mustStr(m["strategy"]), "source": "group", "label": fmt.Sprintf("%s（%s / 节点组）", mustStr(m["tag"]), mustStr(m["strategy"]))})
		}
	}
	chains := []any{}
	for _, c := range getSlice(nr, "chains") {
		if m, ok := c.(map[string]any); ok {
			chains = append(chains, map[string]any{"tag": mustStr(m["tag"]), "type": "chain", "source": "chain", "label": fmt.Sprintf("%s（chain / 链式代理）", mustStr(m["tag"]))})
		}
	}
	manualNodes := []any{}
	for _, n := range getSlice(nr, "manualNodes") {
		if m, ok := n.(map[string]any); ok {
			manualNodes = append(manualNodes, map[string]any{"tag": mustStr(m["tag"]), "type": mustStr(m["type"]), "source": "manual", "label": fmt.Sprintf("%s（%s / 手动）", mustStr(m["tag"]), mustStr(m["type"]))})
		}
	}
	subscriptionNodes := []any{}
	for _, n := range getSlice(sub, "nodes") {
		if m, ok := n.(map[string]any); ok {
			if disabledSubscriptionTags[mustStr(m["tag"])] {
				continue
			}
			subscriptionNodes = append(subscriptionNodes, map[string]any{"tag": mustStr(m["tag"]), "type": mustStr(m["type"]), "source": "subscription", "label": fmt.Sprintf("%s（%s / 订阅）", mustStr(m["tag"]), mustStr(m["type"]))})
		}
	}
	builtins := []any{
		map[string]any{"tag": "proxy", "type": "selector", "source": "builtin", "label": "proxy（自动选择）"},
		map[string]any{"tag": "auto", "type": "urltest", "source": "builtin", "label": "auto（延迟测试）"},
		map[string]any{"tag": "block", "type": "block", "source": "builtin", "label": "block"},
	}
	return append(append(append(append(groups, chains...), manualNodes...), subscriptionNodes...), builtins...)
}
func buildSingBoxConfig(cfg, sub map[string]any) map[string]any {
	nr := getMap(cfg, "nodeRegistry")
	disabledSubscriptionTags := toStringSet(getSlice(nr, "disabledSubscriptionTags"))
	nodes := []any{}
	for _, n := range getSlice(sub, "nodes") {
		m, ok := n.(map[string]any)
		if !ok || disabledSubscriptionTags[mustStr(m["tag"])] {
			continue
		}
		nodes = append(nodes, m)
	}
	nodes = append(nodes, getSlice(nr, "manualNodes")...)

	outbounds := []any{map[string]any{"type": "direct", "tag": "direct"}, map[string]any{"type": "block", "tag": "block"}}
	normalizedNodeMap := map[string]map[string]any{}
	tags := []string{}
	for _, n := range nodes {
		m, ok := n.(map[string]any)
		if !ok {
			continue
		}
		normalized := normalizeOutboundForSingBox(m)
		if normalized == nil {
			continue
		}
		outbounds = append(outbounds, normalized)
		normalizedNodeMap[mustStr(normalized["tag"])] = normalized
		m = normalized
		if t := mustStr(m["tag"]); t != "" {
			tags = append(tags, t)
		}
	}

	groupTags := []string{}
	for _, g := range getSlice(nr, "groups") {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		tag := strings.TrimSpace(mustStr(gm["tag"]))
		if tag == "" {
			continue
		}
		members := []string{}
		for _, m := range getSlice(gm, "members") {
			mtag := strings.TrimSpace(mustStr(m))
			if mtag == "" {
				continue
			}
			if _, ok := normalizedNodeMap[mtag]; ok {
				members = append(members, mtag)
			}
		}
		if len(members) == 0 {
			continue
		}
		strategy := strings.TrimSpace(mustStr(gm["strategy"]))
		if strategy == "fallback" {
			outbounds = append(outbounds, map[string]any{
				"type":                        "selector",
				"tag":                         tag,
				"outbounds":                   toAnySliceString(members),
				"default":                     members[0],
				"interrupt_exist_connections": false,
			})
		} else {
			url := strings.TrimSpace(mustStr(gm["url"]))
			if url == "" {
				url = "https://www.gstatic.com/generate_204"
			}
			interval := strings.TrimSpace(mustStr(gm["interval"]))
			if interval == "" {
				interval = "10m"
			}
			outbounds = append(outbounds, map[string]any{
				"type":      "urltest",
				"tag":       tag,
				"outbounds": toAnySliceString(members),
				"url":       url,
				"interval":  interval,
				"tolerance": 50,
			})
		}
		groupTags = append(groupTags, tag)
	}

	chainTags := []string{}
	for _, c := range getSlice(nr, "chains") {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		chainTag := strings.TrimSpace(mustStr(cm["tag"]))
		if chainTag == "" {
			continue
		}
		members := []string{}
		for _, m := range getSlice(cm, "members") {
			mtag := strings.TrimSpace(mustStr(m))
			if _, ok := normalizedNodeMap[mtag]; ok {
				members = append(members, mtag)
			}
		}
		if len(members) == 0 {
			continue
		}
		previous := ""
		for i, memberTag := range members {
			base := normalizeOutboundForSingBox(normalizedNodeMap[memberTag])
			if base == nil {
				continue
			}
			hopTag := fmt.Sprintf("%s__hop_%d", chainTag, i+1)
			base["tag"] = hopTag
			if previous != "" {
				base["detour"] = previous
			}
			outbounds = append(outbounds, base)
			previous = hopTag
		}
		if previous != "" {
			outbounds = append(outbounds, map[string]any{
				"type":                        "selector",
				"tag":                         chainTag,
				"outbounds":                   []any{previous},
				"default":                     previous,
				"interrupt_exist_connections": false,
			})
			chainTags = append(chainTags, chainTag)
		}
	}

	tags = append(tags, groupTags...)
	tags = append(tags, chainTags...)
	if len(tags) > 0 {
		outbounds = append(outbounds, map[string]any{"type": "selector", "tag": "proxy", "outbounds": tags, "default": tags[0]})
		outbounds = append(outbounds, map[string]any{"type": "urltest", "tag": "auto", "outbounds": tags, "url": "https://www.gstatic.com/generate_204", "interval": "10m", "tolerance": 50})
	}

	inbounds := []any{}
	routeRules := []any{}
	for _, p := range getSlice(cfg, "ports") {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		inboundTag := strings.TrimSpace(mustStr(pm["tag"]))
		if inboundTag == "" {
			continue
		}
		listen := mustStr(pm["listen"])
		listenPort := int(toFloat(pm["port"]))
		target := strings.TrimSpace(mustStr(pm["target"]))
		inbounds = append(inbounds, map[string]any{
			"type":        "socks",
			"tag":         inboundTag,
			"listen":      listen,
			"listen_port": listenPort,
		})
		if target == "" {
			target = "proxy"
		}
		routeRules = append(routeRules, map[string]any{
			"inbound":  []any{inboundTag},
			"outbound": target,
		})
	}

	dnsCfg := getMap(cfg, "dns")
	routing := getMap(cfg, "routing")
	return map[string]any{
		"log":          map[string]any{"level": getString(getMap(cfg, "app"), "logLevel", "info"), "timestamp": true},
		"dns":          map[string]any{"servers": []any{map[string]any{"tag": "dns-remote-default", "type": "https", "server": "cloudflare-dns.com", "path": "/dns-query", "detour": "proxy"}, map[string]any{"tag": "dns-bootstrap", "type": "udp", "server": getString(dnsCfg, "bootstrapServer", "1.1.1.1"), "server_port": 53}, map[string]any{"tag": "dns-direct", "type": "local"}}, "rules": []any{map[string]any{"clash_mode": "Direct", "server": "dns-direct"}, map[string]any{"server": "dns-remote-default"}}, "final": "dns-remote-default", "strategy": getString(dnsCfg, "strategy", "prefer_ipv4")},
		"inbounds":     inbounds,
		"outbounds":    outbounds,
		"route":        map[string]any{"auto_detect_interface": true, "final": getString(routing, "routeFinal", "proxy"), "default_domain_resolver": map[string]any{"server": "dns-bootstrap", "strategy": getString(dnsCfg, "strategy", "prefer_ipv4")}, "rules": routeRules},
		"experimental": map[string]any{"cache_file": map[string]any{"enabled": true, "path": "cache.db", "store_rdrc": true, "store_fakeip": true}, "clash_api": map[string]any{"external_controller": "127.0.0.1:19090", "external_ui": "", "secret": ""}},
	}
}
