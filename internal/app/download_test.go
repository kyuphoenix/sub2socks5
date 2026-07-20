package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewKernelDownloadClientHasFiniteTimeouts(t *testing.T) {
	client := newKernelDownloadClient()
	if client.Timeout != kernelDownloadTimeout {
		t.Fatalf("unexpected client timeout: %s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	if transport.ResponseHeaderTimeout <= 0 {
		t.Fatal("ResponseHeaderTimeout must be configured")
	}
	if transport.TLSHandshakeTimeout <= 0 {
		t.Fatal("TLSHandshakeTimeout must be configured")
	}
	if transport.DialContext == nil {
		t.Fatal("DialContext must be configured")
	}
}

func TestDownloadKernelArchiveRejectsOversizedContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "2048")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := downloadKernelArchive(context.Background(), server.Client(), server.URL, filepath.Join(t.TempDir(), "kernel.zip"), 0, 1024, time.Second, nil)
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("expected maximum-size error, got %v", err)
	}
}

func TestDownloadKernelArchiveCancelsStalledBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	started := time.Now()
	_, err := downloadKernelArchive(context.Background(), server.Client(), server.URL, filepath.Join(t.TempDir(), "kernel.zip"), 0, 1024, 50*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("expected stalled-download error, got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("stalled download was not cancelled promptly: %s", time.Since(started))
	}
}

func TestDownloadProgressLimiterThrottlesSmallChunks(t *testing.T) {
	base := time.Unix(100, 0)
	limiter := downloadProgressLimiter{lastAt: base}
	if limiter.shouldReport(base.Add(10*time.Millisecond), 64<<10, false) {
		t.Fatal("first small chunk should not report immediately")
	}
	if !limiter.shouldReport(base.Add(20*time.Millisecond), kernelDownloadProgressBytes, false) {
		t.Fatal("byte threshold should trigger a report")
	}
	if limiter.shouldReport(base.Add(30*time.Millisecond), kernelDownloadProgressBytes+(64<<10), false) {
		t.Fatal("small chunk immediately after a report should be throttled")
	}
	if !limiter.shouldReport(base.Add(20*time.Millisecond+kernelDownloadProgressInterval+time.Millisecond), kernelDownloadProgressBytes+(128<<10), false) {
		t.Fatal("time threshold should trigger a report")
	}
	if !limiter.shouldReport(base.Add(20*time.Millisecond+kernelDownloadProgressInterval+2*time.Millisecond), kernelDownloadProgressBytes+(129<<10), true) {
		t.Fatal("forced final update should always report new progress")
	}
}
