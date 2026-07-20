package app

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestCaptureLogsBoundsUnterminatedLineAndFlushesEOF(t *testing.T) {
	app := newConcurrencyTestApp(t)
	input := strings.Repeat("x", maxRuntimeLogLineBytes+4096)

	app.captureLogs(io.NopCloser(strings.NewReader(input)))

	app.mu.RLock()
	logs := append([]string(nil), getStringSlice(app.runtimeInfo, "logs")...)
	app.mu.RUnlock()
	if len(logs) != 2 {
		t.Fatalf("expected existing log plus one flushed partial line, got %d: %#v", len(logs), logs)
	}
	parts := strings.SplitN(logs[1], " ", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected runtime log format: %q", logs[1])
	}
	if len([]byte(parts[1])) > maxRuntimeLogLineBytes {
		t.Fatalf("captured log line exceeded %d bytes: %d", maxRuntimeLogLineBytes, len([]byte(parts[1])))
	}
	if !strings.Contains(parts[1], runtimeLogTruncatedSuffix) {
		t.Fatalf("truncated line should carry a marker: %q", parts[1])
	}
}

func TestCaptureLogsFlushesShortPartialLineAtEOF(t *testing.T) {
	app := newConcurrencyTestApp(t)
	app.captureLogs(io.NopCloser(strings.NewReader("partial line")))

	app.mu.RLock()
	logs := append([]string(nil), getStringSlice(app.runtimeInfo, "logs")...)
	app.mu.RUnlock()
	if len(logs) != 2 || !strings.HasSuffix(logs[1], " partial line") {
		t.Fatalf("partial line was not flushed at EOF: %#v", logs)
	}
}

func TestRecordRuntimeStartedPreservesAutomaticRestartAttempts(t *testing.T) {
	app := newConcurrencyTestApp(t)
	app.autoRestartAttempts = 3
	now := time.Unix(100, 0)

	app.recordRuntimeStartedLocked(now, false)
	if app.autoRestartAttempts != 3 {
		t.Fatalf("automatic restart reset attempts to %d", app.autoRestartAttempts)
	}
	if !app.runtimeStartedAt.Equal(now) {
		t.Fatalf("runtime start time was not recorded: %s", app.runtimeStartedAt)
	}

	app.recordRuntimeStartedLocked(now.Add(time.Second), true)
	if app.autoRestartAttempts != 0 {
		t.Fatalf("manual restart should reset attempts, got %d", app.autoRestartAttempts)
	}
}

func TestNextAutoRestartUsesCappedExponentialBackoff(t *testing.T) {
	attempt, delay := nextAutoRestart(0, time.Second)
	if attempt != 1 || delay != 2*time.Second {
		t.Fatalf("unexpected first restart: attempt=%d delay=%s", attempt, delay)
	}
	attempt, delay = nextAutoRestart(attempt, time.Second)
	if attempt != 2 || delay != 4*time.Second {
		t.Fatalf("unexpected second restart: attempt=%d delay=%s", attempt, delay)
	}
	for attempt < 8 {
		attempt, delay = nextAutoRestart(attempt, time.Second)
	}
	if delay != maxAutoRestartDelay {
		t.Fatalf("restart delay should be capped at %s, got %s", maxAutoRestartDelay, delay)
	}

	attempt, delay = nextAutoRestart(8, autoRestartStablePeriod)
	if attempt != 1 || delay != 2*time.Second {
		t.Fatalf("stable runtime should reset backoff: attempt=%d delay=%s", attempt, delay)
	}
}
