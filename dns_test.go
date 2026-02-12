package main

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestParseDNSName(t *testing.T) {
	tests := []struct {
		name   string
		buf    []byte
		offset int
		want   string
		wantOk bool
	}{
		{
			name:   "simple",
			buf:    append(make([]byte, 12), 3, 'a', 'p', 'p', 2, 'm', 'y', 5, 'l', 'o', 'c', 'a', 'l', 0),
			offset: 12,
			want:   "app.my.local",
			wantOk: true,
		},
		{
			name:   "single label",
			buf:    append(make([]byte, 12), 4, 't', 'e', 's', 't', 0),
			offset: 12,
			want:   "test",
			wantOk: true,
		},
		{
			name:   "compression pointer",
			buf:    buildCompressedName(),
			offset: 26, // pointer at offset 26
			want:   "app.my.local",
			wantOk: true,
		},
		{
			name:   "empty buffer",
			buf:    []byte{},
			offset: 0,
			want:   "",
			wantOk: false,
		},
		{
			name:   "truncated label",
			buf:    append(make([]byte, 12), 10, 'a', 'b'),
			offset: 12,
			want:   "",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, offset := parseDNSName(tt.buf, tt.offset)
			if tt.wantOk {
				if offset < 0 {
					t.Fatalf("parseDNSName returned error offset %d", offset)
				}
				if got != tt.want {
					t.Errorf("parseDNSName = %q, want %q", got, tt.want)
				}
			} else {
				if offset >= 0 && got != "" {
					t.Errorf("expected error, got name=%q offset=%d", got, offset)
				}
			}
		})
	}
}

// buildCompressedName builds a buffer where offset 26 has a compression pointer to offset 12
func buildCompressedName() []byte {
	buf := make([]byte, 12) // header
	// Name at offset 12: app.my.local
	buf = append(buf, 3, 'a', 'p', 'p', 2, 'm', 'y', 5, 'l', 'o', 'c', 'a', 'l', 0)
	// Pointer at offset 26 -> offset 12
	buf = append(buf, 0xC0, 0x0C)
	return buf
}

func TestParseDNSName_CompressionLoop(t *testing.T) {
	// Build a buffer with a self-referencing compression pointer at offset 12
	buf := make([]byte, 14)
	buf[12] = 0xC0 // compression pointer
	buf[13] = 0x0C // points back to offset 12 (itself)

	name, offset := parseDNSName(buf, 12)
	if offset != -1 {
		t.Errorf("expected error offset -1, got %d (name=%q)", offset, name)
	}
}

func TestEncodeDNSName(t *testing.T) {
	tests := []struct {
		input string
		want  []byte
	}{
		{"app.my.local", []byte{3, 'a', 'p', 'p', 2, 'm', 'y', 5, 'l', 'o', 'c', 'a', 'l', 0}},
		{"test", []byte{4, 't', 'e', 's', 't', 0}},
		{"a.b", []byte{1, 'a', 1, 'b', 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := encodeDNSName(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("encodeDNSName(%q) length = %d, want %d", tt.input, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("byte[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildDNSResponse_A(t *testing.T) {
	// Build a query for app.my.local A
	query := buildTestQuery("app.my.local", 1, 1)
	questionEnd := len(query)

	records := []Record{{ID: 1, Domain: "app.my.local", Type: "A", Value: "100.70.30.1"}}
	resp := buildDNSResponse(query, questionEnd, records)

	// Verify header
	if resp[2]&0x80 == 0 {
		t.Error("QR bit not set in response")
	}
	if resp[2]&0x04 == 0 {
		t.Error("AA bit not set in response")
	}

	ancount := binary.BigEndian.Uint16(resp[6:8])
	if ancount != 1 {
		t.Errorf("ANCOUNT = %d, want 1", ancount)
	}

	// Find the answer section (after question)
	answerStart := questionEnd
	// Skip to RDATA: name(2) + type(2) + class(2) + ttl(4) + rdlength(2) = 12 bytes before RDATA
	rdataOffset := answerStart + 2 + 2 + 2 + 4 + 2
	if rdataOffset+4 > len(resp) {
		t.Fatalf("response too short: %d bytes", len(resp))
	}

	ip := net.IP(resp[rdataOffset : rdataOffset+4])
	if ip.String() != "100.70.30.1" {
		t.Errorf("RDATA IP = %s, want 100.70.30.1", ip)
	}
}

func TestBuildDNSResponse_Empty(t *testing.T) {
	query := buildTestQuery("unknown.local", 1, 1)
	questionEnd := len(query)

	resp := buildDNSResponse(query, questionEnd, nil)

	ancount := binary.BigEndian.Uint16(resp[6:8])
	if ancount != 0 {
		t.Errorf("ANCOUNT = %d, want 0", ancount)
	}

	// Response should be header + question only
	if len(resp) != questionEnd {
		t.Errorf("response length = %d, want %d", len(resp), questionEnd)
	}
}

func TestBuildDNSResponse_AAAA(t *testing.T) {
	query := buildTestQuery("v6.local", 28, 1)
	questionEnd := len(query)

	records := []Record{{ID: 1, Domain: "v6.local", Type: "AAAA", Value: "fd00::1"}}
	resp := buildDNSResponse(query, questionEnd, records)

	ancount := binary.BigEndian.Uint16(resp[6:8])
	if ancount != 1 {
		t.Errorf("ANCOUNT = %d, want 1", ancount)
	}

	answerStart := questionEnd
	rdataOffset := answerStart + 2 + 2 + 2 + 4 + 2
	if rdataOffset+16 > len(resp) {
		t.Fatalf("response too short: %d bytes", len(resp))
	}

	ip := net.IP(resp[rdataOffset : rdataOffset+16])
	expected := net.ParseIP("fd00::1")
	if !ip.Equal(expected) {
		t.Errorf("RDATA IP = %s, want %s", ip, expected)
	}
}

func TestBuildDNSResponse_CNAME(t *testing.T) {
	query := buildTestQuery("alias.local", 5, 1)
	questionEnd := len(query)

	records := []Record{{ID: 1, Domain: "alias.local", Type: "CNAME", Value: "target.local"}}
	resp := buildDNSResponse(query, questionEnd, records)

	ancount := binary.BigEndian.Uint16(resp[6:8])
	if ancount != 1 {
		t.Errorf("ANCOUNT = %d, want 1", ancount)
	}
}

func TestBuildDNSResponse_InvalidIP(t *testing.T) {
	query := buildTestQuery("bad.local", 1, 1)
	questionEnd := len(query)

	records := []Record{{ID: 1, Domain: "bad.local", Type: "A", Value: "not-an-ip"}}
	resp := buildDNSResponse(query, questionEnd, records)

	ancount := binary.BigEndian.Uint16(resp[6:8])
	if ancount != 0 {
		t.Errorf("ANCOUNT = %d, want 0 (invalid IP should be skipped)", ancount)
	}
}

func TestBuildServFail(t *testing.T) {
	query := buildTestQuery("fail.local", 1, 1)
	questionEnd := len(query)

	resp := buildServFail(query, questionEnd)

	// Check QR bit
	if resp[2]&0x80 == 0 {
		t.Error("QR bit not set")
	}

	// Check RCODE = 2 (SERVFAIL)
	if resp[3]&0x0F != 2 {
		t.Errorf("RCODE = %d, want 2", resp[3]&0x0F)
	}
}

func TestGetLocalIPs(t *testing.T) {
	ips := getLocalIPs()
	if !ips["127.0.0.1"] {
		t.Error("expected 127.0.0.1 in local IPs")
	}
	if !ips["::1"] {
		t.Error("expected ::1 in local IPs")
	}
}

func buildTestQuery(domain string, qtype, qclass uint16) []byte {
	buf := make([]byte, 12) // header
	buf[0] = 0xAB           // ID high
	buf[1] = 0xCD           // ID low
	buf[2] = 0x01           // RD=1
	buf[4] = 0x00           // QDCOUNT high
	buf[5] = 0x01           // QDCOUNT low

	buf = append(buf, encodeDNSName(domain)...)
	buf = append(buf, byte(qtype>>8), byte(qtype))
	buf = append(buf, byte(qclass>>8), byte(qclass))
	return buf
}
