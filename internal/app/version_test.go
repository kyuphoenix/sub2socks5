package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAppVersionPriority(t *testing.T) {
	tempDir := t.TempDir()

	// Case 1: Default fallback when no file and no embedded version
	app1 := &App{
		rootDir: tempDir,
	}
	if v := app1.resolveAppVersion(); v != defaultAppVersion {
		t.Fatalf("expected default version %q, got %q", defaultAppVersion, v)
	}

	// Case 2: Falls back to embedded version when file does not exist
	app2 := &App{
		rootDir:         tempDir,
		embeddedVersion: "0.2.0",
	}
	if v := app2.resolveAppVersion(); v != "0.2.0" {
		t.Fatalf("expected embedded version 0.2.0, got %q", v)
	}

	// Case 3: Priority: file overrides embedded version
	versionPath := filepath.Join(tempDir, "version")
	if err := os.WriteFile(versionPath, []byte("  1.0.0-rc1 \n"), 0o644); err != nil {
		t.Fatalf("failed to write version file: %v", err)
	}
	if v := app2.resolveAppVersion(); v != "1.0.0-rc1" {
		t.Fatalf("expected file version 1.0.0-rc1, got %q", v)
	}
}

func TestFormatDisplayVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0.1.0", "v0.1.0"},
		{"v0.1.0", "v0.1.0"},
		{"V1.2.3", "V1.2.3"},
		{" 2.0.0 ", "v2.0.0"},
		{"", "v" + defaultAppVersion},
	}

	for _, tt := range tests {
		actual := formatDisplayVersion(tt.input)
		if actual != tt.expected {
			t.Errorf("formatDisplayVersion(%q) = %q; want %q", tt.input, actual, tt.expected)
		}
	}
}

func TestHandleVersionAPI(t *testing.T) {
	app := newConcurrencyTestApp(t)
	versionPath := filepath.Join(app.rootDir, "version")
	if err := os.WriteFile(versionPath, []byte("0.9.5\n"), 0o644); err != nil {
		t.Fatalf("failed to write version file: %v", err)
	}

	handler := newHTTPHandler(app)

	// GET /api/version
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.Code)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if data["version"] != "0.9.5" {
		t.Errorf("expected version 0.9.5, got %v", data["version"])
	}
	if data["displayVersion"] != "v0.9.5" {
		t.Errorf("expected displayVersion v0.9.5, got %v", data["displayVersion"])
	}

	// POST /api/version should be 405 Method Not Allowed
	reqPost := httptest.NewRequest(http.MethodPost, "/api/version", nil)
	respPost := httptest.NewRecorder()
	handler.ServeHTTP(respPost, reqPost)
	if respPost.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", respPost.Code)
	}
}

func TestHandleConfigIncludesVersion(t *testing.T) {
	app := newConcurrencyTestApp(t)
	versionPath := filepath.Join(app.rootDir, "version")
	if err := os.WriteFile(versionPath, []byte("0.3.4\n"), 0o644); err != nil {
		t.Fatalf("failed to write version file: %v", err)
	}

	handler := newHTTPHandler(app)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.Code)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if data["appVersion"] != "0.3.4" {
		t.Errorf("expected appVersion 0.3.4 in /api/config, got %v", data["appVersion"])
	}
	if data["displayVersion"] != "v0.3.4" {
		t.Errorf("expected displayVersion v0.3.4 in /api/config, got %v", data["displayVersion"])
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"0.1.0", "1.2.5", -1},
		{"v0.1.0", "v1.2.5", -1},
		{"1.2.5", "1.2.5", 0},
		{"v1.2.5", "1.2.5", 0},
		{"1.3.0", "1.2.5", 1},
		{"1.2.5-beta", "1.2.5", 0},
		{"1.2", "1.2.1", -1},
		{"1.2.0.1", "1.2.0", 1},
	}

	for _, tt := range tests {
		actual := compareSemver(tt.v1, tt.v2)
		if actual != tt.expected {
			t.Errorf("compareSemver(%q, %q) = %d; want %d", tt.v1, tt.v2, actual, tt.expected)
		}
	}
}

func TestPickAppAsset(t *testing.T) {
	assets := []any{
		map[string]any{"name": "sub2socks5-linux-x64.zip", "browser_download_url": "https://example.com/linux"},
		map[string]any{"name": "sub2socks5-windows-x64.zip", "browser_download_url": "https://example.com/win-x64"},
		map[string]any{"name": "sub2socks5-windows-arm64.zip", "browser_download_url": "https://example.com/win-arm64"},
	}

	pickedWin := pickAppAsset(assets, "windows", "amd64")
	if pickedWin == nil || pickedWin["name"] != "sub2socks5-windows-x64.zip" {
		t.Fatalf("expected sub2socks5-windows-x64.zip, got %v", pickedWin)
	}

	pickedLinux := pickAppAsset(assets, "linux", "amd64")
	if pickedLinux == nil || pickedLinux["name"] != "sub2socks5-linux-x64.zip" {
		t.Fatalf("expected sub2socks5-linux-x64.zip, got %v", pickedLinux)
	}

	pickedUnknown := pickAppAsset(assets, "freebsd", "mips")
	if pickedUnknown != nil {
		t.Fatalf("expected nil for unsupported platform, got %v", pickedUnknown)
	}
}

func TestHandleAppUpdateApply(t *testing.T) {
	app := newConcurrencyTestApp(t)
	versionPath := filepath.Join(app.rootDir, "version")
	if err := os.WriteFile(versionPath, []byte("0.1.0\n"), 0o644); err != nil {
		t.Fatalf("failed to write version file: %v", err)
	}

	handler := newHTTPHandler(app)

	// Valid update
	body := strings.NewReader(`{"version":"v1.2.5"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/app/update/apply", body)
	req.Header.Set("content-type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.Code, resp.Body.String())
	}

	content, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatalf("failed to read version file: %v", err)
	}
	if strings.TrimSpace(string(content)) != "1.2.5" {
		t.Fatalf("expected version file to be updated to 1.2.5, got %q", string(content))
	}
	if app.resolveAppVersion() != "1.2.5" {
		t.Fatalf("expected resolved version to be 1.2.5, got %q", app.resolveAppVersion())
	}

	// Missing version
	bodyInvalid := strings.NewReader(`{}`)
	reqInvalid := httptest.NewRequest(http.MethodPost, "/api/app/update/apply", bodyInvalid)
	reqInvalid.Header.Set("content-type", "application/json")
	respInvalid := httptest.NewRecorder()
	handler.ServeHTTP(respInvalid, reqInvalid)

	if respInvalid.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", respInvalid.Code)
	}
}
