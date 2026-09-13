// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { BlockNodeModel } from "@/app/block/blocktypes";
import {
    atoms,
    getAllBlockComponentModels,
    getApi,
    getBlockComponentModel,
    getFocusedBlockId,
    getSettingsKeyAtom,
    globalStore,
    refocusNode,
    WOS,
} from "@/app/store/global";
import type { TabModel } from "@/app/store/tab-model";
import { waveEventSubscribeSingle } from "@/app/store/wps";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { WaveEnv } from "@/app/waveenv/waveenv";
import { atom, PrimitiveAtom, useAtomValue } from "jotai";
import { memo, useEffect, useState } from "react";
import { ProviderIcon } from "./providericons";
import { accountMatchesFilter, mergeAccountExports, providerTags, sanitizeFilename } from "./accounts-util";

function formatReset(resetAt: number, resetText?: string): string {
    if (resetText) {
        return resetText;
    }
    if (!resetAt) {
        return "";
    }
    const diffMs = resetAt * 1000 - Date.now();
    if (diffMs <= 0) {
        return "resetting";
    }
    const minutes = Math.floor(diffMs / 60000);
    if (minutes < 60) return `resets in ${minutes}m`;
    const hours = Math.floor(minutes / 60);
    if (hours < 48) return `resets in ${hours}h ${minutes % 60}m`;
    return `resets in ${Math.floor(hours / 24)}d`;
}

function formatRelative(ts: number): string {
    if (!ts) {
        return "never";
    }
    const minutes = Math.floor((Date.now() - ts) / 60000);
    if (minutes < 1) return "just now";
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
}

function quotaColor(remaining: number): string {
    if (remaining <= 15) return "#ef4444";
    if (remaining <= 40) return "#f59e0b";
    return "#22c55e";
}

const MetricBar = memo(({ metric }: { metric: QuotaMetric }) => {
    const percent = Math.max(0, Math.min(100, metric.remainingpercent));
    const color = quotaColor(percent);
    return (
        <div className="flex flex-col gap-1 w-full">
            <div className="flex flex-row items-center justify-between text-[11px]">
                <span className="text-secondary">{metric.name}</span>
                <span style={{ color }} className="font-semibold">
                    {percent.toFixed(0)}%
                </span>
            </div>
            <div className="w-full h-[3px] rounded bg-hover overflow-hidden">
                <div className="h-full rounded" style={{ width: `${percent}%`, background: color }} />
            </div>
            <span className="text-[10px] text-muted text-right">{formatReset(metric.resetat, metric.resettext)}</span>
        </div>
    );
});
MetricBar.displayName = "MetricBar";

function accountMetrics(account?: Account): QuotaMetric[] {
    if (account?.quota == null) {
        return [];
    }
    if (account.quota.metrics?.length) {
        return account.quota.metrics;
    }
    const metrics: QuotaMetric[] = [];
    if (account.quota.primary) {
        metrics.push({
            name: "Session",
            remainingpercent: account.quota.primary.remainingpercent,
            resetat: account.quota.primary.resetat,
        } as QuotaMetric);
    }
    if (account.quota.secondary) {
        metrics.push({
            name: "Weekly",
            remainingpercent: account.quota.secondary.remainingpercent,
            resetat: account.quota.secondary.resetat,
        } as QuotaMetric);
    }
    return metrics;
}

function accountScore(account?: Account): number {
    const metrics = accountMetrics(account);
    if (metrics.length === 0) {
        return 50;
    }
    return Math.min(...metrics.map((m) => m.remainingpercent));
}

const AccountMiniCard = memo(({ account, title }: { account: Account; title: string }) => {
    const metrics = accountMetrics(account);
    return (
        <div className="flex flex-col gap-3 p-3 rounded-lg border border-border bg-panel/60 min-w-0">
            <div className="flex flex-row items-center gap-2 min-w-0">
                <i
                    className={`fa ${title === "Current account" ? "fa-circle-dot text-accent" : "fa-wand-magic-sparkles text-secondary"} text-[11px]`}
                />
                <span className="text-[11px] text-secondary">{title}</span>
            </div>
            <div className="flex flex-row items-center justify-between gap-2 min-w-0">
                <span className="text-[12px] text-primary font-medium truncate">
                    {account.label || account.email || account.id}
                </span>
                {account.plantype && (
                    <span className="text-[9px] px-1.5 py-0.5 rounded bg-hover text-secondary uppercase shrink-0">
                        {account.plantype}
                    </span>
                )}
            </div>
            {metrics.length > 0 ? (
                <div className="flex flex-col gap-2">
                    {metrics.map((metric) => (
                        <MetricBar key={metric.name} metric={metric} />
                    ))}
                </div>
            ) : (
                <span className="text-[11px] text-muted">no quota data</span>
            )}
        </div>
    );
});
AccountMiniCard.displayName = "AccountMiniCard";

const ProviderPanel = memo(
    ({ model, provider, accounts }: { model: AccountsViewModel; provider: ProviderInfo; accounts: Account[] }) => {
        const current = accounts.find((a) => a.isactive) ?? accounts[0];
        const alternatives = accounts.filter((a) => a.id !== current?.id);
        const recommended = alternatives.sort((a, b) => accountScore(b) - accountScore(a))[0] ?? current;
        return (
            <div className="flex flex-col rounded-xl border border-border bg-panel/30 overflow-hidden">
                <div className="flex flex-row items-center justify-between px-4 py-3 border-b border-border">
                    <div className="flex flex-row items-center gap-2">
                        <ProviderIcon provider={provider} size={18} />
                        <span className="text-[13px] font-medium text-primary">{provider.name}</span>
                        <span className="text-[10px] text-muted">{accounts.length}</span>
                    </div>
                    <button
                        className="px-2 py-1 text-[11px] rounded bg-hover hover:bg-hover/70 text-secondary"
                        onClick={() => model.refreshProvider(provider.id)}
                    >
                        <i className="fa fa-arrows-rotate mr-1" />
                        Refresh
                    </button>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-2 p-3">
                    {current && <AccountMiniCard account={current} title="Current account" />}
                    {recommended && recommended.id !== current?.id && (
                        <AccountMiniCard account={recommended} title="Recommended" />
                    )}
                </div>
                <button
                    className="mx-3 mb-3 py-1.5 text-[12px] rounded-lg bg-hover hover:bg-hover/70 text-primary"
                    onClick={() => model.openProvider(provider.id)}
                >
                    View all accounts
                </button>
            </div>
        );
    }
);
ProviderPanel.displayName = "ProviderPanel";

export class AccountsViewModel implements ViewModel {
    viewType = "accounts";
    viewIcon = atom("table-columns");
    viewName = atom("Accounts");
    noPadding = atom(true);
    blockId: string;
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    waveEnv: WaveEnv;

    providersAtom = atom<ProviderInfo[]>([]);
    accountsByProviderAtom = atom<Record<string, Account[]>>({});
    pageAtom: PrimitiveAtom<string> = atom("dashboard");
    selectedProviderAtom: PrimitiveAtom<string> = atom("codex");
    loadingAtom: PrimitiveAtom<boolean> = atom(false);
    errorAtom: PrimitiveAtom<string> = atom("");
    autoSwitchAtom: PrimitiveAtom<boolean> = atom(true);
    lastSwitchAtom: PrimitiveAtom<SwitchEvent> = atom<SwitchEvent>(null as SwitchEvent);
    targetBlockAtom: PrimitiveAtom<string> = atom("");
    loginStartAtom: PrimitiveAtom<LoginStart> = atom<LoginStart>(null as LoginStart);
    loginErrorAtom: PrimitiveAtom<string> = atom("");
    loginCodeAtom: PrimitiveAtom<string> = atom("");
    instancesAtom = atom<Instance[]>([]);
    searchAtom: PrimitiveAtom<string> = atom("");
    tagFilterAtom = atom<string[]>([]);
    selectedAtom = atom<string[]>([]);
    wakeTasksAtom = atom<WakeTask[]>([]);
    wakeRunsAtom = atom<Record<string, WakeRun[]>>({});
    wakeBusyAtom: PrimitiveAtom<boolean> = atom(false);
    codexApiAtom = atom<LocalAPIStatus>(null as LocalAPIStatus);
    codexApiErrorAtom: PrimitiveAtom<string> = atom("");

    private unsubs: (() => void)[] = [];
    private loginTimer: NodeJS.Timeout = null;

    constructor(initOpts: ViewModelInitType) {
        this.blockId = initOpts.blockId;
        this.nodeModel = initOpts.nodeModel;
        this.tabModel = initOpts.tabModel;
        this.waveEnv = initOpts.waveEnv;
        this.unsubs.push(
            waveEventSubscribeSingle({
                eventType: "accounts:update",
                handler: () => {
                    this.refresh();
                    if (globalStore.get(this.pageAtom) === "provider") {
                        this.loadWakeTasks(globalStore.get(this.selectedProviderAtom));
                    }
                },
            })
        );
        this.unsubs.push(
            waveEventSubscribeSingle({
                eventType: "accounts:switched",
                handler: (event) => {
                    if (event.data) {
                        globalStore.set(this.lastSwitchAtom, event.data);
                        this.refresh();
                    }
                },
            })
        );
        setTimeout(() => this.refresh(), 0);
    }

    get viewComponent(): ViewComponent {
        return AccountsView;
    }

    get providerAccounts(): Record<string, Account[]> {
        return globalStore.get(this.accountsByProviderAtom);
    }

    async refresh() {
        globalStore.set(this.loadingAtom, true);
        try {
            const providers = await this.waveEnv.rpc.ListAccountProvidersCommand(TabRpcClient);
            globalStore.set(this.providersAtom, providers);
            const byProvider: Record<string, Account[]> = {};
            for (const provider of providers) {
                try {
                    byProvider[provider.id] = await this.waveEnv.rpc.ListAccountsCommand(TabRpcClient, { provider: provider.id });
                } catch (e) {
                    byProvider[provider.id] = [];
                }
            }
            globalStore.set(this.accountsByProviderAtom, byProvider);
            const page = globalStore.get(this.pageAtom);
            const selected = globalStore.get(this.selectedProviderAtom);
            if (page === "provider" && byProvider[selected] != null) {
                const autoSwitch = await this.waveEnv.rpc.GetAccountAutoSwitchCommand(TabRpcClient, selected);
                globalStore.set(this.autoSwitchAtom, autoSwitch);
            }
            globalStore.set(this.errorAtom, "");
        } catch (e) {
            globalStore.set(this.errorAtom, `Error loading accounts: ${e?.message ?? e}`);
        } finally {
            globalStore.set(this.loadingAtom, false);
        }
    }

    openDashboard() {
        globalStore.set(this.pageAtom, "dashboard");
    }

    async openProvider(provider: string) {
        globalStore.set(this.selectedProviderAtom, provider);
        globalStore.set(this.pageAtom, "provider");
        globalStore.set(this.searchAtom, "");
        globalStore.set(this.tagFilterAtom, []);
        globalStore.set(this.selectedAtom, []);
        await this.refresh();
        await this.loadInstances(provider);
        await this.loadWakeTasks(provider);
        if (provider === "codex") {
            await this.loadCodexApiStatus();
        }
    }

    async loadInstances(provider: string) {
        try {
            const instances = await this.waveEnv.rpc.ListAccountInstancesCommand(TabRpcClient, provider);
            globalStore.set(this.instancesAtom, instances);
        } catch (e) {
            console.log("failed to load instances", e);
        }
    }

    async createInstance(providerID: string, account: Account) {
        try {
            const instance = await this.waveEnv.rpc.CreateAccountInstanceCommand(TabRpcClient, {
                provider: providerID,
                accountid: account.id,
            });
            await this.startInstance(instance);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not create instance: ${e?.message ?? e}`);
        }
    }

    async startInstance(instance: Instance) {
        try {
            await this.waveEnv.rpc.StartAccountInstanceCommand(TabRpcClient, {
                instanceid: instance.id,
                tabid: globalStore.get(atoms.staticTabId),
            });
            await this.loadInstances(instance.provider);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not start instance: ${e?.message ?? e}`);
        }
    }

    async stopInstance(instance: Instance) {
        try {
            await this.waveEnv.rpc.StopAccountInstanceCommand(TabRpcClient, instance.id);
            await this.loadInstances(instance.provider);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not stop instance: ${e?.message ?? e}`);
        }
    }

    async deleteInstance(instance: Instance) {
        if (!window.confirm(`Delete instance "${instance.name}" and its profile?`)) {
            return;
        }
        try {
            await this.waveEnv.rpc.DeleteAccountInstanceCommand(TabRpcClient, instance.id);
            await this.loadInstances(instance.provider);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not delete instance: ${e?.message ?? e}`);
        }
    }

    providerInfo(providerID: string): ProviderInfo {
        return globalStore.get(this.providersAtom).find((p) => p.id === providerID);
    }

    async refreshProvider(providerID: string) {
        const accounts = this.providerAccounts[providerID] ?? [];
        for (const account of accounts) {
            try {
                await this.waveEnv.rpc.RefreshAccountQuotaCommand(TabRpcClient, { provider: providerID, accountid: account.id });
            } catch (e) {
                globalStore.set(
                    this.errorAtom,
                    `Quota refresh failed for ${account.label ?? account.id}: ${e?.message ?? e}`
                );
            }
        }
        await this.refresh();
    }

    async refreshAllQuotas() {
        const providers = globalStore.get(this.providersAtom);
        for (const provider of providers) {
            await this.refreshProvider(provider.id);
        }
    }

    async importCurrent(providerID: string) {
        try {
            await this.waveEnv.rpc.ImportCurrentAccountCommand(TabRpcClient, providerID);
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Import failed: ${e?.message ?? e}`);
        }
    }

    async startBrowserLogin(providerID: string) {
        try {
            const start = await this.waveEnv.rpc.StartAccountLoginCommand(TabRpcClient, providerID);
            globalStore.set(this.loginStartAtom, start);
            globalStore.set(this.loginErrorAtom, "");
            if (start.provider === "grok") {
                this.startLoginPolling();
            }
        } catch (e) {
            globalStore.set(this.loginErrorAtom, `Login failed: ${e?.message ?? e}`);
        }
    }

    async submitLoginCode() {
        const start = globalStore.get(this.loginStartAtom);
        const code = globalStore.get(this.loginCodeAtom);
        if (start == null || code.trim() === "") {
            return;
        }
        try {
            const account = await this.waveEnv.rpc.SubmitAccountLoginCodeCommand(TabRpcClient, {
                sessionid: start.sessionid,
                code: code.trim(),
            });
            if (account) {
                globalStore.set(this.loginStartAtom, null);
                globalStore.set(this.loginCodeAtom, "");
                globalStore.set(this.loginErrorAtom, "");
                await this.refresh();
            }
        } catch (e) {
            globalStore.set(this.loginErrorAtom, `Login failed: ${e?.message ?? e}`);
        }
    }

    startLoginPolling() {
        this.stopLoginPolling();
        this.loginTimer = setInterval(async () => {
            const start = globalStore.get(this.loginStartAtom);
            if (start == null) {
                this.stopLoginPolling();
                return;
            }
            try {
                const account = await this.waveEnv.rpc.PollAccountLoginCommand(TabRpcClient, start.sessionid);
                if (account) {
                    this.stopLoginPolling();
                    globalStore.set(this.loginStartAtom, null);
                    globalStore.set(this.loginErrorAtom, "");
                    await this.refresh();
                }
            } catch (e) {
                this.stopLoginPolling();
                globalStore.set(this.loginErrorAtom, `Login failed: ${e?.message ?? e}`);
            }
        }, 5000);
    }

    stopLoginPolling() {
        if (this.loginTimer != null) {
            clearInterval(this.loginTimer);
            this.loginTimer = null;
        }
    }

    cancelBrowserLogin() {
        this.stopLoginPolling();
        globalStore.set(this.loginStartAtom, null);
        globalStore.set(this.loginErrorAtom, "");
    }

    async exportAccounts() {
        try {
            const data = await this.waveEnv.rpc.ExportAccountsCommand(TabRpcClient);
            await getApi().saveTextFile("quasar-accounts-backup.json", data);
        } catch (e) {
            globalStore.set(this.errorAtom, `Export failed: ${e?.message ?? e}`);
        }
    }

    async importAccounts(data: string) {
        try {
            const imported = await this.waveEnv.rpc.ImportAccountsCommand(TabRpcClient, data);
            await this.refresh();
            if (imported === 0) {
                globalStore.set(this.errorAtom, "No accounts were imported from the backup");
            }
        } catch (e) {
            globalStore.set(this.errorAtom, `Import failed: ${e?.message ?? e}`);
        }
    }

    async importBackup() {
        try {
            const result = await getApi().openTextFile({ title: "Import accounts backup", extensions: ["json"] });
            if (result == null) {
                return;
            }
            await this.importAccounts(result.content);
        } catch (e) {
            globalStore.set(this.errorAtom, `Import failed: ${e?.message ?? e}`);
        }
    }

    async switchAccount(providerID: string, account: Account, resume: boolean) {
        try {
            if (resume) {
                const blockId = this.getResumeBlockId();
                if (blockId == null) {
                    globalStore.set(this.errorAtom, "No terminal block available to resume");
                    return;
                }
                await this.waveEnv.rpc.SwitchAccountAndResumeCommand(TabRpcClient, {
                    provider: providerID,
                    accountid: account.id,
                    blockid: blockId,
                });
            } else {
                await this.waveEnv.rpc.SwitchAccountCommand(TabRpcClient, { provider: providerID, accountid: account.id });
            }
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Switch failed: ${e?.message ?? e}`);
        }
    }

    getResumeBlockId(): string {
        const selected = globalStore.get(this.targetBlockAtom);
        if (selected) {
            return selected;
        }
        const focused = getFocusedBlockId();
        const models = getAllBlockComponentModels();
        const focusedModel = models.find((m) => m.blockId === focused);
        if (focusedModel?.viewModel?.viewType === "term") {
            return focused;
        }
        return models.find((m) => m.viewModel?.viewType === "term")?.blockId ?? null;
    }

    async refreshQuota(providerID: string, account: Account) {
        try {
            await this.waveEnv.rpc.RefreshAccountQuotaCommand(TabRpcClient, { provider: providerID, accountid: account.id });
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Quota refresh failed: ${e?.message ?? e}`);
        }
    }

    async deleteAccount(providerID: string, account: Account) {
        if (!window.confirm(`Delete account "${account.label ?? account.email ?? account.id}"?`)) {
            return;
        }
        try {
            await this.waveEnv.rpc.DeleteAccountCommand(TabRpcClient, { provider: providerID, accountid: account.id });
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Delete failed: ${e?.message ?? e}`);
        }
    }

    async renameAccount(providerID: string, account: Account) {
        const current = account.label || account.email || account.id;
        const next = window.prompt("Account label", current);
        if (next == null || next.trim() === "" || next === current) {
            return;
        }
        try {
            await this.waveEnv.rpc.SetAccountLabelCommand(TabRpcClient, {
                provider: providerID,
                accountid: account.id,
                label: next.trim(),
            });
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Rename failed: ${e?.message ?? e}`);
        }
    }

    async toggleAutoSwitch() {
        const provider = globalStore.get(this.selectedProviderAtom);
        const next = !globalStore.get(this.autoSwitchAtom);
        try {
            await this.waveEnv.rpc.SetAccountAutoSwitchCommand(TabRpcClient, { provider, enabled: next });
            globalStore.set(this.autoSwitchAtom, next);
        } catch (e) {
            globalStore.set(this.errorAtom, `Setting update failed: ${e?.message ?? e}`);
        }
    }

    setSearch(value: string) {
        globalStore.set(this.searchAtom, value);
    }

    toggleTagFilter(tag: string) {
        const current = globalStore.get(this.tagFilterAtom);
        const next = current.includes(tag) ? current.filter((t) => t !== tag) : [...current, tag];
        globalStore.set(this.tagFilterAtom, next);
    }

    clearFilters() {
        globalStore.set(this.searchAtom, "");
        globalStore.set(this.tagFilterAtom, []);
    }

    getSelectedIds(): string[] {
        return globalStore.get(this.selectedAtom);
    }

    toggleSelect(accountID: string) {
        const current = globalStore.get(this.selectedAtom);
        const next = current.includes(accountID) ? current.filter((id) => id !== accountID) : [...current, accountID];
        globalStore.set(this.selectedAtom, next);
    }

    toggleSelectAll(accounts: Account[]) {
        const current = globalStore.get(this.selectedAtom);
        const allIds = accounts.map((a) => a.id);
        const allSelected = allIds.length > 0 && allIds.every((id) => current.includes(id));
        globalStore.set(this.selectedAtom, allSelected ? [] : allIds);
    }

    clearSelection() {
        globalStore.set(this.selectedAtom, []);
    }

    selectedAccounts(providerID: string): Account[] {
        const selected = globalStore.get(this.selectedAtom);
        return (this.providerAccounts[providerID] ?? []).filter((a) => selected.includes(a.id));
    }

    async setTags(providerID: string, account: Account) {
        const current = (account.tags ?? []).join(", ");
        const next = window.prompt("Tags (comma separated, max 10)", current);
        if (next == null) {
            return;
        }
        const tags = next
            .split(",")
            .map((tag) => tag.trim())
            .filter((tag) => tag !== "");
        try {
            await this.waveEnv.rpc.SetAccountTagsCommand(TabRpcClient, {
                provider: providerID,
                accountid: account.id,
                tags,
            });
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Tag update failed: ${e?.message ?? e}`);
        }
    }

    async exportAccount(providerID: string, account: Account) {
        try {
            const data = await this.waveEnv.rpc.ExportAccountCommand(TabRpcClient, {
                provider: providerID,
                accountid: account.id,
                includesecrets: true,
            });
            const name = sanitizeFilename(account.label || account.email || account.id);
            await getApi().saveTextFile(`quasar-account-${providerID}-${name}.json`, data);
        } catch (e) {
            globalStore.set(this.errorAtom, `Export failed: ${e?.message ?? e}`);
        }
    }

    async exportSelected(providerID: string) {
        const accounts = this.selectedAccounts(providerID);
        if (accounts.length === 0) {
            return;
        }
        try {
            const parts: string[] = [];
            for (const account of accounts) {
                parts.push(
                    await this.waveEnv.rpc.ExportAccountCommand(TabRpcClient, {
                        provider: providerID,
                        accountid: account.id,
                        includesecrets: true,
                    })
                );
            }
            await getApi().saveTextFile(`quasar-accounts-${providerID}.json`, mergeAccountExports(parts));
            this.clearSelection();
        } catch (e) {
            globalStore.set(this.errorAtom, `Export failed: ${e?.message ?? e}`);
        }
    }

    async batchRefresh(providerID: string) {
        const accounts = this.selectedAccounts(providerID);
        for (const account of accounts) {
            try {
                await this.waveEnv.rpc.RefreshAccountQuotaCommand(TabRpcClient, { provider: providerID, accountid: account.id });
            } catch (e) {
                globalStore.set(
                    this.errorAtom,
                    `Quota refresh failed for ${account.label ?? account.id}: ${e?.message ?? e}`
                );
            }
        }
        await this.refresh();
    }

    async batchDelete(providerID: string) {
        const accounts = this.selectedAccounts(providerID);
        if (accounts.length === 0) {
            return;
        }
        const names = accounts.map((a) => a.label || a.email || a.id).join(", ");
        if (!window.confirm(`Delete ${accounts.length} account(s)?\n\n${names}`)) {
            return;
        }
        try {
            for (const account of accounts) {
                await this.waveEnv.rpc.DeleteAccountCommand(TabRpcClient, { provider: providerID, accountid: account.id });
            }
            this.clearSelection();
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Delete failed: ${e?.message ?? e}`);
        }
    }

    async updateInstance(instance: Instance) {
        const name = window.prompt("Instance name", instance.name);
        if (name == null) {
            return;
        }
        const dir = window.prompt("Profile directory (empty = app default)", instance.dir ?? "");
        if (dir == null) {
            return;
        }
        const argsText = window.prompt("Extra launch arguments (space separated)", (instance.args ?? []).join(" "));
        if (argsText == null) {
            return;
        }
        const args = argsText
            .split(/\s+/)
            .map((arg) => arg.trim())
            .filter((arg) => arg !== "");
        try {
            await this.waveEnv.rpc.UpdateAccountInstanceCommand(TabRpcClient, {
                instanceid: instance.id,
                name: name.trim(),
                dir: dir.trim(),
                args,
            });
            await this.loadInstances(instance.provider);
        } catch (e) {
            globalStore.set(this.errorAtom, `Instance update failed: ${e?.message ?? e}`);
        }
    }

    focusInstance(instance: Instance) {
        if (!instance.blockid) {
            globalStore.set(this.errorAtom, "Instance is not running");
            return;
        }
        try {
            refocusNode(instance.blockid);
            globalStore.set(this.errorAtom, "");
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not focus the instance terminal: ${e?.message ?? e}`);
        }
    }

    async stopAllInstances(providerID: string) {
        const instances = globalStore.get(this.instancesAtom).filter((i) => i.status === "running");
        if (instances.length === 0) {
            return;
        }
        try {
            for (const instance of instances) {
                await this.waveEnv.rpc.StopAccountInstanceCommand(TabRpcClient, instance.id);
            }
            await this.loadInstances(providerID);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not stop instances: ${e?.message ?? e}`);
        }
    }

    async loadWakeTasks(provider: string) {
        try {
            const tasks = await this.waveEnv.rpc.ListWakeTasksCommand(TabRpcClient, provider);
            globalStore.set(this.wakeTasksAtom, tasks ?? []);
            const runs: Record<string, WakeRun[]> = {};
            for (const task of tasks ?? []) {
                try {
                    runs[task.id] = await this.waveEnv.rpc.ListWakeRunsCommand(TabRpcClient, {
                        taskid: task.id,
                        limit: 5,
                    });
                } catch (e) {
                    runs[task.id] = [];
                }
            }
            globalStore.set(this.wakeRunsAtom, runs);
        } catch (e) {
            console.log("failed to load wake tasks", e);
        }
    }

    async createWakeTask(providerID: string, accountID: string, intervalMinutes: number, model: string) {
        globalStore.set(this.wakeBusyAtom, true);
        try {
            await this.waveEnv.rpc.CreateWakeTaskCommand(TabRpcClient, {
                provider: providerID,
                accountid: accountID,
                intervalminutes: intervalMinutes,
                model: model.trim() || undefined,
            });
            await this.loadWakeTasks(providerID);
            globalStore.set(this.errorAtom, "");
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not create wake task: ${e?.message ?? e}`);
        } finally {
            globalStore.set(this.wakeBusyAtom, false);
        }
    }

    async toggleWakeTask(task: WakeTask) {
        try {
            await this.waveEnv.rpc.UpdateWakeTaskCommand(TabRpcClient, {
                taskid: task.id,
                enabled: !task.enabled,
                intervalminutes: task.intervalminutes,
                model: task.model ?? "",
            });
            await this.loadWakeTasks(task.provider);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not update wake task: ${e?.message ?? e}`);
        }
    }

    async editWakeTask(task: WakeTask) {
        const intervalText = window.prompt("Wake interval in minutes (15 - 1440)", String(task.intervalminutes));
        if (intervalText == null) {
            return;
        }
        const interval = Number(intervalText);
        if (!Number.isFinite(interval) || interval <= 0) {
            globalStore.set(this.errorAtom, "Invalid wake interval");
            return;
        }
        const model = window.prompt("Model (empty = CLI default)", task.model ?? "");
        if (model == null) {
            return;
        }
        try {
            await this.waveEnv.rpc.UpdateWakeTaskCommand(TabRpcClient, {
                taskid: task.id,
                enabled: task.enabled,
                intervalminutes: Math.round(interval),
                model: model.trim(),
            });
            await this.loadWakeTasks(task.provider);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not update wake task: ${e?.message ?? e}`);
        }
    }

    async runWakeTask(task: WakeTask) {
        globalStore.set(this.wakeBusyAtom, true);
        try {
            const run = await this.waveEnv.rpc.RunWakeTaskCommand(TabRpcClient, task.id);
            if (run?.status !== "ok") {
                globalStore.set(this.errorAtom, `Wake failed: ${run?.error ?? "unknown error"}`);
            } else {
                globalStore.set(this.errorAtom, "");
            }
            await this.loadWakeTasks(task.provider);
            await this.refresh();
        } catch (e) {
            globalStore.set(this.errorAtom, `Wake failed: ${e?.message ?? e}`);
        } finally {
            globalStore.set(this.wakeBusyAtom, false);
        }
    }

    async deleteWakeTask(task: WakeTask) {
        if (!window.confirm(`Delete the wake task for this account?`)) {
            return;
        }
        try {
            await this.waveEnv.rpc.DeleteWakeTaskCommand(TabRpcClient, task.id);
            await this.loadWakeTasks(task.provider);
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not delete wake task: ${e?.message ?? e}`);
        }
    }

    async loadCodexApiStatus() {
        try {
            const status = await this.waveEnv.rpc.GetCodexApiStatusCommand(TabRpcClient);
            globalStore.set(this.codexApiAtom, status);
        } catch (e) {
            console.log("failed to load codex api status", e);
        }
    }

    async applyCodexApiSettings(enabled: boolean, port: number, apiKey: string) {
        try {
            const status = await this.waveEnv.rpc.SetCodexApiSettingsCommand(TabRpcClient, {
                enabled,
                port,
                apikey: apiKey,
            });
            globalStore.set(this.codexApiAtom, status);
            globalStore.set(this.codexApiErrorAtom, "");
        } catch (e) {
            globalStore.set(this.codexApiErrorAtom, `${e?.message ?? e}`);
        }
    }

    async bindFocusedTerminal(provider: string | null) {
        const blockId = getFocusedBlockId();
        if (!blockId) {
            globalStore.set(this.errorAtom, "Focus a terminal block first");
            return;
        }
        const bcm = getBlockComponentModel(blockId);
        if (bcm?.viewModel?.viewType !== "term") {
            globalStore.set(this.errorAtom, "Focus a terminal block first");
            return;
        }
        try {
            await this.waveEnv.rpc.SetMetaCommand(TabRpcClient, {
                oref: WOS.makeORef("block", blockId),
                meta: { "account:provider": provider },
            });
            globalStore.set(this.errorAtom, "");
        } catch (e) {
            globalStore.set(this.errorAtom, `Binding failed: ${e?.message ?? e}`);
        }
    }

    giveFocus(): boolean {
        return true;
    }

    dispose() {
        this.stopLoginPolling();
        this.unsubs.forEach((unsub) => unsub());
        this.unsubs = [];
    }
}

const AccountListCard = memo(
    ({
        model,
        provider,
        account,
        selected,
    }: {
        model: AccountsViewModel;
        provider: ProviderInfo;
        account: Account;
        selected: boolean;
    }) => {
        const [busy, setBusy] = useState(false);
        const metrics = accountMetrics(account);
        const run = async (fn: () => Promise<void>) => {
            setBusy(true);
            try {
                await fn();
            } finally {
                setBusy(false);
            }
        };
        return (
            <div
                className={`flex flex-col gap-3 p-4 rounded-xl border bg-panel/50 ${account.isactive ? "border-accent/60" : "border-border"} ${selected ? "ring-1 ring-accent/40" : ""}`}
            >
                <div className="flex flex-row items-center justify-between gap-2">
                    <div className="flex flex-row items-center gap-2 min-w-0">
                        <input
                            type="checkbox"
                            className="shrink-0"
                            checked={selected}
                            title="Select for batch actions"
                            onChange={() => model.toggleSelect(account.id)}
                        />
                        <span className="text-[13px] font-medium text-primary truncate">
                            {account.label || account.email || account.id}
                        </span>
                        {account.isactive && (
                            <span className="text-[9px] px-1.5 py-0.5 rounded-full bg-accent/20 text-accent font-bold">
                                ACTIVE
                            </span>
                        )}
                        {account.plantype && (
                            <span className="text-[9px] px-1.5 py-0.5 rounded bg-hover text-secondary uppercase">
                                {account.plantype}
                            </span>
                        )}
                    </div>
                    <div className="flex flex-row items-center gap-1 shrink-0">
                        <button
                            className="w-7 h-7 rounded-md hover:bg-hover text-secondary"
                            title="Edit tags"
                            onClick={() => run(() => model.setTags(provider.id, account))}
                        >
                            <i className="fa fa-tags text-[11px]" />
                        </button>
                        <button
                            className="w-7 h-7 rounded-md hover:bg-hover text-secondary"
                            title="Export this account (includes credentials)"
                            onClick={() => run(() => model.exportAccount(provider.id, account))}
                        >
                            <i className="fa fa-download text-[11px]" />
                        </button>
                        <button
                            className="w-7 h-7 rounded-md hover:bg-hover text-secondary"
                            title="Rename"
                            onClick={() => run(() => model.renameAccount(provider.id, account))}
                        >
                            <i className="fa fa-pen text-[11px]" />
                        </button>
                        <button
                            className="w-7 h-7 rounded-md hover:bg-red-500/20 text-secondary hover:text-red-400"
                            title="Delete"
                            onClick={() => run(() => model.deleteAccount(provider.id, account))}
                        >
                            <i className="fa fa-trash text-[11px]" />
                        </button>
                    </div>
                </div>
                {(account.tags?.length ?? 0) > 0 && (
                    <div className="flex flex-row items-center gap-1.5 flex-wrap">
                        {account.tags.map((tag) => (
                            <button
                                key={tag}
                                className="text-[10px] px-1.5 py-0.5 rounded bg-hover text-secondary hover:text-primary"
                                title="Filter by this tag"
                                onClick={() => model.toggleTagFilter(tag)}
                            >
                                #{tag}
                            </button>
                        ))}
                    </div>
                )}
                {account.quotaerror && (
                    <div className="text-[11px] text-red-400 bg-red-500/10 border border-red-500/20 rounded-lg px-3 py-2">
                        {account.quotaerror}
                    </div>
                )}
                {metrics.length > 0 && (
                    <div className="flex flex-col gap-2">
                        {metrics.map((metric) => (
                            <MetricBar key={metric.name} metric={metric} />
                        ))}
                    </div>
                )}
                <div className="flex flex-row items-center justify-between gap-2">
                    <span className="text-[10px] text-muted">
                        {account.quota ? `checked ${formatRelative(account.quota.updatedat)}` : ""}
                    </span>
                    <div className="flex flex-row items-center gap-1.5">
                        <button
                            className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70 text-secondary"
                            disabled={busy}
                            onClick={() => run(() => model.refreshQuota(provider.id, account))}
                        >
                            Refresh
                        </button>
                        <button
                            className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70 text-secondary"
                            disabled={busy}
                            title="Create an isolated instance (own profile) bound to this account"
                            onClick={() => run(() => model.createInstance(provider.id, account))}
                        >
                            <i className="fa fa-window-restore" />
                        </button>
                        {!account.isactive && (
                            <button
                                className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70 text-primary"
                                disabled={busy}
                                onClick={() => run(() => model.switchAccount(provider.id, account, false))}
                            >
                                Switch
                            </button>
                        )}
                        <button
                            className="px-2.5 py-1 text-[11px] rounded-md font-medium bg-accent text-black hover:bg-accenthover disabled:opacity-40"
                            disabled={busy || !provider.resumeargs}
                            title={
                                provider.resumeargs
                                    ? "Switch account and resume the latest conversation in the selected terminal"
                                    : "This provider does not support resume"
                            }
                            onClick={() => run(() => model.switchAccount(provider.id, account, true))}
                        >
                            Switch &amp; Continue
                        </button>
                    </div>
                </div>
            </div>
        );
    }
);
AccountListCard.displayName = "AccountListCard";

const ProviderRail = memo(({ model }: { model: AccountsViewModel }) => {
    const providers = useAtomValue(model.providersAtom);
    const accountsByProvider = useAtomValue(model.accountsByProviderAtom);
    const page = useAtomValue(model.pageAtom);
    const selected = useAtomValue(model.selectedProviderAtom);
    const totalAccounts = Object.values(accountsByProvider).reduce((sum, list) => sum + list.length, 0);
    return (
        <div className="w-[58px] shrink-0 border-r border-border flex flex-col items-center py-3 gap-1 overflow-y-auto">
            <button
                className={`w-10 h-10 rounded-xl flex items-center justify-center text-[16px] ${page === "dashboard" ? "bg-accent/20 text-accent" : "text-secondary hover:bg-hover"}`}
                title={`Dashboard (${totalAccounts} accounts)`}
                onClick={() => model.openDashboard()}
            >
                <i className="fa fa-grip text-[15px]" />
            </button>
            <div className="w-6 border-t border-border my-1" />
            {providers.map((provider) => {
                const count = accountsByProvider[provider.id]?.length ?? 0;
                const isSelected = page === "provider" && selected === provider.id;
                return (
                    <button
                        key={provider.id}
                        className={`relative w-10 h-10 rounded-xl flex items-center justify-center ${isSelected ? "bg-hover" : "hover:bg-hover"} ${provider.available ? "" : "opacity-40"}`}
                        title={`${provider.name}${provider.available ? "" : " (coming soon)"}`}
                        onClick={() => model.openProvider(provider.id)}
                    >
                        <ProviderIcon provider={provider} size={22} />
                        {count > 0 && (
                            <span className="absolute -top-0.5 -right-0.5 min-w-[15px] h-[15px] px-0.5 rounded-full bg-accent text-white text-[9px] font-bold flex items-center justify-center">
                                {count}
                            </span>
                        )}
                    </button>
                );
            })}
        </div>
    );
});
ProviderRail.displayName = "ProviderRail";

function formatDuration(ms: number): string {
    if (!ms) {
        return "0s";
    }
    if (ms < 1000) {
        return "<1s";
    }
    const seconds = Math.round(ms / 1000);
    if (seconds < 60) {
        return `${seconds}s`;
    }
    return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

const WakeTaskForm = memo(
    ({ model, provider, accounts }: { model: AccountsViewModel; provider: ProviderInfo; accounts: Account[] }) => {
        const [accountID, setAccountID] = useState("");
        const [intervalText, setIntervalText] = useState("240");
        const [modelText, setModelText] = useState("");
        const busy = useAtomValue(model.wakeBusyAtom);
        useEffect(() => {
            if (accountID === "" && accounts.length > 0) {
                setAccountID(accounts[0].id);
            }
        }, [accounts, accountID]);
        return (
            <div className="flex flex-row items-center gap-2 flex-wrap">
                <select
                    className="bg-panel border border-border rounded-md px-2 py-1.5 text-[12px] min-w-[160px]"
                    value={accountID}
                    onChange={(e) => setAccountID(e.target.value)}
                >
                    {accounts.map((account) => (
                        <option key={account.id} value={account.id}>
                            {account.label || account.email || account.id}
                        </option>
                    ))}
                </select>
                <input
                    type="number"
                    min={15}
                    max={1440}
                    value={intervalText}
                    title="Interval in minutes"
                    className="w-24 bg-panel border border-border rounded-md px-2 py-1.5 text-[12px]"
                    onChange={(e) => setIntervalText(e.target.value)}
                />
                <input
                    placeholder="model (optional)"
                    value={modelText}
                    className="w-40 bg-panel border border-border rounded-md px-2 py-1.5 text-[12px]"
                    onChange={(e) => setModelText(e.target.value)}
                />
                <button
                    className="px-3 py-1.5 text-[12px] rounded-md font-medium bg-accent text-black hover:bg-accenthover disabled:opacity-40"
                    disabled={busy || accountID === ""}
                    onClick={() =>
                        model.createWakeTask(provider.id, accountID, Math.round(Number(intervalText)) || 240, modelText)
                    }
                >
                    <i className="fa fa-plus mr-1.5 text-[11px]" />
                    Add task
                </button>
            </div>
        );
    }
);
WakeTaskForm.displayName = "WakeTaskForm";

const WakeTasksPanel = memo(
    ({ model, provider, accounts }: { model: AccountsViewModel; provider: ProviderInfo; accounts: Account[] }) => {
        const tasks = useAtomValue(model.wakeTasksAtom);
        const runs = useAtomValue(model.wakeRunsAtom);
        const busy = useAtomValue(model.wakeBusyAtom);
        return (
            <div className="rounded-xl border border-border bg-panel/40 p-4 flex flex-col gap-3">
                <div className="flex flex-col gap-1">
                    <span className="text-[13px] text-primary font-medium">Wake tasks</span>
                    <span className="text-[11px] text-secondary max-w-[620px]">
                        Runs a tiny request with the account&apos;s own CLI on a schedule: it verifies the account still
                        works and starts its quota window early, so the window has reset by the time you need it. Each
                        wake consumes a small amount of quota.
                    </span>
                </div>
                <WakeTaskForm model={model} provider={provider} accounts={accounts} />
                {tasks.length === 0 ? (
                    <span className="text-[11px] text-muted">No wake tasks yet.</span>
                ) : (
                    tasks.map((task) => {
                        const account = accounts.find((a) => a.id === task.accountid);
                        const taskRuns = runs[task.id] ?? [];
                        return (
                            <div key={task.id} className="flex flex-col gap-1.5 px-3 py-2 rounded-lg bg-hover/40">
                                <div className="flex flex-row items-center gap-3 flex-wrap">
                                    <span
                                        className={`w-2 h-2 rounded-full ${
                                            task.laststatus === "ok"
                                                ? "bg-green-500"
                                                : task.laststatus === "error"
                                                  ? "bg-red-500"
                                                  : "bg-zinc-500"
                                        }`}
                                    />
                                    <span className="text-[12px] text-primary">
                                        {account?.label || account?.email || task.accountid}
                                    </span>
                                    <span className="text-[10px] text-muted">every {task.intervalminutes}m</span>
                                    {task.model && <span className="text-[10px] text-muted">{task.model}</span>}
                                    <span className="text-[10px] text-muted">
                                        {task.lastrun
                                            ? `last ran ${formatRelative(task.lastrun)} (${formatDuration(task.lastdurationms ?? 0)})`
                                            : "never ran"}
                                    </span>
                                    <div className="ml-auto flex flex-row items-center gap-1.5">
                                        <label className="flex flex-row items-center gap-1 text-[11px] text-secondary cursor-pointer select-none">
                                            <input
                                                type="checkbox"
                                                checked={task.enabled}
                                                onChange={() => model.toggleWakeTask(task)}
                                            />
                                            enabled
                                        </label>
                                        <button
                                            className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70"
                                            disabled={busy}
                                            title="Run this wake task right now"
                                            onClick={() => model.runWakeTask(task)}
                                        >
                                            <i className="fa fa-bolt mr-1" />
                                            Run now
                                        </button>
                                        <button
                                            className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70 text-secondary"
                                            title="Edit interval and model"
                                            onClick={() => model.editWakeTask(task)}
                                        >
                                            <i className="fa fa-pen" />
                                        </button>
                                        <button
                                            className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-red-500/20 text-secondary"
                                            onClick={() => model.deleteWakeTask(task)}
                                        >
                                            <i className="fa fa-trash" />
                                        </button>
                                    </div>
                                </div>
                                {task.laststatus === "error" && task.lasterror && (
                                    <span className="text-[10px] text-red-400 truncate" title={task.lasterror}>
                                        {task.lasterror}
                                    </span>
                                )}
                                {taskRuns.length > 0 && (
                                    <div className="flex flex-col gap-0.5">
                                        {taskRuns.map((run) => (
                                            <span key={run.startedat} className="text-[10px] text-muted truncate">
                                                <i
                                                    className={`fa mr-1 ${
                                                        run.status === "ok"
                                                            ? "fa-circle-check text-green-500"
                                                            : "fa-circle-xmark text-red-400"
                                                    }`}
                                                />
                                                {formatRelative(run.startedat)} · {formatDuration(run.durationms)}
                                                {run.error ? ` — ${run.error}` : run.output ? ` — ${run.output}` : ""}
                                            </span>
                                        ))}
                                    </div>
                                )}
                            </div>
                        );
                    })
                )}
            </div>
        );
    }
);
WakeTasksPanel.displayName = "WakeTasksPanel";

const CodexApiPanel = memo(({ model }: { model: AccountsViewModel }) => {
    const status = useAtomValue(model.codexApiAtom);
    const error = useAtomValue(model.codexApiErrorAtom);
    const [portText, setPortText] = useState("");
    const [keyText, setKeyText] = useState("");
    useEffect(() => {
        if (status != null) {
            setPortText(String(status.port));
            setKeyText(status.apikey ?? "");
        }
    }, [status?.port, status?.apikey]);
    const enabled = status?.enabled ?? false;
    const apply = (nextEnabled: boolean) => {
        model.applyCodexApiSettings(nextEnabled, Math.round(Number(portText)) || 8318, keyText);
    };
    return (
        <div className="rounded-xl border border-border bg-panel/40 p-4 flex flex-col gap-3">
            <div className="flex flex-row items-center justify-between gap-3 flex-wrap">
                <div className="flex flex-col gap-1">
                    <span className="text-[13px] text-primary font-medium">Local Codex API</span>
                    <span className="text-[11px] text-secondary max-w-[620px]">
                        Serves an OpenAI-compatible endpoint on 127.0.0.1 backed by the managed Codex accounts, so other
                        tools can use your Codex subscription. Endpoints: <span className="font-mono">/v1/models</span>,{" "}
                        <span className="font-mono">/v1/responses</span> (native Codex format) and{" "}
                        <span className="font-mono">/v1/chat/completions</span> (non-streaming). Requests rotate to the
                        next account on auth or quota errors.
                    </span>
                </div>
                <label className="flex flex-row items-center gap-2 text-[12px] text-secondary cursor-pointer select-none shrink-0">
                    <input type="checkbox" checked={enabled} onChange={(e) => apply(e.target.checked)} />
                    {enabled ? "Enabled" : "Disabled"}
                </label>
            </div>
            <div className="flex flex-row items-center gap-2 flex-wrap">
                <label className="flex flex-row items-center gap-2 text-[11px] text-secondary">
                    Port
                    <input
                        type="number"
                        min={1024}
                        max={65535}
                        value={portText}
                        className="w-24 bg-panel border border-border rounded-md px-2 py-1.5 text-[12px]"
                        onChange={(e) => setPortText(e.target.value)}
                    />
                </label>
                <label className="flex flex-row items-center gap-2 text-[11px] text-secondary">
                    API key (optional)
                    <input
                        value={keyText}
                        placeholder="none"
                        className="w-56 bg-panel border border-border rounded-md px-2 py-1.5 text-[12px] font-mono"
                        onChange={(e) => setKeyText(e.target.value)}
                    />
                </label>
                <button
                    className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-hover/70"
                    onClick={() => apply(enabled)}
                >
                    Apply
                </button>
            </div>
            <div className="flex flex-row items-center gap-2 flex-wrap text-[11px]">
                <span
                    className={`w-2 h-2 rounded-full ${status?.running ? "bg-green-500" : "bg-zinc-500"}`}
                />
                <span className="text-secondary">
                    {status?.running
                        ? `running on ${status.baseurl}`
                        : enabled
                          ? "enabled but not running"
                          : "stopped"}
                </span>
                {status?.baseurl && (
                    <button
                        className="px-2 py-0.5 text-[10px] rounded bg-hover hover:bg-hover/70 text-secondary"
                        onClick={() => navigator.clipboard?.writeText(status.baseurl)}
                    >
                        <i className="fa fa-copy mr-1" />
                        Copy base URL
                    </button>
                )}
                {status?.apikey && (
                    <span className="text-muted">clients must send the API key as a bearer token</span>
                )}
            </div>
            {status?.error && <span className="text-[11px] text-red-400">{status.error}</span>}
            {error && <span className="text-[11px] text-red-400">{error}</span>}
        </div>
    );
});
CodexApiPanel.displayName = "CodexApiPanel";

const DashboardPage = memo(({ model }: { model: AccountsViewModel }) => {
    const providers = useAtomValue(model.providersAtom);
    const accountsByProvider = useAtomValue(model.accountsByProviderAtom);
    const withAccounts = providers.filter((p) => (accountsByProvider[p.id]?.length ?? 0) > 0);
    return (
        <div className="flex flex-col gap-4 p-5 overflow-y-auto h-full">
            <div className="flex flex-row items-center justify-between">
                <div className="flex flex-col">
                    <h1 className="text-[20px] font-semibold text-primary">Dashboard</h1>
                    <span className="text-[12px] text-secondary">
                        {Object.values(accountsByProvider).reduce((sum, list) => sum + list.length, 0)} accounts across{" "}
                        {providers.length} platforms
                    </span>
                </div>
                <button
                    className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-hover/70"
                    onClick={() => model.exportAccounts()}
                    title="Export all accounts and credentials to a JSON backup"
                >
                    <i className="fa fa-download mr-1.5 text-[11px]" />
                    Export
                </button>
                <button
                    className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-hover/70"
                    onClick={() => model.importBackup()}
                    title="Import accounts from a JSON backup"
                >
                    <i className="fa fa-upload mr-1.5 text-[11px]" />
                    Import
                </button>
                <button
                    className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-hover/70"
                    onClick={() => model.refreshAllQuotas()}
                >
                    <i className="fa fa-arrows-rotate mr-1.5 text-[11px]" />
                    Refresh all
                </button>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-4 gap-3">
                {providers.map((provider) => {
                    const count = accountsByProvider[provider.id]?.length ?? 0;
                    return (
                        <button
                            key={provider.id}
                            className={`flex flex-row items-center gap-3 px-4 py-3 rounded-xl border text-left transition-colors ${provider.available ? "border-border bg-panel/50 hover:border-accent/40" : "border-border/60 bg-panel/20 opacity-60"}`}
                            onClick={() => model.openProvider(provider.id)}
                        >
                            <ProviderIcon provider={provider} size={26} />
                            <div className="flex flex-col min-w-0">
                                <span className="text-[12px] text-secondary truncate">{provider.name}</span>
                                <span className="text-[18px] font-semibold text-primary leading-tight">{count}</span>
                            </div>
                        </button>
                    );
                })}
            </div>
            {withAccounts.length === 0 && (
                <div className="flex flex-col items-center justify-center gap-3 py-14 text-center border border-dashed border-border rounded-xl">
                    <i className="fa fa-user-plus text-[26px] text-muted" />
                    <div className="text-primary text-[14px] font-medium">No accounts yet</div>
                    <div className="text-secondary text-[12px] max-w-[480px]">
                        Open a provider from the sidebar, log in with its CLI and press "Import current" to manage it
                        here.
                    </div>
                </div>
            )}
            {withAccounts.length > 0 && (
                <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                    {withAccounts.map((provider) => (
                        <ProviderPanel
                            key={provider.id}
                            model={model}
                            provider={provider}
                            accounts={accountsByProvider[provider.id] ?? []}
                        />
                    ))}
                </div>
            )}
        </div>
    );
});
DashboardPage.displayName = "DashboardPage";

const ProviderPage = memo(({ model }: { model: AccountsViewModel }) => {
    const providers = useAtomValue(model.providersAtom);
    const accountsByProvider = useAtomValue(model.accountsByProviderAtom);
    const selected = useAtomValue(model.selectedProviderAtom);
    const autoSwitch = useAtomValue(model.autoSwitchAtom);
    const targetBlock = useAtomValue(model.targetBlockAtom);
    const loginStart = useAtomValue(model.loginStartAtom);
    const loginError = useAtomValue(model.loginErrorAtom);
    const loginCode = useAtomValue(model.loginCodeAtom);
    const instances = useAtomValue(model.instancesAtom);
    const search = useAtomValue(model.searchAtom);
    const tagFilter = useAtomValue(model.tagFilterAtom);
    const selectedIds = useAtomValue(model.selectedAtom);
    const terminalBlocks = getAllBlockComponentModels().filter((m) => m.viewModel?.viewType === "term");
    const provider = providers.find((p) => p.id === selected);
    const accounts = accountsByProvider[selected] ?? [];
    const filteredAccounts = accounts.filter((account) => accountMatchesFilter(account, search, tagFilter));
    const allTags = providerTags(accounts);
    const selectedCount = selectedIds.length;
    const runningInstances = instances.filter((i) => i.status === "running").length;
    const filtersActive = search.trim() !== "" || tagFilter.length > 0;
    if (provider == null) {
        return null;
    }
    return (
        <div className="flex flex-col gap-4 p-5 overflow-y-auto h-full">
            <div className="flex flex-row items-center justify-between gap-4">
                <div className="flex flex-row items-center gap-3">
                    <ProviderIcon provider={provider} size={30} />
                    <div className="flex flex-col">
                        <h1 className="text-[18px] font-semibold text-primary">{provider.name}</h1>
                        <span className="text-[12px] text-secondary">
                            {provider.description} · {accounts.length} account{accounts.length === 1 ? "" : "s"}
                            {!provider.available ? " · coming soon" : ""}
                        </span>
                    </div>
                </div>
                <div className="flex flex-row items-center gap-2">
                    <button
                        className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-hover/70"
                        onClick={() => model.refreshProvider(provider.id)}
                    >
                        <i className="fa fa-arrows-rotate mr-1.5 text-[11px]" />
                        Refresh
                    </button>
                    <button
                        className="px-3 py-1.5 text-[12px] rounded-md font-medium bg-accent text-black hover:bg-accenthover disabled:opacity-40"
                        disabled={!provider.available}
                        onClick={() => model.importCurrent(provider.id)}
                        title={`Import the account currently logged in to the ${provider.name} CLI`}
                    >
                        <i className="fa fa-plus mr-1.5 text-[11px]" />
                        Import current
                    </button>
                    {(provider.id === "grok" || provider.id === "claude" || provider.id === "codex") && (
                        <button
                            className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-hover/70"
                            onClick={() => model.startBrowserLogin(provider.id)}
                            title="Log in with the browser and add the account automatically"
                        >
                            <i className="fa fa-globe mr-1.5 text-[11px]" />
                            Add via browser
                        </button>
                    )}
                </div>
            </div>

            {loginStart != null && loginStart.provider === provider.id && (
                <div className="rounded-xl border border-accent/40 bg-accent/5 p-4 flex flex-col gap-3">
                    {loginStart.provider === "claude" ? (
                        <>
                            <span className="text-[12px] text-secondary">
                                Open the Claude authorization page, approve access, then paste the code shown on the
                                callback page.
                            </span>
                            <div className="flex flex-row items-center gap-3 flex-wrap">
                                <button
                                    className="px-3 py-1.5 text-[12px] rounded-md bg-accent text-black"
                                    onClick={() => getApi().openExternal(loginStart.verificationuri)}
                                >
                                    Open browser
                                </button>
                                <input
                                    className="flex-1 min-w-[260px] bg-panel border border-border rounded-md px-2 py-1.5 text-[12px] font-mono"
                                    placeholder="Paste the authorization code (code#state)"
                                    value={loginCode}
                                    onChange={(e) => globalStore.set(model.loginCodeAtom, e.target.value)}
                                    onKeyDown={(e) => {
                                        if (e.key === "Enter") {
                                            model.submitLoginCode();
                                        }
                                    }}
                                />
                                <button
                                    className="px-3 py-1.5 text-[12px] rounded-md bg-accent text-black"
                                    onClick={() => model.submitLoginCode()}
                                >
                                    Complete
                                </button>
                                <button
                                    className="px-3 py-1.5 text-[12px] rounded-md bg-hover"
                                    onClick={() => model.cancelBrowserLogin()}
                                >
                                    Cancel
                                </button>
                            </div>
                        </>
                    ) : (
                        <>
                            <span className="text-[12px] text-secondary">
                                Open the xAI verification page, enter the code below and authorize. Quasar will detect
                                it automatically.
                            </span>
                            <div className="flex flex-row items-center gap-3 flex-wrap">
                                <span className="text-[24px] font-bold font-mono tracking-[0.25em] text-primary">
                                    {loginStart.usercode}
                                </span>
                                <button
                                    className="px-3 py-1.5 text-[12px] rounded-md bg-accent text-black"
                                    onClick={() =>
                                        getApi().openExternal(
                                            loginStart.verificationuricomplete || loginStart.verificationuri
                                        )
                                    }
                                >
                                    Open browser
                                </button>
                                <button
                                    className="px-3 py-1.5 text-[12px] rounded-md bg-hover"
                                    onClick={() => model.cancelBrowserLogin()}
                                >
                                    Cancel
                                </button>
                            </div>
                            <span className="text-[11px] text-muted">Waiting for authorization…</span>
                        </>
                    )}
                    {loginError && <span className="text-[11px] text-red-400">{loginError}</span>}
                </div>
            )}

            {!provider.available && (
                <div className="px-4 py-3 rounded-xl border border-border bg-panel/40 text-[12px] text-secondary">
                    This platform is registered with its logo, account model and switching entry points, but its
                    credential format and quota API still need to be ported. It will appear here when ready.
                </div>
            )}

            <div className="flex flex-col gap-2">
                <div className="flex flex-row items-center gap-2 flex-wrap">
                    <div className="relative flex-1 min-w-[200px] max-w-[320px]">
                        <i className="fa fa-magnifying-glass absolute left-2.5 top-1/2 -translate-y-1/2 text-[11px] text-muted" />
                        <input
                            className="w-full bg-panel border border-border rounded-md pl-7 pr-2 py-1.5 text-[12px]"
                            placeholder="Search accounts…"
                            value={search}
                            onChange={(e) => model.setSearch(e.target.value)}
                        />
                    </div>
                    {accounts.length > 0 && (
                        <label className="flex flex-row items-center gap-1.5 text-[11px] text-secondary cursor-pointer select-none">
                            <input
                                type="checkbox"
                                checked={selectedCount > 0 && selectedCount === accounts.length}
                                onChange={() => model.toggleSelectAll(accounts)}
                            />
                            Select all
                        </label>
                    )}
                    {filtersActive && (
                        <button
                            className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70 text-secondary"
                            onClick={() => model.clearFilters()}
                        >
                            Clear filters
                        </button>
                    )}
                    {allTags.length > 0 && (
                        <div className="flex flex-row items-center gap-1.5 flex-wrap">
                            {allTags.map((tag) => (
                                <button
                                    key={tag}
                                    className={`text-[11px] px-2 py-0.5 rounded-full border ${tagFilter.includes(tag) ? "border-accent/60 bg-accent/15 text-primary" : "border-border bg-hover/40 text-secondary hover:text-primary"}`}
                                    onClick={() => model.toggleTagFilter(tag)}
                                >
                                    #{tag}
                                </button>
                            ))}
                        </div>
                    )}
                </div>
                {selectedCount > 0 && (
                    <div className="flex flex-row items-center gap-2 px-3 py-2 rounded-lg border border-accent/40 bg-accent/10 text-[12px]">
                        <span className="text-primary">{selectedCount} selected</span>
                        <div className="ml-auto flex flex-row items-center gap-1.5">
                            <button
                                className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70"
                                onClick={() => model.batchRefresh(provider.id)}
                            >
                                <i className="fa fa-arrows-rotate mr-1" />
                                Refresh
                            </button>
                            <button
                                className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70"
                                onClick={() => model.exportSelected(provider.id)}
                            >
                                <i className="fa fa-download mr-1" />
                                Export
                            </button>
                            <button
                                className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-red-500/20 text-secondary"
                                onClick={() => model.batchDelete(provider.id)}
                            >
                                <i className="fa fa-trash mr-1" />
                                Delete
                            </button>
                            <button
                                className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70 text-secondary"
                                onClick={() => model.clearSelection()}
                            >
                                Clear
                            </button>
                        </div>
                    </div>
                )}
            </div>

            {accounts.length === 0 ? (
                <div className="flex flex-col items-center justify-center gap-3 py-14 text-center border border-dashed border-border rounded-xl">
                    <ProviderIcon provider={provider} size={34} />
                    <div className="text-primary text-[14px] font-medium">No {provider.name} accounts yet</div>
                    <div className="text-secondary text-[12px] max-w-[420px]">
                        Log in with <span className="font-mono">{provider.clicommand}</span> in a terminal, then press
                        "Import current".
                    </div>
                </div>
            ) : filteredAccounts.length === 0 ? (
                <div className="flex flex-col items-center justify-center gap-2 py-10 text-center border border-dashed border-border rounded-xl">
                    <i className="fa fa-filter text-[20px] text-muted" />
                    <div className="text-secondary text-[12px]">No accounts match the current search or tag filter.</div>
                </div>
            ) : (
                <div className="grid grid-cols-1 xl:grid-cols-2 gap-3">
                    {filteredAccounts.map((account) => (
                        <AccountListCard
                            key={account.id}
                            model={model}
                            provider={provider}
                            account={account}
                            selected={selectedIds.includes(account.id)}
                        />
                    ))}
                </div>
            )}

            <div className="rounded-xl border border-border bg-panel/40 p-4 flex flex-col gap-2">
                <div className="flex flex-row items-center justify-between">
                    <span className="text-[13px] text-primary font-medium">Instances</span>
                    <div className="flex flex-row items-center gap-2">
                        <span className="text-[11px] text-secondary">Isolated profiles bound to an account</span>
                        {runningInstances > 0 && (
                            <button
                                className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-red-500/20 text-secondary"
                                title="Stop every running instance of this platform"
                                onClick={() => model.stopAllInstances(provider.id)}
                            >
                                <i className="fa fa-stop mr-1" />
                                Stop all ({runningInstances})
                            </button>
                        )}
                    </div>
                </div>
                {instances.length === 0 ? (
                    <span className="text-[11px] text-muted">
                        No instances yet. Use the window icon on an account card to create one.
                    </span>
                ) : (
                    instances.map((instance) => (
                        <div
                            key={instance.id}
                            className="flex flex-row items-center gap-3 px-3 py-2 rounded-lg bg-hover/40"
                            title={`profile: ${instance.dir || instance.profiledir}${instance.args?.length ? `\nargs: ${instance.args.join(" ")}` : ""}`}
                        >
                            <span
                                className={`w-2 h-2 rounded-full ${instance.status === "running" ? "bg-green-500" : "bg-zinc-500"}`}
                            />
                            <span className="text-[12px] text-primary">{instance.name}</span>
                            <span className="text-[10px] text-muted">{instance.status}</span>
                            {instance.dir && (
                                <span className="text-[10px] text-muted truncate max-w-[160px]" title={instance.dir}>
                                    {instance.dir}
                                </span>
                            )}
                            <div className="ml-auto flex flex-row items-center gap-1.5">
                                {instance.status === "running" && (
                                    <button
                                        className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70"
                                        title="Focus this instance's terminal"
                                        onClick={() => model.focusInstance(instance)}
                                    >
                                        <i className="fa fa-crosshairs" />
                                    </button>
                                )}
                                <button
                                    className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70 text-secondary"
                                    title="Edit name, profile directory and launch arguments"
                                    onClick={() => model.updateInstance(instance)}
                                >
                                    <i className="fa fa-pen" />
                                </button>
                                {instance.status === "running" ? (
                                    <button
                                        className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-red-500/20"
                                        onClick={() => model.stopInstance(instance)}
                                    >
                                        Stop
                                    </button>
                                ) : (
                                    <button
                                        className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-hover/70"
                                        onClick={() => model.startInstance(instance)}
                                    >
                                        Open
                                    </button>
                                )}
                                <button
                                    className="px-2.5 py-1 text-[11px] rounded-md bg-hover hover:bg-red-500/20 text-secondary"
                                    onClick={() => model.deleteInstance(instance)}
                                >
                                    <i className="fa fa-trash" />
                                </button>
                            </div>
                        </div>
                    ))
                )}
            </div>

            {provider.wakesupported && (
                <WakeTasksPanel model={model} provider={provider} accounts={accounts} />
            )}

            {provider.id === "codex" && <CodexApiPanel model={model} />}

            <div className="rounded-xl border border-border bg-panel/40 p-4 flex flex-col gap-3">
                <div className="flex flex-row items-center justify-between gap-3 flex-wrap">
                    <div className="flex flex-col">
                        <span className="text-[13px] text-primary font-medium">Session binding</span>
                        <span className="text-[11px] text-secondary">
                            Bind a terminal to {provider.name} so Quasar can auto-switch on quota exhaustion and resume
                            the conversation.
                        </span>
                    </div>
                    <label className="flex flex-row items-center gap-2 text-[12px] text-secondary cursor-pointer select-none">
                        <input type="checkbox" checked={autoSwitch} onChange={() => model.toggleAutoSwitch()} />
                        Auto-switch on quota exhaustion
                    </label>
                </div>
                <div className="flex flex-row items-center gap-2 flex-wrap">
                    <button
                        className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-hover/70"
                        onClick={() => model.bindFocusedTerminal(provider.id)}
                    >
                        Bind focused terminal
                    </button>
                    <button
                        className="px-3 py-1.5 text-[12px] rounded-md bg-hover hover:bg-red-500/20 text-secondary"
                        onClick={() => model.bindFocusedTerminal(null)}
                    >
                        Unbind
                    </button>
                    {terminalBlocks.length > 0 && (
                        <label className="flex flex-row items-center gap-2 text-[12px] text-secondary ml-auto">
                            Resume in:
                            <select
                                className="bg-panel border border-border rounded-md px-2 py-1"
                                value={targetBlock}
                                onChange={(e) => globalStore.set(model.targetBlockAtom, e.target.value || "")}
                            >
                                <option value="">(focused terminal)</option>
                                {terminalBlocks.map((m) => (
                                    <option key={m.blockId} value={m.blockId}>
                                        {m.blockId.substring(0, 8)}
                                        {m.blockId === getFocusedBlockId() ? " (focused)" : ""}
                                    </option>
                                ))}
                            </select>
                        </label>
                    )}
                </div>
            </div>
        </div>
    );
});
ProviderPage.displayName = "ProviderPage";

const AccountsView = memo(({ model }: ViewComponentProps<AccountsViewModel>) => {
    const page = useAtomValue(model.pageAtom);
    const error = useAtomValue(model.errorAtom);
    const lastSwitch = useAtomValue(model.lastSwitchAtom);
    const refreshMinutes = useAtomValue(getSettingsKeyAtom("accounts:refreshinterval")) ?? 5;

    useEffect(() => {
        if (!refreshMinutes || refreshMinutes <= 0) {
            return;
        }
        const timer = setInterval(() => model.refreshAllQuotas(), Math.max(2, refreshMinutes) * 60000);
        return () => clearInterval(timer);
    }, [model, refreshMinutes]);

    return (
        <div className="flex flex-row w-full h-full overflow-hidden">
            <ProviderRail model={model} />
            <div className="flex-1 flex flex-col overflow-hidden">
                {lastSwitch && (
                    <div className="mx-5 mt-4 flex flex-row items-center gap-3 px-4 py-2.5 rounded-xl border border-accent/40 bg-accent/10 text-[12px] text-primary">
                        <i className="fa fa-right-left text-accent" />
                        <span>
                            Switched <b>{lastSwitch.provider}</b>
                            {lastSwitch.fromlabel ? ` from ${lastSwitch.fromlabel}` : ""} to{" "}
                            <b>{lastSwitch.tolabel ?? lastSwitch.toaccountid}</b>
                            {lastSwitch.reason === "quota-exhausted" ? " — quota exhausted" : ""}
                        </span>
                    </div>
                )}
                {error && (
                    <div className="mx-5 mt-4 flex flex-row items-center gap-3 px-4 py-2.5 rounded-xl border border-red-500/40 bg-red-500/10 text-[12px]">
                        <i className="fa fa-triangle-exclamation text-red-400" />
                        <span>{error}</span>
                    </div>
                )}
                {page === "dashboard" ? <DashboardPage model={model} /> : <ProviderPage model={model} />}
            </div>
        </div>
    );
});
AccountsView.displayName = "AccountsView";
