package database

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := NewDB(path, zap.NewNop())
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSerialWriter_ConcurrentCreatesSerialize(t *testing.T) {
	db := newTestDB(t)

	// A conversation row is required by the FK on vulnerabilities.
	if _, err := db.Exec(
		"INSERT INTO conversations (id, title, created_at, updated_at) VALUES ('c1','t',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
	); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	w := NewSerialWriter(db, 16)
	defer w.Close()

	const n = 40
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = w.CreateVulnerability(context.Background(), &Vulnerability{
				ConversationID: "c1",
				Title:          "finding",
				Severity:       "medium",
			})
		}(i)
	}
	wg.Wait()

	count, err := db.CountVulnerabilities("", "c1", "", "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != n {
		t.Errorf("got %d findings, want %d", count, n)
	}
}

func TestMigrationAddsEnrichmentColumns(t *testing.T) {
	db := newTestDB(t)
	for _, col := range []string{"cve_ids", "cwe_ids", "cvss_vector", "cvss_base_score", "cvss_severity", "kev", "epss_score"} {
		var c int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('vulnerabilities') WHERE name=?", col).Scan(&c); err != nil {
			t.Fatalf("pragma: %v", err)
		}
		if c != 1 {
			t.Errorf("column %q missing after migration", col)
		}
	}
}

func TestCVECacheRoundTrip(t *testing.T) {
	db := newTestDB(t)
	in := &CVECacheEntry{
		CVEID:       "CVE-2021-44228",
		CVSSVersion: "3.1",
		CVSSVector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H",
		CVSSScore:   10.0,
		CWEIDs:      []string{"CWE-502", "CWE-917"},
		Source:      "nvd",
	}
	if err := db.UpsertCVECache(in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := db.GetCVECache("CVE-2021-44228")
	if err != nil || got == nil {
		t.Fatalf("get: %v (got=%v)", err, got)
	}
	if got.CVSSScore != 10.0 || len(got.CWEIDs) != 2 || got.Source != "nvd" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
