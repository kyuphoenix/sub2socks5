package app

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type blockingResponseWriter struct {
	header       http.Header
	writeStarted chan struct{}
	releaseWrite chan struct{}
	startOnce    sync.Once
}

func newBlockingResponseWriter() *blockingResponseWriter {
	return &blockingResponseWriter{
		header:       make(http.Header),
		writeStarted: make(chan struct{}),
		releaseWrite: make(chan struct{}),
	}
}

func (w *blockingResponseWriter) Header() http.Header { return w.header }
func (w *blockingResponseWriter) WriteHeader(int)     {}
func (w *blockingResponseWriter) Write(p []byte) (int, error) {
	w.startOnce.Do(func() { close(w.writeStarted) })
	<-w.releaseWrite
	return len(p), nil
}

func newConcurrencyTestApp(t *testing.T) *App {
	t.Helper()
	return &App{
		cfg: map[string]any{
			"app": map[string]any{},
			"subscription": map[string]any{
				"urls": []any{},
			},
			"nodeRegistry": map[string]any{},
		},
		subState: map[string]any{
			"raw":       "",
			"nodes":     []any{},
			"warnings":  []any{},
			"updatedAt": nil,
		},
		runtimeInfo: map[string]any{
			"state":   "running",
			"running": true,
			"logs":    []string{"ready"},
		},
		dataDir:           t.TempDir(),
		autoUpdateLastRun: map[string]time.Time{},
	}
}

func lockAcquiredWithin(mu *sync.RWMutex, timeout time.Duration) bool {
	acquired := make(chan struct{})
	go func() {
		mu.Lock()
		close(acquired)
		mu.Unlock()
	}()
	select {
	case <-acquired:
		return true
	case <-time.After(timeout):
		return false
	}
}

func readLockAcquiredWithin(mu *sync.RWMutex, timeout time.Duration) bool {
	acquired := make(chan struct{})
	go func() {
		mu.RLock()
		close(acquired)
		mu.RUnlock()
	}()
	select {
	case <-acquired:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestHandleRuntimeLogsReleasesLockBeforeWritingResponse(t *testing.T) {
	app := newConcurrencyTestApp(t)
	writer := newBlockingResponseWriter()
	done := make(chan struct{})

	go func() {
		app.handleRuntimeLogs(writer, httptest.NewRequest(http.MethodGet, "/api/runtime/logs", nil))
		close(done)
	}()

	select {
	case <-writer.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("handler did not start writing its response")
	}

	lockWasFree := lockAcquiredWithin(&app.mu, 150*time.Millisecond)
	close(writer.releaseWrite)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after the writer was released")
	}
	if !lockWasFree {
		t.Fatal("runtime logs handler held App.mu while blocked on the client response writer")
	}
}

func TestSubscriptionRefreshDoesNotHoldAppLockDuringNetworkIO(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte("ss://YWVzLTEyOC1nY206cGFzcw@example.com:8388#test"))
	}))
	defer upstream.Close()

	app := newConcurrencyTestApp(t)
	app.cfg["subscription"] = map[string]any{"urls": []any{upstream.URL}}
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		app.handleSubscriptionRefresh(response, httptest.NewRequest(http.MethodPost, "/api/subscription/refresh", nil))
		close(done)
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("subscription refresh did not reach the upstream server")
	}

	lockWasFree := readLockAcquiredWithin(&app.mu, 150*time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscription refresh did not finish")
	}
	if !lockWasFree {
		t.Fatal("subscription refresh held App.mu during upstream network I/O")
	}
}

func TestSubscriptionRefreshPreservesLastGoodStateWhenAllSourcesFail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	app := newConcurrencyTestApp(t)
	app.cfg["subscription"] = map[string]any{"urls": []any{upstream.URL}}
	app.subState = map[string]any{
		"raw": "last-good",
		"nodes": []any{map[string]any{
			"type":        "socks",
			"tag":         "last-good-node",
			"server":      "127.0.0.1",
			"server_port": 1080,
		}},
		"warnings":  []any{},
		"updatedAt": "2026-07-18T00:00:00Z",
	}

	response := httptest.NewRecorder()
	app.handleSubscriptionRefresh(response, httptest.NewRequest(http.MethodPost, "/api/subscription/refresh", nil))

	if response.Code != http.StatusBadGateway {
		t.Fatalf("expected upstream failure status %d, got %d: %s", http.StatusBadGateway, response.Code, response.Body.String())
	}
	app.mu.RLock()
	defer app.mu.RUnlock()
	nodes := getSlice(app.subState, "nodes")
	if len(nodes) != 1 || mustStr(nodes[0].(map[string]any)["tag"]) != "last-good-node" {
		t.Fatalf("last good subscription state was replaced: %#v", app.subState)
	}
}
