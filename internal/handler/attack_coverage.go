package handler

import (
	"net/http"
	"sort"
	"strings"

	"pyntra/internal/database"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AttackCoverageHandler maps attack-chain activity onto MITRE ATT&CK tactics and
// techniques, producing a coverage view per engagement/conversation. The mapping
// is a compact keyword heuristic over node labels/types and tool names — a
// pragmatic first pass, not an authoritative classifier.
type AttackCoverageHandler struct {
	db     *database.DB
	logger *zap.Logger
}

// NewAttackCoverageHandler constructs the handler.
func NewAttackCoverageHandler(db *database.DB, logger *zap.Logger) *AttackCoverageHandler {
	return &AttackCoverageHandler{db: db, logger: logger}
}

type attackTechnique struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tactic   string `json:"tactic"`
	keywords []string
}

// attackCatalogue is a compact ATT&CK subset covering common pentest activity.
var attackCatalogue = []attackTechnique{
	{"T1595", "Active Scanning", "Reconnaissance", []string{"nmap", "masscan", "scan", "recon", "amass", "subfinder", "shodan", "censys", "fierce", "dnsenum", "autorecon"}},
	{"T1592", "Gather Victim Host Info", "Reconnaissance", []string{"enum4linux", "whatweb", "fingerprint", "banner"}},
	{"T1190", "Exploit Public-Facing Application", "Initial Access", []string{"sqlmap", "sqli", "xss", "ssrf", "xxe", "rce", "exploit", "dalfox", "nuclei", "jaeles", "idor", "lfi", "upload"}},
	{"T1110", "Brute Force", "Credential Access", []string{"hydra", "john", "hashcat", "brute", "crack", "fcrackzip", "password"}},
	{"T1083", "File and Directory Discovery", "Discovery", []string{"ffuf", "dirb", "dirsearch", "gobuster", "feroxbuster", "katana", "hakrawler", "gau", "fuzz", "directory"}},
	{"T1046", "Network Service Discovery", "Discovery", []string{"service", "port", "arp-scan", "version"}},
	{"T1059", "Command and Scripting Interpreter", "Execution", []string{"exec", "shell", "command", "python", "bash", "webshell", "reverse"}},
	{"T1203", "Exploitation for Client Execution", "Execution", []string{"metasploit", "payload", "msfvenom"}},
	{"T1552", "Unsecured Credentials", "Credential Access", []string{"secret", "token", "api_key", "credential", "leak"}},
	{"T1078", "Valid Accounts", "Persistence", []string{"login", "auth", "session", "cookie"}},
	{"T1505", "Server Software Component", "Persistence", []string{"webshell", "backdoor", "implant"}},
	{"T1068", "Exploitation for Privilege Escalation", "Privilege Escalation", []string{"privesc", "escalation", "suid", "sudo", "kernel"}},
	{"T1005", "Data from Local System", "Collection", []string{"exfil", "dump", "loot", "collect", "download"}},
}

// GetCoverage returns ATT&CK coverage for a conversation's attack chain.
func (h *AttackCoverageHandler) GetCoverage(c *gin.Context) {
	conversationID := c.Param("conversationId")
	nodes, err := h.db.LoadAttackChainNodes(conversationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hits := map[string]int{} // technique id -> node matches
	for _, n := range nodes {
		hay := strings.ToLower(n.Label + " " + n.Type)
		for _, meta := range n.Metadata {
			if s, ok := meta.(string); ok {
				hay += " " + strings.ToLower(s)
			}
		}
		for _, tech := range attackCatalogue {
			for _, kw := range tech.keywords {
				if strings.Contains(hay, kw) {
					hits[tech.ID]++
					break
				}
			}
		}
	}

	// Group by tactic.
	type techOut struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Covered bool   `json:"covered"`
		Count   int    `json:"count"`
	}
	byTactic := map[string][]techOut{}
	coveredCount := 0
	for _, tech := range attackCatalogue {
		cnt := hits[tech.ID]
		if cnt > 0 {
			coveredCount++
		}
		byTactic[tech.Tactic] = append(byTactic[tech.Tactic], techOut{tech.ID, tech.Name, cnt > 0, cnt})
	}
	tactics := make([]string, 0, len(byTactic))
	for k := range byTactic {
		tactics = append(tactics, k)
	}
	sort.Strings(tactics)

	matrix := make([]gin.H, 0, len(tactics))
	for _, tac := range tactics {
		matrix = append(matrix, gin.H{"tactic": tac, "techniques": byTactic[tac]})
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation_id":    conversationID,
		"total_techniques":   len(attackCatalogue),
		"covered_techniques": coveredCount,
		"matrix":             matrix,
	})
}
