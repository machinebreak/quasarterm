// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wconfig"
)

const localAPIDefaultPort = 8318
const codexResponsesURL = "https://chatgpt.com/backend-api/codex/responses"
const codexModelsURL = "https://chatgpt.com/backend-api/codex/models"
const localAPIMaxBody = 8 << 20

var codexVersionOnce sync.Once
var codexVersionCached string

// codexCLIVersion returns the installed codex CLI version (cached); the models
// endpoint requires it as client_version.
func codexCLIVersion() string {
	codexVersionOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd.exe", "/c", "codex", "--version")
		} else {
			cmd = exec.CommandContext(ctx, "codex", "--version")
		}
		out, err := cmd.Output()
		if err == nil {
			if match := regexp.MustCompile(`(\d+\.\d+\.\d+)`).FindString(string(out)); match != "" {
				codexVersionCached = match
			}
		}
		if codexVersionCached == "" {
			codexVersionCached = "0.154.0"
		}
	})
	return codexVersionCached
}

func codexRequestHeaders(req *http.Request, tokens *codexAuthTokens, account *Account, targetPath string) {
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", codexWebUserAgent)
	req.Header.Set("Referer", codexWebReferer)
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("x-openai-target-path", targetPath)
	req.Header.Set("x-openai-target-route", targetPath)
	if account != nil && account.RemoteID != "" {
		req.Header.Set("ChatGPT-Account-Id", account.RemoteID)
	}
}

// LocalAPIStatus describes the local OpenAI-compatible endpoint backed by the
// managed Codex accounts.
type LocalAPIStatus struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	BaseURL string `json:"baseurl"`
	APIKey  string `json:"apikey,omitempty"`
	Error   string `json:"error,omitempty"`
}

type localAPIRuntime struct {
	mu       sync.Mutex
	server   *http.Server
	listener net.Listener
	port     int
	apiKey   string
	lastErr  string
}

var localAPI localAPIRuntime

func localAPISettings() (bool, int, string) {
	settings := wconfig.GetWatcher().GetFullConfig().Settings
	enabled := false
	if settings.AccountsCodexAPI != nil {
		enabled = *settings.AccountsCodexAPI
	}
	port := localAPIDefaultPort
	if settings.AccountsCodexAPIPort != nil && *settings.AccountsCodexAPIPort > 0 {
		port = *settings.AccountsCodexAPIPort
	}
	apiKey := ""
	if settings.AccountsCodexAPIKey != nil {
		apiKey = strings.TrimSpace(*settings.AccountsCodexAPIKey)
	}
	return enabled, port, apiKey
}

func (m *Manager) LocalAPIStatus() LocalAPIStatus {
	enabled, port, apiKey := localAPISettings()
	localAPI.mu.Lock()
	defer localAPI.mu.Unlock()
	status := LocalAPIStatus{
		Enabled: enabled,
		Running: localAPI.server != nil,
		Port:    port,
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", port),
		APIKey:  apiKey,
		Error:   localAPI.lastErr,
	}
	return status
}

// ApplyLocalAPISettings persists the local API settings and (re)starts or
// stops the endpoint immediately.
func (m *Manager) ApplyLocalAPISettings(enabled bool, port int, apiKey string) (LocalAPIStatus, error) {
	if port <= 0 || port > 65535 {
		port = localAPIDefaultPort
	}
	toMerge := waveobj.MetaMapType{
		"accounts:codexapi":     enabled,
		"accounts:codexapiport": port,
		"accounts:codexapikey":  strings.TrimSpace(apiKey),
	}
	if err := wconfig.SetBaseConfigValue(toMerge); err != nil {
		return LocalAPIStatus{}, err
	}
	if enabled {
		if err := m.startLocalAPI(port, strings.TrimSpace(apiKey)); err != nil {
			return m.LocalAPIStatus(), err
		}
	} else {
		m.stopLocalAPI()
	}
	return m.LocalAPIStatus(), nil
}

func (m *Manager) StartLocalAPIFromSettings() {
	enabled, port, apiKey := localAPISettings()
	if !enabled {
		return
	}
	if err := m.startLocalAPI(port, apiKey); err != nil {
		localAPI.mu.Lock()
		localAPI.lastErr = err.Error()
		localAPI.mu.Unlock()
	}
}

func (m *Manager) startLocalAPI(port int, apiKey string) error {
	m.stopLocalAPI()
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		localAPI.mu.Lock()
		localAPI.lastErr = err.Error()
		localAPI.mu.Unlock()
		return fmt.Errorf("could not listen on 127.0.0.1:%d: %w", port, err)
	}
	server := &http.Server{Handler: &localAPIHandler{manager: m, apiKey: apiKey}}
	localAPI.mu.Lock()
	localAPI.server = server
	localAPI.listener = listener
	localAPI.port = port
	localAPI.apiKey = apiKey
	localAPI.lastErr = ""
	localAPI.mu.Unlock()
	go func() {
		defer func() {
			recover()
		}()
		server.Serve(listener)
	}()
	return nil
}

func (m *Manager) stopLocalAPI() {
	localAPI.mu.Lock()
	server := localAPI.server
	localAPI.server = nil
	localAPI.listener = nil
	localAPI.mu.Unlock()
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}
}

type localAPIHandler struct {
	manager *Manager
	apiKey  string
}

func (h *localAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			http.Error(w, fmt.Sprintf("internal error: %v", rec), http.StatusInternalServerError)
		}
	}()
	if h.apiKey != "" {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if strings.TrimSpace(token) != h.apiKey {
			writeJSONError(w, http.StatusUnauthorized, "invalid API key")
			return
		}
	}
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/health" || r.URL.Path == "/v1/health"):
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		h.handleModels(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		h.handleResponses(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		h.handleChatCompletions(w, r)
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

type codexModelInfo struct {
	Slug           string `json:"slug"`
	DisplayName    string `json:"display_name"`
	Priority       int    `json:"priority"`
	Visibility     string `json:"visibility"`
	SupportedInAPI bool   `json:"supported_in_api"`
}

func (h *localAPIHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	account, credentials, err := h.manager.pickCodexAccount(nil)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	_, tokens, err := (&CodexProvider{}).ensureFreshTokens(r.Context(), credentials)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("codex auth error: %v", err))
		return
	}
	url := fmt.Sprintf("%s?client_version=%s", codexModelsURL, codexCLIVersion())
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	codexRequestHeaders(req, tokens, account, "/backend-api/codex/models")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("codex models request failed: %v", err))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		writeJSONError(w, resp.StatusCode, fmt.Sprintf("codex models request returned %d: %s", resp.StatusCode, summarizeBody(body)))
		return
	}
	var parsed struct {
		Models []codexModelInfo `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("could not parse codex models: %v", err))
		return
	}
	sort.SliceStable(parsed.Models, func(i, j int) bool {
		return parsed.Models[i].Priority < parsed.Models[j].Priority
	})
	data := []map[string]any{}
	for _, model := range parsed.Models {
		if !model.SupportedInAPI || (model.Visibility != "" && model.Visibility != "list") {
			continue
		}
		data = append(data, map[string]any{
			"id":           model.Slug,
			"object":       "model",
			"created":      nowMillis() / 1000,
			"owned_by":     "openai",
			"display_name": model.DisplayName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"message": message, "type": "invalid_request_error"},
	})
}

// pickCodexAccount selects the best Codex account for an API request: the
// active account first, then the best remaining quota. Accounts in `exclude`
// (already tried in this request) are skipped; exhausted accounts are only
// used when nothing better is left, so the backend verdict is passed through.
func (m *Manager) pickCodexAccount(exclude []string) (*Account, []byte, error) {
	accounts, err := m.ListAccounts("codex")
	if err != nil {
		return nil, nil, err
	}
	isExcluded := func(id string) bool {
		for _, tried := range exclude {
			if tried == id {
				return true
			}
		}
		return false
	}
	pick := func(allowExhausted bool) *Account {
		var best *Account
		bestScore := -1e18
		for i := range accounts {
			account := accounts[i]
			if isExcluded(account.ID) {
				continue
			}
			exhausted := account.Quota != nil && account.Quota.LimitReached
			if exhausted && !allowExhausted {
				continue
			}
			score := 50.0
			if account.Quota != nil && account.Quota.Primary != nil {
				score = account.Quota.Primary.RemainingPercent
			}
			if account.IsActive {
				score += 1000
			}
			if exhausted {
				score -= 10000
			}
			if score > bestScore {
				bestScore = score
				bestCopy := account
				best = &bestCopy
			}
		}
		return best
	}
	best := pick(false)
	if best == nil {
		best = pick(true)
	}
	if best == nil {
		return nil, nil, fmt.Errorf("no Codex account with available quota")
	}
	credentials, err := loadCredentials("codex", best.ID)
	if err != nil {
		return nil, nil, err
	}
	return best, credentials, nil
}

// upstreamAPIError carries a verdict from the Codex backend so it can be
// passed through to the client unchanged when no other account can serve the
// request.
type upstreamAPIError struct {
	status int
	body   []byte
}

func (e *upstreamAPIError) Error() string {
	return fmt.Sprintf("codex backend returned %d: %s", e.status, summarizeBody(e.body))
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	var upstreamErr *upstreamAPIError
	if errors.As(err, &upstreamErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upstreamErr.status)
		w.Write(upstreamErr.body)
		return
	}
	writeJSONError(w, http.StatusBadGateway, err.Error())
}

func (h *localAPIHandler) handleResponses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, localAPIMaxBody))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}
	if _, ok := payload["store"]; !ok {
		payload["store"] = false
	}
	clientStream, _ := payload["stream"].(bool)
	payload["stream"] = true // the Codex backend requires streaming
	outBody, err := json.Marshal(payload)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "could not encode request body")
		return
	}
	if clientStream {
		h.proxyToCodex(w, r, outBody)
		return
	}
	response, err := h.completeCodex(r.Context(), outBody)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// proxyToCodex forwards an OpenAI-responses-shaped body to the ChatGPT Codex
// backend using a managed account, rotating to the next account on auth or
// quota errors.
func (h *localAPIHandler) proxyToCodex(w http.ResponseWriter, r *http.Request, body []byte) {
	accept := r.Header.Get("Accept")
	if accept == "" {
		accept = "application/json"
	}
	tried := []string{}
	var lastUpstream *upstreamAPIError
	for attempt := 0; attempt < 3; attempt++ {
		account, credentials, err := h.manager.pickCodexAccount(tried)
		if err != nil {
			if lastUpstream != nil {
				writeUpstreamError(w, lastUpstream)
				return
			}
			writeJSONError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		tried = append(tried, account.ID)
		updatedCredentials, tokens, err := (&CodexProvider{}).ensureFreshTokens(r.Context(), credentials)
		if err == nil && updatedCredentials != nil && !bytes.Equal(updatedCredentials, credentials) {
			saveCredentials("codex", account.ID, updatedCredentials)
		}
		if err != nil {
			if attempt == 2 {
				writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("codex auth error: %v", err))
				return
			}
			continue
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, codexResponsesURL, bytes.NewReader(body))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		codexRequestHeaders(req, tokens, account, "/backend-api/codex/responses")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", accept)
		resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("codex backend error: %v", err))
			return
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusTooManyRequests {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			lastUpstream = &upstreamAPIError{status: resp.StatusCode, body: respBody}
			continue
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 32*1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				if flusher != nil {
					flusher.Flush()
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	if lastUpstream != nil {
		writeUpstreamError(w, lastUpstream)
		return
	}
	writeJSONError(w, http.StatusServiceUnavailable, "all Codex accounts are unavailable right now")
}

func (h *localAPIHandler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, localAPIMaxBody))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	var chatReq chatCompletionRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		writeJSONError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}
	if chatReq.Stream != nil && *chatReq.Stream {
		writeJSONError(w, http.StatusBadRequest, "streaming is not supported on /v1/chat/completions yet; use stream=false or POST /v1/responses")
		return
	}
	if len(chatReq.Messages) == 0 {
		writeJSONError(w, http.StatusBadRequest, "messages must not be empty")
		return
	}
	if strings.TrimSpace(chatReq.Model) == "" {
		writeJSONError(w, http.StatusBadRequest, "model is required (see GET /v1/models)")
		return
	}
	responsesReq := chatToResponsesRequest(chatReq)
	payload, err := json.Marshal(responsesReq)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	proxyResp, err := h.completeCodex(r.Context(), payload)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	completion, err := responsesToChatCompletion(chatReq.Model, proxyResp)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, completion)
}

// completeCodex performs a request against the Codex backend with account
// rotation, aggregates the required SSE stream and returns the final response
// object.
func (h *localAPIHandler) completeCodex(ctx context.Context, body []byte) (map[string]any, error) {
	tried := []string{}
	var lastErr error = fmt.Errorf("no Codex account available")
	for attempt := 0; attempt < 3; attempt++ {
		account, credentials, err := h.manager.pickCodexAccount(tried)
		if err != nil {
			// no more accounts to try: surface the last upstream verdict
			return nil, lastErr
		}
		tried = append(tried, account.ID)
		_, tokens, tokenErr := (&CodexProvider{}).ensureFreshTokens(ctx, credentials)
		if tokenErr != nil {
			lastErr = tokenErr
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexResponsesURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		codexRequestHeaders(req, tokens, account, "/backend-api/codex/responses")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusTooManyRequests {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			lastErr = &upstreamAPIError{status: resp.StatusCode, body: respBody}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			return nil, fmt.Errorf("codex backend returned %d: %s", resp.StatusCode, summarizeBody(respBody))
		}
		parsed, err := parseResponsesSSE(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		return parsed, nil
	}
	return nil, lastErr
}

// parseResponsesSSE reads a Codex responses SSE stream and returns the response
// object carried by the terminal event.
func parseResponsesSSE(reader io.Reader) (map[string]any, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var streamErr string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch jsonString(event["type"]) {
		case "response.completed":
			if response, ok := event["response"].(map[string]any); ok {
				return response, nil
			}
			return event, nil
		case "response.failed", "response.incomplete", "error":
			streamErr = summarizeBody([]byte(payload))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading codex stream: %w", err)
	}
	if streamErr != "" {
		return nil, fmt.Errorf("codex stream failed: %s", streamErr)
	}
	return nil, fmt.Errorf("codex stream ended without a completed response")
}

type chatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []chatCompletionMessage `json:"messages"`
	Stream   *bool                   `json:"stream"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func chatMessageText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := []string{}
		for _, part := range value {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text := jsonString(partMap["text"]); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// chatToResponsesRequest converts an OpenAI chat completion request into a
// Codex responses request (best effort).
func chatToResponsesRequest(chatReq chatCompletionRequest) map[string]any {
	input := []map[string]any{}
	instructions := ""
	for _, message := range chatReq.Messages {
		if message.Role == "system" {
			if instructions != "" {
				instructions += "\n\n"
			}
			instructions += chatMessageText(message.Content)
			continue
		}
		role := message.Role
		if role != "assistant" {
			role = "user"
		}
		partType := "input_text"
		if role == "assistant" {
			partType = "output_text"
		}
		input = append(input, map[string]any{
			"type": "message",
			"role": role,
			"content": []map[string]any{
				{"type": partType, "text": chatMessageText(message.Content)},
			},
		})
	}
	model := strings.TrimSpace(chatReq.Model)
	if model == "" {
		model = "gpt-5-codex"
	}
	rtn := map[string]any{
		"model":  model,
		"input":  input,
		"store":  false,
		"stream": true, // the Codex backend requires streaming
	}
	if instructions != "" {
		rtn["instructions"] = instructions
	}
	return rtn
}

// responsesToChatCompletion converts a Codex responses payload into an OpenAI
// chat completion payload.
func responsesToChatCompletion(model string, resp map[string]any) (map[string]any, error) {
	if model == "" {
		model = jsonString(resp["model"])
	}
	textParts := []string{}
	output, _ := resp["output"].([]any)
	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok || jsonString(itemMap["type"]) != "message" {
			continue
		}
		content, _ := itemMap["content"].([]any)
		for _, part := range content {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			partType := jsonString(partMap["type"])
			if partType == "output_text" || partType == "text" {
				if text := jsonString(partMap["text"]); text != "" {
					textParts = append(textParts, text)
				}
			}
		}
	}
	if len(textParts) == 0 {
		if errText := jsonString(resp["error"]); errText != "" {
			return nil, fmt.Errorf("codex backend error: %s", errText)
		}
		if incomplete := jsonString(resp["incomplete_details"]); incomplete != "" {
			return nil, fmt.Errorf("codex response incomplete: %s", incomplete)
		}
		return nil, fmt.Errorf("codex response did not include any text output")
	}
	content := strings.Join(textParts, "")
	usage := map[string]int{}
	if usageMap, ok := resp["usage"].(map[string]any); ok {
		inputTokens := int(numberValue(usageMap["input_tokens"]))
		outputTokens := int(numberValue(usageMap["output_tokens"]))
		usage["prompt_tokens"] = inputTokens
		usage["completion_tokens"] = outputTokens
		usage["total_tokens"] = inputTokens + outputTokens
	}
	return map[string]any{
		"id":      jsonString(resp["id"]),
		"object":  "chat.completion",
		"created": nowMillis() / 1000,
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
			},
		},
		"usage": usage,
	}, nil
}

func numberValue(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case int:
		return float64(number)
	case int64:
		return float64(number)
	case json.Number:
		parsed, _ := number.Float64()
		return parsed
	default:
		return 0
	}
}
