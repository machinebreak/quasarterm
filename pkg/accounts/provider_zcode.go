// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const (
	zcodeCredentialPrefix = "enc:v1:"
	zcodeBillingURL       = "https://zcode.z.ai/api/v1/zcode-plan/billing/balance"
	zcodeActiveProvider   = "oauth:active_provider"
	zcodeJWTKey           = "zcodejwttoken"
)

type ZCodeProvider struct{}

func (p *ZCodeProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "zcode",
		Name:        "ZCode",
		Description: "Z.ai ZCode",
		CLICommand:  "zcode",
		ResumeArgs:  "",
		Icon:        "zcode",
		Order:       130,
		Available:   true,
	}
}

func zcodeHomeDir() string {
	return wavebase.GetHomeDir()
}

func zcodeCredentialsPath() string {
	return filepath.Join(zcodeHomeDir(), ".zcode", "v2", "credentials.json")
}

func zcodeUsername() string {
	for _, key := range []string{"USERNAME", "USER"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return "user"
}

func zcodeFallbackSecret() string {
	return fmt.Sprintf("zcode-credential-fallback:%s:%s:%s", runtime.GOOS, zcodeHomeDir(), zcodeUsername())
}

func zcodeCredentialKey() []byte {
	secret := os.Getenv("ZCODE_CREDENTIAL_SECRET")
	if secret == "" {
		secret = zcodeFallbackSecret()
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func zcodeDecrypt(value string) (string, error) {
	if !strings.HasPrefix(value, zcodeCredentialPrefix) {
		return value, nil
	}
	parts := strings.Split(strings.TrimPrefix(value, zcodeCredentialPrefix), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid ZCode credential ciphertext")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(nonce) != 12 {
		return "", fmt.Errorf("invalid ZCode nonce")
	}
	tag, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(tag) != 16 {
		return "", fmt.Errorf("invalid ZCode tag")
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid ZCode ciphertext")
	}
	encrypted = append(encrypted, tag...)
	block, err := aes.NewCipher(zcodeCredentialKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt ZCode credential (user/home mismatch)")
	}
	return string(plain), nil
}

func zcodeEncrypt(value string) (string, error) {
	block, err := aes.NewCipher(zcodeCredentialKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(value), nil)
	tag := sealed[len(sealed)-16:]
	encrypted := sealed[:len(sealed)-16]
	return fmt.Sprintf("%s%s.%s.%s", zcodeCredentialPrefix,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(tag),
		base64.RawURLEncoding.EncodeToString(encrypted)), nil
}

func readZCodeCredentialFile() (map[string]string, error) {
	path := zcodeCredentialsPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ZCode credentials not found at %s (log in to ZCode first)", path)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("invalid ZCode credentials file: %w", err)
	}
	values := map[string]string{}
	for key, value := range parsed {
		if str, ok := value.(string); ok {
			values[key] = str
		}
	}
	return values, nil
}

type zcodeStoredCredentials struct {
	Provider     string `json:"provider"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ZCodeJWT     string `json:"zcodeJwt,omitempty"`
	UserInfo     string `json:"userInfo,omitempty"`
}

func parseZCodeStoredCredentials(credentials []byte) (*zcodeStoredCredentials, error) {
	var creds zcodeStoredCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid ZCode credentials: %w", err)
	}
	if creds.Provider == "" || creds.AccessToken == "" {
		return nil, fmt.Errorf("ZCode credentials are incomplete")
	}
	return &creds, nil
}

func zcodeDecryptedValue(values map[string]string, key string) (string, error) {
	raw, found := values[key]
	if !found || raw == "" {
		return "", nil
	}
	return zcodeDecrypt(raw)
}

func (p *ZCodeProvider) CurrentCredentialsPath() (string, error) {
	path := zcodeCredentialsPath()
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("ZCode credentials not found at %s", path)
	}
	return path, nil
}

func (p *ZCodeProvider) ImportCurrent() (*Account, []byte, error) {
	values, err := readZCodeCredentialFile()
	if err != nil {
		return nil, nil, err
	}
	provider, err := zcodeDecryptedValue(values, zcodeActiveProvider)
	if err != nil {
		return nil, nil, err
	}
	if provider == "" {
		return nil, nil, fmt.Errorf("ZCode credentials do not contain an active OAuth provider")
	}
	accessToken, err := zcodeDecryptedValue(values, fmt.Sprintf("oauth:%s:access_token", provider))
	if err != nil {
		return nil, nil, err
	}
	if accessToken == "" {
		return nil, nil, fmt.Errorf("ZCode credentials are missing an access token")
	}
	refreshToken, _ := zcodeDecryptedValue(values, fmt.Sprintf("oauth:%s:refresh_token", provider))
	zcodeJWT, _ := zcodeDecryptedValue(values, zcodeJWTKey)
	userInfo, _ := zcodeDecryptedValue(values, fmt.Sprintf("oauth:%s:user_info", provider))
	label := ""
	if userInfo != "" {
		var info map[string]any
		if json.Unmarshal([]byte(userInfo), &info) == nil {
			label = firstNonEmpty(jsonString(info["email"]), jsonString(info["name"]), jsonString(info["nickname"]))
		}
	}
	if label == "" {
		label = "ZCode " + provider
	}
	creds := zcodeStoredCredentials{
		Provider:     provider,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ZCodeJWT:     zcodeJWT,
		UserInfo:     userInfo,
	}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	account := &Account{
		ID:        makeAccountID("zcode", hashKey(provider+accessToken)[:24]),
		Provider:  "zcode",
		RemoteID:  provider,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *ZCodeProvider) RemoteKey(credentials []byte) (string, error) {
	creds, err := parseZCodeStoredCredentials(credentials)
	if err != nil {
		return "", err
	}
	return creds.Provider, nil
}

func (p *ZCodeProvider) SwitchAccount(account *Account, credentials []byte) error {
	creds, err := parseZCodeStoredCredentials(credentials)
	if err != nil {
		return err
	}
	values := map[string]string{}
	if existing, readErr := readZCodeCredentialFile(); readErr == nil {
		values = existing
	}
	put := func(key string, value string) error {
		if value == "" {
			return nil
		}
		encrypted, err := zcodeEncrypt(value)
		if err != nil {
			return err
		}
		values[key] = encrypted
		return nil
	}
	if err := put(zcodeActiveProvider, creds.Provider); err != nil {
		return err
	}
	if err := put(fmt.Sprintf("oauth:%s:access_token", creds.Provider), creds.AccessToken); err != nil {
		return err
	}
	if err := put(fmt.Sprintf("oauth:%s:refresh_token", creds.Provider), creds.RefreshToken); err != nil {
		return err
	}
	if err := put(zcodeJWTKey, creds.ZCodeJWT); err != nil {
		return err
	}
	if err := put(fmt.Sprintf("oauth:%s:user_info", creds.Provider), creds.UserInfo); err != nil {
		return err
	}
	path := zcodeCredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, encoded, 0600)
}

type zcodePlan struct {
	Name   string `json:"name"`
	PlanID string `json:"plan_id"`
	Status string `json:"status"`
}

type zcodeBalance struct {
	TotalUnits     any `json:"total_units"`
	UsedUnits      any `json:"used_units"`
	RemainingUnits any `json:"remaining_units"`
	AvailableUnits any `json:"available_units"`
	PeriodEnd      any `json:"period_end"`
	ExpiresAt      any `json:"expires_at"`
}

type zcodeBalanceResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Plans    []zcodePlan    `json:"plans"`
		Balances []zcodeBalance `json:"balances"`
	} `json:"data"`
}

func zcodeNumber(value any) float64 {
	f, _ := jsonFloat(value)
	return f
}

func (p *ZCodeProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	creds, err := parseZCodeStoredCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	token := creds.ZCodeJWT
	if token == "" {
		token = creds.AccessToken
	}
	req, err := newRequest(ctx, http.MethodGet, zcodeBillingURL)
	if err != nil {
		return nil, credentials, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-request-id", fmt.Sprintf("quasar-%d", nowMillis()))
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, credentials, fmt.Errorf("error requesting ZCode billing: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, credentials, fmt.Errorf("ZCode session expired. Re-import the account.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, credentials, fmt.Errorf("ZCode billing API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var payload zcodeBalanceResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, credentials, fmt.Errorf("error parsing ZCode billing response: %w", err)
	}
	if payload.Code != 0 {
		message := payload.Msg
		if message == "" {
			message = "billing request failed"
		}
		return nil, credentials, fmt.Errorf("ZCode billing: %s", message)
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	for _, plan := range payload.Data.Plans {
		if plan.Status == "active" || quota.PlanType == "" {
			quota.PlanType = firstNonEmpty(plan.Name, plan.PlanID, quota.PlanType)
		}
	}
	var total, remaining float64
	resetAt := int64(0)
	for _, balance := range payload.Data.Balances {
		total += zcodeNumber(balance.TotalUnits)
		remaining += zcodeNumber(balance.RemainingUnits)
		if zcodeNumber(balance.RemainingUnits) == 0 && balance.AvailableUnits != nil {
			remaining += zcodeNumber(balance.AvailableUnits)
		}
		if resetAt == 0 {
			resetAt = jsonInt64(balance.PeriodEnd)
			if resetAt == 0 {
				resetAt = jsonInt64(balance.ExpiresAt)
			}
		}
	}
	if total > 0 {
		remainingPercent := (remaining / total) * 100
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             "Plan Credits",
			RemainingPercent: normalizePercent(remainingPercent),
			ResetAt:          resetAt,
		})
		quota.Primary = &QuotaWindow{RemainingPercent: normalizePercent(remainingPercent), ResetAt: resetAt}
		if remainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	if len(quota.Metrics) == 0 {
		return nil, credentials, fmt.Errorf("ZCode billing response did not include balances")
	}
	return quota, credentials, nil
}

func jsonInt64(value any) int64 {
	if value == nil {
		return 0
	}
	if f, ok := jsonFloat(value); ok {
		return int64(f)
	}
	if text, ok := value.(string); ok {
		return parseISO8601(text)
	}
	return 0
}
