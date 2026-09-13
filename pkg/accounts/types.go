// Copyright 2026, Orion
// SPDX-License-Identifier: Apache-2.0

package accounts

type ProviderID string

const (
	ProviderCodex   ProviderID = "codex"
	ProviderClaude  ProviderID = "claude"
	ProviderCopilot ProviderID = "github-copilot"
)

type ProviderInfo struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	CLICommand    string  `json:"clicommand"`
	ResumeArgs    string  `json:"resumeargs"`
	Icon          string  `json:"icon"`
	Order         float64 `json:"order"`
	Available     bool    `json:"available"`
	WakeSupported bool    `json:"wakesupported,omitempty"`
}

type QuotaMetric struct {
	Name             string  `json:"name"`
	RemainingPercent float64 `json:"remainingpercent"`
	ResetAt          int64   `json:"resetat,omitempty"`
	ResetText        string  `json:"resettext,omitempty"`
}

type QuotaWindow struct {
	RemainingPercent float64 `json:"remainingpercent"`
	ResetAt          int64   `json:"resetat,omitempty"`
	WindowMinutes    int64   `json:"windowminutes,omitempty"`
}

type Quota struct {
	UpdatedAt    int64         `json:"updatedat"`
	PlanType     string        `json:"plantype,omitempty"`
	Allowed      bool          `json:"allowed"`
	LimitReached bool          `json:"limitreached"`
	ResetCredits int           `json:"resetcredits,omitempty"`
	Primary      *QuotaWindow  `json:"primary,omitempty"`
	Secondary    *QuotaWindow  `json:"secondary,omitempty"`
	Metrics      []QuotaMetric `json:"metrics,omitempty"`
}

type Account struct {
	ID            string   `json:"id"`
	Provider      string   `json:"provider"`
	Label         string   `json:"label,omitempty"`
	Email         string   `json:"email,omitempty"`
	PlanType      string   `json:"plantype,omitempty"`
	RemoteID      string   `json:"remoteid,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	CreatedAt     int64    `json:"createdat"`
	LastUsed      int64    `json:"lastused,omitempty"`
	IsActive      bool     `json:"isactive"`
	Quota         *Quota   `json:"quota,omitempty"`
	QuotaError    string   `json:"quotaerror,omitempty"`
	CredentialsID string   `json:"-"`
}

type indexFile struct {
	Version    int             `json:"version"`
	Accounts   []Account       `json:"accounts"`
	AutoSwitch map[string]bool `json:"autoswitch,omitempty"`
	Instances  []Instance      `json:"instances,omitempty"`
}

type Instance struct {
	ID          string   `json:"id"`
	Provider    string   `json:"provider"`
	AccountID   string   `json:"accountid"`
	Name        string   `json:"name"`
	ProfileDir  string   `json:"profiledir"`
	Dir         string   `json:"dir,omitempty"`
	Args        []string `json:"args,omitempty"`
	TabID       string   `json:"tabid,omitempty"`
	BlockID     string   `json:"blockid,omitempty"`
	PID         int      `json:"pid,omitempty"`
	Status      string   `json:"status"`
	CreatedAt   int64    `json:"createdat"`
	LastStarted int64    `json:"laststarted,omitempty"`
}

type InstanceLaunchSpec struct {
	Instance Instance          `json:"instance"`
	Exec     string            `json:"exec"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Cwd      string            `json:"cwd,omitempty"`
	Account  *Account          `json:"account,omitempty"`
}

type AutoSwitchSettings struct {
	Enabled map[string]bool `json:"enabled"`
}

type QuotaAlertEvent struct {
	Provider         string  `json:"provider"`
	AccountID        string  `json:"accountid"`
	Label            string  `json:"label,omitempty"`
	Metric           string  `json:"metric"`
	RemainingPercent float64 `json:"remainingpercent"`
}
