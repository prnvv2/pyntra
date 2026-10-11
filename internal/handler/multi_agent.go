package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"pyntra/internal/config"
	"pyntra/internal/multiagent"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)
func (h *AgentHandler) MultiAgentLoopStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	if h.config == nil || !h.config.MultiAgent.Enabled {
		ev := StreamEvent{Type: "error", Message: "multi-agentenable,or config.yaml in multi_agent.enabled"}
		b, _ := json.Marshal(ev)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		done := StreamEvent{Type: "done", Message: ""}
		db, _ := json.Marshal(done)
		fmt.Fprintf(c.Writer, "data: %s\n\n", db)
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		event := StreamEvent{Type: "error", Message: "request parameter error: " + err.Error()}
		b, _ := json.Marshal(event)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		c.Writer.Flush()
		return
	}

	c.Header("X-Accel-Buffering", "no")
	var baseCtx context.Context

	clientDisconnected := false
	// shared with sseKeepalive: prohibit concurrent writes to ResponseWriter, otherwise will corrupt chunked encoding (ERR_INVALID_CHUNKED_ENCODING).
	var sseWriteMu sync.Mutex
	sendEvent := func(eventType, message string, data interface{}) {
		if clientDisconnected {
			return
		}
		if eventType == "error" && baseCtx != nil && errors.Is(context.Cause(baseCtx), ErrTaskCancelled) {
			return
		}
		select {
		case <-c.Request.Context().Done():
			clientDisconnected = true
			return
		default:
		}
		ev := StreamEvent{Type: eventType, Message: message, Data: data}
		b, _ := json.Marshal(ev)
		sseWriteMu.Lock()
		_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		if err != nil {
			sseWriteMu.Unlock()
			clientDisconnected = true
			return
		}
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		} else {
			c.Writer.Flush()
		}
		sseWriteMu.Unlock()
	}

	h.logger.Info(" Eino DeepAgent request",
		zap.String("conversationId", req.ConversationID),
	)

	prep, err := h.prepareMultiAgentSession(&req)
	if err != nil {
		sendEvent("error", err.Error(), nil)
		sendEvent("done", "", nil)
		return
	}
	if prep.CreatedNew {
		sendEvent("conversation", "session created", map[string]interface{}{
			"conversationId": prep.ConversationID,
		})
	}

	conversationID := prep.ConversationID
	assistantMessageID := prep.AssistantMessageID

	if prep.UserMessageID != "" {
		sendEvent("message_saved", "", map[string]interface{}{
			"conversationId": conversationID,
			"userMessageId": prep.UserMessageID,
		})
	}

	progressCallback := h.createProgressCallback(conversationID, assistantMessageID, sendEvent)

	baseCtx, cancelWithCause := context.WithCancelCause(context.Background())
	taskCtx, timeoutCancel := context.WithTimeout(baseCtx, 600*time.Minute)
	defer timeoutCancel()
	defer cancelWithCause(nil)

	if _, err := h.tasks.StartTask(conversationID, req.Message, cancelWithCause); err != nil {
		var errorMsg string
		if errors.Is(err, ErrTaskAlreadyRunning) {
			errorMsg = "⚠️ currenthastaskexecutein,currenttaskcompletedor「stoptask」."
			sendEvent("error", errorMsg, map[string]interface{}{
				"conversationId": conversationID,
				"errorType": "task_already_running",
			})
		} else {
			errorMsg = "❌ unable to start task: " + err.Error()
			sendEvent("error", errorMsg, nil)
		}
		if assistantMessageID != "" {
			_, _ = h.db.Exec("UPDATE messages SET content = ? WHERE id = ?", errorMsg, assistantMessageID)
		}
		sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
		return
	}

	taskStatus := "completed"
	defer h.tasks.FinishTask(conversationID, taskStatus)

	sendEvent("progress", "start Eino multi-agent...", map[string]interface{}{
		"conversationId": conversationID,
	})

	stopKeepalive := make(chan struct{})
	go sseKeepalive(c, stopKeepalive, &sseWriteMu)
	defer close(stopKeepalive)

	var result *multiagent.RunResult
	var runErr error
	if isFanoutOrchestration(req.Orchestration) {
		result, runErr = h.runFanout(taskCtx, conversationID, prep.FinalMessage, prep.History, prep.RoleTools, progressCallback)
	} else {
		result, runErr = multiagent.RunDeepAgent(
			taskCtx,
			h.config,
			&h.config.MultiAgent,
			h.agent,
			h.logger,
			conversationID,
			prep.FinalMessage,
			prep.History,
			prep.RoleTools,
			progressCallback,
			h.agentsMarkdownDir,
			strings.TrimSpace(req.Orchestration),
		)
	}

	if runErr != nil {
		cause := context.Cause(baseCtx)
		if errors.Is(cause, ErrTaskCancelled) {
			taskStatus = "cancelled"
			h.tasks.UpdateTaskStatus(conversationID, taskStatus)
			cancelMsg := "task has been cancelled by user, subsequent operations have stopped."
			if assistantMessageID != "" {
				_, _ = h.db.Exec("UPDATE messages SET content = ? WHERE id = ?", cancelMsg, assistantMessageID)
				_ = h.db.AddProcessDetail(assistantMessageID, conversationID, "cancelled", cancelMsg, nil)
			}
			sendEvent("cancelled", cancelMsg, map[string]interface{}{
				"conversationId": conversationID,
				"messageId": assistantMessageID,
			})
			sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
			return
		}

		h.logger.Error("Eino DeepAgent execution failed", zap.Error(runErr))
		taskStatus = "failed"
		h.tasks.UpdateTaskStatus(conversationID, taskStatus)
		errMsg := "execution failed: " + runErr.Error()
		if assistantMessageID != "" {
			_, _ = h.db.Exec("UPDATE messages SET content = ? WHERE id = ?", errMsg, assistantMessageID)
			_ = h.db.AddProcessDetail(assistantMessageID, conversationID, "error", errMsg, nil)
		}
		sendEvent("error", errMsg, map[string]interface{}{
			"conversationId": conversationID,
			"messageId": assistantMessageID,
		})
		sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
		return
	}

	if assistantMessageID != "" {
		mcpIDsJSON := ""
		if len(result.MCPExecutionIDs) > 0 {
			jsonData, _ := json.Marshal(result.MCPExecutionIDs)
			mcpIDsJSON = string(jsonData)
		}
		_, _ = h.db.Exec(
			"UPDATE messages SET content = ?, mcp_execution_ids = ? WHERE id = ?",
			result.Response,
			mcpIDsJSON,
			assistantMessageID,
		)
	}

	if result.LastReActInput != "" || result.LastReActOutput != "" {
		if err := h.db.SaveReActData(conversationID, result.LastReActInput, result.LastReActOutput); err != nil {
			h.logger.Warn("save ReAct datafailed", zap.Error(err))
		}
	}

	effectiveOrch := config.NormalizeMultiAgentOrchestration(h.config.MultiAgent.Orchestration)
	if o := strings.TrimSpace(req.Orchestration); o != "" {
		if isFanoutOrchestration(o) {
			effectiveOrch = "fanout"
		} else {
			effectiveOrch = config.NormalizeMultiAgentOrchestration(o)
		}
	}
	sendEvent("response", result.Response, map[string]interface{}{
		"mcpExecutionIds": result.MCPExecutionIDs,
		"conversationId": conversationID,
		"messageId": assistantMessageID,
		"agentMode": "eino_" + effectiveOrch,
	})
	sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
}
func (h *AgentHandler) MultiAgentLoop(c *gin.Context) {
	if h.config == nil || !h.config.MultiAgent.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "multi-agentenable, config.yaml in multi_agent.enabled: true"})
		return
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info(" Eino DeepAgent request", zap.String("conversationId", req.ConversationID))

	prep, err := h.prepareMultiAgentSession(&req)
	if err != nil {
		status, msg := multiAgentHTTPErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}

	var result *multiagent.RunResult
	var runErr error
	if isFanoutOrchestration(req.Orchestration) {
		result, runErr = h.runFanout(c.Request.Context(), prep.ConversationID, prep.FinalMessage, prep.History, prep.RoleTools, nil)
	} else {
		result, runErr = multiagent.RunDeepAgent(
			c.Request.Context(),
			h.config,
			&h.config.MultiAgent,
			h.agent,
			h.logger,
			prep.ConversationID,
			prep.FinalMessage,
			prep.History,
			prep.RoleTools,
			nil,
			h.agentsMarkdownDir,
			strings.TrimSpace(req.Orchestration),
		)
	}
	if runErr != nil {
		h.logger.Error("Eino DeepAgent execution failed", zap.Error(runErr))
		errMsg := "execution failed: " + runErr.Error()
		if prep.AssistantMessageID != "" {
			_, _ = h.db.Exec("UPDATE messages SET content = ? WHERE id = ?", errMsg, prep.AssistantMessageID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	if prep.AssistantMessageID != "" {
		mcpIDsJSON := ""
		if len(result.MCPExecutionIDs) > 0 {
			jsonData, _ := json.Marshal(result.MCPExecutionIDs)
			mcpIDsJSON = string(jsonData)
		}
		_, _ = h.db.Exec(
			"UPDATE messages SET content = ?, mcp_execution_ids = ? WHERE id = ?",
			result.Response,
			mcpIDsJSON,
			prep.AssistantMessageID,
		)
	}

	if result.LastReActInput != "" || result.LastReActOutput != "" {
		if err := h.db.SaveReActData(prep.ConversationID, result.LastReActInput, result.LastReActOutput); err != nil {
			h.logger.Warn("save ReAct datafailed", zap.Error(err))
		}
	}

	c.JSON(http.StatusOK, ChatResponse{
		Response: result.Response,
		MCPExecutionIDs: result.MCPExecutionIDs,
		ConversationID: prep.ConversationID,
		Time: time.Now(),
	})
}

func multiAgentHTTPErrorStatus(err error) (int, string) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "conversation does not exist"):
		return http.StatusNotFound, msg
	case strings.Contains(msg, "not found WebShell"):
		return http.StatusBadRequest, msg
	case strings.Contains(msg, "maximum attachments"):
		return http.StatusBadRequest, msg
	case strings.Contains(msg, "failed to save user message"), strings.Contains(msg, "failed to create conversation"):
		return http.StatusInternalServerError, msg
	case strings.Contains(msg, "failed to save uploaded file"):
		return http.StatusInternalServerError, msg
	default:
		return http.StatusBadRequest, msg
	}
}
