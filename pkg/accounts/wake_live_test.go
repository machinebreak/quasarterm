// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveWakeAndLocalAPI is an opt-in end-to-end check: it imports the
// currently logged-in CLI accounts (real codex/claude logins), wakes them
// through their real CLIs, and serves real requests through the local Codex
// API. It consumes a small amount of real quota and never touches the live
// data dir (everything runs against the temp store from TestMain).
//
// Run with: QUASAR_LIVE_CHECK=1 go test ./pkg/accounts/ -run TestLiveWake -v
func TestLiveWakeAndLocalAPI(t *testing.T) {
	if os.Getenv("QUASAR_LIVE_CHECK") != "1" {
		t.Skip("set QUASAR_LIVE_CHECK=1 to run the live wake/local-API check")
	}
	manager := GetManager()

	// --- wake via the real CLIs -----------------------------------------
	wakeChecked := 0
	for _, providerID := range []string{"codex", "claude"} {
		account, err := manager.ImportCurrent(providerID)
		if err != nil {
			t.Logf("skipping %s: no live CLI login (%v)", providerID, err)
			continue
		}
		task, err := manager.CreateWakeTask(providerID, account.ID, 60, "")
		if err != nil {
			t.Fatalf("could not create wake task for %s: %v", providerID, err)
		}
		run, err := manager.RunWakeTask(task.ID)
		if err != nil {
			t.Fatalf("wake run for %s failed to start: %v", providerID, err)
		}
		t.Logf("%s wake: status=%s duration=%dms output=%q error=%q", providerID, run.Status, run.DurationMs, run.Output, run.Error)
		if run.Status != "ok" {
			// The CLI ran and the service answered (e.g. quota exhausted or the
			// org disabled access) — that is a valid wake outcome; the pipeline
			// itself is what this check verifies.
			t.Logf("%s wake ran but the account is not usable right now: %s", providerID, run.Output)
		}
		if run.DurationMs == 0 || (run.Output == "" && run.Error == "") {
			t.Fatalf("wake for %s did not actually run (duration=%dms output=%q error=%q)", providerID, run.DurationMs, run.Output, run.Error)
		}
		runs, err := manager.ListWakeRuns(task.ID, 5)
		if err != nil || len(runs) != 1 {
			t.Fatalf("expected 1 recorded run for %s, got %d (err=%v)", providerID, len(runs), err)
		}
		wakeChecked++
	}
	if wakeChecked == 0 {
		t.Skip("no live CLI logins found (codex/claude)")
	}

	// --- local codex API -------------------------------------------------
	if _, err := manager.ImportCurrent("codex"); err != nil {
		t.Skipf("no live codex login for the API check: %v", err)
	}
	if err := manager.startLocalAPI(8321, ""); err != nil {
		t.Fatalf("could not start the local API: %v", err)
	}
	defer manager.stopLocalAPI()
	client := &http.Client{Timeout: 10 * time.Minute}

	resp, err := client.Get("http://127.0.0.1:8321/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health returned %d", resp.StatusCode)
	}

	resp, err = client.Get("http://127.0.0.1:8321/v1/models")
	if err != nil {
		t.Fatalf("models request failed: %v", err)
	}
	modelsBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("models returned %d: %s", resp.StatusCode, modelsBody)
	}
	var modelsParsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(modelsBody, &modelsParsed); err != nil || len(modelsParsed.Data) == 0 {
		t.Fatalf("models response had no models: %s", summarizeLive(modelsBody))
	}
	modelID := modelsParsed.Data[0].ID
	t.Logf("models: %s (using %s)", strings.TrimSpace(string(modelsBody)), modelID)

	requestBody := fmt.Sprintf(`{"model":%q,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Reply with exactly: OK"}]}],"stream":false,"store":false}`, modelID)
	resp, err = client.Post("http://127.0.0.1:8321/v1/responses", "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("responses request failed: %v", err)
	}
	responsesBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	t.Logf("responses: status=%d body=%s", resp.StatusCode, summarizeLive(responsesBody))
	if resp.StatusCode != http.StatusOK {
		if !upstreamVerdict(responsesBody) {
			t.Fatalf("responses returned %d: %s", resp.StatusCode, summarizeLive(responsesBody))
		}
	} else if !strings.Contains(string(responsesBody), "\"output\"") {
		t.Fatalf("responses payload missing output: %s", summarizeLive(responsesBody))
	}

	chatBody := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Reply with exactly: OK"}],"stream":false}`, modelID)
	resp, err = client.Post("http://127.0.0.1:8321/v1/chat/completions", "application/json", strings.NewReader(chatBody))
	if err != nil {
		t.Fatalf("chat completions request failed: %v", err)
	}
	chatRespBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	t.Logf("chat.completions: status=%d body=%s", resp.StatusCode, summarizeLive(chatRespBody))
	if resp.StatusCode != http.StatusOK {
		if !upstreamVerdict(chatRespBody) {
			t.Fatalf("chat completions returned %d: %s", resp.StatusCode, summarizeLive(chatRespBody))
		}
	} else {
		var chatParsed map[string]any
		if err := json.Unmarshal(chatRespBody, &chatParsed); err != nil {
			t.Fatalf("chat completions returned invalid json: %v", err)
		}
		choices, _ := chatParsed["choices"].([]any)
		if len(choices) == 0 {
			t.Fatalf("chat completions returned no choices: %s", summarizeLive(chatRespBody))
		}
	}
}

func summarizeLive(data []byte) string {
	if len(data) > 600 {
		data = data[:600]
	}
	return strings.ReplaceAll(string(data), "\n", " ")
}

// upstreamVerdict reports whether a non-200 local-API response is a verdict
// from the Codex backend (quota/model/auth message passed through) rather than
// a failure of the proxy itself.
func upstreamVerdict(body []byte) bool {
	lower := strings.ToLower(string(body))
	proxyFailures := []string{
		"no codex account",
		"all codex accounts are unavailable",
		"could not read request body",
		"request body must be valid json",
		"streaming is not supported",
	}
	for _, failure := range proxyFailures {
		if strings.Contains(lower, failure) {
			return false
		}
	}
	return strings.Contains(lower, "detail") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "usage limit") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "quota")
}
