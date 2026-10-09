package knowledge

import (
	"database/sql"
	"fmt"
)

// EnsureKnowledgeEmbeddingsSchema migrates knowledge_embeddings for sub_indexes + embedding metadata.
func EnsureKnowledgeEmbeddingsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='knowledge_embeddings'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if err := addKnowledgeEmbeddingsColumnIfMissing(db, "sub_indexes",
		`ALTER TABLE knowledge_embeddings ADD COLUMN sub_indexes TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := addKnowledgeEmbeddingsColumnIfMissing(db, "embedding_model",
		`ALTER TABLE knowledge_embeddings ADD COLUMN embedding_model TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := addKnowledgeEmbeddingsColumnIfMissing(db, "embedding_dim",
		`ALTER TABLE knowledge_embeddings ADD COLUMN embedding_dim INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	// Best-effort FTS5 lexical index for hybrid search. If the driver lacks FTS5
	// this is a no-op and dense retrieval is unaffected.
	ensureKnowledgeFTS(db)
	return nil
}

// KnowledgeFTSAvailable reports whether the FTS5 lexical index exists.
func KnowledgeFTSAvailable(db *sql.DB) bool {
	if db == nil {
		return false
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='knowledge_fts'`).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// ensureKnowledgeFTS creates the FTS5 virtual table and backfills it once from
// existing chunks. All errors are swallowed: hybrid search is an enhancement,
// never a hard dependency.
func ensureKnowledgeFTS(db *sql.DB) {
	if db == nil {
		return
	}
	_, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_fts USING fts5(
		chunk_id UNINDEXED, item_id UNINDEXED, chunk_text, tokenize='porter unicode61')`)
	if err != nil {
		return // FTS5 unavailable; dense retrieval still works
	}
	// Backfill once if empty but embeddings exist.
	var ftsCount, embCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM knowledge_fts`).Scan(&ftsCount)
	_ = db.QueryRow(`SELECT COUNT(*) FROM knowledge_embeddings`).Scan(&embCount)
	if ftsCount == 0 && embCount > 0 {
		_, _ = db.Exec(`INSERT INTO knowledge_fts (chunk_id, item_id, chunk_text)
			SELECT id, item_id, chunk_text FROM knowledge_embeddings`)
	}
}

func addKnowledgeEmbeddingsColumnIfMissing(db *sql.DB, column, alterSQL string) error {
	var colCount int
	q := `SELECT COUNT(*) FROM pragma_table_info('knowledge_embeddings') WHERE name = ?`
	if err := db.QueryRow(q, column).Scan(&colCount); err != nil {
		return err
	}
	if colCount > 0 {
		return nil
	}
	_, err := db.Exec(alterSQL)
	return err
}
func ensureKnowledgeEmbeddingsSubIndexesColumn(db *sql.DB) error {
	return EnsureKnowledgeEmbeddingsSchema(db)
}
