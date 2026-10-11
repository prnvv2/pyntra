package enrich

import (
	"context"
	"testing"

	"pyntra/internal/database"
	"pyntra/internal/enrich/cwe"
)

func TestEnrich_VectorFromProof(t *testing.T) {
	e := New(Options{DefaultVersion: "3.1"})
	v := &database.Vulnerability{
		Title:    "Remote code execution",
		Severity: "high",
		Proof:    "PoC confirmed. Scanner reported CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H for this issue.",
	}
	if err := e.Enrich(context.Background(), v); err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if v.CVSSBaseScore != 9.8 {
		t.Errorf("CVSSBaseScore = %.1f, want 9.8", v.CVSSBaseScore)
	}
	if v.CVSSSeverity != "Critical" {
		t.Errorf("CVSSSeverity = %q, want Critical", v.CVSSSeverity)
	}
	if v.CVSSVersion != "3.1" {
		t.Errorf("CVSSVersion = %q, want 3.1", v.CVSSVersion)
	}
}

func TestEnrich_CVEExtractionAndCWEMapping(t *testing.T) {
	mapper := cwe.New()
	// Build an in-memory mapper via the loader on a temp-free path is overkill;
	// exercise the type->CWE fallback through the real seeded map instead.
	m, err := cwe.Load("../../knowledge_base/cwe_map.yaml")
	if err == nil && m.Size() > 0 {
		mapper = m
	}

	e := New(Options{DefaultVersion: "3.1", CWEMapper: mapper})
	v := &database.Vulnerability{
		Title:    "SQL injection in login form (CVE-2021-44228 referenced nearby)",
		Type:     "sql-injection",
		Severity: "high",
	}
	if err := e.Enrich(context.Background(), v); err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if len(v.CVEIDs) != 1 || v.CVEIDs[0] != "CVE-2021-44228" {
		t.Errorf("CVEIDs = %v, want [CVE-2021-44228]", v.CVEIDs)
	}
	if mapper.Size() > 0 {
		found := false
		for _, id := range v.CWEIDs {
			if id == "CWE-89" {
				found = true
			}
		}
		if !found {
			t.Errorf("CWEIDs = %v, want to contain CWE-89", v.CWEIDs)
		}
	}
}

func TestScoreFromMetrics(t *testing.T) {
	e := New(Options{DefaultVersion: "3.1"})
	vector, score, band, err := e.ScoreFromMetrics("N", "L", "N", "N", "U", "H", "H", "H")
	if err != nil {
		t.Fatalf("ScoreFromMetrics error: %v", err)
	}
	if score != 9.8 {
		t.Errorf("score = %.1f, want 9.8", score)
	}
	if band != "Critical" {
		t.Errorf("band = %q, want Critical", band)
	}
	if vector == "" {
		t.Error("expected a non-empty canonical vector")
	}
}
