package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func fetchSubscription(sub map[string]any) map[string]any {
	urls := []string{}
	for _, v := range getSlice(sub, "urls") {
		s := strings.TrimSpace(mustStr(v))
		if s != "" {
			urls = append(urls, s)
		}
	}
	if len(urls) == 0 {
		if s := strings.TrimSpace(getString(sub, "url", "")); s != "" {
			urls = append(urls, s)
		}
	}
	if len(urls) == 0 {
		return map[string]any{"nodes": []any{}, "raw": "", "warnings": []any{"订阅地址为空"}}
	}

	warnings := []any{}
	rawParts := []string{}
	nodes := []map[string]any{}
	filters := getSlice(sub, "filters")
	client := &http.Client{Timeout: 20 * time.Second}
	for idx, u := range urls {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("user-agent", getString(sub, "userAgent", "sub2socks5-go/0.1.0"))
		resp, err := client.Do(req)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("订阅拉取失败: %s %v", u, err))
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			warnings = append(warnings, fmt.Sprintf("订阅拉取失败: %s HTTP %d", u, resp.StatusCode))
			continue
		}
		txt := string(body)
		rawParts = append(rawParts, "### "+u+"\n"+txt)
		parsed := parseSubscription(txt)
		filterMode := "off"
		filterKeywords := []string{}
		if idx < len(filters) {
			if fm, ok := filters[idx].(map[string]any); ok {
				filterMode = strings.TrimSpace(mustStr(fm["mode"]))
				for _, kw := range getSlice(fm, "keywords") {
					s := strings.TrimSpace(mustStr(kw))
					if s != "" {
						filterKeywords = append(filterKeywords, strings.ToLower(s))
					}
				}
			}
		}
		for _, n := range parsed.nodes {
			if shouldKeepNodeByFilter(n, filterMode, filterKeywords) {
				nodes = append(nodes, n)
			}
		}
		for _, w := range parsed.warnings {
			warnings = append(warnings, "["+u+"] "+w)
		}
	}
	return map[string]any{"nodes": dedupeNodes(nodes), "raw": strings.Join(rawParts, "\n\n"), "warnings": warnings}
}

func shouldKeepNodeByFilter(node map[string]any, mode string, keywords []string) bool {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" || mode == "off" || len(keywords) == 0 {
		return true
	}
	tag := strings.ToLower(strings.TrimSpace(mustStr(node["tag"])))
	matched := false
	for _, kw := range keywords {
		if kw != "" && strings.Contains(tag, kw) {
			matched = true
			break
		}
	}
	if mode == "whitelist" {
		return matched
	}
	if mode == "blacklist" {
		return !matched
	}
	return true
}

type parseResult struct {
	nodes    []map[string]any
	warnings []string
}

var subscriptionLinkRe = regexp.MustCompile(`(?i)(vmess|vless|trojan|ss|socks5|socks|tuic|hysteria2)://[^\s"'<>]+`)

func parseSubscription(raw string) parseResult {
	txt := strings.TrimSpace(raw)
	txt = decodeMaybeBase64Subscription(txt)
	lines := extractSubscriptionLines(txt)
	out := parseResult{nodes: []map[string]any{}, warnings: []string{}}
	for _, line := range lines {
		line = sanitizeSubscriptionLine(line)
		if line == "" {
			continue
		}
		node, err := parseNodeLine(line)
		if err != nil {
			if looksLikeSubscriptionPayload(line) {
				out.warnings = append(out.warnings, "节点解析失败: "+err.Error())
			}
			continue
		}
		out.nodes = append(out.nodes, node)
	}
	if len(out.nodes) == 0 {
		yamlNodes, yamlWarnings := parseClashYAMLSubscription(txt)
		out.nodes = append(out.nodes, yamlNodes...)
		out.warnings = append(out.warnings, yamlWarnings...)
	}
	return out
}

func parseClashYAMLSubscription(raw string) ([]map[string]any, []string) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, nil
	}
	proxies, ok := doc["proxies"].([]any)
	if !ok || len(proxies) == 0 {
		return nil, nil
	}
	nodes := make([]map[string]any, 0, len(proxies))
	warnings := []string{}
	for _, item := range proxies {
		pm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		node, err := parseClashProxy(pm)
		if err != nil {
			warnings = append(warnings, "Clash YAML 节点解析失败: "+err.Error())
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, warnings
}

func parseClashProxy(pm map[string]any) (map[string]any, error) {
	pType := strings.ToLower(strings.TrimSpace(mustStr(pm["type"])))
	tag := strings.TrimSpace(firstNonEmpty(mustStr(pm["name"]), mustStr(pm["tag"])))
	server := strings.TrimSpace(mustStr(pm["server"]))
	port := int(toFloat(pm["port"]))
	if tag == "" {
		tag = firstNonEmpty(server, "clash-node")
	}
	switch pType {
	case "ss":
		return map[string]any{"type": "shadowsocks", "tag": tag, "server": server, "server_port": port, "method": mustStr(pm["cipher"]), "password": mustStr(pm["password"])}, nil
	case "trojan":
		return map[string]any{"type": "trojan", "tag": tag, "server": server, "server_port": port, "password": mustStr(pm["password"]), "tls": map[string]any{"enabled": true, "server_name": firstNonEmpty(mustStr(pm["sni"]), server), "insecure": boolFromAny(pm["skip-cert-verify"])}}, nil
	case "vmess":
		node := map[string]any{"type": "vmess", "tag": tag, "server": server, "server_port": port, "uuid": mustStr(pm["uuid"]), "alter_id": int(toFloat(pm["alterId"])), "security": firstNonEmpty(mustStr(pm["cipher"]), "auto")}
		return node, nil
	case "vless":
		node := map[string]any{"type": "vless", "tag": tag, "server": server, "server_port": port, "uuid": mustStr(pm["uuid"])}
		if flow := strings.TrimSpace(mustStr(pm["flow"])); flow != "" {
			node["flow"] = flow
		}
		node["tls"] = map[string]any{"enabled": true, "server_name": firstNonEmpty(mustStr(pm["servername"]), mustStr(pm["sni"]), server), "insecure": boolFromAny(pm["skip-cert-verify"])}
		return node, nil
	case "tuic":
		tls := map[string]any{
			"enabled":     true,
			"server_name": firstNonEmpty(mustStr(pm["servername"]), mustStr(pm["sni"]), server),
			"insecure":    boolFromAny(pm["skip-cert-verify"]),
		}
		if alpn := strings.TrimSpace(mustStr(pm["alpn"])); alpn != "" {
			tls["alpn"] = splitCSV(alpn)
		} else {
			tls["alpn"] = []any{"h3"}
		}
		node := map[string]any{
			"type":               "tuic",
			"tag":                tag,
			"server":             server,
			"server_port":        port,
			"uuid":               firstNonEmpty(mustStr(pm["uuid"]), mustStr(pm["id"])),
			"password":           firstNonEmpty(mustStr(pm["password"]), mustStr(pm["token"])),
			"congestion_control": firstNonEmpty(mustStr(pm["congestion-controller"]), mustStr(pm["congestion_control"]), "bbr"),
			"tls":                tls,
		}
		return node, nil
	case "hysteria2", "hy2":
		tls := map[string]any{
			"enabled":     true,
			"server_name": firstNonEmpty(mustStr(pm["servername"]), mustStr(pm["sni"]), server),
			"insecure":    boolFromAny(pm["skip-cert-verify"]),
		}
		if alpn := strings.TrimSpace(mustStr(pm["alpn"])); alpn != "" {
			tls["alpn"] = splitCSV(alpn)
		}
		node := map[string]any{
			"type":        "hysteria2",
			"tag":         tag,
			"server":      server,
			"server_port": port,
			"password":    firstNonEmpty(mustStr(pm["password"]), mustStr(pm["auth"]), mustStr(pm["auth-str"]), mustStr(pm["token"])),
			"tls":         tls,
		}
		if up := strings.TrimSpace(firstNonEmpty(mustStr(pm["up"]), mustStr(pm["up_mbps"]), mustStr(pm["upmbps"]))); up != "" {
			node["up_mbps"] = parseRateMbps(up)
		}
		if down := strings.TrimSpace(firstNonEmpty(mustStr(pm["down"]), mustStr(pm["down_mbps"]), mustStr(pm["downmbps"]))); down != "" {
			node["down_mbps"] = parseRateMbps(down)
		}
		if obfs := strings.TrimSpace(mustStr(pm["obfs"])); obfs != "" {
			node["obfs"] = map[string]any{
				"type":     obfs,
				"password": firstNonEmpty(mustStr(pm["obfs-password"]), mustStr(pm["obfs_password"])),
			}
		}
		return node, nil
	case "anytls":
		tls := map[string]any{
			"enabled":     true,
			"server_name": firstNonEmpty(mustStr(pm["servername"]), mustStr(pm["sni"]), server),
			"insecure":    boolFromAny(pm["skip-cert-verify"]),
		}
		node := map[string]any{
			"type":        "anytls",
			"tag":         tag,
			"server":      server,
			"server_port": port,
			"password":    firstNonEmpty(mustStr(pm["password"]), mustStr(pm["id"]), mustStr(pm["uuid"])),
			"tls":         tls,
		}
		return node, nil
	case "http":
		return map[string]any{
			"type":        "http",
			"tag":         tag,
			"server":      server,
			"server_port": firstInt(port, 80),
			"username":    mustStr(pm["username"]),
			"password":    mustStr(pm["password"]),
		}, nil
	case "ssr":
		return map[string]any{
			"type":        "shadowsocks",
			"tag":         tag,
			"server":      server,
			"server_port": port,
			"method":      firstNonEmpty(mustStr(pm["cipher"]), mustStr(pm["method"])),
			"password":    mustStr(pm["password"]),
		}, nil
	case "snell":
		return map[string]any{
			"type":        "snell",
			"tag":         tag,
			"server":      server,
			"server_port": port,
			"psk":         firstNonEmpty(mustStr(pm["psk"]), mustStr(pm["password"])),
			"version":     mustAtoiDefault(mustStr(pm["version"]), 3),
		}, nil
	case "wireguard":
		return map[string]any{
			"type":            "wireguard",
			"tag":             tag,
			"server":          server,
			"server_port":     port,
			"private_key":     mustStr(pm["private-key"]),
			"peer_public_key": firstNonEmpty(mustStr(pm["public-key"]), mustStr(pm["peer-public-key"])),
		}, nil
	case "socks5", "socks":
		return map[string]any{"type": "socks", "tag": tag, "server": server, "server_port": firstInt(port, 1080), "username": mustStr(pm["username"]), "password": mustStr(pm["password"])}, nil
	default:
		fallbackTag := tag
		if !strings.HasPrefix(strings.ToLower(fallbackTag), "[fallback]") {
			fallbackTag = "[fallback] " + fallbackTag
		}
		if server == "" || port <= 0 {
			return nil, fmt.Errorf("不支持的 Clash 类型: %s", pType)
		}
		if user := strings.TrimSpace(mustStr(pm["username"])); user != "" {
			return map[string]any{
				"type":               "http",
				"tag":                fallbackTag,
				"server":             server,
				"server_port":        firstInt(port, 80),
				"username":           user,
				"password":           mustStr(pm["password"]),
				"compat_fallback":    true,
				"compat_origin_type": pType,
			}, nil
		}
		if method := strings.TrimSpace(firstNonEmpty(mustStr(pm["cipher"]), mustStr(pm["method"]))); method != "" {
			return map[string]any{
				"type":               "shadowsocks",
				"tag":                fallbackTag,
				"server":             server,
				"server_port":        port,
				"method":             method,
				"password":           mustStr(pm["password"]),
				"compat_fallback":    true,
				"compat_origin_type": pType,
			}, nil
		}
		return map[string]any{
			"type":               "socks",
			"tag":                fallbackTag,
			"server":             server,
			"server_port":        firstInt(port, 1080),
			"username":           mustStr(pm["username"]),
			"password":           mustStr(pm["password"]),
			"compat_fallback":    true,
			"compat_origin_type": pType,
		}, nil
	}
}

func boolFromAny(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	case float64:
		return t != 0
	default:
		return false
	}
}

func firstInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func parseManualNodeInput(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{"nodes": []any{}, "warnings": []any{"手动导入内容为空"}}
	}
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var v any
		if json.Unmarshal([]byte(raw), &v) == nil {
			arr := []any{}
			switch t := v.(type) {
			case []any:
				arr = t
			default:
				arr = []any{t}
			}
			nodes := []any{}
			warnings := []any{}
			for _, it := range arr {
				m, ok := it.(map[string]any)
				if !ok {
					warnings = append(warnings, "结构化节点解析失败: 节点必须是对象")
					continue
				}
				if r, ok := m["raw"].(string); ok && strings.TrimSpace(r) != "" {
					n, err := parseNodeLine(strings.TrimSpace(r))
					if err != nil {
						warnings = append(warnings, "结构化节点解析失败: "+err.Error())
						continue
					}
					nodes = append(nodes, n)
					continue
				}
				nodes = append(nodes, m)
			}
			return map[string]any{"nodes": nodes, "warnings": warnings}
		}
	}
	pr := parseSubscription(raw)
	nodes := make([]any, 0, len(pr.nodes))
	for _, n := range pr.nodes {
		nodes = append(nodes, n)
	}
	ws := make([]any, 0, len(pr.warnings))
	for _, w := range pr.warnings {
		ws = append(ws, w)
	}
	return map[string]any{"nodes": nodes, "warnings": ws}
}

func parseNodeLine(line string) (map[string]any, error) {
	line = sanitizeSubscriptionLine(line)
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "vless://"), strings.HasPrefix(lower, "trojan://"), strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "tuic://"), strings.HasPrefix(lower, "socks5://"), strings.HasPrefix(lower, "socks://"):
		u, err := url.Parse(line)
		if err != nil {
			return nil, err
		}
		tag := strings.TrimPrefix(u.Fragment, "#")
		if d, err := url.QueryUnescape(tag); err == nil {
			tag = d
		}
		if tag == "" {
			tag = u.Host
		}
		node := map[string]any{"tag": tag, "server": u.Hostname(), "server_port": mustAtoiDefault(u.Port(), 443)}
		switch u.Scheme {
		case "vless":
			node["type"] = "vless"
			node["uuid"] = u.User.Username()
			if flow := strings.TrimSpace(u.Query().Get("flow")); flow != "" {
				node["flow"] = flow
			}
			if tls := buildTLSFromURL(u); tls != nil {
				node["tls"] = tls
			}
			if transport := buildTransportFromURL(u); transport != nil {
				node["transport"] = transport
			}
		case "trojan":
			node["type"] = "trojan"
			node["password"] = u.User.Username()
			if tls := buildTLSFromURL(u); tls != nil {
				node["tls"] = tls
			}
		case "hysteria2":
			node["type"] = "hysteria2"
			node["password"] = firstNonEmpty(u.User.Username(), u.Query().Get("auth"), u.Query().Get("password"), u.Query().Get("token"))
			if tls := buildTLSFromURL(u); tls != nil {
				node["tls"] = tls
			}
			if up := firstNonEmpty(u.Query().Get("upmbps"), u.Query().Get("up_mbps"), u.Query().Get("up")); strings.TrimSpace(up) != "" {
				node["up_mbps"] = parseRateMbps(up)
			}
			if down := firstNonEmpty(u.Query().Get("downmbps"), u.Query().Get("down_mbps"), u.Query().Get("down")); strings.TrimSpace(down) != "" {
				node["down_mbps"] = parseRateMbps(down)
			}
			obfsType := firstNonEmpty(u.Query().Get("obfs"), u.Query().Get("obfs-type"), u.Query().Get("obfsType"))
			obfsPassword := firstNonEmpty(u.Query().Get("obfs-password"), u.Query().Get("obfsPassword"), u.Query().Get("salamander"))
			if strings.TrimSpace(obfsType) != "" {
				node["obfs"] = map[string]any{"type": strings.TrimSpace(obfsType), "password": strings.TrimSpace(obfsPassword)}
			}
		case "tuic":
			node["type"] = "tuic"
			node["uuid"] = u.User.Username()
			p, _ := u.User.Password()
			node["password"] = p
			tls := buildTLSFromURL(u)
			if tls == nil {
				tls = map[string]any{"enabled": true, "server_name": u.Hostname(), "insecure": false}
			}
			if alpn := strings.TrimSpace(u.Query().Get("alpn")); alpn != "" {
				tls["alpn"] = splitCSV(alpn)
			}
			node["tls"] = tls
			if cc := strings.TrimSpace(u.Query().Get("congestion_control")); cc != "" {
				node["congestion_control"] = cc
			} else {
				node["congestion_control"] = "bbr"
			}
			if z := strings.TrimSpace(firstNonEmpty(u.Query().Get("zero_rtt_handshake"), u.Query().Get("0rtt"))); z != "" {
				node["zero_rtt_handshake"] = z == "1" || strings.EqualFold(z, "true") || strings.EqualFold(z, "yes")
			}
		default:
			node["type"] = "socks"
			node["server_port"] = mustAtoiDefault(u.Port(), 1080)
			node["username"] = u.User.Username()
			p, _ := u.User.Password()
			node["password"] = p
		}
		return node, nil
	case strings.HasPrefix(lower, "vmess://"):
		s := strings.TrimPrefix(line, "vmess://")
		b, err := base64.StdEncoding.DecodeString(padBase64(s))
		if err != nil {
			return nil, err
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		node := map[string]any{"type": "vmess", "tag": getString(v, "ps", "vmess"), "server": getString(v, "add", ""), "server_port": mustAtoiDefault(getString(v, "port", "0"), 0), "uuid": getString(v, "id", "")}
		if scy := strings.TrimSpace(getString(v, "scy", "")); scy != "" {
			node["security"] = scy
		} else {
			node["security"] = "auto"
		}
		node["alter_id"] = mustAtoiDefault(getString(v, "aid", "0"), 0)
		if strings.EqualFold(getString(v, "tls", ""), "tls") {
			tls := map[string]any{"enabled": true, "server_name": firstNonEmpty(getString(v, "sni", ""), getString(v, "host", ""), getString(v, "add", ""))}
			if getString(v, "allowInsecure", "") == "1" {
				tls["insecure"] = true
			}
			node["tls"] = tls
		}
		if tr := buildVmessTransport(v); tr != nil {
			node["transport"] = tr
		}
		return node, nil
	case strings.HasPrefix(lower, "ss://"):
		s := strings.TrimPrefix(line, "ss://")
		parts := strings.SplitN(s, "#", 2)
		main := parts[0]
		tag := "shadowsocks"
		if len(parts) == 2 {
			tag, _ = url.QueryUnescape(parts[1])
		}
		if !strings.Contains(main, "@") {
			dec, err := base64.StdEncoding.DecodeString(padBase64(main))
			if err == nil {
				main = string(dec)
			}
		} else {
			parts2 := strings.SplitN(main, "@", 2)
			if len(parts2) == 2 {
				if dec, err := base64.StdEncoding.DecodeString(padBase64(parts2[0])); err == nil {
					if strings.Contains(string(dec), ":") {
						main = string(dec) + "@" + parts2[1]
					}
				}
			}
		}
		u, err := url.Parse("ss://" + main)
		if err != nil {
			return nil, err
		}
		pwd, _ := u.User.Password()
		return map[string]any{"type": "shadowsocks", "tag": tag, "server": u.Hostname(), "server_port": mustAtoiDefault(u.Port(), 0), "method": u.User.Username(), "password": pwd}, nil
	default:
		return nil, fmt.Errorf("不支持的协议")
	}
}

func buildTLSFromURL(u *url.URL) map[string]any {
	q := u.Query()
	security := strings.TrimSpace(q.Get("security"))
	isTLS := u.Scheme == "trojan" || q.Get("tls") == "1" || strings.EqualFold(security, "tls") || strings.EqualFold(security, "reality")
	if !isTLS {
		return nil
	}
	fingerprint := firstNonEmpty(q.Get("fp"), q.Get("fingerprint"), q.Get("client-fingerprint"))
	if fingerprint == "" && strings.EqualFold(security, "reality") {
		fingerprint = "chrome"
	}
	tls := map[string]any{
		"enabled":     true,
		"server_name": firstNonEmpty(q.Get("sni"), u.Hostname()),
		"insecure":    q.Get("allowInsecure") == "1",
	}
	if fingerprint != "" && u.Scheme != "hysteria2" && u.Scheme != "tuic" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	if strings.EqualFold(security, "reality") {
		tls["reality"] = map[string]any{
			"enabled":    true,
			"public_key": emptyToNil(q.Get("pbk")),
			"short_id":   emptyToNil(q.Get("sid")),
		}
	}
	if u.Scheme == "hysteria2" || u.Scheme == "tuic" {
		tls["alpn"] = []any{"h3"}
	}
	return tls
}

func buildTransportFromURL(u *url.URL) map[string]any {
	t := strings.TrimSpace(u.Query().Get("type"))
	if t == "" || t == "tcp" {
		return nil
	}
	q := u.Query()
	switch t {
	case "ws":
		tr := map[string]any{"type": "ws", "path": firstNonEmpty(q.Get("path"), "/")}
		if host := strings.TrimSpace(q.Get("host")); host != "" {
			tr["headers"] = map[string]any{"Host": host}
		}
		return tr
	case "grpc":
		return map[string]any{"type": "grpc", "service_name": q.Get("serviceName")}
	case "http":
		tr := map[string]any{"type": "http", "path": firstNonEmpty(q.Get("path"), "/")}
		if host := strings.TrimSpace(q.Get("host")); host != "" {
			tr["host"] = []any{host}
		}
		return tr
	default:
		return map[string]any{"type": t}
	}
}

func buildVmessTransport(v map[string]any) map[string]any {
	netType := strings.TrimSpace(getString(v, "net", ""))
	switch netType {
	case "ws":
		tr := map[string]any{"type": "ws", "path": firstNonEmpty(getString(v, "path", ""), "/")}
		if host := strings.TrimSpace(getString(v, "host", "")); host != "" {
			tr["headers"] = map[string]any{"Host": host}
		}
		return tr
	case "grpc":
		return map[string]any{"type": "grpc", "service_name": getString(v, "path", "")}
	case "http":
		tr := map[string]any{"type": "http", "path": firstNonEmpty(getString(v, "path", ""), "/")}
		if host := strings.TrimSpace(getString(v, "host", "")); host != "" {
			tr["host"] = []any{host}
		}
		return tr
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func splitCSV(v string) []any {
	parts := strings.Split(v, ",")
	out := make([]any, 0, len(parts))
	for _, part := range parts {
		s := strings.TrimSpace(part)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseRateMbps(v string) int {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSuffix(v, "mbps")
	v = strings.TrimSuffix(v, "m")
	v = strings.TrimSpace(v)
	return mustAtoiDefault(v, 0)
}

func emptyToNil(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func sanitizeSubscriptionLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "\uFEFF")
	line = strings.TrimLeft(line, "`'\"[{(")
	line = strings.TrimRight(line, "`'\"]})],;")
	line = strings.ReplaceAll(line, "&amp;", "&")
	line = strings.Join(strings.Fields(line), "")
	return line
}

func extractSubscriptionLines(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := sanitizeSubscriptionLine(line)
		if clean == "" {
			continue
		}
		matches := subscriptionLinkRe.FindAllString(clean, -1)
		if len(matches) > 0 {
			out = append(out, matches...)
			continue
		}
		nested := decodeBase64Line(clean)
		if nested != "" {
			nestedLines := strings.Split(strings.ReplaceAll(nested, "\r\n", "\n"), "\n")
			for _, nl := range nestedLines {
				nl = sanitizeSubscriptionLine(nl)
				if nl == "" {
					continue
				}
				nm := subscriptionLinkRe.FindAllString(nl, -1)
				if len(nm) > 0 {
					out = append(out, nm...)
				}
			}
			continue
		}
		out = append(out, clean)
	}
	return out
}

func decodeMaybeBase64Subscription(text string) string {
	clean := strings.TrimSpace(text)
	if subscriptionLinkRe.MatchString(clean) {
		return clean
	}
	n := normalizeBase64(clean)
	if n == "" {
		return clean
	}
	b, err := base64.StdEncoding.DecodeString(n)
	if err != nil {
		return clean
	}
	decoded := strings.TrimSpace(string(b))
	if subscriptionLinkRe.MatchString(decoded) {
		return decoded
	}
	return clean
}

func decodeBase64Line(line string) string {
	n := normalizeBase64(line)
	if n == "" {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(n)
	if err != nil {
		return ""
	}
	decoded := strings.TrimSpace(string(b))
	if subscriptionLinkRe.MatchString(decoded) {
		return decoded
	}
	return ""
}

func normalizeBase64(value string) string {
	compact := strings.Join(strings.Fields(value), "")
	if len(compact) < 16 {
		return ""
	}
	for _, ch := range compact {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '/' || ch == '_' || ch == '+' || ch == '=' || ch == '-') {
			return ""
		}
	}
	base := strings.ReplaceAll(strings.ReplaceAll(compact, "-", "+"), "_", "/")
	if len(base)%4 == 1 {
		return ""
	}
	for len(base)%4 != 0 {
		base += "="
	}
	return base
}

func looksLikeSubscriptionPayload(line string) bool {
	if subscriptionLinkRe.MatchString(line) {
		return true
	}
	if len(line) < 16 {
		return false
	}
	for _, ch := range line {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '/' || ch == '_' || ch == '+' || ch == '=' || ch == '-') {
			return false
		}
	}
	return true
}
