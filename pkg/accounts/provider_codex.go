// Copyright 2026, Orion
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const (
	codexUsageURL         = "https://chatgpt.com/backend-api/wham/usage"
	codexTokenEndpoint    = "https://auth.openai.com/oauth/token"
	codexOAuthClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexWebReferer       = "https://chatgpt.com/"
	codexWebUserAgent     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
	codexAuthScope        = "openid profile email"
)

type CodexProvider struct{}

func (p *CodexProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          string(ProviderCodex),
		Name:        "Codex",
		Description: "OpenAI Codex CLI",
		CLICommand:  "codex",
		ResumeArgs:  "codex resume --last",
		Icon:        "codex",
		Order:       20,
		Available:   true,
	}
}

func codexHomeDir() string {
	if env := os.Getenv("CODEX_HOME"); env != "" {
		return env
	}
	return filepath.Join(wavebase.GetHomeDir(), ".codex")
}

func (p *CodexProvider) authFilePath() string {
	return filepath.Join(codexHomeDir(), "auth.json")
}

func (p *CodexProvider) CurrentCredentialsPath() (string, error) {
	path := p.authFilePath()
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("Codex credentials not found at %s", path)
	}
	return path, nil
}

type codexAuthTokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
}

func parseCodexAuth(credentials []byte) (map[string]any, *codexAuthTokens, error) {
	var root map[string]any
	if err := json.Unmarshal(credentials, &root); err != nil {
		return nil, nil, fmt.Errorf("invalid Codex auth.json: %w", err)
	}
	tokensMap, _ := root["tokens"].(map[string]any)
	if tokensMap == nil {
		return nil, nil, fmt.Errorf("Codex auth.json has no tokens (API key auth is not supported for switching)")
	}
	tokens := &codexAuthTokens{
		IDToken:      jsonString(tokensMap["id_token"]),
		AccessToken:  jsonString(tokensMap["access_token"]),
		RefreshToken: jsonString(tokensMap["refresh_token"]),
		AccountID:    jsonString(tokensMap["account_id"]),
	}
	if tokens.AccessToken == "" {
		return nil, nil, fmt.Errorf("Codex auth.json has an empty access_token")
	}
	return root, tokens, nil
}

func codexIdentityFromTokens(tokens *codexAuthTokens) (email string, accountID string, planType string) {
	claims := decodeJWTPayload(tokens.IDToken)
	if claims == nil {
		claims = decodeJWTPayload(tokens.AccessToken)
	}
	if claims == nil {
		return "", tokens.AccountID, ""
	}
	email = jsonString(claims["email"])
	accountID = tokens.AccountID
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if accountID == "" {
			accountID = jsonString(auth["chatgpt_account_id"])
		}
		planType = jsonString(auth["chatgpt_plan_type"])
	}
	if email == "" {
		if profile, ok := claims["https://api.openai.com/profile"].(map[string]any); ok {
			email = jsonString(profile["email"])
		}
	}
	return email, accountID, planType
}

func (p *CodexProvider) ImportCurrent() (*Account, []byte, error) {
	path, err := p.CurrentCredentialsPath()
	if err != nil {
		return nil, nil, err
	}
	credentials, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading Codex credentials: %w", err)
	}
	_, tokens, err := parseCodexAuth(credentials)
	if err != nil {
		return nil, nil, err
	}
	email, accountID, planType := codexIdentityFromTokens(tokens)
	remoteKey, err := p.RemoteKey(credentials)
	if err != nil {
		return nil, nil, err
	}
	attribution := email
	account := &Account{
		ID:        makeAccountID(string(ProviderCodex), remoteKey),
		Provider:  string(ProviderCodex),
		Email:     email,
		PlanType:  planType,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
	}
	if accountID != "" && accountID != remoteKey {
		attribution = fmt.Sprintf("%s (%s)", email, accountID)
	}
	account.Label = strings.TrimSpace(attribution)
	if account.Label == "" {
		account.Label = "Codex account"
	}
	return account, credentials, nil
}

func (p *CodexProvider) RemoteKey(credentials []byte) (string, error) {
	root, tokens, err := parseCodexAuth(credentials)
	if err != nil {
		return "", err
	}
	_, accountID, _ := codexIdentityFromTokens(tokens)
	if accountID != "" {
		return accountID, nil
	}
	if key := jsonString(root["account_id"]); key != "" {
		return key, nil
	}
	return hashKey(tokens.AccessToken)[:24], nil
}

func (p *CodexProvider) InstancePrepare(account *Account, credentials []byte, profileDir string) (string, []string, []string, error) {
	if _, _, err := parseCodexAuth(credentials); err != nil {
		return "", nil, nil, err
	}
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return "", nil, nil, err
	}
	if err := atomicWriteFile(filepath.Join(profileDir, "auth.json"), credentials, 0600); err != nil {
		return "", nil, nil, err
	}
	return "codex", nil, []string{"CODEX_HOME=" + profileDir}, nil
}

// WakeArgs runs a minimal non-interactive Codex request against an account
// profile prepared by InstancePrepare.
func (p *CodexProvider) WakeArgs(model string) []string {
	args := []string{"exec", "--skip-git-repo-check"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", strings.TrimSpace(model))
	}
	return append(args, wakePrompt)
}

func (p *CodexProvider) SwitchAccount(account *Account, credentials []byte) error {
	if _, _, err := parseCodexAuth(credentials); err != nil {
		return err
	}
	path := p.authFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("error creating Codex config dir: %w", err)
	}
	if err := atomicWriteFile(path, credentials, 0600); err != nil {
		return fmt.Errorf("error writing Codex credentials: %w", err)
	}
	return nil
}

type codexWindowInfo struct {
	UsedPercent       *float64 `json:"used_percent"`
	LimitWindowSecs   *float64 `json:"limit_window_seconds"`
	ResetAfterSeconds *float64 `json:"reset_after_seconds"`
	ResetAt           *int64   `json:"reset_at"`
}

type codexRateLimit struct {
	Allowed       *bool            `json:"allowed"`
	LimitReached  *bool            `json:"limit_reached"`
	PrimaryWindow *codexWindowInfo `json:"primary_window"`
	SecondaryWindow *codexWindowInfo `json:"secondary_window"`
}

type codexUsageResponse struct {
	PlanType              string             `json:"plan_type"`
	RateLimit             *codexRateLimit    `json:"rate_limit"`
	RateLimitResetCredits *struct {
		AvailableCount *int `json:"available_count"`
	} `json:"rate_limit_reset_credits"`
}

func codexWindowQuota(window *codexWindowInfo) *QuotaWindow {
	if window == nil || window.UsedPercent == nil {
		return nil
	}
	remaining := 100 - normalizePercent(*window.UsedPercent)
	quotaWindow := &QuotaWindow{RemainingPercent: remaining}
	if window.ResetAt != nil {
		quotaWindow.ResetAt = *window.ResetAt
	}
	if window.LimitWindowSecs != nil && *window.LimitWindowSecs > 0 {
		quotaWindow.WindowMinutes = int64(*window.LimitWindowSecs / 60)
	}
	return quotaWindow
}

func (p *CodexProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	updatedCredentials, tokens, err := p.ensureFreshTokens(ctx, credentials)
	if err != nil {
		return nil, nil, err
	}
	req, err := newRequest(ctx, http.MethodGet, codexUsageURL)
	if err != nil {
		return nil, updatedCredentials, err
	}
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", codexWebReferer)
	req.Header.Set("User-Agent", codexWebUserAgent)
	req.Header.Set("x-openai-target-path", "/backend-api/wham/usage")
	req.Header.Set("x-openai-target-route", "/backend-api/wham/usage")
	if account.RemoteID != "" {
		req.Header.Set("ChatGPT-Account-Id", account.RemoteID)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, updatedCredentials, fmt.Errorf("error requesting Codex usage: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, updatedCredentials, fmt.Errorf("Codex token expired or revoked (401). Re-import the account.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, updatedCredentials, fmt.Errorf("Codex usage API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var usage codexUsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, updatedCredentials, fmt.Errorf("error parsing Codex usage response: %w", err)
	}
	quota := &Quota{UpdatedAt: nowMillis(), PlanType: usage.PlanType, Allowed: true}
	if usage.RateLimitResetCredits != nil && usage.RateLimitResetCredits.AvailableCount != nil {
		quota.ResetCredits = *usage.RateLimitResetCredits.AvailableCount
	}
	if usage.RateLimit != nil {
		if usage.RateLimit.Allowed != nil {
			quota.Allowed = *usage.RateLimit.Allowed
		}
		if usage.RateLimit.LimitReached != nil {
			quota.LimitReached = *usage.RateLimit.LimitReached
		}
		quota.Primary = codexWindowQuota(usage.RateLimit.PrimaryWindow)
		quota.Secondary = codexWindowQuota(usage.RateLimit.SecondaryWindow)
	}
	if quota.Primary == nil && quota.Secondary == nil && !quota.LimitReached {
		return nil, updatedCredentials, fmt.Errorf("Codex usage response did not include quota windows")
	}
	if quota.Primary != nil {
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             "5h",
			RemainingPercent: quota.Primary.RemainingPercent,
			ResetAt:          quota.Primary.ResetAt,
		})
	}
	if quota.Secondary != nil {
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             "Weekly",
			RemainingPercent: quota.Secondary.RemainingPercent,
			ResetAt:          quota.Secondary.ResetAt,
		})
	}
	return quota, updatedCredentials, nil
}

func (p *CodexProvider) ensureFreshTokens(ctx context.Context, credentials []byte) ([]byte, *codexAuthTokens, error) {
	root, tokens, err := parseCodexAuth(credentials)
	if err != nil {
		return nil, nil, err
	}
	if !codexTokensNeedRefresh(tokens) {
		return credentials, tokens, nil
	}
	if tokens.RefreshToken == "" {
		return credentials, tokens, nil
	}
	newCredentials, newTokens, err := p.refreshTokens(ctx, root, tokens)
	if err != nil {
		return credentials, tokens, nil
	}
	return newCredentials, newTokens, nil
}

// codexTokensNeedRefresh reports whether either OAuth token is expired or
// about to expire. The CLI uses the access token for requests, but an expired
// id_token pushes clients into the sign-in flow on launch, so refresh in both
// cases (mirrors the fix in cockpit-tools).
func codexTokensNeedRefresh(tokens *codexAuthTokens) bool {
	return codexTokenExpired(tokens.AccessToken) || codexTokenExpired(tokens.IDToken)
}

func codexTokenExpired(accessToken string) bool {
	claims := decodeJWTPayload(accessToken)
	if claims == nil {
		return false
	}
	exp, ok := jsonFloat(claims["exp"])
	if !ok {
		return false
	}
	return int64(exp) < nowMillis()/1000+120
}

func (p *CodexProvider) refreshTokens(ctx context.Context, root map[string]any, tokens *codexAuthTokens) ([]byte, *codexAuthTokens, error) {
	payload := map[string]string{
		"client_id":     codexOAuthClientID,
		"grant_type":    "refresh_token",
		"refresh_token": tokens.RefreshToken,
		"scope":         codexAuthScope,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("Codex token refresh failed (%d): %s", resp.StatusCode, summarizeBody(respBody))
	}
	var refreshResp struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(respBody, &refreshResp); err != nil {
		return nil, nil, err
	}
	if refreshResp.AccessToken == "" {
		return nil, nil, fmt.Errorf("Codex token refresh returned an empty access token")
	}
	if refreshResp.IDToken != "" {
		tokens.IDToken = refreshResp.IDToken
	}
	tokens.AccessToken = refreshResp.AccessToken
	if refreshResp.RefreshToken != "" {
		tokens.RefreshToken = refreshResp.RefreshToken
	}
	tokensMap, _ := root["tokens"].(map[string]any)
	if tokensMap == nil {
		tokensMap = map[string]any{}
		root["tokens"] = tokensMap
	}
	tokensMap["access_token"] = tokens.AccessToken
	tokensMap["id_token"] = tokens.IDToken
	if tokens.RefreshToken != "" {
		tokensMap["refresh_token"] = tokens.RefreshToken
	}
	if tokens.AccountID != "" {
		tokensMap["account_id"] = tokens.AccountID
	}
	newCredentials, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return newCredentials, tokens, nil
}

func summarizeBody(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if len(text) > 300 {
		text = text[:300] + "..."
	}
	return text
}
