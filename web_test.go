package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func testWebServer(t *testing.T) (*WebServer, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "records.tsv")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewWebServer(store, ""), store
}

func TestWebList_Empty(t *testing.T) {
	ws, _ := testWebServer(t)
	req := httptest.NewRequest("GET", "/api/records", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var records []Record
	json.NewDecoder(w.Body).Decode(&records)
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestWebCreate(t *testing.T) {
	ws, _ := testWebServer(t)
	body := `{"domain":"app.my.local","type":"A","value":"10.0.0.1"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("status = %d, want 201, body = %s", w.Code, w.Body.String())
	}

	var rec Record
	json.NewDecoder(w.Body).Decode(&rec)
	if rec.ID != 1 {
		t.Errorf("ID = %d, want 1", rec.ID)
	}
	if rec.Domain != "app.my.local" {
		t.Errorf("Domain = %q, want %q", rec.Domain, "app.my.local")
	}
}

func TestWebCreate_InvalidIP(t *testing.T) {
	ws, _ := testWebServer(t)
	body := `{"domain":"app.local","type":"A","value":"not-an-ip"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestWebCreate_MissingDomain(t *testing.T) {
	ws, _ := testWebServer(t)
	body := `{"type":"A","value":"10.0.0.1"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestWebCreate_InvalidType(t *testing.T) {
	ws, _ := testWebServer(t)
	body := `{"domain":"x.local","type":"MX","value":"mail.local"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestWebUpdate(t *testing.T) {
	ws, store := testWebServer(t)
	store.Add(Record{Domain: "app.local", Type: "A", Value: "10.0.0.1"})

	body := `{"domain":"app.local","type":"A","value":"10.0.0.2"}`
	req := httptest.NewRequest("PUT", "/api/records/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	var rec Record
	json.NewDecoder(w.Body).Decode(&rec)
	if rec.Value != "10.0.0.2" {
		t.Errorf("Value = %q, want %q", rec.Value, "10.0.0.2")
	}
}

func TestWebUpdate_NotFound(t *testing.T) {
	ws, _ := testWebServer(t)
	body := `{"domain":"x.local","type":"A","value":"10.0.0.1"}`
	req := httptest.NewRequest("PUT", "/api/records/999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestWebDelete(t *testing.T) {
	ws, store := testWebServer(t)
	store.Add(Record{Domain: "app.local", Type: "A", Value: "10.0.0.1"})

	req := httptest.NewRequest("DELETE", "/api/records/1", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 204 {
		t.Fatalf("status = %d, want 204", w.Code)
	}

	if len(store.List()) != 0 {
		t.Error("expected 0 records after delete")
	}
}

func TestWebDelete_NotFound(t *testing.T) {
	ws, _ := testWebServer(t)
	req := httptest.NewRequest("DELETE", "/api/records/999", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestWebServeHTML(t *testing.T) {
	ws, _ := testWebServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	if !strings.Contains(w.Body.String(), "Regieleki") {
		t.Error("HTML should contain 'Regieleki'")
	}
}

func TestWebCreate_AAAA(t *testing.T) {
	ws, _ := testWebServer(t)
	body := `{"domain":"v6.local","type":"AAAA","value":"fd00::1"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("status = %d, want 201", w.Code)
	}
}

func TestWebCreate_AAAA_InvalidIPv4(t *testing.T) {
	ws, _ := testWebServer(t)
	// IPv4 address is not valid for AAAA
	body := `{"domain":"v6.local","type":"AAAA","value":"10.0.0.1"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestWebCreate_CNAME(t *testing.T) {
	ws, _ := testWebServer(t)
	body := `{"domain":"alias.local","type":"CNAME","value":"target.local"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("status = %d, want 201, body = %s", w.Code, w.Body.String())
	}
}

func TestWebServeHTML_Index(t *testing.T) {
	ws, _ := testWebServer(t)
	// /index.html redirects to / with http.FileServer + embed.FS
	req := httptest.NewRequest("GET", "/index.html", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	if w.Code != 301 {
		t.Fatalf("status = %d, want 301 redirect", w.Code)
	}
}

func TestValidateRecord(t *testing.T) {
	tests := []struct {
		name    string
		rec     Record
		wantErr bool
	}{
		{"valid A", Record{Domain: "app.local", Type: "A", Value: "10.0.0.1"}, false},
		{"valid AAAA", Record{Domain: "app.local", Type: "AAAA", Value: "fd00::1"}, false},
		{"valid CNAME", Record{Domain: "app.local", Type: "CNAME", Value: "target.local"}, false},
		{"empty domain", Record{Domain: "", Type: "A", Value: "10.0.0.1"}, true},
		{"empty value", Record{Domain: "app.local", Type: "A", Value: ""}, true},
		{"bad type", Record{Domain: "app.local", Type: "MX", Value: "mail"}, true},
		{"bad IPv4", Record{Domain: "app.local", Type: "A", Value: "not-ip"}, true},
		{"IPv6 in A", Record{Domain: "app.local", Type: "A", Value: "fd00::1"}, true},
		{"IPv4 in AAAA", Record{Domain: "app.local", Type: "AAAA", Value: "10.0.0.1"}, true},
		{"bad CNAME", Record{Domain: "app.local", Type: "CNAME", Value: "has space"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRecord(&tt.rec)
			if tt.wantErr && err == "" {
				t.Error("expected validation error")
			}
			if !tt.wantErr && err != "" {
				t.Errorf("unexpected validation error: %s", err)
			}
		})
	}
}

// Integration test: full DNS flow using the real store + DNS server on a random port
func TestDNSIntegration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	store.Add(Record{Domain: "app.my.local", Type: "A", Value: "100.70.30.1"})
	store.Add(Record{Domain: "v6.my.local", Type: "AAAA", Value: "fd00::1"})

	dns := NewDNSServer(store, []string{"8.8.8.8:53"})

	// Listen on random port
	go func() {
		if err := dns.ListenAndServe("127.0.0.1:0"); err != nil {
			// Will error on close, ignore
		}
	}()

	<-dns.ready
	defer dns.Close()

	addr := dns.conn.LocalAddr().(*net.UDPAddr)

	// Query for custom A record
	query := buildTestQuery("app.my.local", 1, 1)
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.Write(query)
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	resp := buf[:n]
	// Check it's a response
	if resp[2]&0x80 == 0 {
		t.Error("QR bit not set")
	}
	// Check AA
	if resp[2]&0x04 == 0 {
		t.Error("AA bit not set")
	}
	// Check ANCOUNT >= 1
	ancount := int(resp[6])<<8 | int(resp[7])
	if ancount < 1 {
		t.Errorf("ANCOUNT = %d, want >= 1", ancount)
	}
}

func TestHTTPIntegration(t *testing.T) {
	ws, _ := testWebServer(t)
	handler := ws.Handler()

	// Create a record
	body := `{"domain":"test.local","type":"A","value":"10.0.0.1"}`
	req := httptest.NewRequest("POST", "/api/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d", w.Code)
	}

	// List records
	req = httptest.NewRequest("GET", "/api/records", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var records []Record
	json.NewDecoder(w.Body).Decode(&records)
	if len(records) != 1 {
		t.Fatalf("list: %d records, want 1", len(records))
	}

	// Update record
	body = `{"domain":"test.local","type":"A","value":"10.0.0.2"}`
	req = httptest.NewRequest("PUT", "/api/records/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: status %d, body %s", w.Code, w.Body.String())
	}

	// Delete record
	req = httptest.NewRequest("DELETE", "/api/records/1", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d", w.Code)
	}

	// Verify empty
	req = httptest.NewRequest("GET", "/api/records", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	json.NewDecoder(w.Body).Decode(&records)
	if len(records) != 0 {
		t.Fatalf("list after delete: %d records, want 0", len(records))
	}
}
