package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
	"go.uber.org/zap"
)

// DB database connection
type DB struct {
	*sql.DB
	logger *zap.Logger
}

// NewDB creates a database connection
func NewDB(dbPath string, logger *zap.Logger) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	database := &DB{
		DB:     db,
		logger: logger,
	}

	// Initialize tables
	if err := database.initTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return database, nil
}

// initTables initializes database tables
func (db *DB) initTables() error {
	// Create conversations table
	createConversationsTable := `
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		last_react_input TEXT,
		last_react_output TEXT
	);`

	// Create messages table
	createMessagesTable := `
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		mcp_execution_ids TEXT,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
	);`

	// Create process details table
	createProcessDetailsTable := `
	CREATE TABLE IF NOT EXISTS process_details (
		id TEXT PRIMARY KEY,
		message_id TEXT NOT NULL,
		conversation_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		message TEXT,
		data TEXT,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
	);`

	// Create tool execution records table
	createToolExecutionsTable := `
	CREATE TABLE IF NOT EXISTS tool_executions (
		id TEXT PRIMARY KEY,
		tool_name TEXT NOT NULL,
		arguments TEXT NOT NULL,
		status TEXT NOT NULL,
		result TEXT,
		error TEXT,
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		duration_ms INTEGER,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	// Create tool statistics table
	createToolStatsTable := `
	CREATE TABLE IF NOT EXISTS tool_stats (
		tool_name TEXT PRIMARY KEY,
		total_calls INTEGER NOT NULL DEFAULT 0,
		success_calls INTEGER NOT NULL DEFAULT 0,
		failed_calls INTEGER NOT NULL DEFAULT 0,
		last_call_time DATETIME,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	// Create Skills statistics table
	createSkillStatsTable := `
	CREATE TABLE IF NOT EXISTS skill_stats (
		skill_name TEXT PRIMARY KEY,
		total_calls INTEGER NOT NULL DEFAULT 0,
		success_calls INTEGER NOT NULL DEFAULT 0,
		failed_calls INTEGER NOT NULL DEFAULT 0,
		last_call_time DATETIME,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	// Create attack chain nodes table
	createAttackChainNodesTable := `
	CREATE TABLE IF NOT EXISTS attack_chain_nodes (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		node_type TEXT NOT NULL,
		node_name TEXT NOT NULL,
		tool_execution_id TEXT,
		metadata TEXT,
		risk_score INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
		FOREIGN KEY (tool_execution_id) REFERENCES tool_executions(id) ON DELETE SET NULL
	);`

	// Create attack chain edges table
	createAttackChainEdgesTable := `
	CREATE TABLE IF NOT EXISTS attack_chain_edges (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		source_node_id TEXT NOT NULL,
		target_node_id TEXT NOT NULL,
		edge_type TEXT NOT NULL,
		weight INTEGER DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
		FOREIGN KEY (source_node_id) REFERENCES attack_chain_nodes(id) ON DELETE CASCADE,
		FOREIGN KEY (target_node_id) REFERENCES attack_chain_nodes(id) ON DELETE CASCADE
	);`

	// Create knowledge retrieval logs table (kept in session database due to foreign key relationships)
	createKnowledgeRetrievalLogsTable := `
	CREATE TABLE IF NOT EXISTS knowledge_retrieval_logs (
		id TEXT PRIMARY KEY,
		conversation_id TEXT,
		message_id TEXT,
		query TEXT NOT NULL,
		risk_type TEXT,
		retrieved_items TEXT,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE SET NULL,
		FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE SET NULL
	);`

	// Create conversation groups table
	createConversationGroupsTable := `
	CREATE TABLE IF NOT EXISTS conversation_groups (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		icon TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);`

	// Create conversation group mappings table
	createConversationGroupMappingsTable := `
	CREATE TABLE IF NOT EXISTS conversation_group_mappings (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		group_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES conversation_groups(id) ON DELETE CASCADE,
		UNIQUE(conversation_id, group_id)
	);`

	// Create vulnerabilities table
	createVulnerabilitiesTable := `
	CREATE TABLE IF NOT EXISTS vulnerabilities (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		severity TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'open',
		vulnerability_type TEXT,
		target TEXT,
		proof TEXT,
		impact TEXT,
		recommendation TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
	);`

	// Create batch task queue table
	createBatchTaskQueuesTable := `
	CREATE TABLE IF NOT EXISTS batch_task_queues (
		id TEXT PRIMARY KEY,
		title TEXT,
		role TEXT,
		agent_mode TEXT NOT NULL DEFAULT 'single',
		schedule_mode TEXT NOT NULL DEFAULT 'manual',
		cron_expr TEXT,
		next_run_at DATETIME,
		schedule_enabled INTEGER NOT NULL DEFAULT 1,
		last_schedule_trigger_at DATETIME,
		last_schedule_error TEXT,
		last_run_error TEXT,
		status TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		current_index INTEGER NOT NULL DEFAULT 0
	);`

	// Create batch tasks table
	createBatchTasksTable := `
	CREATE TABLE IF NOT EXISTS batch_tasks (
		id TEXT PRIMARY KEY,
		queue_id TEXT NOT NULL,
		message TEXT NOT NULL,
		conversation_id TEXT,
		status TEXT NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		error TEXT,
		result TEXT,
		FOREIGN KEY (queue_id) REFERENCES batch_task_queues(id) ON DELETE CASCADE
	);`

	// Create WebShell connections table
	createWebshellConnectionsTable := `
	CREATE TABLE IF NOT EXISTS webshell_connections (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		password TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT 'php',
		method TEXT NOT NULL DEFAULT 'post',
		cmd_param TEXT NOT NULL DEFAULT '',
		remark TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	// Create WebShell connection extended state table (frontend workspace/terminal state persistence)
	createWebshellConnectionStatesTable := `
	CREATE TABLE IF NOT EXISTS webshell_connection_states (
		connection_id TEXT PRIMARY KEY,
		state_json TEXT NOT NULL DEFAULT '{}',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (connection_id) REFERENCES webshell_connections(id) ON DELETE CASCADE
	);`

	// Create indexes
	createIndexes := `
	CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_conversations_updated_at ON conversations(updated_at);
	CREATE INDEX IF NOT EXISTS idx_process_details_message_id ON process_details(message_id);
	CREATE INDEX IF NOT EXISTS idx_process_details_conversation_id ON process_details(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_tool_executions_tool_name ON tool_executions(tool_name);
	CREATE INDEX IF NOT EXISTS idx_tool_executions_start_time ON tool_executions(start_time);
	CREATE INDEX IF NOT EXISTS idx_tool_executions_status ON tool_executions(status);
	CREATE INDEX IF NOT EXISTS idx_chain_nodes_conversation ON attack_chain_nodes(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_chain_edges_conversation ON attack_chain_edges(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_chain_edges_source ON attack_chain_edges(source_node_id);
	CREATE INDEX IF NOT EXISTS idx_chain_edges_target ON attack_chain_edges(target_node_id);
	CREATE INDEX IF NOT EXISTS idx_knowledge_retrieval_logs_conversation ON knowledge_retrieval_logs(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_knowledge_retrieval_logs_message ON knowledge_retrieval_logs(message_id);
	CREATE INDEX IF NOT EXISTS idx_knowledge_retrieval_logs_created_at ON knowledge_retrieval_logs(created_at);
	CREATE INDEX IF NOT EXISTS idx_conversation_group_mappings_conversation ON conversation_group_mappings(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_conversation_group_mappings_group ON conversation_group_mappings(group_id);
	CREATE INDEX IF NOT EXISTS idx_conversations_pinned ON conversations(pinned);
	CREATE INDEX IF NOT EXISTS idx_vulnerabilities_conversation_id ON vulnerabilities(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_vulnerabilities_severity ON vulnerabilities(severity);
	CREATE INDEX IF NOT EXISTS idx_vulnerabilities_status ON vulnerabilities(status);
	CREATE INDEX IF NOT EXISTS idx_vulnerabilities_created_at ON vulnerabilities(created_at);
	CREATE INDEX IF NOT EXISTS idx_batch_tasks_queue_id ON batch_tasks(queue_id);
	CREATE INDEX IF NOT EXISTS idx_batch_task_queues_created_at ON batch_task_queues(created_at);
	CREATE INDEX IF NOT EXISTS idx_batch_task_queues_title ON batch_task_queues(title);
	CREATE INDEX IF NOT EXISTS idx_webshell_connections_created_at ON webshell_connections(created_at);
	CREATE INDEX IF NOT EXISTS idx_webshell_connection_states_updated_at ON webshell_connection_states(updated_at);
	`

	if _, err := db.Exec(createConversationsTable); err != nil {
		return fmt.Errorf("failed to create conversations table: %w", err)
	}

	if _, err := db.Exec(createMessagesTable); err != nil {
		return fmt.Errorf("failed to create messages table: %w", err)
	}

	if _, err := db.Exec(createProcessDetailsTable); err != nil {
		return fmt.Errorf("failed to create process_details table: %w", err)
	}

	if _, err := db.Exec(createToolExecutionsTable); err != nil {
		return fmt.Errorf("failed to create tool_executions table: %w", err)
	}

	if _, err := db.Exec(createToolStatsTable); err != nil {
		return fmt.Errorf("failed to create tool_stats table: %w", err)
	}

	if _, err := db.Exec(createSkillStatsTable); err != nil {
		return fmt.Errorf("failed to create skill_stats table: %w", err)
	}

	if _, err := db.Exec(createAttackChainNodesTable); err != nil {
		return fmt.Errorf("failed to create attack_chain_nodes table: %w", err)
	}

	if _, err := db.Exec(createAttackChainEdgesTable); err != nil {
		return fmt.Errorf("failed to create attack_chain_edges table: %w", err)
	}

	if _, err := db.Exec(createKnowledgeRetrievalLogsTable); err != nil {
		return fmt.Errorf("failed to create knowledge_retrieval_logs table: %w", err)
	}

	if _, err := db.Exec(createConversationGroupsTable); err != nil {
		return fmt.Errorf("failed to create conversation_groups table: %w", err)
	}

	if _, err := db.Exec(createConversationGroupMappingsTable); err != nil {
		return fmt.Errorf("failed to create conversation_group_mappings table: %w", err)
	}

	if _, err := db.Exec(createVulnerabilitiesTable); err != nil {
		return fmt.Errorf("failed to create vulnerabilities table: %w", err)
	}

	if _, err := db.Exec(createBatchTaskQueuesTable); err != nil {
		return fmt.Errorf("failed to create batch_task_queues table: %w", err)
	}

	if _, err := db.Exec(createBatchTasksTable); err != nil {
		return fmt.Errorf("failed to create batch_tasks table: %w", err)
	}

	if _, err := db.Exec(createWebshellConnectionsTable); err != nil {
		return fmt.Errorf("failed to create webshell_connections table: %w", err)
	}

	if _, err := db.Exec(createWebshellConnectionStatesTable); err != nil {
		return fmt.Errorf("failed to create webshell_connection_states table: %w", err)
	}

	if _, err := db.Exec(createEngagementsTable); err != nil {
		return fmt.Errorf("failed to create engagements table: %w", err)
	}

	// Add new fields to existing tables (if they don't exist) - must be done before creating indexes
	if err := db.migrateConversationsTable(); err != nil {
		db.logger.Warn("failed to migrate conversations table", zap.Error(err))
		// Don't return error, allow continued operation
	}

	if err := db.migrateConversationGroupsTable(); err != nil {
		db.logger.Warn("failed to migrate conversation_groups table", zap.Error(err))
		// Don't return error, allow continued operation
	}

	if err := db.migrateConversationGroupMappingsTable(); err != nil {
		db.logger.Warn("failed to migrate conversation_group_mappings table", zap.Error(err))
		// Don't return error, allow continued operation
	}

	if err := db.migrateBatchTaskQueuesTable(); err != nil {
		db.logger.Warn("failed to migrate batch_task_queues table", zap.Error(err))
		// Don't return error, allow continued operation
	}

	if _, err := db.Exec(createIndexes); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	db.logger.Info("database tables initialized successfully")
	return nil
}

// migrateConversationsTable migrates the conversations table to add new fields
func (db *DB) migrateConversationsTable() error {
	// Check if last_react_input field exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conversations') WHERE name='last_react_input'").Scan(&count)
	if err != nil {
		// If the query fails, try to add the field
		if _, addErr := db.Exec("ALTER TABLE conversations ADD COLUMN last_react_input TEXT"); addErr != nil {
			// If the field already exists, ignore the error (SQLite error messages may vary)
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add last_react_input field", zap.Error(addErr))
			}
		}
	} else if count == 0 {
		// Field doesn't exist, add it
		if _, err := db.Exec("ALTER TABLE conversations ADD COLUMN last_react_input TEXT"); err != nil {
			db.logger.Warn("failed to add last_react_input field", zap.Error(err))
		}
	}

	// Check if last_react_output field exists
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conversations') WHERE name='last_react_output'").Scan(&count)
	if err != nil {
		// If the query fails, try to add the field
		if _, addErr := db.Exec("ALTER TABLE conversations ADD COLUMN last_react_output TEXT"); addErr != nil {
			// If the field already exists, ignore the error
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add last_react_output field", zap.Error(addErr))
			}
		}
	} else if count == 0 {
		// Field doesn't exist, add it
		if _, err := db.Exec("ALTER TABLE conversations ADD COLUMN last_react_output TEXT"); err != nil {
			db.logger.Warn("failed to add last_react_output field", zap.Error(err))
		}
	}

	// Check if pinned field exists
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conversations') WHERE name='pinned'").Scan(&count)
	if err != nil {
		// If the query fails, try to add the field
		if _, addErr := db.Exec("ALTER TABLE conversations ADD COLUMN pinned INTEGER DEFAULT 0"); addErr != nil {
			// If the field already exists, ignore the error
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add pinned field", zap.Error(addErr))
			}
		}
	} else if count == 0 {
		// Field doesn't exist, add it
		if _, err := db.Exec("ALTER TABLE conversations ADD COLUMN pinned INTEGER DEFAULT 0"); err != nil {
			db.logger.Warn("failed to add pinned field", zap.Error(err))
		}
	}

	// Check if webshell_connection_id field exists (WebShell AI assistant conversation association)
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conversations') WHERE name='webshell_connection_id'").Scan(&count)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE conversations ADD COLUMN webshell_connection_id TEXT"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add webshell_connection_id field", zap.Error(addErr))
			}
		}
	} else if count == 0 {
		if _, err := db.Exec("ALTER TABLE conversations ADD COLUMN webshell_connection_id TEXT"); err != nil {
			db.logger.Warn("failed to add webshell_connection_id field", zap.Error(err))
		}
	}

	return nil
}

// migrateConversationGroupsTable migrates the conversation_groups table to add new fields
func (db *DB) migrateConversationGroupsTable() error {
	// Check if pinned field exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conversation_groups') WHERE name='pinned'").Scan(&count)
	if err != nil {
		// If the query fails, try to add the field
		if _, addErr := db.Exec("ALTER TABLE conversation_groups ADD COLUMN pinned INTEGER DEFAULT 0"); addErr != nil {
			// If the field already exists, ignore the error
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add pinned field", zap.Error(addErr))
			}
		}
	} else if count == 0 {
		// Field doesn't exist, add it
		if _, err := db.Exec("ALTER TABLE conversation_groups ADD COLUMN pinned INTEGER DEFAULT 0"); err != nil {
			db.logger.Warn("failed to add pinned field", zap.Error(err))
		}
	}

	return nil
}

// migrateConversationGroupMappingsTable migrates the conversation_group_mappings table to add new fields
func (db *DB) migrateConversationGroupMappingsTable() error {
	// Check if pinned field exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('conversation_group_mappings') WHERE name='pinned'").Scan(&count)
	if err != nil {
		// If the query fails, try to add the field
		if _, addErr := db.Exec("ALTER TABLE conversation_group_mappings ADD COLUMN pinned INTEGER DEFAULT 0"); addErr != nil {
			// If the field already exists, ignore the error
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add pinned field", zap.Error(addErr))
			}
		}
	} else if count == 0 {
		// Field doesn't exist, add it
		if _, err := db.Exec("ALTER TABLE conversation_group_mappings ADD COLUMN pinned INTEGER DEFAULT 0"); err != nil {
			db.logger.Warn("failed to add pinned field", zap.Error(err))
		}
	}

	return nil
}

// migrateBatchTaskQueuesTable migrates the batch_task_queues table to add new fields
func (db *DB) migrateBatchTaskQueuesTable() error {
	// Check if title field exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='title'").Scan(&count)
	if err != nil {
		// If the query fails, try to add the field
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN title TEXT"); addErr != nil {
			// If the field already exists, ignore the error
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add title field", zap.Error(addErr))
			}
		}
	} else if count == 0 {
		// Field doesn't exist, add it
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN title TEXT"); err != nil {
			db.logger.Warn("failed to add title field", zap.Error(err))
		}
	}

	// Check if role field exists
	var roleCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='role'").Scan(&roleCount)
	if err != nil {
		// If the query fails, try to add the field
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN role TEXT"); addErr != nil {
			// If the field already exists, ignore the error
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add role field", zap.Error(addErr))
			}
		}
	} else if roleCount == 0 {
		// Field doesn't exist, add it
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN role TEXT"); err != nil {
			db.logger.Warn("failed to add role field", zap.Error(err))
		}
	}

	// Check if agent_mode field exists
	var agentModeCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='agent_mode'").Scan(&agentModeCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN agent_mode TEXT NOT NULL DEFAULT 'single'"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add agent_mode field", zap.Error(addErr))
			}
		}
	} else if agentModeCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN agent_mode TEXT NOT NULL DEFAULT 'single'"); err != nil {
			db.logger.Warn("failed to add agent_mode field", zap.Error(err))
		}
	}

	// Check if schedule_mode field exists
	var scheduleModeCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='schedule_mode'").Scan(&scheduleModeCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN schedule_mode TEXT NOT NULL DEFAULT 'manual'"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add schedule_mode field", zap.Error(addErr))
			}
		}
	} else if scheduleModeCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN schedule_mode TEXT NOT NULL DEFAULT 'manual'"); err != nil {
			db.logger.Warn("failed to add schedule_mode field", zap.Error(err))
		}
	}

	// Check if cron_expr field exists
	var cronExprCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='cron_expr'").Scan(&cronExprCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN cron_expr TEXT"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add cron_expr field", zap.Error(addErr))
			}
		}
	} else if cronExprCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN cron_expr TEXT"); err != nil {
			db.logger.Warn("failed to add cron_expr field", zap.Error(err))
		}
	}

	// Check if next_run_at field exists
	var nextRunAtCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='next_run_at'").Scan(&nextRunAtCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN next_run_at DATETIME"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add next_run_at field", zap.Error(addErr))
			}
		}
	} else if nextRunAtCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN next_run_at DATETIME"); err != nil {
			db.logger.Warn("failed to add next_run_at field", zap.Error(err))
		}
	}

	// schedule_enabled: 0=pause Cron auto-scheduling, 1=allow (manual execution is not affected)
	var scheduleEnCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='schedule_enabled'").Scan(&scheduleEnCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN schedule_enabled INTEGER NOT NULL DEFAULT 1"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add schedule_enabled field", zap.Error(addErr))
			}
		}
	} else if scheduleEnCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN schedule_enabled INTEGER NOT NULL DEFAULT 1"); err != nil {
			db.logger.Warn("failed to add schedule_enabled field", zap.Error(err))
		}
	}

	var lastTrigCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='last_schedule_trigger_at'").Scan(&lastTrigCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN last_schedule_trigger_at DATETIME"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add last_schedule_trigger_at field", zap.Error(addErr))
			}
		}
	} else if lastTrigCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN last_schedule_trigger_at DATETIME"); err != nil {
			db.logger.Warn("failed to add last_schedule_trigger_at field", zap.Error(err))
		}
	}

	var lastSchedErrCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='last_schedule_error'").Scan(&lastSchedErrCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN last_schedule_error TEXT"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add last_schedule_error field", zap.Error(addErr))
			}
		}
	} else if lastSchedErrCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN last_schedule_error TEXT"); err != nil {
			db.logger.Warn("failed to add last_schedule_error field", zap.Error(err))
		}
	}

	var lastRunErrCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('batch_task_queues') WHERE name='last_run_error'").Scan(&lastRunErrCount)
	if err != nil {
		if _, addErr := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN last_run_error TEXT"); addErr != nil {
			errMsg := strings.ToLower(addErr.Error())
			if !strings.Contains(errMsg, "duplicate column") && !strings.Contains(errMsg, "already exists") {
				db.logger.Warn("failed to add last_run_error field", zap.Error(addErr))
			}
		}
	} else if lastRunErrCount == 0 {
		if _, err := db.Exec("ALTER TABLE batch_task_queues ADD COLUMN last_run_error TEXT"); err != nil {
			db.logger.Warn("failed to add last_run_error field", zap.Error(err))
		}
	}

	return nil
}

// NewKnowledgeDB creates a knowledge base database connection (only contains knowledge base related tables)
func NewKnowledgeDB(dbPath string, logger *zap.Logger) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open knowledge base database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to knowledge base database: %w", err)
	}

	database := &DB{
		DB:     sqlDB,
		logger: logger,
	}

	// Initialize knowledge base tables
	if err := database.initKnowledgeTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize knowledge base tables: %w", err)
	}

	return database, nil
}

// initKnowledgeTables initializes knowledge base database tables (only contains knowledge base related tables)
func (db *DB) initKnowledgeTables() error {
	// Create knowledge base items table
	createKnowledgeBaseItemsTable := `
	CREATE TABLE IF NOT EXISTS knowledge_base_items (
		id TEXT PRIMARY KEY,
		category TEXT NOT NULL,
		title TEXT NOT NULL,
		file_path TEXT NOT NULL,
		content TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);`

	// Create knowledge base vectors table
	createKnowledgeEmbeddingsTable := `
	CREATE TABLE IF NOT EXISTS knowledge_embeddings (
		id TEXT PRIMARY KEY,
		item_id TEXT NOT NULL,
		chunk_index INTEGER NOT NULL,
		chunk_text TEXT NOT NULL,
		embedding TEXT NOT NULL,
		sub_indexes TEXT NOT NULL DEFAULT '',
		embedding_model TEXT NOT NULL DEFAULT '',
		embedding_dim INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (item_id) REFERENCES knowledge_base_items(id) ON DELETE CASCADE
	);`

	// Create knowledge retrieval logs table (in independent knowledge base database, no foreign key constraints since conversations and messages tables may not be in this database)
	createKnowledgeRetrievalLogsTable := `
	CREATE TABLE IF NOT EXISTS knowledge_retrieval_logs (
		id TEXT PRIMARY KEY,
		conversation_id TEXT,
		message_id TEXT,
		query TEXT NOT NULL,
		risk_type TEXT,
		retrieved_items TEXT,
		created_at DATETIME NOT NULL
	);`

	// Create indexes
	createIndexes := `
	CREATE INDEX IF NOT EXISTS idx_knowledge_items_category ON knowledge_base_items(category);
	CREATE INDEX IF NOT EXISTS idx_knowledge_embeddings_item_id ON knowledge_embeddings(item_id);
	CREATE INDEX IF NOT EXISTS idx_knowledge_retrieval_logs_conversation ON knowledge_retrieval_logs(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_knowledge_retrieval_logs_message ON knowledge_retrieval_logs(message_id);
	CREATE INDEX IF NOT EXISTS idx_knowledge_retrieval_logs_created_at ON knowledge_retrieval_logs(created_at);
	`

	if _, err := db.Exec(createKnowledgeBaseItemsTable); err != nil {
		return fmt.Errorf("failed to create knowledge_base_items table: %w", err)
	}

	if _, err := db.Exec(createKnowledgeEmbeddingsTable); err != nil {
		return fmt.Errorf("failed to create knowledge_embeddings table: %w", err)
	}

	if _, err := db.Exec(createKnowledgeRetrievalLogsTable); err != nil {
		return fmt.Errorf("failed to create knowledge_retrieval_logs table: %w", err)
	}

	if _, err := db.Exec(createIndexes); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	if err := db.migrateKnowledgeEmbeddingsColumns(); err != nil {
		return fmt.Errorf("failed to migrate knowledge_embeddings columns: %w", err)
	}

	db.logger.Info("knowledge base database tables initialized successfully")
	return nil
}

// migrateKnowledgeEmbeddingsColumns adds sub_indexes, embedding_model, embedding_dim to existing databases.
func (db *DB) migrateKnowledgeEmbeddingsColumns() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='knowledge_embeddings'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	migrations := []struct {
		col  string
		stmt string
	}{
		{"sub_indexes", `ALTER TABLE knowledge_embeddings ADD COLUMN sub_indexes TEXT NOT NULL DEFAULT ''`},
		{"embedding_model", `ALTER TABLE knowledge_embeddings ADD COLUMN embedding_model TEXT NOT NULL DEFAULT ''`},
		{"embedding_dim", `ALTER TABLE knowledge_embeddings ADD COLUMN embedding_dim INTEGER NOT NULL DEFAULT 0`},
	}
	for _, m := range migrations {
		var colCount int
		q := `SELECT COUNT(*) FROM pragma_table_info('knowledge_embeddings') WHERE name = ?`
		if err := db.QueryRow(q, m.col).Scan(&colCount); err != nil {
			return err
		}
		if colCount > 0 {
			continue
		}
		if _, err := db.Exec(m.stmt); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
