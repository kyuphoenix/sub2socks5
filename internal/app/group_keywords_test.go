package app

import (
	"testing"
)

func TestApplyGroupKeywordRulesMatchesAndPersists(t *testing.T) {
	app := newConcurrencyTestApp(t)
	app.cfg = map[string]any{
		"subscription": map[string]any{},
		"nodeRegistry": map[string]any{
			"manualNodes": []any{
				map[string]any{"tag": "hk-manual", "type": "vmess", "server": "hk.example.com", "server_port": 443},
			},
			"groups": []any{
				map[string]any{"tag": "hk-group", "keywords": "hk, hongkong", "strategy": "urltest", "members": []any{}},
			},
		},
	}
	app.subState = map[string]any{
		"nodes": []any{
			map[string]any{"tag": "hk-1", "type": "vmess", "server": "a.example.com", "server_port": 443},
			map[string]any{"tag": "singapore-1", "type": "trojan", "server": "sg.example.com", "server_port": 443},
			map[string]any{"tag": "hongkong-2", "type": "vless", "server": "b.example.com", "server_port": 443},
		},
	}

	if err := app.applyGroupKeywordRules(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	groups := getSlice(getMap(app.cfg, "nodeRegistry"), "groups")
	g, _ := groups[0].(map[string]any)
	got := toStringSet(getSlice(g, "members"))
	want := toStringSet([]any{"hk-1", "hongkong-2", "hk-manual"})
	if len(got) != len(want) {
		t.Fatalf("expected members %#v, got %#v", want, got)
	}
	for tag := range want {
		if !got[tag] {
			t.Fatalf("expected member %q missing from %#v", tag, got)
		}
	}
}

func TestApplyGroupKeywordRulesNoChange(t *testing.T) {
	app := newConcurrencyTestApp(t)
	app.cfg = map[string]any{
		"subscription": map[string]any{},
		"nodeRegistry": map[string]any{
			"manualNodes": []any{},
			"groups": []any{
				map[string]any{"tag": "us-group", "keywords": "us, america", "strategy": "urltest", "members": []any{"us-1"}},
			},
		},
	}
	app.subState = map[string]any{
		"nodes": []any{
			map[string]any{"tag": "us-1", "type": "vmess", "server": "us.example.com", "server_port": 443},
			map[string]any{"tag": "japan-1", "type": "trojan", "server": "jp.example.com", "server_port": 443},
		},
	}

	if err := app.applyGroupKeywordRules(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The matched set {us-1} is unchanged, so members must stay exactly as-is.
	groups := getSlice(getMap(app.cfg, "nodeRegistry"), "groups")
	g, _ := groups[0].(map[string]any)
	got := currentGroupMembers(g)
	if len(got) != 1 || got[0] != "us-1" {
		t.Fatalf("expected unchanged members [us-1], got %#v", got)
	}
}

func TestApplyGroupKeywordRulesIgnoresUnknownStaleMembers(t *testing.T) {
	app := newConcurrencyTestApp(t)
	app.cfg = map[string]any{
		"subscription": map[string]any{},
		"nodeRegistry": map[string]any{
			"manualNodes": []any{},
			"groups": []any{
				map[string]any{"tag": "eu-group", "keywords": "eu", "strategy": "urltest", "members": []any{"old-removed-node", "eu-1"}},
			},
		},
	}
	app.subState = map[string]any{
		"nodes": []any{
			map[string]any{"tag": "eu-1", "type": "vmess", "server": "eu.example.com", "server_port": 443},
		},
	}

	if err := app.applyGroupKeywordRules(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	groups := getSlice(getMap(app.cfg, "nodeRegistry"), "groups")
	g, _ := groups[0].(map[string]any)
	got := currentGroupMembers(g)
	if len(got) != 1 || got[0] != "eu-1" {
		t.Fatalf("expected stale member removed and set to [eu-1], got %#v", got)
	}
}

func TestSplitKeywords(t *testing.T) {
	cases := map[string][]string{
		"":                   nil,
		"hk, hongkong,  hk ": {"hk", "hongkong"},
		"US":                 {"US"},
		",,,,,":              nil,
	}
	for input, want := range cases {
		got := splitKeywords(input)
		if len(got) != len(want) {
			t.Fatalf("splitKeywords(%q) = %#v, want %#v", input, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("splitKeywords(%q) = %#v, want %#v", input, got, want)
			}
		}
	}
}

func TestSameTagSetToStrings(t *testing.T) {
	if !sameTagSetToStrings([]string{"a", "b"}, []string{"b", "a"}) {
		t.Fatal("expected equal sets")
	}
	if sameTagSetToStrings([]string{"a", "b"}, []string{"a", "c"}) {
		t.Fatal("expected different sets")
	}
	if sameTagSetToStrings([]string{"a"}, []string{"a", "a"}) {
		t.Fatal("expected different lengths to differ")
	}
}
