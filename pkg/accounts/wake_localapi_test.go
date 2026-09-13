// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"testing"
	"time"
)

func TestClampWakeInterval(t *testing.T) {
	if got := clampWakeInterval(1); got != wakeMinIntervalMinutes {
		t.Fatalf("expected clamp to min, got %d", got)
	}
	if got := clampWakeInterval(45); got != 45 {
		t.Fatalf("expected 45, got %d", got)
	}
	if got := clampWakeInterval(999999); got != wakeMaxIntervalMinutes {
		t.Fatalf("expected clamp to max, got %d", got)
	}
}

func TestWakeTaskDue(t *testing.T) {
	now := time.Now()
	task := WakeTask{Enabled: true, IntervalMinutes: 60}
	if !wakeTaskDue(task, now) {
		t.Fatal("a task that never ran should be due")
	}
	task.LastRun = now.Add(-30 * time.Minute).UnixMilli()
	if wakeTaskDue(task, now) {
		t.Fatal("a task that ran 30m ago with a 60m interval should not be due")
	}
	task.LastRun = now.Add(-61 * time.Minute).UnixMilli()
	if !wakeTaskDue(task, now) {
		t.Fatal("a task that ran 61m ago with a 60m interval should be due")
	}
	task.Enabled = false
	task.LastRun = 0
	if wakeTaskDue(task, now) {
		t.Fatal("disabled tasks should never be due")
	}
}

func TestWakeTaskStoreRoundTrip(t *testing.T) {
	manager := GetManager()
	seed := `{
  "version": 1,
  "accounts": [
    {
      "id": "wake-account-1",
      "provider": "claude",
      "email": "wake@example.com",
      "credentials": {"claudeAiOauth": {"accessToken": "wake-token"}}
    }
  ]
}`
	if _, err := manager.ImportAccounts(seed); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if _, err := manager.CreateWakeTask("cursor", "wake-account-1", 60, ""); err == nil {
		t.Fatal("cursor does not support wake tasks, expected an error")
	}
	task, err := manager.CreateWakeTask("claude", "wake-account-1", 5, " haiku ")
	if err != nil {
		t.Fatalf("create wake task failed: %v", err)
	}
	if task.IntervalMinutes != wakeMinIntervalMinutes {
		t.Fatalf("expected interval clamped to %d, got %d", wakeMinIntervalMinutes, task.IntervalMinutes)
	}
	if !task.Enabled || task.Model != "haiku" {
		t.Fatalf("unexpected task defaults: %+v", task)
	}
	if _, err := manager.CreateWakeTask("claude", "wake-account-1", 60, ""); err == nil {
		t.Fatal("duplicate wake task for the same account should be rejected")
	}
	updated, err := manager.UpdateWakeTask(task.ID, false, 120, "sonnet")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Enabled || updated.IntervalMinutes != 120 || updated.Model != "sonnet" {
		t.Fatalf("unexpected updated task: %+v", updated)
	}
	tasks, err := manager.ListWakeTasks("claude")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d (err=%v)", len(tasks), err)
	}
	if tasks, _ := manager.ListWakeTasks("codex"); len(tasks) != 0 {
		t.Fatalf("expected no codex tasks, got %d", len(tasks))
	}
	runs, err := manager.ListWakeRuns(task.ID, 10)
	if err != nil || len(runs) != 0 {
		t.Fatalf("expected no runs, got %d (err=%v)", len(runs), err)
	}
	if err := manager.DeleteWakeTask(task.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if tasks, _ := manager.ListWakeTasks("claude"); len(tasks) != 0 {
		t.Fatalf("expected task list to be empty after delete, got %d", len(tasks))
	}
}

func TestChatToResponsesRequest(t *testing.T) {
	req := chatCompletionRequest{
		Model: "gpt-5-codex",
		Messages: []chatCompletionMessage{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		},
	}
	converted := chatToResponsesRequest(req)
	if converted["model"] != "gpt-5-codex" {
		t.Fatalf("unexpected model: %v", converted["model"])
	}
	if converted["store"] != false || converted["stream"] != true {
		t.Fatal("expected store=false and stream=true (the Codex backend requires streaming)")
	}
	if converted["instructions"] != "be brief" {
		t.Fatalf("system message should map to instructions, got %v", converted["instructions"])
	}
	input, ok := converted["input"].([]map[string]any)
	if !ok || len(input) != 2 {
		t.Fatalf("expected 2 input messages, got %#v", converted["input"])
	}
	if input[0]["role"] != "user" || input[1]["role"] != "assistant" {
		t.Fatalf("unexpected roles: %v / %v", input[0]["role"], input[1]["role"])
	}
	parts, _ := input[1]["content"].([]map[string]any)
	if len(parts) != 1 || parts[0]["type"] != "output_text" {
		t.Fatalf("assistant content should use output_text parts, got %#v", input[1]["content"])
	}
}

func TestResponsesToChatCompletion(t *testing.T) {
	resp := map[string]any{
		"id":    "resp_1",
		"model": "gpt-5-codex",
		"output": []any{
			map[string]any{
				"type": "message",
				"content": []any{
					map[string]any{"type": "output_text", "text": "hola"},
				},
			},
		},
		"usage": map[string]any{"input_tokens": float64(7), "output_tokens": float64(2)},
	}
	completion, err := responsesToChatCompletion("", resp)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}
	choices, _ := completion["choices"].([]map[string]any)
	if len(choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(choices))
	}
	message, _ := choices[0]["message"].(map[string]any)
	if message["content"] != "hola" {
		t.Fatalf("unexpected content: %v", message["content"])
	}
	usage, _ := completion["usage"].(map[string]int)
	if usage["total_tokens"] != 9 {
		t.Fatalf("expected total_tokens 9, got %v", usage["total_tokens"])
	}
	if _, err := responsesToChatCompletion("", map[string]any{"output": []any{}}); err == nil {
		t.Fatal("expected an error when the response has no text output")
	}
}
