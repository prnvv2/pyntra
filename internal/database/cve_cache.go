package database

import (
	"database/sql"
	"fmt"
	"time"
)

// CVECacheEntry is a locally cached CVE record. It makes CVE enrichment
// reproducible and offline-friendly: once a CVE is fetched (from NVD or a local
// mirror) it is stored here and reused until its TTL expires.
type CVECacheEntry struct {
	CVEID       string    `json:"cve_id"`
	Description string    `json:"description"`
	CVSSVersion string    `json:"cvss_version"`
	CVSSVector  string    `json:"cvss_vector"`
	CVSSScore   float64   `json:"cvss_score"`
	CWEIDs      []string  `json:"cwe_ids"`
	EPSSScore   float64   `json:"epss_score"`
	KEV         bool      `json:"kev"`
	Published   time.Time `json:"published"`
	Modified    time.Time `json:"modified"`
	Source      string    `json:"source"` // nvd | local_mirror | nuclei
	FetchedAt   time.Time `json:"fetched_at"`
}

// GetCVECache returns a cached CVE entry, or (nil, nil) on a miss.
func (db *DB) GetCVECache(cveID string) (*CVECacheEntry, error) {
	query := `
		SELECT cve_id, COALESCE(description,''), COALESCE(cvss_version,''),
		       COALESCE(cvss_vector,''), COALESCE(cvss_score,0), COALESCE(cwe_ids,''),
		       COALESCE(epss_score,0), COALESCE(kev,0),
		       published, modified, COALESCE(source,''), fetched_at
		FROM cve_cache WHERE cve_id = ?`

	var e CVECacheEntry
	var cweIDs string
	var kev int
	var published, modified sql.NullTime
	err := db.QueryRow(query, cveID).Scan(
		&e.CVEID, &e.Description, &e.CVSSVersion, &e.CVSSVector, &e.CVSSScore,
		&cweIDs, &e.EPSSScore, &kev, &published, &modified, &e.Source, &e.FetchedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get cve_cache failed: %w", err)
	}
	e.CWEIDs = unmarshalIDs(cweIDs)
	e.KEV = kev != 0
	if published.Valid {
		e.Published = published.Time
	}
	if modified.Valid {
		e.Modified = modified.Time
	}
	return &e, nil
}

// UpsertCVECache inserts or replaces a cached CVE entry, stamping fetched_at.
func (db *DB) UpsertCVECache(e *CVECacheEntry) error {
	if e.FetchedAt.IsZero() {
		e.FetchedAt = time.Now()
	}
	query := `
		INSERT INTO cve_cache (
			cve_id, description, cvss_version, cvss_vector, cvss_score,
			cwe_ids, epss_score, kev, published, modified, source, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cve_id) DO UPDATE SET
			description=excluded.description,
			cvss_version=excluded.cvss_version,
			cvss_vector=excluded.cvss_vector,
			cvss_score=excluded.cvss_score,
			cwe_ids=excluded.cwe_ids,
			epss_score=excluded.epss_score,
			kev=excluded.kev,
			published=excluded.published,
			modified=excluded.modified,
			source=excluded.source,
			fetched_at=excluded.fetched_at`

	var published, modified interface{}
	if !e.Published.IsZero() {
		published = e.Published
	}
	if !e.Modified.IsZero() {
		modified = e.Modified
	}
	_, err := db.Exec(query,
		e.CVEID, e.Description, e.CVSSVersion, e.CVSSVector, e.CVSSScore,
		marshalIDs(e.CWEIDs), e.EPSSScore, boolToInt(e.KEV),
		published, modified, e.Source, e.FetchedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert cve_cache failed: %w", err)
	}
	return nil
}

// CVECacheFresh reports whether a cached entry is within the given TTL.
func (e *CVECacheEntry) Fresh(ttl time.Duration) bool {
	if e == nil {
		return false
	}
	return time.Since(e.FetchedAt) < ttl
}
