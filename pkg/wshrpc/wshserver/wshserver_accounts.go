// Copyright 2026, Orion
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"
	"time"

	"github.com/wavetermdev/waveterm/pkg/accounts"
	"github.com/wavetermdev/waveterm/pkg/blockcontroller"
	"github.com/wavetermdev/waveterm/pkg/secretstore"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

type accountSecretBackend struct{}

func (accountSecretBackend) Set(name string, value string) error {
	return secretstore.SetSecret(name, value)
}

func (accountSecretBackend) Get(name string) (string, bool, error) {
	return secretstore.GetSecret(name)
}

func (accountSecretBackend) Delete(name string) error {
	return secretstore.DeleteSecret(name)
}

func init() {
	accounts.SetSecretBackend(accountSecretBackend{})
}

func (ws *WshServer) ListAccountProvidersCommand(ctx context.Context) ([]accounts.ProviderInfo, error) {
	return accounts.GetManager().ListProviders(), nil
}

func (ws *WshServer) ListAccountsCommand(ctx context.Context, data wshrpc.CommandListAccountsData) ([]accounts.Account, error) {
	return accounts.GetManager().ListAccounts(data.Provider)
}

func (ws *WshServer) ImportCurrentAccountCommand(ctx context.Context, provider string) (*accounts.Account, error) {
	return accounts.GetManager().ImportCurrent(provider)
}

func (ws *WshServer) SwitchAccountCommand(ctx context.Context, data wshrpc.CommandSwitchAccountData) error {
	return accounts.GetManager().SwitchAccount(data.Provider, data.AccountID)
}

func (ws *WshServer) SwitchAccountAndResumeCommand(ctx context.Context, data wshrpc.CommandSwitchAccountData) error {
	manager := accounts.GetManager()
	provider, err := manager.GetProvider(data.Provider)
	if err != nil {
		return err
	}
	if err := manager.SwitchAccount(data.Provider, data.AccountID); err != nil {
		return err
	}
	if data.BlockID == "" {
		return nil
	}
	resumeCommand := provider.Info().ResumeArgs
	if resumeCommand == "" {
		return nil
	}
	go sendResumeSequence(data.BlockID, resumeCommand)
	return nil
}

func sendResumeSequence(blockId string, resumeCommand string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("error sending resume sequence: %v\n", r)
		}
	}()
	send := func(data []byte) {
		blockcontroller.SendInput(blockId, &blockcontroller.BlockInputUnion{InputData: data})
	}
	time.Sleep(200 * time.Millisecond)
	send([]byte{3})
	time.Sleep(400 * time.Millisecond)
	send([]byte{3})
	time.Sleep(900 * time.Millisecond)
	send([]byte(resumeCommand + "\r"))
	time.Sleep(1500 * time.Millisecond)
	send([]byte("\r"))
}

func (ws *WshServer) RefreshAccountQuotaCommand(ctx context.Context, data wshrpc.CommandRefreshAccountQuotaData) (*accounts.Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return accounts.GetManager().RefreshQuota(ctx, data.Provider, data.AccountID)
}

func (ws *WshServer) DeleteAccountCommand(ctx context.Context, data wshrpc.CommandDeleteAccountData) error {
	return accounts.GetManager().DeleteAccount(data.Provider, data.AccountID)
}

func (ws *WshServer) SetAccountLabelCommand(ctx context.Context, data wshrpc.CommandSetAccountLabelData) error {
	return accounts.GetManager().SetLabel(data.Provider, data.AccountID, data.Label)
}

func (ws *WshServer) SetAccountTagsCommand(ctx context.Context, data wshrpc.CommandSetAccountTagsData) error {
	return accounts.GetManager().SetTags(data.Provider, data.AccountID, data.Tags)
}

func (ws *WshServer) ExportAccountCommand(ctx context.Context, data wshrpc.CommandExportAccountData) (string, error) {
	return accounts.GetManager().ExportAccount(data.Provider, data.AccountID, data.IncludeSecrets)
}

func (ws *WshServer) GetAccountAutoSwitchCommand(ctx context.Context, provider string) (bool, error) {
	return accounts.GetManager().AutoSwitchEnabled(provider), nil
}

func (ws *WshServer) SetAccountAutoSwitchCommand(ctx context.Context, data wshrpc.CommandSetAccountAutoSwitchData) error {
	return accounts.GetManager().SetAutoSwitch(data.Provider, data.Enabled)
}

func (ws *WshServer) ExportAccountsCommand(ctx context.Context) (string, error) {
	return accounts.GetManager().ExportAccounts()
}

func (ws *WshServer) ImportAccountsCommand(ctx context.Context, data string) (int, error) {
	return accounts.GetManager().ImportAccounts(data)
}

func (ws *WshServer) StartAccountLoginCommand(ctx context.Context, provider string) (*accounts.LoginStart, error) {
	return accounts.StartBrowserLogin(ctx, provider)
}

func (ws *WshServer) PollAccountLoginCommand(ctx context.Context, sessionID string) (*accounts.Account, error) {
	return accounts.PollBrowserLogin(ctx, sessionID)
}

func (ws *WshServer) SubmitAccountLoginCodeCommand(ctx context.Context, data wshrpc.CommandSubmitLoginCodeData) (*accounts.Account, error) {
	return accounts.SubmitBrowserLoginCode(ctx, data.SessionID, data.Code)
}

func (ws *WshServer) ListAccountInstancesCommand(ctx context.Context, provider string) ([]accounts.Instance, error) {
	return accounts.GetManager().ListInstances(provider)
}

func (ws *WshServer) CreateAccountInstanceCommand(ctx context.Context, data wshrpc.CommandCreateInstanceData) (*accounts.Instance, error) {
	return accounts.GetManager().CreateInstance(data.Provider, data.AccountID, data.Name, data.Dir, data.Args)
}

func (ws *WshServer) UpdateAccountInstanceCommand(ctx context.Context, data wshrpc.CommandUpdateInstanceData) (*accounts.Instance, error) {
	return accounts.GetManager().UpdateInstance(data.InstanceID, data.Name, data.Dir, data.Args)
}

func (ws *WshServer) DeleteAccountInstanceCommand(ctx context.Context, instanceID string) error {
	instance, err := accounts.GetManager().GetInstance(instanceID)
	if err != nil {
		return err
	}
	if instance.BlockID != "" {
		wcore.DeleteBlock(ctx, instance.BlockID, false)
	}
	return accounts.GetManager().DeleteInstance(instanceID)
}

func (ws *WshServer) StartAccountInstanceCommand(ctx context.Context, data wshrpc.CommandStartInstanceData) (*accounts.Instance, error) {
	spec, err := accounts.GetManager().InstanceLaunchSpec(data.InstanceID)
	if err != nil {
		return nil, err
	}
	meta := waveobj.MetaMapType{
		"view":       "term",
		"controller": "cmd",
		"cmd":        spec.Exec,
		"cmd:cwd":    spec.Cwd,
	}
	if len(spec.Args) > 0 {
		meta["cmd:args"] = spec.Args
	}
	if len(spec.Env) > 0 {
		meta["cmd:env"] = spec.Env
	}
	blockDef := &waveobj.BlockDef{Meta: meta}
	block, err := wcore.CreateBlock(ctx, data.TabID, blockDef, nil)
	if err != nil {
		return nil, err
	}
	return accounts.GetManager().MarkInstanceStarted(data.InstanceID, block.OID, data.TabID)
}

func (ws *WshServer) StopAccountInstanceCommand(ctx context.Context, instanceID string) (*accounts.Instance, error) {
	instance, err := accounts.GetManager().GetInstance(instanceID)
	if err != nil {
		return nil, err
	}
	if instance.BlockID != "" {
		wcore.DeleteBlock(ctx, instance.BlockID, false)
	}
	return accounts.GetManager().MarkInstanceStopped(instanceID)
}

func (ws *WshServer) FocusAccountInstanceCommand(ctx context.Context, instanceID string) error {
	instance, err := accounts.GetManager().GetInstance(instanceID)
	if err != nil {
		return err
	}
	if instance.BlockID == "" {
		return fmt.Errorf("instance is not running")
	}
	return nil
}

func (ws *WshServer) ListWakeTasksCommand(ctx context.Context, provider string) ([]accounts.WakeTask, error) {
	return accounts.GetManager().ListWakeTasks(provider)
}

func (ws *WshServer) CreateWakeTaskCommand(ctx context.Context, data wshrpc.CommandCreateWakeTaskData) (*accounts.WakeTask, error) {
	return accounts.GetManager().CreateWakeTask(data.Provider, data.AccountID, data.IntervalMinutes, data.Model)
}

func (ws *WshServer) UpdateWakeTaskCommand(ctx context.Context, data wshrpc.CommandUpdateWakeTaskData) (*accounts.WakeTask, error) {
	return accounts.GetManager().UpdateWakeTask(data.TaskID, data.Enabled, data.IntervalMinutes, data.Model)
}

func (ws *WshServer) DeleteWakeTaskCommand(ctx context.Context, taskID string) error {
	return accounts.GetManager().DeleteWakeTask(taskID)
}

func (ws *WshServer) RunWakeTaskCommand(ctx context.Context, taskID string) (*accounts.WakeRun, error) {
	return accounts.GetManager().RunWakeTask(taskID)
}

func (ws *WshServer) ListWakeRunsCommand(ctx context.Context, data wshrpc.CommandListWakeRunsData) ([]accounts.WakeRun, error) {
	return accounts.GetManager().ListWakeRuns(data.TaskID, data.Limit)
}

func (ws *WshServer) GetCodexApiStatusCommand(ctx context.Context) (accounts.LocalAPIStatus, error) {
	return accounts.GetManager().LocalAPIStatus(), nil
}

func (ws *WshServer) SetCodexApiSettingsCommand(ctx context.Context, data wshrpc.CommandSetCodexApiData) (accounts.LocalAPIStatus, error) {
	return accounts.GetManager().ApplyLocalAPISettings(data.Enabled, data.Port, data.APIKey)
}
