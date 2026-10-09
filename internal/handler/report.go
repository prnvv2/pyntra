package handler

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"pyntra/internal/database"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ReportHandler generates pentest reports from stored findings.
type ReportHandler struct {
	db     *database.DB
	logger *zap.Logger
}

// NewReportHandler constructs the handler.
func NewReportHandler(db *database.DB, logger *zap.Logger) *ReportHandler {
	return &ReportHandler{db: db, logger: logger}
}

var severityOrder = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}

func severityRank(s string) int {
	if r, ok := severityOrder[strings.ToLower(strings.TrimSpace(s))]; ok {
		return r
	}
	return 5
}

// GenerateReport renders all findings into a report. ?format=md|html (default md).
// ?conversation_id= optionally scopes to one run.
func (h *ReportHandler) GenerateReport(c *gin.Context) {
	format := strings.ToLower(c.DefaultQuery("format", "md"))
	conversationID := c.Query("conversation_id")

	findings, err := h.db.ListVulnerabilities(100000, 0, "", conversationID, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sort.SliceStable(findings, func(i, j int) bool {
		return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
	})

	// Engagement context (if active).
	eng, _ := h.db.GetActiveEngagement()
	engName := "Pyntra Assessment"
	if eng != nil {
		engName = eng.Name
	}

	counts := map[string]int{}
	for _, f := range findings {
		counts[strings.ToLower(f.Severity)]++
	}

	switch format {
	case "html":
		out := renderReportHTML(engName, eng, findings, counts)
		c.Header("Content-Disposition", "inline; filename=pyntra-report.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(out))
	default:
		out := renderReportMarkdown(engName, eng, findings, counts)
		c.Header("Content-Disposition", "attachment; filename=pyntra-report.md")
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(out))
	}
}

func sevLine(counts map[string]int) string {
	return fmt.Sprintf("Critical: %d · High: %d · Medium: %d · Low: %d · Info: %d",
		counts["critical"], counts["high"], counts["medium"], counts["low"], counts["info"])
}

func renderReportMarkdown(title string, eng *database.Engagement, findings []*database.Vulnerability, counts map[string]int) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Penetration Test Report — %s\n\n", title)
	fmt.Fprintf(&b, "_Generated %s_\n\n", time.Now().Format("2006-01-02 15:04 MST"))
	if eng != nil {
		if eng.Client != "" {
			fmt.Fprintf(&b, "**Client:** %s  \n", eng.Client)
		}
		if len(eng.Scope.Domains) > 0 || len(eng.Scope.CIDRs) > 0 || len(eng.Scope.URLs) > 0 {
			fmt.Fprintf(&b, "**Scope:** %s\n", strings.Join(append(append(append([]string{}, eng.Scope.Domains...), eng.Scope.CIDRs...), eng.Scope.URLs...), ", "))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Executive summary\n\n")
	fmt.Fprintf(&b, "A total of **%d finding(s)** were identified.\n\n%s\n\n", len(findings), sevLine(counts))

	b.WriteString("## Findings\n\n")
	if len(findings) == 0 {
		b.WriteString("_No findings recorded._\n")
	}
	for i, f := range findings {
		fmt.Fprintf(&b, "### %d. [%s] %s\n\n", i+1, strings.ToUpper(f.Severity), f.Title)
		if f.Target != "" {
			fmt.Fprintf(&b, "- **Target:** `%s`\n", f.Target)
		}
		if f.Type != "" {
			fmt.Fprintf(&b, "- **Type:** %s\n", f.Type)
		}
		fmt.Fprintf(&b, "- **Status:** %s\n\n", f.Status)
		if f.Description != "" {
			fmt.Fprintf(&b, "**Description**\n\n%s\n\n", f.Description)
		}
		if f.Proof != "" {
			fmt.Fprintf(&b, "**Evidence**\n\n```\n%s\n```\n\n", f.Proof)
		}
		if f.Impact != "" {
			fmt.Fprintf(&b, "**Impact**\n\n%s\n\n", f.Impact)
		}
		if f.Recommendation != "" {
			fmt.Fprintf(&b, "**Remediation**\n\n%s\n\n", f.Recommendation)
		}
		b.WriteString("---\n\n")
	}
	return b.String()
}

func renderReportHTML(title string, eng *database.Engagement, findings []*database.Vulnerability, counts map[string]int) string {
	e := html.EscapeString
	var b bytes.Buffer
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>")
	b.WriteString(e(title))
	b.WriteString("</title><style>")
	b.WriteString(`body{font-family:system-ui,Segoe UI,Roboto,sans-serif;max-width:900px;margin:2rem auto;padding:0 1.5rem;color:#141416;line-height:1.55}
h1{border-bottom:3px solid #dc2626;padding-bottom:.4rem}
.sev{display:inline-block;padding:.1rem .5rem;border-radius:4px;color:#fff;font-size:.78rem;font-weight:600}
.sev-critical{background:#b4232a}.sev-high{background:#c2410c}.sev-medium{background:#b45309}.sev-low{background:#1d4ed8}.sev-info{background:#6a7688}
.finding{border:1px solid #e4e8ee;border-radius:8px;padding:1rem 1.25rem;margin:1rem 0}
pre{background:#f6f7f9;border:1px solid #e4e8ee;border-radius:6px;padding:.75rem;overflow:auto}
.meta{color:#4d5867;font-size:.9rem}`)
	b.WriteString("</style></head><body>")
	fmt.Fprintf(&b, "<h1>Penetration Test Report — %s</h1>", e(title))
	fmt.Fprintf(&b, "<p class=\"meta\">Generated %s</p>", e(time.Now().Format("2006-01-02 15:04 MST")))
	if eng != nil && eng.Client != "" {
		fmt.Fprintf(&b, "<p class=\"meta\"><strong>Client:</strong> %s</p>", e(eng.Client))
	}
	b.WriteString("<h2>Executive summary</h2>")
	fmt.Fprintf(&b, "<p>A total of <strong>%d finding(s)</strong> were identified.</p><p class=\"meta\">%s</p>", len(findings), e(sevLine(counts)))
	b.WriteString("<h2>Findings</h2>")
	if len(findings) == 0 {
		b.WriteString("<p><em>No findings recorded.</em></p>")
	}
	for i, f := range findings {
		sev := strings.ToLower(f.Severity)
		b.WriteString("<div class=\"finding\">")
		fmt.Fprintf(&b, "<h3>%d. <span class=\"sev sev-%s\">%s</span> %s</h3>", i+1, e(sev), e(strings.ToUpper(f.Severity)), e(f.Title))
		b.WriteString("<p class=\"meta\">")
		if f.Target != "" {
			fmt.Fprintf(&b, "<strong>Target:</strong> <code>%s</code> &nbsp; ", e(f.Target))
		}
		if f.Type != "" {
			fmt.Fprintf(&b, "<strong>Type:</strong> %s &nbsp; ", e(f.Type))
		}
		fmt.Fprintf(&b, "<strong>Status:</strong> %s</p>", e(f.Status))
		if f.Description != "" {
			fmt.Fprintf(&b, "<p>%s</p>", e(f.Description))
		}
		if f.Proof != "" {
			fmt.Fprintf(&b, "<p><strong>Evidence</strong></p><pre>%s</pre>", e(f.Proof))
		}
		if f.Impact != "" {
			fmt.Fprintf(&b, "<p><strong>Impact:</strong> %s</p>", e(f.Impact))
		}
		if f.Recommendation != "" {
			fmt.Fprintf(&b, "<p><strong>Remediation:</strong> %s</p>", e(f.Recommendation))
		}
		b.WriteString("</div>")
	}
	b.WriteString("</body></html>")
	return b.String()
}
