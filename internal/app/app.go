package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type App struct {
	mu                  sync.RWMutex
	cfg                 map[string]any
	subState            map[string]any
	runtimeInfo         map[string]any
	proc                *exec.Cmd
	manualStopRequested bool
	autoRestartAttempts int
	plannedKernel       map[string]any
	releaseList         []any
	downloadState       map[string]any
	rootDir             string
	dataDir             string
	runtimeDir          string
	binDir              string
	publicDir           string
	staticFS            fs.FS
	autoUpdateLastRun   map[string]time.Time
}

func Run() error {
	return RunWithStaticFS(nil)
}

func RunWithStaticFS(staticFS fs.FS) error {
	cwd, err := os.Getwd()
	must(err)
	app := &App{
		rootDir:    cwd,
		dataDir:    filepath.Join(cwd, "internal", "data"),
		runtimeDir: filepath.Join(cwd, "internal", "runtime"),
		binDir:     filepath.Join(cwd, "internal", "bin"),
		publicDir:  filepath.Join(cwd, "internal", "public"),
		staticFS:   staticFS,
		runtimeInfo: map[string]any{
			"state":   "stopped",
			"running": false,
			"logs":    []string{},
		},
		plannedKernel:     nil,
		releaseList:       []any{},
		downloadState:     map[string]any{"active": false, "steps": []any{}, "progress": nil, "updatedAt": nil},
		autoUpdateLastRun: map[string]time.Time{},
	}
	must(os.MkdirAll(app.dataDir, 0o755))
	must(os.MkdirAll(app.runtimeDir, 0o755))
	must(os.MkdirAll(app.binDir, 0o755))
	must(app.loadOrInit())

	if getBool(getMap(app.cfg, "app"), "autoStart", false) {
		app.mu.Lock()
		if err := app.startRuntimeLocked(); err != nil {
			app.appendRuntimeLog("auto start failed: " + err.Error())
		}
		app.mu.Unlock()
	}

	go app.runSubscriptionAutoUpdateScheduler()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", app.handleConfig)
	mux.HandleFunc("/api/subscription/refresh", app.handleSubscriptionRefresh)
	mux.HandleFunc("/api/nodes", app.handleNodes)
	mux.HandleFunc("/api/nodes/import", app.handleNodeImport)
	mux.HandleFunc("/api/nodes/check", app.handleNodesCheck)
	mux.HandleFunc("/api/nodes/egress", app.handleNodesEgress)
	mux.HandleFunc("/api/ports/next", app.handleNextPort)
	mux.HandleFunc("/api/runtime/generate", app.handleRuntimeGenerate)
	mux.HandleFunc("/api/runtime/start", app.handleRuntimeStart)
	mux.HandleFunc("/api/runtime/stop", app.handleRuntimeStop)
	mux.HandleFunc("/api/runtime/logs", app.handleRuntimeLogs)
	mux.HandleFunc("/api/runtime/generated", app.handleRuntimeGenerated)
	mux.HandleFunc("/api/kernel/architecture", app.handleKernelArch)
	mux.HandleFunc("/api/kernel/status", app.handleKernelStatus)
	mux.HandleFunc("/api/kernel/releases", app.handleKernelReleases)
	mux.HandleFunc("/api/kernel/releases/update", app.handleKernelReleasesUpdate)
	mux.HandleFunc("/api/kernel/plan", app.handleKernelPlan)
	mux.HandleFunc("/api/kernel/download", app.handleKernelDownload)
	mux.HandleFunc("/", app.handleStatic)

	host := getString(getMap(app.cfg, "app"), "host", "0.0.0.0")
	port := getInt(getMap(app.cfg, "app"), "port", 18080)
	addr := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("Web UI listening on http://%s\n", addr)
	return http.ListenAndServe(addr, withCORS(mux))
}

func (a *App) runSubscriptionAutoUpdateScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		a.mu.Lock()
		a.runSubscriptionAutoUpdateLocked(time.Now())
		a.mu.Unlock()
	}
}

func (a *App) runSubscriptionAutoUpdateLocked(now time.Time) {
	subCfg := getMap(a.cfg, "subscription")
	auto := getMap(subCfg, "autoUpdate")
	scope := strings.TrimSpace(mustStr(auto["scope"]))
	if scope == "" || scope == "off" {
		return
	}

	if scope == "simultaneous" {
		if !shouldRunAutoUpdate(now, a.autoUpdateLastRun["simultaneous"], auto) {
			return
		}
		if err := a.refreshSubscriptionLocked("auto-update(simultaneous)"); err != nil {
			a.appendRuntimeLog("auto update failed: " + err.Error())
			return
		}
		a.autoUpdateLastRun["simultaneous"] = now
		a.appendRuntimeLog("auto update completed (simultaneous)")
		return
	}

	if scope == "independent" {
		urls := normalizeSubscriptionURLs(subCfg)
		items := getSlice(auto, "items")
		if len(urls) == 0 || len(items) == 0 {
			return
		}
		updated := false
		for idx := 0; idx < len(urls) && idx < len(items); idx += 1 {
			item, ok := items[idx].(map[string]any)
			if !ok {
				continue
			}
			key := fmt.Sprintf("independent:%d", idx)
			if !shouldRunAutoUpdate(now, a.autoUpdateLastRun[key], item) {
				continue
			}

			localSub := cloneMap(subCfg)
			localSub["url"] = urls[idx]
			localSub["urls"] = []any{urls[idx]}
			st := fetchSubscription(localSub)
			st["updatedAt"] = now.Format(time.RFC3339)
			a.subState = mergeSubscriptionState(a.subState, st)
			a.autoUpdateLastRun[key] = now
			updated = true
			a.appendRuntimeLog(fmt.Sprintf("auto update completed (independent #%d)", idx+1))
		}
		if updated {
			_ = writeJSON(filepath.Join(a.dataDir, "subscription-state.json"), a.subState)
		}
	}
}

func shouldRunAutoUpdate(now, last time.Time, cfg map[string]any) bool {
	mode := strings.TrimSpace(mustStr(cfg["mode"]))
	if mode == "" {
		mode = "interval"
	}
	if mode == "interval" {
		minutes := int(toFloat(cfg["intervalMinutes"]))
		if minutes <= 0 {
			minutes = 60
		}
		if last.IsZero() {
			return true
		}
		return now.Sub(last) >= time.Duration(minutes)*time.Minute
	}

	if mode == "schedule" {
		timeText := strings.TrimSpace(mustStr(cfg["time"]))
		if timeText == "" {
			timeText = "03:00"
		}
		parts := strings.Split(timeText, ":")
		if len(parts) != 2 {
			return false
		}
		hh, err1 := strconv.Atoi(parts[0])
		mm, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
			return false
		}
		target := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
		if now.Before(target) {
			return false
		}

		dayMode := strings.TrimSpace(mustStr(cfg["dayMode"]))
		if dayMode == "" {
			dayMode = "daily"
		}
		if last.IsZero() {
			return true
		}
		lastDay := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, last.Location())
		nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		days := int(nowDay.Sub(lastDay).Hours() / 24)
		switch dayMode {
		case "daily":
			return days >= 1
		case "every3days":
			return days >= 3
		case "weekly":
			return days >= 7
		default:
			return days >= 1
		}
	}

	return false
}

func normalizeSubscriptionURLs(sub map[string]any) []string {
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
	return urls
}

func mergeSubscriptionState(base, incoming map[string]any) map[string]any {
	out := map[string]any{"raw": "", "nodes": []any{}, "warnings": []any{}, "updatedAt": nil}
	if base != nil {
		out = cloneMap(base)
	}
	nodes := map[string]map[string]any{}
	appendNodes := func(items []any) {
		for _, n := range items {
			m, ok := n.(map[string]any)
			if !ok {
				continue
			}
			tag := strings.TrimSpace(mustStr(m["tag"]))
			if tag == "" {
				continue
			}
			nodes[tag] = m
		}
	}
	appendNodes(getSlice(out, "nodes"))
	appendNodes(getSlice(incoming, "nodes"))
	mergedNodes := make([]any, 0, len(nodes))
	for _, n := range nodes {
		mergedNodes = append(mergedNodes, n)
	}
	sort.SliceStable(mergedNodes, func(i, j int) bool {
		mi, _ := mergedNodes[i].(map[string]any)
		mj, _ := mergedNodes[j].(map[string]any)
		return mustStr(mi["tag"]) < mustStr(mj["tag"])
	})
	out["nodes"] = mergedNodes

	warns := []any{}
	warns = append(warns, getSlice(out, "warnings")...)
	warns = append(warns, getSlice(incoming, "warnings")...)
	out["warnings"] = warns
	out["updatedAt"] = incoming["updatedAt"]
	out["raw"] = incoming["raw"]
	return out
}

func (a *App) refreshSubscriptionLocked(reason string) error {
	subCfg := getMap(a.cfg, "subscription")
	st := fetchSubscription(subCfg)
	st["updatedAt"] = time.Now().Format(time.RFC3339)
	a.subState = st
	if err := writeJSON(filepath.Join(a.dataDir, "subscription-state.json"), st); err != nil {
		return err
	}
	a.appendRuntimeLog("subscription refreshed: " + reason)
	return nil
}

func (a *App) loadOrInit() error {
	cfgPath := filepath.Join(a.dataDir, "app-config.json")
	subPath := filepath.Join(a.dataDir, "subscription-state.json")
	archPath := filepath.Join(a.dataDir, "architecture-info.json")
	plannedPath := filepath.Join(a.dataDir, "planned-kernel-info.json")
	releasePath := filepath.Join(a.dataDir, "release-list.json")
	generatedPath := filepath.Join(a.runtimeDir, "sing-box.json")

	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		a.cfg = defaultConfig()
		if err := writeJSON(cfgPath, a.cfg); err != nil {
			return err
		}
	} else {
		var cfg map[string]any
		if err := readJSON(cfgPath, &cfg); err != nil {
			return err
		}
		a.cfg = mergeMap(defaultConfig(), cfg)
	}
	var cfgChanged bool
	a.cfg, cfgChanged = normalizeAppConfig(a.cfg)
	if cfgChanged {
		if err := writeJSON(cfgPath, a.cfg); err != nil {
			return err
		}
	}

	if _, err := os.Stat(subPath); errors.Is(err, os.ErrNotExist) {
		a.subState = map[string]any{"raw": "", "nodes": []any{}, "warnings": []any{}, "updatedAt": nil}
		if err := writeJSON(subPath, a.subState); err != nil {
			return err
		}
	} else {
		var st map[string]any
		if err := readJSON(subPath, &st); err != nil {
			return err
		}
		a.subState = st
	}

	if _, err := os.Stat(archPath); errors.Is(err, os.ErrNotExist) {
		if err := writeJSON(archPath, detectPlatform()); err != nil {
			return err
		}
	}

	if _, err := os.Stat(plannedPath); errors.Is(err, os.ErrNotExist) {
		if err := writeJSON(plannedPath, nil); err != nil {
			return err
		}
	} else {
		var planned map[string]any
		if err := readJSON(plannedPath, &planned); err == nil {
			a.plannedKernel = planned
		}
	}

	if _, err := os.Stat(releasePath); errors.Is(err, os.ErrNotExist) {
		if err := writeJSON(releasePath, []any{}); err != nil {
			return err
		}
	} else {
		var releases []any
		if err := readJSON(releasePath, &releases); err == nil {
			a.releaseList = releases
		}
	}

	if _, err := os.Stat(generatedPath); errors.Is(err, os.ErrNotExist) || cfgChanged {
		generated := buildSingBoxConfig(a.cfg, a.subState)
		if err := writeJSON(generatedPath, generated); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.mu.RLock()
		defer a.mu.RUnlock()
		ok(w, map[string]any{
			"config":             a.cfg,
			"subscription":       a.subState,
			"availableOutbounds": collectOutbounds(a.cfg, a.subState),
			"runtime":            a.runtimeInfo,
			"kernel":             a.kernelStatus(),
			"architecture":       detectPlatform(),
			"plannedKernel":      a.plannedKernel,
			"releaseList":        a.releaseList,
			"download":           a.downloadState,
		})
	case http.MethodPost:
		var body map[string]any
		if err := decodeJSON(r.Body, &body); err != nil {
			fail(w, 400, err.Error())
			return
		}
		skipRuntimeRestart := strings.TrimSpace(r.Header.Get("x-skip-runtime-restart")) == "1"
		a.mu.Lock()
		a.cfg, _ = normalizeAppConfig(body)
		_ = writeJSON(filepath.Join(a.dataDir, "app-config.json"), a.cfg)
		generated := buildSingBoxConfig(a.cfg, a.subState)
		_ = writeJSON(filepath.Join(a.runtimeDir, "sing-box.json"), generated)
		wasRunning := a.proc != nil && a.proc.Process != nil
		if wasRunning && !skipRuntimeRestart {
			if err := a.startRuntimeLocked(); err != nil {
				a.appendRuntimeLog("apply config failed: " + err.Error())
				a.mu.Unlock()
				fail(w, 500, err.Error())
				return
			}
			a.appendRuntimeLog("config applied and runtime restarted")
		}
		runtimeState := a.runtimeInfo
		a.mu.Unlock()
		ok(w, map[string]any{"ok": true, "generated": generated, "runtime": runtimeState})
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (a *App) handleSubscriptionRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refreshSubscriptionLocked("manual"); err != nil {
		fail(w, 500, err.Error())
		return
	}
	ok(w, a.subState)
}

func (a *App) handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.mu.RLock()
		defer a.mu.RUnlock()
		nr := getMap(a.cfg, "nodeRegistry")
		disabled := toStringSet(getSlice(nr, "disabledSubscriptionTags"))
		nodes := []any{}
		for _, n := range getSlice(a.subState, "nodes") {
			m, okk := n.(map[string]any)
			if !okk || disabled[mustStr(m["tag"])] {
				continue
			}
			nodes = append(nodes, m)
		}
		ok(w, map[string]any{
			"subscriptionNodes":        nodes,
			"disabledSubscriptionTags": getSlice(nr, "disabledSubscriptionTags"),
			"manualNodes":              getSlice(nr, "manualNodes"),
			"groups":                   getSlice(nr, "groups"),
			"chains":                   getSlice(nr, "chains"),
			"availableOutbounds":       collectOutbounds(a.cfg, a.subState),
			"fallbackStates":           map[string]any{},
		})
	case http.MethodPost:
		var body map[string]any
		if err := decodeJSON(r.Body, &body); err != nil {
			fail(w, 400, err.Error())
			return
		}
		a.mu.Lock()
		nr := getMap(a.cfg, "nodeRegistry")
		nr["manualNodes"] = getSlice(body, "manualNodes")
		nr["groups"] = getSlice(body, "groups")
		nr["chains"] = getSlice(body, "chains")
		nr["disabledSubscriptionTags"] = getSlice(body, "disabledSubscriptionTags")
		a.cfg["nodeRegistry"] = nr
		a.cfg, _ = normalizeAppConfig(a.cfg)
		_ = writeJSON(filepath.Join(a.dataDir, "app-config.json"), a.cfg)
		if a.proc != nil && a.proc.Process != nil {
			if err := a.startRuntimeLocked(); err != nil {
				a.appendRuntimeLog("apply node config failed: " + err.Error())
				a.mu.Unlock()
				fail(w, 500, err.Error())
				return
			}
			a.appendRuntimeLog("node config applied and runtime restarted")
		}
		outbounds := collectOutbounds(a.cfg, a.subState)
		a.mu.Unlock()
		ok(w, map[string]any{"ok": true, "manualNodes": nr["manualNodes"], "groups": nr["groups"], "chains": nr["chains"], "availableOutbounds": outbounds})
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (a *App) handleNodeImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	res := parseManualNodeInput(mustStr(body["raw"]))
	ok(w, res)
}

func (a *App) handleNodesCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	tags := []string{}
	for _, t := range getSlice(body, "tags") {
		s := strings.TrimSpace(mustStr(t))
		if s != "" {
			tags = append(tags, s)
		}
	}
	if len(tags) == 0 {
		fail(w, 400, "Missing node tags for check")
		return
	}
	urlToTest := mustStr(body["url"])
	if urlToTest == "" {
		urlToTest = "https://www.gstatic.com/generate_204"
	}
	timeout := int(toFloat(body["timeoutMs"]))
	if timeout <= 0 {
		timeout = 5000
	}
	results := map[string]any{}
	for _, tag := range tags {
		delay, err := measureProxyDelay(tag, urlToTest, timeout)
		if err != nil {
			results[tag] = map[string]any{"ok": false, "text": "失败", "error": err.Error(), "checkedAt": time.Now().Format(time.RFC3339), "checkedTag": tag}
			continue
		}
		results[tag] = map[string]any{"ok": true, "delay": delay, "text": fmt.Sprintf("%d ms", delay), "checkedAt": time.Now().Format(time.RFC3339), "checkedTag": tag}
	}
	ok(w, map[string]any{"ok": true, "url": urlToTest, "timeoutMs": timeout, "results": results})
}

func (a *App) handleNextPort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	host := mustStr(body["host"])
	if host == "" {
		host = "127.0.0.1"
	}
	start := int(toFloat(body["start"]))
	if start <= 0 {
		fail(w, 400, "Invalid start port")
		return
	}
	p := findPort(host, start)
	ok(w, map[string]any{"host": host, "port": p})
}

func (a *App) handleRuntimeGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	a.mu.RLock()
	generated := buildSingBoxConfig(a.cfg, a.subState)
	a.mu.RUnlock()
	_ = writeJSON(filepath.Join(a.runtimeDir, "sing-box.json"), generated)
	ok(w, map[string]any{"ok": true, "path": filepath.Join("internal", "runtime", "sing-box.json"), "generated": generated})
}

func (a *App) handleRuntimeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.startRuntimeLocked(); err != nil {
		fail(w, 400, err.Error())
		return
	}
	rt := a.runtimeInfo
	ok(w, rt)
}

func (a *App) startRuntimeLocked() error {
	if err := ensureNodesLoaded(a.cfg, a.subState); err != nil {
		return err
	}
	generated := buildSingBoxConfig(a.cfg, a.subState)
	cfgPath := filepath.Join(a.runtimeDir, "sing-box.json")
	_ = writeJSON(cfgPath, generated)
	if a.proc != nil && a.proc.Process != nil {
		_ = a.proc.Process.Kill()
		a.proc = nil
	}
	bin, err := a.resolveSingBoxBinaryPathLocked()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "run", "-c", cfgPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	a.manualStopRequested = false
	a.autoRestartAttempts = 0
	a.proc = cmd
	a.runtimeInfo["state"] = "running"
	a.runtimeInfo["running"] = true
	a.appendRuntimeLog("sing-box started")
	go a.captureLogs(stdout)
	go a.captureLogs(stderr)
	go func(c *exec.Cmd) {
		waitErr := c.Wait()
		a.mu.Lock()
		if a.proc == c {
			a.proc = nil
			a.runtimeInfo["state"] = "stopped"
			a.runtimeInfo["running"] = false
			if waitErr != nil {
				a.appendRuntimeLog("sing-box exited with error: " + waitErr.Error())
			} else {
				a.appendRuntimeLog("sing-box exited")
			}
			if !a.manualStopRequested {
				a.autoRestartAttempts += 1
				attempt := a.autoRestartAttempts
				delay := time.Duration(attempt*2) * time.Second
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
				a.appendRuntimeLog(fmt.Sprintf("runtime stopped unexpectedly, auto-restart in %ds (attempt %d)", int(delay/time.Second), attempt))
				go a.autoRestartAfter(delay)
			}
		}
		a.mu.Unlock()
	}(cmd)
	return nil
}

func (a *App) autoRestartAfter(delay time.Duration) {
	time.Sleep(delay)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.proc != nil || a.manualStopRequested {
		return
	}
	if err := a.startRuntimeLocked(); err != nil {
		a.appendRuntimeLog("auto restart failed: " + err.Error())
	}
}

func (a *App) handleRuntimeStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	a.mu.Lock()
	a.manualStopRequested = true
	a.autoRestartAttempts = 0
	if a.proc != nil && a.proc.Process != nil {
		_ = a.proc.Process.Kill()
		a.proc = nil
	}
	a.runtimeInfo["state"] = "stopped"
	a.runtimeInfo["running"] = false
	a.appendRuntimeLog("runtime stop requested")
	rt := a.runtimeInfo
	a.mu.Unlock()
	ok(w, rt)
}

func (a *App) handleRuntimeLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	ok(w, a.runtimeInfo)
}

func (a *App) handleRuntimeGenerated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	p := filepath.Join(a.runtimeDir, "sing-box.json")
	b, err := os.ReadFile(p)
	if err != nil {
		ok(w, map[string]any{})
		return
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		ok(w, map[string]any{})
		return
	}
	ok(w, v)
}

func (a *App) handleKernelArch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost:
		arch := detectPlatform()
		_ = writeJSON(filepath.Join(a.dataDir, "architecture-info.json"), arch)
		if r.Method == http.MethodPost {
			if latest, err := getLatestRelease(arch); err == nil {
				a.mu.Lock()
				a.plannedKernel = latest
				_ = writeJSON(filepath.Join(a.dataDir, "planned-kernel-info.json"), latest)
				a.mu.Unlock()
			}
		}
		a.mu.RLock()
		planned := a.plannedKernel
		a.mu.RUnlock()
		ok(w, map[string]any{"architecture": arch, "stored": true, "plannedKernel": planned, "kernel": a.kernelStatus()})
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (a *App) handleKernelStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	ok(w, a.kernelStatus())
}

func (a *App) handleKernelReleases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	a.mu.RLock()
	if len(a.releaseList) > 0 {
		cached := a.releaseList
		a.mu.RUnlock()
		ok(w, cached)
		return
	}
	a.mu.RUnlock()
	releases, err := listReleases(detectPlatform())
	if err != nil {
		a.mu.RLock()
		cached := a.releaseList
		a.mu.RUnlock()
		ok(w, cached)
		return
	}
	a.mu.Lock()
	a.releaseList = releases
	_ = writeJSON(filepath.Join(a.dataDir, "release-list.json"), releases)
	a.mu.Unlock()
	ok(w, releases)
}

func (a *App) handleKernelReleasesUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	releases, err := listReleases(detectPlatform())
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	a.mu.Lock()
	a.releaseList = releases
	_ = writeJSON(filepath.Join(a.dataDir, "release-list.json"), releases)
	if len(releases) > 0 {
		if p, okk := releases[0].(map[string]any); okk {
			a.plannedKernel = p
			_ = writeJSON(filepath.Join(a.dataDir, "planned-kernel-info.json"), p)
		}
	}
	planned := a.plannedKernel
	a.mu.Unlock()
	ok(w, map[string]any{"releaseList": releases, "plannedKernel": planned})
}

func (a *App) handleKernelPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	_ = decodeJSON(r.Body, &body)
	version := mustStr(body["version"])
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, item := range a.releaseList {
		m, okk := item.(map[string]any)
		if okk && mustStr(m["version"]) == version {
			a.plannedKernel = m
			_ = writeJSON(filepath.Join(a.dataDir, "planned-kernel-info.json"), m)
			ok(w, m)
			return
		}
	}
	fail(w, 404, "Requested kernel version not found")
}

func (a *App) handleKernelDownload(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.mu.RLock()
		st := a.downloadState
		a.mu.RUnlock()
		ok(w, st)
	case http.MethodPost:
		a.mu.Lock()
		planned := a.plannedKernel
		a.downloadState = map[string]any{"active": true, "steps": []any{}, "progress": map[string]any{"percent": 0, "stage": "prepare", "message": "preparing"}, "updatedAt": time.Now().Format(time.RFC3339)}
		a.pushDownloadStepLocked("prepare", "Prepared download workspace", map[string]any{})
		a.mu.Unlock()
		if planned == nil {
			fail(w, 400, "No planned kernel selected")
			return
		}
		result, err := a.downloadKernel(planned)
		if err != nil {
			a.mu.Lock()
			a.downloadState = map[string]any{"active": false, "steps": []any{map[string]any{"stage": "error", "message": err.Error()}}, "progress": map[string]any{"percent": nil, "stage": "error", "message": err.Error()}, "updatedAt": time.Now().Format(time.RFC3339)}
			ds := a.downloadState
			a.mu.Unlock()
			fail(w, 500, err.Error())
			_ = ds
			return
		}
		a.mu.Lock()
		a.downloadState = map[string]any{"active": false, "steps": []any{map[string]any{"stage": "done", "message": "Kernel installation completed"}}, "progress": map[string]any{"percent": 100, "stage": "done", "message": "Kernel installation completed"}, "updatedAt": time.Now().Format(time.RFC3339)}
		ds := a.downloadState
		a.mu.Unlock()
		ok(w, map[string]any{"result": result, "kernel": a.kernelStatus(), "download": ds})
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (a *App) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, "GET, HEAD")
		return
	}
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") {
		fail(w, 404, "API route not found")
		return
	}
	if p == "/" {
		p = "/index.html"
	}
	clean := strings.TrimPrefix(path.Clean(p), "/")
	var (
		b   []byte
		err error
	)
	if a.staticFS != nil {
		b, err = fs.ReadFile(a.staticFS, clean)
	} else {
		full := filepath.Join(a.publicDir, filepath.FromSlash(clean))
		if !strings.HasPrefix(full, a.publicDir) {
			http.Error(w, "Forbidden", 403)
			return
		}
		b, err = os.ReadFile(full)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := "text/plain; charset=utf-8"
	switch filepath.Ext(clean) {
	case ".html":
		ct = "text/html; charset=utf-8"
	case ".css":
		ct = "text/css; charset=utf-8"
	case ".js":
		ct = "application/javascript; charset=utf-8"
	}
	w.Header().Set("content-type", ct)
	_, _ = w.Write(b)
}

func (a *App) kernelStatus() map[string]any {
	exe := "sing-box"
	if runtime.GOOS == "windows" {
		exe = "sing-box.exe"
	}
	p := filepath.Join(a.binDir, exe)
	_, err := os.Stat(p)
	installed := err == nil
	var releaseInfo any = nil
	verFile := filepath.Join(a.binDir, "sing-box-version.json")
	if b, err := os.ReadFile(verFile); err == nil {
		var tmp any
		if json.Unmarshal(b, &tmp) == nil {
			releaseInfo = tmp
		}
	}
	return map[string]any{"installed": installed, "binaryPath": p, "platform": detectPlatform(), "releaseInfo": releaseInfo}
}

func (a *App) appendRuntimeLog(msg string) {
	logs := getStringSlice(a.runtimeInfo, "logs")
	logs = append(logs, time.Now().Format(time.RFC3339)+" "+msg)
	if len(logs) > 1000 {
		logs = logs[len(logs)-1000:]
	}
	a.runtimeInfo["logs"] = logs
}

func (a *App) captureLogs(r io.ReadCloser) {
	defer r.Close()
	buf := make([]byte, 4096)
	acc := ""
	for {
		n, err := r.Read(buf)
		if n > 0 {
			acc += string(buf[:n])
			for {
				i := strings.IndexAny(acc, "\r\n")
				if i < 0 {
					break
				}
				line := strings.TrimSpace(acc[:i])
				acc = strings.TrimLeft(acc[i+1:], "\r\n")
				if line != "" {
					a.mu.Lock()
					a.appendRuntimeLog(line)
					a.mu.Unlock()
				}
			}
		}
		if err != nil {
			break
		}
	}
}

func resolveManagedPath(rootDir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(rootDir, filepath.FromSlash(p))
}

func measureProxyDelay(tag, testURL string, timeoutMs int) (int, error) {
	endpoint := fmt.Sprintf("http://127.0.0.1:19090/proxies/%s/delay?url=%s&timeout=%d", url.QueryEscape(tag), url.QueryEscape(testURL), timeoutMs)
	client := &http.Client{Timeout: time.Duration(timeoutMs+1500) * time.Millisecond}
	resp, err := client.Get(endpoint)
	if err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "connection refused"):
			return 0, fmt.Errorf("测速控制接口未就绪（connection refused），请先确认 sing-box 已正常启动")
		case strings.Contains(msg, "timeout"):
			return 0, fmt.Errorf("测速控制接口请求超时，请稍后重试")
		default:
			return 0, fmt.Errorf("测速控制接口未就绪，请先确认 sing-box 已正常启动")
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		text := strings.TrimSpace(string(body))
		if text != "" {
			return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, text)
		}
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	delay := int(toFloat(data["delay"]))
	if delay < 0 {
		return 0, fmt.Errorf("No delay data")
	}
	return delay, nil
}

func (a *App) handleNodesEgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		fail(w, 400, err.Error())
		return
	}
	tags := []string{}
	for _, t := range getSlice(body, "tags") {
		s := strings.TrimSpace(mustStr(t))
		if s != "" {
			tags = append(tags, s)
		}
	}
	if len(tags) == 0 {
		fail(w, 400, "Missing node tags")
		return
	}
	timeout := int(toFloat(body["timeoutMs"]))
	if timeout <= 0 {
		timeout = 5000
	}

	a.mu.RLock()
	cfg := cloneMap(a.cfg)
	a.mu.RUnlock()
	host, port, err := pickProxySocksPort(cfg)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}

	results := map[string]any{}
	for _, tag := range tags {
		if err := clashSelectProxy("proxy", tag, timeout); err != nil {
			results[tag] = map[string]any{"ok": false, "error": err.Error()}
			continue
		}
		time.Sleep(120 * time.Millisecond)
		ip, ipErr := fetchIPViaSocks(host, port, timeout)
		if ipErr != nil {
			results[tag] = map[string]any{"ok": false, "error": ipErr.Error()}
			continue
		}
		results[tag] = map[string]any{"ok": true, "egressIP": ip}
	}
	ok(w, map[string]any{"ok": true, "results": results})
}

func pickProxySocksPort(cfg map[string]any) (string, int, error) {
	host := "127.0.0.1"
	port := 0
	for _, item := range getSlice(cfg, "ports") {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if mustStr(m["target"]) != "proxy" {
			continue
		}
		if h := strings.TrimSpace(mustStr(m["listen"])); h != "" {
			host = h
		}
		p := int(toFloat(m["port"]))
		if p > 0 {
			port = p
			break
		}
	}
	if port <= 0 {
		return "", 0, fmt.Errorf("no socks5 service targeting proxy found")
	}
	return host, port, nil
}

func clashSelectProxy(groupTag, selectedTag string, timeoutMs int) error {
	endpoint := fmt.Sprintf("http://127.0.0.1:19090/proxies/%s", url.QueryEscape(groupTag))
	payload, _ := json.Marshal(map[string]any{"name": selectedTag})
	req, _ := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(payload))
	req.Header.Set("content-type", "application/json")
	client := &http.Client{Timeout: time.Duration(timeoutMs+1500) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("selector update failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func fetchIPViaSocks(socksHost string, socksPort int, timeoutMs int) (string, error) {
	targets := []struct {
		host string
		path string
	}{
		{host: "api.ipify.org", path: "/?format=text"},
		{host: "ipv4.icanhazip.com", path: "/"},
		{host: "ifconfig.me", path: "/ip"},
		{host: "api.ip.sb", path: "/ip"},
	}

	var lastErr error
	for _, target := range targets {
		ip, err := fetchIPViaSocksTarget(socksHost, socksPort, timeoutMs, target.host, target.path)
		if err == nil && strings.TrimSpace(ip) != "" {
			return strings.TrimSpace(ip), nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no egress ip endpoint available")
}

func fetchIPViaSocksTarget(socksHost string, socksPort int, timeoutMs int, domain string, reqPath string) (string, error) {
	address := net.JoinHostPort(socksHost, strconv.Itoa(socksPort))
	conn, err := net.DialTimeout("tcp", address, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Duration(timeoutMs+1500) * time.Millisecond))

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return "", err
	}
	hello := make([]byte, 2)
	if _, err := io.ReadFull(conn, hello); err != nil {
		return "", err
	}
	if hello[0] != 0x05 || hello[1] == 0xff {
		return "", fmt.Errorf("socks handshake failed")
	}

	domainBytes := []byte(domain)
	request := make([]byte, 0, 7+len(domainBytes))
	request = append(request, 0x05, 0x01, 0x00, 0x03, byte(len(domainBytes)))
	request = append(request, domainBytes...)
	request = append(request, 0x00, 0x50)
	if _, err := conn.Write(request); err != nil {
		return "", err
	}

	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return "", err
	}
	if head[1] != 0x00 {
		return "", fmt.Errorf("socks connect failed: %d", int(head[1]))
	}
	switch head[3] {
	case 0x01:
		skip := make([]byte, 6)
		if _, err := io.ReadFull(conn, skip); err != nil {
			return "", err
		}
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", err
		}
		skip := make([]byte, int(length[0])+2)
		if _, err := io.ReadFull(conn, skip); err != nil {
			return "", err
		}
	case 0x04:
		skip := make([]byte, 18)
		if _, err := io.ReadFull(conn, skip); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown atyp: %d", int(head[3]))
	}

	rawReq := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: sub2socks5-go/0.1.0\r\nConnection: close\r\n\r\n", reqPath, domain)
	if _, err := conn.Write([]byte(rawReq)); err != nil {
		return "", err
	}
	responseBytes, err := io.ReadAll(conn)
	if err != nil {
		return "", err
	}
	text := string(responseBytes)
	if idx := strings.Index(text, "\r\n\r\n"); idx >= 0 {
		text = text[idx+4:]
	}
	ip := strings.TrimSpace(strings.Split(text, "\n")[0])
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid egress ip response: %q", ip)
	}
	return ip, nil
}

func listReleases(platform map[string]any) ([]any, error) {
	req, _ := newGitHubRequest(http.MethodGet, "https://api.github.com/repos/SagerNet/sing-box/releases?per_page=20")
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Failed to fetch sing-box releases: HTTP %d", resp.StatusCode)
	}
	var raw []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []any{}
	suffix := mustStr(platform["assetSuffix"])
	for _, rel := range raw {
		assets, _ := rel["assets"].([]any)
		asset := pickAsset(assets, suffix)
		if asset == nil {
			continue
		}
		out = append(out, map[string]any{
			"version":     mustStr(rel["tag_name"]),
			"publishedAt": mustStr(rel["published_at"]),
			"assetName":   mustStr(asset["name"]),
			"downloadUrl": mustStr(asset["browser_download_url"]),
			"size":        int(toFloat(asset["size"])),
			"platform":    platform,
		})
	}
	return out, nil
}

func getLatestRelease(platform map[string]any) (map[string]any, error) {
	req, _ := newGitHubRequest(http.MethodGet, "https://api.github.com/repos/SagerNet/sing-box/releases/latest")
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Failed to fetch sing-box latest release: HTTP %d", resp.StatusCode)
	}
	var rel map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	assets, _ := rel["assets"].([]any)
	suffix := mustStr(platform["assetSuffix"])
	asset := pickAsset(assets, suffix)
	if asset == nil {
		return nil, fmt.Errorf("No asset found for %s", suffix)
	}
	return map[string]any{
		"version":     mustStr(rel["tag_name"]),
		"publishedAt": mustStr(rel["published_at"]),
		"assetName":   mustStr(asset["name"]),
		"downloadUrl": mustStr(asset["browser_download_url"]),
		"size":        int(toFloat(asset["size"])),
		"platform":    platform,
	}, nil
}

func newGitHubRequest(method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", "sub2socks5-go/0.1.0")
	req.Header.Set("accept", "application/vnd.github+json")
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("authorization", "Bearer "+token)
	}
	return req, nil
}

func pickAsset(assets []any, suffix string) map[string]any {
	for _, a := range assets {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		name := strings.ToLower(mustStr(m["name"]))
		if strings.Contains(name, strings.ToLower(suffix)) && (strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz")) && !strings.Contains(name, "lite") {
			if strings.Contains(name, "legacy") || strings.Contains(name, "windows-7") {
				continue
			}
			return m
		}
	}
	for _, a := range assets {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		name := strings.ToLower(mustStr(m["name"]))
		if strings.Contains(name, strings.ToLower(suffix)) && (strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz")) {
			return m
		}
	}
	return nil
}

func (a *App) downloadKernel(release map[string]any) (map[string]any, error) {
	urlStr := mustStr(release["downloadUrl"])
	assetName := mustStr(release["assetName"])
	if urlStr == "" || assetName == "" {
		return nil, fmt.Errorf("Missing release information for download")
	}
	tmpDir, err := os.MkdirTemp("", "sub2socks5-go-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	archivePath := filepath.Join(tmpDir, assetName)
	a.mu.Lock()
	a.pushDownloadStepLocked("prepare", "Download workspace ready", map[string]any{"assetName": assetName})
	a.mu.Unlock()
	req, _ := http.NewRequest(http.MethodGet, urlStr, nil)
	req.Header.Set("user-agent", "sub2socks5-go/0.1.0")
	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Failed to download sing-box: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(archivePath)
	if err != nil {
		return nil, err
	}
	total := toFloat(resp.ContentLength)
	if total <= 0 {
		total = toFloat(release["size"])
	}
	buf := make([]byte, 64*1024)
	var downloaded float64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				f.Close()
				return nil, wErr
			}
			downloaded += float64(n)
			percent := any(nil)
			if total > 0 {
				percent = float64(int((downloaded/total)*10000)) / 100
			}
			a.mu.Lock()
			a.pushDownloadStepLocked("download", "Downloading kernel archive", map[string]any{
				"downloadedBytes": int(downloaded),
				"totalBytes":      int(total),
				"percent":         percent,
				"threads":         1,
			})
			a.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			return nil, readErr
		}
	}
	_ = f.Close()
	extractDir := filepath.Join(tmpDir, "extract")
	_ = os.MkdirAll(extractDir, 0o755)
	a.mu.Lock()
	a.pushDownloadStepLocked("extract", "Extracting kernel archive", map[string]any{"archivePath": archivePath})
	a.mu.Unlock()
	if strings.HasSuffix(strings.ToLower(assetName), ".zip") {
		if err := extractZip(archivePath, extractDir); err != nil {
			return nil, err
		}
	} else if strings.HasSuffix(strings.ToLower(assetName), ".tar.gz") {
		if err := extractTarGz(archivePath, extractDir); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("Unsupported archive format: %s", assetName)
	}
	exe := mustStr(getMap(release, "platform")["executableName"])
	binSource, err := findBinaryFile(extractDir, exe)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.pushDownloadStepLocked("search", "Locating executable file", map[string]any{"executableName": exe})
	a.mu.Unlock()
	binTarget := filepath.Join(a.binDir, exe)
	b, err := os.ReadFile(binSource)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(binTarget, b, 0o755); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.pushDownloadStepLocked("install", "Installing kernel binary", map[string]any{"binaryTarget": binTarget})
	a.mu.Unlock()
	_ = writeJSON(filepath.Join(a.binDir, "sing-box-version.json"), release)
	a.mu.Lock()
	appCfg := getMap(a.cfg, "app")
	appCfg["singBoxBinary"] = filepath.ToSlash(filepath.Join("internal", "bin", exe))
	a.cfg["app"] = appCfg
	_ = writeJSON(filepath.Join(a.dataDir, "app-config.json"), a.cfg)
	a.mu.Unlock()
	a.mu.Lock()
	a.pushDownloadStepLocked("done", "Kernel installation completed", map[string]any{"binaryPath": filepath.ToSlash(filepath.Join("internal", "bin", exe))})
	a.mu.Unlock()
	return map[string]any{"ok": true, "binaryPath": filepath.ToSlash(filepath.Join("internal", "bin", exe)), "version": release["version"], "assetName": assetName}, nil
}

func (a *App) pushDownloadStepLocked(stage, message string, details map[string]any) {
	step := map[string]any{"stage": stage, "message": message, "details": details, "time": time.Now().Format(time.RFC3339)}
	steps := getSlice(a.downloadState, "steps")
	steps = append(steps, step)
	if len(steps) > 200 {
		steps = steps[len(steps)-200:]
	}
	a.downloadState["steps"] = steps
	progress := map[string]any{
		"percent":         details["percent"],
		"stage":           stage,
		"message":         message,
		"downloadedBytes": details["downloadedBytes"],
		"totalBytes":      details["totalBytes"],
		"threads":         details["threads"],
	}
	a.downloadState["progress"] = progress
	a.downloadState["updatedAt"] = step["time"]
}

func ensureNodesLoaded(cfg, sub map[string]any) error {
	if len(getSlice(sub, "nodes")) == 0 && len(getSlice(getMap(cfg, "nodeRegistry"), "manualNodes")) == 0 {
		return fmt.Errorf("没有可用节点，请先更新订阅或添加手动节点。")
	}
	return nil
}

func (a *App) resolveSingBoxBinaryPathLocked() (string, error) {
	configured := resolveManagedPath(a.rootDir, getString(getMap(a.cfg, "app"), "singBoxBinary", ""))
	if configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured, nil
		}
	}
	ks := a.kernelStatus()
	installed := mustStr(ks["binaryPath"])
	if installed != "" {
		if _, err := os.Stat(installed); err == nil {
			appCfg := getMap(a.cfg, "app")
			appCfg["singBoxBinary"] = filepath.ToSlash(filepath.Join("internal", "bin", filepath.Base(installed)))
			a.cfg["app"] = appCfg
			_ = writeJSON(filepath.Join(a.dataDir, "app-config.json"), a.cfg)
			a.appendRuntimeLog("sing-box binary fallback to installed path: " + installed)
			return installed, nil
		}
	}
	return "", fmt.Errorf("sing-box binary not found. configured=%s, installed=%s", emptyIf(configured, "(empty)"), emptyIf(installed, "(none)"))
}

func emptyIf(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func extractZip(archivePath, extractDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		dest := filepath.Join(extractDir, filepath.Clean(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(archivePath, extractDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		dest := filepath.Join(extractDir, filepath.Clean(hdr.Name))
		if hdr.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func findBinaryFile(root, name string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == name {
			found = path
			return io.EOF
		}
		return nil
	})
	if err == io.EOF && found != "" {
		return found, nil
	}
	if found != "" {
		return found, nil
	}
	return "", fmt.Errorf("Executable not found in archive: %s", name)
}

func detectPlatform() map[string]any {
	osName := map[string]string{"windows": "windows", "linux": "linux", "darwin": "darwin"}[runtime.GOOS]
	arch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	if osName == "" || arch == "" {
		osName = runtime.GOOS
		arch = runtime.GOARCH
	}
	exe := "sing-box"
	if osName == "windows" {
		exe = "sing-box.exe"
	}
	return map[string]any{"detectedAt": time.Now().Format(time.RFC3339), "platform": runtime.GOOS, "arch": runtime.GOARCH, "os": osName, "archName": arch, "assetSuffix": osName + "-" + arch, "executableName": exe}
}

func defaultConfig() map[string]any {
	exe := filepath.ToSlash(filepath.Join("internal", "bin", map[bool]string{true: "sing-box.exe", false: "sing-box"}[runtime.GOOS == "windows"]))
	return map[string]any{
		"app":          map[string]any{"host": "0.0.0.0", "port": 18080, "singBoxBinary": exe, "autoStart": false, "autoConfigureOnSubscription": false, "logLevel": "info"},
		"subscription": map[string]any{"url": "", "urls": []any{}, "format": "raw", "userAgent": "sub2socks5/0.1.0", "refreshIntervalMinutes": 60, "headers": map[string]any{}},
		"dns":          map[string]any{"strategy": "prefer_ipv4", "remotePreset": "cloudflare", "remoteUrl": "https://cloudflare-dns.com/dns-query", "bootstrapServer": "1.1.1.1"},
		"routing":      map[string]any{"routeFinal": "proxy", "autoDetectInterface": true, "ruleSetUrls": []any{}, "rules": []any{map[string]any{"action": "sniff"}}},
		"nodeRegistry": map[string]any{"manualNodes": []any{}, "groups": []any{}, "chains": []any{}, "disabledSubscriptionTags": []any{}},
		"runtimeState": map[string]any{"fallbackGroups": map[string]any{}},
		"ports":        []any{map[string]any{"tag": "default-socks", "listen": "127.0.0.1", "port": 18081, "target": "proxy", "sniff": true}},
	}
}

func normalizeAppConfig(cfg map[string]any) (map[string]any, bool) {
	if cfg == nil {
		return defaultConfig(), true
	}
	changed := false

	nr, ok := cfg["nodeRegistry"].(map[string]any)
	if !ok {
		nr = map[string]any{"manualNodes": []any{}, "groups": []any{}, "chains": []any{}, "disabledSubscriptionTags": []any{}}
		cfg["nodeRegistry"] = nr
		changed = true
	}

	groups := getSlice(nr, "groups")
	normalizedGroups := make([]any, 0, len(groups))
	for _, item := range groups {
		group, ok := item.(map[string]any)
		if !ok {
			normalizedGroups = append(normalizedGroups, item)
			continue
		}
		strategy := strings.TrimSpace(strings.ToLower(mustStr(group["strategy"])))
		switch strategy {
		case "", "rotate", "loadbalance":
			group["strategy"] = "urltest"
			changed = true
		case "url-test":
			group["strategy"] = "urltest"
			changed = true
		case "urltest", "fallback":
			if mustStr(group["strategy"]) != strategy {
				group["strategy"] = strategy
				changed = true
			}
		default:
			group["strategy"] = "urltest"
			changed = true
		}
		normalizedGroups = append(normalizedGroups, group)
	}
	nr["groups"] = normalizedGroups

	if runtimeState, ok := cfg["runtimeState"].(map[string]any); ok {
		if _, exists := runtimeState["rotateGroups"]; exists {
			delete(runtimeState, "rotateGroups")
			changed = true
		}
	}

	return cfg, changed
}

func findPort(host string, start int) int {
	for p := start; p <= 65535; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, p))
		if err == nil {
			_ = l.Close()
			return p
		}
	}
	return start
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-content-type-options", "nosniff")
		w.Header().Set("x-frame-options", "DENY")
		w.Header().Set("referrer-policy", "no-referrer")
		w.Header().Set("cross-origin-resource-policy", "same-origin")
		w.Header().Set("cache-control", "no-store")
		w.Header().Set("access-control-allow-origin", "*")
		w.Header().Set("access-control-allow-methods", "GET, POST, HEAD, OPTIONS")
		w.Header().Set("access-control-allow-headers", "content-type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ok(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg, "status": status}})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	fail(w, 405, "Method Not Allowed")
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func decodeJSON(r io.Reader, v any) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{}")
	}
	return json.Unmarshal(b, v)
}

func mergeMap(base, incoming map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range incoming {
		if bm, ok := out[k].(map[string]any); ok {
			if im, ok2 := v.(map[string]any); ok2 {
				out[k] = mergeMap(bm, im)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func getMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}
func getSlice(m map[string]any, key string) []any {
	v, ok := m[key]
	if !ok || v == nil {
		return []any{}
	}
	if arr, ok := v.([]any); ok {
		return arr
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return []any{}
	}
	out := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i += 1 {
		out = append(out, rv.Index(i).Interface())
	}
	return out
}
func getString(m map[string]any, key, def string) string {
	s := mustStr(m[key])
	if s == "" {
		return def
	}
	return s
}
func getInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v := int(toFloat(m[key]))
	if v == 0 {
		return def
	}
	return v
}
func getBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		if s == "true" || s == "1" || s == "yes" || s == "on" {
			return true
		}
		if s == "false" || s == "0" || s == "no" || s == "off" {
			return false
		}
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return def
}
func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	default:
		return 0
	}
}
func mustStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		b, _ := json.Marshal(v)
		s := string(b)
		s = strings.Trim(s, `"`)
		return s
	}
}
func mustAtoiDefault(s string, d int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n == 0 {
		return d
	}
	return n
}
func getStringSlice(m map[string]any, key string) []string {
	out := []string{}
	for _, v := range getSlice(m, key) {
		out = append(out, mustStr(v))
	}
	return out
}
func toStringSet(in []any) map[string]bool {
	out := map[string]bool{}
	for _, v := range in {
		out[mustStr(v)] = true
	}
	return out
}
func padBase64(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "-", "+"), "_", "/")
	for len(s)%4 != 0 {
		s += "="
	}
	return s
}
func tryDecodeBase64Subscription(s string) string {
	if strings.Contains(s, "://") {
		return s
	}
	b, err := base64.StdEncoding.DecodeString(padBase64(s))
	if err != nil {
		return s
	}
	t := strings.TrimSpace(string(b))
	if strings.Contains(t, "://") {
		return t
	}
	return s
}
func dedupeNodes(in []map[string]any) []map[string]any {
	seen := map[string]bool{}
	out := []map[string]any{}
	for _, n := range in {
		k := fmt.Sprintf("%s::%s::%s::%v", mustStr(n["type"]), mustStr(n["tag"]), mustStr(n["server"]), n["server_port"])
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool { return mustStr(out[i]["tag"]) < mustStr(out[j]["tag"]) })
	return out
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var _ = exec.Command
