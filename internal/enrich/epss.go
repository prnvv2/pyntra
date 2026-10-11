package enrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// EPSSClient queries the FIRST EPSS API for a CVE's exploit-prediction score and
// percentile. Failures return ok=false so enrichment degrades gracefully.
type EPSSClient struct {
	http *http.Client
	base string
}

const defaultEPSSURL = "https://api.first.org/data/v1/epss"

// NewEPSSClient builds a client. A nil HTTP client gets a 15s default.
func NewEPSSClient(hc *http.Client) *EPSSClient {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &EPSSClient{http: hc, base: defaultEPSSURL}
}

type epssResponse struct {
	Data []struct {
		CVE        string `json:"cve"`
		EPSS       string `json:"epss"`
		Percentile string `json:"percentile"`
	} `json:"data"`
}

// Score returns (epss, percentile, ok) for a CVE. Both values are 0..1.
func (c *EPSSClient) Score(ctx context.Context, cveID string) (float64, float64, bool) {
	if c == nil {
		return 0, 0, false
	}
	cveID = strings.ToUpper(strings.TrimSpace(cveID))
	if cveID == "" {
		return 0, 0, false
	}
	u, _ := url.Parse(c.base)
	q := u.Query()
	q.Set("cve", cveID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, 0, false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, false
	}
	var payload epssResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || len(payload.Data) == 0 {
		return 0, 0, false
	}
	epss, err1 := strconv.ParseFloat(payload.Data[0].EPSS, 64)
	pct, err2 := strconv.ParseFloat(payload.Data[0].Percentile, 64)
	if err1 != nil {
		return 0, 0, false
	}
	if err2 != nil {
		pct = 0
	}
	return epss, pct, true
}
