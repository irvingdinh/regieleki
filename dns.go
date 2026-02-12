package main

import (
	"encoding/binary"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	udpBufSize     = 4096
	forwardTimeout = 2 * time.Second
)

const maxConcurrentQueries = 1000

type DNSServer struct {
	conn      *net.UDPConn
	store     *Store
	upstreams []string
	pool      sync.Pool
	ready     chan struct{}
	sem       chan struct{}
}

func NewDNSServer(store *Store, upstreams []string) *DNSServer {
	return &DNSServer{
		store:     store,
		upstreams: upstreams,
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, udpBufSize)
				return &b
			},
		},
		ready: make(chan struct{}),
		sem:   make(chan struct{}, maxConcurrentQueries),
	}
}

func (s *DNSServer) ListenAndServe(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	s.conn = conn
	close(s.ready)
	slog.Info("dns server listening", "addr", addr, "upstreams", s.upstreams)

	for {
		bufPtr := s.pool.Get().(*[]byte)
		n, remoteAddr, err := conn.ReadFromUDP(*bufPtr)
		if err != nil {
			s.pool.Put(bufPtr)
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}

		query := make([]byte, n)
		copy(query, (*bufPtr)[:n])
		s.pool.Put(bufPtr)

		select {
		case s.sem <- struct{}{}:
			go func() {
				defer func() { <-s.sem }()
				s.handleQuery(query, remoteAddr)
			}()
		default:
			slog.Warn("dropping query, at capacity", "remote", remoteAddr)
		}
	}
}

func (s *DNSServer) Close() {
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *DNSServer) handleQuery(buf []byte, addr *net.UDPAddr) {
	n := len(buf)
	if n < 12 {
		return
	}

	// Must be a query (QR bit = 0)
	if buf[2]&0x80 != 0 {
		return
	}

	qdcount := binary.BigEndian.Uint16(buf[4:6])
	if qdcount == 0 {
		return
	}

	// Parse first question
	qname, offset := parseDNSName(buf, 12)
	if offset < 0 || offset+4 > n {
		return
	}

	qtype := binary.BigEndian.Uint16(buf[offset : offset+2])
	questionEnd := offset + 4

	// Resolve against custom records
	records, authoritative := s.store.Resolve(qname, qtype)

	if authoritative {
		resp := buildDNSResponse(buf[:n], questionEnd, records)
		s.conn.WriteToUDP(resp, addr)
		if len(records) > 0 {
			slog.Debug("resolved", "domain", qname, "type", qtype, "answers", len(records))
		}
		return
	}

	// Forward to upstream
	resp := s.forwardQuery(buf)
	if resp != nil {
		s.conn.WriteToUDP(resp, addr)
	} else {
		s.conn.WriteToUDP(buildServFail(buf[:n], questionEnd), addr)
	}
}

// parseDNSName reads a DNS name from the wire format starting at offset.
// Returns the name as a dotted string and the offset after the name.
func parseDNSName(buf []byte, offset int) (string, int) {
	var parts []string
	jumped := false
	returnOffset := offset
	maxJumps := 10 // prevent infinite loops from malicious pointers

	for maxJumps > 0 {
		if offset >= len(buf) {
			return "", -1
		}

		length := int(buf[offset])

		if length == 0 {
			if !jumped {
				returnOffset = offset + 1
			}
			break
		}

		// Compression pointer
		if length&0xC0 == 0xC0 {
			if offset+1 >= len(buf) {
				return "", -1
			}
			ptr := int(buf[offset]&0x3F)<<8 | int(buf[offset+1])
			if !jumped {
				returnOffset = offset + 2
			}
			offset = ptr
			jumped = true
			maxJumps--
			continue
		}

		offset++
		if offset+length > len(buf) {
			return "", -1
		}
		parts = append(parts, string(buf[offset:offset+length]))
		offset += length
	}

	if maxJumps == 0 {
		return "", -1
	}

	return strings.Join(parts, "."), returnOffset
}

func encodeDNSName(name string) []byte {
	var buf []byte
	for _, part := range strings.Split(name, ".") {
		if len(part) == 0 {
			continue
		}
		buf = append(buf, byte(len(part)))
		buf = append(buf, part...)
	}
	buf = append(buf, 0)
	return buf
}

func buildDNSResponse(query []byte, questionEnd int, records []Record) []byte {
	// Build answers first to get accurate count
	var answers []byte
	var ancount uint16
	namePtr := []byte{0xC0, 0x0C} // pointer to name at offset 12

	for _, r := range records {
		var rdata []byte
		var rtype uint16

		switch r.Type {
		case "A":
			ip := net.ParseIP(r.Value).To4()
			if ip == nil {
				continue
			}
			rtype = 1
			rdata = ip
		case "AAAA":
			ip := net.ParseIP(r.Value)
			if ip == nil || ip.To4() != nil {
				continue
			}
			rtype = 28
			rdata = ip.To16()
		case "CNAME":
			rtype = 5
			rdata = encodeDNSName(r.Value)
		default:
			continue
		}

		answers = append(answers, namePtr...)
		answers = append(answers, byte(rtype>>8), byte(rtype))
		answers = append(answers, 0, 1)    // Class IN
		answers = append(answers, 0, 0, 0, 60) // TTL = 60s
		answers = append(answers, byte(len(rdata)>>8), byte(len(rdata)))
		answers = append(answers, rdata...)
		ancount++
	}

	// Assemble response
	resp := make([]byte, 0, 12+(questionEnd-12)+len(answers))

	// Header
	resp = append(resp, query[0], query[1])                // ID
	resp = append(resp, 0x84|(query[2]&0x01), 0x80)        // QR=1 AA=1 RD=copy RA=1 RCODE=0
	resp = append(resp, 0, 1)                              // QDCOUNT
	resp = append(resp, byte(ancount>>8), byte(ancount))   // ANCOUNT
	resp = append(resp, 0, 0)                              // NSCOUNT
	resp = append(resp, 0, 0)                              // ARCOUNT

	// Question section (copied from query)
	resp = append(resp, query[12:questionEnd]...)

	// Answers
	resp = append(resp, answers...)

	return resp
}

func buildServFail(query []byte, questionEnd int) []byte {
	resp := make([]byte, 0, questionEnd)
	resp = append(resp, query[0], query[1])
	resp = append(resp, 0x80|(query[2]&0x01), 0x82) // QR=1 RD=copy RA=1 RCODE=2
	resp = append(resp, 0, 1)                        // QDCOUNT
	resp = append(resp, 0, 0)                        // ANCOUNT
	resp = append(resp, 0, 0)                        // NSCOUNT
	resp = append(resp, 0, 0)                        // ARCOUNT
	resp = append(resp, query[12:questionEnd]...)
	return resp
}

func (s *DNSServer) forwardQuery(query []byte) []byte {
	for _, upstream := range s.upstreams {
		if resp := s.forwardTo(query, upstream); resp != nil {
			return resp
		}
	}
	return nil
}

func (s *DNSServer) forwardTo(query []byte, upstream string) []byte {
	conn, err := net.DialTimeout("udp", upstream, forwardTimeout)
	if err != nil {
		return nil
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(forwardTimeout))

	if _, err := conn.Write(query); err != nil {
		return nil
	}

	buf := make([]byte, udpBufSize)
	n, err := conn.Read(buf)
	if err != nil {
		return nil
	}

	return buf[:n]
}

// getLocalIPs returns all IP addresses assigned to local interfaces.
func getLocalIPs() map[string]bool {
	ips := map[string]bool{
		"127.0.0.1": true,
		"::1":       true,
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				ips[v.IP.String()] = true
			case *net.IPAddr:
				ips[v.IP.String()] = true
			}
		}
	}
	return ips
}

// parseResolvConf reads upstream DNS servers from the system configuration.
func parseResolvConf() []string {
	localIPs := getLocalIPs()
	paths := []string{"/etc/resolv.conf", "/run/systemd/resolve/resolv.conf"}
	var servers []string

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "nameserver") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			ip := fields[1]
			if localIPs[ip] {
				continue
			}
			servers = append(servers, net.JoinHostPort(ip, "53"))
		}
		if len(servers) > 0 {
			break
		}
	}

	if len(servers) == 0 {
		servers = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}

	return servers
}
