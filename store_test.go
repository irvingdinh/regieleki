package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreNewEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Errorf("expected empty store, got %d records", len(s.List()))
	}
}

func TestStoreAddAndList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	rec, err := s.Add(Record{Domain: "app.my.local", Type: "A", Value: "100.70.30.1"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != 1 {
		t.Errorf("ID = %d, want 1", rec.ID)
	}
	if rec.Domain != "app.my.local" {
		t.Errorf("Domain = %q, want %q", rec.Domain, "app.my.local")
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("List() returned %d records, want 1", len(list))
	}
	if list[0].Value != "100.70.30.1" {
		t.Errorf("Value = %q, want %q", list[0].Value, "100.70.30.1")
	}
}

func TestStoreUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	rec, _ := s.Add(Record{Domain: "app.my.local", Type: "A", Value: "100.70.30.1"})

	updated, err := s.Update(rec.ID, "app.my.local", "A", "100.70.30.2")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Value != "100.70.30.2" {
		t.Errorf("Value = %q, want %q", updated.Value, "100.70.30.2")
	}

	// Update non-existent record
	_, err = s.Update(999, "x", "A", "1.2.3.4")
	if err == nil {
		t.Error("expected error updating non-existent record")
	}
}

func TestStoreDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	rec, _ := s.Add(Record{Domain: "app.my.local", Type: "A", Value: "100.70.30.1"})

	if err := s.Delete(rec.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Error("expected empty store after delete")
	}

	if err := s.Delete(rec.ID); err == nil {
		t.Error("expected error deleting non-existent record")
	}
}

func TestStoreResolve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(Record{Domain: "app.my.local", Type: "A", Value: "100.70.30.1"})
	s.Add(Record{Domain: "app.my.local", Type: "A", Value: "100.70.30.2"})
	s.Add(Record{Domain: "app.my.local", Type: "AAAA", Value: "fd00::1"})

	// A query
	recs, auth := s.Resolve("app.my.local", 1)
	if !auth {
		t.Error("expected authoritative")
	}
	if len(recs) != 2 {
		t.Errorf("expected 2 A records, got %d", len(recs))
	}

	// AAAA query
	recs, auth = s.Resolve("app.my.local", 28)
	if !auth {
		t.Error("expected authoritative")
	}
	if len(recs) != 1 {
		t.Errorf("expected 1 AAAA record, got %d", len(recs))
	}

	// Unknown domain
	recs, auth = s.Resolve("unknown.local", 1)
	if auth {
		t.Error("expected non-authoritative for unknown domain")
	}
	if len(recs) != 0 {
		t.Errorf("expected 0 records, got %d", len(recs))
	}

	// Case insensitive
	recs, auth = s.Resolve("APP.MY.LOCAL", 1)
	if !auth {
		t.Error("expected authoritative (case insensitive)")
	}
	if len(recs) != 2 {
		t.Errorf("expected 2 records (case insensitive), got %d", len(recs))
	}
}

func TestStoreResolveCNAMEFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(Record{Domain: "alias.local", Type: "CNAME", Value: "target.local"})

	// A query for a CNAME domain should return the CNAME as fallback
	recs, auth := s.Resolve("alias.local", 1)
	if !auth {
		t.Error("expected authoritative")
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 CNAME fallback record, got %d", len(recs))
	}
	if recs[0].Type != "CNAME" {
		t.Errorf("expected CNAME type, got %s", recs[0].Type)
	}
}

func TestStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")

	s1, _ := NewStore(path)
	s1.Add(Record{Domain: "app.my.local", Type: "A", Value: "10.0.0.1"})
	s1.Add(Record{Domain: "db.my.local", Type: "A", Value: "10.0.0.2"})

	// Create new store from same file
	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	list := s2.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 records after reload, got %d", len(list))
	}

	// IDs should be preserved
	if list[0].ID != 1 || list[1].ID != 2 {
		t.Errorf("IDs not preserved: %d, %d", list[0].ID, list[1].ID)
	}

	// NextID should continue from max
	rec, _ := s2.Add(Record{Domain: "new.local", Type: "A", Value: "10.0.0.3"})
	if rec.ID != 3 {
		t.Errorf("expected next ID 3, got %d", rec.ID)
	}
}

func TestStoreLoadSkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	data := "1\tapp.local\tA\t10.0.0.1\nbad line no tabs\n2\tdb.local\tA\t10.0.0.2\n"
	os.WriteFile(path, []byte(data), 0644)

	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 2 {
		t.Errorf("expected 2 records, got %d", len(s.List()))
	}
}

func TestStoreLoadTruncatedLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	data := "1\tapp.local\tA\t10.0.0.1\n2\tdb.local\tA"
	os.WriteFile(path, []byte(data), 0644)

	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 record, got %d", len(list))
	}
	if list[0].ID != 1 {
		t.Errorf("ID = %d, want 1", list[0].ID)
	}
}

func TestStoreLoadInvalidID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	data := "abc\tapp.local\tA\t10.0.0.1\n2\tdb.local\tA\t10.0.0.2\n"
	os.WriteFile(path, []byte(data), 0644)

	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 record, got %d", len(list))
	}
	if list[0].ID != 2 {
		t.Errorf("ID = %d, want 2", list[0].ID)
	}
}

func TestStoreLoadBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	data := "1\tapp.local\tA\t10.0.0.1\n\n\n2\tdb.local\tA\t10.0.0.2\n"
	os.WriteFile(path, []byte(data), 0644)

	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 2 {
		t.Errorf("expected 2 records, got %d", len(s.List()))
	}
}

func TestStoreSaveFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	s.Add(Record{Domain: "app.local", Type: "A", Value: "10.0.0.1"})
	s.Add(Record{Domain: "v6.local", Type: "AAAA", Value: "fd00::1"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "1\tapp.local\tA\t10.0.0.1\n2\tv6.local\tAAAA\tfd00::1\n"
	if string(data) != want {
		t.Errorf("file contents = %q, want %q", string(data), want)
	}
}

func TestStoreLoadNextIDAfterSkippedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.tsv")
	data := "1\tapp.local\tA\t10.0.0.1\nbad line\n5\tdb.local\tA\t10.0.0.2\n"
	os.WriteFile(path, []byte(data), 0644)

	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 2 {
		t.Fatalf("expected 2 records, got %d", len(s.List()))
	}

	rec, err := s.Add(Record{Domain: "new.local", Type: "A", Value: "10.0.0.3"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != 6 {
		t.Errorf("next ID = %d, want 6", rec.ID)
	}
}
