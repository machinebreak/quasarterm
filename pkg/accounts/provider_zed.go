// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const zedCloudBase = "https://cloud.zed.dev"

type ZedProvider struct{}

func (p *ZedProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "zed",
		Name:        "Zed",
		Description: "Zed editor",
		CLICommand:  "zed",
		ResumeArgs:  "",
		Icon:        "zed",
		Order:       120,
		Available:   true,
	}
}

type zedCredentials struct {
	UserID      string `json:"userId"`
	AccessToken string `json:"accessToken"`
}

func parseZedCredentials(credentials []byte) (*zedCredentials, error) {
	var creds zedCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid Zed credentials: %w", err)
	}
	if creds.UserID == "" || creds.AccessToken == "" {
		return nil, fmt.Errorf("Zed credentials are incomplete")
	}
	return &creds, nil
}

func (p *ZedProvider) RemoteKey(credentials []byte) (string, error) {
	creds, err := parseZedCredentials(credentials)
	if err != nil {
		return "", err
	}
	return creds.UserID, nil
}

func zedNested(root map[string]any, path ...string) any {
	var current any = root
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[key]
	}
	return current
}

func zedNestedInt(root map[string]any, path ...string) (int64, bool) {
	return jsonFloatInt(zedNested(root, path...))
}

func jsonFloatInt(value any) (int64, bool) {
	f, ok := jsonFloat(value)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

func zedNestedString(root map[string]any, path ...string) string {
	return jsonString(zedNested(root, path...))
}

func fetchZedUser(ctx context.Context, userID string, accessToken string) (map[string]any, error) {
	req, err := newRequest(ctx, http.MethodGet, zedCloudBase+"/client/users/me")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", userID+" "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error requesting Zed user info: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("Zed session expired. Re-import the account.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zed API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("error parsing Zed response: %w", err)
	}
	return root, nil
}

func zedUsageObject(root map[string]any) map[string]any {
	for _, path := range [][]string{
		{"current_usage"},
		{"usage"},
		{"plan", "usage"},
	} {
		if obj, ok := zedNested(root, path...).(map[string]any); ok {
			return obj
		}
	}
	return root
}

func (p *ZedProvider) CurrentCredentialsPath() (string, error) {
	if _, _, err := readZedCredential(); err != nil {
		return "", err
	}
	return "Windows Credential Manager (zed:url=https://zed.dev)", nil
}

func (p *ZedProvider) ActiveRemoteKey() (string, error) {
	userID, _, err := readZedCredential()
	return userID, err
}

func (p *ZedProvider) ImportCurrent() (*Account, []byte, error) {
	userID, accessToken, err := readZedCredential()
	if err != nil {
		return nil, nil, err
	}
	creds := zedCredentials{UserID: userID, AccessToken: accessToken}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	label := userID
	email := ""
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if root, fetchErr := fetchZedUser(ctx, userID, accessToken); fetchErr == nil {
		if login := firstNonEmpty(
			zedNestedString(root, "user", "github_login"),
			zedNestedString(root, "user", "githubLogin"),
			zedNestedString(root, "github_login"),
		); login != "" {
			label = login
		}
		email = firstNonEmpty(zedNestedString(root, "user", "email"), zedNestedString(root, "email"))
	}
	account := &Account{
		ID:        makeAccountID("zed", userID),
		Provider:  "zed",
		Email:     email,
		RemoteID:  userID,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *ZedProvider) SwitchAccount(account *Account, credentials []byte) error {
	creds, err := parseZedCredentials(credentials)
	if err != nil {
		return err
	}
	return writeZedCredential(creds.UserID, creds.AccessToken)
}

func remainingFromUsage(usage map[string]any, key string) (float64, bool) {
	limit, okLimit := zedNestedInt(usage, key, "limit")
	if !okLimit || limit <= 0 {
		return 0, false
	}
	if remaining, okRemaining := zedNestedInt(usage, key, "remaining"); okRemaining {
		return (float64(remaining) / float64(limit)) * 100, true
	}
	if used, okUsed := zedNestedInt(usage, key, "used"); okUsed {
		return (float64(limit-used) / float64(limit)) * 100, true
	}
	return 0, false
}

func (p *ZedProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	creds, err := parseZedCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	root, err := fetchZedUser(ctx, creds.UserID, creds.AccessToken)
	if err != nil {
		return nil, credentials, err
	}
	usage := zedUsageObject(root)
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	if plan := firstNonEmpty(
		zedNestedString(root, "plan", "name"),
		zedNestedString(root, "subscription", "name"),
	); plan != "" {
		quota.PlanType = plan
	}
	if remaining, ok := remainingFromUsage(usage, "token_spend"); ok {
		quota.Metrics = append(quota.Metrics, QuotaMetric{Name: "Token Spend", RemainingPercent: normalizePercent(remaining)})
	}
	if remaining, ok := remainingFromUsage(usage, "edit_predictions"); ok {
		quota.Metrics = append(quota.Metrics, QuotaMetric{Name: "Edit Predictions", RemainingPercent: normalizePercent(remaining)})
	}
	if len(quota.Metrics) == 0 {
		return nil, credentials, fmt.Errorf("Zed response did not include usage limits")
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent}
	return quota, credentials, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
