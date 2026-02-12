package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Record struct {
	ID     int    `json:"id"`
	Domain string `json:"domain"`
	Type   string `json:"type"`
	Value  string `json:"value"`
}

type Store struct {
	mu      sync.RWMutex
	records []Record
	nextID  int
	index   map[string][]Record
	path    string
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:  path,
		index: make(map[string][]Record),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.records = []Record{}
			s.nextID = 1
			return nil
		}
		return err
	}
	if len(data) == 0 {
		s.records = []Record{}
		s.nextID = 1
		return nil
	}

	var records []Record
	maxID := 0
	for i, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			slog.Warn("skipping malformed record", "file", s.path, "line", i+1)
			continue
		}
		id, err := strconv.Atoi(fields[0])
		if err != nil {
			slog.Warn("skipping malformed record", "file", s.path, "line", i+1, "error", err)
			continue
		}
		rtype := fields[2]
		if rtype != "A" && rtype != "AAAA" && rtype != "CNAME" {
			slog.Warn("skipping malformed record", "file", s.path, "line", i+1, "type", rtype)
			continue
		}
		records = append(records, Record{
			ID:     id,
			Domain: fields[1],
			Type:   rtype,
			Value:  fields[3],
		})
		if id > maxID {
			maxID = id
		}
	}
	s.records = records
	s.nextID = maxID + 1
	s.rebuildIndex()
	return nil
}

func (s *Store) save() error {
	var buf strings.Builder
	for _, r := range s.records {
		buf.WriteString(strconv.Itoa(r.ID))
		buf.WriteByte('\t')
		buf.WriteString(r.Domain)
		buf.WriteByte('\t')
		buf.WriteString(r.Type)
		buf.WriteByte('\t')
		buf.WriteString(r.Value)
		buf.WriteByte('\n')
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".regieleki-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(buf.String()); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

func (s *Store) rebuildIndex() {
	s.index = make(map[string][]Record, len(s.records))
	for _, r := range s.records {
		key := strings.ToLower(r.Domain)
		s.index[key] = append(s.index[key], r)
	}
}

func (s *Store) List() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Record, len(s.records))
	copy(result, s.records)
	return result
}

// Resolve looks up records for a domain. Returns matching records and whether
// the domain is managed by us (authoritative).
func (s *Store) Resolve(domain string, qtype uint16) ([]Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := strings.ToLower(domain)
	all := s.index[key]
	if len(all) == 0 {
		return nil, false
	}

	// ANY query returns all records
	if qtype == 255 {
		result := make([]Record, len(all))
		copy(result, all)
		return result, true
	}

	var result []Record
	for _, r := range all {
		if matchType(r.Type, qtype) {
			result = append(result, r)
		}
	}

	// CNAME fallback: if no direct match for A/AAAA, return CNAME if present
	if len(result) == 0 {
		for _, r := range all {
			if r.Type == "CNAME" {
				result = append(result, r)
				break
			}
		}
	}

	return result, true
}

func matchType(rtype string, qtype uint16) bool {
	switch qtype {
	case 1:
		return rtype == "A"
	case 28:
		return rtype == "AAAA"
	case 5:
		return rtype == "CNAME"
	}
	return false
}

func (s *Store) Add(r Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = s.nextID
	s.nextID++
	r.Domain = strings.ToLower(r.Domain)
	r.Type = strings.ToUpper(r.Type)
	s.records = append(s.records, r)
	s.rebuildIndex()
	return r, s.save()
}

func (s *Store) Update(id int, domain, rtype, value string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.records {
		if r.ID == id {
			s.records[i].Domain = strings.ToLower(domain)
			s.records[i].Type = strings.ToUpper(rtype)
			s.records[i].Value = value
			s.rebuildIndex()
			return s.records[i], s.save()
		}
	}
	return Record{}, os.ErrNotExist
}

func (s *Store) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.records {
		if r.ID == id {
			s.records = append(s.records[:i], s.records[i+1:]...)
			s.rebuildIndex()
			return s.save()
		}
	}
	return os.ErrNotExist
}
