// Copyright 2026, Orion
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wps"
)

var quotaExhaustedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(you've|you have) hit your usage limit`),
	regexp.MustCompile(`(?i)usage limit (reached|exceeded)`),
	regexp.MustCompile(`(?i)you've reached your (usage )?limit`),
	regexp.MustCompile(`(?i)(5-hour|weekly|hourly) limit reached`),
	regexp.MustCompile(`(?i)rate limit reached`),
	regexp.MustCompile(`(?i)insufficient[_ ]quota`),
	regexp.MustCompile(`(?i)quota exceeded`),
	regexp.MustCompile(`(?i)429 too many requests`),
	regexp.MustCompile(`(?i)usage limit for this (account|plan)`),
}

func DetectQuotaExhaustedOutput(output string) bool {
	for _, pattern := range quotaExhaustedPatterns {
		if pattern.MatchString(output) {
			return true
		}
	}
	return false
}

var lastTriggerByBlock sync.Map

const autoSwitchCooldown = 90 * time.Second

type SwitchEvent struct {
	Provider      string `json:"provider"`
	FromAccountID string `json:"fromaccountid,omitempty"`
	FromLabel     string `json:"fromlabel,omitempty"`
	ToAccountID   string `json:"toaccountid"`
	ToLabel       string `json:"tolabel,omitempty"`
	BlockID       string `json:"blockid,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

func (m *Manager) MaybeAutoSwitch(providerID string, blockID string, sendInput func([]byte)) (*Account, bool) {
	if !m.AutoSwitchEnabled(providerID) {
		return nil, false
	}
	now := time.Now()
	if last, ok := lastTriggerByBlock.Load(blockID); ok {
		if now.Sub(last.(time.Time)) < autoSwitchCooldown {
			return nil, false
		}
	}
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, false
	}
	accounts, err := m.ListAccounts(providerID)
	if err != nil {
		return nil, false
	}
	var current *Account
	for i := range accounts {
		if accounts[i].IsActive {
			current = &accounts[i]
			break
		}
	}
	if current == nil {
		return nil, false
	}
	next, err := m.pickNextAccount(providerID, current.ID)
	if err != nil {
		return nil, false
	}
	if err := m.SwitchAccount(providerID, next.ID); err != nil {
		return nil, false
	}
	lastTriggerByBlock.Store(blockID, now)
	if sendInput != nil && provider.Info().ResumeArgs != "" {
		resumeCommand := provider.Info().ResumeArgs
		go func() {
			time.Sleep(200 * time.Millisecond)
			sendInput([]byte{3})
			time.Sleep(400 * time.Millisecond)
			sendInput([]byte{3})
			time.Sleep(900 * time.Millisecond)
			sendInput([]byte(resumeCommand + "\r"))
			time.Sleep(1500 * time.Millisecond)
			sendInput([]byte("\r"))
		}()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		m.RefreshQuota(ctx, providerID, next.ID)
	}()
	wps.Broker.Publish(wps.WaveEvent{
		Event: wps.Event_AccountSwitch,
		Data: SwitchEvent{
			Provider:      providerID,
			FromAccountID: current.ID,
			FromLabel:     current.Label,
			ToAccountID:   next.ID,
			ToLabel:       next.Label,
			BlockID:       blockID,
			Reason:        "quota-exhausted",
		},
	})
	return next, true
}
