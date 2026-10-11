// Package cvss implements a pure-Go, offline, deterministic CVSS calculator.
//
// The guiding principle (see docs/DESIGN-concurrent-orchestration-and-cve-cvss.md):
// a CVSS score is ALWAYS computed in code from a vector string, never a number
// an LLM guesses. The formula is self-contained — no network, no data files — so
// scoring always works, even fully air-gapped.
//
// CVSS v3.1 base score is implemented per the FIRST specification
// (https://www.first.org/cvss/v3-1/specification-document). v4.0 base scoring is
// provided behind ParseVector's version detection with a documented approximation.
package cvss

import (
	"fmt"
	"math"
	"strings"
)

// Severity is the qualitative band derived from a numeric base score.
type Severity string

const (
	SeverityNone     Severity = "None"
	SeverityLow      Severity = "Low"
	SeverityMedium   Severity = "Medium"
	SeverityHigh     Severity = "High"
	SeverityCritical Severity = "Critical"
)

// Version identifies the CVSS specification a vector is expressed in.
type Version string

const (
	Version31 Version = "3.1"
	Version30 Version = "3.0"
	Version40 Version = "4.0"
)

// Vector is a parsed CVSS base-metric vector. Only the base metrics required for
// the base score are retained; temporal/environmental metrics are accepted during
// parsing but ignored by BaseScore (base score is, by definition, base-only).
type Vector struct {
	Version Version

	AttackVector       string // N, A, L, P
	AttackComplexity   string // L, H
	PrivilegesRequired string // N, L, H
	UserInteraction    string // N, R
	Scope              string // U, C
	Confidentiality    string // H, L, N
	Integrity          string // H, L, N
	Availability       string // H, L, N

	// Raw is the original, unmodified vector string.
	Raw string
}

// Band maps a numeric base score to its qualitative severity band per the CVSS
// v3.1 rating scale.
func Band(score float64) Severity {
	switch {
	case score <= 0.0:
		return SeverityNone
	case score < 4.0:
		return SeverityLow
	case score < 7.0:
		return SeverityMedium
	case score < 9.0:
		return SeverityHigh
	default:
		return SeverityCritical
	}
}

// metric lookup tables (CVSS v3.1 specification, section 7.4).
var (
	attackVectorScore = map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}
	attackComplexity  = map[string]float64{"L": 0.77, "H": 0.44}
	userInteraction   = map[string]float64{"N": 0.85, "R": 0.62}
	ciaScore          = map[string]float64{"H": 0.56, "L": 0.22, "N": 0.0}
	// Privileges Required is scope-dependent.
	prUnchanged = map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}
	prChanged   = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.5}
)

// ParseVector strictly parses a CVSS vector string into a Vector. It validates
// the version prefix, the presence of every mandatory base metric, and that each
// metric value is legal. Unknown or temporal/environmental metrics are tolerated
// and ignored so that real-world vectors (which often carry extras) still parse.
func ParseVector(s string) (Vector, error) {
	v := Vector{Raw: strings.TrimSpace(s)}
	if v.Raw == "" {
		return v, fmt.Errorf("empty CVSS vector")
	}

	parts := strings.Split(v.Raw, "/")
	if len(parts) < 2 {
		return v, fmt.Errorf("malformed CVSS vector %q: expected 'CVSS:x.y/...'", s)
	}

	// Leading prefix: CVSS:3.1 / CVSS:3.0 / CVSS:4.0
	prefix := strings.SplitN(parts[0], ":", 2)
	if len(prefix) != 2 || !strings.EqualFold(prefix[0], "CVSS") {
		return v, fmt.Errorf("malformed CVSS vector %q: missing 'CVSS:' prefix", s)
	}
	switch prefix[1] {
	case "3.1":
		v.Version = Version31
	case "3.0":
		v.Version = Version30
	case "4.0":
		v.Version = Version40
	default:
		return v, fmt.Errorf("unsupported CVSS version %q", prefix[1])
	}

	seen := map[string]string{}
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			return v, fmt.Errorf("malformed metric %q in vector", p)
		}
		seen[strings.ToUpper(kv[0])] = strings.ToUpper(kv[1])
	}

	if v.Version == Version40 {
		return parseV4(v, seen)
	}
	return parseV3(v, seen)
}

func parseV3(v Vector, seen map[string]string) (Vector, error) {
	// Base metrics are mandatory in v3.x.
	need := func(key string, valid ...string) (string, error) {
		val, ok := seen[key]
		if !ok {
			return "", fmt.Errorf("missing mandatory base metric %s", key)
		}
		for _, c := range valid {
			if val == c {
				return val, nil
			}
		}
		return "", fmt.Errorf("invalid value %q for metric %s", val, key)
	}

	var err error
	if v.AttackVector, err = need("AV", "N", "A", "L", "P"); err != nil {
		return v, err
	}
	if v.AttackComplexity, err = need("AC", "L", "H"); err != nil {
		return v, err
	}
	if v.PrivilegesRequired, err = need("PR", "N", "L", "H"); err != nil {
		return v, err
	}
	if v.UserInteraction, err = need("UI", "N", "R"); err != nil {
		return v, err
	}
	if v.Scope, err = need("S", "U", "C"); err != nil {
		return v, err
	}
	if v.Confidentiality, err = need("C", "H", "L", "N"); err != nil {
		return v, err
	}
	if v.Integrity, err = need("I", "H", "L", "N"); err != nil {
		return v, err
	}
	if v.Availability, err = need("A", "H", "L", "N"); err != nil {
		return v, err
	}
	return v, nil
}

// BaseScore computes the CVSS base score and its qualitative band from a parsed
// vector. For v3.0/v3.1 it follows the FIRST formula exactly, including the
// specified roundup. For v4.0 it uses a documented approximation (see scoreV4).
func BaseScore(v Vector) (float64, Severity) {
	if v.Version == Version40 {
		return scoreV4(v)
	}

	scopeChanged := v.Scope == "C"

	pr := prUnchanged[v.PrivilegesRequired]
	if scopeChanged {
		pr = prChanged[v.PrivilegesRequired]
	}

	iss := 1 - ((1 - ciaScore[v.Confidentiality]) *
		(1 - ciaScore[v.Integrity]) *
		(1 - ciaScore[v.Availability]))

	var impact float64
	if scopeChanged {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}

	exploitability := 8.22 *
		attackVectorScore[v.AttackVector] *
		attackComplexity[v.AttackComplexity] *
		pr *
		userInteraction[v.UserInteraction]

	var base float64
	if impact <= 0 {
		base = 0
	} else if scopeChanged {
		base = roundup(math.Min(1.08*(impact+exploitability), 10))
	} else {
		base = roundup(math.Min(impact+exploitability, 10))
	}

	return base, Band(base)
}

// roundup implements the CVSS v3.1 Roundup function (specification appendix A):
// round up to one decimal place, using an integer-scaled comparison to avoid
// floating-point drift.
func roundup(input float64) float64 {
	intInput := int64(math.Round(input * 100000))
	if intInput%10000 == 0 {
		return float64(intInput) / 100000.0
	}
	return (math.Floor(float64(intInput)/10000.0) + 1) / 10.0
}

// RiskScore maps a 0.0–10.0 CVSS base score onto the 0–100 scale the attack-chain
// builder uses for node risk (design P4: "seed attack-chain risk_score from CVSS").
func RiskScore(baseScore float64) int {
	if baseScore <= 0 {
		return 0
	}
	if baseScore >= 10 {
		return 100
	}
	return int(baseScore*10 + 0.5)
}

// Score is a convenience that parses and scores in one call.
func Score(vector string) (float64, Severity, error) {
	v, err := ParseVector(vector)
	if err != nil {
		return 0, SeverityNone, err
	}
	score, band := BaseScore(v)
	return score, band, nil
}
