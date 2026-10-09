package security

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pyntra/internal/config"
	"pyntra/internal/mcp"
	"pyntra/internal/storage"

	"github.com/creack/pty"
	"go.uber.org/zap"
)
type ToolOutputCallback func(chunk string)

type toolOutputCallbackCtxKey struct{}
var ToolOutputCallbackCtxKey = toolOutputCallbackCtxKey{}
type Executor struct {
	config *config.SecurityConfig
	toolIndex map[string]*config.ToolConfig
	mcpServer *mcp.Server
	logger *zap.Logger
	resultStorage ResultStorage
	scopeGuard ScopeGuard
	halted atomic.Bool
}

// SetHalted engages or releases the global kill-switch. While halted, every tool
// call is refused — a panic button to stop all agent activity immediately.
func (e *Executor) SetHalted(v bool) {
	if e != nil {
		e.halted.Store(v)
	}
}

// Halted reports whether the kill-switch is engaged.
func (e *Executor) Halted() bool {
	return e != nil && e.halted.Load()
}

// ScopeGuard decides whether a tool call is permitted against the active
// engagement scope. A nil guard (the default) permits everything.
type ScopeGuard interface {
	// Check returns a non-nil error to DENY the call; the error message is
	// surfaced to the agent.
	Check(toolName string, args map[string]interface{}) error
}

// SetScopeGuard installs (or clears, with nil) the engagement scope guard.
func (e *Executor) SetScopeGuard(g ScopeGuard) {
	if e != nil {
		e.scopeGuard = g
	}
}
type ResultStorage interface {
	SaveResult(executionID string, toolName string, result string) error
	GetResult(executionID string) (string, error)
	GetResultPage(executionID string, page int, limit int) (*storage.ResultPage, error)
	SearchResult(executionID string, keyword string, useRegex bool) ([]string, error)
	FilterResult(executionID string, filter string, useRegex bool) ([]string, error)
	GetResultMetadata(executionID string) (*storage.ResultMetadata, error)
	GetResultPath(executionID string) string
	DeleteResult(executionID string) error
}
func NewExecutor(cfg *config.SecurityConfig, mcpServer *mcp.Server, logger *zap.Logger) *Executor {
	executor := &Executor{
		config: cfg,
		toolIndex: make(map[string]*config.ToolConfig),
		mcpServer: mcpServer,
		logger: logger,
		resultStorage: nil,
	}
	executor.buildToolIndex()
	return executor
}
func (e *Executor) SetResultStorage(storage ResultStorage) {
	e.resultStorage = storage
}
func (e *Executor) buildToolIndex() {
	e.toolIndex = make(map[string]*config.ToolConfig)
	for i := range e.config.Tools {
		if e.config.Tools[i].Enabled {
			e.toolIndex[e.config.Tools[i].Name] = &e.config.Tools[i]
		}
	}
	e.logger.Info("toolbuildcompleted",
		zap.Int("totalTools", len(e.config.Tools)),
		zap.Int("enabledTools", len(e.toolIndex)),
	)
}
func (e *Executor) ExecuteTool(ctx context.Context, toolName string, args map[string]interface{}) (*mcp.ToolResult, error) {
	e.logger.Info("ExecuteToolcall",
		zap.String("toolName", toolName),
		zap.Any("args", args),
	)
	// Global kill-switch: refuse everything while halted.
	if e.Halted() {
		return &mcp.ToolResult{
			Content: []mcp.Content{{Type: "text", Text: "BLOCKED: agent execution is halted (kill-switch engaged). Resume from the console to continue."}},
			IsError: true,
		}, nil
	}
	// Engagement scope enforcement (no-op when no guard / no active scope).
	if e.scopeGuard != nil {
		if err := e.scopeGuard.Check(toolName, args); err != nil {
			e.logger.Warn("tool call blocked by engagement scope",
				zap.String("toolName", toolName), zap.Error(err))
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "BLOCKED by engagement scope: " + err.Error()}},
				IsError: true,
			}, nil
		}
	}
	if toolName == "exec" {
		e.logger.Info("exectool")
		return e.executeSystemCommand(ctx, args)
	}
	toolConfig, exists := e.toolIndex[toolName]
	if !exists {
		e.logger.Error("toolenabled",
			zap.String("toolName", toolName),
			zap.Int("totalTools", len(e.config.Tools)),
			zap.Int("enabledTools", len(e.toolIndex)),
		)
		return nil, fmt.Errorf("tool %s enabled", toolName)
	}

	e.logger.Info("toolconfig",
		zap.String("toolName", toolName),
		zap.String("command", toolConfig.Command),
		zap.Strings("args", toolConfig.Args),
	)
	if strings.HasPrefix(toolConfig.Command, "internal:") {
		e.logger.Info("executing internal tool",
			zap.String("toolName", toolName),
			zap.String("command", toolConfig.Command),
		)
		return e.executeInternalTool(ctx, toolName, toolConfig.Command, args)
	}
	cmdArgs := e.buildCommandArgs(toolName, toolConfig, args)

	e.logger.Info("buildcompleted",
		zap.String("toolName", toolName),
		zap.Strings("cmdArgs", cmdArgs),
		zap.Int("argsCount", len(cmdArgs)),
	)
	if len(cmdArgs) == 0 {
		e.logger.Warn("is empty",
			zap.String("toolName", toolName),
			zap.Any("inputArgs", args),
		)
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("error: tool %s .: %v", toolName, args),
				},
			},
			IsError: true,
		}, nil
	}
	cmd := exec.CommandContext(ctx, toolConfig.Command, cmdArgs...)
	applyDefaultTerminalEnv(cmd)

	e.logger.Info("tool",
		zap.String("tool", toolName),
		zap.Strings("args", cmdArgs),
	)

	var output string
	var err error
	if cb, ok := ctx.Value(ToolOutputCallbackCtxKey).(ToolOutputCallback); ok && cb != nil {
		output, err = streamCommandOutput(cmd, cb)
		if err != nil && shouldRetryWithPTY(output) {
			e.logger.Info("Tool requires a TTY; retrying with a PTY",
				zap.String("tool", toolName),
			)
			cmd2 := exec.CommandContext(ctx, toolConfig.Command, cmdArgs...)
			applyDefaultTerminalEnv(cmd2)
			output, err = runCommandWithPTY(ctx, cmd2, cb)
		}
	} else {
		outputBytes, err2 := cmd.CombinedOutput()
		output = string(outputBytes)
		err = err2
		if err != nil && shouldRetryWithPTY(output) {
			e.logger.Info("Tool requires a TTY; retrying with a PTY",
				zap.String("tool", toolName),
			)
			cmd2 := exec.CommandContext(ctx, toolConfig.Command, cmdArgs...)
			applyDefaultTerminalEnv(cmd2)
			output, err = runCommandWithPTY(ctx, cmd2, nil)
		}
	}
	if err != nil {
		exitCode := getExitCode(err)
		if exitCode != nil && toolConfig.AllowedExitCodes != nil {
			for _, allowedCode := range toolConfig.AllowedExitCodes {
				if *exitCode == allowedCode {
					e.logger.Info("toolcompleted(sign out)",
						zap.String("tool", toolName),
						zap.Int("exitCode", *exitCode),
						zap.String("output", string(output)),
					)
					return &mcp.ToolResult{
						Content: []mcp.Content{
							{
								Type: "text",
								Text: string(output),
							},
						},
						IsError: false,
					}, nil
				}
			}
		}

		e.logger.Error("toolfailed",
			zap.String("tool", toolName),
			zap.Error(err),
			zap.Int("exitCode", getExitCodeValue(err)),
			zap.String("output", string(output)),
		)
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("toolfailed: %v\noutput: %s", err, string(output)),
				},
			},
			IsError: true,
		}, nil
	}

	e.logger.Info("toolsuccessful",
		zap.String("tool", toolName),
		zap.String("output", string(output)),
	)

	return &mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: string(output),
			},
		},
		IsError: false,
	}, nil
}
func (e *Executor) RegisterTools(mcpServer *mcp.Server) {
	e.logger.Info("startregistertool",
		zap.Int("totalTools", len(e.config.Tools)),
		zap.Int("enabledTools", len(e.toolIndex)),
	)
	e.buildToolIndex()

	for i, toolConfig := range e.config.Tools {
		if !toolConfig.Enabled {
			e.logger.Debug("enabledtool",
				zap.String("tool", toolConfig.Name),
			)
			continue
		}
		toolName := toolConfig.Name
		toolConfigCopy := toolConfig
		useFullDescription := strings.TrimSpace(strings.ToLower(e.config.ToolDescriptionMode)) == "full"
		shortDesc := toolConfigCopy.ShortDescription
		if shortDesc == "" {
			desc := toolConfigCopy.Description
			if len(desc) > 10000 {
				if idx := strings.Index(desc, "\n"); idx > 0 && idx < 10000 {
					shortDesc = strings.TrimSpace(desc[:idx])
				} else {
					shortDesc = desc[:10000] + "..."
				}
			} else {
				shortDesc = desc
			}
		}
		if useFullDescription {
			shortDesc = ""
		}

		tool := mcp.Tool{
			Name: toolConfigCopy.Name,
			Description: toolConfigCopy.Description,
			ShortDescription: shortDesc,
			InputSchema: e.buildInputSchema(&toolConfigCopy),
		}

		handler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
			e.logger.Info("toolhandlercall",
				zap.String("toolName", toolName),
				zap.Any("args", args),
			)
			return e.ExecuteTool(ctx, toolName, args)
		}

		mcpServer.RegisterTool(tool, handler)
		e.logger.Info("registertoolsuccessful",
			zap.String("tool", toolConfigCopy.Name),
			zap.String("command", toolConfigCopy.Command),
			zap.Int("index", i),
		)
	}

	e.logger.Info("toolregistercompleted",
		zap.Int("registeredCount", len(e.config.Tools)),
	)
}
func (e *Executor) buildCommandArgs(toolName string, toolConfig *config.ToolConfig, args map[string]interface{}) []string {
	cmdArgs := make([]string, 0)
	if len(toolConfig.Parameters) > 0 {
		hasScanType := false
		var scanTypeValue string
		if scanType, ok := args["scan_type"].(string); ok && scanType != "" {
			hasScanType = true
			scanTypeValue = scanType
		}
		if hasScanType && toolName == "nmap" {
		} else {
			cmdArgs = append(cmdArgs, toolConfig.Args...)
		}
		positionalParams := make([]config.ParameterConfig, 0)
		flagParams := make([]config.ParameterConfig, 0)

		for _, param := range toolConfig.Parameters {
			if param.Position != nil {
				positionalParams = append(positionalParams, param)
			} else {
				flagParams = append(flagParams, param)
			}
		}
		for _, param := range positionalParams {
			if param.Name == "additional_args" || param.Name == "scan_type" || param.Name == "action" {
				continue
			}
			if param.Position != nil && *param.Position == 0 {
				value := e.getParamValue(args, param)
				if value == nil && param.Default != nil {
					value = param.Default
				}
				if value != nil {
					cmdArgs = append(cmdArgs, e.formatParamValue(param, value))
				}
				break
			}
		}
		for _, param := range flagParams {
			if param.Name == "additional_args" || param.Name == "scan_type" || param.Name == "action" {
				continue
			}

			value := e.getParamValue(args, param)
			if value == nil {
				if param.Required {
					e.logger.Warn("",
						zap.String("tool", toolName),
						zap.String("param", param.Name),
					)
					return []string{}
				}
				continue
			}
			if param.Type == "bool" {
				var boolVal bool
				var ok bool
				if boolVal, ok = value.(bool); ok {
				} else if numVal, ok := value.(float64); ok {
					boolVal = numVal != 0
					ok = true
				} else if numVal, ok := value.(int); ok {
					boolVal = numVal != 0
					ok = true
				} else if strVal, ok := value.(string); ok {
					boolVal = strVal == "true" || strVal == "1" || strVal == "yes"
					ok = true
				}

				if ok {
					if !boolVal {
						continue
					}
					if param.Flag != "" {
						cmdArgs = append(cmdArgs, param.Flag)
					}
					continue
				}
			}

			format := param.Format
			if format == "" {
				format = "flag"
			}

			switch format {
			case "flag":
				if param.Flag != "" {
					cmdArgs = append(cmdArgs, param.Flag)
				}
				formattedValue := e.formatParamValue(param, value)
				if formattedValue != "" {
					cmdArgs = append(cmdArgs, formattedValue)
				}
			case "combined":
				if param.Flag != "" {
					cmdArgs = append(cmdArgs, fmt.Sprintf("%s=%s", param.Flag, e.formatParamValue(param, value)))
				} else {
					cmdArgs = append(cmdArgs, e.formatParamValue(param, value))
				}
			case "template":
				if param.Template != "" {
					template := param.Template
					template = strings.ReplaceAll(template, "{flag}", param.Flag)
					template = strings.ReplaceAll(template, "{value}", e.formatParamValue(param, value))
					template = strings.ReplaceAll(template, "{name}", param.Name)
					cmdArgs = append(cmdArgs, strings.Fields(template)...)
				} else {
					if param.Flag != "" {
						cmdArgs = append(cmdArgs, param.Flag)
					}
					cmdArgs = append(cmdArgs, e.formatParamValue(param, value))
				}
			case "positional":
				cmdArgs = append(cmdArgs, e.formatParamValue(param, value))
			default:
				cmdArgs = append(cmdArgs, e.formatParamValue(param, value))
			}
		}
		maxPosition := -1
		for _, param := range positionalParams {
			if param.Position != nil && *param.Position > maxPosition {
				maxPosition = *param.Position
			}
		}
		for i := 0; i <= maxPosition; i++ {
			if i == 0 {
				continue
			}
			for _, param := range positionalParams {
				if param.Name == "additional_args" || param.Name == "scan_type" || param.Name == "action" {
					continue
				}

				if param.Position != nil && *param.Position == i {
					value := e.getParamValue(args, param)
					if value == nil {
						if param.Required {
							e.logger.Warn("",
								zap.String("tool", toolName),
								zap.String("param", param.Name),
								zap.Int("position", *param.Position),
							)
							return []string{}
						}
						if param.Default != nil {
							value = param.Default
						} else {
							break
						}
					}
					if value != nil {
						cmdArgs = append(cmdArgs, e.formatParamValue(param, value))
					}
					break
				}
			}
		}
		if additionalArgs, ok := args["additional_args"].(string); ok && additionalArgs != "" {
			additionalArgsList := e.parseAdditionalArgs(additionalArgs)
			cmdArgs = append(cmdArgs, additionalArgsList...)
		}
		if hasScanType {
			scanTypeArgs := e.parseAdditionalArgs(scanTypeValue)
			if len(scanTypeArgs) > 0 {
				insertPos := len(cmdArgs)
				for i := len(cmdArgs) - 1; i >= 0; i-- {
					if !strings.HasPrefix(cmdArgs[i], "-") {
						insertPos = i
						break
					}
				}
				newArgs := make([]string, 0, len(cmdArgs)+len(scanTypeArgs))
				newArgs = append(newArgs, cmdArgs[:insertPos]...)
				newArgs = append(newArgs, scanTypeArgs...)
				newArgs = append(newArgs, cmdArgs[insertPos:]...)
				cmdArgs = newArgs
			}
		}

		return cmdArgs
	}
	cmdArgs = append(cmdArgs, toolConfig.Args...)
	for key, value := range args {
		if key == "_tool_name" {
			continue
		}
		cmdArgs = append(cmdArgs, fmt.Sprintf("--%s", key))
		if strValue, ok := value.(string); ok {
			cmdArgs = append(cmdArgs, strValue)
		} else {
			cmdArgs = append(cmdArgs, fmt.Sprintf("%v", value))
		}
	}

	return cmdArgs
}
func (e *Executor) parseAdditionalArgs(argsStr string) []string {
	if argsStr == "" {
		return []string{}
	}

	result := make([]string, 0)
	var current strings.Builder
	inQuotes := false
	var quoteChar rune
	escapeNext := false

	runes := []rune(argsStr)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if escapeNext {
			current.WriteRune(r)
			escapeNext = false
			continue
		}

		if r == '\\' {
			if i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\'') {
				i++
				current.WriteRune(runes[i])
			} else {
				escapeNext = true
				current.WriteRune(r)
			}
			continue
		}

		if !inQuotes && (r == '"' || r == '\'') {
			inQuotes = true
			quoteChar = r
			continue
		}

		if inQuotes && r == quoteChar {
			inQuotes = false
			quoteChar = 0
			continue
		}

		if !inQuotes && (r == ' ' || r == '\t' || r == '\n') {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	if len(result) == 0 {
		result = strings.Fields(argsStr)
	}

	return result
}
func (e *Executor) getParamValue(args map[string]interface{}, param config.ParameterConfig) interface{} {
	if value, ok := args[param.Name]; ok && value != nil {
		return value
	}
	if param.Required {
		return nil
	}
	return param.Default
}
func (e *Executor) formatParamValue(param config.ParameterConfig, value interface{}) string {
	switch param.Type {
	case "bool":
		if boolVal, ok := value.(bool); ok {
			return fmt.Sprintf("%v", boolVal)
		}
		return "false"
	case "array":
		if arr, ok := value.([]interface{}); ok {
			strs := make([]string, 0, len(arr))
			for _, item := range arr {
				strs = append(strs, fmt.Sprintf("%v", item))
			}
			return strings.Join(strs, ",")
		}
		return fmt.Sprintf("%v", value)
	case "object":
		if jsonBytes, err := json.Marshal(value); err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%v", value)
	default:
		formattedValue := fmt.Sprintf("%v", value)
		if param.Name == "ports" {
			formattedValue = strings.ReplaceAll(formattedValue, " ", "")
		}
		return formattedValue
	}
}
func (e *Executor) isBackgroundCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	lastAmpersandPos := -1

	for i, r := range command {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if r == '&' && !inSingleQuote && !inDoubleQuote {
			isStandalone := false
			if i == 0 {
				isStandalone = true
			} else {
				prev := command[i-1]
				if prev == ' ' || prev == '\t' || prev == '\n' || prev == '\r' {
					isStandalone = true
				}
			}
			if isStandalone {
				if i == len(command)-1 {
					lastAmpersandPos = i
				} else {
					next := command[i+1]
					if next == ' ' || next == '\t' || next == '\n' || next == '\r' {
						lastAmpersandPos = i
					}
				}
			}
		}
	}
	if lastAmpersandPos == -1 {
		return false
	}
	afterAmpersand := strings.TrimSpace(command[lastAmpersandPos+1:])
	if afterAmpersand == "" {
		beforeAmpersand := strings.TrimSpace(command[:lastAmpersandPos])
		return beforeAmpersand != ""
	}
	return false
}
func (e *Executor) executeSystemCommand(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
	command, ok := args["command"].(string)
	if !ok {
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "error: command",
				},
			},
			IsError: true,
		}, nil
	}

	if command == "" {
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "error: commandcannot be empty",
				},
			},
			IsError: true,
		}, nil
	}
	e.logger.Warn("",
		zap.String("command", command),
	)
	shell := "sh"
	if s, ok := args["shell"].(string); ok && s != "" {
		shell = s
	}
	workDir := ""
	if wd, ok := args["workdir"].(string); ok && wd != "" {
		workDir = wd
	}
	isBackground := e.isBackgroundCommand(command)
	var cmd *exec.Cmd
	if workDir != "" {
		cmd = exec.CommandContext(ctx, shell, "-c", command)
		cmd.Dir = workDir
	} else {
		cmd = exec.CommandContext(ctx, shell, "-c", command)
	}
	e.logger.Info("",
		zap.String("command", command),
		zap.String("shell", shell),
		zap.String("workdir", workDir),
		zap.Bool("isBackground", isBackground),
	)
	if isBackground {
		commandWithoutAmpersand := strings.TrimSuffix(strings.TrimSpace(command), "&")
		commandWithoutAmpersand = strings.TrimSpace(commandWithoutAmpersand)
		pidCommand := fmt.Sprintf("%s & pid=$!; echo $pid", commandWithoutAmpersand)
		var pidCmd *exec.Cmd
		if workDir != "" {
			pidCmd = exec.CommandContext(ctx, shell, "-c", pidCommand)
			pidCmd.Dir = workDir
		} else {
			pidCmd = exec.CommandContext(ctx, shell, "-c", pidCommand)
		}
		stdout, err := pidCmd.StdoutPipe()
		if err != nil {
			e.logger.Error("createstdoutfailed",
				zap.String("command", command),
				zap.Error(err),
			)
			if err := pidCmd.Start(); err != nil {
				return &mcp.ToolResult{
					Content: []mcp.Content{
						{
							Type: "text",
							Text: fmt.Sprintf("failed to start background command: %v", err),
						},
					},
					IsError: true,
				}, nil
			}
			pid := pidCmd.Process.Pid
			go pidCmd.Wait()
			return &mcp.ToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("\n: %s\nID: %d (,fetchPIDfailed)\n\n: ,completed.", command, pid),
					},
				},
				IsError: false,
			}, nil
		}
		if err := pidCmd.Start(); err != nil {
			stdout.Close()
			e.logger.Error("failed to start background command",
				zap.String("command", command),
				zap.Error(err),
			)
			return &mcp.ToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("failed to start background command: %v", err),
					},
				},
				IsError: true,
			}, nil
		}
		reader := bufio.NewReader(stdout)
		pidLine, err := reader.ReadString('\n')
		stdout.Close()

		var actualPid int
		if err != nil && err != io.EOF {
			e.logger.Warn("PIDfailed",
				zap.String("command", command),
				zap.Error(err),
			)
			actualPid = pidCmd.Process.Pid
		} else {
			pidStr := strings.TrimSpace(pidLine)
			if parsedPid, err := strconv.Atoi(pidStr); err == nil {
				actualPid = parsedPid
			} else {
				e.logger.Warn("parsePIDfailed",
					zap.String("command", command),
					zap.String("pidLine", pidStr),
					zap.Error(err),
				)
				actualPid = pidCmd.Process.Pid
			}
		}
		go func() {
			if err := pidCmd.Wait(); err != nil {
				e.logger.Debug("shellcompleted",
					zap.String("command", command),
					zap.Error(err),
				)
			}
		}()

		e.logger.Info("",
			zap.String("command", command),
			zap.Int("actualPid", actualPid),
		)

		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("\n: %s\nID: %d\n\n: ,completed.", command, actualPid),
				},
			},
			IsError: false,
		}, nil
	}
	var output string
	var err error
	if cb, ok := ctx.Value(ToolOutputCallbackCtxKey).(ToolOutputCallback); ok && cb != nil {
		output, err = streamCommandOutput(cmd, cb)
		if err != nil && shouldRetryWithPTY(output) {
			e.logger.Info(" TTY, PTY retry")
			cmd2 := exec.CommandContext(ctx, shell, "-c", command)
			if workDir != "" {
				cmd2.Dir = workDir
			}
			applyDefaultTerminalEnv(cmd2)
			output, err = runCommandWithPTY(ctx, cmd2, cb)
		}
	} else {
		outputBytes, err2 := cmd.CombinedOutput()
		output = string(outputBytes)
		err = err2
		if err != nil && shouldRetryWithPTY(output) {
			e.logger.Info(" TTY, PTY retry")
			cmd2 := exec.CommandContext(ctx, shell, "-c", command)
			if workDir != "" {
				cmd2.Dir = workDir
			}
			applyDefaultTerminalEnv(cmd2)
			output, err = runCommandWithPTY(ctx, cmd2, nil)
		}
	}
	if err != nil {
		e.logger.Error("failed",
			zap.String("command", command),
			zap.Error(err),
			zap.String("output", string(output)),
		)
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("failed: %v\noutput: %s", err, string(output)),
				},
			},
			IsError: true,
		}, nil
	}

	e.logger.Info("successful",
		zap.String("command", command),
		zap.String("output_length", fmt.Sprintf("%d", len(output))),
	)

	return &mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: string(output),
			},
		},
		IsError: false,
	}, nil
}
func streamCommandOutput(cmd *exec.Cmd, cb ToolOutputCallback) (string, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdoutPipe.Close()
		return "", err
	}
	if err := cmd.Start(); err != nil {
		_ = stdoutPipe.Close()
		_ = stderrPipe.Close()
		return "", err
	}

	chunks := make(chan string, 64)
	var wg sync.WaitGroup
	readFn := func(r io.Reader) {
		defer wg.Done()
		br := bufio.NewReader(r)
		for {
			s, readErr := br.ReadString('\n')
			if s != "" {
				chunks <- s
			}
			if readErr != nil {
				return
			}
		}
	}

	wg.Add(2)
	go readFn(stdoutPipe)
	go readFn(stderrPipe)

	go func() {
		wg.Wait()
		close(chunks)
	}()

	var outBuilder strings.Builder
	var deltaBuilder strings.Builder
	lastFlush := time.Now()

	flush := func() {
		if deltaBuilder.Len() == 0 {
			return
		}
		cb(deltaBuilder.String())
		deltaBuilder.Reset()
		lastFlush = time.Now()
	}

	for chunk := range chunks {
		outBuilder.WriteString(chunk)
		deltaBuilder.WriteString(chunk)
		if deltaBuilder.Len() >= 2048 || time.Since(lastFlush) >= 200*time.Millisecond {
			flush()
		}
	}
	flush()
	waitErr := cmd.Wait()
	return outBuilder.String(), waitErr
}
func applyDefaultTerminalEnv(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	has := func(k string) bool {
		prefix := k + "="
		for _, e := range cmd.Env {
			if strings.HasPrefix(e, prefix) {
				return true
			}
		}
		return false
	}
	if !has("TERM") {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}
	if !has("COLUMNS") {
		cmd.Env = append(cmd.Env, "COLUMNS=256")
	}
	if !has("LINES") {
		cmd.Env = append(cmd.Env, "LINES=40")
	}
}

func shouldRetryWithPTY(output string) bool {
	o := strings.ToLower(output)
	if strings.Contains(o, "inappropriate ioctl for device") {
		return true
	}
	if strings.Contains(o, "termios.error") {
		return true
	}
	if strings.Contains(o, "not a tty") {
		return true
	}
	return false
}
func runCommandWithPTY(ctx context.Context, cmd *exec.Cmd, cb ToolOutputCallback) (string, error) {
	if runtime.GOOS == "windows" {
		if cb != nil {
			return streamCommandOutput(cmd, cb)
		}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = ptmx.Close() }()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ptmx.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		case <-done:
		}
	}()
	defer close(done)

	var outBuilder strings.Builder
	var deltaBuilder strings.Builder
	lastFlush := time.Now()
	flush := func() {
		if cb == nil || deltaBuilder.Len() == 0 {
			deltaBuilder.Reset()
			lastFlush = time.Now()
			return
		}
		cb(deltaBuilder.String())
		deltaBuilder.Reset()
		lastFlush = time.Now()
	}

	buf := make([]byte, 4096)
	for {
		n, readErr := ptmx.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			chunk = strings.ReplaceAll(chunk, "\r\n", "\n")
			chunk = strings.ReplaceAll(chunk, "\r", "\n")
			outBuilder.WriteString(chunk)
			deltaBuilder.WriteString(chunk)
			if deltaBuilder.Len() >= 2048 || time.Since(lastFlush) >= 200*time.Millisecond {
				flush()
			}
		}
		if readErr != nil {
			break
		}
	}
	flush()

	waitErr := cmd.Wait()
	return outBuilder.String(), waitErr
}
func (e *Executor) executeInternalTool(ctx context.Context, toolName string, command string, args map[string]interface{}) (*mcp.ToolResult, error) {
	internalToolType := strings.TrimPrefix(command, "internal:")

	e.logger.Info("executing internal tool",
		zap.String("toolName", toolName),
		zap.String("internalToolType", internalToolType),
		zap.Any("args", args),
	)
	switch internalToolType {
	case "query_execution_result":
		return e.executeQueryExecutionResult(ctx, args)
	default:
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("error: Unknowntool: %s", internalToolType),
				},
			},
			IsError: true,
		}, nil
	}
}
func (e *Executor) executeQueryExecutionResult(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
	executionID, ok := args["execution_id"].(string)
	if !ok || executionID == "" {
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "error: execution_id cannot be empty",
				},
			},
			IsError: true,
		}, nil
	}
	page := 1
	if p, ok := args["page"].(float64); ok {
		page = int(p)
	}
	if page < 1 {
		page = 1
	}

	limit := 100
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	search := ""
	if s, ok := args["search"].(string); ok {
		search = s
	}

	filter := ""
	if f, ok := args["filter"].(string); ok {
		filter = f
	}

	useRegex := false
	if r, ok := args["use_regex"].(bool); ok {
		useRegex = r
	}
	if e.resultStorage == nil {
		return &mcp.ToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "error: resultinitialize",
				},
			},
			IsError: true,
		}, nil
	}
	var resultPage *storage.ResultPage
	var err error

	if search != "" {
		matchedLines, err := e.resultStorage.SearchResult(executionID, search, useRegex)
		if err != nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("search failed: %v", err),
					},
				},
				IsError: true,
			}, nil
		}
		resultPage = paginateLines(matchedLines, page, limit)
	} else if filter != "" {
		filteredLines, err := e.resultStorage.FilterResult(executionID, filter, useRegex)
		if err != nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("failed: %v", err),
					},
				},
				IsError: true,
			}, nil
		}
		resultPage = paginateLines(filteredLines, page, limit)
	} else {
		resultPage, err = e.resultStorage.GetResultPage(executionID, page, limit)
		if err != nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("queryfailed: %v", err),
					},
				},
				IsError: true,
			}, nil
		}
	}
	metadata, err := e.resultStorage.GetResultMetadata(executionID)
	if err != nil {
		e.logger.Warn("fetchresultfailed", zap.Error(err))
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("queryresult (ID: %s)\n", executionID))

	if metadata != nil {
		sb.WriteString(fmt.Sprintf("tool: %s | : %d (%.2f KB) | : %d\n",
			metadata.ToolName, metadata.TotalSize, float64(metadata.TotalSize)/1024, metadata.TotalLines))
	}

	sb.WriteString(fmt.Sprintf(" %d/%d , %d , %d \n\n",
		resultPage.Page, resultPage.TotalPages, resultPage.Limit, resultPage.TotalLines))

	if len(resultPage.Lines) == 0 {
		sb.WriteString("noresult.\n")
	} else {
		for i, line := range resultPage.Lines {
			lineNum := (resultPage.Page-1)*resultPage.Limit + i + 1
			sb.WriteString(fmt.Sprintf("%d: %s\n", lineNum, line))
		}
	}

	sb.WriteString("\n")
	if resultPage.Page < resultPage.TotalPages {
		sb.WriteString(fmt.Sprintf("hint: page=%d ", resultPage.Page+1))
		if search != "" {
			sb.WriteString(fmt.Sprintf(", search=\"%s\" ", search))
			if useRegex {
				sb.WriteString(" (regex mode)")
			}
		}
		if filter != "" {
			sb.WriteString(fmt.Sprintf(", filter=\"%s\" ", filter))
			if useRegex {
				sb.WriteString(" (regex mode)")
			}
		}
		sb.WriteString("\n")
	}

	return &mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: sb.String(),
			},
		},
		IsError: false,
	}, nil
}
func paginateLines(lines []string, page int, limit int) *storage.ResultPage {
	totalLines := len(lines)
	totalPages := (totalLines + limit - 1) / limit
	if page < 1 {
		page = 1
	}
	if page > totalPages && totalPages > 0 {
		page = totalPages
	}

	start := (page - 1) * limit
	end := start + limit
	if end > totalLines {
		end = totalLines
	}

	var pageLines []string
	if start < totalLines {
		pageLines = lines[start:end]
	} else {
		pageLines = []string{}
	}

	return &storage.ResultPage{
		Lines: pageLines,
		Page: page,
		Limit: limit,
		TotalLines: totalLines,
		TotalPages: totalPages,
	}
}
func (e *Executor) buildInputSchema(toolConfig *config.ToolConfig) map[string]interface{} {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{},
		"required": []string{},
	}
	if len(toolConfig.Parameters) > 0 {
		properties := make(map[string]interface{})
		required := []string{}

		for _, param := range toolConfig.Parameters {
			if strings.TrimSpace(param.Name) == "" {
				e.logger.Debug("name",
					zap.String("tool", toolConfig.Name),
					zap.String("type", param.Type),
				)
				continue
			}
			openAIType := e.convertToOpenAIType(param.Type)

			prop := map[string]interface{}{
				"type": openAIType,
				"description": param.Description,
			}
			if openAIType == "array" {
				itemType := strings.TrimSpace(param.ItemType)
				if itemType == "" {
					itemType = "string"
				}
				prop["items"] = map[string]interface{}{
					"type": e.convertToOpenAIType(itemType),
				}
			}
			if param.Default != nil {
				prop["default"] = param.Default
			}
			if len(param.Options) > 0 {
				prop["enum"] = param.Options
			}

			properties[param.Name] = prop
			if param.Required {
				required = append(required, param.Name)
			}
		}

		schema["properties"] = properties
		schema["required"] = required
		return schema
	}
	e.logger.Warn("toolconfig,backschema",
		zap.String("tool", toolConfig.Name),
	)
	return schema
}
func (e *Executor) convertToOpenAIType(configType string) string {
	if strings.TrimSpace(configType) == "" {
		return "string"
	}
	switch configType {
	case "bool":
		return "boolean"
	case "int", "integer":
		return "number"
	case "float", "double":
		return "number"
	case "string", "array", "object":
		return configType
	default:
		e.logger.Warn("Unknown,",
			zap.String("type", configType),
		)
		return configType
	}
}
func getExitCode(err error) *int {
	if err == nil {
		return nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ProcessState != nil {
			exitCode := exitError.ExitCode()
			return &exitCode
		}
	}
	return nil
}
func getExitCodeValue(err error) int {
	if code := getExitCode(err); code != nil {
		return *code
	}
	return -1
}
