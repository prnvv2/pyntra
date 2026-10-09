package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// createEngagementsTable defines the engagement (project/scope) table. An
// engagement scopes findings, tasks and — when active — enforces which targets
// the agent's tools may touch (see internal/security/scope). A NULL/absent
// active engagement means no restriction, preserving pre-engagement behavior.
const createEngagementsTable = `
CREATE TABLE IF NOT EXISTS engagements (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	client TEXT,
	status TEXT NOT NULL DEFAULT 'active',
	is_active INTEGER NOT NULL DEFAULT 0,
	scope_json TEXT NOT NULL DEFAULT '{}',
	authorized_from DATETIME,
	authorized_to DATETIME,
	roe TEXT,
	dry_run INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

// EngagementScope is the structured authorized-target scope.
type EngagementScope struct {
	Domains    []string `json:"domains"`
	CIDRs      []string `json:"cidrs"`
	URLs       []string `json:"urls"`
	Exclusions []string `json:"exclusions"`
}

// Engagement is an authorized testing project.
type Engagement struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Client         string          `json:"client"`
	Status         string          `json:"status"`
	IsActive       bool            `json:"is_active"`
	Scope          EngagementScope `json:"scope"`
	AuthorizedFrom *time.Time      `json:"authorized_from,omitempty"`
	AuthorizedTo   *time.Time      `json:"authorized_to,omitempty"`
	ROE            string          `json:"roe,omitempty"`
	DryRun         bool            `json:"dry_run"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (db *DB) scanEngagement(s interface {
	Scan(dest ...interface{}) error
}) (*Engagement, error) {
	var e Engagement
	var scopeJSON, client, roe sql.NullString
	var from, to sql.NullTime
	var active, dryRun int
	if err := s.Scan(&e.ID, &e.Name, &client, &e.Status, &active, &scopeJSON, &from, &to, &roe, &dryRun, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.Client = client.String
	e.ROE = roe.String
	e.IsActive = active != 0
	e.DryRun = dryRun != 0
	if from.Valid {
		e.AuthorizedFrom = &from.Time
	}
	if to.Valid {
		e.AuthorizedTo = &to.Time
	}
	if scopeJSON.Valid && strings.TrimSpace(scopeJSON.String) != "" {
		_ = json.Unmarshal([]byte(scopeJSON.String), &e.Scope)
	}
	return &e, nil
}

const engagementCols = `id, name, client, status, is_active, scope_json, authorized_from, authorized_to, roe, dry_run, created_at, updated_at`

// ListEngagements returns all engagements, newest first.
func (db *DB) ListEngagements() ([]*Engagement, error) {
	rows, err := db.DB.Query(`SELECT ` + engagementCols + ` FROM engagements ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Engagement
	for rows.Next() {
		e, err := db.scanEngagement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEngagement returns one engagement by id.
func (db *DB) GetEngagement(id string) (*Engagement, error) {
	row := db.DB.QueryRow(`SELECT `+engagementCols+` FROM engagements WHERE id = ?`, id)
	return db.scanEngagement(row)
}

// GetActiveEngagement returns the engagement flagged active, or (nil, nil) when
// none is active (meaning: no scope restriction).
func (db *DB) GetActiveEngagement() (*Engagement, error) {
	row := db.DB.QueryRow(`SELECT ` + engagementCols + ` FROM engagements WHERE is_active = 1 AND status = 'active' ORDER BY updated_at DESC LIMIT 1`)
	e, err := db.scanEngagement(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// CreateEngagement inserts a new engagement.
func (db *DB) CreateEngagement(e *Engagement) (*Engagement, error) {
	if strings.TrimSpace(e.Name) == "" {
		return nil, fmt.Errorf("engagement name is required")
	}
	e.ID = uuid.NewString()
	now := time.Now()
	e.CreatedAt, e.UpdatedAt = now, now
	if e.Status == "" {
		e.Status = "active"
	}
	scopeBytes, _ := json.Marshal(e.Scope)
	_, err := db.DB.Exec(`INSERT INTO engagements (`+engagementCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Name, e.Client, e.Status, boolToInt(e.IsActive), string(scopeBytes),
		nullTime(e.AuthorizedFrom), nullTime(e.AuthorizedTo), e.ROE, boolToInt(e.DryRun), e.CreatedAt, e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if e.IsActive {
		_ = db.SetActiveEngagement(e.ID)
	}
	return e, nil
}

// UpdateEngagement updates mutable fields.
func (db *DB) UpdateEngagement(e *Engagement) error {
	scopeBytes, _ := json.Marshal(e.Scope)
	_, err := db.DB.Exec(`UPDATE engagements SET name=?, client=?, status=?, scope_json=?, authorized_from=?, authorized_to=?, roe=?, dry_run=?, updated_at=? WHERE id=?`,
		e.Name, e.Client, e.Status, string(scopeBytes), nullTime(e.AuthorizedFrom), nullTime(e.AuthorizedTo), e.ROE, boolToInt(e.DryRun), time.Now(), e.ID)
	if err != nil {
		return err
	}
	if e.IsActive {
		return db.SetActiveEngagement(e.ID)
	}
	return nil
}

// SetActiveEngagement makes one engagement active and clears the flag on others.
func (db *DB) SetActiveEngagement(id string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE engagements SET is_active = 0`); err != nil {
		tx.Rollback()
		return err
	}
	if strings.TrimSpace(id) != "" {
		if _, err := tx.Exec(`UPDATE engagements SET is_active = 1, updated_at = ? WHERE id = ?`, time.Now(), id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DeleteEngagement removes an engagement.
func (db *DB) DeleteEngagement(id string) error {
	_, err := db.DB.Exec(`DELETE FROM engagements WHERE id = ?`, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
