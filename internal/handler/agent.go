package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"pyntra/internal/agent"
	"pyntra/internal/config"
	"pyntra/internal/database"
	"pyntra/internal/enrich"
	"pyntra/internal/mcp/builtin"
	"pyntra/internal/multiagent"
	"pyntra/internal/security"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// safeTruncateString safely truncate string to avoid cutting UTF-8 characters in the middle
func safeTruncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}

	// convert string to rune slice for correct character count
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	// truncate to maximum length
	truncated := string(runes[:maxLen])

	// attempt to truncate at punctuation or space for more natural truncation
	// search backward from truncation point for suitable break point (no more than 20% of length)
	searchRange := maxLen / 5
	if searchRange > maxLen {
		searchRange = maxLen
	}
	breakChars := []rune(",./ ,.;:!?!？/\\-_")
	bestBreakPos := len(runes[:maxLen])

	for i := bestBreakPos - 1; i >= bestBreakPos-searchRange && i >= 0; i-- {
		for _, breakChar := range breakChars {
			if runes[i] == breakChar {
				bestBreakPos = i + 1 // break after punctuation
				goto found
			}
		}
	}

found:
	truncated = string(runes[:bestBreakPos])
	return truncated + "..."
}

// responsePlanAgg buffers main-assistant response_stream chunks for one "planning" process_detail row.
type responsePlanAgg struct {
	meta map[string]interface{}
	b strings.Builder
}

func normalizeProcessDetailText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

// discardPlanningIfEchoesToolResult drops buffered planning text when it only repeats the
// upcoming tool_result body. Streaming models often echo tool stdout in chunk.Content; flushing
// that into "planning" before persisting tool_result duplicates the output after page refresh.
func discardPlanningIfEchoesToolResult(respPlan *responsePlanAgg, toolData interface{}) {
	if respPlan == nil {
		return
	}
	plan := normalizeProcessDetailText(respPlan.b.String())
	if plan == "" {
		return
	}
	dataMap, ok := toolData.(map[string]interface{})
	if !ok {
		return
	}
	res, ok := dataMap["result"].(string)
	if !ok {
		return
	}
	r := normalizeProcessDetailText(res)
	if r == "" {
		return
	}
	if plan == r || strings.HasSuffix(plan, r) {
		respPlan.meta = nil
		respPlan.b.Reset()
	}
}

// AgentHandler Agentprocessor
type AgentHandler struct {
	agent *agent.Agent
	db *database.DB
	logger *zap.Logger
	tasks *AgentTaskManager
	batchTaskManager *BatchTaskManager
	config *config.Config // configuration reference, for getting role information
	knowledgeManager interface { // knowledge base manager interface
		LogRetrieval(conversationID, messageID, query, riskType string, retrievedItems []string) error
	}
	agentsMarkdownDir string // multi-agent: Markdown sub-Agent directory (absolute path, empty means not merging from disk)
	executor *security.Executor // for fanout per-host rate limiting
	enricher *enrich.Enricher   // for fanout fan-in enrichment
	batchCronParser cron.Parser
	batchRunnerMu sync.Mutex
	batchRunning map[string]struct{}
}

// SetExecutor wires the security executor (used by fanout mode to install a
// per-host rate limiter during a run).
func (h *AgentHandler) SetExecutor(e *security.Executor) { h.executor = e }

// SetEnricher wires the CVE/CWE/CVSS enricher used by fanout fan-in.
func (h *AgentHandler) SetEnricher(e *enrich.Enricher) { h.enricher = e }

// NewAgentHandler create new Agent handler
func NewAgentHandler(agent *agent.Agent, db *database.DB, cfg *config.Config, logger *zap.Logger) *AgentHandler {
	batchTaskManager := NewBatchTaskManager(logger)
	batchTaskManager.SetDB(db)
	if err := batchTaskManager.LoadFromDB(); err != nil {
		logger.Warn("failed to load batch task queue from database", zap.Error(err))
	}

	handler := &AgentHandler{
		agent: agent,
		db: db,
		logger: logger,
		tasks: NewAgentTaskManager(),
		batchTaskManager: batchTaskManager,
		config: cfg,
		batchCronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		batchRunning: make(map[string]struct{}),
	}
	go handler.batchQueueSchedulerLoop()
	return handler
}

// SetKnowledgeManager set knowledge base manager (for recording retrieval logs)
func (h *AgentHandler) SetKnowledgeManager(manager interface {
	LogRetrieval(conversationID, messageID, query, riskType string, retrievedItems []string) error
}) {
	h.knowledgeManager = manager
}

// SetAgentsMarkdownDir set agents/*.md sub-agent directory (absolute path); empty means only use sub_agents from config.yaml
func (h *AgentHandler) SetAgentsMarkdownDir(absDir string) {
	h.agentsMarkdownDir = strings.TrimSpace(absDir)
}

// ProcessMessage runs a single user message through the agent (single-agent, or
// multi-agent when enabled in config) and returns the assistant's reply together
// with the conversation ID. It persists the user and assistant messages and does
// not emit SSE. Used by chat-bot integrations (Telegram / Slack / Discord).
func (h *AgentHandler) ProcessMessage(ctx context.Context, conversationID, message, role string) (response string, convID string, err error) {
	if strings.TrimSpace(message) == "" {
		return "", conversationID, fmt.Errorf("empty message")
	}
	if conversationID == "" {
		conv, cerr := h.db.CreateConversation(safeTruncateString(message, 50))
		if cerr != nil {
			return "", "", fmt.Errorf("create conversation: %w", cerr)
		}
		conversationID = conv.ID
	}

	// Restore history: prefer saved ReAct data, fall back to the message table.
	history, herr := h.loadHistoryFromReActData(conversationID)
	if herr != nil {
		if msgs, merr := h.db.GetMessages(conversationID); merr == nil {
			history = make([]agent.ChatMessage, 0, len(msgs))
			for _, m := range msgs {
				history = append(history, agent.ChatMessage{Role: m.Role, Content: m.Content})
			}
		} else {
			history = []agent.ChatMessage{}
		}
	}

	// Resolve role prompt / tools / skills.
	finalMessage := message
	var roleTools, roleSkills []string
	if role != "" && role != "default" && h.config != nil && h.config.Roles != nil {
		if r, ok := h.config.Roles[role]; ok && r.Enabled {
			if r.UserPrompt != "" {
				finalMessage = r.UserPrompt + "\n\n" + message
			}
			if len(r.Tools) > 0 {
				roleTools = r.Tools
			}
			if len(r.Skills) > 0 {
				roleSkills = r.Skills
			}
		}
	}

	if _, aerr := h.db.AddMessage(conversationID, "user", message, nil); aerr != nil {
		return "", conversationID, fmt.Errorf("save user message: %w", aerr)
	}

	var mcpIDs []string
	var lastIn, lastOut string
	if h.config != nil && h.config.MultiAgent.Enabled {
		res, rerr := multiagent.RunDeepAgent(ctx, h.config, &h.config.MultiAgent, h.agent, h.logger,
			conversationID, finalMessage, history, roleTools, nil, h.agentsMarkdownDir, "")
		if rerr != nil {
			return "", conversationID, rerr
		}
		response, mcpIDs, lastIn, lastOut = res.Response, res.MCPExecutionIDs, res.LastReActInput, res.LastReActOutput
	} else {
		res, rerr := h.agent.AgentLoopWithProgress(ctx, finalMessage, history, conversationID, nil, roleTools, roleSkills)
		if rerr != nil {
			return "", conversationID, rerr
		}
		response, mcpIDs, lastIn, lastOut = res.Response, res.MCPExecutionIDs, res.LastReActInput, res.LastReActOutput
	}

	if _, aerr := h.db.AddMessage(conversationID, "assistant", response, mcpIDs); aerr != nil {
		h.logger.Warn("bot: failed to save assistant message", zap.Error(aerr))
	}
	if lastIn != "" || lastOut != "" {
		if serr := h.db.SaveReActData(conversationID, lastIn, lastOut); serr != nil {
			h.logger.Warn("bot: failed to save ReAct data", zap.Error(serr))
		}
	}
	return response, conversationID, nil
}

// ChatAttachment chat attachment (user uploaded file)
type ChatAttachment struct {
	FileName string `json:"fileName"` // display file name
	Content string `json:"content,omitempty"` // text or base64; leave empty if already uploaded to server
	MimeType string `json:"mimeType,omitempty"`
	ServerPath string `json:"serverPath,omitempty"` // absolute path saved under chat_uploads (returned by POST /api/chat-uploads)
}

// ChatRequest chat request
type ChatRequest struct {
	Message string `json:"message" binding:"required"`
	ConversationID string `json:"conversationId,omitempty"`
	Role string `json:"role,omitempty"` // role name
	Attachments []ChatAttachment `json:"attachments,omitempty"`
	WebShellConnectionID string `json:"webshellConnectionId,omitempty"` // WebShell Management - AI Assistant: currently selected connection ID, only use webshell_* tools
	// Orchestration only for /api/multi-agent, /api/multi-agent/stream: deep | plan_execute | supervisor; empty is equivalent to deep. Server defaults to deep for robot/batch without request body. /api/eino-agent* does not use this field.
	Orchestration string `json:"orchestration,omitempty"`
}

const (
	maxAttachments = 10
	chatUploadsDirName = "chat_uploads" // root directory for saving conversation attachments (relative to current working directory)
)

// validateChatAttachmentServerPath validate absolute path falls under working directory chat_uploads and is a regular file (prevent path traversal)
func validateChatAttachmentServerPath(abs string) (string, error) {
	p := strings.TrimSpace(abs)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	root := filepath.Join(cwd, chatUploadsDirName)
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", err
	}
	sep := string(filepath.Separator)
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+sep) {
		return "", fmt.Errorf("path outside chat_uploads")
	}
	st, err := os.Stat(pathAbs)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("not a regular file")
	}
	return pathAbs, nil
}

// avoidChatUploadDestCollision if path already exists, generate new filename with timestamp + random suffix (consistent with upload interface naming style)
func avoidChatUploadDestCollision(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	nameNoExt := strings.TrimSuffix(base, ext)
	suffix := fmt.Sprintf("_%s_%s", time.Now().Format("150405"), shortRand(6))
	var unique string
	if ext != "" {
		unique = nameNoExt + suffix + ext
	} else {
		unique = base + suffix
	}
	return filepath.Join(dir, unique)
}

// relocateManualOrNewUploadToConversation when there is no conversation ID, frontend uploads to .../date/_manual; after the first message creates a conversation, move files to .../date/{conversationId}/ for conversation isolation
func relocateManualOrNewUploadToConversation(absPath, conversationID string, logger *zap.Logger) (string, error) {
	conv := strings.TrimSpace(conversationID)
	if conv == "" {
		return absPath, nil
	}
	convSan := strings.ReplaceAll(conv, string(filepath.Separator), "_")
	if convSan == "" || convSan == "_manual" || convSan == "_new" {
		return absPath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return absPath, err
	}
	rootAbs, err := filepath.Abs(filepath.Join(cwd, chatUploadsDirName))
	if err != nil {
		return absPath, err
	}
	rel, err := filepath.Rel(rootAbs, absPath)
	if err != nil {
		return absPath, nil
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	var segs []string
	for _, p := range strings.Split(rel, "/") {
		if p != "" && p != "." {
			segs = append(segs, p)
		}
	}
	if len(segs) != 3 {
		return absPath, nil
	}
	datePart, placeFolder, baseName := segs[0], segs[1], segs[2]
	if placeFolder != "_manual" && placeFolder != "_new" {
		return absPath, nil
	}
	targetDir := filepath.Join(rootAbs, datePart, convSan)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create conversation attachment directory: %w", err)
	}
	dest := filepath.Join(targetDir, baseName)
	dest = avoidChatUploadDestCollision(dest)
	if err := os.Rename(absPath, dest); err != nil {
		return "", fmt.Errorf("failed to move attachment to conversation directory: %w", err)
	}
	out, _ := filepath.Abs(dest)
	if logger != nil {
		logger.Info("conversation attachment moved from placeholder directory to conversation directory",
			zap.String("from", absPath),
			zap.String("to", out),
			zap.String("conversationId", conv))
	}
	return out, nil
}

// saveAttachmentsToDateAndConversationDir process attachment: if serverPath is provided, only validate existing file; otherwise write content to chat_uploads/YYYY-MM-DD/{conversationID}/. Use "_new" as directory name when conversationID is empty (new conversation has no ID yet)
func saveAttachmentsToDateAndConversationDir(attachments []ChatAttachment, conversationID string, logger *zap.Logger) (savedPaths []string, err error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	dateDir := filepath.Join(cwd, chatUploadsDirName, time.Now().Format("2006-01-02"))
	convDirName := strings.TrimSpace(conversationID)
	if convDirName == "" {
		convDirName = "_new"
	} else {
		convDirName = strings.ReplaceAll(convDirName, string(filepath.Separator), "_")
	}
	targetDir := filepath.Join(dateDir, convDirName)
	if err = os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}
	savedPaths = make([]string, 0, len(attachments))
	for i, a := range attachments {
		if sp := strings.TrimSpace(a.ServerPath); sp != "" {
			valid, verr := validateChatAttachmentServerPath(sp)
			if verr != nil {
				return nil, fmt.Errorf("attachment %s: %w", a.FileName, verr)
			}
			finalPath, rerr := relocateManualOrNewUploadToConversation(valid, conversationID, logger)
			if rerr != nil {
				return nil, fmt.Errorf("attachment %s: %w", a.FileName, rerr)
			}
			savedPaths = append(savedPaths, finalPath)
			if logger != nil {
				logger.Debug("conversationattachment uses already uploaded path", zap.Int("index", i+1), zap.String("fileName", a.FileName), zap.String("path", finalPath))
			}
			continue
		}
		if strings.TrimSpace(a.Content) == "" {
			return nil, fmt.Errorf("attachment %s or serverPath", a.FileName)
		}
		raw, decErr := attachmentContentToBytes(a)
		if decErr != nil {
			return nil, fmt.Errorf("attachment %s failed: %w", a.FileName, decErr)
		}
		baseName := filepath.Base(a.FileName)
		if baseName == "" || baseName == "." {
			baseName = "file"
		}
		baseName = strings.ReplaceAll(baseName, string(filepath.Separator), "_")
		ext := filepath.Ext(baseName)
		nameNoExt := strings.TrimSuffix(baseName, ext)
		suffix := fmt.Sprintf("_%s_%s", time.Now().Format("150405"), shortRand(6))
		var unique string
		if ext != "" {
			unique = nameNoExt + suffix + ext
		} else {
			unique = baseName + suffix
		}
		fullPath := filepath.Join(targetDir, unique)
		if err = os.WriteFile(fullPath, raw, 0644); err != nil {
			return nil, fmt.Errorf("write file %s failed: %w", a.FileName, err)
		}
		absPath, _ := filepath.Abs(fullPath)
		savedPaths = append(savedPaths, absPath)
		if logger != nil {
			logger.Debug("conversation attachment saved", zap.Int("index", i+1), zap.String("fileName", a.FileName), zap.String("path", absPath))
		}
	}
	return savedPaths, nil
}

func shortRand(n int) string {
	const letters = "0123456789abcdef"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

func attachmentContentToBytes(a ChatAttachment) ([]byte, error) {
	content := a.Content
	if decoded, err := base64.StdEncoding.DecodeString(content); err == nil && len(decoded) > 0 {
		return decoded, nil
	}
	return []byte(content), nil
}

// userMessageContentForStorage return user message content to be stored in database: when there are attachments, append attachment names (and paths) after the text, they can still be displayed after refresh, and the large model can get paths from history when continuing the conversation
func userMessageContentForStorage(message string, attachments []ChatAttachment, savedPaths []string) string {
	if len(attachments) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	for i, a := range attachments {
		b.WriteString("\n📎 ")
		b.WriteString(a.FileName)
		if i < len(savedPaths) && savedPaths[i] != "" {
			b.WriteString(": ")
			b.WriteString(savedPaths[i])
		}
	}
	return b.String()
}

// appendAttachmentsToMessage only append the saved path of attachments to the end of the user message, do not inline attachment content anymore, avoid overly long context
func appendAttachmentsToMessage(msg string, attachments []ChatAttachment, savedPaths []string) string {
	if len(attachments) == 0 {
		return msg
	}
	var b strings.Builder
	b.WriteString(msg)
	b.WriteString("\n\n[User uploaded files have been saved to the following paths (please read file content as needed, instead of relying on inline content)]\n")
	for i, a := range attachments {
		if i < len(savedPaths) && savedPaths[i] != "" {
			b.WriteString(fmt.Sprintf("- %s: %s\n", a.FileName, savedPaths[i]))
		} else {
			b.WriteString(fmt.Sprintf("- %s: (path unknown, may have failed to save)\n", a.FileName))
		}
	}
	return b.String()
}

// ChatResponse chat response
type ChatResponse struct {
	Response string `json:"response"`
	MCPExecutionIDs []string `json:"mcpExecutionIds,omitempty"` // list of MCP execution IDs executed in this conversation
	ConversationID string `json:"conversationId"` // conversation ID
	Time time.Time `json:"time"`
}

// AgentLoop handle Agent Loop request
func (h *AgentHandler) AgentLoop(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("received Agent Loop request",
		zap.String("message", req.Message),
		zap.String("conversationId", req.ConversationID),
	)
	conversationID := req.ConversationID
	if conversationID == "" {
		title := safeTruncateString(req.Message, 50)
		conv, err := h.db.CreateConversation(title)
		if err != nil {
			h.logger.Error("failed to create conversation", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		conversationID = conv.ID
	} else {
		// verify conversation exists
		_, err := h.db.GetConversation(conversationID)
		if err != nil {
			h.logger.Error("conversation does not exist", zap.String("conversationId", conversationID), zap.Error(err))
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation does not exist"})
			return
		}
	}

	// preferentially try to restore history context from saved ReAct data
	agentHistoryMessages, err := h.loadHistoryFromReActData(conversationID)
	if err != nil {
		h.logger.Warn("failed to load history messages from ReAct data, using message table", zap.Error(err))
		// fall back to using database message table
		historyMessages, err := h.db.GetMessages(conversationID)
		if err != nil {
			h.logger.Warn("failed to get history messages", zap.Error(err))
			agentHistoryMessages = []agent.ChatMessage{}
		} else {
			// convert database messages to Agent message format
			agentHistoryMessages = make([]agent.ChatMessage, 0, len(historyMessages))
			for _, msg := range historyMessages {
				agentHistoryMessages = append(agentHistoryMessages, agent.ChatMessage{
					Role: msg.Role,
					Content: msg.Content,
				})
			}
			h.logger.Info("load history messages from message table", zap.Int("count", len(agentHistoryMessages)))
		}
	} else {
		h.logger.Info("restore history context from ReAct data", zap.Int("count", len(agentHistoryMessages)))
	}

	// validate attachment count (non-streaming)
	if len(req.Attachments) > maxAttachments {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("maximum attachments %d items", maxAttachments)})
		return
	}

	// apply role user prompt and tool configuration
	finalMessage := req.Message
	var roleTools []string // role configured tool list
	var roleSkills []string

	// WebShell AI Assistant mode: bind current connection, only open webshell_* tools and inject connection_id
	if req.WebShellConnectionID != "" {
		conn, err := h.db.GetWebshellConnection(strings.TrimSpace(req.WebShellConnectionID))
		if err != nil || conn == nil {
			h.logger.Warn("WebShell AI assistant:not foundconnection", zap.String("id", req.WebShellConnectionID), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "WebShell connection not found"})
			return
		}
		remark := conn.Remark
		if remark == "" {
			remark = conn.URL
		}
		finalMessage = fmt.Sprintf("[WebShell assistant context] current connection ID:%s,remark:%s.available tools (only use when operating on this connection, connection_id fill \"%s\"):webshell_exec/webshell_file_list/webshell_file_read/webshell_file_write/record_vulnerability/list_knowledge_risk_types/search_knowledge_base.Skills packages please use the built-in `skill` tool in 'Multi-Agent / Eino DeepAgent' conversation for progressive loading.\n\nuser request:%s",
			conn.ID, remark, conn.ID, req.Message)
		roleTools = []string{
			builtin.ToolWebshellExec,
			builtin.ToolWebshellFileList,
			builtin.ToolWebshellFileRead,
			builtin.ToolWebshellFileWrite,
			builtin.ToolRecordVulnerability,
			builtin.ToolListKnowledgeRiskTypes,
			builtin.ToolSearchKnowledgeBase,
		}
		roleSkills = nil
	} else if req.Role != "" && req.Role != "default" {
		if h.config.Roles != nil {
			if role, exists := h.config.Roles[req.Role]; exists && role.Enabled {
				if role.UserPrompt != "" {
					finalMessage = role.UserPrompt + "\n\n" + req.Message
					h.logger.Info("apply role user prompt", zap.String("role", req.Role))
				}
				// getrole configured tool list(prefer to usetoolsfield,backward compatiblemcpsfield)
				if len(role.Tools) > 0 {
					roleTools = role.Tools
					h.logger.Info("use role configured tool list", zap.String("role", req.Role), zap.Int("toolCount", len(roleTools)))
				}
				if len(role.Skills) > 0 {
					roleSkills = role.Skills
					h.logger.Info("role configured skills, will be prompted in system prompt", zap.String("role", req.Role), zap.Int("skillCount", len(roleSkills)), zap.Strings("skills", roleSkills))
				}
			}
		}
	}
	var savedPaths []string
	if len(req.Attachments) > 0 {
		savedPaths, err = saveAttachmentsToDateAndConversationDir(req.Attachments, conversationID, h.logger)
		if err != nil {
			h.logger.Error("failed to save conversation attachment", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save uploaded file: " + err.Error()})
			return
		}
	}
	finalMessage = appendAttachmentsToMessage(finalMessage, req.Attachments, savedPaths)
	userContent := userMessageContentForStorage(req.Message, req.Attachments, savedPaths)
	_, err = h.db.AddMessage(conversationID, "user", userContent, nil)
	if err != nil {
		h.logger.Error("failed to save user message", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save user message: " + err.Error()})
		return
	}
	result, err := h.agent.AgentLoopWithProgress(c.Request.Context(), finalMessage, agentHistoryMessages, conversationID, nil, roleTools, roleSkills)
	if err != nil {
		h.logger.Error("Agent Loop execution failed", zap.Error(err))

		// even if execution fails, try to save ReAct data (if available in result)
		if result != nil && (result.LastReActInput != "" || result.LastReActOutput != "") {
			if saveErr := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); saveErr != nil {
				h.logger.Warn("failed to save ReAct data for failed task", zap.Error(saveErr))
			} else {
				h.logger.Info("saved ReAct data for failed task", zap.String("conversationId", conversationID))
			}
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// saveassistantreply
	_, err = h.db.AddMessage(conversationID, "assistant", result.Response, result.MCPExecutionIDs)
	if err != nil {
		h.logger.Error("failed to save assistant message", zap.Error(err))
		// return response even if save fails, but record error
		// because AI has already generated a response, user should be able to see it
	}

	// save input and output of the last round of ReAct
	if result.LastReActInput != "" || result.LastReActOutput != "" {
		if err := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); err != nil {
			h.logger.Warn("failed to save ReAct data", zap.Error(err))
		} else {
			h.logger.Info("saved ReAct data", zap.String("conversationId", conversationID))
		}
	}

	c.JSON(http.StatusOK, ChatResponse{
		Response: result.Response,
		MCPExecutionIDs: result.MCPExecutionIDs,
		ConversationID: conversationID,
		Time: time.Now(),
	})
}


// StreamEvent streaming event
type StreamEvent struct {
	Type string `json:"type"` // conversation, progress, tool_call, tool_result, response, error, cancelled, done
	Message string `json:"message"` // display message
	Data interface{} `json:"data,omitempty"`
}

// createProgressCallback create progress callback function for saving processDetails
// sendEventFunc: optional streaming event send function, if nil does not send streaming event
func (h *AgentHandler) createProgressCallback(conversationID, assistantMessageID string, sendEventFunc func(eventType, message string, data interface{})) agent.ProgressCallback {
	// used to save parameters in tool_call events for use in tool_result
	toolCallCache := make(map[string]map[string]interface{}) // toolCallId -> arguments

	// thinking_stream_*: do not persist individual records, aggregate by streamId, add a persistent thinking record before subsequent key events
	type thinkingBuf struct {
		b strings.Builder
		meta map[string]interface{}
	}
	thinkingStreams := make(map[string]*thinkingBuf) // streamId -> buf
	flushedThinking := make(map[string]bool) // streamId -> flushed

	// response_start + response_delta: frontend timeline displays as '📝 planning' (monitor.js), does not persist individual delta;
	// aggregate into one planning written to process_details, consistent with online after refresh.
	var respPlan responsePlanAgg
	flushResponsePlan := func() {
		if assistantMessageID == "" {
			return
		}
		content := strings.TrimSpace(respPlan.b.String())
		if content == "" {
			respPlan.meta = nil
			respPlan.b.Reset()
			return
		}
		data := map[string]interface{}{
			"source": "response_stream",
		}
		for k, v := range respPlan.meta {
			data[k] = v
		}
		if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "planning", content, data); err != nil {
			h.logger.Warn("failed to save process detail", zap.Error(err), zap.String("eventType", "planning"))
		}
		respPlan.meta = nil
		respPlan.b.Reset()
	}

	flushThinkingStreams := func() {
		if assistantMessageID == "" {
			return
		}
		for sid, tb := range thinkingStreams {
			if sid == "" || flushedThinking[sid] || tb == nil {
				continue
			}
			content := strings.TrimSpace(tb.b.String())
			if content == "" {
				flushedThinking[sid] = true
				continue
			}
			data := map[string]interface{}{
				"streamId": sid,
			}
			for k, v := range tb.meta {
				// avoid overwriting streamId
				if k == "streamId" {
					continue
				}
				data[k] = v
			}
			if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "thinking", content, data); err != nil {
				h.logger.Warn("failed to save process detail", zap.Error(err), zap.String("eventType", "thinking"))
			}
			flushedThinking[sid] = true
		}
	}

	return func(eventType, message string, data interface{}) {
		if sendEventFunc != nil {
			sendEventFunc(eventType, message, data)
		}
		if eventType == "tool_call" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				toolName, _ := dataMap["toolName"].(string)
				if toolName == builtin.ToolSearchKnowledgeBase {
					if toolCallId, ok := dataMap["toolCallId"].(string); ok && toolCallId != "" {
						if argumentsObj, ok := dataMap["argumentsObj"].(map[string]interface{}); ok {
							toolCallCache[toolCallId] = argumentsObj
						}
					}
				}
			}
		}
		if eventType == "tool_result" && h.knowledgeManager != nil {
			if dataMap, ok := data.(map[string]interface{}); ok {
				toolName, _ := dataMap["toolName"].(string)
				if toolName == builtin.ToolSearchKnowledgeBase {
					query := ""
					riskType := ""
					var retrievedItems []string
					if toolCallId, ok := dataMap["toolCallId"].(string); ok && toolCallId != "" {
						if cachedArgs, exists := toolCallCache[toolCallId]; exists {
							if q, ok := cachedArgs["query"].(string); ok && q != "" {
								query = q
							}
							if rt, ok := cachedArgs["risk_type"].(string); ok && rt != "" {
								riskType = rt
							}
							delete(toolCallCache, toolCallId)
						}
					}
					if query == "" {
						if arguments, ok := dataMap["argumentsObj"].(map[string]interface{}); ok {
							if q, ok := arguments["query"].(string); ok && q != "" {
								query = q
							}
							if rt, ok := arguments["risk_type"].(string); ok && rt != "" {
								riskType = rt
							}
						}
					}
					if query == "" {
						if result, ok := dataMap["result"].(string); ok && result != "" {
							if strings.Contains(result, "not found related to query '") {
								start := strings.Index(result, "not found related to query '") + len("not found related to query '")
								end := strings.Index(result[start:], "'")
								if end > 0 {
									query = result[start : start+end]
								}
							}
						}
						if query == "" {
							query = "unknown query"
						}
					}
					if result, ok := dataMap["result"].(string); ok && result != "" {
						metadataMatch := strings.Index(result, "<!-- METADATA:")
						if metadataMatch > 0 {
							metadataStart := metadataMatch + len("<!-- METADATA: ")
							metadataEnd := strings.Index(result[metadataStart:], " -->")
							if metadataEnd > 0 {
								metadataJSON := result[metadataStart : metadataStart+metadataEnd]
								var metadata map[string]interface{}
								if err := json.Unmarshal([]byte(metadataJSON), &metadata); err == nil {
									if meta, ok := metadata["_metadata"].(map[string]interface{}); ok {
										if ids, ok := meta["retrievedItemIDs"].([]interface{}); ok {
											retrievedItems = make([]string, 0, len(ids))
											for _, id := range ids {
												if idStr, ok := id.(string); ok {
													retrievedItems = append(retrievedItems, idStr)
												}
											}
										}
									}
								}
							}
						}
						if len(retrievedItems) == 0 && strings.Contains(result, "found") && !strings.Contains(result, "not found") {
							retrievedItems = []string{"_has_results"}
						}
					}

					// log retrieval (async, non-blocking)
					go func() {
						if err := h.knowledgeManager.LogRetrieval(conversationID, assistantMessageID, query, riskType, retrievedItems); err != nil {
							h.logger.Warn("failed to log knowledge retrieval", zap.Error(err))
						}
					}()
					if assistantMessageID != "" {
						retrievalData := map[string]interface{}{
							"query": query,
							"riskType": riskType,
							"toolName": toolName,
						}
						if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "knowledge_retrieval", fmt.Sprintf("retrieve knowledge: %s", query), retrievalData); err != nil {
							h.logger.Warn("failed to save knowledge retrieval details", zap.Error(err))
						}
					}
				}
			}
		}

		// sub-agent reply streaming delta is not persisted; when finished, merge into one eino_agent_reply
		if assistantMessageID != "" && eventType == "eino_agent_reply_stream_end" {
			flushResponsePlan()
			// ensure thinking stream is persisted before sub-agent reply (readable after refresh)
			flushThinkingStreams()
			if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "eino_agent_reply", message, data); err != nil {
				h.logger.Warn("failed to save process detail", zap.Error(err), zap.String("eventType", eventType))
			}
			return
		}

		// multi-agent main agent 'planning': response_start / response_delta only for SSE, aggregate into one planning
		if eventType == "response_start" {
			flushResponsePlan()
			respPlan.meta = nil
			if dataMap, ok := data.(map[string]interface{}); ok {
				respPlan.meta = make(map[string]interface{}, len(dataMap))
				for k, v := range dataMap {
					respPlan.meta[k] = v
				}
			}
			respPlan.b.Reset()
			return
		}
		if eventType == "response_delta" {
			respPlan.b.WriteString(message)
			if dataMap, ok := data.(map[string]interface{}); ok && respPlan.meta == nil {
				respPlan.meta = make(map[string]interface{}, len(dataMap))
				for k, v := range dataMap {
					respPlan.meta[k] = v
				}
			} else if dataMap, ok := data.(map[string]interface{}); ok {
				for k, v := range dataMap {
					respPlan.meta[k] = v
				}
			}
			return
		}
		if eventType == "response" {
			flushResponsePlan()
			return
		}

		// aggregate thinking_stream_* (ReasoningContent), do not persist individual records
		if eventType == "thinking_stream_start" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				if sid, ok2 := dataMap["streamId"].(string); ok2 && sid != "" {
					tb := thinkingStreams[sid]
					if tb == nil {
						tb = &thinkingBuf{meta: map[string]interface{}{}}
						thinkingStreams[sid] = tb
					}
					for k, v := range dataMap {
						tb.meta[k] = v
					}
				}
			}
			return
		}
		if eventType == "thinking_stream_delta" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				if sid, ok2 := dataMap["streamId"].(string); ok2 && sid != "" {
					tb := thinkingStreams[sid]
					if tb == nil {
						tb = &thinkingBuf{meta: map[string]interface{}{}}
						thinkingStreams[sid] = tb
					}
					tb.b.WriteString(message)
					// sometimes delta arrives before start, supplement metadata
					for k, v := range dataMap {
						tb.meta[k] = v
					}
				}
			}
			return
		}

		// when Agent sends thinking_stream_* and thinking (with same streamId) at the same time,
		// thinking_stream_* will already be aggregated and persisted in flushThinkingStreams();
		// here skip thinking with same streamId, avoid duplicate display in processDetails.
		if eventType == "thinking" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				if sid, ok2 := dataMap["streamId"].(string); ok2 && sid != "" {
					if tb, exists := thinkingStreams[sid]; exists && tb != nil {
						if strings.TrimSpace(tb.b.String()) != "" {
							return
						}
					}
					if flushedThinking[sid] {
						return
					}
				}
			}
		}

		// save process details to database (exclude response/done; response body is already in messages table)
		// response_start/response_delta already aggregated to planning, no individual records.
		if assistantMessageID != "" &&
			eventType != "response" &&
			eventType != "done" &&
			eventType != "response_start" &&
			eventType != "response_delta" &&
			eventType != "tool_result_delta" &&
			eventType != "eino_agent_reply_stream_start" &&
			eventType != "eino_agent_reply_stream_delta" &&
			eventType != "eino_agent_reply_stream_end" {
			if eventType == "tool_result" {
				discardPlanningIfEchoesToolResult(&respPlan, data)
			}
			flushResponsePlan()
			flushThinkingStreams()
			if err := h.db.AddProcessDetail(assistantMessageID, conversationID, eventType, message, data); err != nil {
				h.logger.Warn("failed to save process detail", zap.Error(err), zap.String("eventType", eventType))
			}
		}
	}
}

// AgentLoopStream handle Agent Loop streaming request
func (h *AgentHandler) AgentLoopStream(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		event := StreamEvent{
			Type: "error",
			Message: "request parameter error: " + err.Error(),
		}
		eventJSON, _ := json.Marshal(event)
		fmt.Fprintf(c.Writer, "data: %s\n\n", eventJSON)
		c.Writer.Flush()
		return
	}

	h.logger.Info("received Agent Loop streaming request",
		zap.String("message", req.Message),
		zap.String("conversationId", req.ConversationID),
	)

	// set SSE response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // disable nginx buffering

	// send initial event
	// used to track whether client has disconnected
	clientDisconnected := false
	// shared with sseKeepalive: prohibit concurrent writes to ResponseWriter, otherwise will corrupt chunked encoding (ERR_INVALID_CHUNKED_ENCODING).
	var sseWriteMu sync.Mutex
	// used to quickly confirm whether the model really produced streaming delta
	var responseDeltaCount int
	var responseStartLogged bool

	sendEvent := func(eventType, message string, data interface{}) {
		if eventType == "response_start" {
			responseDeltaCount = 0
			responseStartLogged = true
			h.logger.Info("SSE: response_start",
				zap.Int("conversationIdPresent", func() int {
					if m, ok := data.(map[string]interface{}); ok {
						if v, ok2 := m["conversationId"]; ok2 && v != nil && fmt.Sprint(v) != "" {
							return 1
						}
					}
					return 0
				}()),
				zap.String("messageGeneratedBy", func() string {
					if m, ok := data.(map[string]interface{}); ok {
						if v, ok2 := m["messageGeneratedBy"]; ok2 {
							if s, ok3 := v.(string); ok3 {
								return s
							}
							return fmt.Sprint(v)
						}
					}
					return ""
				}()),
			)
		} else if eventType == "response_delta" {
			responseDeltaCount++
			if responseStartLogged && responseDeltaCount <= 3 {
				h.logger.Info("SSE: response_delta",
					zap.Int("index", responseDeltaCount),
					zap.Int("deltaLen", len(message)),
					zap.String("deltaPreview", func() string {
						p := strings.ReplaceAll(message, "\n", "\\n")
						if len(p) > 80 {
							return p[:80] + "..."
						}
						return p
					}()),
				)
			}
		}

		// if client has disconnected, no more events sent
		if clientDisconnected {
			return
		}

		// check if request context is cancelled (client disconnected)
		select {
		case <-c.Request.Context().Done():
			clientDisconnected = true
			return
		default:
		}

		event := StreamEvent{
			Type: eventType,
			Message: message,
			Data: data,
		}
		eventJSON, _ := json.Marshal(event)

		sseWriteMu.Lock()
		_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", eventJSON)
		if err != nil {
			sseWriteMu.Unlock()
			clientDisconnected = true
			h.logger.Debug("client disconnected, stop sending SSE events", zap.Error(err))
			return
		}
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		} else {
			c.Writer.Flush()
		}
		sseWriteMu.Unlock()
	}

	// if no conversation ID, create new conversation (associate connection ID in WebShell assistant mode for persistent display)
	conversationID := req.ConversationID
	if conversationID == "" {
		title := safeTruncateString(req.Message, 50)
		var conv *database.Conversation
		var err error
		if req.WebShellConnectionID != "" {
			conv, err = h.db.CreateConversationWithWebshell(strings.TrimSpace(req.WebShellConnectionID), title)
		} else {
			conv, err = h.db.CreateConversation(title)
		}
		if err != nil {
			h.logger.Error("failed to create conversation", zap.Error(err))
			sendEvent("error", "failed to create conversation: "+err.Error(), nil)
			return
		}
		conversationID = conv.ID
		sendEvent("conversation", "session created", map[string]interface{}{
			"conversationId": conversationID,
		})
	} else {
		// verify conversation exists
		_, err := h.db.GetConversation(conversationID)
		if err != nil {
			h.logger.Error("conversation does not exist", zap.String("conversationId", conversationID), zap.Error(err))
			sendEvent("error", "conversation does not exist", nil)
			return
		}
	}

	// preferentially try to restore history context from saved ReAct data
	agentHistoryMessages, err := h.loadHistoryFromReActData(conversationID)
	if err != nil {
		h.logger.Warn("failed to load history messages from ReAct data, using message table", zap.Error(err))
		// fall back to using database message table
		historyMessages, err := h.db.GetMessages(conversationID)
		if err != nil {
			h.logger.Warn("failed to get history messages", zap.Error(err))
			agentHistoryMessages = []agent.ChatMessage{}
		} else {
			// convert database messages to Agent message format
			agentHistoryMessages = make([]agent.ChatMessage, 0, len(historyMessages))
			for _, msg := range historyMessages {
				agentHistoryMessages = append(agentHistoryMessages, agent.ChatMessage{
					Role: msg.Role,
					Content: msg.Content,
				})
			}
			h.logger.Info("load history messages from message table", zap.Int("count", len(agentHistoryMessages)))
		}
	} else {
		h.logger.Info("restore history context from ReAct data", zap.Int("count", len(agentHistoryMessages)))
	}

	// validate attachment count
	if len(req.Attachments) > maxAttachments {
		sendEvent("error", fmt.Sprintf("maximum attachments %d items", maxAttachments), nil)
		return
	}

	// apply role user prompt and tool configuration
	finalMessage := req.Message
	var roleTools []string // role configured tool list
	var roleSkills []string
	if req.WebShellConnectionID != "" {
		conn, errConn := h.db.GetWebshellConnection(strings.TrimSpace(req.WebShellConnectionID))
		if errConn != nil || conn == nil {
			h.logger.Warn("WebShell AI assistant:not foundconnection", zap.String("id", req.WebShellConnectionID), zap.Error(errConn))
			sendEvent("error", "WebShell connection not found", nil)
			return
		}
		remark := conn.Remark
		if remark == "" {
			remark = conn.URL
		}
		finalMessage = fmt.Sprintf("[WebShell assistant context] current connection ID:%s,remark:%s.available tools (only use when operating on this connection, connection_id fill \"%s\"):webshell_exec/webshell_file_list/webshell_file_read/webshell_file_write/record_vulnerability/list_knowledge_risk_types/search_knowledge_base.Skills packages please use the built-in `skill` tool in 'Multi-Agent / Eino DeepAgent' conversation for progressive loading.\n\nuser request:%s",
			conn.ID, remark, conn.ID, req.Message)
		roleTools = []string{
			builtin.ToolWebshellExec,
			builtin.ToolWebshellFileList,
			builtin.ToolWebshellFileRead,
			builtin.ToolWebshellFileWrite,
			builtin.ToolRecordVulnerability,
			builtin.ToolListKnowledgeRiskTypes,
			builtin.ToolSearchKnowledgeBase,
		}
	} else if req.Role != "" && req.Role != "default" {
		if h.config.Roles != nil {
			if role, exists := h.config.Roles[req.Role]; exists && role.Enabled {
				if role.UserPrompt != "" {
					finalMessage = role.UserPrompt + "\n\n" + req.Message
					h.logger.Info("apply role user prompt", zap.String("role", req.Role))
				}
				// getrole configured tool list(prefer to usetoolsfield,backward compatiblemcpsfield)
				if len(role.Tools) > 0 {
					roleTools = role.Tools
					h.logger.Info("use role configured tool list", zap.String("role", req.Role), zap.Int("toolCount", len(roleTools)))
				} else if len(role.MCPs) > 0 {
					h.logger.Info("role configuration uses old mcps field, will use all tools", zap.String("role", req.Role))
				}
				// note: role skills are only prompted in system prompt; for runtime loading please use Eino multi-agent built-in `skill` tool
				if len(role.Skills) > 0 {
					roleSkills = role.Skills
					h.logger.Info("role configured skills, AI can call on demand through tools", zap.String("role", req.Role), zap.Int("skillCount", len(role.Skills)), zap.Strings("skills", role.Skills))
				}
			}
		}
	}
	var savedPaths []string
	if len(req.Attachments) > 0 {
		savedPaths, err = saveAttachmentsToDateAndConversationDir(req.Attachments, conversationID, h.logger)
		if err != nil {
			h.logger.Error("failed to save conversation attachment", zap.Error(err))
			sendEvent("error", "failed to save uploaded file: "+err.Error(), nil)
			return
		}
	}
	// only append attachment save paths to finalMessage, avoid inlining file content into large model context
	finalMessage = appendAttachmentsToMessage(finalMessage, req.Attachments, savedPaths)
	// if roleTools is empty, indicates using all tools (default role or role without configured tools)
	userContent := userMessageContentForStorage(req.Message, req.Attachments, savedPaths)
	userMsgRow, err := h.db.AddMessage(conversationID, "user", userContent, nil)
	if err != nil {
		h.logger.Error("failed to save user message", zap.Error(err))
	}

	// pre-create assistant message for associating process details
	assistantMsg, err := h.db.AddMessage(conversationID, "assistant", "processing...", nil)
	if err != nil {
		h.logger.Error("failed to create assistant message", zap.Error(err))
		// if creation fails, continue but do not save process details
		assistantMsg = nil
	}
	var assistantMessageID string
	if assistantMsg != nil {
		assistantMessageID = assistantMsg.ID
	}

	// issue message ID early, convenient for frontend to hang 'delete this round' etc before streaming ends (no need to wait for entire segment to end before refresh)
	if userMsgRow != nil {
		sendEvent("message_saved", "", map[string]interface{}{
			"conversationId": conversationID,
			"userMessageId": userMsgRow.ID,
		})
	}

	// create progress callback function, reuse unified logic
	progressCallback := h.createProgressCallback(conversationID, assistantMessageID, sendEvent)

	// create a separate context for task execution, not cancelled by HTTP request
	// this way even if client disconnects (such as refreshing the page), the task can continue to execute
	baseCtx, cancelWithCause := context.WithCancelCause(context.Background())
	taskCtx, timeoutCancel := context.WithTimeout(baseCtx, 600*time.Minute)
	defer timeoutCancel()
	defer cancelWithCause(nil)

	if _, err := h.tasks.StartTask(conversationID, req.Message, cancelWithCause); err != nil {
		var errorMsg string
		if errors.Is(err, ErrTaskAlreadyRunning) {
			errorMsg = "⚠️ there is already a task running in the current session, please wait for the current task to complete or click the 'Stop Task' button and try again."
			sendEvent("error", errorMsg, map[string]interface{}{
				"conversationId": conversationID,
				"errorType": "task_already_running",
			})
		} else {
			errorMsg = "❌ unable to start task: " + err.Error()
			sendEvent("error", errorMsg, map[string]interface{}{
				"conversationId": conversationID,
				"errorType": "task_start_failed",
			})
		}
		if assistantMessageID != "" {
			if _, updateErr := h.db.Exec(
				"UPDATE messages SET content = ? WHERE id = ?",
				errorMsg,
				assistantMessageID,
			); updateErr != nil {
				h.logger.Warn("failed to update assistant message after error", zap.Error(updateErr))
			}
			if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "error", errorMsg, map[string]interface{}{
				"errorType": func() string {
					if errors.Is(err, ErrTaskAlreadyRunning) {
						return "task_already_running"
					}
					return "task_start_failed"
				}(),
			}); err != nil {
				h.logger.Warn("failed to save error details", zap.Error(err))
			}
		}

		sendEvent("done", "", map[string]interface{}{
			"conversationId": conversationID,
		})
		return
	}

	taskStatus := "completed"
	defer h.tasks.FinishTask(conversationID, taskStatus)

	// execute Agent Loop, pass in independent context, ensure task will not be interrupted due to client disconnection (use finalMessage containing role prompt and role tool list)
	sendEvent("progress", "analyzing your request...", nil)
	// note: roleSkills has been set above based on req.Role or WebShell mode
	stopKeepalive := make(chan struct{})
	go sseKeepalive(c, stopKeepalive, &sseWriteMu)
	defer close(stopKeepalive)

	result, err := h.agent.AgentLoopWithProgress(taskCtx, finalMessage, agentHistoryMessages, conversationID, progressCallback, roleTools, roleSkills)
	if err != nil {
		h.logger.Error("Agent Loop execution failed", zap.Error(err))
		cause := context.Cause(baseCtx)

		// check if it is user cancellation: context's cause is ErrTaskCancelled
		// if cause is ErrTaskCancelled, regardless of error type (including context.Canceled), treat as user cancellation
		// this way can correctly handle cancellation during API call
		isCancelled := errors.Is(cause, ErrTaskCancelled)

		switch {
		case isCancelled:
			taskStatus = "cancelled"
			cancelMsg := "task has been cancelled by user, subsequent operations have stopped."

			// update task status before sending event, ensure frontend can see status change in time
			h.tasks.UpdateTaskStatus(conversationID, taskStatus)

			if assistantMessageID != "" {
				if _, updateErr := h.db.Exec(
					"UPDATE messages SET content = ? WHERE id = ?",
					cancelMsg,
					assistantMessageID,
				); updateErr != nil {
					h.logger.Warn("updatecancelofassistantmessagefailed", zap.Error(updateErr))
				}
				h.db.AddProcessDetail(assistantMessageID, conversationID, "cancelled", cancelMsg, nil)
			}

			// even if task is cancelled, try to save ReAct data (if available in result)
			if result != nil && (result.LastReActInput != "" || result.LastReActOutput != "") {
				if err := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); err != nil {
					h.logger.Warn("failed to save ReAct data for cancelled task", zap.Error(err))
				} else {
					h.logger.Info("saved ReAct data for cancelled task", zap.String("conversationId", conversationID))
				}
			}

			sendEvent("cancelled", cancelMsg, map[string]interface{}{
				"conversationId": conversationID,
				"messageId": assistantMessageID,
			})
			sendEvent("done", "", map[string]interface{}{
				"conversationId": conversationID,
			})
			return
		case errors.Is(err, context.DeadlineExceeded) || errors.Is(cause, context.DeadlineExceeded):
			taskStatus = "timeout"
			timeoutMsg := "task execution timeout, has been automatically terminated."

			// update task status before sending event, ensure frontend can see status change in time
			h.tasks.UpdateTaskStatus(conversationID, taskStatus)

			if assistantMessageID != "" {
				if _, updateErr := h.db.Exec(
					"UPDATE messages SET content = ? WHERE id = ?",
					timeoutMsg,
					assistantMessageID,
				); updateErr != nil {
					h.logger.Warn("updatetimeoutofassistantmessagefailed", zap.Error(updateErr))
				}
				h.db.AddProcessDetail(assistantMessageID, conversationID, "timeout", timeoutMsg, nil)
			}

			// even if task times out, try to save ReAct data (if available in result)
			if result != nil && (result.LastReActInput != "" || result.LastReActOutput != "") {
				if err := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); err != nil {
					h.logger.Warn("failed to save ReAct data for timed out task", zap.Error(err))
				} else {
					h.logger.Info("saved ReAct data for timed out task", zap.String("conversationId", conversationID))
				}
			}

			sendEvent("error", timeoutMsg, map[string]interface{}{
				"conversationId": conversationID,
				"messageId": assistantMessageID,
			})
			sendEvent("done", "", map[string]interface{}{
				"conversationId": conversationID,
			})
			return
		default:
			taskStatus = "failed"
			errorMsg := "execution failed: " + err.Error()

			// update task status before sending event, ensure frontend can see status change in time
			h.tasks.UpdateTaskStatus(conversationID, taskStatus)

			if assistantMessageID != "" {
				if _, updateErr := h.db.Exec(
					"UPDATE messages SET content = ? WHERE id = ?",
					errorMsg,
					assistantMessageID,
				); updateErr != nil {
					h.logger.Warn("updatefailedofassistantmessagefailed", zap.Error(updateErr))
				}
				h.db.AddProcessDetail(assistantMessageID, conversationID, "error", errorMsg, nil)
			}
			if result != nil && (result.LastReActInput != "" || result.LastReActOutput != "") {
				if err := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); err != nil {
					h.logger.Warn("failed to save ReAct data for failed task", zap.Error(err))
				} else {
					h.logger.Info("saved ReAct data for failed task", zap.String("conversationId", conversationID))
				}
			}

			sendEvent("error", errorMsg, map[string]interface{}{
				"conversationId": conversationID,
				"messageId": assistantMessageID,
			})
			sendEvent("done", "", map[string]interface{}{
				"conversationId": conversationID,
			})
		}
		return
	}

	// updateassistantmessage content
	if assistantMsg != nil {
		_, err = h.db.Exec(
			"UPDATE messages SET content = ?, mcp_execution_ids = ? WHERE id = ?",
			result.Response,
			func() string {
				if len(result.MCPExecutionIDs) > 0 {
					jsonData, _ := json.Marshal(result.MCPExecutionIDs)
					return string(jsonData)
				}
				return ""
			}(),
			assistantMessageID,
		)
		if err != nil {
			h.logger.Error("updateassistantmessagefailed", zap.Error(err))
		}
	} else {
		_, err = h.db.AddMessage(conversationID, "assistant", result.Response, result.MCPExecutionIDs)
		if err != nil {
			h.logger.Error("failed to save assistant message", zap.Error(err))
		}
	}

	// save input and output of the last round of ReAct
	if result.LastReActInput != "" || result.LastReActOutput != "" {
		if err := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); err != nil {
			h.logger.Warn("failed to save ReAct data", zap.Error(err))
		} else {
			h.logger.Info("saved ReAct data", zap.String("conversationId", conversationID))
		}
	}
	sendEvent("response", result.Response, map[string]interface{}{
		"mcpExecutionIds": result.MCPExecutionIDs,
		"conversationId": conversationID,
		"messageId": assistantMessageID,
	})
	sendEvent("done", "", map[string]interface{}{
		"conversationId": conversationID,
	})
}
func (h *AgentHandler) CancelAgentLoop(c *gin.Context) {
	var req struct {
		ConversationID string `json:"conversationId" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ok, err := h.tasks.CancelTask(req.ConversationID, ErrTaskCancelled)
	if err != nil {
		h.logger.Error("canceltaskfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not foundexecuteoftask"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "cancelling",
		"conversationId": req.ConversationID,
		"message": "hascancelrequest,taskcurrentcompletedstop.",
	})
}
func (h *AgentHandler) ListAgentTasks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"tasks": h.tasks.GetActiveTasks(),
	})
}
func (h *AgentHandler) ListCompletedTasks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"tasks": h.tasks.GetCompletedTasks(),
	})
}

// BatchTaskRequest batch taskrequest
type BatchTaskRequest struct {
	Title string `json:"title"` // task title (optional)
	Tasks []string `json:"tasks" binding:"required"` // task list, one task per row
	Role string `json:"role,omitempty"`
	AgentMode string `json:"agentMode,omitempty"`
	ScheduleMode string `json:"scheduleMode,omitempty"` // manual | cron
	CronExpr string `json:"cronExpr,omitempty"`
	ExecuteNow bool `json:"executeNow,omitempty"`
}

func normalizeBatchQueueAgentMode(mode string) string {
	m := strings.TrimSpace(strings.ToLower(mode))
	if m == "multi" {
		return "deep"
	}
	if m == "" || m == "single" || m == "react" {
		return "single"
	}
	if m == "eino_single" {
		return "eino_single"
	}
	switch config.NormalizeMultiAgentOrchestration(m) {
	case "plan_execute":
		return "plan_execute"
	case "supervisor":
		return "supervisor"
	default:
		return "deep"
	}
}
func batchQueueWantsEino(agentMode string) bool {
	m := strings.TrimSpace(strings.ToLower(agentMode))
	return m == "multi" || m == "deep" || m == "plan_execute" || m == "supervisor"
}

func normalizeBatchQueueScheduleMode(mode string) string {
	if strings.TrimSpace(mode) == "cron" {
		return "cron"
	}
	return "manual"
}

// CreateBatchQueue createbatch task queue
func (h *AgentHandler) CreateBatchQueue(c *gin.Context) {
	var req BatchTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task listcannot be empty"})
		return
	}
	validTasks := make([]string, 0, len(req.Tasks))
	for _, task := range req.Tasks {
		if task != "" {
			validTasks = append(validTasks, task)
		}
	}

	if len(validTasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nooftask"})
		return
	}

	agentMode := normalizeBatchQueueAgentMode(req.AgentMode)
	scheduleMode := normalizeBatchQueueScheduleMode(req.ScheduleMode)
	cronExpr := strings.TrimSpace(req.CronExpr)
	var nextRunAt *time.Time
	if scheduleMode == "cron" {
		if cronExpr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cron expression cannot be empty when cron scheduling is enabled"})
			return
		}
		schedule, err := h.batchCronParser.Parse(cronExpr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cron expression: " + err.Error()})
			return
		}
		next := schedule.Next(time.Now())
		nextRunAt = &next
	}

	queue, createErr := h.batchTaskManager.CreateBatchQueue(req.Title, req.Role, agentMode, scheduleMode, cronExpr, nextRunAt, validTasks)
	if createErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": createErr.Error()})
		return
	}
	started := false
	if req.ExecuteNow {
		ok, err := h.startBatchQueueExecution(queue.ID, false)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "queueId": queue.ID})
			return
		}
		started = true
		if refreshed, exists := h.batchTaskManager.GetBatchQueue(queue.ID); exists {
			queue = refreshed
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"queueId": queue.ID,
		"queue": queue,
		"started": started,
	})
}

// GetBatchQueue getbatch task queue
func (h *AgentHandler) GetBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"queue": queue})
}

// ListBatchQueuesResponse batch task queuelistresponse
type ListBatchQueuesResponse struct {
	Queues []*BatchTaskQueue `json:"queues"`
	Total int `json:"total"`
	Page int `json:"page"`
	PageSize int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}
func (h *AgentHandler) ListBatchQueues(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	offsetStr := c.DefaultQuery("offset", "0")
	pageStr := c.Query("page")
	status := c.Query("status")
	keyword := c.Query("keyword")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
			offset = (page - 1) * limit
		}
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	const maxOffset = 100000
	if offset > maxOffset {
		offset = maxOffset
	}

	// defaultstatusis"all"
	if status == "" {
		status = "all"
	}

	// getqueuelistandtotal
	queues, total, err := h.batchTaskManager.ListQueues(limit, offset, status, keyword)
	if err != nil {
		h.logger.Error("getbatch task queuelistfailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}
	if pageStr == "" {
		page = (offset / limit) + 1
	}

	response := ListBatchQueuesResponse{
		Queues: queues,
		Total: total,
		Page: page,
		PageSize: limit,
		TotalPages: totalPages,
	}

	c.JSON(http.StatusOK, response)
}
func (h *AgentHandler) StartBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	ok, err := h.startBatchQueueExecution(queueID, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "batch taskhasstartexecute", "queueId": queueID})
}
func (h *AgentHandler) RerunBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	if queue.Status != "completed" && queue.Status != "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "completedorhascancelofqueue"})
		return
	}
	if !h.batchTaskManager.ResetQueueForRerun(queueID) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "resetqueuefailed"})
		return
	}
	ok, err := h.startBatchQueueExecution(queueID, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "startfailed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "batch taskhasstartexecute", "queueId": queueID})
}
func (h *AgentHandler) PauseBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	success := h.batchTaskManager.PauseQueue(queueID)
	if !success {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not existor"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "batch taskhas"})
}
func (h *AgentHandler) UpdateBatchQueueMetadata(c *gin.Context) {
	queueID := c.Param("queueId")
	var req struct {
		Title string `json:"title"`
		Role string `json:"role"`
		AgentMode string `json:"agentMode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.batchTaskManager.UpdateQueueMetadata(queueID, req.Title, req.Role, req.AgentMode); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, _ := h.batchTaskManager.GetBatchQueue(queueID)
	c.JSON(http.StatusOK, gin.H{"queue": updated})
}
func (h *AgentHandler) UpdateBatchQueueSchedule(c *gin.Context) {
	queueID := c.Param("queueId")
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	if queue.Status == "running" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "queuein,modifyconfigure"})
		return
	}
	var req struct {
		ScheduleMode string `json:"scheduleMode"`
		CronExpr string `json:"cronExpr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scheduleMode := normalizeBatchQueueScheduleMode(req.ScheduleMode)
	cronExpr := strings.TrimSpace(req.CronExpr)
	var nextRunAt *time.Time
	if scheduleMode == "cron" {
		if cronExpr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cron expression cannot be empty when cron scheduling is enabled"})
			return
		}
		schedule, err := h.batchCronParser.Parse(cronExpr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cron expression: " + err.Error()})
			return
		}
		next := schedule.Next(time.Now())
		nextRunAt = &next
	}
	h.batchTaskManager.UpdateQueueSchedule(queueID, scheduleMode, cronExpr, nextRunAt)
	updated, _ := h.batchTaskManager.GetBatchQueue(queueID)
	c.JSON(http.StatusOK, gin.H{"queue": updated})
}
func (h *AgentHandler) SetBatchQueueScheduleEnabled(c *gin.Context) {
	queueID := c.Param("queueId")
	if _, exists := h.batchTaskManager.GetBatchQueue(queueID); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	var req struct {
		ScheduleEnabled bool `json:"scheduleEnabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.batchTaskManager.SetScheduleEnabled(queueID, req.ScheduleEnabled) {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	queue, _ := h.batchTaskManager.GetBatchQueue(queueID)
	c.JSON(http.StatusOK, gin.H{"queue": queue})
}

// DeleteBatchQueue deletebatch task queue
func (h *AgentHandler) DeleteBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	success := h.batchTaskManager.DeleteQueue(queueID)
	if !success {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "batch task queuehasdelete"})
}

// UpdateBatchTask updatebatch taskmessage
func (h *AgentHandler) UpdateBatchTask(c *gin.Context) {
	queueID := c.Param("queueId")
	taskID := c.Param("taskId")

	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "taskmessagecannot be empty"})
		return
	}

	err := h.batchTaskManager.UpdateTaskMessage(queueID, taskID, req.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "taskhasupdate", "queue": queue})
}
func (h *AgentHandler) AddBatchTask(c *gin.Context) {
	queueID := c.Param("queueId")

	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "taskmessagecannot be empty"})
		return
	}

	task, err := h.batchTaskManager.AddTaskToQueue(queueID, req.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "taskhas", "task": task, "queue": queue})
}

// DeleteBatchTask deletebatch task
func (h *AgentHandler) DeleteBatchTask(c *gin.Context) {
	queueID := c.Param("queueId")
	taskID := c.Param("taskId")

	err := h.batchTaskManager.DeleteTask(queueID, taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue does not exist"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "taskhasdelete", "queue": queue})
}

func (h *AgentHandler) markBatchQueueRunning(queueID string) bool {
	h.batchRunnerMu.Lock()
	defer h.batchRunnerMu.Unlock()
	if _, exists := h.batchRunning[queueID]; exists {
		return false
	}
	h.batchRunning[queueID] = struct{}{}
	return true
}

func (h *AgentHandler) unmarkBatchQueueRunning(queueID string) {
	h.batchRunnerMu.Lock()
	defer h.batchRunnerMu.Unlock()
	delete(h.batchRunning, queueID)
}

func (h *AgentHandler) nextBatchQueueRunAt(cronExpr string, from time.Time) (*time.Time, error) {
	expr := strings.TrimSpace(cronExpr)
	if expr == "" {
		return nil, nil
	}
	schedule, err := h.batchCronParser.Parse(expr)
	if err != nil {
		return nil, err
	}
	next := schedule.Next(from)
	return &next, nil
}

func (h *AgentHandler) startBatchQueueExecution(queueID string, scheduled bool) (bool, error) {
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		return false, nil
	}
	if !h.markBatchQueueRunning(queueID) {
		return true, nil
	}

	if scheduled {
		if queue.ScheduleMode != "cron" {
			h.unmarkBatchQueueRunning(queueID)
			err := fmt.Errorf("queueenable cron ")
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
			return true, err
		}
		if queue.Status == "running" || queue.Status == "paused" || queue.Status == "cancelled" {
			h.unmarkBatchQueueRunning(queueID)
			err := fmt.Errorf("currentqueue statusexecute")
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
			return true, err
		}
		if !h.batchTaskManager.ResetQueueForRerun(queueID) {
			h.unmarkBatchQueueRunning(queueID)
			err := fmt.Errorf("resetqueuefailed")
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
			return true, err
		}
		queue, _ = h.batchTaskManager.GetBatchQueue(queueID)
	} else if queue.Status != "pending" && queue.Status != "paused" {
		h.unmarkBatchQueueRunning(queueID)
		return true, fmt.Errorf("queue statusstart")
	}

	if queue != nil && batchQueueWantsEino(queue.AgentMode) && (h.config == nil || !h.config.MultiAgent.Enabled) {
		h.unmarkBatchQueueRunning(queueID)
		err := fmt.Errorf("currentqueueconfigureis Eino multi-agent,enablemulti-agent")
		if scheduled {
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
		}
		return true, err
	}

	if scheduled {
		h.batchTaskManager.RecordScheduledRunStart(queueID)
	}
	h.batchTaskManager.UpdateQueueStatus(queueID, "running")
	if queue != nil && queue.ScheduleMode == "cron" {
		nextRunAt, err := h.nextBatchQueueRunAt(queue.CronExpr, time.Now())
		if err == nil {
			h.batchTaskManager.UpdateQueueSchedule(queueID, "cron", queue.CronExpr, nextRunAt)
		}
	}

	go h.executeBatchQueue(queueID)
	return true, nil
}

func (h *AgentHandler) batchQueueSchedulerLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		queues := h.batchTaskManager.GetLoadedQueues()
		now := time.Now()
		for _, queue := range queues {
			if queue == nil || queue.ScheduleMode != "cron" || !queue.ScheduleEnabled || queue.Status == "cancelled" || queue.Status == "running" || queue.Status == "paused" {
				continue
			}
			nextRunAt := queue.NextRunAt
			if nextRunAt == nil {
				next, err := h.nextBatchQueueRunAt(queue.CronExpr, now)
				if err != nil {
					h.logger.Warn("batch task cron expressioninvalid,", zap.String("queueId", queue.ID), zap.String("cronExpr", queue.CronExpr), zap.Error(err))
					continue
				}
				h.batchTaskManager.UpdateQueueSchedule(queue.ID, "cron", queue.CronExpr, next)
				nextRunAt = next
			}
			if nextRunAt != nil && (nextRunAt.Before(now) || nextRunAt.Equal(now)) {
				if _, err := h.startBatchQueueExecution(queue.ID, true); err != nil {
					h.logger.Warn("batch taskfailed", zap.String("queueId", queue.ID), zap.Error(err))
				}
			}
		}
	}
}

// executeBatchQueue executebatch task queue
func (h *AgentHandler) executeBatchQueue(queueID string) {
	defer h.unmarkBatchQueueRunning(queueID)
	h.logger.Info("startexecutebatch task queue", zap.String("queueId", queueID))

	for {
		// checkqueue status
		queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
		if !exists || queue.Status == "cancelled" || queue.Status == "completed" || queue.Status == "paused" {
			break
		}
		task, hasNext := h.batchTaskManager.GetNextTask(queueID)
		if !hasNext {
			q, ok := h.batchTaskManager.GetBatchQueue(queueID)
			lastRunErr := ""
			if ok {
				for _, t := range q.Tasks {
					if t.Status == "failed" && t.Error != "" {
						lastRunErr = t.Error
					}
				}
			}
			h.batchTaskManager.SetLastRunError(queueID, lastRunErr)
			h.batchTaskManager.UpdateQueueStatus(queueID, "completed")
			h.logger.Info("batch task queueexecutecompleted", zap.String("queueId", queueID))
			break
		}
		h.batchTaskManager.UpdateTaskStatus(queueID, task.ID, "running", "", "")

		// createnew conversation
		title := safeTruncateString(task.Message, 50)
		conv, err := h.db.CreateConversation(title)
		var conversationID string
		if err != nil {
			h.logger.Error("failed to create conversation", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(err))
			h.batchTaskManager.UpdateTaskStatus(queueID, task.ID, "failed", "", "failed to create conversation: "+err.Error())
			h.batchTaskManager.MoveToNextTask(queueID)
			continue
		}
		conversationID = conv.ID
		h.batchTaskManager.UpdateTaskStatusWithConversationID(queueID, task.ID, "running", "", "", conversationID)

		// apply role user prompt and tool configuration
		finalMessage := task.Message
		var roleTools []string // role configured tool list
		var roleSkills []string
		if queue.Role != "" && queue.Role != "default" {
			if h.config.Roles != nil {
				if role, exists := h.config.Roles[queue.Role]; exists && role.Enabled {
					if role.UserPrompt != "" {
						finalMessage = role.UserPrompt + "\n\n" + task.Message
						h.logger.Info("apply role user prompt", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("role", queue.Role))
					}
					// getrole configured tool list(prefer to usetoolsfield,backward compatiblemcpsfield)
					if len(role.Tools) > 0 {
						roleTools = role.Tools
						h.logger.Info("use role configured tool list", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("role", queue.Role), zap.Int("toolCount", len(roleTools)))
					}
					if len(role.Skills) > 0 {
						roleSkills = role.Skills
						h.logger.Info("role configured skills, will be prompted in system prompt", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("role", queue.Role), zap.Int("skillCount", len(roleSkills)), zap.Strings("skills", roleSkills))
					}
				}
			}
		}
		_, err = h.db.AddMessage(conversationID, "user", task.Message, nil)
		if err != nil {
			h.logger.Error("failed to save user message", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID), zap.Error(err))
		}

		// pre-create assistant message for associating process details
		assistantMsg, err := h.db.AddMessage(conversationID, "assistant", "processing...", nil)
		if err != nil {
			h.logger.Error("failed to create assistant message", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID), zap.Error(err))
			// if creation fails, continue but do not save process details
			assistantMsg = nil
		}
		var assistantMessageID string
		if assistantMsg != nil {
			assistantMessageID = assistantMsg.ID
		}
		progressCallback := h.createProgressCallback(conversationID, assistantMessageID, nil)
		h.logger.Info("executebatch task", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("message", task.Message), zap.String("role", queue.Role), zap.String("conversationId", conversationID))
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		h.batchTaskManager.SetTaskCancel(queueID, cancel)
		useBatchMulti := false
		useEinoSingle := false
		batchOrch := "deep"
		am := strings.TrimSpace(strings.ToLower(queue.AgentMode))
		if am == "multi" {
			am = "deep"
		}
		if am == "eino_single" {
			useEinoSingle = true
		} else if batchQueueWantsEino(queue.AgentMode) && h.config != nil && h.config.MultiAgent.Enabled {
			useBatchMulti = true
			batchOrch = config.NormalizeMultiAgentOrchestration(am)
		} else if queue.AgentMode == "" {
			if h.config != nil && h.config.MultiAgent.Enabled && h.config.MultiAgent.BatchUseMultiAgent {
				useBatchMulti = true
				batchOrch = "deep"
			}
		}
		useRunResult := useBatchMulti || useEinoSingle
		var result *agent.AgentLoopResult
		var resultMA *multiagent.RunResult
		var runErr error
		switch {
		case useBatchMulti:
			resultMA, runErr = multiagent.RunDeepAgent(ctx, h.config, &h.config.MultiAgent, h.agent, h.logger, conversationID, finalMessage, []agent.ChatMessage{}, roleTools, progressCallback, h.agentsMarkdownDir, batchOrch)
		case useEinoSingle:
			if h.config == nil {
				runErr = fmt.Errorf("serverconfigureload")
			} else {
				resultMA, runErr = multiagent.RunEinoSingleChatModelAgent(ctx, h.config, &h.config.MultiAgent, h.agent, h.logger, conversationID, finalMessage, []agent.ChatMessage{}, roleTools, roleSkills, progressCallback)
			}
		default:
			result, runErr = h.agent.AgentLoopWithProgress(ctx, finalMessage, []agent.ChatMessage{}, conversationID, progressCallback, roleTools, roleSkills)
		}
		h.batchTaskManager.SetTaskCancel(queueID, nil)
		cancel()

		if runErr != nil {
			errStr := runErr.Error()
			partialResp := ""
			if useRunResult && resultMA != nil {
				partialResp = resultMA.Response
			} else if result != nil {
				partialResp = result.Response
			}
			isCancelled := errors.Is(runErr, context.Canceled) ||
				strings.Contains(strings.ToLower(errStr), "context canceled") ||
				strings.Contains(strings.ToLower(errStr), "context cancelled") ||
				(partialResp != "" && (strings.Contains(partialResp, "taskhascancel") || strings.Contains(partialResp, "taskexecutein")))

			if isCancelled {
				h.logger.Info("batch taskcancel", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID))
				cancelMsg := "task has been cancelled by user, subsequent operations have stopped."
				if partialResp != "" && (strings.Contains(partialResp, "taskhascancel") || strings.Contains(partialResp, "taskexecutein")) {
					cancelMsg = partialResp
				}
				// updateassistantmessage content
				if assistantMessageID != "" {
					if _, updateErr := h.db.Exec(
						"UPDATE messages SET content = ? WHERE id = ?",
						cancelMsg,
						assistantMessageID,
					); updateErr != nil {
						h.logger.Warn("updatecancelofassistantmessagefailed", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(updateErr))
					}
					if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "cancelled", cancelMsg, nil); err != nil {
						h.logger.Warn("savecanceldetailsfailed", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(err))
					}
				} else {
					_, errMsg := h.db.AddMessage(conversationID, "assistant", cancelMsg, nil)
					if errMsg != nil {
						h.logger.Warn("savecancelmessagefailed", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(errMsg))
					}
				}
				// saveReActdata(if exists)
				if result != nil && (result.LastReActInput != "" || result.LastReActOutput != "") {
					if err := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); err != nil {
						h.logger.Warn("failed to save ReAct data for cancelled task", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(err))
					}
				} else if useRunResult && resultMA != nil && (resultMA.LastReActInput != "" || resultMA.LastReActOutput != "") {
					if err := h.db.SaveReActData(conversationID, resultMA.LastReActInput, resultMA.LastReActOutput); err != nil {
						h.logger.Warn("failed to save ReAct data for cancelled task", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(err))
					}
				}
				h.batchTaskManager.UpdateTaskStatusWithConversationID(queueID, task.ID, "cancelled", cancelMsg, "", conversationID)
			} else {
				h.logger.Error("batch taskexecution failed", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID), zap.Error(runErr))
				errorMsg := "execution failed: " + runErr.Error()
				// updateassistantmessage content
				if assistantMessageID != "" {
					if _, updateErr := h.db.Exec(
						"UPDATE messages SET content = ? WHERE id = ?",
						errorMsg,
						assistantMessageID,
					); updateErr != nil {
						h.logger.Warn("updatefailedofassistantmessagefailed", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(updateErr))
					}
					if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "error", errorMsg, nil); err != nil {
						h.logger.Warn("failed to save error details", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(err))
					}
				}
				h.batchTaskManager.UpdateTaskStatus(queueID, task.ID, "failed", "", runErr.Error())
			}
		} else {
			h.logger.Info("batch taskexecutesuccessful", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID))

			var resText string
			var mcpIDs []string
			var lastIn, lastOut string
			if useRunResult {
				resText = resultMA.Response
				mcpIDs = resultMA.MCPExecutionIDs
				lastIn = resultMA.LastReActInput
				lastOut = resultMA.LastReActOutput
			} else {
				resText = result.Response
				mcpIDs = result.MCPExecutionIDs
				lastIn = result.LastReActInput
				lastOut = result.LastReActOutput
			}

			// updateassistantmessage content
			if assistantMessageID != "" {
				mcpIDsJSON := ""
				if len(mcpIDs) > 0 {
					jsonData, _ := json.Marshal(mcpIDs)
					mcpIDsJSON = string(jsonData)
				}
				if _, updateErr := h.db.Exec(
					"UPDATE messages SET content = ?, mcp_execution_ids = ? WHERE id = ?",
					resText,
					mcpIDsJSON,
					assistantMessageID,
				); updateErr != nil {
					h.logger.Warn("updateassistantmessagefailed", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(updateErr))
					_, err = h.db.AddMessage(conversationID, "assistant", resText, mcpIDs)
					if err != nil {
						h.logger.Error("failed to save assistant message", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID), zap.Error(err))
					}
				}
			} else {
				_, err = h.db.AddMessage(conversationID, "assistant", resText, mcpIDs)
				if err != nil {
					h.logger.Error("failed to save assistant message", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID), zap.Error(err))
				}
			}

			// saveReActdata
			if lastIn != "" || lastOut != "" {
				if err := h.db.SaveReActData(conversationID, lastIn, lastOut); err != nil {
					h.logger.Warn("failed to save ReAct data", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.Error(err))
				} else {
					h.logger.Info("saved ReAct data", zap.String("queueId", queueID), zap.String("taskId", task.ID), zap.String("conversationId", conversationID))
				}
			}
			h.batchTaskManager.UpdateTaskStatusWithConversationID(queueID, task.ID, "completed", resText, "", conversationID)
		}
		h.batchTaskManager.MoveToNextTask(queueID)
		queue, _ = h.batchTaskManager.GetBatchQueue(queueID)
		if queue.Status == "cancelled" || queue.Status == "paused" {
			break
		}
	}
}
func (h *AgentHandler) loadHistoryFromReActData(conversationID string) ([]agent.ChatMessage, error) {
	reactInputJSON, reactOutput, err := h.db.GetReActData(conversationID)
	if err != nil {
		return nil, fmt.Errorf("getReActdatafailed: %w", err)
	}
	if reactInputJSON == "" {
		return nil, fmt.Errorf("ReActdatais,usemessage")
	}

	dataSource := "database_last_react_input"
	var messagesArray []map[string]interface{}
	if err := json.Unmarshal([]byte(reactInputJSON), &messagesArray); err != nil {
		return nil, fmt.Errorf("parseReActinputJSONfailed: %w", err)
	}

	messageCount := len(messagesArray)

	h.logger.Info("usesaveofReActdatarestore",
		zap.String("conversationId", conversationID),
		zap.String("dataSource", dataSource),
		zap.Int("reactInputSize", len(reactInputJSON)),
		zap.Int("messageCount", messageCount),
		zap.Int("reactOutputSize", len(reactOutput)),
	)
	// fmt.Println("messagesArray:", messagesArray)//debug
	agentMessages := make([]agent.ChatMessage, 0, len(messagesArray))
	for _, msgMap := range messagesArray {
		msg := agent.ChatMessage{}

		// parserole
		if role, ok := msgMap["role"].(string); ok {
			msg.Role = role
		} else {
			continue
		}
		if msg.Role == "system" {
			continue
		}

		// parsecontent
		if content, ok := msgMap["content"].(string); ok {
			msg.Content = content
		}

		// parsetool_calls(if exists)
		if toolCallsRaw, ok := msgMap["tool_calls"]; ok && toolCallsRaw != nil {
			if toolCallsArray, ok := toolCallsRaw.([]interface{}); ok {
				msg.ToolCalls = make([]agent.ToolCall, 0, len(toolCallsArray))
				for _, tcRaw := range toolCallsArray {
					if tcMap, ok := tcRaw.(map[string]interface{}); ok {
						toolCall := agent.ToolCall{}

						// parseID
						if id, ok := tcMap["id"].(string); ok {
							toolCall.ID = id
						}

						// parseType
						if toolType, ok := tcMap["type"].(string); ok {
							toolCall.Type = toolType
						}

						// parseFunction
						if funcMap, ok := tcMap["function"].(map[string]interface{}); ok {
							toolCall.Function = agent.FunctionCall{}
							if name, ok := funcMap["name"].(string); ok {
								toolCall.Function.Name = name
							}
							if argsRaw, ok := funcMap["arguments"]; ok {
								if argsStr, ok := argsRaw.(string); ok {
									var argsMap map[string]interface{}
									if err := json.Unmarshal([]byte(argsStr), &argsMap); err == nil {
										toolCall.Function.Arguments = argsMap
									}
								} else if argsMap, ok := argsRaw.(map[string]interface{}); ok {
									toolCall.Function.Arguments = argsMap
								}
							}
						}

						if toolCall.ID != "" {
							msg.ToolCalls = append(msg.ToolCalls, toolCall)
						}
					}
				}
			}
		}

		// parsetool_call_id(toolrolemessage)
		if toolCallID, ok := msgMap["tool_call_id"].(string); ok {
			msg.ToolCallID = toolCallID
		}

		agentMessages = append(agentMessages, msg)
	}
	if reactOutput != "" {
		if len(agentMessages) > 0 {
			lastMsg := &agentMessages[len(agentMessages)-1]
			if strings.EqualFold(lastMsg.Role, "assistant") && len(lastMsg.ToolCalls) == 0 {
				lastMsg.Content = reactOutput
			} else {
				agentMessages = append(agentMessages, agent.ChatMessage{
					Role: "assistant",
					Content: reactOutput,
				})
			}
		} else {
			agentMessages = append(agentMessages, agent.ChatMessage{
				Role: "assistant",
				Content: reactOutput,
			})
		}
	}

	if len(agentMessages) == 0 {
		return nil, fmt.Errorf("fromReActdataparseofmessageis")
	}
	if h.agent != nil {
		if fixed := h.agent.RepairOrphanToolMessages(&agentMessages); fixed {
			h.logger.Info("fromReActdatarestoreofmessageinoftoolmessage",
				zap.String("conversationId", conversationID),
			)
		}
	}

	h.logger.Info("fromReActdatarestoremessagecompleted",
		zap.String("conversationId", conversationID),
		zap.String("dataSource", dataSource),
		zap.Int("originalMessageCount", messageCount),
		zap.Int("finalMessageCount", len(agentMessages)),
		zap.Bool("hasReactOutput", reactOutput != ""),
	)
	fmt.Println("agentMessages:", agentMessages) //debug
	return agentMessages, nil
}
