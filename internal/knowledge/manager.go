package knowledge

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)
type Manager struct {
	db *sql.DB
	basePath string
	logger *zap.Logger
}
func NewManager(db *sql.DB, basePath string, logger *zap.Logger) *Manager {
	return &Manager{
		db: db,
		basePath: basePath,
		logger: logger,
	}
}
func (m *Manager) ScanKnowledgeBase() ([]string, error) {
	if m.basePath == "" {
		return nil, fmt.Errorf("knowledge basepathconfig")
	}
	if err := os.MkdirAll(m.basePath, 0755); err != nil {
		return nil, fmt.Errorf("createknowledge basedirectoryfailed: %w", err)
	}

	var itemsToIndex []string
	err := filepath.WalkDir(m.basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isIngestibleKnowledgeFile(path) {
			return nil
		}
		relPath, err := filepath.Rel(m.basePath, path)
		if err != nil {
			return err
		}
		parts := strings.Split(relPath, string(filepath.Separator))
		category := ""
		if len(parts) > 1 {
			category = parts[0]
		}
		base := filepath.Base(path)
		title := strings.TrimSuffix(base, filepath.Ext(base))
		content, err := os.ReadFile(path)
		if err != nil {
			m.logger.Warn("knowledge baseFilesfailed", zap.String("path", path), zap.Error(err))
			return nil
		}
		var existingID string
		var existingContent string
		var existingUpdatedAt time.Time
		err = m.db.QueryRow(
			"SELECT id, content, updated_at FROM knowledge_base_items WHERE file_path = ?",
			path,
		).Scan(&existingID, &existingContent, &existingUpdatedAt)

		if err == sql.ErrNoRows {
			id := uuid.New().String()
			now := time.Now()
			_, err = m.db.Exec(
				"INSERT INTO knowledge_base_items (id, category, title, file_path, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
				id, category, title, path, string(content), now, now,
			)
			if err != nil {
				return fmt.Errorf("Knowledgefailed: %w", err)
			}
			m.logger.Info("Knowledge", zap.String("id", id), zap.String("title", title), zap.String("category", category))
			itemsToIndex = append(itemsToIndex, id)
		} else if err == nil {
			contentChanged := existingContent != string(content)
			if contentChanged {
				_, err = m.db.Exec(
					"UPDATE knowledge_base_items SET category = ?, title = ?, content = ?, updated_at = ? WHERE id = ?",
					category, title, string(content), time.Now(), existingID,
				)
				if err != nil {
					return fmt.Errorf("updateKnowledgefailed: %w", err)
				}
				m.logger.Info("updateKnowledge", zap.String("id", existingID), zap.String("title", title))
				itemsToIndex = append(itemsToIndex, existingID)
			} else {
				m.logger.Debug("Knowledge,", zap.String("id", existingID), zap.String("title", title))
			}
		} else {
			return fmt.Errorf("failed to query knowledge items: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return itemsToIndex, nil
}
func (m *Manager) GetCategories() ([]string, error) {
	rows, err := m.db.Query("SELECT DISTINCT category FROM knowledge_base_items ORDER BY category")
	if err != nil {
		return nil, fmt.Errorf("queryfailed: %w", err)
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, fmt.Errorf("scanfailed: %w", err)
		}
		categories = append(categories, category)
	}

	return categories, nil
}
func (m *Manager) GetStats() (int, int, error) {
	categories, err := m.GetCategories()
	if err != nil {
		return 0, 0, fmt.Errorf("fetchfailed: %w", err)
	}
	totalCategories := len(categories)
	var totalItems int
	err = m.db.QueryRow("SELECT COUNT(*) FROM knowledge_base_items").Scan(&totalItems)
	if err != nil {
		return totalCategories, 0, fmt.Errorf("fetchKnowledgefailed: %w", err)
	}

	return totalCategories, totalItems, nil
}
func (m *Manager) GetCategoriesWithItems(limit, offset int) ([]*CategoryWithItems, int, error) {
	rows, err := m.db.Query(`
		SELECT category, COUNT(*) as item_count 
		FROM knowledge_base_items 
		GROUP BY category 
		ORDER BY category
	`)
	if err != nil {
		return nil, 0, fmt.Errorf("queryfailed: %w", err)
	}
	defer rows.Close()
	type categoryInfo struct {
		name string
		itemCount int
	}
	var allCategories []categoryInfo
	for rows.Next() {
		var info categoryInfo
		if err := rows.Scan(&info.name, &info.itemCount); err != nil {
			return nil, 0, fmt.Errorf("scanfailed: %w", err)
		}
		allCategories = append(allCategories, info)
	}

	totalCategories := len(allCategories)
	var paginatedCategories []categoryInfo
	if limit > 0 {
		start := offset
		end := offset + limit
		if start >= totalCategories {
			paginatedCategories = []categoryInfo{}
		} else {
			if end > totalCategories {
				end = totalCategories
			}
			paginatedCategories = allCategories[start:end]
		}
	} else {
		paginatedCategories = allCategories
	}
	result := make([]*CategoryWithItems, 0, len(paginatedCategories))
	for _, catInfo := range paginatedCategories {
		items, _, err := m.GetItemsSummary(catInfo.name, 0, 0)
		if err != nil {
			return nil, 0, fmt.Errorf("fetch %s Knowledgefailed: %w", catInfo.name, err)
		}

		result = append(result, &CategoryWithItems{
			Category: catInfo.name,
			ItemCount: catInfo.itemCount,
			Items: items,
		})
	}

	return result, totalCategories, nil
}
func (m *Manager) GetItems(category string) ([]*KnowledgeItem, error) {
	return m.GetItemsWithOptions(category, 0, 0, true)
}
func (m *Manager) GetItemsWithOptions(category string, limit, offset int, includeContent bool) ([]*KnowledgeItem, error) {
	var rows *sql.Rows
	var err error
	var query string
	var args []interface{}

	if includeContent {
		query = "SELECT id, category, title, file_path, content, created_at, updated_at FROM knowledge_base_items"
	} else {
		query = "SELECT id, category, title, file_path, created_at, updated_at FROM knowledge_base_items"
	}

	if category != "" {
		query += " WHERE category = ?"
		args = append(args, category)
	}

	query += " ORDER BY category, title"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	rows, err = m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query knowledge items: %w", err)
	}
	defer rows.Close()

	var items []*KnowledgeItem
	for rows.Next() {
		item := &KnowledgeItem{}
		var createdAt, updatedAt string

		if includeContent {
			if err := rows.Scan(&item.ID, &item.Category, &item.Title, &item.FilePath, &item.Content, &createdAt, &updatedAt); err != nil {
				return nil, fmt.Errorf("failed to scan knowledge item: %w", err)
			}
		} else {
			if err := rows.Scan(&item.ID, &item.Category, &item.Title, &item.FilePath, &createdAt, &updatedAt); err != nil {
				return nil, fmt.Errorf("failed to scan knowledge item: %w", err)
			}
			item.Content = ""
		}
		timeFormats := []string{
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			time.RFC3339,
			time.RFC3339Nano,
		}
		if createdAt != "" {
			for _, format := range timeFormats {
				parsed, err := time.Parse(format, createdAt)
				if err == nil && !parsed.IsZero() {
					item.CreatedAt = parsed
					break
				}
			}
		}
		if updatedAt != "" {
			for _, format := range timeFormats {
				parsed, err := time.Parse(format, updatedAt)
				if err == nil && !parsed.IsZero() {
					item.UpdatedAt = parsed
					break
				}
			}
		}
		if item.UpdatedAt.IsZero() && !item.CreatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}

		items = append(items, item)
	}

	return items, nil
}
func (m *Manager) GetItemsCount(category string) (int, error) {
	var count int
	var err error

	if category != "" {
		err = m.db.QueryRow("SELECT COUNT(*) FROM knowledge_base_items WHERE category = ?", category).Scan(&count)
	} else {
		err = m.db.QueryRow("SELECT COUNT(*) FROM knowledge_base_items").Scan(&count)
	}

	if err != nil {
		return 0, fmt.Errorf("queryKnowledgefailed: %w", err)
	}

	return count, nil
}
func (m *Manager) SearchItemsByKeyword(keyword string, category string) ([]*KnowledgeItemSummary, error) {
	if keyword == "" {
		return nil, fmt.Errorf("cannot be empty")
	}
	var query string
	var args []interface{}
	searchPattern := "%" + keyword + "%"

	query = `
		SELECT id, category, title, file_path, created_at, updated_at 
		FROM knowledge_base_items 
		WHERE (LOWER(title) LIKE LOWER(?) OR LOWER(category) LIKE LOWER(?) OR LOWER(file_path) LIKE LOWER(?) OR LOWER(content) LIKE LOWER(?))
	`
	args = append(args, searchPattern, searchPattern, searchPattern, searchPattern)
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}

	query += " ORDER BY category, title"

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("Knowledgefailed: %w", err)
	}
	defer rows.Close()

	var items []*KnowledgeItemSummary
	for rows.Next() {
		item := &KnowledgeItemSummary{}
		var createdAt, updatedAt string

		if err := rows.Scan(&item.ID, &item.Category, &item.Title, &item.FilePath, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan knowledge item: %w", err)
		}
		timeFormats := []string{
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			time.RFC3339,
			time.RFC3339Nano,
		}

		if createdAt != "" {
			for _, format := range timeFormats {
				parsed, err := time.Parse(format, createdAt)
				if err == nil && !parsed.IsZero() {
					item.CreatedAt = parsed
					break
				}
			}
		}

		if updatedAt != "" {
			for _, format := range timeFormats {
				parsed, err := time.Parse(format, updatedAt)
				if err == nil && !parsed.IsZero() {
					item.UpdatedAt = parsed
					break
				}
			}
		}

		if item.UpdatedAt.IsZero() && !item.CreatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}

		items = append(items, item)
	}

	return items, nil
}
func (m *Manager) GetItemsSummary(category string, limit, offset int) ([]*KnowledgeItemSummary, int, error) {
	total, err := m.GetItemsCount(category)
	if err != nil {
		return nil, 0, err
	}
	var rows *sql.Rows
	var query string
	var args []interface{}

	query = "SELECT id, category, title, file_path, created_at, updated_at FROM knowledge_base_items"

	if category != "" {
		query += " WHERE category = ?"
		args = append(args, category)
	}

	query += " ORDER BY category, title"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}
	}

	rows, err = m.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query knowledge items: %w", err)
	}
	defer rows.Close()

	var items []*KnowledgeItemSummary
	for rows.Next() {
		item := &KnowledgeItemSummary{}
		var createdAt, updatedAt string

		if err := rows.Scan(&item.ID, &item.Category, &item.Title, &item.FilePath, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan knowledge item: %w", err)
		}
		timeFormats := []string{
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			time.RFC3339,
			time.RFC3339Nano,
		}

		if createdAt != "" {
			for _, format := range timeFormats {
				parsed, err := time.Parse(format, createdAt)
				if err == nil && !parsed.IsZero() {
					item.CreatedAt = parsed
					break
				}
			}
		}

		if updatedAt != "" {
			for _, format := range timeFormats {
				parsed, err := time.Parse(format, updatedAt)
				if err == nil && !parsed.IsZero() {
					item.UpdatedAt = parsed
					break
				}
			}
		}

		if item.UpdatedAt.IsZero() && !item.CreatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}

		items = append(items, item)
	}

	return items, total, nil
}
func (m *Manager) GetItem(id string) (*KnowledgeItem, error) {
	item := &KnowledgeItem{}
	var createdAt, updatedAt string
	err := m.db.QueryRow(
		"SELECT id, category, title, file_path, content, created_at, updated_at FROM knowledge_base_items WHERE id = ?",
		id,
	).Scan(&item.ID, &item.Category, &item.Title, &item.FilePath, &item.Content, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("Knowledgedoes not exist")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query knowledge items: %w", err)
	}
	timeFormats := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	}
	if createdAt != "" {
		for _, format := range timeFormats {
			parsed, err := time.Parse(format, createdAt)
			if err == nil && !parsed.IsZero() {
				item.CreatedAt = parsed
				break
			}
		}
	}
	if updatedAt != "" {
		for _, format := range timeFormats {
			parsed, err := time.Parse(format, updatedAt)
			if err == nil && !parsed.IsZero() {
				item.UpdatedAt = parsed
				break
			}
		}
	}
	if item.UpdatedAt.IsZero() && !item.CreatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}

	return item, nil
}
func (m *Manager) CreateItem(category, title, content string) (*KnowledgeItem, error) {
	id := uuid.New().String()
	now := time.Now()
	filePath := filepath.Join(m.basePath, category, title+".md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	_, err := m.db.Exec(
		"INSERT INTO knowledge_base_items (id, category, title, file_path, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, category, title, filePath, content, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("Knowledgefailed: %w", err)
	}

	return &KnowledgeItem{
		ID: id,
		Category: category,
		Title: title,
		FilePath: filePath,
		Content: content,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
func (m *Manager) UpdateItem(id, category, title, content string) (*KnowledgeItem, error) {
	item, err := m.GetItem(id)
	if err != nil {
		return nil, err
	}
	newFilePath := filepath.Join(m.basePath, category, title+".md")
	if item.FilePath != newFilePath {
		if err := os.MkdirAll(filepath.Dir(newFilePath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
		if err := os.Rename(item.FilePath, newFilePath); err != nil {
			return nil, fmt.Errorf("Filesfailed: %w", err)
		}
		oldDir := filepath.Dir(item.FilePath)
		if isEmpty, _ := isEmptyDir(oldDir); isEmpty {
			if oldDir != m.basePath {
				if err := os.Remove(oldDir); err != nil {
					m.logger.Warn("deletedirectoryfailed", zap.String("dir", oldDir), zap.Error(err))
				}
			}
		}
	}
	if err := os.WriteFile(newFilePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	_, err = m.db.Exec(
		"UPDATE knowledge_base_items SET category = ?, title = ?, file_path = ?, content = ?, updated_at = ? WHERE id = ?",
		category, title, newFilePath, content, time.Now(), id,
	)
	if err != nil {
		return nil, fmt.Errorf("updateKnowledgefailed: %w", err)
	}
	_, _ = m.db.Exec("DELETE FROM knowledge_fts WHERE item_id = ?", id)
	_, err = m.db.Exec("DELETE FROM knowledge_embeddings WHERE item_id = ?", id)
	if err != nil {
		m.logger.Warn("deletefailed", zap.Error(err))
	}

	return m.GetItem(id)
}
func (m *Manager) DeleteItem(id string) error {
	var filePath string
	err := m.db.QueryRow("SELECT file_path FROM knowledge_base_items WHERE id = ?", id).Scan(&filePath)
	if err != nil {
		return fmt.Errorf("failed to query knowledge items: %w", err)
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		m.logger.Warn("deleteFilesfailed", zap.String("path", filePath), zap.Error(err))
	}
	_, err = m.db.Exec("DELETE FROM knowledge_base_items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleteKnowledgefailed: %w", err)
	}
	dir := filepath.Dir(filePath)
	if isEmpty, _ := isEmptyDir(dir); isEmpty {
		if dir != m.basePath {
			if err := os.Remove(dir); err != nil {
				m.logger.Warn("deletedirectoryfailed", zap.String("dir", dir), zap.Error(err))
			}
		}
	}

	return nil
}
func isEmptyDir(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			return false, nil
		}
	}
	return true, nil
}
func (m *Manager) LogRetrieval(conversationID, messageID, query, riskType string, retrievedItems []string) error {
	id := uuid.New().String()
	itemsJSON, _ := json.Marshal(retrievedItems)

	_, err := m.db.Exec(
		"INSERT INTO knowledge_retrieval_logs (id, conversation_id, message_id, query, risk_type, retrieved_items, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, conversationID, messageID, query, riskType, string(itemsJSON), time.Now(),
	)
	return err
}
func (m *Manager) GetIndexStatus() (map[string]interface{}, error) {
	var totalItems int
	err := m.db.QueryRow("SELECT COUNT(*) FROM knowledge_base_items").Scan(&totalItems)
	if err != nil {
		return nil, fmt.Errorf("queryKnowledgefailed: %w", err)
	}
	var indexedItems int
	err = m.db.QueryRow(`
		SELECT COUNT(DISTINCT item_id) 
		FROM knowledge_embeddings
	`).Scan(&indexedItems)
	if err != nil {
		return nil, fmt.Errorf("queryfailed: %w", err)
	}
	var progressPercent float64
	if totalItems > 0 {
		progressPercent = float64(indexedItems) / float64(totalItems) * 100
	} else {
		progressPercent = 100.0
	}
	isComplete := indexedItems >= totalItems && totalItems > 0

	return map[string]interface{}{
		"total_items": totalItems,
		"indexed_items": indexedItems,
		"progress_percent": progressPercent,
		"is_complete": isComplete,
	}, nil
}
func (m *Manager) GetRetrievalLogs(conversationID, messageID string, limit int) ([]*RetrievalLog, error) {
	var rows *sql.Rows
	var err error

	if messageID != "" {
		rows, err = m.db.Query(
			"SELECT id, conversation_id, message_id, query, risk_type, retrieved_items, created_at FROM knowledge_retrieval_logs WHERE message_id = ? ORDER BY created_at DESC LIMIT ?",
			messageID, limit,
		)
	} else if conversationID != "" {
		rows, err = m.db.Query(
			"SELECT id, conversation_id, message_id, query, risk_type, retrieved_items, created_at FROM knowledge_retrieval_logs WHERE conversation_id = ? ORDER BY created_at DESC LIMIT ?",
			conversationID, limit,
		)
	} else {
		rows, err = m.db.Query(
			"SELECT id, conversation_id, message_id, query, risk_type, retrieved_items, created_at FROM knowledge_retrieval_logs ORDER BY created_at DESC LIMIT ?",
			limit,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("queryretrievallogsfailed: %w", err)
	}
	defer rows.Close()

	var logs []*RetrievalLog
	for rows.Next() {
		log := &RetrievalLog{}
		var createdAt string
		var itemsJSON sql.NullString
		if err := rows.Scan(&log.ID, &log.ConversationID, &log.MessageID, &log.Query, &log.RiskType, &itemsJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scanretrievallogsfailed: %w", err)
		}
		var err error
		timeFormats := []string{
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			time.RFC3339,
			time.RFC3339Nano,
		}

		for _, format := range timeFormats {
			log.CreatedAt, err = time.Parse(format, createdAt)
			if err == nil && !log.CreatedAt.IsZero() {
				break
			}
		}
		if log.CreatedAt.IsZero() {
			m.logger.Warn("parseretrievallogsfailed",
				zap.String("timeStr", createdAt),
				zap.Error(err),
			)
			log.CreatedAt = time.Now()
		}
		if itemsJSON.Valid {
			json.Unmarshal([]byte(itemsJSON.String), &log.RetrievedItems)
		}

		logs = append(logs, log)
	}

	return logs, nil
}
func (m *Manager) DeleteRetrievalLog(id string) error {
	result, err := m.db.Exec("DELETE FROM knowledge_retrieval_logs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleteretrievallogsfailed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("fetchdeletefailed: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("retrievallogsdoes not exist")
	}

	return nil
}

// ingestibleKnowledgeExts are the text-based file types the knowledge loader
// ingests from the knowledge base directory. (PDF/HTML extraction and a live
// file watcher are planned extensions.)
var ingestibleKnowledgeExts = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".text": true,
	".json": true, ".yaml": true, ".yml": true, ".csv": true,
}

// isIngestibleKnowledgeFile reports whether path is a supported knowledge file.
func isIngestibleKnowledgeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ingestibleKnowledgeExts[ext]
}

// GetRetrievalAnalytics summarizes retrieval-log activity: total queries, the
// zero-hit rate (queries that returned nothing — candidate knowledge gaps), and
// the most frequent queries. Supports the knowledge analytics view (T2-P5).
func (m *Manager) GetRetrievalAnalytics(topN int) (map[string]interface{}, error) {
	if topN <= 0 {
		topN = 10
	}
	var total, zeroHit int
	_ = m.db.QueryRow("SELECT COUNT(*) FROM knowledge_retrieval_logs").Scan(&total)
	_ = m.db.QueryRow("SELECT COUNT(*) FROM knowledge_retrieval_logs WHERE retrieved_items IS NULL OR TRIM(retrieved_items)='' OR retrieved_items='[]'").Scan(&zeroHit)

	type qc struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}
	topQueries := []qc{}
	rows, err := m.db.Query("SELECT query, COUNT(*) c FROM knowledge_retrieval_logs GROUP BY query ORDER BY c DESC LIMIT ?", topN)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var q qc
			if err := rows.Scan(&q.Query, &q.Count); err == nil {
				topQueries = append(topQueries, q)
			}
		}
	}
	zeroHitRate := 0.0
	if total > 0 {
		zeroHitRate = float64(zeroHit) / float64(total)
	}
	return map[string]interface{}{
		"total_queries": total,
		"zero_hit":      zeroHit,
		"zero_hit_rate": zeroHitRate,
		"top_queries":   topQueries,
	}, nil
}
