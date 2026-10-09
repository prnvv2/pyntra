package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version     string                `yaml:"version,omitempty" json:"version,omitempty"` // Version number displayed in frontend, e.g. v1.3.3
	Server      ServerConfig          `yaml:"server"`
	Log         LogConfig             `yaml:"log"`
	MCP         MCPConfig             `yaml:"mcp"`
	OpenAI      OpenAIConfig          `yaml:"openai"`
	Agent       AgentConfig           `yaml:"agent"`
	Security    SecurityConfig        `yaml:"security"`
	Database    DatabaseConfig        `yaml:"database"`
	Auth        AuthConfig            `yaml:"auth"`
	ExternalMCP ExternalMCPConfig     `yaml:"external_mcp,omitempty"`
	Knowledge   KnowledgeConfig       `yaml:"knowledge,omitempty"`
	Bots        BotsConfig            `yaml:"bots,omitempty" json:"bots,omitempty"`             // Telegram / Slack / Discord chat bots
	RolesDir    string                `yaml:"roles_dir,omitempty" json:"roles_dir,omitempty"`   // Role configuration file directory (new method)
	Roles       map[string]RoleConfig `yaml:"roles,omitempty" json:"roles,omitempty"`           // Backward compatibility: support defining roles in the main configuration file
	SkillsDir   string                `yaml:"skills_dir,omitempty" json:"skills_dir,omitempty"` // Skills configuration file directory
	AgentsDir   string                `yaml:"agents_dir,omitempty" json:"agents_dir,omitempty"` // Multi-agent sub-Agent Markdown definition directory (*.md, YAML front matter)
	MultiAgent  MultiAgentConfig      `yaml:"multi_agent,omitempty" json:"multi_agent,omitempty"`
}

// MultiAgentConfig multi-agent orchestration based on CloudWeGo Eino adk/prebuilt (deep | plan_execute | supervisor, coexists with single Agent /agent-loop).
type MultiAgentConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	DefaultMode        string `yaml:"default_mode" json:"default_mode"`                   // single | multi, for frontend default display
	BatchUseMultiAgent bool   `yaml:"batch_use_multi_agent" json:"batch_use_multi_agent"` // When true, each sub-task in the batch task queue uses Eino multi-agent
	// Orchestration is deprecated: retained only for backward compatibility with old config.yaml; orchestration is determined by chat/WebShell request body orchestration, defaulting to deep when not provided.
	Orchestration string `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	MaxIteration  int    `yaml:"max_iteration" json:"max_iteration"` // Maximum reasoning iterations for main agent / executor (Deep, Supervisor, plan_execute's Executor)
	// PlanExecuteLoopMaxIterations upper limit of execute↔replan outer loop in plan_execute mode; 0 means use Eino default 10.
	PlanExecuteLoopMaxIterations int                   `yaml:"plan_execute_loop_max_iterations,omitempty" json:"plan_execute_loop_max_iterations,omitempty"`
	SubAgentMaxIterations        int                   `yaml:"sub_agent_max_iterations" json:"sub_agent_max_iterations"`
	WithoutGeneralSubAgent       bool                  `yaml:"without_general_sub_agent" json:"without_general_sub_agent"`
	WithoutWriteTodos            bool                  `yaml:"without_write_todos" json:"without_write_todos"`
	OrchestratorInstruction      string                `yaml:"orchestrator_instruction" json:"orchestrator_instruction"`
	// OrchestratorInstructionPlanExecute system prompt for plan_execute main agent (planning side); takes effect when non-empty and agents/orchestrator-plan-execute.md content is empty or does not exist. Do not mix with Deep's orchestrator_instruction.
	OrchestratorInstructionPlanExecute string `yaml:"orchestrator_instruction_plan_execute,omitempty" json:"orchestrator_instruction_plan_execute,omitempty"`
	// OrchestratorInstructionSupervisor system prompt for supervisor main agent (transfer/exit descriptions still appended by runtime); takes effect when non-empty and agents/orchestrator-supervisor.md content is empty or does not exist.
	OrchestratorInstructionSupervisor string `yaml:"orchestrator_instruction_supervisor,omitempty" json:"orchestrator_instruction_supervisor,omitempty"`
	SubAgents                    []MultiAgentSubConfig `yaml:"sub_agents" json:"sub_agents"`
	// EinoSkills configures CloudWeGo Eino ADK skill middleware + optional local filesystem/execute on DeepAgent.
	EinoSkills MultiAgentEinoSkillsConfig `yaml:"eino_skills,omitempty" json:"eino_skills,omitempty"`
	// EinoMiddleware wires optional ADK middleware (patchtoolcalls, toolsearch, plantask, reduction) and Deep extras.
	EinoMiddleware MultiAgentEinoMiddlewareConfig `yaml:"eino_middleware,omitempty" json:"eino_middleware,omitempty"`
}

// MultiAgentEinoMiddlewareConfig optional Eino ADK middleware and Deep / supervisor tuning.
type MultiAgentEinoMiddlewareConfig struct {
	// PatchToolCalls inserts placeholder tool results for dangling assistant tool_calls (nil = enabled).
	PatchToolCalls *bool `yaml:"patch_tool_calls,omitempty" json:"patch_tool_calls,omitempty"`
	// ToolSearch enables dynamictool/toolsearch: hide tail tools until model calls tool_search (reduces prompt tools).
	ToolSearchEnable        bool `yaml:"tool_search_enable,omitempty" json:"tool_search_enable,omitempty"`
	ToolSearchMinTools      int  `yaml:"tool_search_min_tools,omitempty" json:"tool_search_min_tools,omitempty"`           // default 20; applies when len(tools) >= this
	ToolSearchAlwaysVisible int  `yaml:"tool_search_always_visible,omitempty" json:"tool_search_always_visible,omitempty"` // default 12; first N tools stay always visible
	// Plantask adds TaskCreate/Get/Update/List (file-backed under skills dir); requires eino_skills + local backend.
	PlantaskEnable bool `yaml:"plantask_enable,omitempty" json:"plantask_enable,omitempty"`
	// PlantaskRelDir relative to skills_dir for per-conversation task boards (default .eino/plantask).
	PlantaskRelDir string `yaml:"plantask_rel_dir,omitempty" json:"plantask_rel_dir,omitempty"`
	// Reduction truncates/offloads large tool outputs (requires eino local backend for Write).
	ReductionEnable           bool     `yaml:"reduction_enable,omitempty" json:"reduction_enable,omitempty"`
	ReductionRootDir          string   `yaml:"reduction_root_dir,omitempty" json:"reduction_root_dir,omitempty"` // default: os temp + conversation id
	ReductionClearExclude     []string `yaml:"reduction_clear_exclude,omitempty" json:"reduction_clear_exclude,omitempty"`
	ReductionSubAgents        bool     `yaml:"reduction_sub_agents,omitempty" json:"reduction_sub_agents,omitempty"` // also attach to sub-agents
	// CheckpointDir when non-empty enables adk.Runner CheckPointStore (file-backed) for interrupt/resume persistence.
	CheckpointDir string `yaml:"checkpoint_dir,omitempty" json:"checkpoint_dir,omitempty"`
	// DeepOutputKey passed to deep.Config OutputKey (session final text); empty = off.
	DeepOutputKey string `yaml:"deep_output_key,omitempty" json:"deep_output_key,omitempty"`
	// DeepModelRetryMaxRetries > 0 enables deep.Config ModelRetryConfig (framework-level chat model retries).
	DeepModelRetryMaxRetries int `yaml:"deep_model_retry_max_retries,omitempty" json:"deep_model_retry_max_retries,omitempty"`
	// TaskToolDescriptionPrefix when non-empty sets deep.Config TaskToolDescriptionGenerator (sub-agent names appended).
	TaskToolDescriptionPrefix string `yaml:"task_tool_description_prefix,omitempty" json:"task_tool_description_prefix,omitempty"`
}

// MultiAgentEinoSkillsConfig toggles Eino official skill progressive disclosure and host filesystem tools.
type MultiAgentEinoSkillsConfig struct {
	// Disable skips skill middleware (and does not attach local FS tools for Deep).
	Disable bool `yaml:"disable" json:"disable"`
	// FilesystemTools registers read_file/glob/grep/write/edit/execute (eino-ext local backend). Nil/omitted = true.
	FilesystemTools *bool `yaml:"filesystem_tools,omitempty" json:"filesystem_tools,omitempty"`
	// SkillToolName overrides the default Eino tool name "skill".
	SkillToolName string `yaml:"skill_tool_name,omitempty" json:"skill_tool_name,omitempty"`
}

// EinoSkillFilesystemToolsEffective returns whether Deep/sub-agents should attach local filesystem + streaming shell.
func (c MultiAgentEinoSkillsConfig) EinoSkillFilesystemToolsEffective() bool {
	if c.FilesystemTools != nil {
		return *c.FilesystemTools
	}
	return true
}

// PatchToolCallsEffective returns whether patchtoolcalls middleware should run (default true).
func (c MultiAgentEinoMiddlewareConfig) PatchToolCallsEffective() bool {
	if c.PatchToolCalls != nil {
		return *c.PatchToolCalls
	}
	return true
}

// MultiAgentSubConfig sub-agent (Eino ChatModelAgent): scheduled by task under deep; delegated by transfer under supervisor; plan_execute does not use sub-agent list.
type MultiAgentSubConfig struct {
	ID            string   `yaml:"id" json:"id"`
	Name          string   `yaml:"name" json:"name"`
	Description   string   `yaml:"description" json:"description"`
	Instruction   string   `yaml:"instruction" json:"instruction"`
	BindRole      string   `yaml:"bind_role,omitempty" json:"bind_role,omitempty"` // Optional: associate with a role name in the main configuration roles; when role_tools is not configured, use the role's tools and write skills into the instruction prompt
	RoleTools     []string `yaml:"role_tools" json:"role_tools"`                   // Same key as single Agent role tools; empty means all tools (bind_role can complete tools)
	MaxIterations int      `yaml:"max_iterations" json:"max_iterations"`
	Kind          string   `yaml:"kind,omitempty" json:"kind,omitempty"` // Markdown only: kind=orchestrator indicates Deep main agent (convention of choosing one with orchestrator.md)
}

// MultiAgentPublic simplified information returned to frontend (does not include full sub-agent instructions).
type MultiAgentPublic struct {
	Enabled                      bool   `json:"enabled"`
	DefaultMode                  string `json:"default_mode"`
	BatchUseMultiAgent           bool   `json:"batch_use_multi_agent"`
	SubAgentCount                int    `json:"sub_agent_count"`
	Orchestration                string `json:"orchestration,omitempty"`
	PlanExecuteLoopMaxIterations int    `json:"plan_execute_loop_max_iterations"`
}

// NormalizeMultiAgentOrchestration returns deep, plan_execute, or supervisor.
func NormalizeMultiAgentOrchestration(s string) string {
	v := strings.TrimSpace(strings.ToLower(s))
	switch v {
	case "plan_execute", "plan-execute", "planexecute", "pe":
		return "plan_execute"
	case "supervisor", "super", "sv":
		return "supervisor"
	default:
		return "deep"
	}
}

// MultiAgentAPIUpdate Settings page/API updates only multi-agent scalar fields; when writing YAML does not overwrite blocks like sub_agents.
type MultiAgentAPIUpdate struct {
	Enabled                      bool   `json:"enabled"`
	DefaultMode                  string `json:"default_mode"`
	BatchUseMultiAgent           bool   `json:"batch_use_multi_agent"`
	PlanExecuteLoopMaxIterations *int   `json:"plan_execute_loop_max_iterations,omitempty"`
}

// BotsConfig chat-bot configuration (Telegram, Slack, Discord).
type BotsConfig struct {
	Telegram TelegramBotConfig `yaml:"telegram,omitempty" json:"telegram,omitempty"`
	Slack    SlackBotConfig    `yaml:"slack,omitempty" json:"slack,omitempty"`
	Discord  DiscordBotConfig  `yaml:"discord,omitempty" json:"discord,omitempty"`
}

// TelegramBotConfig Telegram bot (HTTP long-polling; no extra dependency).
type TelegramBotConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Token   string `yaml:"token" json:"token"`                   // Bot token from @BotFather
	Role    string `yaml:"role,omitempty" json:"role,omitempty"` // Optional role name applied to bot conversations
}

// SlackBotConfig Slack bot (Socket Mode; app-level token + bot token).
type SlackBotConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	AppToken string `yaml:"app_token" json:"app_token"`           // Socket Mode app-level token (xapp-...)
	BotToken string `yaml:"bot_token" json:"bot_token"`           // Web API bot token (xoxb-...)
	Role     string `yaml:"role,omitempty" json:"role,omitempty"` // Optional role name applied to bot conversations
}

// DiscordBotConfig Discord bot (gateway connection).
type DiscordBotConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Token   string `yaml:"token" json:"token"`                   // Bot token from the Discord developer portal
	Role    string `yaml:"role,omitempty" json:"role,omitempty"` // Optional role name applied to bot conversations
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Output string `yaml:"output"`
}

type MCPConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	AuthHeader      string `yaml:"auth_header,omitempty"`       // Authentication header name; leave empty for no authentication
	AuthHeaderValue string `yaml:"auth_header_value,omitempty"` // Authentication header value; must match the header in the request
}

type OpenAIConfig struct {
	Provider       string `yaml:"provider,omitempty" json:"provider,omitempty"` // API provider: "openai" (default) or "claude"; for claude automatically bridges to Anthropic Messages API
	APIKey         string `yaml:"api_key" json:"api_key"`
	BaseURL        string `yaml:"base_url" json:"base_url"`
	Model          string `yaml:"model" json:"model"`
	MaxTotalTokens int    `yaml:"max_total_tokens,omitempty" json:"max_total_tokens,omitempty"`
}

type SecurityConfig struct {
	Tools               []ToolConfig `yaml:"tools,omitempty"`                 // Backward compatible: support defining tools in main configuration file
	ToolsDir            string       `yaml:"tools_dir,omitempty"`             // Tool configuration file directory (new method)
	ToolDescriptionMode string       `yaml:"tool_description_mode,omitempty"` // Tool description mode: "short" | "full"; default short
}

type DatabaseConfig struct {
	Path            string `yaml:"path"`                        // Session database path
	KnowledgeDBPath string `yaml:"knowledge_db_path,omitempty"` // Knowledge base database path (optional; uses session database if empty)
}

type AgentConfig struct {
	MaxIterations        int    `yaml:"max_iterations" json:"max_iterations"`
	LargeResultThreshold int    `yaml:"large_result_threshold" json:"large_result_threshold"` // Large result threshold (bytes); default 50KB
	ResultStorageDir     string `yaml:"result_storage_dir" json:"result_storage_dir"`         // Result storage directory; default tmp
	ToolTimeoutMinutes   int    `yaml:"tool_timeout_minutes" json:"tool_timeout_minutes"`     // Maximum duration for single tool execution (minutes); auto-terminates on timeout to prevent prolonged hanging; 0 means no limit (not recommended)
	// SystemPromptPath Single-agent system prompt Markdown/text file path (relative to config.yaml directory, or absolute writable path). When non-empty and readable, replaces built-in single-agent prompt; leave empty to use built-in.
	SystemPromptPath string `yaml:"system_prompt_path,omitempty" json:"system_prompt_path,omitempty"`
}

type AuthConfig struct {
	Password                    string `yaml:"password" json:"password"`
	SessionDurationHours        int    `yaml:"session_duration_hours" json:"session_duration_hours"`
	GeneratedPassword           string `yaml:"-" json:"-"`
	GeneratedPasswordPersisted  bool   `yaml:"-" json:"-"`
	GeneratedPasswordPersistErr string `yaml:"-" json:"-"`
}

// ExternalMCPConfig External MCP configuration
type ExternalMCPConfig struct {
	Servers map[string]ExternalMCPServerConfig `yaml:"servers,omitempty" json:"servers,omitempty"`
}

// ExternalMCPServerConfig External MCP server configuration
type ExternalMCPServerConfig struct {
	// Stdio mode configuration
	Command string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"` // Environment variables (used for stdio mode)

	// HTTP mode configuration
	Transport string            `yaml:"transport,omitempty" json:"transport,omitempty"` // "stdio" | "sse" | "http"(Streamable) | "simple_http"(custom/simple POST endpoint, e.g., http://127.0.0.1:8081/mcp)
	URL       string            `yaml:"url,omitempty" json:"url,omitempty"`
	Headers   map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"` // HTTP/SSE request headers (e.g., x-api-key)

	// Common configuration
	Description       string          `yaml:"description,omitempty" json:"description,omitempty"`
	Timeout           int             `yaml:"timeout,omitempty" json:"timeout,omitempty"`                         // Timeout (seconds)
	ExternalMCPEnable bool            `yaml:"external_mcp_enable,omitempty" json:"external_mcp_enable,omitempty"` // Whether to enable external MCP
	ToolEnabled       map[string]bool `yaml:"tool_enabled,omitempty" json:"tool_enabled,omitempty"`               // Enabled status for each tool (tool name -> enabled)

	// Backward compatible fields (deprecated; retained for reading legacy configurations)
	Enabled  bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`   // Deprecated; use external_mcp_enable
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"` // Deprecated; use external_mcp_enable
}
type ToolConfig struct {
	Name             string            `yaml:"name"`
	Command          string            `yaml:"command"`
	Args             []string          `yaml:"args,omitempty"`              // Fixed parameters (optional)
	ShortDescription string            `yaml:"short_description,omitempty"` // Short description (for tool list; reduces token consumption)
	Description      string            `yaml:"description"`                 // Detailed description (for tool documentation)
	Enabled          bool              `yaml:"enabled"`
	Parameters       []ParameterConfig `yaml:"parameters,omitempty"`         // Parameter definitions (optional)
	ArgMapping       string            `yaml:"arg_mapping,omitempty"`        // Parameter mapping mode: "auto", "manual", "template" (optional)
	AllowedExitCodes []int             `yaml:"allowed_exit_codes,omitempty"` // List of allowed exit codes (some tools return non-zero codes even on success)
}

// ParameterConfig Parameter configuration
type ParameterConfig struct {
	Name        string      `yaml:"name"`                // Parameter name
	Type        string      `yaml:"type"`                // Parameter type: string, int, bool, array
	Description string      `yaml:"description"`         // Parameter description
	Required    bool        `yaml:"required,omitempty"`  // Whether required
	Default     interface{} `yaml:"default,omitempty"`   // Default value
	ItemType    string      `yaml:"item_type,omitempty"` // When type is array, array element type (e.g., string, number, object)
	Flag        string      `yaml:"flag,omitempty"`      // Command-line flag (e.g., "-u", "--url", "-p")
	Position    *int        `yaml:"position,omitempty"`  // Position of positional parameter (0-indexed)
	Format      string      `yaml:"format,omitempty"`    // Parameter format: "flag", "positional", "combined" (flag=value), "template"
	Template    string      `yaml:"template,omitempty"`  // Template string (e.g., "{flag} {value}" or "{value}")
	Options     []string    `yaml:"options,omitempty"`   // List of optional values (for enumeration)
}

func Load(path string) (*Config, error) {
	// First-run convenience: if the config file is missing but an example sits
	// next to it (config.example.yaml), seed the real config from the example.
	// This keeps secrets (api_key / auth.password) out of version control — the
	// example is tracked; the generated config.yaml is git-ignored.
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		example := filepath.Join(filepath.Dir(path), "config.example.yaml")
		if example != path {
			if exampleData, exErr := os.ReadFile(example); exErr == nil {
				if wErr := os.WriteFile(path, exampleData, 0600); wErr != nil {
					return nil, fmt.Errorf("failed to seed configuration from example: %w", wErr)
				}
			}
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	// Expand ${ENV_VAR} references in secret fields so credentials can be kept
	// out of config.yaml entirely (e.g. api_key: ${OPENAI_API_KEY}).
	expandEnvSecrets(&cfg)

	if cfg.Auth.SessionDurationHours <= 0 {
		cfg.Auth.SessionDurationHours = 12
	}

	if strings.TrimSpace(cfg.Auth.Password) == "" {
		password, err := generateStrongPassword(24)
		if err != nil {
			return nil, fmt.Errorf("failed to generate default password: %w", err)
		}

		cfg.Auth.Password = password
		cfg.Auth.GeneratedPassword = password

		if err := PersistAuthPassword(path, password); err != nil {
			cfg.Auth.GeneratedPasswordPersisted = false
			cfg.Auth.GeneratedPasswordPersistErr = err.Error()
		} else {
			cfg.Auth.GeneratedPasswordPersisted = true
		}
	}

	// If tools directory is configured, load tool configurations from directory
	if cfg.Security.ToolsDir != "" {
		configDir := filepath.Dir(path)
		toolsDir := cfg.Security.ToolsDir

		// If it's a relative path, make it relative to the configuration file directory
		if !filepath.IsAbs(toolsDir) {
			toolsDir = filepath.Join(configDir, toolsDir)
		}

		tools, err := LoadToolsFromDir(toolsDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load tool configurations from directory: %w", err)
		}

		// Merge tool configurations: tools from directory take priority, tools from main config are supplemental
		existingTools := make(map[string]bool)
		for _, tool := range tools {
			existingTools[tool.Name] = true
		}

		// Add tools from main config that don't exist in directory (backward compatible)
		for _, tool := range cfg.Security.Tools {
			if !existingTools[tool.Name] {
				tools = append(tools, tool)
			}
		}

		cfg.Security.Tools = tools
	}

	// Migrate external MCP configuration: migrate old enabled/disabled fields to external_mcp_enable
	if cfg.ExternalMCP.Servers != nil {
		for name, serverCfg := range cfg.ExternalMCP.Servers {
			// If external_mcp_enable is already set, skip migration
			// Otherwise migrate from enabled/disabled fields
			// Note: Since ExternalMCPEnable is bool type with zero value false, we need to check if it's actually set
			// Here we determine if migration is needed by checking the old enabled/disabled fields
			if serverCfg.Disabled {
				// Old config uses disabled; migrate to external_mcp_enable
				serverCfg.ExternalMCPEnable = false
			} else if serverCfg.Enabled {
				// Old config uses enabled; migrate to external_mcp_enable
				serverCfg.ExternalMCPEnable = true
			} else {
				// Neither is set; default to enabled
				serverCfg.ExternalMCPEnable = true
			}
			cfg.ExternalMCP.Servers[name] = serverCfg
		}
	}

	// Load role configurations from role directory
	if cfg.RolesDir != "" {
		configDir := filepath.Dir(path)
		rolesDir := cfg.RolesDir

		// If it's a relative path, make it relative to the configuration file directory
		if !filepath.IsAbs(rolesDir) {
			rolesDir = filepath.Join(configDir, rolesDir)
		}

		roles, err := LoadRolesFromDir(rolesDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load role configurations from directory: %w", err)
		}

		cfg.Roles = roles
	} else {
		// If roles_dir is not configured, initialize as empty map
		if cfg.Roles == nil {
			cfg.Roles = make(map[string]RoleConfig)
		}
	}

	return &cfg, nil
}

func generateStrongPassword(length int) (string, error) {
	if length <= 0 {
		length = 24
	}

	bytesLen := length
	randomBytes := make([]byte, bytesLen)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	password := base64.RawURLEncoding.EncodeToString(randomBytes)
	if len(password) > length {
		password = password[:length]
	}
	return password, nil
}

func PersistAuthPassword(path, password string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	inAuthBlock := false
	authIndent := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inAuthBlock {
			if strings.HasPrefix(trimmed, "auth:") {
				inAuthBlock = true
				authIndent = len(line) - len(strings.TrimLeft(line, " "))
			}
			continue
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
		if leadingSpaces <= authIndent {
			// Exit auth block
			inAuthBlock = false
			authIndent = -1
			// Continue searching for other auth blocks (theoretically not possible)
			if strings.HasPrefix(trimmed, "auth:") {
				inAuthBlock = true
				authIndent = leadingSpaces
			}
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), "password:") {
			prefix := line[:len(line)-len(strings.TrimLeft(line, " "))]
			comment := ""
			if idx := strings.Index(line, "#"); idx >= 0 {
				comment = strings.TrimRight(line[idx:], " ")
			}

			newLine := fmt.Sprintf("%spassword: %s", prefix, password)
			if comment != "" {
				if !strings.HasPrefix(comment, " ") {
					newLine += " "
				}
				newLine += comment
			}
			lines[i] = newLine
			break
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func PrintGeneratedPasswordWarning(password string, persisted bool, persistErr string) {
	if strings.TrimSpace(password) == "" {
		return
	}

	if persisted {
		fmt.Println("[Pyntra] ✅ Auto-generated and written web login password for you.")
	} else {
		if persistErr != "" {
			fmt.Printf("[Pyntra] ⚠️ Unable to auto-write password to configuration file: %s\n", persistErr)
		} else {
			fmt.Println("[Pyntra] ⚠️ Unable to auto-write password to configuration file.")
		}
		fmt.Println("Please manually write the following random password to auth.password in config.yaml:")
	}

	fmt.Println("----------------------------------------------------------------")
	fmt.Println("Pyntra Auto-Generated Web Password")
	fmt.Printf("Password: %s\n", password)
	fmt.Println("WARNING: Anyone with this password can fully control Pyntra.")
	fmt.Println("Please store it securely and change it in config.yaml as soon as possible.")
	fmt.Println("Warning: Anyone who holds this password will have complete control over Pyntra.")
	fmt.Println("Please keep it safe and modify auth.password in config.yaml as soon as possible!")
	fmt.Println("----------------------------------------------------------------")
}

// generateRandomToken generates random string for MCP authentication (64-bit hexadecimal)
func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// persistMCPAuth writes MCP auth_header / auth_header_value back to configuration file
func persistMCPAuth(path string, mcp *MCPConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	inMcpBlock := false
	mcpIndent := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inMcpBlock {
			if strings.HasPrefix(trimmed, "mcp:") {
				inMcpBlock = true
				mcpIndent = len(line) - len(strings.TrimLeft(line, " "))
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		leadingSpaces := len(line) - len(strings.TrimLeft(line, " "))
		if leadingSpaces <= mcpIndent {
			inMcpBlock = false
			mcpIndent = -1
			if strings.HasPrefix(trimmed, "mcp:") {
				inMcpBlock = true
				mcpIndent = leadingSpaces
			}
			continue
		}

		prefix := line[:leadingSpaces]
		rest := strings.TrimSpace(line[leadingSpaces:])
		comment := ""
		if idx := strings.Index(line, "#"); idx >= 0 {
			comment = strings.TrimRight(line[idx:], " ")
		}
		withComment := ""
		if comment != "" {
			if !strings.HasPrefix(comment, " ") {
				withComment = " "
			}
			withComment += comment
		}

		if strings.HasPrefix(rest, "auth_header_value:") {
			lines[i] = fmt.Sprintf("%sauth_header_value: %q%s", prefix, mcp.AuthHeaderValue, withComment)
		} else if strings.HasPrefix(rest, "auth_header:") {
			lines[i] = fmt.Sprintf("%sauth_header: %q%s", prefix, mcp.AuthHeader, withComment)
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// EnsureMCPAuth automatically generates and writes back random key when MCP is enabled and auth_header_value is empty
func EnsureMCPAuth(path string, cfg *Config) error {
	if !cfg.MCP.Enabled || strings.TrimSpace(cfg.MCP.AuthHeaderValue) != "" {
		return nil
	}
	token, err := generateRandomToken()
	if err != nil {
		return fmt.Errorf("failed to generate MCP authentication key: %w", err)
	}
	cfg.MCP.AuthHeaderValue = token
	if strings.TrimSpace(cfg.MCP.AuthHeader) == "" {
		cfg.MCP.AuthHeader = "X-MCP-Token"
	}
	return persistMCPAuth(path, &cfg.MCP)
}

// PrintMCPConfigJSON outputs MCP configuration JSON to terminal for direct use in Cursor / Claude Code mcp configuration
func PrintMCPConfigJSON(mcp MCPConfig) {
	if !mcp.Enabled {
		return
	}
	hostForURL := strings.TrimSpace(mcp.Host)
	if hostForURL == "" || hostForURL == "0.0.0.0" {
		hostForURL = "localhost"
	}
	url := fmt.Sprintf("http://%s:%d/mcp", hostForURL, mcp.Port)
	headers := map[string]string{}
	if mcp.AuthHeader != "" {
		headers[mcp.AuthHeader] = mcp.AuthHeaderValue
	}
	serverEntry := map[string]interface{}{
		"url": url,
	}
	if len(headers) > 0 {
		serverEntry["headers"] = headers
	}
	// Claude Code requires type: "http"
	serverEntry["type"] = "http"
	out := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"cyberstrike-ai": serverEntry,
		},
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println("[Pyntra] MCP configuration (can be copied to Cursor / Claude Code):")
	fmt.Println("  Cursor: put into mcpServers in ~/.cursor/mcp.json or project .cursor/mcp.json")
	fmt.Println("  Claude Code: put into mcpServers in .mcp.json or ~/.claude.json")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println(string(b))
	fmt.Println("----------------------------------------------------------------")
}

// LoadToolsFromDir loads all tool configuration files from directory
func LoadToolsFromDir(dir string) ([]ToolConfig, error) {
	var tools []ToolConfig

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return tools, nil // Return empty list if directory doesn't exist; no error
	}

	// Read all .yaml and .yml files from directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tool directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		filePath := filepath.Join(dir, name)
		tool, err := LoadToolFromFile(filePath)
		if err != nil {
			// Log error but continue loading other files
			fmt.Printf("Warning: failed to load tool configuration file %s: %v\n", filePath, err)
			continue
		}

		tools = append(tools, *tool)
	}

	return tools, nil
}

// LoadToolFromFile loads tool configuration from a single file
func LoadToolFromFile(path string) (*ToolConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var tool ToolConfig
	if err := yaml.Unmarshal(data, &tool); err != nil {
		return nil, fmt.Errorf("failed to parse tool configuration: %w", err)
	}

	// Validate required fields
	if tool.Name == "" {
		return nil, fmt.Errorf("tool name cannot be empty")
	}
	if tool.Command == "" {
		return nil, fmt.Errorf("tool command cannot be empty")
	}

	return &tool, nil
}

// LoadRolesFromDir loads all role configuration files from directory
func LoadRolesFromDir(dir string) (map[string]RoleConfig, error) {
	roles := make(map[string]RoleConfig)

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return roles, nil // Return empty map if directory doesn't exist; no error
	}

	// Read all .yaml and .yml files from directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read role directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		filePath := filepath.Join(dir, name)
		role, err := LoadRoleFromFile(filePath)
		if err != nil {
			// Log error but continue loading other files
			fmt.Printf("Warning: failed to load role configuration file %s: %v\n", filePath, err)
			continue
		}

		// Use role name as key
		roleName := role.Name
		if roleName == "" {
			// If role name is empty, use filename (without extension) as name
			roleName = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
			role.Name = roleName
		}

		roles[roleName] = *role
	}

	return roles, nil
}

// LoadRoleFromFile loads role configuration from a single file
func LoadRoleFromFile(path string) (*RoleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var role RoleConfig
	if err := yaml.Unmarshal(data, &role); err != nil {
		return nil, fmt.Errorf("failed to parse role configuration: %w", err)
	}

	// Process icon field: if it contains Unicode escape format (\U0001F3C6), convert to actual Unicode character
	// Go's yaml library may not automatically parse \U escape sequences; manual conversion is needed
	if role.Icon != "" {
		icon := role.Icon
		// Remove possible quotes
		icon = strings.Trim(icon, `"`)

		// Check if it's Unicode escape format \U0001F3C6 (8-digit hexadecimal) or \uXXXX (4-digit hexadecimal)
		if len(icon) >= 3 && icon[0] == '\\' {
			if icon[1] == 'U' && len(icon) >= 10 {
				// \U0001F3C6 format (8-digit hexadecimal)
				if codePoint, err := strconv.ParseInt(icon[2:10], 16, 32); err == nil {
					role.Icon = string(rune(codePoint))
				}
			} else if icon[1] == 'u' && len(icon) >= 6 {
				// \uXXXX format (4-digit hexadecimal)
				if codePoint, err := strconv.ParseInt(icon[2:6], 16, 32); err == nil {
					role.Icon = string(rune(codePoint))
				}
			}
		}
	}

	// Validate required fields
	if role.Name == "" {
		// If name is empty, try to get from filename
		baseName := filepath.Base(path)
		role.Name = strings.TrimSuffix(strings.TrimSuffix(baseName, ".yaml"), ".yml")
	}

	return &role, nil
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Log: LogConfig{
			Level:  "info",
			Output: "stdout",
		},
		MCP: MCPConfig{
			Enabled: true,
			Host:    "0.0.0.0",
			Port:    8081,
		},
		OpenAI: OpenAIConfig{
			BaseURL:        "https://api.openai.com/v1",
			Model:          "gpt-4",
			MaxTotalTokens: 120000,
		},
		Agent: AgentConfig{
			MaxIterations:      30, // Default maximum iterations
			ToolTimeoutMinutes: 10, // Default maximum 10 minutes per tool execution to avoid abnormally long occupation
		},
		Security: SecurityConfig{
			Tools:    []ToolConfig{}, // Tool configuration should be loaded from config.yaml or tools/ directory
			ToolsDir: "tools",        // Default tools directory
		},
		Database: DatabaseConfig{
			Path:            "data/conversations.db",
			KnowledgeDBPath: "data/knowledge.db", // Default knowledge base database path
		},
		Auth: AuthConfig{
			SessionDurationHours: 12,
		},
		Knowledge: KnowledgeConfig{
			Enabled:  true,
			BasePath: "knowledge_base",
			Embedding: EmbeddingConfig{
				Provider: "openai",
				Model:    "text-embedding-3-small",
				BaseURL:  "https://api.openai.com/v1",
			},
			Retrieval: RetrievalConfig{
				TopK:                5,
				SimilarityThreshold: 0.65, // Reduced threshold to 0.65 for better recall
			},
			Indexing: IndexingConfig{
				ChunkStrategy:         "markdown_then_recursive",
				RequestTimeoutSeconds: 120,
				ChunkSize:             768, // Increased to 768 for better context retention
				ChunkOverlap:          50,
				MaxChunksPerItem:      20, // Limit single item to max 20 chunks to avoid excessive quota consumption
				BatchSize:             64,
				PreferSourceFile:      false,
				MaxRPM:                100, // Default 100 RPM to avoid 429 errors
				RateLimitDelayMs:      600, // 600ms interval corresponding to 100 RPM
				MaxRetries:            3,
				RetryDelayMs:          1000,
				SubIndexes:            nil,
			},
		},
	}
}

// KnowledgeConfig Knowledge base configuration
type KnowledgeConfig struct {
	Enabled   bool            `yaml:"enabled" json:"enabled"`     // Whether to enable knowledge retrieval
	BasePath  string          `yaml:"base_path" json:"base_path"` // Knowledge base path
	Embedding EmbeddingConfig `yaml:"embedding" json:"embedding"`
	Retrieval RetrievalConfig `yaml:"retrieval" json:"retrieval"`
	Indexing  IndexingConfig  `yaml:"indexing,omitempty" json:"indexing,omitempty"` // Indexing configuration
}

// IndexingConfig Indexing configuration (controls knowledge base indexing behavior)
type IndexingConfig struct {
	// ChunkStrategy: "markdown_then_recursive" (default, Eino Markdown heading splitting followed by recursive split) or "recursive" (recursive split only)
	ChunkStrategy string `yaml:"chunk_strategy,omitempty" json:"chunk_strategy,omitempty"`
	// RequestTimeoutSeconds Embedding HTTP client timeout (seconds); 0 means use default 120
	RequestTimeoutSeconds int `yaml:"request_timeout_seconds,omitempty" json:"request_timeout_seconds,omitempty"`
	// Chunk configuration
	ChunkSize        int `yaml:"chunk_size,omitempty" json:"chunk_size,omitempty"`                   // Maximum tokens per chunk (estimate); default 512
	ChunkOverlap     int `yaml:"chunk_overlap,omitempty" json:"chunk_overlap,omitempty"`             // Token overlap between chunks; default 50
	MaxChunksPerItem int `yaml:"max_chunks_per_item,omitempty" json:"max_chunks_per_item,omitempty"` // Maximum chunks per knowledge item; 0 means unlimited

	// PreferSourceFile When true, prefer using Eino FileLoader to read from file_path and index (disk takes precedence when inconsistent with library content)
	PreferSourceFile bool `yaml:"prefer_source_file,omitempty" json:"prefer_source_file,omitempty"`

	// Rate limiting configuration (to avoid API rate limits)
	RateLimitDelayMs int `yaml:"rate_limit_delay_ms,omitempty" json:"rate_limit_delay_ms,omitempty"` // Request interval (milliseconds); 0 means no fixed delay
	MaxRPM           int `yaml:"max_rpm,omitempty" json:"max_rpm,omitempty"`                         // Maximum requests per minute; 0 means unlimited

	// Retry configuration (for handling temporary errors)
	MaxRetries   int `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`       // Maximum retry attempts; default 3
	RetryDelayMs int `yaml:"retry_delay_ms,omitempty" json:"retry_delay_ms,omitempty"` // Retry interval (milliseconds); default 1000

	// BatchSize Embedding batch size (SQLite index writes); 0 means default 64
	BatchSize int `yaml:"batch_size,omitempty" json:"batch_size,omitempty"`
	// SubIndexes Pass to Eino indexer.WithSubIndexes (logical partition markers passed with Document metadata)
	SubIndexes []string `yaml:"sub_indexes,omitempty" json:"sub_indexes,omitempty"`
}

// EmbeddingConfig Embedding configuration
type EmbeddingConfig struct {
	Provider string `yaml:"provider" json:"provider"` // Embedding model provider
	Model    string `yaml:"model" json:"model"`       // Model name
	BaseURL  string `yaml:"base_url" json:"base_url"` // API Base URL
	APIKey   string `yaml:"api_key" json:"api_key"`   // API Key (inherited from OpenAI configuration)
}

// PostRetrieveConfig Post-retrieval processing: normalize and deduplicate document text (best practice), context budget truncation; PrefetchTopK is used to fetch more candidates and converge to top_k.
type PostRetrieveConfig struct {
	// PrefetchTopK Maximum candidates retained during vector retrieval (cosine order); should be >= top_k; 0 means same as top_k; see upper limit in knowledge base package constants.
	PrefetchTopK int `yaml:"prefetch_top_k,omitempty" json:"prefetch_top_k,omitempty"`
	// MaxContextChars Maximum total Unicode characters in returned document content (complete chunks; not truncated mid-chunk); 0 means unlimited.
	MaxContextChars int `yaml:"max_context_chars,omitempty" json:"max_context_chars,omitempty"`
	// MaxContextTokens Maximum total tokens in returned document content (tiktoken; mapped by embedding model name; falls back to cl100k_base if failed); 0 means unlimited.
	MaxContextTokens int `yaml:"max_context_tokens,omitempty" json:"max_context_tokens,omitempty"`
}

// RetrievalConfig Retrieval configuration
type RetrievalConfig struct {
	TopK                int     `yaml:"top_k" json:"top_k"`                               // Retrieve Top-K
	SimilarityThreshold float64 `yaml:"similarity_threshold" json:"similarity_threshold"` // Cosine similarity threshold
	// SubIndexFilter When non-empty, only retain rows whose sub_indexes contain this label (comma-separated); old rows with empty sub_indexes are still returned.
	SubIndexFilter string `yaml:"sub_index_filter,omitempty" json:"sub_index_filter,omitempty"`
	// PostRetrieve Post-retrieval processing (deduplication, budget truncation); reranking through code injection [knowledge.DocumentReranker].
	PostRetrieve PostRetrieveConfig `yaml:"post_retrieve,omitempty" json:"post_retrieve,omitempty"`
	// Mode selects the retrieval strategy: "dense" (default, vector only),
	// "lexical" (BM25/FTS5 only), or "hybrid" (both, fused with RRF).
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// RRFK is the Reciprocal Rank Fusion constant for hybrid mode (default 60).
	RRFK int `yaml:"rrf_k,omitempty" json:"rrf_k,omitempty"`
	// Rerank optionally reranks candidates with a cross-encoder endpoint.
	Rerank RerankConfig `yaml:"rerank,omitempty" json:"rerank,omitempty"`
}

// RerankConfig configures an optional cross-encoder reranker (OpenAI-compatible
// /rerank endpoint, e.g. Jina, Cohere, or a local bge-reranker).
type RerankConfig struct {
	Enabled        bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	BaseURL        string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKey         string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model          string `yaml:"model,omitempty" json:"model,omitempty"`
	TopN           int    `yaml:"top_n,omitempty" json:"top_n,omitempty"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
}

// RolesConfig Role configuration (deprecated; use map[string]RoleConfig instead)
// Retaining this type for backward compatibility with legacy code, but direct use of map[string]RoleConfig is recommended
type RolesConfig struct {
	Roles map[string]RoleConfig `yaml:"roles,omitempty" json:"roles,omitempty"`
}

// RoleConfig Single role configuration
type RoleConfig struct {
	Name        string   `yaml:"name" json:"name"`                         // Role name
	Description string   `yaml:"description" json:"description"`           // Role description
	UserPrompt  string   `yaml:"user_prompt" json:"user_prompt"`           // User prompt (appended before user message)
	Icon        string   `yaml:"icon,omitempty" json:"icon,omitempty"`     // Role icon (optional)
	Tools       []string `yaml:"tools,omitempty" json:"tools,omitempty"`   // Associated tools list (toolKey format: "toolName" or "mcpName::toolName")
	MCPs        []string `yaml:"mcps,omitempty" json:"mcps,omitempty"`     // Backward compatible: associated MCP servers list (deprecated; use tools instead)
	Skills      []string `yaml:"skills,omitempty" json:"skills,omitempty"` // Associated skills list (skill name list; contents of these skills will be read before executing tasks)
	Enabled     bool     `yaml:"enabled" json:"enabled"`                   // Whether enabled
}
