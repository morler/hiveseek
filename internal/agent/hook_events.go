package agent

import (
	"context"
	"strings"
)

func (a *Agent) hookRunObserver(ctx context.Context, events chan<- AgentEvent) HookRunObserver {
	return func(stage HookRunStage, info HookEventInfo) {
		var typ AgentEventType
		switch stage {
		case HookRunStarted:
			typ = AgentEventTypeHookStarted
		case HookRunBlocked:
			typ = AgentEventTypeHookBlocked
		case HookRunWarned:
			typ = AgentEventTypeHookWarned
		case HookRunFailed:
			typ = AgentEventTypeHookFailed
		default:
			typ = AgentEventTypeHookCompleted
		}
		_ = sendAgentEvent(ctx, events, AgentEvent{Type: typ, Hook: &info})
	}
}

func hookOutcomeName(oc HookOutcome) string {
	if strings.TrimSpace(oc.Name) != "" {
		return strings.TrimSpace(oc.Name)
	}
	return strings.TrimSpace(oc.Hook.Command)
}

func hookOutcomeMessage(oc HookOutcome) string {
	for _, value := range []string{oc.Message, oc.Stderr, oc.Stdout, oc.AdditionalContext} {
		if msg := strings.TrimSpace(value); msg != "" {
			return msg
		}
	}
	return ""
}
