package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"pyntra/internal/agents"
	"pyntra/internal/config"
	"pyntra/internal/knowledge"
	"pyntra/internal/mcp"
	"pyntra/internal/openai"
	"pyntra/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)
type KnowledgeToolRegistrar func() error
type VulnerabilityToolRegistrar func() error
type WebshellToolRegistrar func() error
type SkillsToolRegistrar func() error
type BatchTaskToolRegistrar func() error
type RetrieverUpdater interface {
	UpdateConfig(config *knowledge.RetrievalConfig)
}
type KnowledgeInitializer func() (*KnowledgeHandler, error)
type AppUpdater interface {
	UpdateKnowledgeComponents(handler *KnowledgeHandler, manager interface{}, retriever interface{}, indexer interface{})
}
// ConfigHandler configureprocessor
type ConfigHandler struct {
	configPath string
	config *config.Config
	mcpServer *mcp.Server
	executor *security.Executor
	agent AgentUpdater // Agentinterface,used forupdateAgentconfigure
	attackChainHandler AttackChainUpdater // attack chainprocessorinterface,used forupdateconfigure
	externalMCPMgr *mcp.ExternalMCPManager // externalMCPmanagedevice/processor
	knowledgeToolRegistrar KnowledgeToolRegistrar
	vulnerabilityToolRegistrar VulnerabilityToolRegistrar
	webshellToolRegistrar WebshellToolRegistrar
	skillsToolRegistrar SkillsToolRegistrar
	batchTaskToolRegistrar BatchTaskToolRegistrar // batch task MCP tool(optional)
	retrieverUpdater RetrieverUpdater
	knowledgeInitializer KnowledgeInitializer
	appUpdater AppUpdater // Appupdatedevice/processor(optional)
	logger *zap.Logger
	mu sync.RWMutex
	lastEmbeddingConfig *config.EmbeddingConfig
}

// AttackChainUpdater attack chainprocessorupdateinterface
type AttackChainUpdater interface {
	UpdateConfig(cfg *config.OpenAIConfig)
}

// AgentUpdater Agentupdateinterface
type AgentUpdater interface {
	UpdateConfig(cfg *config.OpenAIConfig)
	UpdateMaxIterations(maxIterations int)
}
func NewConfigHandler(configPath string, cfg *config.Config, mcpServer *mcp.Server, executor *security.Executor, agent AgentUpdater, attackChainHandler AttackChainUpdater, externalMCPMgr *mcp.ExternalMCPManager, logger *zap.Logger) *ConfigHandler {
	var lastEmbeddingConfig *config.EmbeddingConfig
	if cfg.Knowledge.Enabled {
		lastEmbeddingConfig = &config.EmbeddingConfig{
			Provider: cfg.Knowledge.Embedding.Provider,
			Model: cfg.Knowledge.Embedding.Model,
			BaseURL: cfg.Knowledge.Embedding.BaseURL,
			APIKey: cfg.Knowledge.Embedding.APIKey,
		}
	}
	return &ConfigHandler{
		configPath: configPath,
		config: cfg,
		mcpServer: mcpServer,
		executor: executor,
		agent: agent,
		attackChainHandler: attackChainHandler,
		externalMCPMgr: externalMCPMgr,
		logger: logger,
		lastEmbeddingConfig: lastEmbeddingConfig,
	}
}
func (h *ConfigHandler) SetKnowledgeToolRegistrar(registrar KnowledgeToolRegistrar) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.knowledgeToolRegistrar = registrar
}
func (h *ConfigHandler) SetVulnerabilityToolRegistrar(registrar VulnerabilityToolRegistrar) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.vulnerabilityToolRegistrar = registrar
}
func (h *ConfigHandler) SetWebshellToolRegistrar(registrar WebshellToolRegistrar) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.webshellToolRegistrar = registrar
}
func (h *ConfigHandler) SetSkillsToolRegistrar(registrar SkillsToolRegistrar) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.skillsToolRegistrar = registrar
}
func (h *ConfigHandler) SetBatchTaskToolRegistrar(registrar BatchTaskToolRegistrar) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.batchTaskToolRegistrar = registrar
}
func (h *ConfigHandler) SetRetrieverUpdater(updater RetrieverUpdater) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retrieverUpdater = updater
}
func (h *ConfigHandler) SetKnowledgeInitializer(initializer KnowledgeInitializer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.knowledgeInitializer = initializer
}
func (h *ConfigHandler) SetAppUpdater(updater AppUpdater) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appUpdater = updater
}
// GetConfigResponse getconfigureresponse
type GetConfigResponse struct {
	OpenAI config.OpenAIConfig `json:"openai"`
	MCP config.MCPConfig `json:"mcp"`
	Tools []ToolConfigInfo `json:"tools"`
	Agent config.AgentConfig `json:"agent"`
	Knowledge config.KnowledgeConfig `json:"knowledge"`
	MultiAgent config.MultiAgentPublic `json:"multi_agent,omitempty"`
}

// ToolConfigInfo toolconfigureinformation
type ToolConfigInfo struct {
	Name string `json:"name"`
	Description string `json:"description"`
	Enabled bool `json:"enabled"`
	IsExternal bool `json:"is_external,omitempty"`
	ExternalMCP string `json:"external_mcp,omitempty"`
	RoleEnabled *bool `json:"role_enabled,omitempty"`
}

// GetConfig getcurrentconfigure
func (h *ConfigHandler) GetConfig(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	configToolMap := make(map[string]bool)
	tools := make([]ToolConfigInfo, 0, len(h.config.Security.Tools))
	for _, tool := range h.config.Security.Tools {
		configToolMap[tool.Name] = true
		tools = append(tools, ToolConfigInfo{
			Name: tool.Name,
			Description: h.pickToolDescription(tool.ShortDescription, tool.Description),
			Enabled: tool.Enabled,
			IsExternal: false,
		})
	}
	if h.mcpServer != nil {
		mcpTools := h.mcpServer.GetAllTools()
		for _, mcpTool := range mcpTools {
			if configToolMap[mcpTool.Name] {
				continue
			}
			description := mcpTool.ShortDescription
			if description == "" {
				description = mcpTool.Description
			}
			if len(description) > 10000 {
				description = description[:10000] + "..."
			}
			tools = append(tools, ToolConfigInfo{
				Name: mcpTool.Name,
				Description: description,
				Enabled: true,
				IsExternal: false,
			})
		}
	}

	// getexternalMCPtool
	if h.externalMCPMgr != nil {
		ctx := context.Background()
		externalTools := h.getExternalMCPTools(ctx)
		for _, toolInfo := range externalTools {
			tools = append(tools, toolInfo)
		}
	}

	subAgentCount := len(h.config.MultiAgent.SubAgents)
	agentsDir := strings.TrimSpace(h.config.AgentsDir)
	if agentsDir == "" {
		agentsDir = "agents"
	}
	if !filepath.IsAbs(agentsDir) {
		agentsDir = filepath.Join(filepath.Dir(h.configPath), agentsDir)
	}
	if load, err := agents.LoadMarkdownAgentsDir(agentsDir); err == nil {
		subAgentCount = len(agents.MergeYAMLAndMarkdown(h.config.MultiAgent.SubAgents, load.SubAgents))
	}
	multiPub := config.MultiAgentPublic{
		Enabled: h.config.MultiAgent.Enabled,
		DefaultMode: h.config.MultiAgent.DefaultMode,
		BatchUseMultiAgent: h.config.MultiAgent.BatchUseMultiAgent,
		SubAgentCount: subAgentCount,
		Orchestration: config.NormalizeMultiAgentOrchestration(h.config.MultiAgent.Orchestration),
		PlanExecuteLoopMaxIterations: h.config.MultiAgent.PlanExecuteLoopMaxIterations,
	}
	if strings.TrimSpace(multiPub.DefaultMode) == "" {
		multiPub.DefaultMode = "single"
	}

	// Redact secrets before sending config to the client. The real api_key must
	// never leave the server; a non-empty key is replaced with apiKeyMask. On
	// save, an incoming value equal to the mask means "keep the stored key"
	// (see UpdateConfig / TestOpenAI).
	openaiRedacted := h.config.OpenAI
	if openaiRedacted.APIKey != "" {
		openaiRedacted.APIKey = apiKeyMask
	}
	knowledgeRedacted := h.config.Knowledge
	if knowledgeRedacted.Embedding.APIKey != "" {
		knowledgeRedacted.Embedding.APIKey = apiKeyMask
	}

	c.JSON(http.StatusOK, GetConfigResponse{
		OpenAI: openaiRedacted,
		MCP: h.config.MCP,
		Tools: tools,
		Agent: h.config.Agent,
		Knowledge: knowledgeRedacted,
		MultiAgent: multiPub,
	})
}

// apiKeyMask is the placeholder returned to clients in place of a stored API
// key. When the client sends this value back on save, the existing key is kept.
const apiKeyMask = "********"

// GetToolsResponse gettoollistresponse(pagination)
type GetToolsResponse struct {
	Tools []ToolConfigInfo `json:"tools"`
	Total int `json:"total"`
	TotalEnabled int `json:"total_enabled"` // hasenableoftooltotal
	Page int `json:"page"`
	PageSize int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

// GetTools gettoollist(supportpaginationandsearch)
func (h *ConfigHandler) GetTools(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// parsepaginationparameter
	page := 1
	pageSize := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// parsesearchparameter
	searchTerm := c.Query("search")
	searchTermLower := ""
	if searchTerm != "" {
		searchTermLower = strings.ToLower(searchTerm)
	}
	enabledFilter := c.Query("enabled")
	var filterEnabled *bool
	if enabledFilter == "true" {
		v := true
		filterEnabled = &v
	} else if enabledFilter == "false" {
		v := false
		filterEnabled = &v
	}
	roleName := c.Query("role")
	var roleToolsSet map[string]bool
	var roleUsesAllTools bool = true
	if roleName != "" && roleName != "default" && h.config.Roles != nil {
		if role, exists := h.config.Roles[roleName]; exists && role.Enabled {
			if len(role.Tools) > 0 {
				roleToolsSet = make(map[string]bool)
				for _, toolKey := range role.Tools {
					roleToolsSet[toolKey] = true
				}
				roleUsesAllTools = false
			}
		}
	}
	configToolMap := make(map[string]bool)
	allTools := make([]ToolConfigInfo, 0, len(h.config.Security.Tools))
	for _, tool := range h.config.Security.Tools {
		configToolMap[tool.Name] = true
		toolInfo := ToolConfigInfo{
			Name: tool.Name,
			Description: h.pickToolDescription(tool.ShortDescription, tool.Description),
			Enabled: tool.Enabled,
			IsExternal: false,
		}
		if roleName != "" {
			if roleUsesAllTools {
				if tool.Enabled {
					roleEnabled := true
					toolInfo.RoleEnabled = &roleEnabled
				} else {
					roleEnabled := false
					toolInfo.RoleEnabled = &roleEnabled
				}
			} else {
				if roleToolsSet[tool.Name] {
					roleEnabled := tool.Enabled
					toolInfo.RoleEnabled = &roleEnabled
				} else {
					roleEnabled := false
					toolInfo.RoleEnabled = &roleEnabled
				}
			}
		}
		if searchTermLower != "" {
			nameLower := strings.ToLower(toolInfo.Name)
			descLower := strings.ToLower(toolInfo.Description)
			if !strings.Contains(nameLower, searchTermLower) && !strings.Contains(descLower, searchTermLower) {
				continue
			}
		}

		// statusfilter
		if filterEnabled != nil && toolInfo.Enabled != *filterEnabled {
			continue
		}

		allTools = append(allTools, toolInfo)
	}
	if h.mcpServer != nil {
		mcpTools := h.mcpServer.GetAllTools()
		for _, mcpTool := range mcpTools {
			if configToolMap[mcpTool.Name] {
				continue
			}

			description := mcpTool.ShortDescription
			if description == "" {
				description = mcpTool.Description
			}
			if len(description) > 10000 {
				description = description[:10000] + "..."
			}

			toolInfo := ToolConfigInfo{
				Name: mcpTool.Name,
				Description: description,
				Enabled: true,
				IsExternal: false,
			}
			if roleName != "" {
				if roleUsesAllTools {
					roleEnabled := true
					toolInfo.RoleEnabled = &roleEnabled
				} else {
					if roleToolsSet[mcpTool.Name] {
						roleEnabled := true
						toolInfo.RoleEnabled = &roleEnabled
					} else {
						roleEnabled := false
						toolInfo.RoleEnabled = &roleEnabled
					}
				}
			}
			if searchTermLower != "" {
				nameLower := strings.ToLower(toolInfo.Name)
				descLower := strings.ToLower(toolInfo.Description)
				if !strings.Contains(nameLower, searchTermLower) && !strings.Contains(descLower, searchTermLower) {
					continue
				}
			}

			// statusfilter
			if filterEnabled != nil && toolInfo.Enabled != *filterEnabled {
				continue
			}

			allTools = append(allTools, toolInfo)
		}
	}

	// getexternalMCPtool
	if h.externalMCPMgr != nil {
		// createcontextused forgetexternaltool
		ctx := context.Background()
		externalTools := h.getExternalMCPTools(ctx)
		for _, toolInfo := range externalTools {
			if searchTermLower != "" {
				nameLower := strings.ToLower(toolInfo.Name)
				descLower := strings.ToLower(toolInfo.Description)
				if !strings.Contains(nameLower, searchTermLower) && !strings.Contains(descLower, searchTermLower) {
					continue
				}
			}
			if roleName != "" {
				if roleUsesAllTools {
					roleEnabled := toolInfo.Enabled
					toolInfo.RoleEnabled = &roleEnabled
				} else {
					externalToolKey := fmt.Sprintf("%s::%s", toolInfo.ExternalMCP, toolInfo.Name)
					if roleToolsSet[externalToolKey] {
						roleEnabled := toolInfo.Enabled
						toolInfo.RoleEnabled = &roleEnabled
					} else {
						roleEnabled := false
						toolInfo.RoleEnabled = &roleEnabled
					}
				}
			}

			// statusfilter
			if filterEnabled != nil && toolInfo.Enabled != *filterEnabled {
				continue
			}

			allTools = append(allTools, toolInfo)
		}
	}

	total := len(allTools)
	totalEnabled := 0
	for _, tool := range allTools {
		if tool.RoleEnabled != nil && *tool.RoleEnabled {
			totalEnabled++
		} else if tool.RoleEnabled == nil && tool.Enabled {
			totalEnabled++
		}
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if end > total {
		end = total
	}

	var tools []ToolConfigInfo
	if offset < total {
		tools = allTools[offset:end]
	} else {
		tools = []ToolConfigInfo{}
	}

	c.JSON(http.StatusOK, GetToolsResponse{
		Tools: tools,
		Total: total,
		TotalEnabled: totalEnabled,
		Page: page,
		PageSize: pageSize,
		TotalPages: totalPages,
	})
}

// UpdateConfigRequest updateconfigurerequest
type UpdateConfigRequest struct {
	OpenAI *config.OpenAIConfig `json:"openai,omitempty"`
	MCP *config.MCPConfig `json:"mcp,omitempty"`
	Tools []ToolEnableStatus `json:"tools,omitempty"`
	Agent *config.AgentConfig `json:"agent,omitempty"`
	Knowledge *config.KnowledgeConfig `json:"knowledge,omitempty"`
	MultiAgent *config.MultiAgentAPIUpdate `json:"multi_agent,omitempty"`
}

// ToolEnableStatus toolenablestatus
type ToolEnableStatus struct {
	Name string `json:"name"`
	Enabled bool `json:"enabled"`
	IsExternal bool `json:"is_external,omitempty"`
	ExternalMCP string `json:"external_mcp,omitempty"`
}

// UpdateConfig updateconfigure
func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// updateOpenAIconfigure
	if req.OpenAI != nil {
		// Refuse to store a config that would send the API key in cleartext to a
		// remote host (plain http to a non-local address).
		if config.IsInsecureRemoteBaseURL(req.OpenAI.BaseURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "insecure base_url: refusing to send the API key over plain http to a remote host — use https:// (http is allowed only for localhost/private endpoints)"})
			return
		}
		// A masked api_key means the client never saw the real one; keep it.
		if req.OpenAI.APIKey == apiKeyMask {
			req.OpenAI.APIKey = h.config.OpenAI.APIKey
		}
		h.config.OpenAI = *req.OpenAI
		h.logger.Info("updateOpenAIconfigure",
			zap.String("base_url", h.config.OpenAI.BaseURL),
			zap.String("model", h.config.OpenAI.Model),
		)
	}

	// updateMCPconfigure
	if req.MCP != nil {
		h.config.MCP = *req.MCP
		h.logger.Info("updateMCPconfigure",
			zap.Bool("enabled", h.config.MCP.Enabled),
			zap.String("host", h.config.MCP.Host),
			zap.Int("port", h.config.MCP.Port),
		)
	}

	// updateAgentconfigure
	if req.Agent != nil {
		h.config.Agent = *req.Agent
		h.logger.Info("updateAgentconfigure",
			zap.Int("max_iterations", h.config.Agent.MaxIterations),
		)
	}

	// updateKnowledgeconfigure
	if req.Knowledge != nil {
		// A masked embedding api_key means the client never saw the real one; keep it.
		if req.Knowledge.Embedding.APIKey == apiKeyMask {
			req.Knowledge.Embedding.APIKey = h.config.Knowledge.Embedding.APIKey
		}
		if h.config.Knowledge.Enabled {
			h.lastEmbeddingConfig = &config.EmbeddingConfig{
				Provider: h.config.Knowledge.Embedding.Provider,
				Model: h.config.Knowledge.Embedding.Model,
				BaseURL: h.config.Knowledge.Embedding.BaseURL,
				APIKey: h.config.Knowledge.Embedding.APIKey,
			}
		}
		h.config.Knowledge = *req.Knowledge
		h.logger.Info("updateKnowledgeconfigure",
			zap.Bool("enabled", h.config.Knowledge.Enabled),
			zap.String("base_path", h.config.Knowledge.BasePath),
			zap.String("embedding_model", h.config.Knowledge.Embedding.Model),
			zap.Int("retrieval_top_k", h.config.Knowledge.Retrieval.TopK),
			zap.Float64("similarity_threshold", h.config.Knowledge.Retrieval.SimilarityThreshold),
		)
	}
	if req.MultiAgent != nil {
		h.config.MultiAgent.Enabled = req.MultiAgent.Enabled
		dm := strings.TrimSpace(req.MultiAgent.DefaultMode)
		if dm == "multi" || dm == "single" {
			h.config.MultiAgent.DefaultMode = dm
		}
		h.config.MultiAgent.BatchUseMultiAgent = req.MultiAgent.BatchUseMultiAgent
		if req.MultiAgent.PlanExecuteLoopMaxIterations != nil {
			h.config.MultiAgent.PlanExecuteLoopMaxIterations = *req.MultiAgent.PlanExecuteLoopMaxIterations
		}
		h.logger.Info("updatemulti-agentconfigure",
			zap.Bool("enabled", h.config.MultiAgent.Enabled),
			zap.String("default_mode", h.config.MultiAgent.DefaultMode),
			zap.Bool("batch_use_multi_agent", h.config.MultiAgent.BatchUseMultiAgent),
			zap.Int("plan_execute_loop_max_iterations", h.config.MultiAgent.PlanExecuteLoopMaxIterations),
		)
	}

	// updatetoolenablestatus
	if req.Tools != nil {
		internalToolMap := make(map[string]bool)
		// externaltoolstatus:MCPname -> tool name -> enablestatus
		externalMCPToolMap := make(map[string]map[string]bool)

		for _, toolStatus := range req.Tools {
			if toolStatus.IsExternal && toolStatus.ExternalMCP != "" {
				mcpName := toolStatus.ExternalMCP
				if externalMCPToolMap[mcpName] == nil {
					externalMCPToolMap[mcpName] = make(map[string]bool)
				}
				externalMCPToolMap[mcpName][toolStatus.Name] = toolStatus.Enabled
			} else {
				internalToolMap[toolStatus.Name] = toolStatus.Enabled
			}
		}
		for i := range h.config.Security.Tools {
			if enabled, ok := internalToolMap[h.config.Security.Tools[i].Name]; ok {
				h.config.Security.Tools[i].Enabled = enabled
				h.logger.Info("updatetoolenablestatus",
					zap.String("tool", h.config.Security.Tools[i].Name),
					zap.Bool("enabled", enabled),
				)
			}
		}

		// updateexternalMCPtoolstatus
		if h.externalMCPMgr != nil {
			for mcpName, toolStates := range externalMCPToolMap {
				// updateconfigureinoftoolenablestatus
				if h.config.ExternalMCP.Servers == nil {
					h.config.ExternalMCP.Servers = make(map[string]config.ExternalMCPServerConfig)
				}
				cfg, exists := h.config.ExternalMCP.Servers[mcpName]
				if !exists {
					h.logger.Warn("externalMCPconfiguredoes not exist", zap.String("mcp", mcpName))
					continue
				}
				if cfg.ToolEnabled == nil {
					cfg.ToolEnabled = make(map[string]bool)
				}

				// updateeach/peritemstoolofenablestatus
				for toolName, enabled := range toolStates {
					cfg.ToolEnabled[toolName] = enabled
					h.logger.Info("updateexternaltoolenablestatus",
						zap.String("mcp", mcpName),
						zap.String("tool", toolName),
						zap.Bool("enabled", enabled),
					)
				}
				hasEnabledTool := false
				for _, enabled := range cfg.ToolEnabled {
					if enabled {
						hasEnabledTool = true
						break
					}
				}
				if !cfg.ExternalMCPEnable && hasEnabledTool {
					cfg.ExternalMCPEnable = true
					h.logger.Info("enableexternalMCP(istoolenable)", zap.String("mcp", mcpName))
				}

				h.config.ExternalMCP.Servers[mcpName] = cfg
			}
			h.externalMCPMgr.LoadConfigs(&h.config.ExternalMCP)
			for mcpName := range externalMCPToolMap {
				cfg := h.config.ExternalMCP.Servers[mcpName]
				if cfg.ExternalMCPEnable {
					client, exists := h.externalMCPMgr.GetClient(mcpName)
					if !exists || !client.IsConnected() {
						go func(name string) {
							if err := h.externalMCPMgr.StartClient(name); err != nil {
								h.logger.Warn("start externalMCPfailed",
									zap.String("mcp", name),
									zap.Error(err),
								)
							} else {
								h.logger.Info("start externalMCP",
									zap.String("mcp", name),
								)
							}
						}(mcpName)
					}
				}
			}
		}
	}

	// saveconfigureto file
	if err := h.saveConfig(); err != nil {
		h.logger.Error("saveconfigurefailed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saveconfigurefailed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "configurehasupdate"})
}
type TestOpenAIRequest struct {
	Provider string `json:"provider"`
	BaseURL string `json:"base_url"`
	APIKey string `json:"api_key"`
	Model string `json:"model"`
}
func (h *ConfigHandler) TestOpenAI(c *gin.Context) {
	var req TestOpenAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	// If the client sent the mask, it never saw the real key — test with the stored one.
	if strings.TrimSpace(req.APIKey) == apiKeyMask {
		h.mu.RLock()
		req.APIKey = h.config.OpenAI.APIKey
		h.mu.RUnlock()
	}

	if strings.TrimSpace(req.APIKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API Key cannot be empty"})
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "modelcannot be empty"})
		return
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		if strings.EqualFold(strings.TrimSpace(req.Provider), "claude") {
			baseURL = "https://api.anthropic.com"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}
	if config.IsInsecureRemoteBaseURL(baseURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "insecure base_url: refusing to send the API key over plain http to a remote host — use https://"})
		return
	}
	payload := map[string]interface{}{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "Hi"},
		},
		"max_tokens": 5,
	}
	tmpCfg := &config.OpenAIConfig{
		Provider: req.Provider,
		BaseURL: baseURL,
		APIKey: strings.TrimSpace(req.APIKey),
		Model: req.Model,
	}
	client := openai.NewClient(tmpCfg, nil, h.logger)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	start := time.Now()
	var chatResp struct {
		ID string `json:"id"`
		Object string `json:"object"`
		Model string `json:"model"`
		Choices []struct {
			Message struct {
				Role string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	err := client.ChatCompletion(ctx, payload, &chatResp)
	latency := time.Since(start)

	if err != nil {
		if apiErr, ok := err.(*openai.APIError); ok {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"error": fmt.Sprintf("API returnerror (HTTP %d): %s", apiErr.StatusCode, apiErr.Body),
				"status_code": apiErr.StatusCode,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error": "connectionfailed: " + err.Error(),
		})
		return
	}
	if len(chatResp.Choices) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error": "API response choices field,check Base URL path",
		})
		return
	}
	if chatResp.ID == "" && chatResp.Model == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error": "API responseFormat,check Base URL ",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"model": chatResp.Model,
		"latency_ms": latency.Milliseconds(),
	})
}
func (h *ConfigHandler) ApplyConfig(c *gin.Context) {
	var needInitKnowledge bool
	var knowledgeInitializer KnowledgeInitializer

	h.mu.RLock()
	needInitKnowledge = h.config.Knowledge.Enabled && h.knowledgeToolRegistrar == nil && h.knowledgeInitializer != nil
	if needInitKnowledge {
		knowledgeInitializer = h.knowledgeInitializer
	}
	h.mu.RUnlock()
	if needInitKnowledge {
		h.logger.Info("knowledge basefromdisabledisenable,startinitializeknowledge base")
		if _, err := knowledgeInitializer(); err != nil {
			h.logger.Error("initializeknowledge basefailed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "initializeknowledge basefailed: " + err.Error()})
			return
		}
		h.logger.Info("knowledge baseinitializecompleted,toolhasregister")
	}
	var needReinitKnowledge bool
	var reinitKnowledgeInitializer KnowledgeInitializer
	h.mu.RLock()
	if h.config.Knowledge.Enabled && h.knowledgeInitializer != nil && h.lastEmbeddingConfig != nil {
		currentEmbedding := h.config.Knowledge.Embedding
		if currentEmbedding.Provider != h.lastEmbeddingConfig.Provider ||
			currentEmbedding.Model != h.lastEmbeddingConfig.Model ||
			currentEmbedding.BaseURL != h.lastEmbeddingConfig.BaseURL ||
			currentEmbedding.APIKey != h.lastEmbeddingConfig.APIKey {
			needReinitKnowledge = true
			reinitKnowledgeInitializer = h.knowledgeInitializer
			h.logger.Info("embedding modelconfiguremore,needinitializeknowledge base",
				zap.String("old_model", h.lastEmbeddingConfig.Model),
				zap.String("new_model", currentEmbedding.Model),
				zap.String("old_base_url", h.lastEmbeddingConfig.BaseURL),
				zap.String("new_base_url", currentEmbedding.BaseURL),
			)
		}
	}
	h.mu.RUnlock()
	if needReinitKnowledge {
		h.logger.Info("startinitializeknowledge base(embedding modelconfigurehasmore)")
		if _, err := reinitKnowledgeInitializer(); err != nil {
			h.logger.Error("initializeknowledge basefailed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "initializeknowledge basefailed: " + err.Error()})
			return
		}
		h.logger.Info("knowledge baseinitializecompleted")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if needReinitKnowledge && h.config.Knowledge.Enabled {
		h.lastEmbeddingConfig = &config.EmbeddingConfig{
			Provider: h.config.Knowledge.Embedding.Provider,
			Model: h.config.Knowledge.Embedding.Model,
			BaseURL: h.config.Knowledge.Embedding.BaseURL,
			APIKey: h.config.Knowledge.Embedding.APIKey,
		}
		h.logger.Info("hasupdateembedding modelconfigurerecord")
	}
	h.logger.Info("reregistertool")

	// clearMCPserverinoftool
	h.mcpServer.ClearTools()
	h.executor.RegisterTools(h.mcpServer)
	if h.vulnerabilityToolRegistrar != nil {
		h.logger.Info("reregistervulnerabilityrecordtool")
		if err := h.vulnerabilityToolRegistrar(); err != nil {
			h.logger.Error("reregistervulnerabilityrecordtoolfailed", zap.Error(err))
		} else {
			h.logger.Info("vulnerabilityrecordtoolhasreregister")
		}
	}
	if h.webshellToolRegistrar != nil {
		h.logger.Info("reregister WebShell tool")
		if err := h.webshellToolRegistrar(); err != nil {
			h.logger.Error("reregister WebShell toolfailed", zap.Error(err))
		} else {
			h.logger.Info("WebShell toolhasreregister")
		}
	}
	if h.skillsToolRegistrar != nil {
		h.logger.Info("reregisterSkillstool")
		if err := h.skillsToolRegistrar(); err != nil {
			h.logger.Error("reregisterSkillstoolfailed", zap.Error(err))
		} else {
			h.logger.Info("Skillstoolhasreregister")
		}
	}
	if h.batchTaskToolRegistrar != nil {
		h.logger.Info("reregisterbatch task MCP tool")
		if err := h.batchTaskToolRegistrar(); err != nil {
			h.logger.Error("reregisterbatch task MCP toolfailed", zap.Error(err))
		} else {
			h.logger.Info("batch task MCP toolhasreregister")
		}
	}
	if h.config.Knowledge.Enabled && h.knowledgeToolRegistrar != nil {
		h.logger.Info("reregisterknowledge basetool")
		if err := h.knowledgeToolRegistrar(); err != nil {
			h.logger.Error("reregisterknowledge basetoolfailed", zap.Error(err))
		} else {
			h.logger.Info("knowledge basetoolhasreregister")
		}
	}

	// updateAgentofOpenAIconfigure
	if h.agent != nil {
		h.agent.UpdateConfig(&h.config.OpenAI)
		h.agent.UpdateMaxIterations(h.config.Agent.MaxIterations)
		h.logger.Info("Agentconfigurehasupdate")
	}

	// updateAttackChainHandlerofOpenAIconfigure
	if h.attackChainHandler != nil {
		h.attackChainHandler.UpdateConfig(&h.config.OpenAI)
		h.logger.Info("AttackChainHandlerconfigurehasupdate")
	}
	if h.config.Knowledge.Enabled && h.retrieverUpdater != nil {
		retrievalConfig := &knowledge.RetrievalConfig{
			TopK: h.config.Knowledge.Retrieval.TopK,
			SimilarityThreshold: h.config.Knowledge.Retrieval.SimilarityThreshold,
			SubIndexFilter: h.config.Knowledge.Retrieval.SubIndexFilter,
			PostRetrieve: h.config.Knowledge.Retrieval.PostRetrieve,
		}
		h.retrieverUpdater.UpdateConfig(retrievalConfig)
		h.logger.Info("retrievaldevice/processorconfigurehasupdate",
			zap.Int("top_k", retrievalConfig.TopK),
			zap.Float64("similarity_threshold", retrievalConfig.SimilarityThreshold),
		)
	}
	if h.config.Knowledge.Enabled {
		h.lastEmbeddingConfig = &config.EmbeddingConfig{
			Provider: h.config.Knowledge.Embedding.Provider,
			Model: h.config.Knowledge.Embedding.Model,
			BaseURL: h.config.Knowledge.Embedding.BaseURL,
			APIKey: h.config.Knowledge.Embedding.APIKey,
		}
	}
	h.logger.Info("configurehasapply",
		zap.Int("tools_count", len(h.config.Security.Tools)),
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "configurehasapply",
		"tools_count": len(h.config.Security.Tools),
	})
}

// saveConfig saveconfigureto file
func (h *ConfigHandler) saveConfig() error {
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	if err := os.WriteFile(h.configPath+".backup", data, 0644); err != nil {
		h.logger.Warn("createconfigurebackupfailed", zap.Error(err))
	}

	root, err := loadYAMLDocument(h.configPath)
	if err != nil {
		return fmt.Errorf("parseconfigurefilefailed: %w", err)
	}

	updateAgentConfig(root, h.config.Agent.MaxIterations)
	updateMCPConfig(root, h.config.MCP)
	updateOpenAIConfig(root, h.config.OpenAI)
	updateKnowledgeConfig(root, h.config.Knowledge)
	updateMultiAgentConfig(root, h.config.MultiAgent)
	originalConfigs := make(map[string]map[string]bool)
	externalMCPNode := findMapValue(root, "external_mcp")
	if externalMCPNode != nil && externalMCPNode.Kind == yaml.MappingNode {
		serversNode := findMapValue(externalMCPNode, "servers")
		if serversNode != nil && serversNode.Kind == yaml.MappingNode {
			for i := 0; i < len(serversNode.Content); i += 2 {
				if i+1 >= len(serversNode.Content) {
					break
				}
				nameNode := serversNode.Content[i]
				serverNode := serversNode.Content[i+1]
				if nameNode.Kind == yaml.ScalarNode && serverNode.Kind == yaml.MappingNode {
					serverName := nameNode.Value
					originalConfigs[serverName] = make(map[string]bool)
					if enabledVal := findBoolInMap(serverNode, "enabled"); enabledVal != nil {
						originalConfigs[serverName]["enabled"] = *enabledVal
					}
					if disabledVal := findBoolInMap(serverNode, "disabled"); disabledVal != nil {
						originalConfigs[serverName]["disabled"] = *disabledVal
					}
				}
			}
		}
	}
	updateExternalMCPConfig(root, h.config.ExternalMCP, originalConfigs)

	if err := writeYAMLDocument(h.configPath, root); err != nil {
		return fmt.Errorf("saveconfigurefilefailed: %w", err)
	}

	// updatetoolconfigurefileinofenabledstatus
	if h.config.Security.ToolsDir != "" {
		configDir := filepath.Dir(h.configPath)
		toolsDir := h.config.Security.ToolsDir
		if !filepath.IsAbs(toolsDir) {
			toolsDir = filepath.Join(configDir, toolsDir)
		}

		for _, tool := range h.config.Security.Tools {
			toolFile := filepath.Join(toolsDir, tool.Name+".yaml")
			if _, err := os.Stat(toolFile); os.IsNotExist(err) {
				toolFile = filepath.Join(toolsDir, tool.Name+".yml")
				if _, err := os.Stat(toolFile); os.IsNotExist(err) {
					h.logger.Warn("toolconfigurefiledoes not exist", zap.String("tool", tool.Name))
					continue
				}
			}

			toolDoc, err := loadYAMLDocument(toolFile)
			if err != nil {
				h.logger.Warn("parsetoolconfigurefailed", zap.String("tool", tool.Name), zap.Error(err))
				continue
			}

			setBoolInMap(toolDoc.Content[0], "enabled", tool.Enabled)

			if err := writeYAMLDocument(toolFile, toolDoc); err != nil {
				h.logger.Warn("savetoolconfigurefilefailed", zap.String("tool", tool.Name), zap.Error(err))
				continue
			}

			h.logger.Info("updatetoolconfigure", zap.String("tool", tool.Name), zap.Bool("enabled", tool.Enabled))
		}
	}

	h.logger.Info("configurehassave", zap.String("path", h.configPath))
	return nil
}

func loadYAMLDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return newEmptyYAMLDocument(), nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return newEmptyYAMLDocument(), nil
	}

	if doc.Content[0].Kind != yaml.MappingNode {
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Content = []*yaml.Node{root}
	}

	return &doc, nil
}

func newEmptyYAMLDocument() *yaml.Node {
	root := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
	}
	return root
}

func writeYAMLDocument(path string, doc *yaml.Node) error {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func updateAgentConfig(doc *yaml.Node, maxIterations int) {
	root := doc.Content[0]
	agentNode := ensureMap(root, "agent")
	setIntInMap(agentNode, "max_iterations", maxIterations)
}

func updateMCPConfig(doc *yaml.Node, cfg config.MCPConfig) {
	root := doc.Content[0]
	mcpNode := ensureMap(root, "mcp")
	setBoolInMap(mcpNode, "enabled", cfg.Enabled)
	setStringInMap(mcpNode, "host", cfg.Host)
	setIntInMap(mcpNode, "port", cfg.Port)
}

func updateOpenAIConfig(doc *yaml.Node, cfg config.OpenAIConfig) {
	root := doc.Content[0]
	openaiNode := ensureMap(root, "openai")
	if cfg.Provider != "" {
		setStringInMap(openaiNode, "provider", cfg.Provider)
	}
	setStringInMap(openaiNode, "api_key", cfg.APIKey)
	setStringInMap(openaiNode, "base_url", cfg.BaseURL)
	setStringInMap(openaiNode, "model", cfg.Model)
	if cfg.MaxTotalTokens > 0 {
		setIntInMap(openaiNode, "max_total_tokens", cfg.MaxTotalTokens)
	}
}

func updateKnowledgeConfig(doc *yaml.Node, cfg config.KnowledgeConfig) {
	root := doc.Content[0]
	knowledgeNode := ensureMap(root, "knowledge")
	setBoolInMap(knowledgeNode, "enabled", cfg.Enabled)
	setStringInMap(knowledgeNode, "base_path", cfg.BasePath)
	embeddingNode := ensureMap(knowledgeNode, "embedding")
	setStringInMap(embeddingNode, "provider", cfg.Embedding.Provider)
	setStringInMap(embeddingNode, "model", cfg.Embedding.Model)
	if cfg.Embedding.BaseURL != "" {
		setStringInMap(embeddingNode, "base_url", cfg.Embedding.BaseURL)
	}
	if cfg.Embedding.APIKey != "" {
		setStringInMap(embeddingNode, "api_key", cfg.Embedding.APIKey)
	}
	retrievalNode := ensureMap(knowledgeNode, "retrieval")
	setIntInMap(retrievalNode, "top_k", cfg.Retrieval.TopK)
	setFloatInMap(retrievalNode, "similarity_threshold", cfg.Retrieval.SimilarityThreshold)
	setStringInMap(retrievalNode, "sub_index_filter", cfg.Retrieval.SubIndexFilter)
	postNode := ensureMap(retrievalNode, "post_retrieve")
	setIntInMap(postNode, "prefetch_top_k", cfg.Retrieval.PostRetrieve.PrefetchTopK)
	setIntInMap(postNode, "max_context_chars", cfg.Retrieval.PostRetrieve.MaxContextChars)
	setIntInMap(postNode, "max_context_tokens", cfg.Retrieval.PostRetrieve.MaxContextTokens)
	indexingNode := ensureMap(knowledgeNode, "indexing")
	setStringInMap(indexingNode, "chunk_strategy", cfg.Indexing.ChunkStrategy)
	setIntInMap(indexingNode, "request_timeout_seconds", cfg.Indexing.RequestTimeoutSeconds)
	setIntInMap(indexingNode, "chunk_size", cfg.Indexing.ChunkSize)
	setIntInMap(indexingNode, "chunk_overlap", cfg.Indexing.ChunkOverlap)
	setIntInMap(indexingNode, "max_chunks_per_item", cfg.Indexing.MaxChunksPerItem)
	setBoolInMap(indexingNode, "prefer_source_file", cfg.Indexing.PreferSourceFile)
	setIntInMap(indexingNode, "batch_size", cfg.Indexing.BatchSize)
	setStringSliceInMap(indexingNode, "sub_indexes", cfg.Indexing.SubIndexes)
	setIntInMap(indexingNode, "max_rpm", cfg.Indexing.MaxRPM)
	setIntInMap(indexingNode, "rate_limit_delay_ms", cfg.Indexing.RateLimitDelayMs)
	setIntInMap(indexingNode, "max_retries", cfg.Indexing.MaxRetries)
	setIntInMap(indexingNode, "retry_delay_ms", cfg.Indexing.RetryDelayMs)
}

func updateMultiAgentConfig(doc *yaml.Node, cfg config.MultiAgentConfig) {
	root := doc.Content[0]
	maNode := ensureMap(root, "multi_agent")
	setBoolInMap(maNode, "enabled", cfg.Enabled)
	setStringInMap(maNode, "default_mode", cfg.DefaultMode)
	setBoolInMap(maNode, "batch_use_multi_agent", cfg.BatchUseMultiAgent)
	setIntInMap(maNode, "plan_execute_loop_max_iterations", cfg.PlanExecuteLoopMaxIterations)
}

func ensureMap(parent *yaml.Node, path ...string) *yaml.Node {
	current := parent
	for _, key := range path {
		value := findMapValue(current, key)
		if value == nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			mapNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			current.Content = append(current.Content, keyNode, mapNode)
			value = mapNode
		}

		if value.Kind != yaml.MappingNode {
			value.Kind = yaml.MappingNode
			value.Tag = "!!map"
			value.Style = 0
			value.Content = nil
		}

		current = value
	}

	return current
}

func findMapValue(mapNode *yaml.Node, key string) *yaml.Node {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}

func ensureKeyValue(mapNode *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	for i := 0; i < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i], mapNode.Content[i+1]
		}
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{}
	mapNode.Content = append(mapNode.Content, keyNode, valueNode)
	return keyNode, valueNode
}

func setStringInMap(mapNode *yaml.Node, key, value string) {
	_, valueNode := ensureKeyValue(mapNode, key)
	valueNode.Kind = yaml.ScalarNode
	valueNode.Tag = "!!str"
	valueNode.Style = 0
	valueNode.Value = value
}

func setStringSliceInMap(mapNode *yaml.Node, key string, values []string) {
	_, valueNode := ensureKeyValue(mapNode, key)
	valueNode.Kind = yaml.SequenceNode
	valueNode.Tag = "!!seq"
	valueNode.Style = 0
	valueNode.Content = nil
	for _, v := range values {
		valueNode.Content = append(valueNode.Content, &yaml.Node{
			Kind: yaml.ScalarNode,
			Tag: "!!str",
			Value: v,
		})
	}
}

func setIntInMap(mapNode *yaml.Node, key string, value int) {
	_, valueNode := ensureKeyValue(mapNode, key)
	valueNode.Kind = yaml.ScalarNode
	valueNode.Tag = "!!int"
	valueNode.Style = 0
	valueNode.Value = fmt.Sprintf("%d", value)
}

func findBoolInMap(mapNode *yaml.Node, key string) *bool {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(mapNode.Content); i += 2 {
		if i+1 >= len(mapNode.Content) {
			break
		}
		keyNode := mapNode.Content[i]
		valueNode := mapNode.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == key {
			if valueNode.Kind == yaml.ScalarNode {
				if valueNode.Value == "true" {
					result := true
					return &result
				} else if valueNode.Value == "false" {
					result := false
					return &result
				}
			}
			return nil
		}
	}
	return nil
}

func setBoolInMap(mapNode *yaml.Node, key string, value bool) {
	_, valueNode := ensureKeyValue(mapNode, key)
	valueNode.Kind = yaml.ScalarNode
	valueNode.Tag = "!!bool"
	valueNode.Style = 0
	if value {
		valueNode.Value = "true"
	} else {
		valueNode.Value = "false"
	}
}

func setFloatInMap(mapNode *yaml.Node, key string, value float64) {
	_, valueNode := ensureKeyValue(mapNode, key)
	valueNode.Kind = yaml.ScalarNode
	valueNode.Tag = "!!float"
	valueNode.Style = 0
	if value >= 0.0 && value <= 1.0 {
		valueNode.Value = fmt.Sprintf("%.1f", value)
	} else {
		valueNode.Value = fmt.Sprintf("%g", value)
	}
}
func (h *ConfigHandler) getExternalMCPTools(ctx context.Context) []ToolConfigInfo {
	var result []ToolConfigInfo

	if h.externalMCPMgr == nil {
		return result
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	externalTools, err := h.externalMCPMgr.GetAllTools(timeoutCtx)
	if err != nil {
		h.logger.Warn("getexternalMCPtoolfailed(connection),returncacheoftool",
			zap.Error(err),
			zap.String("hint", "ifexternalMCPtool,checkconnection statusorrefresh"),
		)
	}
	if len(externalTools) == 0 {
		return result
	}

	externalMCPConfigs := h.externalMCPMgr.GetConfigs()

	for _, externalTool := range externalTools {
		// parsetool name:mcpName::toolName
		mcpName, actualToolName := h.parseExternalToolName(externalTool.Name)
		if mcpName == "" || actualToolName == "" {
			continue
		}
		enabled := h.calculateExternalToolEnabled(mcpName, actualToolName, externalMCPConfigs)
		description := h.pickToolDescription(externalTool.ShortDescription, externalTool.Description)

		result = append(result, ToolConfigInfo{
			Name: actualToolName,
			Description: description,
			Enabled: enabled,
			IsExternal: true,
			ExternalMCP: mcpName,
		})
	}

	return result
}
func (h *ConfigHandler) parseExternalToolName(fullName string) (mcpName, toolName string) {
	idx := strings.Index(fullName, "::")
	if idx > 0 {
		return fullName[:idx], fullName[idx+2:]
	}
	return "", ""
}
func (h *ConfigHandler) calculateExternalToolEnabled(mcpName, toolName string, configs map[string]config.ExternalMCPServerConfig) bool {
	cfg, exists := configs[mcpName]
	if !exists {
		return false
	}
	if !cfg.ExternalMCPEnable && !(cfg.Enabled && !cfg.Disabled) {
		return false
	}
	if cfg.ToolEnabled == nil {
	} else if toolEnabled, exists := cfg.ToolEnabled[toolName]; exists {
		// useconfigureoftoolstatus
		if !toolEnabled {
			return false
		}
	}
	client, exists := h.externalMCPMgr.GetClient(mcpName)
	if !exists || !client.IsConnected() {
		return false
	}

	return true
}
func (h *ConfigHandler) pickToolDescription(shortDesc, fullDesc string) string {
	useFull := strings.TrimSpace(strings.ToLower(h.config.Security.ToolDescriptionMode)) == "full"
	description := shortDesc
	if useFull {
		description = fullDesc
	} else if description == "" {
		description = fullDesc
	}
	if len(description) > 10000 {
		description = description[:10000] + "..."
	}
	return description
}
