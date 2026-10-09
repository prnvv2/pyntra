package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"pyntra/internal/config"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)
type Retriever struct {
	db *sql.DB
	embedder *Embedder
	config *RetrievalConfig
	logger *zap.Logger

	rerankMu sync.RWMutex
	reranker DocumentReranker
}
type RetrievalConfig struct {
	TopK int
	SimilarityThreshold float64
	SubIndexFilter string
	PostRetrieve config.PostRetrieveConfig
	Mode string // "dense" (default) | "lexical" | "hybrid"
	RRFK int    // Reciprocal Rank Fusion constant (default 60)
}
func NewRetriever(db *sql.DB, embedder *Embedder, config *RetrievalConfig, logger *zap.Logger) *Retriever {
	return &Retriever{
		db: db,
		embedder: embedder,
		config: config,
		logger: logger,
	}
}
func (r *Retriever) UpdateConfig(cfg *RetrievalConfig) {
	if cfg != nil {
		r.config = cfg
		if r.logger != nil {
			r.logger.Info("retrievalconfigupdate",
				zap.Int("top_k", cfg.TopK),
				zap.Float64("similarity_threshold", cfg.SimilarityThreshold),
				zap.String("sub_index_filter", cfg.SubIndexFilter),
				zap.Int("post_retrieve_prefetch_top_k", cfg.PostRetrieve.PrefetchTopK),
				zap.Int("post_retrieve_max_context_chars", cfg.PostRetrieve.MaxContextChars),
				zap.Int("post_retrieve_max_context_tokens", cfg.PostRetrieve.MaxContextTokens),
			)
		}
	}
}
func (r *Retriever) SetDocumentReranker(rr DocumentReranker) {
	if r == nil {
		return
	}
	r.rerankMu.Lock()
	defer r.rerankMu.Unlock()
	r.reranker = rr
}

func (r *Retriever) documentReranker() DocumentReranker {
	if r == nil {
		return nil
	}
	r.rerankMu.RLock()
	defer r.rerankMu.RUnlock()
	return r.reranker
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
func (r *Retriever) Search(ctx context.Context, req *SearchRequest) ([]*RetrievalResult, error) {
	if req == nil {
		return nil, fmt.Errorf("cannot be empty")
	}
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	// Hybrid / lexical modes (default "" / "dense" keep the eino vector path).
	if r.config != nil {
		switch strings.ToLower(strings.TrimSpace(r.config.Mode)) {
		case "hybrid", "lexical":
			return r.hybridSearch(ctx, req, strings.ToLower(strings.TrimSpace(r.config.Mode)))
		}
	}
	opts := r.einoRetrieverOptions(req)
	docs, err := NewVectorEinoRetriever(r).Retrieve(ctx, q, opts...)
	if err != nil {
		return nil, err
	}
	return documentsToRetrievalResults(docs)
}

func (r *Retriever) einoRetrieverOptions(req *SearchRequest) []retriever.Option {
	var opts []retriever.Option
	if req.TopK > 0 {
		opts = append(opts, retriever.WithTopK(req.TopK))
	}
	dsl := map[string]any{}
	if strings.TrimSpace(req.RiskType) != "" {
		dsl[DSLRiskType] = strings.TrimSpace(req.RiskType)
	}
	if req.Threshold > 0 {
		dsl[DSLSimilarityThreshold] = req.Threshold
	}
	if strings.TrimSpace(req.SubIndexFilter) != "" {
		dsl[DSLSubIndexFilter] = strings.TrimSpace(req.SubIndexFilter)
	}
	if len(dsl) > 0 {
		opts = append(opts, retriever.WithDSLInfo(dsl))
	}
	return opts
}
func (r *Retriever) EinoRetrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	return NewVectorEinoRetriever(r).Retrieve(ctx, query, opts...)
}

func (r *Retriever) knowledgeEmbeddingSelectSQL(riskType, subIndexFilter string) (string, []interface{}) {
	q := `SELECT e.id, e.item_id, e.chunk_index, e.chunk_text, e.embedding, e.embedding_model, e.embedding_dim, i.category, i.title
FROM knowledge_embeddings e
JOIN knowledge_base_items i ON e.item_id = i.id
WHERE 1=1`
	var args []interface{}
	if strings.TrimSpace(riskType) != "" {
		q += ` AND TRIM(i.category) = TRIM(?) COLLATE NOCASE`
		args = append(args, riskType)
	}
	if tag := strings.TrimSpace(subIndexFilter); tag != "" {
		tag = strings.ToLower(strings.ReplaceAll(tag, " ", ""))
		q += ` AND (TRIM(COALESCE(e.sub_indexes,'')) = '' OR INSTR(',' || LOWER(REPLACE(e.sub_indexes,' ','')) || ',', ',' || ? || ',') > 0)`
		args = append(args, tag)
	}
	return q, args
}
func (r *Retriever) vectorSearch(ctx context.Context, req *SearchRequest) ([]*RetrievalResult, error) {
	if req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	topK := req.TopK
	if topK <= 0 && r.config != nil {
		topK = r.config.TopK
	}
	if topK <= 0 {
		topK = 5
	}

	threshold := req.Threshold
	if threshold <= 0 && r.config != nil {
		threshold = r.config.SimilarityThreshold
	}
	if threshold <= 0 {
		threshold = 0.7
	}

	subIdxFilter := strings.TrimSpace(req.SubIndexFilter)
	if subIdxFilter == "" && r.config != nil {
		subIdxFilter = strings.TrimSpace(r.config.SubIndexFilter)
	}

	queryText := FormatQueryEmbeddingText(req.RiskType, req.Query)
	queryEmbedding, err := r.embedder.EmbedText(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("queryfailed: %w", err)
	}
	queryDim := len(queryEmbedding)
	expectedModel := ""
	if r.embedder != nil {
		expectedModel = r.embedder.EmbeddingModelName()
	}

	sqlStr, sqlArgs := r.knowledgeEmbeddingSelectSQL(strings.TrimSpace(req.RiskType), subIdxFilter)
	rows, err := r.db.QueryContext(ctx, sqlStr, sqlArgs...)
	if err != nil {
		return nil, fmt.Errorf("queryfailed: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		chunk *KnowledgeChunk
		item *KnowledgeItem
		similarity float64
	}

	candidates := make([]candidate, 0)
	rowNum := 0
	for rows.Next() {
		rowNum++
		if rowNum%48 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		var chunkID, itemID, chunkText, embeddingJSON, category, title, rowModel string
		var chunkIndex, rowDim int

		if err := rows.Scan(&chunkID, &itemID, &chunkIndex, &chunkText, &embeddingJSON, &rowModel, &rowDim, &category, &title); err != nil {
			r.logger.Warn("scanfailed", zap.Error(err))
			continue
		}

		var embedding []float32
		if err := json.Unmarshal([]byte(embeddingJSON), &embedding); err != nil {
			r.logger.Warn("parsefailed", zap.Error(err))
			continue
		}

		if rowDim > 0 && len(embedding) != rowDim {
			r.logger.Debug("", zap.String("chunkId", chunkID), zap.Int("rowDim", rowDim), zap.Int("got", len(embedding)))
			continue
		}
		if queryDim > 0 && len(embedding) != queryDim {
			r.logger.Debug("query", zap.String("chunkId", chunkID), zap.Int("queryDim", queryDim), zap.Int("got", len(embedding)))
			continue
		}
		if expectedModel != "" && strings.TrimSpace(rowModel) != "" && strings.TrimSpace(rowModel) != expectedModel {
			r.logger.Debug("model", zap.String("chunkId", chunkID), zap.String("rowModel", rowModel), zap.String("expected", expectedModel))
			continue
		}

		similarity := cosineSimilarity(queryEmbedding, embedding)
		candidates = append(candidates, candidate{
			chunk: &KnowledgeChunk{
				ID: chunkID,
				ItemID: itemID,
				ChunkIndex: chunkIndex,
				ChunkText: chunkText,
				Embedding: embedding,
			},
			item: &KnowledgeItem{
				ID: itemID,
				Category: category,
				Title: title,
			},
			similarity: similarity,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})

	filtered := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.similarity >= threshold {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) > topK {
		filtered = filtered[:topK]
	}

	results := make([]*RetrievalResult, len(filtered))
	for i, c := range filtered {
		results[i] = &RetrievalResult{
			Chunk: c.chunk,
			Item: c.item,
			Similarity: c.similarity,
			Score: c.similarity,
		}
	}
	return results, nil
}
func (r *Retriever) AsEinoRetriever() retriever.Retriever {
	return NewVectorEinoRetriever(r)
}
