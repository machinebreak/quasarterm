// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const (
	grokAuthRegistryKey = "https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828"
	grokTokenEndpoint   = "https://auth.x.ai/oauth2/token"
	grokOAuthClientID   = "b1a00492-073a-47ea-816f-4c329264a828"
	grokBillingURL      = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
	grokClientVersion   = "0.2.0"
)

type GrokProvider struct{}

func (p *GrokProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "grok",
		Name:        "Grok CLI",
		Description: "xAI Grok CLI",
		CLICommand:  "grok",
		ResumeArgs:  "",
		Icon:        "grok",
		Order:       70,
		Available:   true,
	}
}

func grokHomeDir() string {
	if env := os.Getenv("GROK_HOME"); env != "" {
		return env
	}
	return filepath.Join(wavebase.GetHomeDir(), ".grok")
}

func grokAuthPath() string {
	return filepath.Join(grokHomeDir(), "auth.json")
}

type grokCredentials struct {
	RegistryKey string         `json:"registryKey"`
	Entry       map[string]any `json:"entry"`
}

func parseGrokCredentials(credentials []byte) (*grokCredentials, error) {
	var creds grokCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid Grok credentials: %w", err)
	}
	if creds.Entry == nil || pickStringAlias(creds.Entry, "access_token", "accessToken", "key") == "" {
		return nil, fmt.Errorf("Grok credentials have no access token")
	}
	if creds.RegistryKey == "" {
		creds.RegistryKey = grokAuthRegistryKey
	}
	return &creds, nil
}

func (p *GrokProvider) CurrentCredentialsPath() (string, error) {
	path := grokAuthPath()
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("Grok auth.json not found at %s (log in to Grok CLI first)", path)
	}
	return path, nil
}

func (p *GrokProvider) ImportCurrent() (*Account, []byte, error) {
	path, err := p.CurrentCredentialsPath()
	if err != nil {
		return nil, nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, fmt.Errorf("invalid Grok auth.json: %w", err)
	}
	registryKey := grokAuthRegistryKey
	entry, _ := root[registryKey].(map[string]any)
	if entry == nil {
		for key, value := range root {
			if obj, ok := value.(map[string]any); ok && pickStringAlias(obj, "access_token", "accessToken", "key") != "" {
				registryKey = key
				entry = obj
				break
			}
		}
	}
	if entry == nil {
		return nil, nil, fmt.Errorf("Grok auth.json does not contain an OAuth entry")
	}
	accessToken := pickStringAlias(entry, "access_token", "accessToken", "key")
	creds := grokCredentials{RegistryKey: registryKey, Entry: entry}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	email := ""
	if claims := decodeJWTPayload(accessToken); claims != nil {
		email = firstNonEmpty(jsonString(claims["email"]), jsonString(claims["preferred_username"]))
	}
	label := email
	if label == "" {
		label = "Grok account"
	}
	account := &Account{
		ID:        makeAccountID("grok", hashKey(accessToken)[:24]),
		Provider:  "grok",
		Email:     email,
		RemoteID:  hashKey(accessToken)[:24],
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *GrokProvider) RemoteKey(credentials []byte) (string, error) {
	creds, err := parseGrokCredentials(credentials)
	if err != nil {
		return "", err
	}
	return hashKey(pickStringAlias(creds.Entry, "access_token", "accessToken", "key"))[:24], nil
}

func (p *GrokProvider) SwitchAccount(account *Account, credentials []byte) error {
	creds, err := parseGrokCredentials(credentials)
	if err != nil {
		return err
	}
	path := grokAuthPath()
	root := map[string]any{}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		json.Unmarshal(raw, &root)
	}
	root[creds.RegistryKey] = creds.Entry
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(path), 0700)
	return atomicWriteFile(path, encoded, 0600)
}

func grokExpired(entry map[string]any) bool {
	expiresAt := pickIntAlias(entry, "expires_at", "expiresAt")
	if expiresAt == 0 {
		return false
	}
	if expiresAt > 1e12 {
		return expiresAt/1000 < time.Now().Unix()+120
	}
	return expiresAt < time.Now().Unix()+120
}

func (p *GrokProvider) refreshTokens(ctx context.Context, creds *grokCredentials) (*grokCredentials, bool) {
	refreshToken := pickStringAlias(creds.Entry, "refresh_token", "refreshToken")
	if refreshToken == "" {
		return creds, false
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", grokOAuthClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, grokTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return creds, false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return creds, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return creds, false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return creds, false
	}
	accessToken := pickStringAlias(payload, "access_token", "accessToken")
	if accessToken == "" {
		return creds, false
	}
	creds.Entry["access_token"] = accessToken
	if newRefresh := pickStringAlias(payload, "refresh_token", "refreshToken"); newRefresh != "" {
		creds.Entry["refresh_token"] = newRefresh
	}
	if expiresIn := pickIntAlias(payload, "expires_in", "expiresIn"); expiresIn > 0 {
		creds.Entry["expires_at"] = time.Now().Unix() + expiresIn
	}
	return creds, true
}

func grokProductUsage(item map[string]any) (string, float64, int64, bool) {
	product := firstNonEmpty(jsonString(item["product"]), jsonString(item["name"]), "Usage")
	used := -1.0
	total := -1.0
	if percent, ok := jsonFloat(item["usagePercent"]); ok {
		return product, normalizePercent(percent), 0, true
	}
	if value, ok := jsonFloat(item["used"]); ok {
		used = value
	}
	if value, ok := jsonFloat(item["total"]); ok {
		total = value
	}
	if used >= 0 && total > 0 {
		return product, (used / total) * 100, 0, true
	}
	return "", 0, 0, false
}

func (p *GrokProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	creds, err := parseGrokCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	updated := credentials
	if grokExpired(creds.Entry) {
		if newCreds, ok := p.refreshTokens(ctx, creds); ok {
			creds = newCreds
			updated, _ = json.MarshalIndent(creds, "", "  ")
		}
	}
	accessToken := pickStringAlias(creds.Entry, "access_token", "accessToken", "key")
	req, err := newRequest(ctx, http.MethodGet, grokBillingURL)
	if err != nil {
		return nil, updated, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-xai-token-auth", "xai-grok-cli")
	req.Header.Set("x-grok-cli-version", grokClientVersion)
	req.Header.Set("x-grok-client-version", grokClientVersion)
	req.Header.Set("x-grok-client-surface", "grok-cli")
	req.Header.Set("x-grok-client-identifier", "quasar")
	req.Header.Set("User-Agent", "grok-cli/"+grokClientVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, updated, fmt.Errorf("error requesting Grok billing: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, updated, fmt.Errorf("Grok session expired. Re-import the account.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, updated, fmt.Errorf("Grok billing API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, updated, fmt.Errorf("error parsing Grok billing response: %w", err)
	}
	config, _ := payload["config"].(map[string]any)
	if config == nil {
		config = payload
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	lifetime := ""
	if entry, ok := creds.Entry["oidc_client_id"]; ok {
		lifetime = jsonString(entry)
	}
	_ = lifetime
	if tier := firstNonEmpty(jsonString(config["subscription_tier"]), jsonString(config["subscriptionTier"])); tier != "" {
		quota.PlanType = tier
	}
	resetAt := int64(0)
	if period, ok := config["currentPeriod"].(map[string]any); ok {
		resetAt = parseISO8601(jsonString(period["end"]))
	}
	if percent, ok := jsonFloat(config["creditUsagePercent"]); ok {
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             "Weekly credits",
			RemainingPercent: normalizePercent(100 - percent),
			ResetAt:          resetAt,
		})
	}
	if products, ok := config["productUsage"].([]any); ok {
		for _, entry := range products {
			if item, ok := entry.(map[string]any); ok {
				if name, usedPercent, productReset, ok := grokProductUsage(item); ok {
					quota.Metrics = append(quota.Metrics, QuotaMetric{
						Name:             name,
						RemainingPercent: normalizePercent(100 - usedPercent),
						ResetAt:          productReset,
					})
				}
			}
		}
	}
	if len(quota.Metrics) == 0 {
		return nil, updated, fmt.Errorf("Grok billing response did not include usage data")
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: quota.Metrics[0].ResetAt}
	return quota, updated, nil
}
