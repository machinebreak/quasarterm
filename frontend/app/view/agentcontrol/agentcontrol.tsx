// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { BlockNodeModel } from "@/app/block/blocktypes";
import {
    AgentLastRunMetaKey,
    getBlockAgentRunAtom,
    getBlockLastActivityAtom,
    parsePersistedAgentRun,
    type AgentRunStatus,
} from "@/app/store/cliprovider";
import { getAllBlockComponentModels, getApi, globalStore, refocusNode } from "@/app/store/global";
import { uxCloseBlock } from "@/app/store/keymodel";
import type { TabModel } from "@/app/store/tab-model";
import * as WOS from "@/app/store/wos";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import {
    agentAlertColor,
    AgentWaitingIdleMs,
    formatAgentDuration,
    isAgentAlertVisible,
    useNowTick,
} from "@/app/tab/agentstatus";
import { getFolderBasename, getTabDisplayName } from "@/app/tab/tabdisplay";
import { getProviderIconUrl, providerAccent, providerDisplayName } from "@/app/view/accounts/providericons";
import type { TermViewModel } from "@/app/view/term/term-model";
import { useWaveEnv, WaveEnv } from "@/app/waveenv/waveenv";
import { cn, fireAndForget } from "@/util/util";
import { atom, useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useMemo, useState } from "react";

// Mission Control: a live dashboard of every AI CLI agent (Claude, Codex, ...)
// running in the workspace, aggregated across all tabs (tile and canvas mode).
// Reuses the per-block run state published by the terminal blocks
// (store/cliprovider.ts) and the account quotas from the accounts service.

type AgentEntry = {
    tabId: string;
    tabLabel: string;
    folderName: string;
    blockId: string;
    run: AgentRunStatus;
    waiting: boolean;
    interrupted?: boolean;
};

type ProviderQuotaChip = {
    accountLabel: string;
    metricName: string;
    percent: number;
    resetat?: number;
    metrics?: { name: string; percent: number }[];
};

// Finished runs older than this are not shown (keeps the list focused).
const AgentMaxDoneEntries = 8;
const AgentQuotaRefreshMs = 60_000;
// Interrupted runs older than this are not surfaced anymore.
const AgentInterruptedMaxAgeMs = 7 * 24 * 3600_000;

function quotaColor(remaining: number): string {
    if (remaining <= 15) return "#ef4444";
    if (remaining <= 40) return "#f59e0b";
    return "#22c55e";
}

function formatResetIn(resetat?: number): string {
    if (resetat == null || resetat <= 0) {
        return null;
    }
    const deltaMs = resetat * 1000 - Date.now();
    if (deltaMs <= 0) {
        return "resetting";
    }
    const minutes = Math.round(deltaMs / 60000);
    if (minutes < 60) {
        return `resets in ${minutes}m`;
    }
    const hours = Math.round(minutes / 60);
    if (hours < 48) {
        return `resets in ${hours}h`;
    }
    return `resets in ${Math.round(hours / 24)}d`;
}

// Running runs first (agents waiting for input on top, then most recent
// start), then the most recently finished runs (capped), so the dashboard
// reads top-to-bottom by relevance.
function sortAgentEntries(entries: AgentEntry[]): AgentEntry[] {
    const running = entries
        .filter((e) => e.run.state === "running")
        .sort((a, b) => {
            if (a.waiting !== b.waiting) {
                return a.waiting ? -1 : 1;
            }
            return b.run.startTs - a.run.startTs;
        });
    const interrupted = entries.filter((e) => e.interrupted === true).sort((a, b) => b.run.startTs - a.run.startTs);
    const done = entries
        .filter((e) => e.run.state === "done" && e.interrupted !== true)
        .sort((a, b) => (b.run.endTs ?? 0) - (a.run.endTs ?? 0))
        .slice(0, AgentMaxDoneEntries);
    return [...running, ...interrupted, ...done];
}

function collectAgentEntries(env: WaveEnv): AgentEntry[] {
    const nowTs = Date.now();
    const ws = globalStore.get(env.atoms.workspace);
    const entries: AgentEntry[] = [];
    for (const tabId of ws?.tabids ?? []) {
        const tab = globalStore.get(env.wos.getWaveObjectAtom<Tab>(WOS.makeORef("tab", tabId)));
        if (tab == null) {
            continue;
        }
        const folderRaw = globalStore.get(env.getTabMetaKeyAtom(tabId, "cmd:cwd"));
        const folderName = getFolderBasename(folderRaw);
        for (const blockId of tab.blockids ?? []) {
            const run = globalStore.get(getBlockAgentRunAtom(blockId));
            if (run == null) {
                const block = globalStore.get(env.wos.getWaveObjectAtom<Block>(WOS.makeORef("block", blockId)));
                const persisted = parsePersistedAgentRun((block as any)?.meta?.[AgentLastRunMetaKey]);
                if (
                    persisted != null &&
                    persisted.state === "running" &&
                    nowTs - persisted.startTs < AgentInterruptedMaxAgeMs
                ) {
                    entries.push({
                        tabId,
                        tabLabel: getTabDisplayName(tab.name, folderName) ?? "",
                        folderName,
                        blockId,
                        run: {
                            providerId: persisted.providerId,
                            state: "done",
                            startTs: persisted.startTs,
                            exitCode: null,
                            acknowledged: true,
                        },
                        waiting: false,
                        interrupted: true,
                    });
                }
                continue;
            }
            let waiting = false;
            if (run.state === "running") {
                const activityTs = globalStore.get(getBlockLastActivityAtom(blockId));
                waiting = activityTs > 0 && nowTs - activityTs >= AgentWaitingIdleMs;
            }
            entries.push({
                tabId,
                tabLabel: getTabDisplayName(tab.name, folderName) ?? "",
                folderName,
                blockId,
                run,
                waiting,
            });
        }
    }
    return sortAgentEntries(entries);
}

// Project key used by the filter chips: preferred folder name, tab label as
// fallback (matches what the row shows).
function entryFolderKey(entry: AgentEntry): string {
    if (entry.folderName != null && entry.folderName !== "") {
        return entry.folderName;
    }
    return entry.tabLabel;
}

function filterAgentEntries(entries: AgentEntry[], folder: string): AgentEntry[] {
    if (folder == null || folder === "") {
        return entries;
    }
    return entries.filter((e) => entryFolderKey(e) === folder);
}

// Last visible line of a mounted terminal block, shown in the dashboard rows
// for running agents. Best effort: null when the block is not mounted (its
// tab is not visible) or has no output yet.
function getBlockLastOutputLine(blockId: string): string {
    try {
        const bcm = getAllBlockComponentModels().find((m) => m.blockId === blockId);
        const termWrap = (bcm?.viewModel as TermViewModel)?.termRef?.current;
        const buffer = termWrap?.terminal?.buffer?.active;
        if (buffer == null) {
            return null;
        }
        const startY = Math.min(buffer.baseY + buffer.cursorY, buffer.length - 1);
        for (let i = 0; i < 8 && startY - i >= 0; i++) {
            const text = buffer
                .getLine(startY - i)
                ?.translateToString(true)
                ?.trim();
            if (text) {
                return text.length > 160 ? text.slice(0, 160) : text;
            }
        }
    } catch (e) {
        // terminal may be mid-teardown
    }
    return null;
}

// Quota of the active account per provider (best effort: providers without
// accounts or fresh quota data simply render no chip).
function useProviderQuotas(env: WaveEnv, providerIds: string[]) {
    const [quotas, setQuotas] = useState<Record<string, ProviderQuotaChip>>({});
    const [activeAccountIds, setActiveAccountIds] = useState<Record<string, string>>({});
    const [resumeSupport, setResumeSupport] = useState<Record<string, boolean>>({});
    const [refreshing, setRefreshing] = useState(false);
    const providerKey = providerIds.join(",");

    // Which providers support resuming a previous session (resumeargs set).
    useEffect(() => {
        fireAndForget(async () => {
            try {
                const providers = await env.rpc.ListAccountProvidersCommand(TabRpcClient);
                const support: Record<string, boolean> = {};
                for (const provider of providers ?? []) {
                    support[provider.id] = (provider.resumeargs ?? "") !== "";
                }
                setResumeSupport(support);
            } catch (e) {
                // best effort
            }
        });
    }, [env]);

    const load = useCallback(async () => {
        const ids = providerKey === "" ? [] : providerKey.split(",");
        const next: Record<string, ProviderQuotaChip> = {};
        const nextActive: Record<string, string> = {};
        for (const pid of ids) {
            try {
                const accounts = await env.rpc.ListAccountsCommand(TabRpcClient, { provider: pid });
                const account = accounts?.find((a) => a.isactive) ?? accounts?.[0];
                if (account != null) {
                    nextActive[pid] = account.id;
                }
                const metrics = account?.quota?.metrics ?? [];
                if (account != null && metrics.length > 0) {
                    // show the most at-risk metric (lowest remaining)
                    const lowest = metrics.reduce((a, b) => (b.remainingpercent < a.remainingpercent ? b : a));
                    next[pid] = {
                        accountLabel: account.label ?? account.email ?? "",
                        metricName: lowest.name ?? "",
                        percent: lowest.remainingpercent ?? 0,
                        resetat: lowest.resetat,
                        metrics: metrics.map((m) => ({ name: m.name ?? "", percent: m.remainingpercent ?? 0 })),
                    };
                }
            } catch (e) {
                // best effort - provider may not have accounts configured
            }
        }
        setQuotas(next);
        setActiveAccountIds(nextActive);
    }, [env, providerKey]);

    useEffect(() => {
        fireAndForget(load);
        const timer = window.setInterval(() => fireAndForget(load), AgentQuotaRefreshMs);
        return () => window.clearInterval(timer);
    }, [load]);

    const refresh = useCallback(() => {
        const ids = providerKey === "" ? [] : providerKey.split(",");
        setRefreshing(true);
        fireAndForget(async () => {
            try {
                for (const pid of ids) {
                    try {
                        const accounts = await env.rpc.ListAccountsCommand(TabRpcClient, { provider: pid });
                        for (const account of accounts ?? []) {
                            if (account.isactive) {
                                await env.rpc.RefreshAccountQuotaCommand(TabRpcClient, {
                                    provider: pid,
                                    accountid: account.id,
                                });
                            }
                        }
                    } catch (e) {
                        // best effort
                    }
                }
                await load();
            } finally {
                setRefreshing(false);
            }
        });
    }, [env, providerKey, load]);

    return { quotas, activeAccountIds, resumeSupport, refreshing, refresh };
}

const AgentProviderBadge = memo(({ providerId, size = 20 }: { providerId: string; size?: number }) => {
    const iconUrl = getProviderIconUrl(providerId);
    if (iconUrl != null) {
        return <img src={iconUrl} alt={providerId} style={{ width: size, height: size }} draggable={false} />;
    }
    return (
        <div
            className="flex items-center justify-center rounded font-bold text-white"
            style={{ width: size, height: size, background: providerAccent(providerId), fontSize: size * 0.5 }}
        >
            {providerDisplayName(providerId).substring(0, 1)}
        </div>
    );
});
AgentProviderBadge.displayName = "AgentProviderBadge";

const AgentRow = memo(function AgentRow({
    entry,
    quota,
    canResume,
    resuming,
    onResume,
    onDismiss,
    onJump,
}: {
    entry: AgentEntry;
    quota?: ProviderQuotaChip;
    canResume?: boolean;
    resuming?: boolean;
    onResume?: (entry: AgentEntry) => void;
    onDismiss?: (entry: AgentEntry) => void;
    onJump: (entry: AgentEntry) => void;
}) {
    const { run, waiting, interrupted } = entry;
    const running = run.state === "running";
    const nowTs = useNowTick(running, 1000);
    const name = providerDisplayName(run.providerId);
    const alert = isAgentAlertVisible(run);
    const statusColor = interrupted ? "#94a3b8" : waiting ? "#f59e0b" : running ? "#4ade80" : agentAlertColor(run);
    const lastLine = running ? getBlockLastOutputLine(entry.blockId) : null;
    const statusIcon = interrupted
        ? "fa-circle-pause"
        : run.exitCode == null
          ? "fa-circle-dot"
          : run.exitCode === 0
            ? "fa-circle-check"
            : "fa-circle-xmark";
    const durationText = formatAgentDuration(nowTs - (running ? run.startTs : (run.endTs ?? run.startTs)));
    const statusText =
        interrupted === true
            ? `interrumpido · hace ${durationText}`
            : running
              ? waiting
                  ? `esperando input · ${durationText}`
                  : `corriendo · ${durationText}`
              : run.exitCode == null || run.exitCode === 0
                ? `terminó · hace ${durationText}`
                : `falló (exit ${run.exitCode}) · hace ${durationText}`;
    const folderLine = entry.folderName != null && entry.folderName !== "" ? entry.folderName : entry.tabLabel;
    const tabSuffix =
        folderLine !== "" && entry.tabLabel !== "" && entry.tabLabel !== folderLine ? ` · ${entry.tabLabel}` : "";
    const quotaTitle =
        quota != null
            ? [
                  quota.accountLabel,
                  ...(quota.metrics != null && quota.metrics.length > 0
                      ? quota.metrics.map((m) => `${m.name} ${Math.round(m.percent)}% left`)
                      : [`${quota.metricName} ${Math.round(quota.percent)}% left`]),
                  formatResetIn(quota.resetat),
              ]
                  .filter((p) => p != null && p !== "")
                  .join(" · ")
            : undefined;
    const rowTitle = `${name}${folderLine !== "" ? ` · ${folderLine}` : ""}\n${statusText}${lastLine != null ? `\n${lastLine}` : ""}`;

    return (
        <div
            className="flex items-center gap-2.5 px-2.5 py-2 rounded-lg cursor-pointer select-none hover:bg-hover transition-colors"
            style={
                waiting
                    ? { background: "color-mix(in srgb, #f59e0b 7%, transparent)" }
                    : alert
                      ? { background: "color-mix(in srgb, var(--accent-color) 7%, transparent)" }
                      : undefined
            }
            onClick={() => onJump(entry)}
            title={rowTitle}
        >
            <div className="flex items-center justify-center w-9 h-9 rounded-lg bg-hover shrink-0">
                <AgentProviderBadge providerId={run.providerId} />
            </div>
            <div className="flex flex-col min-w-0 flex-1 gap-0.5">
                <div className="flex items-center gap-1.5 min-w-0">
                    <span className="text-[13px] font-medium text-primary truncate">{name}</span>
                    {alert ? (
                        <span className="w-1.5 h-1.5 rounded-full shrink-0" style={{ background: statusColor }} />
                    ) : null}
                </div>
                <span className="text-[11px] text-muted truncate">
                    {folderLine}
                    {tabSuffix}
                </span>
                {lastLine != null ? (
                    <span className="text-[10px] text-muted truncate font-mono opacity-70" title={lastLine}>
                        {lastLine}
                    </span>
                ) : null}
            </div>
            <div className="flex flex-col items-end gap-0.5 shrink-0">
                <div className="flex items-center gap-1.5">
                    {running ? (
                        <span className="relative flex h-1.5 w-1.5">
                            <span
                                className="animate-ping absolute inline-flex h-full w-full rounded-full opacity-60"
                                style={{ background: statusColor }}
                            />
                            <span
                                className="relative inline-flex rounded-full h-1.5 w-1.5"
                                style={{ background: statusColor }}
                            />
                        </span>
                    ) : (
                        <i className={cn("fa-solid text-[10px]", statusIcon)} style={{ color: statusColor }} />
                    )}
                    <span className={cn("text-[11px]", running ? "text-secondary" : "text-muted")}>{statusText}</span>
                </div>
                {quota != null ? (
                    <span className="text-[10px]" style={{ color: quotaColor(quota.percent) }} title={quotaTitle}>
                        {quota.metricName} {Math.round(quota.percent)}%
                    </span>
                ) : null}
                {!running && canResume ? (
                    <div className="flex items-center gap-1">
                        <button
                            className="flex items-center gap-1 rounded border border-border px-1.5 py-px text-[10px] text-secondary transition-colors cursor-pointer hover:bg-hover hover:text-primary"
                            onClick={(event) => {
                                event.stopPropagation();
                                onResume?.(entry);
                            }}
                            disabled={resuming}
                            title={`Reanudar ${name} donde quedó (${run.providerId} resume)`}
                        >
                            <i
                                className={cn(
                                    "fa-solid text-[9px]",
                                    resuming ? "fa-spinner animate-spin" : "fa-clock-rotate-left"
                                )}
                            />
                            {resuming ? "Reanudando…" : "Reanudar"}
                        </button>
                        {interrupted === true && onDismiss != null ? (
                            <button
                                className="rounded p-0.5 text-muted transition-colors cursor-pointer hover:bg-hover hover:text-secondary"
                                onClick={(event) => {
                                    event.stopPropagation();
                                    onDismiss(entry);
                                }}
                                title="Descartar"
                            >
                                <i className="fa-solid fa-xmark text-[10px]" />
                            </button>
                        ) : null}
                    </div>
                ) : null}
            </div>
        </div>
    );
});

const SectionLabel = memo(({ children }: { children: React.ReactNode }) => (
    <div className="px-2.5 pt-2 pb-1 text-[10px] uppercase tracking-wide text-muted">{children}</div>
));
SectionLabel.displayName = "SectionLabel";

const FilterChip = memo(({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) => (
    <button
        className={cn(
            "text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap cursor-pointer transition-colors",
            active ? "text-primary" : "border-transparent text-muted hover:text-secondary hover:bg-hover"
        )}
        style={
            active
                ? {
                      background: "color-mix(in srgb, var(--accent-color) 18%, transparent)",
                      borderColor: "color-mix(in srgb, var(--accent-color) 45%, transparent)",
                  }
                : undefined
        }
        onClick={onClick}
    >
        {label}
    </button>
));
FilterChip.displayName = "FilterChip";

const AgentEmptyState = memo(() => (
    <div className="flex flex-col items-center justify-center h-full gap-2 text-muted p-6">
        <i className="fa-solid fa-robot text-[26px] opacity-40" />
        <span className="text-[12px] text-secondary">Sin agentes corriendo</span>
        <span className="text-[11px] text-center opacity-80 max-w-[240px]">
            Ejecutá claude, codex o cualquier otro agente en una terminal y va a aparecer acá.
        </span>
    </div>
));
AgentEmptyState.displayName = "AgentEmptyState";

const AgentControlView: ViewComponent = ({ model }) => {
    const env = useWaveEnv();
    const ws = useAtomValue(env.atoms.workspace);
    const tick = useNowTick(true, 1000);
    const [folderFilter, setFolderFilter] = useState<string>(null);
    const entries = useMemo(() => collectAgentEntries(env), [ws, tick, env]);
    const folderKeys = useMemo(() => {
        const counts = new Map<string, number>();
        for (const entry of entries) {
            const key = entryFolderKey(entry);
            counts.set(key, (counts.get(key) ?? 0) + 1);
        }
        return Array.from(counts.entries()).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
    }, [entries]);
    const activeFilter = folderFilter != null && folderKeys.some(([key]) => key === folderFilter) ? folderFilter : null;
    const visibleEntries = filterAgentEntries(entries, activeFilter);
    const runningEntries = visibleEntries.filter((e) => e.run.state === "running");
    const doneEntries = visibleEntries.filter((e) => e.run.state === "done" && e.interrupted !== true);
    const interruptedEntries = visibleEntries.filter((e) => e.interrupted === true);
    const runningTotal = entries.filter((e) => e.run.state === "running").length;
    const waitingTotal = entries.filter((e) => e.waiting).length;
    const providerIds = useMemo(() => Array.from(new Set(entries.map((e) => e.run.providerId))).sort(), [entries]);
    const { quotas, activeAccountIds, resumeSupport, refreshing, refresh } = useProviderQuotas(env, providerIds);

    const closeSelf = useCallback(() => {
        try {
            uxCloseBlock(model.blockId);
        } catch (e) {
            // best effort (e.g. preview)
        }
    }, [model.blockId]);

    const jump = useCallback((entry: AgentEntry) => {
        try {
            getApi().setActiveTab(entry.tabId);
        } catch (e) {
            // best effort (preview / missing electron api)
        }
        window.setTimeout(() => {
            try {
                refocusNode(entry.blockId);
            } catch (e) {
                // best effort
            }
        }, 250);
    }, []);

    const [resumingBlockId, setResumingBlockId] = useState<string>(null);

    // Resumes the provider CLI where it left off, in the entry's terminal
    // (same mechanism the accounts view uses when switching accounts).
    const resume = useCallback(
        (entry: AgentEntry) => {
            const accountId = activeAccountIds[entry.run.providerId];
            if (accountId == null) {
                return;
            }
            setResumingBlockId(entry.blockId);
            fireAndForget(async () => {
                try {
                    await env.rpc.SwitchAccountAndResumeCommand(TabRpcClient, {
                        provider: entry.run.providerId,
                        accountid: accountId,
                        blockid: entry.blockId,
                    });
                    jump(entry);
                } catch (e) {
                    console.log("agent resume failed", e);
                } finally {
                    setResumingBlockId(null);
                }
            });
        },
        [env, activeAccountIds, jump]
    );

    // Hides an interrupted run from the dashboard without resuming it.
    const dismissInterrupted = useCallback(
        (entry: AgentEntry) => {
            fireAndForget(() =>
                env.rpc.SetMetaCommand(TabRpcClient, {
                    oref: `block:${entry.blockId}`,
                    meta: {
                        [AgentLastRunMetaKey]: {
                            providerId: entry.run.providerId,
                            state: "dismissed",
                            startTs: entry.run.startTs,
                        },
                    } as any,
                })
            );
        },
        [env]
    );

    const canResumeEntry = (entry: AgentEntry) =>
        (resumeSupport[entry.run.providerId] ?? false) && activeAccountIds[entry.run.providerId] != null;

    return (
        <div className="flex flex-col h-full w-full bg-transparent">
            <div className="flex items-center justify-between px-2.5 py-1.5 border-b border-border shrink-0">
                <div className="flex items-center gap-1.5 min-w-0">
                    {runningTotal > 0 ? (
                        <span
                            className="text-[10px] px-1.5 py-px rounded-full font-medium"
                            style={{ background: "color-mix(in srgb, #4ade80 15%, transparent)", color: "#4ade80" }}
                        >
                            {runningTotal} corriendo
                        </span>
                    ) : (
                        <span className="text-[11px] text-muted">Sin agentes</span>
                    )}
                    {waitingTotal > 0 ? (
                        <span
                            className="text-[10px] px-1.5 py-px rounded-full font-medium"
                            style={{ background: "color-mix(in srgb, #f59e0b 15%, transparent)", color: "#f59e0b" }}
                        >
                            {waitingTotal} esperando
                        </span>
                    ) : null}
                </div>
                <div className="flex items-center gap-0.5">
                    <button
                        className="p-1 rounded hover:bg-hover text-secondary"
                        title="Refrescar cuotas"
                        onClick={refresh}
                        disabled={refreshing}
                    >
                        <i className={cn("fa-solid fa-rotate text-[11px]", refreshing && "animate-spin")} />
                    </button>
                    <button
                        className="p-1 rounded hover:bg-hover text-secondary"
                        title="Cerrar panel"
                        onClick={closeSelf}
                    >
                        <i className="fa-solid fa-xmark text-[12px]" />
                    </button>
                </div>
            </div>
            {folderKeys.length > 1 ? (
                <div className="flex items-center gap-1 px-2 py-1.5 border-b border-border shrink-0 overflow-x-auto">
                    <FilterChip label="Todos" active={activeFilter == null} onClick={() => setFolderFilter(null)} />
                    {folderKeys.map(([key, count]) => (
                        <FilterChip
                            key={key}
                            label={count > 1 ? `${key} · ${count}` : key}
                            active={activeFilter === key}
                            onClick={() => setFolderFilter(activeFilter === key ? null : key)}
                        />
                    ))}
                </div>
            ) : null}
            {entries.length === 0 ? (
                <AgentEmptyState />
            ) : (
                <div className="flex-1 overflow-y-auto px-1.5 py-1.5 flex flex-col gap-px">
                    {runningEntries.length > 0 ? <SectionLabel>En curso</SectionLabel> : null}
                    {runningEntries.map((e) => (
                        <AgentRow
                            key={e.blockId}
                            entry={e}
                            quota={quotas[e.run.providerId]}
                            canResume={canResumeEntry(e)}
                            resuming={resumingBlockId === e.blockId}
                            onResume={resume}
                            onJump={jump}
                        />
                    ))}
                    {interruptedEntries.length > 0 ? <SectionLabel>Interrumpidos</SectionLabel> : null}
                    {interruptedEntries.map((e) => (
                        <AgentRow
                            key={e.blockId}
                            entry={e}
                            quota={quotas[e.run.providerId]}
                            canResume={canResumeEntry(e)}
                            resuming={resumingBlockId === e.blockId}
                            onResume={resume}
                            onDismiss={dismissInterrupted}
                            onJump={jump}
                        />
                    ))}
                    {doneEntries.length > 0 ? <SectionLabel>Recientes</SectionLabel> : null}
                    {doneEntries.map((e) => (
                        <AgentRow
                            key={e.blockId}
                            entry={e}
                            quota={quotas[e.run.providerId]}
                            canResume={canResumeEntry(e)}
                            resuming={resumingBlockId === e.blockId}
                            onResume={resume}
                            onJump={jump}
                        />
                    ))}
                </div>
            )}
        </div>
    );
};
AgentControlView.displayName = "AgentControlView";

export class AgentControlViewModel implements ViewModel {
    viewType = "agentcontrol";
    viewIcon = atom("robot");
    viewName = atom("Agentes");
    noPadding = atom(true);
    blockId: string;
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    waveEnv: WaveEnv;

    constructor(initOpts: ViewModelInitType) {
        this.blockId = initOpts.blockId;
        this.nodeModel = initOpts.nodeModel;
        this.tabModel = initOpts.tabModel;
        this.waveEnv = initOpts.waveEnv;
    }

    get viewComponent(): ViewComponent {
        return AgentControlView;
    }
}

export { AgentControlView, collectAgentEntries, entryFolderKey, filterAgentEntries, quotaColor, sortAgentEntries };
