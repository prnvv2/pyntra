package cvss

import "testing"

func TestBaseScore_V31(t *testing.T) {
	cases := []struct {
		name   string
		vector string
		score  float64
		band   Severity
	}{
		{"critical-network-full", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8, SeverityCritical},
		{"scope-changed-max", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H", 10.0, SeverityCritical},
		{"no-impact", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N", 0.0, SeverityNone},
		{"local-priv-full", "CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H", 7.8, SeverityHigh},
		{"low-info-leak", "CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:L/I:N/A:N", 3.1, SeverityLow},
		{"v30-conf-only", "CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N", 7.5, SeverityHigh},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score, band, err := Score(tc.vector)
			if err != nil {
				t.Fatalf("Score(%q) error: %v", tc.vector, err)
			}
			if score != tc.score {
				t.Errorf("Score(%q) = %.1f, want %.1f", tc.vector, score, tc.score)
			}
			if band != tc.band {
				t.Errorf("Band(%q) = %s, want %s", tc.vector, band, tc.band)
			}
		})
	}
}

func TestParseVector_Errors(t *testing.T) {
	bad := []string{
		"",
		"not-a-vector",
		"CVSS:2.0/AV:N",
		"CVSS:3.1/AV:X/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", // invalid AV
		"CVSS:3.1/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",      // missing AV
	}
	for _, s := range bad {
		if _, err := ParseVector(s); err == nil {
			t.Errorf("ParseVector(%q) expected error, got nil", s)
		}
	}
}

func TestBand(t *testing.T) {
	cases := []struct {
		score float64
		want  Severity
	}{
		{0.0, SeverityNone},
		{0.1, SeverityLow},
		{3.9, SeverityLow},
		{4.0, SeverityMedium},
		{6.9, SeverityMedium},
		{7.0, SeverityHigh},
		{8.9, SeverityHigh},
		{9.0, SeverityCritical},
		{10.0, SeverityCritical},
	}
	for _, tc := range cases {
		if got := Band(tc.score); got != tc.want {
			t.Errorf("Band(%.1f) = %s, want %s", tc.score, got, tc.want)
		}
	}
}

func TestBaseScore_V4Approximation(t *testing.T) {
	// A v4.0 vector should parse and yield a deterministic (approximate) score.
	v := "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"
	score, band, err := Score(v)
	if err != nil {
		t.Fatalf("Score(%q) error: %v", v, err)
	}
	if score <= 0 || score > 10 {
		t.Errorf("Score(%q) = %.1f out of range", v, score)
	}
	if band == "" {
		t.Errorf("Score(%q) returned empty band", v)
	}
}
