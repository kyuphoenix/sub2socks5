package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestSocks5TestRejectsInvalidPort(t *testing.T) {
	app := newConcurrencyTestApp(t)
	handler := newHTTPHandler(app)

	req := httptest.NewRequest(http.MethodPost, "/api/socks5/test", bytes.NewBufferString(`{"listen":"127.0.0.1","port":0}`))
	req.Header.Set("content-type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, resp.Code, resp.Body.String())
	}
}

func TestSocks5TestRejectsNonPOST(t *testing.T) {
	app := newConcurrencyTestApp(t)
	handler := newHTTPHandler(app)

	req := httptest.NewRequest(http.MethodGet, "/api/socks5/test", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.Code)
	}
}

// TestSocks5TestFallbackFailure ensures that when the target socks5 service is
// unreachable, the endpoint returns a proper error instead of hanging.
func TestSocks5TestUnreachableService(t *testing.T) {
	app := newConcurrencyTestApp(t)
	handler := newHTTPHandler(app)

	req := httptest.NewRequest(http.MethodPost, "/api/socks5/test", bytes.NewBufferString(`{"listen":"127.0.0.1","port":9,"timeoutMs":300}`))
	req.Header.Set("content-type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadGateway, resp.Code, resp.Body.String())
	}
}

func TestSocks5TestUnknownSource(t *testing.T) {
	app := newConcurrencyTestApp(t)
	handler := newHTTPHandler(app)

	req := httptest.NewRequest(http.MethodPost, "/api/socks5/test", bytes.NewBufferString(`{"listen":"127.0.0.1","port":9,"source":"nonexistent.example","timeoutMs":300}`))
	req.Header.Set("content-type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadGateway, resp.Code, resp.Body.String())
	}
}

func TestSocks5TestValidSourceFiltersTarget(t *testing.T) {
	socksHost, socksPort := startFakeSocksEchoServer(t)
	app := newConcurrencyTestApp(t)
	handler := newHTTPHandler(app)

	payload := fmt.Sprintf(`{"listen":%q,"port":%d,"source":"ifconfig.me","timeoutMs":1000}`, socksHost, socksPort)
	req := httptest.NewRequest(http.MethodPost, "/api/socks5/test", bytes.NewBufferString(payload))
	req.Header.Set("content-type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if mustStr(body["source"]) != "ifconfig.me" {
		t.Fatalf("unexpected source: %v", body["source"])
	}
	if mustStr(body["ip"]) != "203.0.113.7" {
		t.Fatalf("unexpected ip: %v", body["ip"])
	}
}

// startFakeSocksEchoServer runs a minimal SOCKS5 server that accepts a connect
// request and responds to the tunneled HTTP GET with a fixed egress IP.
func startFakeSocksEchoServer(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFakeSocks(conn)
		}
	}()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr failed: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func handleFakeSocks(conn net.Conn) {
	defer conn.Close()

	hello := make([]byte, 3)
	if _, err := io.ReadFull(conn, hello); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Consume the connect request: ver(1) cmd(1) rsv(1) atyp(1) then the addr/port.
	prefix := make([]byte, 4)
	if _, err := io.ReadFull(conn, prefix); err != nil {
		return
	}
	atyp := prefix[3]
	switch atyp {
	case 0x01:
		if _, err := io.CopyN(io.Discard, conn, 4+2); err != nil {
			return
		}
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return
		}
		if _, err := io.CopyN(io.Discard, conn, int64(length[0])+2); err != nil {
			return
		}
	case 0x04:
		if _, err := io.CopyN(io.Discard, conn, 16+2); err != nil {
			return
		}
	default:
		return
	}

	// Connect success reply (IPv4, port 0).
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}); err != nil {
		return
	}

	// Consume the HTTP request headers that follow the tunneled connect.
	if _, err := readHTTPHeaders(conn); err != nil {
		return
	}

	// Respond with a fixed egress IP in the body.
	const body = "203.0.113.7\n"
	resp := "HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\nConnection: close\r\n\r\n" + body
	if _, err := conn.Write([]byte(resp)); err != nil {
		return
	}
}

func readHTTPHeaders(r io.Reader) (string, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 512)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if bytes.Contains(buf.Bytes(), []byte("\r\n\r\n")) {
				return buf.String(), nil
			}
		}
		if err != nil {
			return buf.String(), err
		}
	}
}
