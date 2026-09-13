// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const codebuddyExtensionID = "tencent-cloud.coding-copilot"

type codebuddyVariant struct {
	id            string
	name          string
	description   string
	icon          string
	order         float64
	apiEndpoint   string
	secretKey     string
	dataDirNames  []string
	resumeArgs    string
}

var codebuddyVariants = map[string]codebuddyVariant{
	"codebuddy": {
		id:           "codebuddy",
		name:         "CodeBuddy",
		description:  "CodeBuddy IDE",
		icon:         "codebuddy",
		order:        80,
		apiEndpoint:  "https://www.codebuddy.ai",
		secretKey:    "planning-genie.new.accessToken",
		dataDirNames: []string{"CodeBuddy", "codebuddy"},
	},
	"codebuddy-cn": {
		id:           "codebuddy-cn",
		name:         "CodeBuddy CN",
		description:  "CodeBuddy China",
		icon:         "codebuddy",
		order:        82,
		apiEndpoint:  "https://www.codebuddy.cn",
		secretKey:    "planning-genie.new.accessTokencn",
		dataDirNames: []string{"CodeBuddy CN", "codebuddy cn", "codebuddy-cn", "codebuddycn"},
	},
}

type codebuddyProvider struct {
	variant codebuddyVariant
}

func newCodeBuddyProvider(variantKey string) *codebuddyProvider {
	return &codebuddyProvider{variant: codebuddyVariants[variantKey]}
}

func (p *codebuddyProvider) Info() ProviderInfo {
	v := p.variant
	return ProviderInfo{ID: v.id, Name: v.name, Description: v.description, CLICommand: "codebuddy",
		Icon: v.icon, Order: v.order, Available: true}
}

func (p *codebuddyProvider) ActiveRemoteKey() (string, error) {
	secret, _, err := p.readSecret()
	if err != nil {
		return "", err
	}
	creds, err := codebuddyParseSecret(secret)
	if err != nil {
		return "", err
	}
	if creds.UID != "" {
		return creds.UID, nil
	}
	return hashKey(creds.AccessToken)[:24], nil
}

func codebuddyStateDBPath(variant codebuddyVariant) (string, error) {
	roots := []string{}
	for _, envName := range []string{"APPDATA", "LOCALAPPDATA"} {
		base := os.Getenv(envName)
		if base == "" {
			continue
		}
		for _, dirName := range variant.dataDirNames {
			roots = append(roots, filepath.Join(base, dirName))
		}
	}
	for _, root := range roots {
		dbPath := filepath.Join(root, "User", "globalStorage", "state.vscdb")
		if _, err := os.Stat(dbPath); err == nil {
			return dbPath, nil
		}
	}
	return "", fmt.Errorf("%s state.vscdb not found (log in to %s first)", variant.name, variant.name)
}

func codebuddySecretItemKey(variant codebuddyVariant) string {
	return fmt.Sprintf(`secret://{"extensionId":"%s","key":"%s"}`, codebuddyExtensionID, variant.secretKey)
}

type codebuddyCredentials struct {
	AccessToken  string          `json:"accessToken"`
	RefreshToken string          `json:"refreshToken,omitempty"`
	UID          string          `json:"uid,omitempty"`
	Email        string          `json:"email,omitempty"`
	Nickname     string          `json:"nickname,omitempty"`
	EnterpriseID string          `json:"enterpriseId,omitempty"`
	Domain       string          `json:"domain,omitempty"`
	ExpiresAt    int64           `json:"expiresAt,omitempty"`
	SecretRaw    json.RawMessage `json:"secretRaw,omitempty"`
}

func parseCodeBuddyCredentials(credentials []byte) (*codebuddyCredentials, error) {
	var creds codebuddyCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid CodeBuddy credentials: %w", err)
	}
	if creds.AccessToken == "" {
		return nil, fmt.Errorf("CodeBuddy credentials have an empty access token")
	}
	return &creds, nil
}

func codebuddyStringAlias(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := jsonString(obj[key]); value != "" {
			return value
		}
	}
	return ""
}

func codebuddyParseSecret(secret string) (*codebuddyCredentials, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, fmt.Errorf("empty CodeBuddy secret")
	}
	creds := &codebuddyCredentials{}
	if strings.HasPrefix(secret, "{") {
		var root map[string]any
		if err := json.Unmarshal([]byte(secret), &root); err != nil {
			return nil, fmt.Errorf("invalid CodeBuddy secret JSON: %w", err)
		}
		account, _ := root["account"].(map[string]any)
		auth, _ := root["auth"].(map[string]any)
		creds.SecretRaw = json.RawMessage(secret)
		creds.AccessToken = codebuddyStringAlias(root, "accessToken", "access_token", "token")
		if creds.AccessToken == "" && auth != nil {
			creds.AccessToken = codebuddyStringAlias(auth, "accessToken", "access_token", "token")
		}
		creds.RefreshToken = codebuddyStringAlias(root, "refreshToken", "refresh_token")
		if creds.RefreshToken == "" && auth != nil {
			creds.RefreshToken = codebuddyStringAlias(auth, "refreshToken", "refresh_token")
		}
		creds.UID = codebuddyStringAlias(root, "uid")
		if creds.UID == "" && account != nil {
			creds.UID = codebuddyStringAlias(account, "uid", "id")
		}
		creds.Email = codebuddyStringAlias(root, "email")
		if creds.Email == "" && account != nil {
			creds.Email = codebuddyStringAlias(account, "email")
		}
		creds.Nickname = codebuddyStringAlias(root, "nickname", "name")
		creds.EnterpriseID = codebuddyStringAlias(root, "enterpriseId", "enterprise_id")
		creds.Domain = codebuddyStringAlias(root, "domain")
	} else {
		creds.AccessToken = secret
	}
	if creds.AccessToken == "" {
		return nil, fmt.Errorf("CodeBuddy secret does not contain an access token")
	}
	creds.AccessToken = codebuddyNormalizeToken(creds.AccessToken)
	return creds, nil
}

func codebuddyNormalizeToken(token string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		return token
	}
	if separator := strings.Index(token, "::"); separator > 0 {
		suffix := token[separator+2:]
		if strings.HasPrefix(strings.ToLower(suffix), "bearer ") {
			suffix = strings.TrimSpace(suffix[7:])
		}
		if suffix != "" {
			return suffix
		}
	}
	return token
}

func (p *codebuddyProvider) readSecret() (string, string, error) {
	dbPath, err := codebuddyStateDBPath(p.variant)
	if err != nil {
		return "", "", err
	}
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return "", "", err
	}
	defer db.Close()
	var raw string
	if err := db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", codebuddySecretItemKey(p.variant)).Scan(&raw); err != nil {
		return "", "", fmt.Errorf("%s secret not found in state.vscdb", p.variant.name)
	}
	secret, err := decodeCodeBuddySecretValue(raw, filepath.Dir(filepath.Dir(filepath.Dir(dbPath))))
	if err != nil {
		return "", "", err
	}
	return secret, dbPath, nil
}

func (p *codebuddyProvider) CurrentCredentialsPath() (string, error) {
	return codebuddyStateDBPath(p.variant)
}

func (p *codebuddyProvider) ImportCurrent() (*Account, []byte, error) {
	secret, _, err := p.readSecret()
	if err != nil {
		return nil, nil, err
	}
	creds, err := codebuddyParseSecret(secret)
	if err != nil {
		return nil, nil, err
	}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	label := firstNonEmpty(creds.Email, creds.Nickname, creds.UID, p.variant.name+" account")
	remoteKey := creds.UID
	if remoteKey == "" {
		remoteKey = hashKey(creds.AccessToken)[:24]
	}
	account := &Account{
		ID:        makeAccountID(p.variant.id, remoteKey),
		Provider:  p.variant.id,
		Email:     creds.Email,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *codebuddyProvider) RemoteKey(credentials []byte) (string, error) {
	creds, err := parseCodeBuddyCredentials(credentials)
	if err != nil {
		return "", err
	}
	if creds.UID != "" {
		return creds.UID, nil
	}
	return hashKey(creds.AccessToken)[:24], nil
}

func (p *codebuddyProvider) SwitchAccount(account *Account, credentials []byte) error {
	creds, err := parseCodeBuddyCredentials(credentials)
	if err != nil {
		return err
	}
	dbPath, err := codebuddyStateDBPath(p.variant)
	if err != nil {
		return err
	}
	secret := string(creds.SecretRaw)
	if secret == "" {
		payload := map[string]any{
			"accessToken": creds.AccessToken,
			"uid":         creds.UID,
			"email":       creds.Email,
			"nickname":    creds.Nickname,
		}
		if creds.RefreshToken != "" {
			payload["refreshToken"] = creds.RefreshToken
		}
		encoded, _ := json.Marshal(payload)
		secret = string(encoded)
	}
	encrypted, err := encodeCodeBuddySecretValue(secret, filepath.Dir(filepath.Dir(filepath.Dir(dbPath))))
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("INSERT OR REPLACE INTO ItemTable (key, value) VALUES (?, ?)",
		codebuddySecretItemKey(p.variant), encrypted)
	return err
}

func (p *codebuddyProvider) refreshTokens(ctx context.Context, creds *codebuddyCredentials) (*codebuddyCredentials, bool) {
	if creds.RefreshToken == "" {
		return creds, false
	}
	url := p.variant.apiEndpoint + "/v2/plugin/auth/token/refresh"
	req, err := newRequest(ctx, http.MethodPost, url)
	if err != nil {
		return creds, false
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("X-Refresh-Token", creds.RefreshToken)
	req.Header.Set("X-Auth-Refresh-Source", "ide-main")
	if creds.Domain != "" {
		req.Header.Set("X-Domain", creds.Domain)
	}
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
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return creds, false
	}
	accessToken := codebuddyStringAlias(data, "accessToken", "access_token")
	if accessToken == "" {
		return creds, false
	}
	creds.AccessToken = accessToken
	if refreshToken := codebuddyStringAlias(data, "refreshToken", "refresh_token"); refreshToken != "" {
		creds.RefreshToken = refreshToken
	}
	return creds, true
}

type codebuddyResource struct {
	Total   float64
	Remain  float64
	ResetAt int64
	Name    string
}

func codebuddyNumber(value any) float64 {
	switch typed := value.(type) {
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &parsed); err == nil {
			return parsed
		}
	case map[string]any:
		for _, key := range []string{"value", "Value", "amount"} {
			if f, ok := jsonFloat(typed[key]); ok {
				return f
			}
		}
	}
	f, _ := jsonFloat(value)
	return f
}

func codebuddyFindItems(payload map[string]any) []map[string]any {
	for _, path := range [][]string{
		{"data", "resources"},
		{"data", "data", "resources"},
		{"data", "Response", "Data", "Accounts"},
		{"data", "data", "Response", "Data", "Accounts"},
		{"Response", "Data", "Accounts"},
	} {
		current := any(payload)
		for _, key := range path {
			obj, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = obj[key]
		}
		if list, ok := current.([]any); ok {
			items := []map[string]any{}
			for _, entry := range list {
				if obj, ok := entry.(map[string]any); ok {
					items = append(items, obj)
				}
			}
			if len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func codebuddyResourceFromItem(item map[string]any) *codebuddyResource {
	lower := map[string]any{}
	for key, value := range item {
		lower[strings.ToLower(key)] = value
	}
	lookup := func(keys ...string) any {
		for _, key := range keys {
			if value, found := lower[strings.ToLower(key)]; found {
				return value
			}
		}
		return nil
	}
	total := codebuddyNumber(lookup("CycleCapacity", "cycle_capacity", "limit_num", "limitNum", "TotalCapacity", "total"))
	remain := codebuddyNumber(lookup("CycleCapacityRemainPrecise", "CycleCapacityRemain", "CycleRemain", "remain",
		"remaining_units", "RemainCapacity", "remainCapacity"))
	used := codebuddyNumber(lookup("CycleCapacityUsed", "credit", "used_num", "usedNum", "UsedCapacity", "used"))
	resetAt := jsonInt64(lookup("CycleResetTime", "cycle_reset_time", "ResetTime", "expire_time", "end_time"))
	if total <= 0 && remain > 0 && used > 0 {
		total = remain + used
	}
	if total <= 0 && remain > 0 {
		total = remain
	}
	if total <= 0 {
		return nil
	}
	if remain <= 0 && used > 0 {
		remain = total - used
	}
	name := firstNonEmpty(
		jsonString(lookup("ResourceName", "resource_name", "ProductName", "product_name", "name")),
		"Credits",
	)
	return &codebuddyResource{Total: total, Remain: remain, ResetAt: resetAt, Name: name}
}

func (p *codebuddyProvider) fetchEnterpriseUsage(ctx context.Context, creds *codebuddyCredentials) ([]map[string]any, error) {
	url := p.variant.apiEndpoint + "/v2/billing/meter/get-enterprise-user-usage"
	body, _ := json.Marshal(map[string]any{})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Enterprise-Id", creds.EnterpriseID)
	req.Header.Set("X-Tenant-Id", creds.EnterpriseID)
	if creds.UID != "" {
		req.Header.Set("X-User-Id", creds.UID)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CodeBuddy enterprise usage API returned %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, err
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		if nested, ok := payload["data"].(map[string]any); ok {
			if inner, ok := nested["data"].(map[string]any); ok {
				data = inner
			}
		}
	}
	if data == nil {
		return nil, fmt.Errorf("CodeBuddy enterprise usage response was empty")
	}
	return []map[string]any{data}, nil
}

func (p *codebuddyProvider) fetchUserResource(ctx context.Context, creds *codebuddyCredentials) ([]map[string]any, error) {
	url := p.variant.apiEndpoint + "/v2/billing/meter/get-user-resource"
	now := time.Now()
	body, _ := json.Marshal(map[string]any{
		"PageNumber":                 1,
		"PageSize":                   100,
		"ProductCode":                "p_tcaca",
		"Status":                     []int{0, 3},
		"PackageEndTimeRangeBegin":   now.Format("2006-01-02 15:04:05"),
		"PackageEndTimeRangeEnd":     now.AddDate(5, 0, 0).Format("2006-01-02 15:04:05"),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	if creds.UID != "" {
		req.Header.Set("X-User-Id", creds.UID)
	}
	if creds.EnterpriseID != "" {
		req.Header.Set("X-Enterprise-Id", creds.EnterpriseID)
		req.Header.Set("X-Tenant-Id", creds.EnterpriseID)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CodeBuddy user resource API returned %d: %s", resp.StatusCode, summarizeBody(respBody))
	}
	var payload map[string]any
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, err
	}
	return codebuddyFindItems(payload), nil
}

func (p *codebuddyProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	creds, err := parseCodeBuddyCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	updated := credentials
	if newCreds, refreshed := p.refreshTokens(ctx, creds); refreshed {
		newCredentials, _ := json.MarshalIndent(newCreds, "", "  ")
		updated = newCredentials
		creds = newCreds
	}
	var items []map[string]any
	if creds.EnterpriseID != "" {
		items, err = p.fetchEnterpriseUsage(ctx, creds)
		if err != nil {
			items, err = p.fetchUserResource(ctx, creds)
		}
	} else {
		items, err = p.fetchUserResource(ctx, creds)
	}
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
			return nil, updated, fmt.Errorf("CodeBuddy session expired. Re-import the account.")
		}
		return nil, updated, err
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	for _, item := range items {
		resource := codebuddyResourceFromItem(item)
		if resource == nil {
			continue
		}
		remaining := (resource.Remain / resource.Total) * 100
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             resource.Name,
			RemainingPercent: normalizePercent(remaining),
			ResetAt:          resource.ResetAt,
		})
	}
	if len(quota.Metrics) == 0 {
		return nil, updated, fmt.Errorf("CodeBuddy response did not include usage resources")
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: quota.Metrics[0].ResetAt}
	return quota, updated, nil
}
