// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

const (
	qoderUserInfoKey   = "secret://aicoding.auth.userInfo"
	qoderUserPlanKey   = "secret://aicoding.auth.userPlan"
	qoderCreditUsageKey = "secret://aicoding.auth.creditUsage"
)

type QoderProvider struct{}

func (p *QoderProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "qoder",
		Name:        "Qoder",
		Description: "Qoder IDE",
		CLICommand:  "qoder",
		ResumeArgs:  "",
		Icon:        "qoder",
		Order:       90,
		Available:   true,
	}
}

func qoderStateDBPath() (string, error) {
	candidates := []string{}
	for _, envName := range []string{"APPDATA", "LOCALAPPDATA"} {
		base := os.Getenv(envName)
		if base == "" {
			continue
		}
		for _, dirName := range []string{"Qoder", "qoder"} {
			candidates = append(candidates, filepath.Join(base, dirName, "User", "globalStorage", "state.vscdb"))
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Qoder state.vscdb not found (log in to Qoder first)")
}

type qoderCredentials struct {
	UserInfo    json.RawMessage `json:"userInfo,omitempty"`
	UserPlan    json.RawMessage `json:"userPlan,omitempty"`
	CreditUsage json.RawMessage `json:"creditUsage,omitempty"`
}

func parseQoderCredentials(credentials []byte) (*qoderCredentials, error) {
	var creds qoderCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid Qoder credentials: %w", err)
	}
	if len(creds.UserInfo) == 0 && len(creds.CreditUsage) == 0 {
		return nil, fmt.Errorf("Qoder credentials are empty")
	}
	return &creds, nil
}

func qoderReadSecret(db *sql.DB, key string, dataRoot string) (json.RawMessage, error) {
	var raw string
	if err := db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", key).Scan(&raw); err != nil {
		return nil, nil
	}
	decoded, err := decodeCodeBuddySecretValue(raw, dataRoot)
	if err != nil {
		return nil, err
	}
	decoded = trimJSONString(decoded)
	if json.Valid([]byte(decoded)) {
		return json.RawMessage(decoded), nil
	}
	encoded, _ := json.Marshal(decoded)
	return json.RawMessage(encoded), nil
}

func trimJSONString(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		var unquoted string
		if json.Unmarshal([]byte(value), &unquoted) == nil {
			return unquoted
		}
	}
	return value
}

func qoderDataRoot(dbPath string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))
}

func (p *QoderProvider) CurrentCredentialsPath() (string, error) {
	return qoderStateDBPath()
}

func (p *QoderProvider) ImportCurrent() (*Account, []byte, error) {
	dbPath, err := qoderStateDBPath()
	if err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	dataRoot := qoderDataRoot(dbPath)
	userInfo, err := qoderReadSecret(db, qoderUserInfoKey, dataRoot)
	if err != nil {
		return nil, nil, err
	}
	userPlan, _ := qoderReadSecret(db, qoderUserPlanKey, dataRoot)
	creditUsage, _ := qoderReadSecret(db, qoderCreditUsageKey, dataRoot)
	if len(userInfo) == 0 && len(creditUsage) == 0 {
		return nil, nil, fmt.Errorf("Qoder account secrets not found in state.vscdb")
	}
	creds := qoderCredentials{UserInfo: userInfo, UserPlan: userPlan, CreditUsage: creditUsage}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	label := ""
	uid := ""
	if len(userInfo) > 0 {
		var info map[string]any
		if json.Unmarshal(userInfo, &info) == nil {
			label = firstNonEmpty(jsonString(info["email"]), jsonString(info["nickname"]), jsonString(info["name"]))
			uid = firstNonEmpty(jsonString(info["uid"]), jsonString(info["id"]), jsonString(info["userId"]))
		}
	}
	remoteKey := uid
	if remoteKey == "" {
		remoteKey = hashKey(string(userInfo) + string(creditUsage))[:24]
	}
	if label == "" {
		label = "Qoder account"
	}
	account := &Account{
		ID:        makeAccountID("qoder", remoteKey),
		Provider:  "qoder",
		Email:     label,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *QoderProvider) RemoteKey(credentials []byte) (string, error) {
	creds, err := parseQoderCredentials(credentials)
	if err != nil {
		return "", err
	}
	var info map[string]any
	if len(creds.UserInfo) > 0 {
		json.Unmarshal(creds.UserInfo, &info)
	}
	if uid := firstNonEmpty(jsonString(info["uid"]), jsonString(info["id"]), jsonString(info["userId"])); uid != "" {
		return uid, nil
	}
	return hashKey(string(creds.UserInfo) + string(creds.CreditUsage))[:24], nil
}

func (p *QoderProvider) ActiveRemoteKey() (string, error) {
	secret, _, err := p.readUserInfo()
	if err != nil {
		return "", err
	}
	var info map[string]any
	json.Unmarshal(secret, &info)
	if uid := firstNonEmpty(jsonString(info["uid"]), jsonString(info["id"]), jsonString(info["userId"])); uid != "" {
		return uid, nil
	}
	return hashKey(string(secret))[:24], nil
}

func (p *QoderProvider) readUserInfo() (json.RawMessage, string, error) {
	dbPath, err := qoderStateDBPath()
	if err != nil {
		return nil, "", err
	}
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return nil, "", err
	}
	defer db.Close()
	userInfo, err := qoderReadSecret(db, qoderUserInfoKey, qoderDataRoot(dbPath))
	if err != nil || len(userInfo) == 0 {
		return nil, "", fmt.Errorf("Qoder user info not found")
	}
	return userInfo, dbPath, nil
}

func (p *QoderProvider) SwitchAccount(account *Account, credentials []byte) error {
	creds, err := parseQoderCredentials(credentials)
	if err != nil {
		return err
	}
	dbPath, err := qoderStateDBPath()
	if err != nil {
		return err
	}
	dataRoot := qoderDataRoot(dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	upsert := func(key string, value json.RawMessage) error {
		if len(value) == 0 {
			return nil
		}
		encrypted, err := encodeCodeBuddySecretValue(string(value), dataRoot)
		if err != nil {
			return err
		}
		_, err = db.Exec("INSERT OR REPLACE INTO ItemTable (key, value) VALUES (?, ?)", key, encrypted)
		return err
	}
	if err := upsert(qoderUserInfoKey, creds.UserInfo); err != nil {
		return err
	}
	if err := upsert(qoderUserPlanKey, creds.UserPlan); err != nil {
		return err
	}
	if err := upsert(qoderCreditUsageKey, creds.CreditUsage); err != nil {
		return err
	}
	return nil
}

func qoderCreditMetrics(usage json.RawMessage, plan json.RawMessage) (*Quota, error) {
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	if len(plan) > 0 {
		var planObj map[string]any
		if json.Unmarshal(plan, &planObj) == nil {
			quota.PlanType = firstNonEmpty(jsonString(planObj["planName"]), jsonString(planObj["plan"]), jsonString(planObj["level"]))
		}
	}
	if len(usage) == 0 {
		return nil, fmt.Errorf("Qoder credit usage is empty")
	}
	var usageObj map[string]any
	if err := json.Unmarshal(usage, &usageObj); err != nil {
		return nil, err
	}
	credits, _ := usageObj["credits"].(map[string]any)
	if credits == nil {
		credits = usageObj
	}
	remainingPercent := -1.0
	if percent, ok := jsonFloat(credits["usagePercent"]); ok {
		remainingPercent = 100 - normalizePercent(percent)
	} else {
		used, usedOK := jsonFloat(credits["used"])
		total, totalOK := jsonFloat(credits["total"])
		if usedOK && totalOK && total > 0 {
			remainingPercent = 100 - normalizePercent((used/total)*100)
		} else if remaining, okRemaining := jsonFloat(credits["remaining"]); okRemaining && totalOK && total > 0 {
			remainingPercent = normalizePercent((remaining / total) * 100)
		}
	}
	if remainingPercent < 0 {
		return nil, fmt.Errorf("Qoder credit usage did not include recognizable fields")
	}
	resetAt := jsonInt64(credits["resetAt"])
	if resetAt == 0 {
		resetAt = jsonInt64(usageObj["resetAt"])
	}
	quota.Metrics = append(quota.Metrics, QuotaMetric{
		Name:             "Credits",
		RemainingPercent: normalizePercent(remainingPercent),
		ResetAt:          resetAt,
	})
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: resetAt}
	if quota.Metrics[0].RemainingPercent <= 0 {
		quota.LimitReached = true
	}
	return quota, nil
}

func (p *QoderProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	creds, err := parseQoderCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	quota, err := qoderCreditMetrics(creds.CreditUsage, creds.UserPlan)
	if err != nil {
		return nil, credentials, err
	}
	return quota, credentials, nil
}
