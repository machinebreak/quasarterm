// Copyright 2026, Orion
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wconfig"
	"github.com/wavetermdev/waveterm/pkg/wps"
)

type Manager struct {
	mu        sync.Mutex
	providers map[string]Provider
}

var managerOnce sync.Once
var managerInstance *Manager

func GetManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{providers: map[string]Provider{}}
		managerInstance.register(&ClaudeProvider{})
		managerInstance.register(&CodexProvider{})
		managerInstance.register(&CopilotProvider{})
		managerInstance.register(&CursorProvider{})
		managerInstance.register(&WindsurfProvider{})
		managerInstance.register(&ZedProvider{})
		managerInstance.register(&ZCodeProvider{})
		managerInstance.register(&KiroProvider{})
		managerInstance.register(newCodeBuddyProvider("codebuddy"))
		managerInstance.register(newCodeBuddyProvider("codebuddy-cn"))
		managerInstance.register(&GrokProvider{})
		managerInstance.register(&QoderProvider{})
		managerInstance.register(newTraeProvider("trae"))
		managerInstance.register(newTraeProvider("trae-solo"))
		managerInstance.register(newTraeProvider("trae-cn"))
		managerInstance.register(newTraeProvider("trae-solo-cn"))
		managerInstance.register(&AntigravityProvider{})
		for idx := range catalogStubInfos {
			managerInstance.register(&stubProvider{info: catalogStubInfos[idx]})
		}
	})
	return managerInstance
}

func (m *Manager) register(provider Provider) {
	m.providers[provider.Info().ID] = provider
}

func (m *Manager) GetProvider(providerID string) (Provider, error) {
	provider, found := m.providers[providerID]
	if !found {
		return nil, fmt.Errorf("unknown account provider %q", providerID)
	}
	return provider, nil
}

func (m *Manager) ListProviders() []ProviderInfo {
	rtn := make([]ProviderInfo, 0, len(m.providers))
	for _, provider := range m.providers {
		info := provider.Info()
		info.Available = true
		if _, isStub := provider.(*stubProvider); isStub {
			info.Available = false
		}
		if _, canWake := provider.(WakeProvider); canWake {
			info.WakeSupported = true
		}
		rtn = append(rtn, info)
	}
	sort.Slice(rtn, func(i, j int) bool {
		if rtn[i].Order != rtn[j].Order {
			return rtn[i].Order < rtn[j].Order
		}
		return rtn[i].ID < rtn[j].ID
	})
	return rtn
}

func (m *Manager) activeRemoteKey(provider Provider) string {
	if keyProvider, ok := provider.(ActiveKeyProvider); ok {
		key, err := keyProvider.ActiveRemoteKey()
		if err != nil {
			return ""
		}
		return key
	}
	path, err := provider.CurrentCredentialsPath()
	if err != nil {
		return ""
	}
	credentials, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	key, err := provider.RemoteKey(credentials)
	if err != nil {
		return ""
	}
	return key
}

func (m *Manager) ListAccounts(providerID string) ([]Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	rtn := make([]Account, 0, len(idx.Accounts))
	activeKeys := map[string]string{}
	for i := range idx.Accounts {
		account := idx.Accounts[i]
		if providerID != "" && account.Provider != providerID {
			continue
		}
		provider, err := m.GetProvider(account.Provider)
		if err != nil {
			continue
		}
		activeKey, cached := activeKeys[account.Provider]
		if !cached {
			activeKey = m.activeRemoteKey(provider)
			activeKeys[account.Provider] = activeKey
		}
		account.IsActive = activeKey != "" && account.RemoteID == activeKey
		rtn = append(rtn, account)
	}
	sort.SliceStable(rtn, func(i, j int) bool {
		if rtn[i].Provider != rtn[j].Provider {
			return rtn[i].Provider < rtn[j].Provider
		}
		return rtn[i].LastUsed > rtn[j].LastUsed
	})
	return rtn, nil
}

func findAccount(idx *indexFile, providerID string, accountID string) *Account {
	for i := range idx.Accounts {
		if idx.Accounts[i].Provider == providerID && idx.Accounts[i].ID == accountID {
			return &idx.Accounts[i]
		}
	}
	return nil
}

func (m *Manager) ImportCurrent(providerID string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}
	account, credentials, err := provider.ImportCurrent()
	if err != nil {
		return nil, err
	}
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	existing := findAccount(idx, providerID, account.ID)
	if existing == nil {
		for i := range idx.Accounts {
			if idx.Accounts[i].Provider == providerID && idx.Accounts[i].RemoteID == account.RemoteID {
				existing = &idx.Accounts[i]
				break
			}
		}
	}
	if existing != nil {
		account.ID = existing.ID
		account.CreatedAt = existing.CreatedAt
		account.Label = existing.Label
		if account.Email != "" {
			existing.Email = account.Email
		}
		if account.PlanType != "" {
			existing.PlanType = account.PlanType
		}
	} else {
		idx.Accounts = append(idx.Accounts, *account)
	}
	if err := saveCredentials(providerID, account.ID, credentials); err != nil {
		return nil, err
	}
	if err := saveIndexLocked(idx); err != nil {
		return nil, err
	}
	publishAccountsUpdate()
	return account, nil
}

func (m *Manager) SwitchAccount(providerID string, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return err
	}
	// Capture the live credentials of the account we are about to move away
	// from (the CLI may have refreshed its tokens) before overwriting the
	// file with the target account's stored copy.
	m.syncActiveCredentialsToStore(provider)
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	account := findAccount(idx, providerID, accountID)
	if account == nil {
		return fmt.Errorf("account not found")
	}
	credentials, err := loadCredentials(providerID, accountID)
	if err != nil {
		return err
	}
	if err := provider.SwitchAccount(account, credentials); err != nil {
		return err
	}
	account.LastUsed = nowMillis()
	if err := saveIndexLocked(idx); err != nil {
		return err
	}
	publishAccountsUpdate()
	return nil
}

func (m *Manager) RefreshQuota(ctx context.Context, providerID string, accountID string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	account := findAccount(idx, providerID, accountID)
	if account == nil {
		return nil, fmt.Errorf("account not found")
	}
	credentials, err := loadCredentials(providerID, accountID)
	if err != nil {
		return nil, err
	}
	if m.activeRemoteKey(provider) == account.RemoteID {
		if path, pathErr := provider.CurrentCredentialsPath(); pathErr == nil {
			if liveCredentials, readErr := os.ReadFile(path); readErr == nil {
				credentials = liveCredentials
			}
		}
	}
	quota, updatedCredentials, err := provider.RefreshQuota(ctx, account, credentials)
	if updatedCredentials != nil && !bytes.Equal(updatedCredentials, credentials) {
		if saveErr := saveCredentials(providerID, accountID, updatedCredentials); saveErr != nil {
			return nil, saveErr
		}
		if m.activeRemoteKey(provider) == account.RemoteID {
			provider.SwitchAccount(account, updatedCredentials)
		}
	}
	if err != nil {
		account.QuotaError = err.Error()
		saveIndexLocked(idx)
		return account, err
	}
	account.Quota = quota
	account.QuotaError = ""
	if quota.PlanType != "" {
		account.PlanType = quota.PlanType
	}
	saveIndexLocked(idx)
	m.maybePublishQuotaAlert(account, quota)
	publishAccountsUpdate()
	return account, nil
}

func (m *Manager) RefreshAllQuotas(ctx context.Context) {
	idx, err := loadIndex()
	if err != nil {
		return
	}
	for i := range idx.Accounts {
		account := idx.Accounts[i]
		m.RefreshQuota(ctx, account.Provider, account.ID)
	}
}

func (m *Manager) DeleteAccount(providerID string, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	found := false
	accounts := idx.Accounts[:0]
	for _, account := range idx.Accounts {
		if account.Provider == providerID && account.ID == accountID {
			found = true
			continue
		}
		accounts = append(accounts, account)
	}
	if !found {
		return fmt.Errorf("account not found")
	}
	idx.Accounts = accounts
	deleteCredentials(providerID, accountID)
	if err := saveIndexLocked(idx); err != nil {
		return err
	}
	publishAccountsUpdate()
	return nil
}

func (m *Manager) SetLabel(providerID string, accountID string, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	account := findAccount(idx, providerID, accountID)
	if account == nil {
		return fmt.Errorf("account not found")
	}
	account.Label = label
	if err := saveIndexLocked(idx); err != nil {
		return err
	}
	publishAccountsUpdate()
	return nil
}

func (m *Manager) SetTags(providerID string, accountID string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	account := findAccount(idx, providerID, accountID)
	if account == nil {
		return fmt.Errorf("account not found")
	}
	account.Tags = normalizeTags(tags)
	if err := saveIndexLocked(idx); err != nil {
		return err
	}
	publishAccountsUpdate()
	return nil
}

func normalizeTags(tags []string) []string {
	rtn := []string{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if len(tag) > 24 {
			tag = tag[:24]
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		rtn = append(rtn, tag)
		if len(rtn) >= 10 {
			break
		}
	}
	return rtn
}

func (m *Manager) SetAutoSwitch(providerID string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	if idx.AutoSwitch == nil {
		idx.AutoSwitch = map[string]bool{}
	}
	idx.AutoSwitch[providerID] = enabled
	return saveIndexLocked(idx)
}

func (m *Manager) AutoSwitchEnabled(providerID string) bool {
	idx, err := loadIndex()
	if err != nil {
		return false
	}
	if value, found := idx.AutoSwitch[providerID]; found {
		return value
	}
	return true
}

func (m *Manager) pickNextAccount(providerID string, excludeAccountID string) (*Account, error) {
	accounts, err := m.ListAccounts(providerID)
	if err != nil {
		return nil, err
	}
	var best *Account
	bestScore := -1.0
	for i := range accounts {
		account := accounts[i]
		if account.ID == excludeAccountID || account.IsActive {
			continue
		}
		if account.Quota != nil && account.Quota.LimitReached {
			continue
		}
		score := 50.0
		if account.Quota != nil && account.Quota.Primary != nil {
			score = account.Quota.Primary.RemainingPercent
		}
		if score <= 5 {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = &account
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no alternative account with available quota")
	}
	return best, nil
}

type exportAccountEntry struct {
	Account
	Credentials json.RawMessage `json:"credentials,omitempty"`
}

type accountsExportFile struct {
	Version  int                  `json:"version"`
	Exported int64                `json:"exported"`
	Accounts []exportAccountEntry `json:"accounts"`
}

func (m *Manager) ExportAccounts() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return "", err
	}
	export := accountsExportFile{Version: 1, Exported: nowMillis()}
	for _, account := range idx.Accounts {
		entry := exportAccountEntry{Account: account}
		if credentials, loadErr := loadCredentials(account.Provider, account.ID); loadErr == nil {
			entry.Credentials = json.RawMessage(credentials)
		}
		export.Accounts = append(export.Accounts, entry)
	}
	output, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (m *Manager) ExportAccount(providerID string, accountID string, includeCredentials bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return "", err
	}
	account := findAccount(idx, providerID, accountID)
	if account == nil {
		return "", fmt.Errorf("account not found")
	}
	entry := exportAccountEntry{Account: *account}
	if includeCredentials {
		if credentials, loadErr := loadCredentials(providerID, accountID); loadErr == nil {
			entry.Credentials = json.RawMessage(credentials)
		}
	}
	export := accountsExportFile{Version: 1, Exported: nowMillis(), Accounts: []exportAccountEntry{entry}}
	output, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (m *Manager) ImportAccounts(data string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var export accountsExportFile
	if err := json.Unmarshal([]byte(data), &export); err != nil {
		return 0, fmt.Errorf("invalid accounts export file: %w", err)
	}
	idx, err := loadIndex()
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, entry := range export.Accounts {
		if _, err := m.GetProvider(entry.Provider); err != nil {
			continue
		}
		if entry.ID == "" || len(entry.Credentials) == 0 || string(entry.Credentials) == "null" {
			continue
		}
		account := entry.Account
		existing := findAccount(idx, account.Provider, account.ID)
		if existing == nil {
			account.IsActive = false
			idx.Accounts = append(idx.Accounts, account)
		} else {
			existing.Label = account.Label
			existing.Email = account.Email
			existing.PlanType = account.PlanType
			existing.RemoteID = account.RemoteID
		}
		if err := saveCredentials(account.Provider, account.ID, entry.Credentials); err != nil {
			continue
		}
		imported++
	}
	if err := saveIndexLocked(idx); err != nil {
		return imported, err
	}
	publishAccountsUpdate()
	return imported, nil
}

var quotaAlertCooldown sync.Map

const quotaAlertDefaultThreshold = 15.0
const quotaAlertCooldownDuration = 30 * time.Minute

func quotaAlertConfig() (bool, float64) {
	enabled := true
	threshold := quotaAlertDefaultThreshold
	settings := wconfig.GetWatcher().GetFullConfig().Settings
	if settings.AccountsQuotaAlert != nil {
		enabled = *settings.AccountsQuotaAlert
	}
	if settings.AccountsQuotaAlertThreshold != nil {
		threshold = *settings.AccountsQuotaAlertThreshold
	}
	return enabled, threshold
}

func (m *Manager) maybePublishQuotaAlert(account *Account, quota *Quota) {
	if quota == nil {
		return
	}
	enabled, threshold := quotaAlertConfig()
	if !enabled {
		return
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent > threshold {
			continue
		}
		key := account.Provider + "|" + account.ID + "|" + metric.Name
		if last, ok := quotaAlertCooldown.Load(key); ok {
			if time.Since(last.(time.Time)) < quotaAlertCooldownDuration {
				continue
			}
		}
		quotaAlertCooldown.Store(key, time.Now())
		wps.Broker.Publish(wps.WaveEvent{
			Event: wps.Event_QuotaAlert,
			Data: QuotaAlertEvent{
				Provider:         account.Provider,
				AccountID:        account.ID,
				Label:            account.Label,
				Metric:           metric.Name,
				RemainingPercent: metric.RemainingPercent,
			},
		})
	}
}

func (m *Manager) StartQuotaMonitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	go func() {
		time.Sleep(30 * time.Second)
		for {
			// Recover per cycle: a panic in one provider refresh must not stop
			// quota monitoring for the rest.
			func() {
				defer func() {
					recover()
				}()
				m.RefreshAllQuotas(ctx)
			}()
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

func (m *Manager) AddExternalAccount(providerID string, account *Account, credentials []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.GetProvider(providerID); err != nil {
		return err
	}
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	existing := findAccount(idx, providerID, account.ID)
	if existing == nil {
		account.IsActive = false
		account.CreatedAt = nowMillis()
		idx.Accounts = append(idx.Accounts, *account)
	} else {
		account.CreatedAt = existing.CreatedAt
		existing.Label = account.Label
		existing.Email = account.Email
		existing.PlanType = account.PlanType
		existing.RemoteID = account.RemoteID
	}
	if err := saveCredentials(providerID, account.ID, credentials); err != nil {
		return err
	}
	if err := saveIndexLocked(idx); err != nil {
		return err
	}
	publishAccountsUpdate()
	return nil
}

func publishAccountsUpdate() {
	go func() {
		idx, err := loadIndex()
		if err != nil {
			return
		}
		wps.Broker.Publish(wps.WaveEvent{
			Event: wps.Event_AccountsUpdate,
			Data:  idx.Accounts,
		})
	}()
}
