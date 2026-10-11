package fanout

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"pyntra/internal/agent"
	"pyntra/internal/config"
	"pyntra/internal/database"
	"pyntra/internal/enrich"
	"pyntra/internal/security"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Result mirrors the fields the callers expect from an agent run.
type Result struct {
	Response        string
	MCPExecutionIDs []string
}

// Deps are the collaborators the orchestrator needs. All are existing singletons
// wired by the app; fanout does not own any of them.
type Deps struct {
	AppCfg   *config.Config
	Fanout   config.FanoutConfig
	Agent    *agent.Agent
	Executor *security.Executor // for installing the per-host rate limiter
	Enricher *enrich.Enricher   // fan-in enrichment
	DB       *database.DB
	Logger   *zap.Logger
}

// ProgressFunc matches the progress callback used throughout the codebase.
type ProgressFunc func(eventType, message string, data interface{})

// Orchestrator runs the fan-out recon -> bounded-parallel depth -> fan-in flow.
type Orchestrator struct {
	deps Deps
	log  *zap.Logger
}

// NewOrchestrator constructs an Orchestrator.
func NewOrchestrator(deps Deps) *Orchestrator {
	lg := deps.Logger
	if lg == nil {
		lg = zap.NewNop()
	}
	return &Orchestrator{deps: deps, log: lg}
}

// maxAssetsHardCap bounds how many depth agents can ever be spawned in one run,
// independent of the inventory size, as a safety valve.
const maxAssetsHardCap = 50

// Run executes one fanout round. The shared ctx is the kill-switch: cancelling
// it (or engaging the executor's global halt) stops recon and every depth worker.
func (o *Orchestrator) Run(
	ctx context.Context,
	conversationID string,
	userMessage string,
	history []agent.ChatMessage,
	reconRoleTools []string,
	progress ProgressFunc,
) (*Result, error) {
	if o.deps.Agent == nil {
		return nil, fmt.Errorf("fanout: agent is nil")
	}

	// Serialize progress across concurrent workers.
	var progMu sync.Mutex
	emit := func(eventType, message string, data interface{}) {
		if progress == nil {
			return
		}
		progMu.Lock()
		progress(eventType, message, data)
		progMu.Unlock()
	}

	// Install a per-host rate limiter for the duration of the run, then restore.
	if o.deps.Executor != nil {
		rl := security.NewHostRateLimiter(o.deps.Fanout.PerHostRateLimitRPSEffective())
		o.deps.Executor.SetRateLimiter(rl)
		defer o.deps.Executor.SetRateLimiter(nil)
	}

	var execIDs []string
	var execMu sync.Mutex
	addExecIDs := func(ids []string) {
		if len(ids) == 0 {
			return
		}
		execMu.Lock()
		execIDs = append(execIDs, ids...)
		execMu.Unlock()
	}

	// --- Breadth: recon pass ---
	emit("progress", "Fanout: running breadth/recon pass...", map[string]interface{}{
		"conversationId": conversationID, "fanoutPhase": "recon",
	})

	reconObjective := buildReconObjective(userMessage)
	reconRes, err := o.deps.Agent.AgentLoopWithProgress(
		ctx, reconObjective, history, conversationID,
		taggedProgress(emit, "recon", ""), reconRoleTools, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("fanout recon phase failed: %w", err)
	}
	if reconRes != nil {
		addExecIDs(reconRes.MCPExecutionIDs)
	}

	// --- Build the asset inventory ---
	inv := AssetInventory{}
	if reconRes != nil {
		inv = ParseInventory(reconRes.Response)
		if len(inv.Assets) == 0 {
			inv = FallbackInventory(reconRes.Response)
		}
	}
	// As a last resort, seed from targets in the original objective.
	if len(inv.Assets) == 0 {
		inv = FallbackInventory(userMessage)
	}
	if len(inv.Assets) > maxAssetsHardCap {
		inv.Assets = inv.Assets[:maxAssetsHardCap]
	}

	emit("progress", fmt.Sprintf("Fanout: %d asset(s) discovered; dispatching depth agents.", len(inv.Assets)), map[string]interface{}{
		"conversationId": conversationID,
		"fanoutPhase":    "inventory",
		"assetCount":     len(inv.Assets),
		"assets":         assetSummaries(inv.Assets),
	})

	if len(inv.Assets) == 0 {
		// Nothing to drill into; return the recon summary.
		resp := "Fanout completed: recon found no discrete assets to assess in depth.\n\n"
		if reconRes != nil {
			resp += reconRes.Response
		}
		return &Result{Response: resp, MCPExecutionIDs: execIDs}, nil
	}

	// --- Depth: bounded-parallel per-asset agents ---
	maxParallel := int64(o.deps.Fanout.MaxParallelAssetsEffective())
	sem := semaphore.NewWeighted(maxParallel)
	g, gctx := errgroup.WithContext(ctx)

	assetResults := make([]string, len(inv.Assets))
	for i := range inv.Assets {
		i := i
		asset := inv.Assets[i]
		if err := sem.Acquire(gctx, 1); err != nil {
			break // ctx cancelled (kill-switch); stop dispatching
		}
		g.Go(func() error {
			defer sem.Release(1)

			role := o.deps.Fanout.DepthRoleFor(string(asset.Type))
			depthTools, depthSkills := o.resolveRole(role)
			objective := buildDepthObjective(userMessage, asset)

			emit("progress", fmt.Sprintf("Depth agent started: %s (%s, role %s)", asset.Value, asset.Type, role), map[string]interface{}{
				"conversationId": conversationID,
				"fanoutPhase":    "depth",
				"asset":          asset.Value,
				"assetType":      string(asset.Type),
				"role":           role,
			})

			res, derr := o.deps.Agent.AgentLoopWithProgress(
				gctx, objective, nil, conversationID,
				taggedProgress(emit, "depth", asset.Value), depthTools, depthSkills,
			)
			if derr != nil {
				o.log.Warn("fanout depth agent failed",
					zap.String("asset", asset.Value), zap.Error(derr))
				assetResults[i] = fmt.Sprintf("### %s (%s)\n_Depth assessment error: %v_", asset.Value, asset.Type, derr)
				// A single asset failing should not abort the whole pool; swallow.
				return nil
			}
			if res != nil {
				addExecIDs(res.MCPExecutionIDs)
				assetResults[i] = fmt.Sprintf("### %s (%s · role %s)\n%s", asset.Value, asset.Type, role, res.Response)
			}
			return nil
		})
	}

	// errgroup only returns ctx errors here (worker funcs swallow their own
	// errors), so a non-nil err means the run was cancelled.
	if werr := g.Wait(); werr != nil {
		o.log.Info("fanout depth pool stopped", zap.Error(werr))
	}

	// --- Fan-in: enrich + rank + summarize ---
	emit("progress", "Fanout: fan-in — enriching and correlating findings...", map[string]interface{}{
		"conversationId": conversationID, "fanoutPhase": "fanin",
	})
	summary := o.fanIn(ctx, conversationID, inv, assetResults)

	return &Result{Response: summary, MCPExecutionIDs: execIDs}, nil
}

// resolveRole returns the tool list and skills for a role name from config.
func (o *Orchestrator) resolveRole(role string) ([]string, []string) {
	if o.deps.AppCfg == nil || o.deps.AppCfg.Roles == nil {
		return nil, nil
	}
	if r, ok := o.deps.AppCfg.Roles[role]; ok && r.Enabled {
		return r.Tools, r.Skills
	}
	return nil, nil
}

// fanIn re-enriches the conversation's findings (so CVE/CWE/CVSS is current) and
// composes the ranked summary report shown to the user.
func (o *Orchestrator) fanIn(ctx context.Context, conversationID string, inv AssetInventory, assetResults []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Fanout assessment — %d asset(s)\n\n", len(inv.Assets)))
	b.WriteString(fmt.Sprintf("_Completed %s_\n\n", time.Now().Format("2006-01-02 15:04 MST")))

	// Enrich + rank findings for this conversation.
	if o.deps.DB != nil {
		findings, err := o.deps.DB.ListVulnerabilities(100000, 0, "", conversationID, "", "")
		if err == nil && len(findings) > 0 {
			writer := database.NewSerialWriter(o.deps.DB, 64)
			defer writer.Close()
			enriched := 0
			for _, f := range findings {
				if o.deps.Enricher != nil {
					if eerr := o.deps.Enricher.Enrich(ctx, f); eerr == nil {
						if werr := writer.UpdateVulnerability(ctx, f.ID, f); werr == nil {
							enriched++
						}
					}
				}
			}
			ranked, _ := o.deps.DB.ListVulnerabilitiesRanked(100000, 0, "", conversationID, "", "")
			b.WriteString(fmt.Sprintf("## Prioritized findings (%d)\n\n", len(ranked)))
			if len(ranked) == 0 {
				b.WriteString("_No findings recorded by the depth agents._\n\n")
			}
			for i, f := range ranked {
				score := ""
				if f.CVSSBaseScore > 0 {
					score = fmt.Sprintf(" — CVSS %.1f (%s)", f.CVSSBaseScore, f.CVSSSeverity)
				}
				kev := ""
				if f.KEV {
					kev = " ⚠KEV"
				}
				b.WriteString(fmt.Sprintf("%d. **[%s]** %s — `%s`%s%s\n",
					i+1, strings.ToUpper(f.Severity), f.Title, f.Target, score, kev))
			}
			b.WriteString("\n")

			if corr := correlate(ranked); corr != "" {
				b.WriteString(corr)
			}
		}
	}

	b.WriteString("## Per-asset depth results\n\n")
	for _, r := range assetResults {
		if strings.TrimSpace(r) == "" {
			continue
		}
		b.WriteString(r)
		b.WriteString("\n\n")
	}
	return b.String()
}

// --- objective builders ---

func buildReconObjective(userMessage string) string {
	return "FANOUT BREADTH/RECON PHASE.\n" +
		"Enumerate the attack surface for the objective below. Do NOT perform deep exploitation now — " +
		"identify discrete assets (hosts, web apps, APIs, cloud resources) that each deserve a focused follow-up.\n\n" +
		"Output a STRUCTURED asset inventory as a single JSON object inside a ```json code block, shaped exactly:\n" +
		"```json\n{\"assets\":[{\"type\":\"web|host|cloud|api\",\"value\":\"<url-or-host-or-arn>\",\"ports\":[80,443],\"services\":[\"http\"]}]}\n```\n" +
		"Keep every asset strictly within the authorized engagement scope.\n\n" +
		"OBJECTIVE:\n" + userMessage
}

func buildDepthObjective(userMessage string, a Asset) string {
	var meta strings.Builder
	if len(a.Ports) > 0 {
		meta.WriteString(fmt.Sprintf(" Ports: %v.", a.Ports))
	}
	if len(a.Services) > 0 {
		meta.WriteString(fmt.Sprintf(" Services: %s.", strings.Join(a.Services, ", ")))
	}
	return fmt.Sprintf(
		"FANOUT DEPTH PHASE — single asset.\n"+
			"Focus EXCLUSIVELY on this one asset; do not pivot to other hosts or expand scope.\n"+
			"Asset: %s (type: %s).%s\n\n"+
			"Thoroughly assess it within the authorized engagement scope. For every confirmed issue, call the "+
			"record_vulnerability tool. Where possible include a cve_ids value or a cvss_vector "+
			"(e.g. CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H) so the finding is scored deterministically.\n\n"+
			"OVERALL OBJECTIVE (context): %s",
		a.Value, a.Type, meta.String(), userMessage)
}

// taggedProgress wraps the progress callback to annotate every event with the
// fanout phase and asset, so the UI can group sub-streams.
func taggedProgress(emit ProgressFunc, phase, asset string) agent.ProgressCallback {
	return func(eventType, message string, data interface{}) {
		m, ok := data.(map[string]interface{})
		if !ok || m == nil {
			m = map[string]interface{}{}
		}
		m["fanoutPhase"] = phase
		if asset != "" {
			m["asset"] = asset
		}
		m["source"] = "fanout"
		emit(eventType, message, m)
	}
}

func assetSummaries(assets []Asset) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(assets))
	for _, a := range assets {
		out = append(out, map[string]interface{}{
			"id":    a.ID,
			"type":  string(a.Type),
			"value": a.Value,
		})
	}
	return out
}
