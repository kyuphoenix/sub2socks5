package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProxyDelayCandidateURLs(t *testing.T) {
	if got := proxyDelayCandidateURLs("https://custom.example/generate_204"); !reflect.DeepEqual(got, []string{"https://custom.example/generate_204"}) {
		t.Fatalf("custom delay URL should not use fallbacks, got %#v", got)
	}

	got := proxyDelayCandidateURLs("")
	if len(got) < 2 {
		t.Fatalf("default delay test should have fallback targets, got %#v", got)
	}
}

func TestFirstSuccessfulProxyDelayFallsBack(t *testing.T) {
	called := []string{}
	delay, usedURL, err := firstSuccessfulProxyDelay([]string{"primary", "fallback"}, func(testURL string) (int, error) {
		called = append(called, testURL)
		if testURL == "primary" {
			return 0, errors.New("primary unavailable")
		}
		return 123, nil
	})
	if err != nil {
		t.Fatalf("fallback should succeed: %v", err)
	}
	if delay != 123 || usedURL != "fallback" {
		t.Fatalf("unexpected fallback result: delay=%d url=%q", delay, usedURL)
	}
	if !reflect.DeepEqual(called, []string{"primary", "fallback"}) {
		t.Fatalf("unexpected candidate order: %#v", called)
	}
}

func TestWaitForProxyDelayControllerRetriesUntilReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	attempts := 0
	err := waitForProxyDelayController(ctx, time.Millisecond, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("controller is starting")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("controller should become ready: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 readiness attempts, got %d", attempts)
	}
}

func TestBuildProxyDelayEndpointEscapesTagAsPathSegment(t *testing.T) {
	endpoint := buildProxyDelayEndpoint("http://127.0.0.1:19090", "HK node/01", "https://example.com/generate_204", 5000)
	if !strings.Contains(endpoint, "/proxies/HK%20node%2F01/delay?") {
		t.Fatalf("proxy tag should use path escaping, got %q", endpoint)
	}
	if strings.Contains(endpoint, "HK+node") {
		t.Fatalf("proxy tag must not use query escaping in the path, got %q", endpoint)
	}
}

func TestParseClashProxyDelayResultsUsesLatestHistory(t *testing.T) {
	payload := map[string]any{
		"proxies": map[string]any{
			"node-a": map[string]any{
				"history": []any{
					map[string]any{"time": "2026-07-20T10:00:00Z", "delay": 80},
					map[string]any{"time": "2026-07-20T10:01:00Z", "delay": 123},
				},
			},
			"node-b": map[string]any{
				"history": []any{map[string]any{"time": "2026-07-20T10:02:00Z", "delay": 0}},
			},
			"node-without-history": map[string]any{"history": []any{}},
		},
	}

	results := parseClashProxyDelayResults(payload)
	nodeA := getMap(results, "node-a")
	if nodeA["ok"] != true || int(toFloat(nodeA["delay"])) != 123 || mustStr(nodeA["text"]) != "123 ms" {
		t.Fatalf("unexpected node-a delay result: %#v", nodeA)
	}
	if mustStr(nodeA["checkedAt"]) != "2026-07-20T10:01:00Z" || mustStr(nodeA["source"]) != "runtime" {
		t.Fatalf("unexpected node-a metadata: %#v", nodeA)
	}
	nodeB := getMap(results, "node-b")
	if nodeB["ok"] != false || mustStr(nodeB["text"]) != "失败" {
		t.Fatalf("unexpected node-b delay result: %#v", nodeB)
	}
	if _, exists := results["node-without-history"]; exists {
		t.Fatalf("proxy without history should not produce a delay result")
	}
}

func TestHandleNodesIncludesCachedDelayResults(t *testing.T) {
	app := newConcurrencyTestApp(t)
	getMap(app.cfg, "nodeRegistry")["manualNodes"] = []any{map[string]any{"tag": "node-a", "type": "socks"}}
	app.nodeDelayResults = map[string]any{
		"node-a": map[string]any{"ok": true, "delay": 88, "text": "88 ms"},
	}

	recorder := httptest.NewRecorder()
	app.handleNodes(recorder, httptest.NewRequest(http.MethodGet, "/api/nodes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	var response map[string]any
	if err := decodeJSON(recorder.Body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if mustStr(getMap(getMap(response, "nodeDelays"), "node-a")["text"]) != "88 ms" {
		t.Fatalf("cached node delays missing from response: %#v", response["nodeDelays"])
	}
}

func TestMergeNodeDelayResultsKeepsNewerCachedResult(t *testing.T) {
	app := newConcurrencyTestApp(t)
	getMap(app.cfg, "nodeRegistry")["manualNodes"] = []any{map[string]any{"tag": "node-a", "type": "socks"}}
	app.nodeDelayResults = map[string]any{
		"node-a": map[string]any{
			"ok":        true,
			"delay":     88,
			"text":      "88 ms",
			"checkedAt": "2026-07-20T10:02:00Z",
			"source":    "manual",
		},
	}

	results := app.mergeNodeDelayResults(map[string]any{
		"node-a": map[string]any{
			"ok":        true,
			"delay":     123,
			"text":      "123 ms",
			"checkedAt": "2026-07-20T10:01:00Z",
			"source":    "runtime",
		},
	})
	nodeA := getMap(results, "node-a")
	if int(toFloat(nodeA["delay"])) != 88 || mustStr(nodeA["source"]) != "manual" {
		t.Fatalf("older runtime result should not replace newer cached result: %#v", nodeA)
	}
}
