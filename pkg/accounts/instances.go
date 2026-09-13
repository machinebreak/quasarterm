// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) instancesRoot() string {
	return filepath.Join(accountsDir(), "instances")
}

func (m *Manager) ListInstances(providerID string) ([]Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	rtn := []Instance{}
	for _, instance := range idx.Instances {
		if providerID != "" && instance.Provider != providerID {
			continue
		}
		rtn = append(rtn, instance)
	}
	return rtn, nil
}

func normalizeInstanceArgs(args []string) []string {
	rtn := []string{}
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		rtn = append(rtn, arg)
	}
	return rtn
}

func (m *Manager) CreateInstance(providerID string, accountID string, name string, dir string, extraArgs []string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}
	instanceProvider, ok := provider.(InstanceProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support multiple instances yet", provider.Info().Name)
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
	instanceID := makeAccountID(providerID, accountID+fmt.Sprintf("|%d", nowMillis()))
	profileDir := filepath.Join(m.instancesRoot(), providerID, instanceID)
	customDir := strings.TrimSpace(dir)
	if customDir != "" {
		profileDir = customDir
	}
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return nil, err
	}
	if _, _, _, err := instanceProvider.InstancePrepare(account, credentials, profileDir); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		count := 0
		for _, existing := range idx.Instances {
			if existing.Provider == providerID && existing.AccountID == accountID {
				count++
			}
		}
		base := account.Label
		if base == "" {
			base = accountID
		}
		name = fmt.Sprintf("%s #%d", base, count+1)
	}
	instance := Instance{
		ID:         instanceID,
		Provider:   providerID,
		AccountID:  accountID,
		Name:       name,
		ProfileDir: profileDir,
		Dir:        customDir,
		Args:       normalizeInstanceArgs(extraArgs),
		Status:     "stopped",
		CreatedAt:  nowMillis(),
	}
	idx.Instances = append(idx.Instances, instance)
	if err := saveIndexLocked(idx); err != nil {
		return nil, err
	}
	publishAccountsUpdate()
	return &instance, nil
}

func (m *Manager) UpdateInstance(instanceID string, name string, dir string, args []string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	var instance *Instance
	for i := range idx.Instances {
		if idx.Instances[i].ID == instanceID {
			instance = &idx.Instances[i]
			break
		}
	}
	if instance == nil {
		return nil, fmt.Errorf("instance not found")
	}
	if strings.TrimSpace(name) != "" {
		instance.Name = strings.TrimSpace(name)
	}
	instance.Args = normalizeInstanceArgs(args)
	customDir := strings.TrimSpace(dir)
	if customDir != instance.Dir {
		provider, err := m.GetProvider(instance.Provider)
		if err != nil {
			return nil, err
		}
		instanceProvider, ok := provider.(InstanceProvider)
		if !ok {
			return nil, fmt.Errorf("%s does not support multiple instances yet", provider.Info().Name)
		}
		account := findAccount(idx, instance.Provider, instance.AccountID)
		if account == nil {
			return nil, fmt.Errorf("account not found for instance")
		}
		credentials, err := loadCredentials(instance.Provider, instance.AccountID)
		if err != nil {
			return nil, err
		}
		targetDir := customDir
		if targetDir == "" {
			targetDir = filepath.Join(m.instancesRoot(), instance.Provider, instance.ID)
		}
		if err := os.MkdirAll(targetDir, 0700); err != nil {
			return nil, err
		}
		if _, _, _, err := instanceProvider.InstancePrepare(account, credentials, targetDir); err != nil {
			return nil, err
		}
		instance.ProfileDir = targetDir
		instance.Dir = customDir
	}
	if err := saveIndexLocked(idx); err != nil {
		return nil, err
	}
	instanceCopy := *instance
	publishAccountsUpdate()
	return &instanceCopy, nil
}

func (m *Manager) GetInstance(instanceID string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	for i := range idx.Instances {
		if idx.Instances[i].ID == instanceID {
			return &idx.Instances[i], nil
		}
	}
	return nil, fmt.Errorf("instance not found")
}

func (m *Manager) InstanceLaunchSpec(instanceID string) (*InstanceLaunchSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	var instance *Instance
	for i := range idx.Instances {
		if idx.Instances[i].ID == instanceID {
			instance = &idx.Instances[i]
			break
		}
	}
	if instance == nil {
		return nil, fmt.Errorf("instance not found")
	}
	provider, err := m.GetProvider(instance.Provider)
	if err != nil {
		return nil, err
	}
	instanceProvider, ok := provider.(InstanceProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support multiple instances yet", provider.Info().Name)
	}
	account := findAccount(idx, instance.Provider, instance.AccountID)
	if account == nil {
		return nil, fmt.Errorf("account not found for instance")
	}
	credentials, err := loadCredentials(instance.Provider, instance.AccountID)
	if err != nil {
		return nil, err
	}
	profileDir := instance.ProfileDir
	if instance.Dir != "" {
		profileDir = instance.Dir
	}
	execPath, args, env, err := instanceProvider.InstancePrepare(account, credentials, profileDir)
	if err != nil {
		return nil, err
	}
	args = append(args, instance.Args...)
	envMap := map[string]string{}
	for _, entry := range env {
		if key, value, found := strings.Cut(entry, "="); found {
			envMap[key] = value
		}
	}
	return &InstanceLaunchSpec{
		Instance: *instance,
		Exec:     execPath,
		Args:     args,
		Env:      envMap,
		Cwd:      profileDir,
		Account:  account,
	}, nil
}

func (m *Manager) MarkInstanceStarted(instanceID string, blockID string, tabID string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	for i := range idx.Instances {
		if idx.Instances[i].ID == instanceID {
			idx.Instances[i].Status = "running"
			idx.Instances[i].BlockID = blockID
			if tabID != "" {
				idx.Instances[i].TabID = tabID
			}
			idx.Instances[i].LastStarted = nowMillis()
			if err := saveIndexLocked(idx); err != nil {
				return nil, err
			}
			instance := idx.Instances[i]
			publishAccountsUpdate()
			return &instance, nil
		}
	}
	return nil, fmt.Errorf("instance not found")
}

func (m *Manager) MarkInstanceStopped(instanceID string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	for i := range idx.Instances {
		if idx.Instances[i].ID == instanceID {
			idx.Instances[i].Status = "stopped"
			idx.Instances[i].BlockID = ""
			idx.Instances[i].PID = 0
			if err := saveIndexLocked(idx); err != nil {
				return nil, err
			}
			instance := idx.Instances[i]
			publishAccountsUpdate()
			return &instance, nil
		}
	}
	return nil, fmt.Errorf("instance not found")
}

func (m *Manager) DeleteInstance(instanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := loadIndex()
	if err != nil {
		return err
	}
	instances := idx.Instances[:0]
	var removed *Instance
	for i := range idx.Instances {
		if idx.Instances[i].ID == instanceID {
			removed = &idx.Instances[i]
			continue
		}
		instances = append(instances, idx.Instances[i])
	}
	if removed == nil {
		return fmt.Errorf("instance not found")
	}
	idx.Instances = instances
	if err := saveIndexLocked(idx); err != nil {
		return err
	}
	if removed.ProfileDir != "" && strings.HasPrefix(removed.ProfileDir, m.instancesRoot()) {
		os.RemoveAll(removed.ProfileDir)
	}
	publishAccountsUpdate()
	return nil
}
