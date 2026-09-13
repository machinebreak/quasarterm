// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
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
	copilotUserInfoURL = "https://api.github.com/copilot_internal/user"
	copilotUserAgent   = "GitHubCopilotChat/0.26.7"
	copilotAPIVersion  = "2022-11-28"
)

type CopilotProvider struct{}

func (p *CopilotProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          string(ProviderCopilot),
		Name:        "GitHub Copilot",
		Description: "GitHub Copilot CLI / VS Code",
		CLICommand:  "gh copilot",
		ResumeArgs:  "",
		Icon:        "github-copilot",
		Order:       30,
		Available:   true,
	}
}

func copilotConfigDirs() []string {
	dirs := []string{}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		dirs = append(dirs, filepath.Join(localAppData, "github-copilot"))
	}
	dirs = append(dirs, filepath.Join(wavebase.GetHomeDir(), ".config", "github-copilot"))
	return dirs
}

func copilotCredentialFiles() []string {
	files := []string{}
	for _, dir := range copilotConfigDirs() {
		for _, name := range []string{"hosts.json", "apps.json"} {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files
}

func (p *CopilotProvider) CurrentCredentialsPath() (string, error) {
	for _, file := range copilotCredentialFiles() {
		if _, err := os.Stat(file); err == nil {
			return file, nil
		}
	}
	return "", fmt.Errorf("GitHub Copilot credentials not found (hosts.json/apps.json). Log in with Copilot first.")
}

func findCopilotToken(value any) (token string, user string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lowerKey := strings.ToLower(key)
			if lowerKey == "oauth_token" || lowerKey == "token" {
				if str, ok := child.(string); ok && str != "" {
					token = str
				}
			}
			if lowerKey == "user" || lowerKey == "username" {
				if str, ok := child.(string); ok && str != "" {
					user = str
				}
			}
			childToken, childUser := findCopilotToken(child)
			if childToken != "" {
				token = childToken
			}
			if user == "" && childUser != "" {
				user = childUser
			}
		}
	case []any:
		for _, child := range typed {
			childToken, childUser := findCopilotToken(child)
			if childToken != "" {
				token = childToken
			}
			if user == "" && childUser != "" {
				user = childUser
			}
		}
	}
	return token, user
}

func parseCopilotCredentials(credentials []byte) (string, string, error) {
	var root any
	if err := json.Unmarshal(credentials, &root); err != nil {
		return "", "", fmt.Errorf("invalid GitHub Copilot credentials file: %w", err)
	}
	token, user := findCopilotToken(root)
	if token == "" {
		return "", "", fmt.Errorf("no oauth token found in GitHub Copilot credentials")
	}
	return token, user, nil
}

func (p *CopilotProvider) ImportCurrent() (*Account, []byte, error) {
	path, err := p.CurrentCredentialsPath()
	if err != nil {
		return nil, nil, err
	}
	credentials, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading GitHub Copilot credentials: %w", err)
	}
	token, user, err := parseCopilotCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	remoteKey := hashKey(token)[:24]
	label := user
	if label == "" {
		label = "GitHub Copilot"
	}
	account := &Account{
		ID:        makeAccountID(string(ProviderCopilot), remoteKey),
		Provider:  string(ProviderCopilot),
		Email:     user,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *CopilotProvider) RemoteKey(credentials []byte) (string, error) {
	token, _, err := parseCopilotCredentials(credentials)
	if err != nil {
		return "", err
	}
	return hashKey(token)[:24], nil
}

func replaceCopilotTokens(value any, newToken string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lowerKey := strings.ToLower(key)
			if (lowerKey == "oauth_token" || lowerKey == "token") && child != nil {
				typed[key] = newToken
				continue
			}
			replaceCopilotTokens(child, newToken)
		}
	case []any:
		for _, child := range typed {
			replaceCopilotTokens(child, newToken)
		}
	}
}

func (p *CopilotProvider) SwitchAccount(account *Account, credentials []byte) error {
	token, _, err := parseCopilotCredentials(credentials)
	if err != nil {
		return err
	}
	path, err := p.CurrentCredentialsPath()
	if err != nil {
		dir := copilotConfigDirs()[0]
		os.MkdirAll(dir, 0700)
		path = filepath.Join(dir, "hosts.json")
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		existing = credentials
	}
	var root any
	if err := json.Unmarshal(existing, &root); err != nil {
		root = map[string]any{}
	}
	replaceCopilotTokens(root, token)
	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, updated, 0600)
}

type copilotQuotaSnapshot struct {
	PercentRemaining *float64 `json:"percent_remaining"`
	Remaining        *float64 `json:"remaining"`
	Entitlement      *float64 `json:"entitlement"`
	Unlimited        *bool    `json:"unlimited"`
}

type copilotUserInfo struct {
	CopilotPlan    string                          `json:"copilot_plan"`
	QuotaResetDate string                          `json:"quota_reset_date"`
	QuotaSnapshots map[string]copilotQuotaSnapshot `json:"quota_snapshots"`
	LimitedQuotas  map[string]json.RawMessage      `json:"limited_user_quotas"`
}

func (p *CopilotProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	token, _, err := parseCopilotCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	req, err := newRequest(ctx, http.MethodGet, copilotUserInfoURL)
	if err != nil {
		return nil, credentials, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", copilotUserAgent)
	req.Header.Set("Editor-Version", "vscode/1.99.0")
	req.Header.Set("Editor-Plugin-Version", copilotUserAgent)
	req.Header.Set("X-GitHub-Api-Version", copilotAPIVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, credentials, fmt.Errorf("error requesting GitHub Copilot usage: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, credentials, fmt.Errorf("GitHub Copilot token rejected (%d). Re-import the account.", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, credentials, fmt.Errorf("GitHub Copilot API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var info copilotUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, credentials, fmt.Errorf("error parsing GitHub Copilot response: %w", err)
	}
	quota := &Quota{UpdatedAt: nowMillis(), PlanType: info.CopilotPlan, Allowed: true}
	resetAt := parseResetDate(info.QuotaResetDate)
	metricOrder := []struct {
		key  string
		name string
	}{
		{"premium_interactions", "Premium"},
		{"chat", "Chat"},
		{"completions", "Inline"},
	}
	for _, entry := range metricOrder {
		snapshot, found := info.QuotaSnapshots[entry.key]
		if !found {
			continue
		}
		remaining := 100.0
		if snapshot.PercentRemaining != nil {
			remaining = 100 - normalizePercent(*snapshot.PercentRemaining)
		} else if snapshot.Unlimited != nil && *snapshot.Unlimited {
			remaining = 100
		} else if snapshot.Remaining != nil && snapshot.Entitlement != nil && *snapshot.Entitlement > 0 {
			remaining = 100 - normalizePercent((*snapshot.Remaining / *snapshot.Entitlement) * 100)
		}
		if snapshot.Unlimited != nil && *snapshot.Unlimited {
			remaining = 100
		}
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             entry.name,
			RemainingPercent: remaining,
			ResetAt:          resetAt,
		})
	}
	if len(quota.Metrics) == 0 {
		return nil, credentials, fmt.Errorf("GitHub Copilot response did not include quota snapshots")
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	if len(quota.Metrics) > 0 {
		quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: quota.Metrics[0].ResetAt}
	}
	if len(quota.Metrics) > 1 {
		quota.Secondary = &QuotaWindow{RemainingPercent: quota.Metrics[1].RemainingPercent, ResetAt: quota.Metrics[1].ResetAt}
	}
	return quota, credentials, nil
}

func parseResetDate(value string) int64 {
	if value == "" {
		return 0
	}
	for _, layout := range []string{"2006-01-02T15:04:05Z", "2006-01-02T15:04:05.000Z", "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Unix()
		}
	}
	return 0
}
