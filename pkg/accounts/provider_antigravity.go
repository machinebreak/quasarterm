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
	"strings"
	"time"
)

const (
	antigravityCredentialTarget = "gemini:antigravity"
	antigravityCredentialUser   = "antigravity"
	antigravityClientID         = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityClientSecret     = "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"
	antigravityTokenURL         = "https://oauth2.googleapis.com/token"
	antigravityUserInfoURL      = "https://www.googleapis.com/oauth2/v2/userinfo"
	antigravityCloudCodeURL     = "https://cloudcode-pa.googleapis.com"
	antigravityCloudCodeDaily   = "https://daily-cloudcode-pa.googleapis.com"
	antigravityUserAgent        = "antigravity/1.20.5 windows/amd64 google-api-nodejs-client/10.3.0"
	antigravityGoogAPIClient    = "gl-node/22.21.1"
)

type AntigravityProvider struct{}

func (p *AntigravityProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "antigravity",
		Name:        "Antigravity",
		Description: "Antigravity IDE",
		CLICommand:  "antigravity",
		ResumeArgs:  "",
		Icon:        "antigravity",
		Order:       1,
		Available:   true,
	}
}

type antigravityToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type,omitempty"`
	RefreshToken string `json:"refresh_token"`
	Expiry       string `json:"expiry,omitempty"`
}

type antigravityCredential struct {
	Token      antigravityToken `json:"token"`
	AuthMethod string           `json:"auth_method,omitempty"`
}

func parseAntigravityCredential(secret string) (*antigravityCredential, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, fmt.Errorf("Antigravity system credential is empty")
	}
	var credential antigravityCredential
	if err := json.Unmarshal([]byte(secret), &credential); err != nil {
		return nil, fmt.Errorf("invalid Antigravity credential: %w", err)
	}
	if credential.Token.RefreshToken == "" {
		return nil, fmt.Errorf("Antigravity credential has no refresh token")
	}
	return &credential, nil
}

func parseAntigravityCredentialBlob(credentials []byte) (*antigravityCredential, error) {
	return parseAntigravityCredential(string(credentials))
}

func (p *AntigravityProvider) CurrentCredentialsPath() (string, error) {
	_, secret, err := winCredReadTarget(antigravityCredentialTarget)
	if err != nil {
		return "", fmt.Errorf("Antigravity credentials not found (log in to Antigravity first)")
	}
	if secret == "" {
		return "", fmt.Errorf("Antigravity credential entry is empty")
	}
	return "Windows Credential Manager (" + antigravityCredentialTarget + ")", nil
}

func antigravityRefreshToken(ctx context.Context, refreshToken string) (*antigravityToken, error) {
	form := url.Values{}
	form.Set("client_id", antigravityClientID)
	form.Set("client_secret", antigravityClientSecret)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, antigravityTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Antigravity token refresh failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Antigravity token refresh failed (%d): %s", resp.StatusCode, summarizeBody(body))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.AccessToken == "" {
		return nil, fmt.Errorf("Antigravity token refresh returned an empty access token")
	}
	expiry := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	return &antigravityToken{AccessToken: payload.AccessToken, TokenType: "Bearer", RefreshToken: refreshToken, Expiry: expiry}, nil
}

func antigravityTokenExpired(token *antigravityToken) bool {
	if token.AccessToken == "" {
		return true
	}
	if token.Expiry == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, token.Expiry)
	if err != nil {
		return false
	}
	return parsed.Before(time.Now().Add(5 * time.Minute))
}

func antigravityFetchUserInfo(ctx context.Context, accessToken string) (string, string) {
	req, err := newRequest(ctx, http.MethodGet, antigravityUserInfoURL)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var info struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", ""
	}
	return info.Email, info.Name
}

func (p *AntigravityProvider) ImportCurrent() (*Account, []byte, error) {
	_, secret, err := winCredReadTarget(antigravityCredentialTarget)
	if err != nil {
		return nil, nil, fmt.Errorf("Antigravity credentials not found (log in to Antigravity first)")
	}
	credential, err := parseAntigravityCredential(secret)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if antigravityTokenExpired(&credential.Token) {
		if refreshed, refreshErr := antigravityRefreshToken(ctx, credential.Token.RefreshToken); refreshErr == nil {
			credential.Token = *refreshed
		}
	}
	email, name := antigravityFetchUserInfo(ctx, credential.Token.AccessToken)
	credentials, _ := json.Marshal(credential)
	remoteKey := hashKey(credential.Token.RefreshToken)[:24]
	label := firstNonEmpty(email, name, "Antigravity account")
	account := &Account{
		ID:        makeAccountID("antigravity", remoteKey),
		Provider:  "antigravity",
		Email:     email,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *AntigravityProvider) RemoteKey(credentials []byte) (string, error) {
	credential, err := parseAntigravityCredentialBlob(credentials)
	if err != nil {
		return "", err
	}
	return hashKey(credential.Token.RefreshToken)[:24], nil
}

func (p *AntigravityProvider) ActiveRemoteKey() (string, error) {
	_, secret, err := winCredReadTarget(antigravityCredentialTarget)
	if err != nil {
		return "", err
	}
	credential, err := parseAntigravityCredential(secret)
	if err != nil {
		return "", err
	}
	return hashKey(credential.Token.RefreshToken)[:24], nil
}

func (p *AntigravityProvider) SwitchAccount(account *Account, credentials []byte) error {
	credential, err := parseAntigravityCredentialBlob(credentials)
	if err != nil {
		return err
	}
	output, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	return winCredWriteTarget(antigravityCredentialTarget, antigravityCredentialUser, string(output))
}

type antigravityTier struct {
	ID               string `json:"id"`
	AvailableCredits []struct {
		CreditType   string `json:"creditType"`
		CreditAmount string `json:"creditAmount"`
	} `json:"availableCredits"`
}

type antigravityLoadResponse struct {
	CloudAICodeProject any             `json:"cloudaicompanionProject"`
	CurrentTier        *antigravityTier `json:"currentTier"`
	PaidTier           *antigravityTier `json:"paidTier"`
}

type antigravityModelsResponse struct {
	Models map[string]struct {
		DisplayName string `json:"displayName"`
		QuotaInfo   *struct {
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
		} `json:"quotaInfo"`
	} `json:"models"`
}

func antigravityProjectID(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	if obj, ok := value.(map[string]any); ok {
		return firstNonEmpty(jsonString(obj["id"]), jsonString(obj["projectId"]))
	}
	return ""
}

func antigravityPost(ctx context.Context, baseURL string, path string, accessToken string, payload any) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/"+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", antigravityUserAgent)
	req.Header.Set("x-goog-api-client", antigravityGoogAPIClient)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return respBody, resp.StatusCode, nil
}

func (p *AntigravityProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	credential, err := parseAntigravityCredentialBlob(credentials)
	if err != nil {
		return nil, nil, err
	}
	updated := credentials
	if antigravityTokenExpired(&credential.Token) {
		refreshed, refreshErr := antigravityRefreshToken(ctx, credential.Token.RefreshToken)
		if refreshErr != nil {
			return nil, updated, refreshErr
		}
		credential.Token = *refreshed
		updated, _ = json.Marshal(credential)
	}
	accessToken := credential.Token.AccessToken
	loadPayload := map[string]any{
		"metadata": map[string]any{
			"ideName":       "antigravity",
			"ideType":       "ANTIGRAVITY",
			"ideVersion":    "1.20.5",
			"pluginType":    "GEMINI",
			"platform":      "WINDOWS_AMD64",
			"updateChannel": "stable",
		},
		"mode": "FULL_ELIGIBILITY_CHECK",
	}
	loadBody := []byte(nil)
	loadStatus := 0
	baseURL := antigravityCloudCodeURL
	loadBody, loadStatus, err = antigravityPost(ctx, baseURL, "v1internal:loadCodeAssist", accessToken, loadPayload)
	if err != nil || loadStatus != http.StatusOK {
		loadBody, loadStatus, err = antigravityPost(ctx, antigravityCloudCodeDaily, "v1internal:loadCodeAssist", accessToken, loadPayload)
	}
	if err != nil {
		return nil, updated, fmt.Errorf("error requesting Antigravity project info: %w", err)
	}
	if loadStatus == http.StatusUnauthorized || loadStatus == http.StatusForbidden {
		return nil, updated, fmt.Errorf("Antigravity session expired. Re-import the account.")
	}
	if loadStatus != http.StatusOK {
		return nil, updated, fmt.Errorf("Antigravity loadCodeAssist returned %d: %s", loadStatus, summarizeBody(loadBody))
	}
	var loadResponse antigravityLoadResponse
	if err := json.Unmarshal(loadBody, &loadResponse); err != nil {
		return nil, updated, fmt.Errorf("error parsing Antigravity loadCodeAssist: %w", err)
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	tier := loadResponse.PaidTier
	if tier == nil {
		tier = loadResponse.CurrentTier
	}
	if tier != nil && tier.ID != "" {
		quota.PlanType = strings.ToUpper(tier.ID)
	}
	projectID := antigravityProjectID(loadResponse.CloudAICodeProject)
	modelsPayload := map[string]any{}
	if projectID != "" {
		modelsPayload["project"] = projectID
	}
	modelsBody, modelsStatus, modelsErr := antigravityPost(ctx, baseURL, "v1internal:fetchAvailableModels", accessToken, modelsPayload)
	if modelsErr != nil || modelsStatus != http.StatusOK {
		modelsBody, modelsStatus, modelsErr = antigravityPost(ctx, antigravityCloudCodeDaily, "v1internal:fetchAvailableModels", accessToken, modelsPayload)
	}
	if modelsErr == nil && modelsStatus == http.StatusOK {
		var modelsResponse antigravityModelsResponse
		if json.Unmarshal(modelsBody, &modelsResponse) == nil {
			for name, model := range modelsResponse.Models {
				if model.QuotaInfo == nil || model.QuotaInfo.RemainingFraction == nil {
					continue
				}
				lowerName := strings.ToLower(name)
				if !strings.Contains(lowerName, "gemini") && !strings.Contains(lowerName, "claude") {
					continue
				}
				metricName := firstNonEmpty(model.DisplayName, name)
				quota.Metrics = append(quota.Metrics, QuotaMetric{
					Name:             metricName,
					RemainingPercent: normalizePercent(*model.QuotaInfo.RemainingFraction * 100),
					ResetAt:          parseISO8601(model.QuotaInfo.ResetTime),
				})
			}
		}
	}
	if len(quota.Metrics) == 0 {
		if tier != nil && len(tier.AvailableCredits) > 0 {
			quota.Metrics = append(quota.Metrics, QuotaMetric{Name: "Credits", RemainingPercent: 100})
		} else {
			return nil, updated, fmt.Errorf("Antigravity response did not include model quota")
		}
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: quota.Metrics[0].ResetAt}
	return quota, updated, nil
}
