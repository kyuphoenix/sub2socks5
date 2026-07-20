package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHTTPServerHasConnectionTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newHTTPServer("127.0.0.1:18080", handler)

	if server.Handler == nil {
		t.Fatal("server handler must be configured")
	}
	if server.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("unexpected ReadHeaderTimeout: %s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 30*time.Second {
		t.Fatalf("unexpected ReadTimeout: %s", server.ReadTimeout)
	}
	if server.WriteTimeout != 35*time.Minute {
		t.Fatalf("unexpected WriteTimeout: %s", server.WriteTimeout)
	}
	if server.IdleTimeout != 2*time.Minute {
		t.Fatalf("unexpected IdleTimeout: %s", server.IdleTimeout)
	}
	if server.MaxHeaderBytes != 64<<10 {
		t.Fatalf("unexpected MaxHeaderBytes: %d", server.MaxHeaderBytes)
	}
}

func TestAPIRequestBodyLimitReturnsPayloadTooLarge(t *testing.T) {
	app := newConcurrencyTestApp(t)
	handler := newHTTPHandler(app)
	body := append([]byte(`{"raw":"`), bytes.Repeat([]byte("x"), int(maxAPIRequestBodyBytes))...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/import", bytes.NewReader(body))
	req.Header.Set("content-type", "application/json")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d: %s", http.StatusRequestEntityTooLarge, resp.Code, resp.Body.String())
	}
}
