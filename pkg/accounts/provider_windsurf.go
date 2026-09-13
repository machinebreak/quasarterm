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

	_ "github.com/mattn/go-sqlite3"
)

const windsurfDefaultAPIServer = "https://server.codeium.com"
const windsurfAuthItemKey = "codeium.windsurf-windsurf_auth"

type WindsurfProvider struct{}

func (p *WindsurfProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "windsurf",
		Name:        "Windsurf",
		Description: "Windsurf IDE",
		CLICommand:  "windsurf",
		ResumeArgs:  "",
		Icon:        "windsurf",
		Order:       40,
		Available:   true,
	}
}

func windsurfStateDBPaths() []string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}
	candidates := []string{}
	for _, app := range []string{"Windsurf", "Devin"} {
		candidates = append(candidates, filepath.Join(appData, app, "User", "globalStorage", "state.vscdb"))
	}
	return candidates
}

func windsurfStateDBPath() (string, error) {
	for _, candidate := range windsurfStateDBPaths() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Windsurf state.vscdb not found (log in to Windsurf first)")
}

type windsurfAuthStatus struct {
	APIKey       string `json:"apiKey"`
	APIServerURL string `json:"apiServerUrl"`
	Name         string `json:"name"`
	Email        string `json:"email"`
}

func parseWindsurfAuth(raw []byte) (*windsurfAuthStatus, error) {
	var auth windsurfAuthStatus
	if err := json.Unmarshal(raw, &auth); err != nil {
		return nil, fmt.Errorf("invalid Windsurf auth JSON: %w", err)
	}
	if auth.APIKey == "" {
		var generic map[string]any
		if json.Unmarshal(raw, &generic) == nil {
			auth.APIKey = jsonString(generic["api_key"])
		}
	}
	if auth.APIKey == "" {
		return nil, fmt.Errorf("no apiKey found in Windsurf auth")
	}
	if auth.APIServerURL == "" {
		auth.APIServerURL = windsurfDefaultAPIServer
	}
	return &auth, nil
}

func (p *WindsurfProvider) CurrentCredentialsPath() (string, error) {
	return windsurfStateDBPath()
}

func (p *WindsurfProvider) ImportCurrent() (*Account, []byte, error) {
	dbPath, err := windsurfStateDBPath()
	if err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	var raw string
	if err := db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", windsurfAuthItemKey).Scan(&raw); err != nil {
		return nil, nil, fmt.Errorf("Windsurf is not logged in (no auth entry)")
	}
	credentials := []byte(raw)
	auth, err := parseWindsurfAuth(credentials)
	if err != nil {
		return nil, nil, err
	}
	remoteKey := hashKey(auth.APIKey)[:24]
	label := auth.Email
	if label == "" {
		label = auth.Name
	}
	if label == "" {
		label = "Windsurf account"
	}
	account := &Account{
		ID:        makeAccountID("windsurf", remoteKey),
		Provider:  "windsurf",
		Email:     auth.Email,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *WindsurfProvider) RemoteKey(credentials []byte) (string, error) {
	auth, err := parseWindsurfAuth(credentials)
	if err != nil {
		return "", err
	}
	return hashKey(auth.APIKey)[:24], nil
}

func (p *WindsurfProvider) SwitchAccount(account *Account, credentials []byte) error {
	if _, err := parseWindsurfAuth(credentials); err != nil {
		return err
	}
	dbPath, err := windsurfStateDBPath()
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("INSERT OR REPLACE INTO ItemTable (key, value) VALUES (?, ?)", windsurfAuthItemKey, string(credentials))
	return err
}

func protoTimestampSeconds(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case string:
		return parseISO8601(typed)
	case map[string]any:
		if seconds, ok := jsonFloat(typed["seconds"]); ok {
			return int64(seconds)
		}
	}
	return 0
}

func windsurfPickInt(obj map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		if value, ok := jsonFloat(obj[key]); ok {
			return int64(value), true
		}
	}
	return 0, false
}

func (p *WindsurfProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	auth, err := parseWindsurfAuth(credentials)
	if err != nil {
		return nil, nil, err
	}
	apiServer := auth.APIServerURL
	if apiServer == "" {
		apiServer = windsurfDefaultAPIServer
	}
	payload, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"apiKey":         auth.APIKey,
			"ideName":        "Windsurf",
			"extensionName":  "codeium.windsurf",
			"extensionVersion": "1.0.0",
			"locale":         "en-US",
			"os":             "windows",
		},
	})
	url := apiServer + "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, credentials, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, credentials, fmt.Errorf("error requesting Windsurf usage: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, credentials, fmt.Errorf("Windsurf API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, credentials, fmt.Errorf("error parsing Windsurf response: %w", err)
	}
	userStatus, _ := root["userStatus"].(map[string]any)
	planStatus, _ := userStatus["planStatus"].(map[string]any)
	planInfo, _ := userStatus["planInfo"].(map[string]any)
	if planStatus == nil {
		if ps, ok := root["planStatus"].(map[string]any); ok {
			planStatus = ps
		}
	}
	if planStatus == nil {
		return nil, credentials, fmt.Errorf("Windsurf response did not include planStatus")
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	if planInfo != nil {
		if name := jsonString(planInfo["planName"]); name != "" {
			quota.PlanType = name
		}
	}
	resetAt := protoTimestampSeconds(planStatus["planEnd"])
	addMetric := func(name string, availableKey string, usedKey string) {
		available, okA := windsurfPickInt(planStatus, availableKey)
		used, okU := windsurfPickInt(planStatus, usedKey)
		if !okA && !okU {
			return
		}
		total := available + used
		remaining := 100.0
		if total > 0 {
			remaining = (float64(available) / float64(total)) * 100
		}
		quota.Metrics = append(quota.Metrics, QuotaMetric{Name: name, RemainingPercent: remaining, ResetAt: resetAt})
	}
	addMetric("Prompt Credits", "availablePromptCredits", "usedPromptCredits")
	addMetric("Flow Credits", "availableFlowCredits", "usedFlowCredits")
	if len(quota.Metrics) == 0 {
		return nil, credentials, fmt.Errorf("Windsurf response did not include credit data")
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: resetAt}
	return quota, credentials, nil
}
