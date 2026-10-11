package handler

import (
	"context"
	"strings"

	"pyntra/internal/agent"
	"pyntra/internal/multiagent"
	"pyntra/internal/multiagent/fanout"
)

// isFanoutOrchestration reports whether the requested orchestration selects the
// concurrent breadth+depth fan-out mode.
func isFanoutOrchestration(orchestration string) bool {
	return strings.EqualFold(strings.TrimSpace(orchestration), "fanout")
}

// runFanout executes a fanout round, adapting the orchestrator's result to the
// multiagent.RunResult the handlers already consume.
func (h *AgentHandler) runFanout(
	ctx context.Context,
	conversationID string,
	finalMessage string,
	history []agent.ChatMessage,
	reconRoleTools []string,
	progress func(eventType, message string, data interface{}),
) (*multiagent.RunResult, error) {
	orch := fanout.NewOrchestrator(fanout.Deps{
		AppCfg:   h.config,
		Fanout:   h.config.MultiAgent.Fanout,
		Agent:    h.agent,
		Executor: h.executor,
		Enricher: h.enricher,
		DB:       h.db,
		Logger:   h.logger,
	})

	res, err := orch.Run(ctx, conversationID, finalMessage, history, reconRoleTools, progress)
	if err != nil {
		return nil, err
	}
	return &multiagent.RunResult{
		Response:        res.Response,
		MCPExecutionIDs: res.MCPExecutionIDs,
	}, nil
}
