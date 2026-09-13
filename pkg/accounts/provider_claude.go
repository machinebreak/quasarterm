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
	"time"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const (
	claudeUsageURL      = "https://api.anthropic.com/api/oauth/usage"
	claudeProfileURL    = "https://api.anthropic.com/api/oauth/profile"
	claudeTokenEndpoint = "https://platform.claude.com/v1/oauth/token"
	claudeOAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeBetaHeader    = "oauth-2025-04-20"
)

type ClaudeProvider struct{}

func (p *ClaudeProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          string(ProviderClaude),
		Name:        "Claude",
		Description: "Claude Code CLI",
		CLICommand:  "claude",
		ResumeArgs:  "claude --continue",
		Icon:        "claude",
		Order:       10,
		Available:   true,
	}
}

func claudeConfigDir() string {
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		return env
	}
	return filepath.Join(wavebase.GetHomeDir(), ".claude")
}

func (p *ClaudeProvider) credentialsFilePath() string {
	return filepath.Join(claudeConfigDir(), ".credentials.json")
}

func (p *ClaudeProvider) CurrentCredentialsPath() (string, error) {
	path := p.credentialsFilePath()
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("Claude credentials not found at %s", path)
	}
	return path, nil
}

type claudeOAuthCredentials struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken,omitempty"`
	ExpiresAt        int64    `json:"expiresAt,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
	SubscriptionType string   `json:"subscriptionType,omitempty"`
}

func parseClaudeCredentials(credentials []byte) (map[string]any, *claudeOAuthCredentials, error) {
	var root map[string]any
	if err := json.Unmarshal(credentials, &root); err != nil {
		return nil, nil, fmt.Errorf("invalid Claude credentials file: %w", err)
	}
	oauthMap, _ := root["claudeAiOauth"].(map[string]any)
	if oauthMap == nil {
		return nil, nil, fmt.Errorf("Claude credentials file has no claudeAiOauth section")
	}
	creds := &claudeOAuthCredentials{
		AccessToken:      jsonString(oauthMap["accessToken"]),
		RefreshToken:     jsonString(oauthMap["refreshToken"]),
		SubscriptionType: jsonString(oauthMap["subscriptionType"]),
	}
	if creds.AccessToken == "" {
		return nil, nil, fmt.Errorf("Claude credentials have an empty accessToken")
	}
	if expires, ok := jsonFloat(oauthMap["expiresAt"]); ok {
		creds.ExpiresAt = int64(expires)
	}
	if scopes, ok := oauthMap["scopes"].([]any); ok {
		for _, scope := range scopes {
			creds.Scopes = append(creds.Scopes, jsonString(scope))
		}
	}
	return root, creds, nil
}

func (p *ClaudeProvider) fetchProfileEmail(ctx context.Context, accessToken string) string {
	req, err := newRequest(ctx, http.MethodGet, claudeProfileURL)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", claudeBetaHeader)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var profile struct {
		Account struct {
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
		} `json:"account"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return ""
	}
	return profile.Account.Email
}

func (p *ClaudeProvider) ImportCurrent() (*Account, []byte, error) {
	path, err := p.CurrentCredentialsPath()
	if err != nil {
		return nil, nil, err
	}
	credentials, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading Claude credentials: %w", err)
	}
	_, creds, err := parseClaudeCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	remoteKey, err := p.RemoteKey(credentials)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	email := p.fetchProfileEmail(ctx, creds.AccessToken)
	label := email
	if label == "" {
		label = "Claude account"
	}
	account := &Account{
		ID:        makeAccountID(string(ProviderClaude), remoteKey),
		Provider:  string(ProviderClaude),
		Email:     email,
		PlanType:  creds.SubscriptionType,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *ClaudeProvider) RemoteKey(credentials []byte) (string, error) {
	_, creds, err := parseClaudeCredentials(credentials)
	if err != nil {
		return "", err
	}
	if creds.RefreshToken != "" {
		return hashKey(creds.RefreshToken)[:24], nil
	}
	return hashKey(creds.AccessToken)[:24], nil
}

func (p *ClaudeProvider) InstancePrepare(account *Account, credentials []byte, profileDir string) (string, []string, []string, error) {
	if _, _, err := parseClaudeCredentials(credentials); err != nil {
		return "", nil, nil, err
	}
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return "", nil, nil, err
	}
	if err := atomicWriteFile(filepath.Join(profileDir, ".credentials.json"), credentials, 0600); err != nil {
		return "", nil, nil, err
	}
	return "claude", nil, []string{"CLAUDE_CONFIG_DIR=" + profileDir}, nil
}

// WakeArgs runs a minimal non-interactive Claude request against an account
// profile prepared by InstancePrepare.
func (p *ClaudeProvider) WakeArgs(model string) []string {
	args := []string{"-p"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", strings.TrimSpace(model))
	}
	return append(args, wakePrompt)
}

func (p *ClaudeProvider) SwitchAccount(account *Account, credentials []byte) error {
	if _, _, err := parseClaudeCredentials(credentials); err != nil {
		return err
	}
	path := p.credentialsFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("error creating Claude config dir: %w", err)
	}
	if err := atomicWriteFile(path, credentials, 0600); err != nil {
		return fmt.Errorf("error writing Claude credentials: %w", err)
	}
	return nil
}

type claudeUsageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

type claudeUsageResponse struct {
	FiveHour       *claudeUsageWindow `json:"five_hour"`
	SevenDay       *claudeUsageWindow `json:"seven_day"`
	SevenDaySonnet *claudeUsageWindow `json:"seven_day_sonnet"`
}

func claudeWindowQuota(window *claudeUsageWindow, windowMinutes int64) *QuotaWindow {
	if window == nil || window.Utilization == nil {
		return nil
	}
	return &QuotaWindow{
		RemainingPercent: 100 - normalizePercent(*window.Utilization),
		ResetAt:          parseISO8601(window.ResetsAt),
		WindowMinutes:    windowMinutes,
	}
}

func (p *ClaudeProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	updatedCredentials, creds, err := p.ensureFreshTokens(ctx, credentials)
	if err != nil {
		return nil, nil, err
	}
	req, err := newRequest(ctx, http.MethodGet, claudeUsageURL)
	if err != nil {
		return nil, updatedCredentials, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("anthropic-beta", claudeBetaHeader)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "claude-code/1.0.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, updatedCredentials, fmt.Errorf("error requesting Claude usage: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, updatedCredentials, fmt.Errorf("Claude token expired or revoked (401). Re-import the account.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, updatedCredentials, fmt.Errorf("Claude usage API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var usage claudeUsageResponse
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, updatedCredentials, fmt.Errorf("error parsing Claude usage response: %w", err)
	}
	quota := &Quota{UpdatedAt: nowMillis(), PlanType: account.PlanType, Allowed: true}
	quota.Primary = claudeWindowQuota(usage.FiveHour, 300)
	quota.Secondary = claudeWindowQuota(usage.SevenDay, 7*24*60)
	if quota.Primary == nil && quota.Secondary == nil {
		quota.Secondary = claudeWindowQuota(usage.SevenDaySonnet, 7*24*60)
	}
	if quota.Primary == nil && quota.Secondary == nil {
		return nil, updatedCredentials, fmt.Errorf("Claude usage response did not include quota windows")
	}
	if quota.Primary != nil && quota.Primary.RemainingPercent <= 0 {
		quota.LimitReached = true
	}
	if quota.Secondary != nil && quota.Secondary.RemainingPercent <= 0 {
		quota.LimitReached = true
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

func (p *ClaudeProvider) ensureFreshTokens(ctx context.Context, credentials []byte) ([]byte, *claudeOAuthCredentials, error) {
	root, creds, err := parseClaudeCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	if creds.ExpiresAt == 0 || creds.ExpiresAt > nowMillis()+5*60*1000 || creds.RefreshToken == "" {
		return credentials, creds, nil
	}
	newCredentials, newCreds, err := p.refreshTokens(ctx, root, creds)
	if err != nil {
		return credentials, creds, nil
	}
	return newCredentials, newCreds, nil
}

func (p *ClaudeProvider) refreshTokens(ctx context.Context, root map[string]any, creds *claudeOAuthCredentials) ([]byte, *claudeOAuthCredentials, error) {
	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": creds.RefreshToken,
		"client_id":     claudeOAuthClientID,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeTokenEndpoint, bytes.NewReader(body))
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
		return nil, nil, fmt.Errorf("Claude token refresh failed (%d): %s", resp.StatusCode, summarizeBody(respBody))
	}
	var refreshResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &refreshResp); err != nil {
		return nil, nil, err
	}
	if refreshResp.AccessToken == "" {
		return nil, nil, fmt.Errorf("Claude token refresh returned an empty access token")
	}
	creds.AccessToken = refreshResp.AccessToken
	if refreshResp.RefreshToken != "" {
		creds.RefreshToken = refreshResp.RefreshToken
	}
	if refreshResp.ExpiresIn > 0 {
		creds.ExpiresAt = nowMillis() + refreshResp.ExpiresIn*1000
	}
	oauthMap, _ := root["claudeAiOauth"].(map[string]any)
	if oauthMap == nil {
		oauthMap = map[string]any{}
		root["claudeAiOauth"] = oauthMap
	}
	oauthMap["accessToken"] = creds.AccessToken
	if creds.RefreshToken != "" {
		oauthMap["refreshToken"] = creds.RefreshToken
	}
	if creds.ExpiresAt > 0 {
		oauthMap["expiresAt"] = creds.ExpiresAt
	}
	newCredentials, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return newCredentials, creds, nil
}
