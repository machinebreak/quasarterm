// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { AgentWaitingIdleMs } from "@/app/tab/agentstatus";
import { WaveEnv, WaveEnvSubset } from "@/app/waveenv/waveenv";
import { NullAtom } from "@/util/util";
import { atom, Atom, PrimitiveAtom } from "jotai";
import { globalStore } from "./jotaiStore";
import * as WOS from "./wos";

export type CliProviderEnv = WaveEnvSubset<{
    wos: WaveEnv["wos"];
}>;

// Block meta key holding a snapshot of the last agent run, persisted so
// interrupted runs (app closed mid-task) can be surfaced after a restart.
export const AgentLastRunMetaKey = "agent:lastrun";

export type PersistedAgentRun = {
    providerId: string;
    state: "running" | "done" | "dismissed";
    startTs: number;
    endTs?: number;
    exitCode?: number | null;
};

// State of the last AI CLI agent run in a terminal block, used for the
// "agent finished" tab marker and notifications. "running" while the agent
// process is alive; "done" once the command finished (exitCode may be null
// when the shell did not report one). `acknowledged` flips to true once the
// user visited the tab, which clears the finished marker.
export type AgentRunStatus = {
    providerId: string;
    state: "running" | "done";
    startTs: number;
    endTs?: number;
    exitCode?: number | null;
    acknowledged: boolean;
};

const BlockCliProviderMap = new Map<string, PrimitiveAtom<string>>();
const TabCliProviderAtomCache = new Map<string, Atom<string>>();
const BlockAgentRunMap = new Map<string, PrimitiveAtom<AgentRunStatus | null>>();
const TabAgentRunAtomCache = new Map<string, Atom<AgentRunStatus | null>>();
const BlockLastActivityMap = new Map<string, PrimitiveAtom<number>>();
const ZeroAtom = atom(0) as Atom<number>;

// Provider id (see view/term/clidetect.ts and view/accounts/providericons.tsx)
// of the AI CLI agent currently running in a terminal block, or null when no
// known agent is running. Kept in sync by TermWrap.syncCliProvider().
function getBlockCliProviderAtomInternal(blockId: string): PrimitiveAtom<string> {
    let rtn = BlockCliProviderMap.get(blockId);
    if (rtn == null) {
        rtn = atom(null) as PrimitiveAtom<string>;
        BlockCliProviderMap.set(blockId, rtn);
    }
    return rtn;
}

function getBlockCliProviderAtom(blockId: string): Atom<string> {
    if (blockId == null) {
        return NullAtom as Atom<string>;
    }
    return getBlockCliProviderAtomInternal(blockId);
}

function setBlockCliProvider(blockId: string, providerId: string | null) {
    if (blockId == null) {
        return;
    }
    globalStore.set(getBlockCliProviderAtomInternal(blockId), providerId ?? null);
}

function getBlockAgentRunAtomInternal(blockId: string): PrimitiveAtom<AgentRunStatus | null> {
    let rtn = BlockAgentRunMap.get(blockId);
    if (rtn == null) {
        rtn = atom(null) as PrimitiveAtom<AgentRunStatus | null>;
        BlockAgentRunMap.set(blockId, rtn);
    }
    return rtn;
}

function getBlockAgentRunAtom(blockId: string): Atom<AgentRunStatus | null> {
    if (blockId == null) {
        return NullAtom as Atom<AgentRunStatus | null>;
    }
    return getBlockAgentRunAtomInternal(blockId);
}

// Marks the start of an AI CLI run in a block. Keeps the existing start time
// when the same provider is already running (syncCliProvider fires on every
// command-start event, not just the first one).
function beginBlockAgentRun(blockId: string, providerId: string) {
    if (blockId == null || providerId == null) {
        return;
    }
    const blockAtom = getBlockAgentRunAtomInternal(blockId);
    const current = globalStore.get(blockAtom);
    if (current != null && current.state === "running" && current.providerId === providerId) {
        return;
    }
    globalStore.set(blockAtom, {
        providerId,
        state: "running",
        startTs: Date.now(),
        acknowledged: false,
    });
    globalStore.set(getBlockLastActivityAtomInternal(blockId), Date.now());
}

// Finishes the running agent in a block and returns the finalized run, or
// null when there was no agent running.
function finishBlockAgentRun(blockId: string, exitCode: number | null): AgentRunStatus | null {
    if (blockId == null) {
        return null;
    }
    const blockAtom = getBlockAgentRunAtomInternal(blockId);
    const current = globalStore.get(blockAtom);
    if (current == null || current.state !== "running") {
        return null;
    }
    const finished: AgentRunStatus = {
        ...current,
        state: "done",
        endTs: Date.now(),
        exitCode: exitCode ?? null,
        acknowledged: false,
    };
    globalStore.set(blockAtom, finished);
    return finished;
}

// Timestamp of the last output written to a terminal block. Used to detect
// agents that are still running but idle (likely waiting for user input).
function getBlockLastActivityAtomInternal(blockId: string): PrimitiveAtom<number> {
    let rtn = BlockLastActivityMap.get(blockId);
    if (rtn == null) {
        rtn = atom(0) as PrimitiveAtom<number>;
        BlockLastActivityMap.set(blockId, rtn);
    }
    return rtn;
}

function getBlockLastActivityAtom(blockId: string): Atom<number> {
    if (blockId == null) {
        return ZeroAtom;
    }
    return getBlockLastActivityAtomInternal(blockId);
}

// Called by TermWrap whenever terminal output is written for a block whose
// agent is running (throttled by the caller).
function markBlockAgentActivity(blockId: string) {
    if (blockId == null) {
        return;
    }
    globalStore.set(getBlockLastActivityAtomInternal(blockId), Date.now());
}

// Parses the persisted "agent:lastrun" block meta value (object or JSON
// string). Returns null when the value is missing or malformed.
function parsePersistedAgentRun(raw: unknown): PersistedAgentRun {
    if (raw == null) {
        return null;
    }
    let value: any = raw;
    if (typeof raw === "string") {
        try {
            value = JSON.parse(raw);
        } catch (e) {
            return null;
        }
    }
    if (value == null || typeof value !== "object" || typeof value.providerId !== "string") {
        return null;
    }
    if (typeof value.startTs !== "number" || value.startTs <= 0) {
        return null;
    }
    return {
        providerId: value.providerId,
        state: value.state === "done" || value.state === "dismissed" ? value.state : "running",
        startTs: value.startTs,
        endTs: typeof value.endTs === "number" ? value.endTs : undefined,
        exitCode: value.exitCode ?? null,
    };
}

// True when any of the given blocks has a running agent that has been idle
// (no terminal output) for at least AgentWaitingIdleMs.
function isAnyBlockAgentWaiting(blockIds: string[], nowTs: number): boolean {
    for (const blockId of blockIds ?? []) {
        const run = globalStore.get(getBlockAgentRunAtomInternal(blockId));
        if (run == null || run.state !== "running") {
            continue;
        }
        const activityTs = globalStore.get(getBlockLastActivityAtomInternal(blockId));
        if (activityTs > 0 && nowTs - activityTs >= AgentWaitingIdleMs) {
            return true;
        }
    }
    return false;
}

// Aggregates the CLI providers running in a tab's blocks (first one wins) so
// tabs can render the agent logo without knowing about individual blocks.
function getTabCliProviderAtom(tabId: string, env?: CliProviderEnv): Atom<string> {
    if (tabId == null) {
        return NullAtom as Atom<string>;
    }
    let rtn = TabCliProviderAtomCache.get(tabId);
    if (rtn != null) {
        return rtn;
    }
    const tabOref = WOS.makeORef("tab", tabId);
    const tabAtom = env != null ? env.wos.getWaveObjectAtom<Tab>(tabOref) : WOS.getWaveObjectAtom<Tab>(tabOref);
    rtn = atom((get) => {
        const tab = get(tabAtom);
        for (const blockId of tab?.blockids ?? []) {
            const providerId = get(getBlockCliProviderAtomInternal(blockId));
            if (providerId != null) {
                return providerId;
            }
        }
        return null;
    });
    TabCliProviderAtomCache.set(tabId, rtn);
    return rtn;
}

// Aggregates the agent run state for a tab: a running agent wins, otherwise
// the most recently finished run is returned.
function getTabAgentRunAtom(tabId: string, env?: CliProviderEnv): Atom<AgentRunStatus | null> {
    if (tabId == null) {
        return NullAtom as Atom<AgentRunStatus | null>;
    }
    let rtn = TabAgentRunAtomCache.get(tabId);
    if (rtn != null) {
        return rtn;
    }
    const tabOref = WOS.makeORef("tab", tabId);
    const tabAtom = env != null ? env.wos.getWaveObjectAtom<Tab>(tabOref) : WOS.getWaveObjectAtom<Tab>(tabOref);
    rtn = atom((get) => {
        const tab = get(tabAtom);
        let best: AgentRunStatus | null = null;
        for (const blockId of tab?.blockids ?? []) {
            const run = get(getBlockAgentRunAtomInternal(blockId));
            if (run == null) {
                continue;
            }
            if (run.state === "running") {
                return run;
            }
            if (best == null || (run.endTs ?? 0) > (best.endTs ?? 0)) {
                best = run;
            }
        }
        return best;
    });
    TabAgentRunAtomCache.set(tabId, rtn);
    return rtn;
}

// Marks every finished run in a tab as seen (called when the user visits the
// tab while the window has focus).
function acknowledgeTabAgentRuns(tabId: string, env?: CliProviderEnv) {
    if (tabId == null) {
        return;
    }
    const tabOref = WOS.makeORef("tab", tabId);
    const tabAtom = env != null ? env.wos.getWaveObjectAtom<Tab>(tabOref) : WOS.getWaveObjectAtom<Tab>(tabOref);
    const tab = globalStore.get(tabAtom);
    for (const blockId of tab?.blockids ?? []) {
        const blockAtom = getBlockAgentRunAtomInternal(blockId);
        const run = globalStore.get(blockAtom);
        if (run != null && run.state === "done" && !run.acknowledged) {
            globalStore.set(blockAtom, { ...run, acknowledged: true });
        }
    }
}

export {
    acknowledgeTabAgentRuns,
    beginBlockAgentRun,
    finishBlockAgentRun,
    getBlockAgentRunAtom,
    getBlockCliProviderAtom,
    getBlockLastActivityAtom,
    getTabAgentRunAtom,
    getTabCliProviderAtom,
    isAnyBlockAgentWaiting,
    markBlockAgentActivity,
    parsePersistedAgentRun,
    setBlockCliProvider,
};
