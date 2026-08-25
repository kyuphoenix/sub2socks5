package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
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
	mu                    sync.RWMutex
	subscriptionRefreshMu sync.Mutex
	cfg                   map[string]any
	subState              map[string]any
	runtimeInfo           map[string]any
	proc                  *exec.Cmd
	manualStopRequested   bool
	autoRestartAttempts   int
	runtimeStartedAt      time.Time
	plannedKernel         map[string]any
	releaseList           []any
	downloadState         map[string]any
	rootDir               string
	dataDir               string
	runtimeDir            string
	binDir                string
	publicDir             string
	staticFS              fs.FS
	autoUpdateLastRun     map[string]time.Time
	autoUpdateLastAttempt map[string]time.Time
	nodeDelayResults      map[string]any
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
		plannedKernel:         nil,
		releaseList:           []any{},
		downloadState:         map[string]any{"active": false, "steps": []any{}, "progress": nil, "updatedAt": nil},
		autoUpdateLastRun:     map[string]time.Time{},
		autoUpdateLastAttempt: map[string]time.Time{},
		nodeDelayResults:      map[string]any{},
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

	host := getString(getMap(app.cfg, "app"), "host", "0.0.0.0")
	port := getInt(getMap(app.cfg, "app"), "port", 18080)
	addr := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("Web UI listening on http://%s\n", addr)
	return newHTTPServer(addr, newHTTPHandler(app)).ListenAndServe()
}

const (
	subscriptionRefreshTimeout               = 90 * time.Second
	autoUpdateFailureRetryDelay              = 5 * time.Minute
	maxAPIRequestBodyBytes                   = 2 << 20
	maxKernelArchiveBytes                    = 512 << 20
	kernelDownloadTimeout                    = 30 * time.Minute
	kernelDownloadIdleTimeout                = 2 * time.Minute
	kernelDownloadProgressInterval           = 250 * time.Millisecond
	kernelDownloadProgressBytes        int64 = 1 << 20
	maxRuntimeLogLineBytes                   = 16 << 10
	runtimeLogTruncatedSuffix                = " [truncated]"
	autoRestartStablePeriod                  = 5 * time.Minute
	maxAutoRestartDelay                      = 30 * time.Second
	proxyDelayControllerStartupTimeout       = 6 * time.Second
	proxyDelayControllerRetryInterval        = 150 * time.Millisecond
	runtimeDelaySnapshotTimeout              = 3 * time.Second
	maxClashProxyResponseBytes         int64 = 8 << 20
)

const clashAPIBaseURL = "http://127.0.0.1:19090"

func newHTTPHandler(app *App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", app.handleConfig)
	mux.HandleFunc("/api/subscription/refresh", app.handleSubscriptionRefresh)
	mux.HandleFunc("/api/nodes", app.handleNodes)
	mux.HandleFunc("/api/nodes/import", app.handleNodeImport)
	mux.HandleFunc("/api/nodes/check", app.handleNodesCheck)
	mux.HandleFunc("/api/nodes/delays", app.handleNodeDelays)
	mux.HandleFunc("/api/nodes/egress", app.handleNodesEgress)
	mux.HandleFunc("/api/ports/next", app.handleNextPort)
	mux.HandleFunc("/api/socks5/test", app.handleSocks5Test)
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
	return withCORS(withRequestBodyLimit(mux, maxAPIRequestBodyBytes))
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      35 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
}

func withRequestBodyLimit(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) runSubscriptionAutoUpdateScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		a.runSubscriptionAutoUpdate(now)
	}
}

func (a *App) runSubscriptionAutoUpdate(now time.Time) {
	if !a.subscriptionRefreshMu.TryLock() {
		return
	}
	defer a.subscriptionRefreshMu.Unlock()

	a.mu.RLock()
	subCfg := cloneMap(getMap(a.cfg, "subscription"))
	lastRun := cloneTimeMap(a.autoUpdateLastRun)
	lastAttempt := cloneTimeMap(a.autoUpdateLastAttempt)
	a.mu.RUnlock()

	auto := getMap(subCfg, "autoUpdate")
	scope := strings.TrimSpace(mustStr(auto["scope"]))
	if scope == "" || scope == "off" {
		return
	}

	if scope == "simultaneous" {
		key := "simultaneous"
		if !shouldAttemptAutoUpdate(now, lastRun[key], lastAttempt[key], auto) {
			return
		}
		a.recordAutoUpdateAttempt(key, now)
		ctx, cancel := context.WithTimeout(context.Background(), subscriptionRefreshTimeout)
		result := fetchSubscriptionWithContext(ctx, subCfg)
		cancel()
		if result.succeeded == 0 {
			a.mu.Lock()
			a.appendRuntimeLog("auto update failed: " + subscriptionFetchFailure(result).Error())
			a.mu.Unlock()
			return
		}
		state := result.state
		state["updatedAt"] = now.Format(time.RFC3339)
		a.mu.Lock()
		defer a.mu.Unlock()
		if !reflect.DeepEqual(getMap(a.cfg, "subscription"), subCfg) {
			a.appendRuntimeLog("auto update skipped because subscription settings changed during refresh")
			return
		}
		if err := writeJSON(filepath.Join(a.dataDir, "subscription-state.json"), state); err != nil {
			a.appendRuntimeLog("auto update failed: " + err.Error())
			return
		}
		a.subState = state
		a.autoUpdateLastRun[key] = now
		a.appendRuntimeLog("auto update completed (simultaneous)")
		return
	}

	if scope != "independent" {
		return
	}
	urls := normalizeSubscriptionURLs(subCfg)
	items := getSlice(auto, "items")
	if len(urls) == 0 || len(items) == 0 {
		return
	}
	for idx := 0; idx < len(urls) && idx < len(items); idx++ {
		item, ok := items[idx].(map[string]any)
		if !ok {
			continue
		}
		key := fmt.Sprintf("independent:%d", idx)
		if !shouldAttemptAutoUpdate(now, lastRun[key], lastAttempt[key], item) {
			continue
		}
		a.recordAutoUpdateAttempt(key, now)
		localSub := cloneMap(subCfg)
		localSub["url"] = urls[idx]
		localSub["urls"] = []any{urls[idx]}
		ctx, cancel := context.WithTimeout(context.Background(), subscriptionRefreshTimeout)
		result := fetchSubscriptionWithContext(ctx, localSub)
		cancel()
		if result.succeeded == 0 {
			a.mu.Lock()
			a.appendRuntimeLog(fmt.Sprintf("auto update failed (independent #%d): %v", idx+1, subscriptionFetchFailure(result)))
			a.mu.Unlock()
			continue
		}
		state := result.state
		state["updatedAt"] = now.Format(time.RFC3339)
		a.mu.Lock()
		if !reflect.DeepEqual(getMap(a.cfg, "subscription"), subCfg) {
			a.appendRuntimeLog("auto update skipped because subscription settings changed during refresh")
			a.mu.Unlock()
			return
		}
		merged := mergeSubscriptionState(a.subState, state)
		if err := writeJSON(filepath.Join(a.dataDir, "subscription-state.json"), merged); err != nil {
			a.appendRuntimeLog(fmt.Sprintf("auto update failed (independent #%d): %v", idx+1, err))
			a.mu.Unlock()
			continue
		}
		a.subState = merged
		a.autoUpdateLastRun[key] = now
		a.appendRuntimeLog(fmt.Sprintf("auto update completed (independent #%d)", idx+1))
		a.mu.Unlock()
	}
}

func shouldAttemptAutoUpdate(now, lastRun, lastAttempt time.Time, cfg map[string]any) bool {
	if !shouldRunAutoUpdate(now, lastRun, cfg) {
		return false
	}
	return lastAttempt.IsZero() || now.Sub(lastAttempt) >= autoUpdateFailureRetryDelay
}

func cloneTimeMap(in map[string]time.Time) map[string]time.Time {
	out := make(map[string]time.Time, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (a *App) recordAutoUpdateAttempt(key string, now time.Time) {
	a.mu.Lock()
	if a.autoUpdateLastAttempt == nil {
		a.autoUpdateLastAttempt = map[string]time.Time{}
	}
	a.autoUpdateLastAttempt[key] = now
	a.mu.Unlock()
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

func (a *App) refreshSubscription(ctx context.Context, reason string) (map[string]any, error) {
	a.subscriptionRefreshMu.Lock()
	defer a.subscriptionRefreshMu.Unlock()

	a.mu.RLock()
	subCfg := cloneMap(getMap(a.cfg, "subscription"))
	a.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, subscriptionRefreshTimeout)
	defer cancel()
	result := fetchSubscriptionWithContext(ctx, subCfg)
	if result.succeeded == 0 {
		err := subscriptionFetchFailure(result)
		a.mu.Lock()
		a.appendRuntimeLog("subscription refresh failed (" + reason + "): " + err.Error())
		a.mu.Unlock()
		return nil, err
	}

	state := result.state
	state["updatedAt"] = time.Now().Format(time.RFC3339)
	a.mu.Lock()
	defer a.mu.Unlock()
	if !reflect.DeepEqual(getMap(a.cfg, "subscription"), subCfg) {
		return nil, fmt.Errorf("subscription settings changed during refresh; please retry")
	}
	if err := writeJSON(filepath.Join(a.dataDir, "subscription-state.json"), state); err != nil {
		return nil, err
	}
	a.subState = state
	a.appendRuntimeLog("subscription refreshed: " + reason)
	return cloneMap(state), nil
}

func subscriptionFetchFailure(result subscriptionFetchResult) error {
	detail := "no subscription source succeeded"
	warnings := getSlice(result.state, "warnings")
	if len(warnings) > 0 {
		detail = mustStr(warnings[0])
	}
	if result.attempted == 0 {
		return fmt.Errorf("no subscription URL configured: %s", detail)
	}
	return fmt.Errorf("all %d subscription source(s) failed: %s", result.attempted, detail)
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
		payload, err := json.Marshal(map[string]any{
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
		a.mu.RUnlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		okBytes(w, payload)
	case http.MethodPost:
		var body map[string]any
		if err := decodeJSON(r.Body, &body); err != nil {
			failDecodeJSON(w, err)
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
				fail(w, http.StatusInternalServerError, err.Error())
				return
			}
			a.appendRuntimeLog("config applied and runtime restarted")
		}
		payload, err := json.Marshal(map[string]any{"ok": true, "generated": generated, "runtime": a.runtimeInfo})
		a.mu.Unlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		okBytes(w, payload)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (a *App) handleSubscriptionRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	state, err := a.refreshSubscription(r.Context(), "manual")
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	ok(w, state)
}

func (a *App) handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.mu.RLock()
		nr := getMap(a.cfg, "nodeRegistry")
		payload, err := json.Marshal(map[string]any{
			"subscriptionNodes":        getSlice(a.subState, "nodes"),
			"disabledSubscriptionTags": getSlice(nr, "disabledSubscriptionTags"),
			"manualNodes":              getSlice(nr, "manualNodes"),
			"groups":                   getSlice(nr, "groups"),
			"chains":                   getSlice(nr, "chains"),
			"availableOutbounds":       collectOutbounds(a.cfg, a.subState),
			"fallbackStates":           map[string]any{},
			"nodeDelays":               nonNilMap(a.nodeDelayResults),
		})
		a.mu.RUnlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		okBytes(w, payload)
	case http.MethodPost:
		var body map[string]any
		if err := decodeJSON(r.Body, &body); err != nil {
			failDecodeJSON(w, err)
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
				fail(w, http.StatusInternalServerError, err.Error())
				return
			}
			a.appendRuntimeLog("node config applied and runtime restarted")
		}
		payload, err := json.Marshal(map[string]any{
			"ok":                 true,
			"manualNodes":        nr["manualNodes"],
			"groups":             nr["groups"],
			"chains":             nr["chains"],
			"availableOutbounds": collectOutbounds(a.cfg, a.subState),
		})
		a.mu.Unlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		okBytes(w, payload)
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
		failDecodeJSON(w, err)
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
		failDecodeJSON(w, err)
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
	testURLs := proxyDelayCandidateURLs(urlToTest)
	timeout := int(toFloat(body["timeoutMs"]))
	if timeout <= 0 {
		timeout = 5000
	}
	readyCtx, cancelReady := context.WithTimeout(r.Context(), proxyDelayControllerStartupTimeout)
	readyErr := waitForProxyDelayController(readyCtx, proxyDelayControllerRetryInterval, probeProxyDelayController)
	cancelReady()
	if readyErr != nil {
		checkedAt := time.Now().Format(time.RFC3339)
		results := map[string]any{}
		for _, tag := range tags {
			results[tag] = map[string]any{
				"ok":         false,
				"text":       "失败",
				"error":      readyErr.Error(),
				"checkedAt":  checkedAt,
				"checkedTag": tag,
				"source":     "manual",
			}
		}
		a.mergeNodeDelayResults(results)
		ok(w, map[string]any{"ok": true, "url": testURLs[0], "urls": testURLs, "timeoutMs": timeout, "results": results})
		return
	}
	results := map[string]any{}
	for _, tag := range tags {
		delay, usedURL, err := firstSuccessfulProxyDelay(testURLs, func(testURL string) (int, error) {
			return measureProxyDelay(tag, testURL, timeout)
		})
		if err != nil {
			results[tag] = map[string]any{"ok": false, "text": "失败", "error": err.Error(), "checkedAt": time.Now().Format(time.RFC3339), "checkedTag": tag, "source": "manual"}
			continue
		}
		results[tag] = map[string]any{"ok": true, "delay": delay, "text": fmt.Sprintf("%d ms", delay), "url": usedURL, "checkedAt": time.Now().Format(time.RFC3339), "checkedTag": tag, "source": "manual"}
	}
	a.mergeNodeDelayResults(results)
	ok(w, map[string]any{"ok": true, "url": testURLs[0], "urls": testURLs, "timeoutMs": timeout, "results": results})
}

func (a *App) handleNodeDelays(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}

	a.mu.RLock()
	running := getBool(a.runtimeInfo, "running", false)
	cached := cloneMap(nonNilMap(a.nodeDelayResults))
	a.mu.RUnlock()
	if !running {
		ok(w, map[string]any{"ok": true, "running": false, "ready": false, "results": cached})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), runtimeDelaySnapshotTimeout)
	liveResults, err := fetchClashProxyDelayResults(ctx)
	cancel()
	if err != nil {
		ok(w, map[string]any{"ok": true, "running": true, "ready": false, "results": cached})
		return
	}

	results := a.mergeNodeDelayResults(liveResults)
	ok(w, map[string]any{"ok": true, "running": true, "ready": true, "results": results})
}

func (a *App) handleNextPort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		failDecodeJSON(w, err)
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

func (a *App) handleSocks5Test(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		failDecodeJSON(w, err)
		return
	}
	host := strings.TrimSpace(mustStr(body["listen"]))
	if host == "" {
		host = "127.0.0.1"
	}
	port := int(toFloat(body["port"]))
	if port <= 0 {
		fail(w, 400, "Invalid socks5 port")
		return
	}
	timeout := int(toFloat(body["timeoutMs"]))
	if timeout <= 0 {
		timeout = 8000
	}
	source := strings.TrimSpace(mustStr(body["source"]))
	ip, err := fetchIPViaSocks(host, port, source, timeout)
	if err != nil {
		fail(w, http.StatusBadGateway, "查询出口 IP 失败："+err.Error())
		return
	}
	ok(w, map[string]any{"ok": true, "ip": ip, "source": source})
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
	if err := a.startRuntimeLocked(); err != nil {
		a.mu.Unlock()
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, err := json.Marshal(a.runtimeInfo)
	a.mu.Unlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	okBytes(w, payload)
}

func (a *App) startRuntimeLocked() error {
	return a.startRuntimeLockedWithRestartReset(true)
}

func (a *App) startRuntimeLockedWithRestartReset(resetRestartAttempts bool) error {
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create sing-box stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create sing-box stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	a.manualStopRequested = false
	a.recordRuntimeStartedLocked(time.Now(), resetRestartAttempts)
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
				runDuration := time.Since(a.runtimeStartedAt)
				attempt, delay := nextAutoRestart(a.autoRestartAttempts, runDuration)
				a.autoRestartAttempts = attempt
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
	if err := a.startRuntimeLockedWithRestartReset(false); err != nil {
		attempt, nextDelay := nextAutoRestart(a.autoRestartAttempts, 0)
		a.autoRestartAttempts = attempt
		a.appendRuntimeLog(fmt.Sprintf("auto restart failed: %v; retry in %ds (attempt %d)", err, int(nextDelay/time.Second), attempt))
		go a.autoRestartAfter(nextDelay)
	}
}

func (a *App) recordRuntimeStartedLocked(now time.Time, resetRestartAttempts bool) {
	if resetRestartAttempts {
		a.autoRestartAttempts = 0
	}
	a.runtimeStartedAt = now
}

func nextAutoRestart(previousAttempts int, runDuration time.Duration) (int, time.Duration) {
	if previousAttempts < 0 || runDuration >= autoRestartStablePeriod {
		previousAttempts = 0
	}
	attempt := previousAttempts + 1
	delay := 2 * time.Second
	for i := 1; i < attempt && delay < maxAutoRestartDelay; i++ {
		delay *= 2
		if delay > maxAutoRestartDelay {
			delay = maxAutoRestartDelay
		}
	}
	return attempt, delay
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
	payload, err := json.Marshal(a.runtimeInfo)
	a.mu.Unlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	okBytes(w, payload)
}

func (a *App) handleRuntimeLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, "GET")
		return
	}
	a.mu.RLock()
	payload, err := json.Marshal(a.runtimeInfo)
	a.mu.RUnlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	okBytes(w, payload)
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
		payload, err := json.Marshal(map[string]any{"architecture": arch, "stored": true, "plannedKernel": a.plannedKernel, "kernel": a.kernelStatus()})
		a.mu.RUnlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		okBytes(w, payload)
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
		payload, err := json.Marshal(a.releaseList)
		a.mu.RUnlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		okBytes(w, payload)
		return
	}
	a.mu.RUnlock()
	releases, err := listReleases(detectPlatform())
	if err != nil {
		a.mu.RLock()
		payload, marshalErr := json.Marshal(a.releaseList)
		a.mu.RUnlock()
		if marshalErr != nil {
			fail(w, http.StatusInternalServerError, marshalErr.Error())
			return
		}
		okBytes(w, payload)
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
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.mu.Lock()
	a.releaseList = releases
	_ = writeJSON(filepath.Join(a.dataDir, "release-list.json"), releases)
	if len(releases) > 0 {
		if planned, ok := releases[0].(map[string]any); ok {
			a.plannedKernel = planned
			_ = writeJSON(filepath.Join(a.dataDir, "planned-kernel-info.json"), planned)
		}
	}
	payload, marshalErr := json.Marshal(map[string]any{"releaseList": releases, "plannedKernel": a.plannedKernel})
	a.mu.Unlock()
	if marshalErr != nil {
		fail(w, http.StatusInternalServerError, marshalErr.Error())
		return
	}
	okBytes(w, payload)
}

func (a *App) handleKernelPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		failDecodeJSON(w, err)
		return
	}
	version := mustStr(body["version"])
	a.mu.Lock()
	for _, item := range a.releaseList {
		planned, ok := item.(map[string]any)
		if ok && mustStr(planned["version"]) == version {
			a.plannedKernel = planned
			_ = writeJSON(filepath.Join(a.dataDir, "planned-kernel-info.json"), planned)
			payload, err := json.Marshal(planned)
			a.mu.Unlock()
			if err != nil {
				fail(w, http.StatusInternalServerError, err.Error())
				return
			}
			okBytes(w, payload)
			return
		}
	}
	a.mu.Unlock()
	fail(w, http.StatusNotFound, "Requested kernel version not found")
}

func (a *App) handleKernelDownload(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.mu.RLock()
		payload, err := json.Marshal(a.downloadState)
		a.mu.RUnlock()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		okBytes(w, payload)
	case http.MethodPost:
		a.mu.Lock()
		if getBool(a.downloadState, "active", false) {
			a.mu.Unlock()
			fail(w, http.StatusConflict, "A kernel download is already active")
			return
		}
		planned := cloneMap(a.plannedKernel)
		if len(planned) == 0 {
			a.mu.Unlock()
			fail(w, http.StatusBadRequest, "No planned kernel selected")
			return
		}
		a.downloadState = map[string]any{"active": true, "steps": []any{}, "progress": map[string]any{"percent": 0, "stage": "prepare", "message": "preparing"}, "updatedAt": time.Now().Format(time.RFC3339)}
		a.pushDownloadStepLocked("prepare", "Prepared download workspace", map[string]any{})
		a.mu.Unlock()
		result, err := a.downloadKernel(r.Context(), planned)
		if err != nil {
			a.mu.Lock()
			a.downloadState = map[string]any{"active": false, "steps": []any{map[string]any{"stage": "error", "message": err.Error()}}, "progress": map[string]any{"percent": nil, "stage": "error", "message": err.Error()}, "updatedAt": time.Now().Format(time.RFC3339)}
			a.mu.Unlock()
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.mu.Lock()
		a.downloadState = map[string]any{"active": false, "steps": []any{map[string]any{"stage": "done", "message": "Kernel installation completed"}}, "progress": map[string]any{"percent": 100, "stage": "done", "message": "Kernel installation completed"}, "updatedAt": time.Now().Format(time.RFC3339)}
		payload, marshalErr := json.Marshal(map[string]any{"result": result, "kernel": a.kernelStatus(), "download": a.downloadState})
		a.mu.Unlock()
		if marshalErr != nil {
			fail(w, http.StatusInternalServerError, marshalErr.Error())
			return
		}
		okBytes(w, payload)
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
	acc := make([]byte, 0, maxRuntimeLogLineBytes)
	truncated := false

	flush := func() {
		lineBytes := bytes.TrimSpace(acc)
		if truncated {
			contentLimit := maxRuntimeLogLineBytes - len(runtimeLogTruncatedSuffix)
			if len(lineBytes) > contentLimit {
				lineBytes = lineBytes[:contentLimit]
			}
			lineBytes = append(append([]byte(nil), lineBytes...), runtimeLogTruncatedSuffix...)
		}
		line := strings.TrimSpace(strings.ToValidUTF8(string(lineBytes), "?"))
		if line != "" {
			a.mu.Lock()
			a.appendRuntimeLog(line)
			a.mu.Unlock()
		}
		acc = acc[:0]
		truncated = false
	}

	appendSegment := func(segment []byte) {
		remaining := maxRuntimeLogLineBytes - len(acc)
		if remaining > 0 {
			if len(segment) <= remaining {
				acc = append(acc, segment...)
				return
			}
			acc = append(acc, segment[:remaining]...)
		}
		if len(segment) > remaining {
			truncated = true
		}
	}

	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			for len(chunk) > 0 {
				separator := bytes.IndexAny(chunk, "\r\n")
				if separator < 0 {
					appendSegment(chunk)
					break
				}
				appendSegment(chunk[:separator])
				flush()
				chunk = bytes.TrimLeft(chunk[separator+1:], "\r\n")
			}
		}
		if readErr != nil {
			flush()
			return
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

func buildProxyDelayEndpoint(baseURL, tag, testURL string, timeoutMs int) string {
	return fmt.Sprintf("%s/proxies/%s/delay?url=%s&timeout=%d", strings.TrimRight(baseURL, "/"), url.PathEscape(tag), url.QueryEscape(testURL), timeoutMs)
}

func measureProxyDelay(tag, testURL string, timeoutMs int) (int, error) {
	endpoint := buildProxyDelayEndpoint(clashAPIBaseURL, tag, testURL, timeoutMs)
	client := &http.Client{Timeout: time.Duration(timeoutMs+1500) * time.Millisecond}
	resp, err := client.Get(endpoint)
	if err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "connection refused"):
			return 0, fmt.Errorf("延迟测试控制接口未就绪（connection refused），请先确认 sing-box 已正常启动")
		case strings.Contains(msg, "timeout"):
			return 0, fmt.Errorf("延迟测试控制接口请求超时，请稍后重试")
		default:
			return 0, fmt.Errorf("延迟测试控制接口未就绪，请先确认 sing-box 已正常启动")
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

func waitForProxyDelayController(ctx context.Context, retryInterval time.Duration, probe func(context.Context) error) error {
	if retryInterval <= 0 {
		retryInterval = 100 * time.Millisecond
	}
	var lastErr error
	for {
		if err := probe(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if lastErr != nil {
				return fmt.Errorf("延迟测试控制接口启动超时: %w", lastErr)
			}
			return fmt.Errorf("延迟测试控制接口启动超时: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func probeProxyDelayController(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clashAPIBaseURL+"/version", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func fetchClashProxyDelayResults(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clashAPIBaseURL+"/proxies", nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: runtimeDelaySnapshotTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("clash api proxies: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxClashProxyResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxClashProxyResponseBytes {
		return nil, fmt.Errorf("clash api proxies response exceeds %d bytes", maxClashProxyResponseBytes)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return parseClashProxyDelayResults(payload), nil
}

func parseClashProxyDelayResults(payload map[string]any) map[string]any {
	results := map[string]any{}
	for tag, rawProxy := range getMap(payload, "proxies") {
		proxy, ok := rawProxy.(map[string]any)
		if !ok {
			continue
		}
		history := getSlice(proxy, "history")
		if len(history) == 0 {
			continue
		}
		latest, ok := history[len(history)-1].(map[string]any)
		if !ok {
			continue
		}
		delay := int(toFloat(latest["delay"]))
		result := map[string]any{
			"ok":         delay > 0,
			"delay":      delay,
			"checkedAt":  mustStr(latest["time"]),
			"checkedTag": tag,
			"source":     "runtime",
		}
		if delay > 0 {
			result["text"] = fmt.Sprintf("%d ms", delay)
		} else {
			result["text"] = "失败"
			result["error"] = "sing-box 自动延迟测试失败"
		}
		results[tag] = result
	}
	return results
}

func (a *App) mergeNodeDelayResults(results map[string]any) map[string]any {
	incoming := cloneMap(nonNilMap(results))
	a.mu.Lock()
	if a.nodeDelayResults == nil {
		a.nodeDelayResults = map[string]any{}
	}
	allowedTags := a.currentNodeDelayTagsLocked()
	for tag := range a.nodeDelayResults {
		if !allowedTags[tag] {
			delete(a.nodeDelayResults, tag)
		}
	}
	for tag, result := range incoming {
		if !allowedTags[tag] {
			continue
		}
		if existing, exists := a.nodeDelayResults[tag]; exists && !shouldReplaceNodeDelayResult(existing, result) {
			continue
		}
		a.nodeDelayResults[tag] = result
	}
	snapshot := cloneMap(a.nodeDelayResults)
	a.mu.Unlock()
	return snapshot
}

func (a *App) currentNodeDelayTagsLocked() map[string]bool {
	tags := map[string]bool{}
	for _, item := range collectOutbounds(a.cfg, a.subState) {
		outbound, ok := item.(map[string]any)
		if !ok || mustStr(outbound["source"]) == "builtin" {
			continue
		}
		if tag := strings.TrimSpace(mustStr(outbound["tag"])); tag != "" {
			tags[tag] = true
		}
	}
	return tags
}

func shouldReplaceNodeDelayResult(existing, incoming any) bool {
	existingMap, existingOK := existing.(map[string]any)
	incomingMap, incomingOK := incoming.(map[string]any)
	if !incomingOK {
		return true
	}
	incomingTime, incomingTimeOK := parseNodeDelayCheckedAt(mustStr(incomingMap["checkedAt"]))
	if !existingOK {
		return true
	}
	existingTime, existingTimeOK := parseNodeDelayCheckedAt(mustStr(existingMap["checkedAt"]))
	switch {
	case incomingTimeOK && existingTimeOK:
		return !incomingTime.Before(existingTime)
	case incomingTimeOK:
		return true
	case existingTimeOK:
		return false
	default:
		return true
	}
}

func parseNodeDelayCheckedAt(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	return parsed, err == nil
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

var defaultProxyDelayURLs = []string{
	"https://www.gstatic.com/generate_204",
	"https://www.google.com/generate_204",
	"https://cp.cloudflare.com/generate_204",
}

func proxyDelayCandidateURLs(requestedURL string) []string {
	requestedURL = strings.TrimSpace(requestedURL)
	if requestedURL != "" {
		return []string{requestedURL}
	}
	return append([]string(nil), defaultProxyDelayURLs...)
}

func firstSuccessfulProxyDelay(testURLs []string, measure func(testURL string) (int, error)) (int, string, error) {
	errorsByURL := make([]string, 0, len(testURLs))
	for _, testURL := range testURLs {
		delay, err := measure(testURL)
		if err == nil {
			return delay, testURL, nil
		}
		errorsByURL = append(errorsByURL, fmt.Sprintf("%s: %v", testURL, err))
	}
	if len(errorsByURL) == 0 {
		return 0, "", fmt.Errorf("没有可用的延迟测试地址")
	}
	return 0, "", fmt.Errorf("所有延迟测试地址均失败：%s", strings.Join(errorsByURL, "; "))
}

func (a *App) handleNodesEgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, "POST")
		return
	}
	var body map[string]any
	if err := decodeJSON(r.Body, &body); err != nil {
		failDecodeJSON(w, err)
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
		ip, ipErr := fetchIPViaSocks(host, port, "", timeout)
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
	endpoint := fmt.Sprintf("%s/proxies/%s", clashAPIBaseURL, url.PathEscape(groupTag))
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

func fetchIPViaSocks(socksHost string, socksPort int, source string, timeoutMs int) (string, error) {
	targets := []struct {
		id   string
		host string
		path string
	}{
		{id: "api.ipify.org", host: "api.ipify.org", path: "/?format=text"},
		{id: "ipv4.icanhazip.com", host: "ipv4.icanhazip.com", path: "/"},
		{id: "ifconfig.me", host: "ifconfig.me", path: "/ip"},
		{id: "ip.sb", host: "api.ip.sb", path: "/ip"},
	}
	if source != "" {
		filtered := targets[:0]
		for _, t := range targets {
			if strings.EqualFold(t.id, source) || strings.Contains(t.host, source) {
				filtered = append(filtered, t)
			}
		}
		targets = filtered
		if len(targets) == 0 {
			return "", fmt.Errorf("unknown source %q", source)
		}
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

func newKernelDownloadClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 15 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.IdleConnTimeout = 90 * time.Second
	return &http.Client{
		Transport: transport,
		Timeout:   kernelDownloadTimeout,
	}
}

type downloadProgressLimiter struct {
	lastAt    time.Time
	lastBytes int64
}

func (l *downloadProgressLimiter) shouldReport(now time.Time, downloaded int64, force bool) bool {
	if downloaded <= l.lastBytes {
		return false
	}
	if !force && downloaded-l.lastBytes < kernelDownloadProgressBytes && now.Sub(l.lastAt) < kernelDownloadProgressInterval {
		return false
	}
	l.lastAt = now
	l.lastBytes = downloaded
	return true
}

func downloadKernelArchive(
	ctx context.Context,
	client *http.Client,
	urlStr string,
	archivePath string,
	expectedSize int64,
	maxBytes int64,
	idleTimeout time.Duration,
	onProgress func(downloaded, total int64),
) (int64, error) {
	if maxBytes <= 0 {
		return 0, fmt.Errorf("kernel archive maximum size must be positive")
	}
	if idleTimeout <= 0 {
		return 0, fmt.Errorf("kernel download idle timeout must be positive")
	}

	requestCtx, cancel := context.WithCancel(ctx)
	activity := make(chan struct{}, 1)
	idleTimedOut := make(chan struct{})
	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		timer := time.NewTimer(idleTimeout)
		defer timer.Stop()
		for {
			select {
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idleTimeout)
			case <-timer.C:
				close(idleTimedOut)
				cancel()
				return
			case <-requestCtx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-watchdogDone
	}()
	signalActivity := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, urlStr, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("user-agent", "sub2socks5-go/0.1.0")
	resp, err := client.Do(req)
	if err != nil {
		select {
		case <-idleTimedOut:
			return 0, fmt.Errorf("kernel download stalled for %s", idleTimeout)
		default:
			return 0, err
		}
	}
	defer resp.Body.Close()
	signalActivity()
	if resp.StatusCode >= http.StatusBadRequest {
		return 0, fmt.Errorf("failed to download sing-box: HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	if total <= 0 {
		total = expectedSize
	}
	if total > maxBytes {
		return 0, fmt.Errorf("kernel archive size %d exceeds maximum %d bytes", total, maxBytes)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 64*1024)
	var downloaded int64
	limiter := downloadProgressLimiter{lastAt: time.Now()}
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			signalActivity()
			if downloaded+int64(n) > maxBytes {
				return downloaded, fmt.Errorf("kernel archive exceeds maximum %d bytes", maxBytes)
			}
			written, writeErr := f.Write(buf[:n])
			if writeErr != nil {
				return downloaded, writeErr
			}
			if written != n {
				return downloaded, io.ErrShortWrite
			}
			downloaded += int64(n)
			if onProgress != nil && limiter.shouldReport(time.Now(), downloaded, false) {
				onProgress(downloaded, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			select {
			case <-idleTimedOut:
				return downloaded, fmt.Errorf("kernel download stalled for %s", idleTimeout)
			default:
				return downloaded, readErr
			}
		}
	}
	if onProgress != nil && limiter.shouldReport(time.Now(), downloaded, true) {
		onProgress(downloaded, total)
	}
	if err := f.Close(); err != nil {
		return downloaded, err
	}
	return downloaded, nil
}

func (a *App) downloadKernel(ctx context.Context, release map[string]any) (map[string]any, error) {
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
	expectedSize := int64(toFloat(release["size"]))
	downloadCtx, cancel := context.WithTimeout(ctx, kernelDownloadTimeout)
	defer cancel()
	_, err = downloadKernelArchive(
		downloadCtx,
		newKernelDownloadClient(),
		urlStr,
		archivePath,
		expectedSize,
		maxKernelArchiveBytes,
		kernelDownloadIdleTimeout,
		func(downloaded, total int64) {
			percent := any(nil)
			if total > 0 {
				percent = float64(downloaded) / float64(total) * 100
				if percent.(float64) > 100 {
					percent = float64(100)
				}
			}
			a.mu.Lock()
			a.pushDownloadStepLocked("download", "Downloading kernel archive", map[string]any{
				"downloadedBytes": downloaded,
				"totalBytes":      total,
				"percent":         percent,
				"threads":         1,
			})
			a.mu.Unlock()
		},
	)
	if err != nil {
		return nil, err
	}
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

func okBytes(w http.ResponseWriter, payload []byte) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func ok(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(v)
}

func failDecodeJSON(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		status = http.StatusRequestEntityTooLarge
	}
	fail(w, status, err.Error())
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
