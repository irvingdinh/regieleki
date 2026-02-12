package main

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")

	// First call creates the token
	token1, err := loadOrCreateToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(token1) != 64 {
		t.Errorf("token length = %d, want 64", len(token1))
	}
	if _, err := hex.DecodeString(token1); err != nil {
		t.Errorf("token is not valid hex: %v", err)
	}

	// Second call returns the same token
	token2, err := loadOrCreateToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if token2 != token1 {
		t.Errorf("token changed: %q != %q", token2, token1)
	}

	// File permissions should be 0600
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %o, want 600", perm)
	}
}

func TestRequireAuth_ValidToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth("test-token", inner)

	req := httptest.NewRequest("GET", "/api/records", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestRequireAuth_MissingToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth("test-token", inner)

	req := httptest.NewRequest("GET", "/api/records", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireAuth_WrongToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth("test-token", inner)

	req := httptest.NewRequest("GET", "/api/records", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireAuth_StaticNoAuth(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth("test-token", inner)

	// Static files should not require auth
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
