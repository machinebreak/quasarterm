// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bytes"
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
	kiroAuthFileName     = "kiro-auth-token.json"
	kiroRemoteRefreshURL = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"
)

type KiroProvider struct{}

func (p *KiroProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "kiro",
		Name:        "Kiro",
		Description: "Kiro IDE",
		CLICommand:  "kiro",
		ResumeArgs:  "",
		Icon:        "kiro",
		Order:       50,
		Available:   true,
	}
}

func kiroDataDir() string {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, "Kiro")
	}
	return filepath.Join(wavebase.GetHomeDir(), ".config", "Kiro")
}

func kiroAuthPath() string {
	return filepath.Join(wavebase.GetHomeDir(), ".aws", "sso", "cache", kiroAuthFileName)
}

func kiroProfilePath() string {
	return filepath.Join(kiroDataDir(), "User", "globalStorage", "kiro.kiroagent", "profile.json")
}

type kiroAuthToken struct {
	AccessToken   string
	RefreshToken  string
	ProfileArn    string
	Region        string
	ClientID      string
	ClientHash    string
	ExpiresAt     int64
}

func pickStringAlias(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := jsonString(obj[key]); value != "" {
			return value
		}
	}
	return ""
}

func pickIntAlias(obj map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := jsonFloat(obj[key]); ok {
			return int64(value)
		}
	}
	return 0
}

func parseKiroAuth(raw []byte) (map[string]any, *kiroAuthToken, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, fmt.Errorf("invalid Kiro auth token file: %w", err)
	}
	token := &kiroAuthToken{
		AccessToken:  pickStringAlias(root, "accessToken", "access_token", "token", "idToken"),
		RefreshToken: pickStringAlias(root, "refreshToken", "refresh_token"),
		ProfileArn:   pickStringAlias(root, "profileArn", "profile_arn", "arn"),
		Region:       pickStringAlias(root, "idc_region", "idcRegion", "region"),
		ClientID:     pickStringAlias(root, "client_id", "clientId"),
		ClientHash:   pickStringAlias(root, "clientIdHash"),
		ExpiresAt:    pickIntAlias(root, "expiresAt", "expires_at"),
	}
	if token.AccessToken == "" {
		return nil, nil, fmt.Errorf("Kiro auth file has no access token")
	}
	return root, token, nil
}

func (p *KiroProvider) CurrentCredentialsPath() (string, error) {
	path := kiroAuthPath()
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("Kiro auth token not found at %s (log in to Kiro first)", path)
	}
	return path, nil
}

func (p *KiroProvider) ImportCurrent() (*Account, []byte, error) {
	path, err := p.CurrentCredentialsPath()
	if err != nil {
		return nil, nil, err
	}
	rawAuth, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	_, token, err := parseKiroAuth(rawAuth)
	if err != nil {
		return nil, nil, err
	}
	var profile map[string]any
	if rawProfile, readErr := os.ReadFile(kiroProfilePath()); readErr == nil {
		json.Unmarshal(rawProfile, &profile)
	}
	email := ""
	if profile != nil {
		email = pickStringAlias(profile, "email")
	}
	label := email
	if label == "" {
		label = "Kiro account"
	}
	credentials, _ := json.MarshalIndent(map[string]any{
		"authToken": json.RawMessage(rawAuth),
		"profile":   profile,
	}, "", "  ")
	remoteKey := token.ProfileArn
	if remoteKey == "" {
		remoteKey = hashKey(token.RefreshToken + token.AccessToken)[:24]
	}
	account := &Account{
		ID:        makeAccountID("kiro", remoteKey),
		Provider:  "kiro",
		Email:     email,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func parseKiroStoredCredentials(credentials []byte) (map[string]any, map[string]any, *kiroAuthToken, error) {
	var stored struct {
		AuthToken json.RawMessage `json:"authToken"`
		Profile   map[string]any  `json:"profile"`
	}
	if err := json.Unmarshal(credentials, &stored); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid Kiro credentials: %w", err)
	}
	root, token, err := parseKiroAuth(stored.AuthToken)
	if err != nil {
		return nil, nil, nil, err
	}
	return root, stored.Profile, token, nil
}

func (p *KiroProvider) RemoteKey(credentials []byte) (string, error) {
	_, _, token, err := parseKiroStoredCredentials(credentials)
	if err != nil {
		return "", err
	}
	if token.ProfileArn != "" {
		return token.ProfileArn, nil
	}
	return hashKey(token.RefreshToken + token.AccessToken)[:24], nil
}

func (p *KiroProvider) SwitchAccount(account *Account, credentials []byte) error {
	root, profile, _, err := parseKiroStoredCredentials(credentials)
	if err != nil {
		return err
	}
	authBytes, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	authPath := kiroAuthPath()
	if err := os.MkdirAll(filepath.Dir(authPath), 0700); err != nil {
		return err
	}
	if err := atomicWriteFile(authPath, authBytes, 0600); err != nil {
		return err
	}
	if profile != nil {
		profileBytes, err := json.MarshalIndent(profile, "", "  ")
		if err == nil {
			profilePath := kiroProfilePath()
			os.MkdirAll(filepath.Dir(profilePath), 0700)
			atomicWriteFile(profilePath, profileBytes, 0600)
		}
	}
	return nil
}

func kiroParseProfileArnRegion(profileArn string) string {
	parts := strings.Split(profileArn, ":")
	if len(parts) < 4 {
		return ""
	}
	return strings.TrimSpace(parts[3])
}

func kiroRuntimeEndpoint(region string) string {
	switch strings.ToLower(region) {
	case "", "us-east-1":
		return "https://q.us-east-1.amazonaws.com"
	case "eu-central-1":
		return "https://q.eu-central-1.amazonaws.com"
	case "us-gov-east-1":
		return "https://q-fips.us-gov-east-1.amazonaws.com"
	case "us-gov-west-1":
		return "https://q-fips.us-gov-west-1.amazonaws.com"
	default:
		return "https://q." + strings.ToLower(region) + ".amazonaws.com"
	}
}

func (p *KiroProvider) refreshTokens(ctx context.Context, root map[string]any, token *kiroAuthToken) ([]byte, *kiroAuthToken, bool) {
	if token.RefreshToken == "" {
		return nil, nil, false
	}
	if token.ClientID != "" && token.Region != "" {
		clientSecret := ""
		if token.ClientHash != "" {
			registrationPath := filepath.Join(filepath.Dir(kiroAuthPath()), token.ClientHash+".json")
			if rawRegistration, err := os.ReadFile(registrationPath); err == nil {
				var registration map[string]any
				if json.Unmarshal(rawRegistration, &registration) == nil {
					clientSecret = pickStringAlias(registration, "clientSecret", "client_secret")
				}
			}
		}
		payload := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": token.RefreshToken,
			"client_id":     token.ClientID,
		}
		if clientSecret != "" {
			payload["client_secret"] = clientSecret
		}
		endpoint := fmt.Sprintf("https://oidc.%s.amazonaws.com/token", token.Region)
		if newRoot, newToken, ok := kiroDoRefresh(ctx, root, token, endpoint, payload, "accessToken", "refreshToken", "expiresIn"); ok {
			return newRoot, newToken, true
		}
	}
	payload := map[string]string{"refreshToken": token.RefreshToken}
	return kiroDoRefresh(ctx, root, token, kiroRemoteRefreshURL, payload, "accessToken", "refreshToken", "expiresIn")
}

func kiroDoRefresh(ctx context.Context, root map[string]any, token *kiroAuthToken, endpoint string, payload map[string]string, accessKey string, refreshKey string, expiresKey string) ([]byte, *kiroAuthToken, bool) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, nil, false
	}
	var refreshed map[string]any
	if err := json.Unmarshal(respBody, &refreshed); err != nil {
		return nil, nil, false
	}
	accessToken := pickStringAlias(refreshed, accessKey, "access_token", "accessToken", "token")
	if accessToken == "" {
		return nil, nil, false
	}
	token.AccessToken = accessToken
	root["accessToken"] = accessToken
	if newRefresh := pickStringAlias(refreshed, refreshKey, "refresh_token", "refreshToken"); newRefresh != "" {
		token.RefreshToken = newRefresh
		root["refreshToken"] = newRefresh
	}
	if expiresIn := pickIntAlias(refreshed, expiresKey, "expires_in"); expiresIn > 0 {
		token.ExpiresAt = time.Now().Unix() + expiresIn
		root["expiresAt"] = token.ExpiresAt
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, false
	}
	return encoded, token, true
}

func (p *KiroProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	root, _, token, err := parseKiroStoredCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	updated := credentials
	if token.ExpiresAt > 0 && token.ExpiresAt < time.Now().Unix()+120 {
		if newCredentials, newToken, ok := p.refreshTokens(ctx, root, token); ok {
			updated = newCredentials
			token = newToken
		}
	}
	if token.ProfileArn == "" {
		return nil, updated, fmt.Errorf("Kiro account has no profileArn")
	}
	region := kiroParseProfileArnRegion(token.ProfileArn)
	endpoint := kiroRuntimeEndpoint(region)
	requestURL := fmt.Sprintf("%s/getUsageLimits?origin=AI_EDITOR&profileArn=%s&resourceType=AGENTIC_REQUEST",
		strings.TrimRight(endpoint, "/"), url.QueryEscape(token.ProfileArn))
	req, err := newRequest(ctx, http.MethodGet, requestURL)
	if err != nil {
		return nil, updated, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, updated, fmt.Errorf("error requesting Kiro usage: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, updated, fmt.Errorf("Kiro session expired or banned. Re-import the account.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, updated, fmt.Errorf("Kiro usage API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, updated, fmt.Errorf("error parsing Kiro usage response: %w", err)
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	if subscription, ok := payload["subscriptionInfo"].(map[string]any); ok {
		quota.PlanType = firstNonEmpty(jsonString(subscription["subscriptionTitle"]), jsonString(subscription["type"]))
	}
	resetAt := pickIntAlias(payload, "nextDateReset", "next_date_reset")
	breakdowns, _ := payload["usageBreakdownList"].([]any)
	for _, item := range breakdowns {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		limit := pickIntAlias(entry, "usageLimitWithPrecision", "usageLimit")
		used := pickIntAlias(entry, "currentUsageWithPrecision", "currentUsage")
		if limit <= 0 {
			continue
		}
		name := firstNonEmpty(jsonString(entry["displayName"]), jsonString(entry["resourceType"]), "Credits")
		remaining := (float64(limit-used) / float64(limit)) * 100
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             name,
			RemainingPercent: normalizePercent(remaining),
			ResetAt:          resetAt,
		})
	}
	if len(quota.Metrics) == 0 {
		return nil, updated, fmt.Errorf("Kiro usage response did not include usage breakdowns")
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: resetAt}
	return quota, updated, nil
}
