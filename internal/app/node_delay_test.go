package app

import (
	"errors"
	"reflect"
	"testing"
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
