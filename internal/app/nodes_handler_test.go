package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleNodesKeepsDisabledSubscriptionNodesEditable(t *testing.T) {
	app := newConcurrencyTestApp(t)
	app.subState["nodes"] = []any{
		map[string]any{"tag": "enabled-node", "type": "vmess"},
		map[string]any{"tag": "disabled-node", "type": "vless"},
	}

	saveRecorder := httptest.NewRecorder()
	saveRequest := httptest.NewRequest(http.MethodPost, "/api/nodes", strings.NewReader(`{
		"manualNodes": [],
		"groups": [],
		"chains": [],
		"disabledSubscriptionTags": ["disabled-node"]
	}`))
	app.handleNodes(saveRecorder, saveRequest)
	if saveRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected save status: %d body=%s", saveRecorder.Code, saveRecorder.Body.String())
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

	subscriptionTags := map[string]bool{}
	for _, item := range getSlice(response, "subscriptionNodes") {
		node, ok := item.(map[string]any)
		if ok {
			subscriptionTags[mustStr(node["tag"])] = true
		}
	}
	if !subscriptionTags["enabled-node"] || !subscriptionTags["disabled-node"] {
		t.Fatalf("editor must receive enabled and disabled subscription nodes, got %#v", subscriptionTags)
	}
	if disabledTags := toStringSet(getSlice(response, "disabledSubscriptionTags")); !disabledTags["disabled-node"] {
		t.Fatalf("editor must receive the disabled state, got %#v", disabledTags)
	}

	availableTags := map[string]bool{}
	for _, item := range getSlice(response, "availableOutbounds") {
		node, ok := item.(map[string]any)
		if ok {
			availableTags[mustStr(node["tag"])] = true
		}
	}
	if !availableTags["enabled-node"] {
		t.Fatalf("enabled subscription node missing from available outbounds: %#v", availableTags)
	}
	if availableTags["disabled-node"] {
		t.Fatalf("disabled subscription node must stay unavailable outside the editor: %#v", availableTags)
	}
}
