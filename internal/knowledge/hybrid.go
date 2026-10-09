package knowledge

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

var ftsTokenRE = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// buildFTSMatch turns a free-text query into a safe FTS5 MATCH expression:
// alphanumeric tokens OR-ed together. Returns "" when nothing usable remains.
func buildFTSMatch(q string) string {
	toks := ftsTokenRE.FindAllString(strings.ToLower(q), -1)
	quoted := make([]string, 0, len(toks))
	for _, t := range toks {
		if len(t) < 2 {
			continue
		}
		quoted = append(quoted, `"`+t+`"`)
	}
	return strings.Join(quoted, " OR ")
}

// lexicalSearch runs a BM25 FTS5 query over chunk text. Honors the category
// (riskType) filter. Returns results ordered best-first. If FTS5 is unavailable
// or the query is empty, it returns nil without error.
func (r *Retriever) lexicalSearch(ctx context.Context, req *SearchRequest, limit int) ([]*RetrievalResult, error) {
	match := buildFTSMatch(req.Query)
	if match == "" || r.db == nil {
		return nil, nil
	}
	q := `SELECT f.chunk_id, f.item_id, i.category, i.title, f.chunk_text, bm25(knowledge_fts) AS score
	      FROM knowledge_fts f JOIN knowledge_base_items i ON f.item_id = i.id
	      WHERE knowledge_fts MATCH ?`
	args := []interface{}{match}
	if rt := strings.TrimSpace(req.RiskType); rt != "" {
		q += ` AND TRIM(i.category) = TRIM(?) COLLATE NOCASE`
		args = append(args, rt)
	}
	q += ` ORDER BY score LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil // FTS5 absent or query error -> treat as no lexical hits
	}
	defer rows.Close()

	var out []*RetrievalResult
	for rows.Next() {
		var chunkID, itemID, category, title, chunkText string
		var score float64
		if err := rows.Scan(&chunkID, &itemID, &category, &title, &chunkText, &score); err != nil {
			continue
		}
		out = append(out, &RetrievalResult{
			Chunk: &KnowledgeChunk{ID: chunkID, ItemID: itemID, ChunkText: chunkText},
			Item:  &KnowledgeItem{ID: itemID, Category: category, Title: title},
			// bm25 is lower-is-better; expose a positive relevance proxy.
			Score: -score,
		})
	}
	return out, nil
}

// reciprocalRankFusion fuses ranked result lists by RRF: each list contributes
// 1/(k + rank) per document, summed across lists, keyed by chunk id.
func reciprocalRankFusion(lists [][]*RetrievalResult, k, topK int) []*RetrievalResult {
	if k <= 0 {
		k = 60
	}
	type agg struct {
		res   *RetrievalResult
		score float64
	}
	fused := map[string]*agg{}
	for _, list := range lists {
		for rank, res := range list {
			if res == nil || res.Chunk == nil {
				continue
			}
			id := res.Chunk.ID
			a := fused[id]
			if a == nil {
				a = &agg{res: res}
				fused[id] = a
			} else if a.res.Chunk.ChunkText == "" && res.Chunk.ChunkText != "" {
				a.res = res // prefer the richer record
			}
			a.score += 1.0 / float64(k+rank+1)
		}
	}
	out := make([]*RetrievalResult, 0, len(fused))
	for _, a := range fused {
		a.res.Score = a.score
		out = append(out, a.res)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out
}

// hybridSearch implements the "lexical" and "hybrid" retrieval modes.
func (r *Retriever) hybridSearch(ctx context.Context, req *SearchRequest, mode string) ([]*RetrievalResult, error) {
	topK := req.TopK
	if topK <= 0 && r.config != nil {
		topK = r.config.TopK
	}
	if topK <= 0 {
		topK = 5
	}
	prefetch := topK * 4
	if prefetch < 20 {
		prefetch = 20
	}

	lex, _ := r.lexicalSearch(ctx, req, prefetch)

	if mode == "lexical" {
		if len(lex) > topK {
			lex = lex[:topK]
		}
		return lex, nil
	}

	// hybrid: dense + lexical, fused.
	denseReq := *req
	if denseReq.TopK < prefetch {
		denseReq.TopK = prefetch
	}
	dense, err := r.vectorSearch(ctx, &denseReq)
	if err != nil {
		// Dense failed (e.g. embedder down) — fall back to lexical alone.
		if len(lex) > topK {
			lex = lex[:topK]
		}
		return lex, nil
	}
	k := 60
	if r.config != nil && r.config.RRFK > 0 {
		k = r.config.RRFK
	}
	return reciprocalRankFusion([][]*RetrievalResult{dense, lex}, k, topK), nil
}
