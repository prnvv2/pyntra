package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"pyntra/internal/config"

	fileloader "github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)
type Indexer struct {
	db *sql.DB
	embedder *Embedder
	logger *zap.Logger
	chunkSize int
	overlap int
	indexingCfg *config.IndexingConfig

	indexChain compose.Runnable[[]*schema.Document, []string]
	fileLoader *fileloader.FileLoader

	mu sync.RWMutex
	lastError string
	lastErrorTime time.Time
	errorCount int

	rebuildMu sync.RWMutex
	isRebuilding bool
	rebuildTotalItems int
	rebuildCurrent int
	rebuildFailed int
	rebuildStartTime time.Time
	rebuildLastItemID string
	rebuildLastChunks int
}
func NewIndexer(ctx context.Context, db *sql.DB, embedder *Embedder, logger *zap.Logger, kcfg *config.KnowledgeConfig) (*Indexer, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedder is nil")
	}
	if err := EnsureKnowledgeEmbeddingsSchema(db); err != nil {
		return nil, fmt.Errorf("knowledge_embeddings : %w", err)
	}
	if kcfg == nil {
		kcfg = &config.KnowledgeConfig{}
	}
	indexingCfg := &kcfg.Indexing

	chunkSize := 512
	overlap := 50
	if indexingCfg.ChunkSize > 0 {
		chunkSize = indexingCfg.ChunkSize
	}
	if indexingCfg.ChunkOverlap >= 0 {
		overlap = indexingCfg.ChunkOverlap
	}

	embedModel := embedder.EmbeddingModelName()
	splitter, err := newKnowledgeSplitter(chunkSize, overlap, embedModel)
	if err != nil {
		return nil, fmt.Errorf("eino recursive splitter: %w", err)
	}

	chain, err := buildKnowledgeIndexChain(ctx, indexingCfg, db, splitter, embedModel)
	if err != nil {
		return nil, fmt.Errorf("knowledge index chain: %w", err)
	}

	var fl *fileloader.FileLoader
	fl, err = fileloader.NewFileLoader(ctx, nil)
	if err != nil {
		if logger != nil {
			logger.Warn("Eino FileLoader initializefailed,prefer_source_file database", zap.Error(err))
		}
		fl = nil
		err = nil
	}

	return &Indexer{
		db: db,
		embedder: embedder,
		logger: logger,
		chunkSize: chunkSize,
		overlap: overlap,
		indexingCfg: indexingCfg,
		indexChain: chain,
		fileLoader: fl,
	}, nil
}
func (idx *Indexer) RecompileIndexChain(ctx context.Context) error {
	if idx == nil || idx.db == nil || idx.embedder == nil {
		return fmt.Errorf("indexer initialize")
	}
	if err := EnsureKnowledgeEmbeddingsSchema(idx.db); err != nil {
		return err
	}
	embedModel := idx.embedder.EmbeddingModelName()
	splitter, err := newKnowledgeSplitter(idx.chunkSize, idx.overlap, embedModel)
	if err != nil {
		return fmt.Errorf("eino recursive splitter: %w", err)
	}
	chain, err := buildKnowledgeIndexChain(ctx, idx.indexingCfg, idx.db, splitter, embedModel)
	if err != nil {
		return fmt.Errorf("knowledge index chain: %w", err)
	}
	idx.indexChain = chain
	return nil
}
func (idx *Indexer) IndexItem(ctx context.Context, itemID string) error {
	if idx.indexChain == nil {
		return fmt.Errorf("initialize")
	}
	if idx.embedder == nil {
		return fmt.Errorf("initialize")
	}

	var content, category, title, filePath string
	err := idx.db.QueryRow("SELECT content, category, title, file_path FROM knowledge_base_items WHERE id = ?", itemID).Scan(&content, &category, &title, &filePath)
	if err != nil {
		return fmt.Errorf("Failed to fetch knowledge items:%w", err)
	}

	if _, err := idx.db.Exec("DELETE FROM knowledge_embeddings WHERE item_id = ?", itemID); err != nil {
		return fmt.Errorf("deletefailed:%w", err)
	}
	_, _ = idx.db.Exec("DELETE FROM knowledge_fts WHERE item_id = ?", itemID)

	body := strings.TrimSpace(content)
	if idx.indexingCfg != nil && idx.indexingCfg.PreferSourceFile && strings.TrimSpace(filePath) != "" && idx.fileLoader != nil {
		docs, lerr := idx.fileLoader.Load(ctx, document.Source{URI: strings.TrimSpace(filePath)})
		if lerr == nil && len(docs) > 0 {
			var b strings.Builder
			for i, d := range docs {
				if d == nil {
					continue
				}
				if i > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(d.Content)
			}
			if s := strings.TrimSpace(b.String()); s != "" {
				body = s
			}
		} else if idx.logger != nil {
			idx.logger.Warn("Filesfailed,database",
				zap.String("itemId", itemID),
				zap.String("path", filePath),
				zap.Error(lerr))
		}
	}

	root := &schema.Document{
		ID: itemID,
		Content: body,
		MetaData: map[string]any{
			metaKBCategory: category,
			metaKBTitle: title,
			metaKBItemID: itemID,
		},
	}

	idxOpts := []indexer.Option{indexer.WithEmbedding(idx.embedder.EinoEmbeddingComponent())}
	if idx.indexingCfg != nil && len(idx.indexingCfg.SubIndexes) > 0 {
		idxOpts = append(idxOpts, indexer.WithSubIndexes(idx.indexingCfg.SubIndexes))
	}

	ids, err := idx.indexChain.Invoke(ctx, []*schema.Document{root}, compose.WithIndexerOption(idxOpts...))
	if err != nil {
		msg := fmt.Sprintf("failed (Knowledge:%s): %v", itemID, err)
		idx.mu.Lock()
		idx.lastError = msg
		idx.lastErrorTime = time.Now()
		idx.mu.Unlock()
		return err
	}

	if idx.logger != nil {
		idx.logger.Info("Knowledgecompleted", zap.String("itemId", itemID), zap.Int("chunks", len(ids)))
	}
	idx.rebuildMu.Lock()
	idx.rebuildLastItemID = itemID
	idx.rebuildLastChunks = len(ids)
	idx.rebuildMu.Unlock()
	return nil
}
func (idx *Indexer) HasIndex() (bool, error) {
	var count int
	err := idx.db.QueryRow("SELECT COUNT(*) FROM knowledge_embeddings").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed:%w", err)
	}
	return count > 0, nil
}
func (idx *Indexer) RebuildIndex(ctx context.Context) error {
	idx.rebuildMu.Lock()
	idx.isRebuilding = true
	idx.rebuildTotalItems = 0
	idx.rebuildCurrent = 0
	idx.rebuildFailed = 0
	idx.rebuildStartTime = time.Now()
	idx.rebuildLastItemID = ""
	idx.rebuildLastChunks = 0
	idx.rebuildMu.Unlock()

	idx.mu.Lock()
	idx.lastError = ""
	idx.lastErrorTime = time.Time{}
	idx.errorCount = 0
	idx.mu.Unlock()

	rows, err := idx.db.Query("SELECT id FROM knowledge_base_items")
	if err != nil {
		idx.rebuildMu.Lock()
		idx.isRebuilding = false
		idx.rebuildMu.Unlock()
		return fmt.Errorf("failed to query knowledge items:%w", err)
	}
	defer rows.Close()

	var itemIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			idx.rebuildMu.Lock()
			idx.isRebuilding = false
			idx.rebuildMu.Unlock()
			return fmt.Errorf("scanKnowledge ID failed:%w", err)
		}
		itemIDs = append(itemIDs, id)
	}

	idx.rebuildMu.Lock()
	idx.rebuildTotalItems = len(itemIDs)
	idx.rebuildMu.Unlock()

	idx.logger.Info("start", zap.Int("totalItems", len(itemIDs)))

	failedCount := 0
	consecutiveFailures := 0
	maxConsecutiveFailures := 5
	firstFailureItemID := ""
	var firstFailureError error

	for i, itemID := range itemIDs {
		if err := idx.IndexItem(ctx, itemID); err != nil {
			failedCount++
			consecutiveFailures++

			if consecutiveFailures == 1 {
				firstFailureItemID = itemID
				firstFailureError = err
				idx.logger.Warn("Knowledgefailed",
					zap.String("itemId", itemID),
					zap.Int("totalItems", len(itemIDs)),
					zap.Error(err),
				)
			}

			if consecutiveFailures >= maxConsecutiveFailures {
				errorMsg := fmt.Sprintf(" %d Knowledgefailed,config(modelconfigerror/API invalid/).failed:%s, error:%v", consecutiveFailures, firstFailureItemID, firstFailureError)
				idx.mu.Lock()
				idx.lastError = errorMsg
				idx.lastErrorTime = time.Now()
				idx.mu.Unlock()

				idx.logger.Error("failed,",
					zap.Int("consecutiveFailures", consecutiveFailures),
					zap.Int("totalItems", len(itemIDs)),
					zap.Int("processedItems", i+1),
					zap.String("firstFailureItemId", firstFailureItemID),
					zap.Error(firstFailureError),
				)
				return fmt.Errorf("failed:%v", firstFailureError)
			}

			if failedCount > len(itemIDs)*3/10 && failedCount == len(itemIDs)*3/10+1 {
				errorMsg := fmt.Sprintf("failedKnowledge (%d/%d),config.failed:%s, error:%v", failedCount, len(itemIDs), firstFailureItemID, firstFailureError)
				idx.mu.Lock()
				idx.lastError = errorMsg
				idx.lastErrorTime = time.Now()
				idx.mu.Unlock()

				idx.logger.Error("failedKnowledge,config",
					zap.Int("failedCount", failedCount),
					zap.Int("totalItems", len(itemIDs)),
					zap.String("firstFailureItemId", firstFailureItemID),
					zap.Error(firstFailureError),
				)
			}
			continue
		}

		if consecutiveFailures > 0 {
			consecutiveFailures = 0
			firstFailureItemID = ""
			firstFailureError = nil
		}

		idx.rebuildMu.Lock()
		idx.rebuildCurrent = i + 1
		idx.rebuildFailed = failedCount
		idx.rebuildMu.Unlock()

		if (i+1)%10 == 0 || (len(itemIDs) > 0 && (i+1)*100/len(itemIDs)%10 == 0 && (i+1)*100/len(itemIDs) > 0) {
			idx.logger.Info("", zap.Int("current", i+1), zap.Int("total", len(itemIDs)), zap.Int("failed", failedCount))
		}
	}

	idx.rebuildMu.Lock()
	idx.isRebuilding = false
	idx.rebuildMu.Unlock()

	idx.logger.Info("completed", zap.Int("totalItems", len(itemIDs)), zap.Int("failedCount", failedCount))
	return nil
}
func (idx *Indexer) GetLastError() (string, time.Time) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.lastError, idx.lastErrorTime
}
func (idx *Indexer) GetRebuildStatus() (isRebuilding bool, totalItems int, current int, failed int, lastItemID string, lastChunks int, startTime time.Time) {
	idx.rebuildMu.RLock()
	defer idx.rebuildMu.RUnlock()
	return idx.isRebuilding, idx.rebuildTotalItems, idx.rebuildCurrent, idx.rebuildFailed, idx.rebuildLastItemID, idx.rebuildLastChunks, idx.rebuildStartTime
}
