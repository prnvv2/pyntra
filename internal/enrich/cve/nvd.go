package cve

import (
	"strings"
	"time"
)

// Minimal subset of the NVD CVE API 2.0 JSON schema
// (https://nvd.nist.gov/developers/vulnerabilities).

type nvdResponse struct {
	Vulnerabilities []struct {
		CVE nvdCVE `json:"cve"`
	} `json:"vulnerabilities"`
}

type nvdCVE struct {
	ID           string `json:"id"`
	Published    string `json:"published"`
	LastModified string `json:"lastModified"`
	Descriptions []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	} `json:"descriptions"`
	Metrics    nvdMetrics `json:"metrics"`
	Weaknesses []struct {
		Description []struct {
			Lang  string `json:"lang"`
			Value string `json:"value"`
		} `json:"description"`
	} `json:"weaknesses"`
}

type nvdMetrics struct {
	CVSSMetricV31 []nvdCVSSMetric `json:"cvssMetricV31"`
	CVSSMetricV30 []nvdCVSSMetric `json:"cvssMetricV30"`
}

type nvdCVSSMetric struct {
	CVSSData struct {
		Version      string  `json:"version"`
		VectorString string  `json:"vectorString"`
		BaseScore    float64 `json:"baseScore"`
	} `json:"cvssData"`
}

// parseNVDCVE converts an NVD cve object into a Record. CVSS v3.1 is preferred
// over v3.0. The score here is only a hint; the enricher recomputes it from the
// vector so the number is always code-derived.
func parseNVDCVE(c nvdCVE) *Record {
	r := &Record{CVEID: strings.ToUpper(c.ID)}

	for _, d := range c.Descriptions {
		if d.Lang == "en" {
			r.Description = d.Value
			break
		}
	}

	var metric *nvdCVSSMetric
	if len(c.Metrics.CVSSMetricV31) > 0 {
		metric = &c.Metrics.CVSSMetricV31[0]
	} else if len(c.Metrics.CVSSMetricV30) > 0 {
		metric = &c.Metrics.CVSSMetricV30[0]
	}
	if metric != nil {
		r.CVSSVersion = metric.CVSSData.Version
		r.CVSSVector = metric.CVSSData.VectorString
		r.CVSSScore = metric.CVSSData.BaseScore
	}

	seen := map[string]bool{}
	for _, w := range c.Weaknesses {
		for _, d := range w.Description {
			if d.Lang != "en" {
				continue
			}
			id := strings.ToUpper(strings.TrimSpace(d.Value))
			if strings.HasPrefix(id, "CWE-") && !seen[id] {
				seen[id] = true
				r.CWEIDs = append(r.CWEIDs, id)
			}
		}
	}

	r.Published = parseNVDTime(c.Published)
	r.Modified = parseNVDTime(c.LastModified)
	return r
}

// parseNVDTime parses NVD's timestamp formats, returning the zero time on error.
func parseNVDTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
