package cvss

import "fmt"

// CVSS v4.0 support.
//
// A fully spec-accurate v4.0 score requires the MacroVector lookup tables from
// the FIRST v4.0 specification, which are large and rarely needed: the project's
// design doc treats v4.0 as optional because virtually all tooling (nuclei,
// trivy, NVD) still emits v3.1 vectors.
//
// We therefore parse v4.0 vectors strictly, but compute the score via a
// DOCUMENTED APPROXIMATION that maps the v4.0 base metrics onto the v3.1 base
// formula. This keeps scoring deterministic and offline while being honest that
// the result is an approximation, not the official v4.0 MacroVector score. The
// mapping is:
//
//	VC/VI/VA (vulnerable system C/I/A) -> v3.1 C/I/A
//	any of SC/SI/SA (subsequent system) != N -> Scope Changed, else Unchanged
//	AT:P (Attack Requirements present) -> raises Attack Complexity to High
//	UI:P or UI:A (passive/active) -> v3.1 UI Required; UI:N -> None
func parseV4(v Vector, seen map[string]string) (Vector, error) {
	need := func(key string, valid ...string) (string, error) {
		val, ok := seen[key]
		if !ok {
			return "", fmt.Errorf("missing mandatory v4.0 base metric %s", key)
		}
		for _, c := range valid {
			if val == c {
				return val, nil
			}
		}
		return "", fmt.Errorf("invalid value %q for v4.0 metric %s", val, key)
	}

	av, err := need("AV", "N", "A", "L", "P")
	if err != nil {
		return v, err
	}
	ac, err := need("AC", "L", "H")
	if err != nil {
		return v, err
	}
	at, err := need("AT", "N", "P")
	if err != nil {
		return v, err
	}
	pr, err := need("PR", "N", "L", "H")
	if err != nil {
		return v, err
	}
	ui, err := need("UI", "N", "P", "A")
	if err != nil {
		return v, err
	}
	vc, err := need("VC", "H", "L", "N")
	if err != nil {
		return v, err
	}
	vi, err := need("VI", "H", "L", "N")
	if err != nil {
		return v, err
	}
	va, err := need("VA", "H", "L", "N")
	if err != nil {
		return v, err
	}
	// Subsequent-system impacts are mandatory in v4.0.
	sc, err := need("SC", "H", "L", "N")
	if err != nil {
		return v, err
	}
	si, err := need("SI", "H", "L", "N", "S")
	if err != nil {
		return v, err
	}
	sa, err := need("SA", "H", "L", "N", "S")
	if err != nil {
		return v, err
	}

	// Map onto the v3.1 base metric fields.
	v.AttackVector = av
	v.AttackComplexity = ac
	if at == "P" {
		v.AttackComplexity = "H"
	}
	v.PrivilegesRequired = pr
	if ui == "N" {
		v.UserInteraction = "N"
	} else {
		v.UserInteraction = "R"
	}
	if sc != "N" || si != "N" || sa != "N" {
		v.Scope = "C"
	} else {
		v.Scope = "U"
	}
	v.Confidentiality = vc
	v.Integrity = vi
	v.Availability = va
	return v, nil
}

// scoreV4 computes the approximate v4.0 base score by reusing the v3.1 formula on
// the mapped metrics. The returned score is flagged as an approximation only in
// documentation; callers that require the official MacroVector score should
// supply a v3.1 vector instead.
func scoreV4(v Vector) (float64, Severity) {
	// The mapping already populated the v3.1 fields; temporarily present it as
	// a v3.1 vector for the shared formula.
	mapped := v
	mapped.Version = Version31
	return BaseScore(mapped)
}
