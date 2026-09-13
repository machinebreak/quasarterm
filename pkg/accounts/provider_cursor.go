// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	cursorUsageURL = "https://cursor.com/api/usage-summary"
)

type CursorProvider struct{}

func (p *CursorProvider) Info() ProviderInfo {
	return ProviderInfo{
		ID:          "cursor",
		Name:        "Cursor",
		Description: "Cursor editor",
		CLICommand:  "cursor",
		ResumeArgs:  "",
		Icon:        "cursor",
		Order:       60,
		Available:   true,
	}
}

func cursorStateDBPath() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", fmt.Errorf("APPDATA is not set")
	}
	return filepath.Join(appData, "Cursor", "User", "globalStorage", "state.vscdb"), nil
}

func cursorReadItem(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

type cursorCredentials struct {
	AccessToken    string `json:"accessToken"`
	RefreshToken   string `json:"refreshToken,omitempty"`
	Email          string `json:"email,omitempty"`
	MembershipType string `json:"membershipType,omitempty"`
}

func parseCursorCredentials(credentials []byte) (*cursorCredentials, error) {
	var creds cursorCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid Cursor credentials: %w", err)
	}
	if creds.AccessToken == "" {
		return nil, fmt.Errorf("Cursor credentials have an empty accessToken")
	}
	return &creds, nil
}

func (p *CursorProvider) CurrentCredentialsPath() (string, error) {
	path, err := cursorStateDBPath()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return "", fmt.Errorf("Cursor state.vscdb not found at %s", path)
	}
	return path, nil
}

func (p *CursorProvider) ImportCurrent() (*Account, []byte, error) {
	path, err := p.CurrentCredentialsPath()
	if err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("sqlite3", path+"?mode=ro")
	if err != nil {
		return nil, nil, fmt.Errorf("error opening Cursor state: %w", err)
	}
	defer db.Close()
	accessToken, err := cursorReadItem(db, "cursorAuth/accessToken")
	if err != nil || accessToken == "" {
		return nil, nil, fmt.Errorf("Cursor is not logged in (no accessToken in state.vscdb)")
	}
	email, _ := cursorReadItem(db, "cursorAuth/cachedEmail")
	refreshToken, _ := cursorReadItem(db, "cursorAuth/refreshToken")
	membership, _ := cursorReadItem(db, "cursorAuth/stripeMembershipType")
	creds := cursorCredentials{
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		Email:          email,
		MembershipType: membership,
	}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	remoteKey := hashKey(accessToken)[:24]
	label := email
	if label == "" {
		label = "Cursor account"
	}
	account := &Account{
		ID:        makeAccountID("cursor", remoteKey),
		Provider:  "cursor",
		Email:     email,
		PlanType:  membership,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *CursorProvider) RemoteKey(credentials []byte) (string, error) {
	creds, err := parseCursorCredentials(credentials)
	if err != nil {
		return "", err
	}
	return hashKey(creds.AccessToken)[:24], nil
}

func (p *CursorProvider) SwitchAccount(account *Account, credentials []byte) error {
	creds, err := parseCursorCredentials(credentials)
	if err != nil {
		return err
	}
	path, err := cursorStateDBPath()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return fmt.Errorf("Cursor state.vscdb not found at %s", path)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("error opening Cursor state: %w", err)
	}
	defer db.Close()
	upsert := func(key string, value string) error {
		if value == "" {
			return nil
		}
		_, err := db.Exec("INSERT OR REPLACE INTO ItemTable (key, value) VALUES (?, ?)", key, value)
		return err
	}
	if err := upsert("cursorAuth/accessToken", creds.AccessToken); err != nil {
		return err
	}
	if err := upsert("cursorAuth/refreshToken", creds.RefreshToken); err != nil {
		return err
	}
	if err := upsert("cursorAuth/cachedEmail", creds.Email); err != nil {
		return err
	}
	if err := upsert("cursorAuth/stripeMembershipType", creds.MembershipType); err != nil {
		return err
	}
	if err := upsert("cursor.accessToken", creds.AccessToken); err != nil {
		return err
	}
	return nil
}

func cursorWorkOSUserID(accessToken string) string {
	claims := decodeJWTPayload(accessToken)
	if claims == nil {
		return ""
	}
	sub := jsonString(claims["sub"])
	if idx := strings.LastIndex(sub, "|"); idx >= 0 {
		return sub[idx+1:]
	}
	return sub
}

type cursorPlanUsage struct {
	TotalPercentUsed *float64 `json:"totalPercentUsed"`
	AutoPercentUsed  *float64 `json:"autoPercentUsed"`
	APIPercentUsed   *float64 `json:"apiPercentUsed"`
	Used             *float64 `json:"used"`
	Limit            *float64 `json:"limit"`
}

type cursorUsageSummary struct {
	MembershipType  string           `json:"membershipType"`
	IndividualUsage *cursorPlanUsage `json:"individualUsage"`
	PlanUsage       *cursorPlanUsage `json:"planUsage"`
}

func cursorUsagePercent(usage *cursorPlanUsage) float64 {
	if usage == nil {
		return -1
	}
	if usage.TotalPercentUsed != nil {
		return normalizePercent(*usage.TotalPercentUsed)
	}
	if usage.Used != nil && usage.Limit != nil && *usage.Limit > 0 {
		return normalizePercent((*usage.Used / *usage.Limit) * 100)
	}
	return -1
}

func (p *CursorProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	creds, err := parseCursorCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	userID := cursorWorkOSUserID(creds.AccessToken)
	if userID == "" {
		return nil, credentials, fmt.Errorf("could not parse WorkOS user id from Cursor access token")
	}
	req, err := newRequest(ctx, http.MethodGet, cursorUsageURL)
	if err != nil {
		return nil, credentials, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	req.Header.Set("Cookie", fmt.Sprintf("WorkosCursorSessionToken=%s%%3A%%3A%s", userID, creds.AccessToken))
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, credentials, fmt.Errorf("error requesting Cursor usage: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, credentials, fmt.Errorf("Cursor session expired. Re-import the account.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, credentials, fmt.Errorf("Cursor usage API returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var summary cursorUsageSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		return nil, credentials, fmt.Errorf("error parsing Cursor usage: %w", err)
	}
	quota := &Quota{UpdatedAt: nowMillis(), PlanType: summary.MembershipType, Allowed: true}
	plan := summary.IndividualUsage
	if plan == nil {
		plan = summary.PlanUsage
	}
	totalUsed := cursorUsagePercent(plan)
	if totalUsed >= 0 {
		quota.Metrics = append(quota.Metrics, QuotaMetric{Name: "Total Usage", RemainingPercent: 100 - totalUsed})
	}
	if plan != nil && plan.AutoPercentUsed != nil {
		quota.Metrics = append(quota.Metrics, QuotaMetric{Name: "Auto + Composer", RemainingPercent: 100 - normalizePercent(*plan.AutoPercentUsed)})
	}
	if plan != nil && plan.APIPercentUsed != nil {
		quota.Metrics = append(quota.Metrics, QuotaMetric{Name: "API Usage", RemainingPercent: 100 - normalizePercent(*plan.APIPercentUsed)})
	}
	if len(quota.Metrics) == 0 {
		return nil, credentials, fmt.Errorf("Cursor usage response did not include usage data")
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent}
	return quota, credentials, nil
}
