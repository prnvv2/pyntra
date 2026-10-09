package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// APIReranker reranks retrieved documents with an OpenAI-compatible /rerank
// cross-encoder endpoint (Jina, Cohere, or a local bge-reranker). It implements
// [DocumentReranker]. On any error it returns the input order unchanged so a
// reranker outage never breaks retrieval.
type APIReranker struct {
	baseURL string
	apiKey  string
	model   string
	topN    int
	client  *http.Client
	logger  *zap.Logger
}

// NewAPIReranker builds a reranker. timeoutSeconds<=0 defaults to 20s.
func NewAPIReranker(baseURL, apiKey, model string, topN, timeoutSeconds int, logger *zap.Logger) *APIReranker {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &APIReranker{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		model:   strings.TrimSpace(model),
		topN:    topN,
		client:  &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		logger:  logger,
	}
}

type rerankRequest struct {
	Model     string   `json:"model,omitempty"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank implements [DocumentReranker].
func (r *APIReranker) Rerank(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error) {
	if r == nil || r.baseURL == "" || len(docs) <= 1 {
		return docs, nil
	}
	texts := make([]string, len(docs))
	for i, d := range docs {
		if d != nil {
			texts[i] = d.Content
		}
	}
	payload, _ := json.Marshal(rerankRequest{Model: r.model, Query: query, Documents: texts, TopN: r.topN})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/rerank", bytes.NewReader(payload))
	if err != nil {
		return docs, nil
	}
	req.Header.Set("Content-Type", "application/json")
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Debug("rerank request failed", zap.Error(err))
		return docs, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.logger.Debug("rerank non-2xx", zap.Int("status", resp.StatusCode))
		return docs, nil
	}
	var parsed rerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Results) == 0 {
		return docs, nil
	}
	// Reorder by relevance (desc), guarding indices.
	results := parsed.Results
	sort.SliceStable(results, func(i, j int) bool { return results[i].RelevanceScore > results[j].RelevanceScore })
	out := make([]*schema.Document, 0, len(docs))
	seen := map[int]bool{}
	for _, res := range results {
		if res.Index < 0 || res.Index >= len(docs) || seen[res.Index] {
			continue
		}
		seen[res.Index] = true
		out = append(out, docs[res.Index])
	}
	// Append any docs the reranker omitted, preserving original order.
	for i, d := range docs {
		if !seen[i] {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return docs, nil
	}
	return out, nil
}

var _ DocumentReranker = (*APIReranker)(nil)

// rerankSummary is a tiny helper for logging configuration at startup.
func (r *APIReranker) rerankSummary() string {
	return fmt.Sprintf("base=%s model=%s top_n=%d", r.baseURL, r.model, r.topN)
}
