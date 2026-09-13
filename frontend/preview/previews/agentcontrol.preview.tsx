// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { Block } from "@/app/block/block";
import {
    AgentLastRunMetaKey,
    getBlockAgentRunAtom,
    getBlockLastActivityAtom,
    type AgentRunStatus,
} from "@/app/store/cliprovider";
import { globalStore } from "@/app/store/jotaiStore";
import { useWaveEnv, WaveEnv, WaveEnvContext } from "@/app/waveenv/waveenv";
import { makeMockNodeModel } from "@/preview/mock/mock-node-model";
import { applyMockEnvOverrides, MockWaveEnv } from "@/preview/mock/mockwaveenv";
import { atom } from "jotai";
import React from "react";

const AgentBlockId = "preview-agentcontrol-block";
const AgentWorkspaceId = "preview-agents-workspace";
const TabOrion = "preview-agents-tab-orion";
const TabHaxbot = "preview-agents-tab-haxbot";
const TabMovie = "preview-agents-tab-movie";

// Terminal blocks holding the (mock) agent runs.
const BlockClaudeRunning = "preview-agents-block-claude-running";
const BlockCodexDone = "preview-agents-block-codex-done";
const BlockCodexRunning = "preview-agents-block-codex-running";
const BlockClaudeFailed = "preview-agents-block-claude-failed";
const BlockClaudeOld = "preview-agents-block-claude-old";
const BlockClaudeInterrupted = "preview-agents-block-claude-interrupted";

function makeTab(oid: string, name: string, folder: string, blockids: string[]): Tab {
    return {
        otype: "tab",
        oid,
        version: 1,
        name,
        blockids,
        meta: { "cmd:cwd": folder },
    } as Tab;
}

function makeBlock(oid: string): Block {
    return { otype: "block", oid, version: 1, meta: { view: "term" } } as Block;
}

function makeQuotaAccount(
    id: string,
    provider: string,
    label: string,
    metricName: string,
    remainingpercent: number,
    resetInMinutes: number
): Account {
    return {
        id,
        provider,
        label,
        isactive: true,
        createdat: 0,
        quota: {
            updatedat: Date.now(),
            allowed: true,
            limitreached: false,
            metrics: [
                {
                    name: metricName,
                    remainingpercent,
                    resetat: Math.floor(Date.now() / 1000) + resetInMinutes * 60,
                },
            ],
        },
    } as Account;
}

const MockQuotaAccounts: Record<string, Account[]> = {
    claude: [makeQuotaAccount("preview-claude-1", "claude", "Max", "5h", 82, 180)],
    codex: [makeQuotaAccount("preview-codex-1", "codex", "Plus", "Weekly", 31, 2 * 24 * 60)],
};

function seedAgentRuns() {
    const now = Date.now();
    const seed = (blockId: string, run: AgentRunStatus) => globalStore.set(getBlockAgentRunAtom(blockId) as any, run);
    seed(BlockClaudeRunning, {
        providerId: "claude",
        state: "running",
        startTs: now - 12 * 60_000 - 4000,
        acknowledged: false,
    });
    seed(BlockCodexRunning, {
        providerId: "codex",
        state: "running",
        startTs: now - 2 * 60_000 - 12_000,
        acknowledged: false,
    });
    seed(BlockCodexDone, {
        providerId: "codex",
        state: "done",
        startTs: now - 9 * 60_000,
        endTs: now - 3 * 60_000,
        exitCode: 0,
        acknowledged: false,
    });
    seed(BlockClaudeFailed, {
        providerId: "claude",
        state: "done",
        startTs: now - 40 * 60_000,
        endTs: now - 22 * 60_000,
        exitCode: 1,
        acknowledged: true,
    });
    seed(BlockClaudeOld, {
        providerId: "claude",
        state: "done",
        startTs: now - 80 * 60_000,
        endTs: now - 61 * 60_000,
        exitCode: 0,
        acknowledged: true,
    });
    // activity feed: claude just printed (working), codex has been quiet for a
    // while -> shown as waiting for input
    globalStore.set(getBlockLastActivityAtom(BlockClaudeRunning) as any, now - 400);
    globalStore.set(getBlockLastActivityAtom(BlockCodexRunning) as any, now - 45_000);
}

function makeAgentControlEnv(baseEnv: WaveEnv): MockWaveEnv {
    const mockWaveObjs: Record<string, WaveObj> = {
        [`workspace:${AgentWorkspaceId}`]: {
            otype: "workspace",
            oid: AgentWorkspaceId,
            version: 1,
            name: "Preview Workspace",
            tabids: [TabOrion, TabHaxbot, TabMovie],
            activetabid: TabOrion,
            meta: {},
        } as Workspace,
        [`tab:${TabOrion}`]: makeTab(TabOrion, "T1", "C:/Users/Usuario/Projects/orion-term", [
            BlockClaudeRunning,
            BlockCodexDone,
        ]),
        [`tab:${TabHaxbot}`]: makeTab(TabHaxbot, "HaxBot", "C:/Users/Usuario/Projects/haxbot", [
            BlockCodexRunning,
            BlockClaudeFailed,
        ]),
        [`tab:${TabMovie}`]: makeTab(TabMovie, "T7", "C:/Users/Usuario/Projects/movieapp", [
            BlockClaudeOld,
            BlockClaudeInterrupted,
        ]),
        [`block:${BlockClaudeRunning}`]: makeBlock(BlockClaudeRunning),
        [`block:${BlockCodexDone}`]: makeBlock(BlockCodexDone),
        [`block:${BlockCodexRunning}`]: makeBlock(BlockCodexRunning),
        [`block:${BlockClaudeFailed}`]: makeBlock(BlockClaudeFailed),
        [`block:${BlockClaudeOld}`]: makeBlock(BlockClaudeOld),
        [`block:${BlockClaudeInterrupted}`]: {
            otype: "block",
            oid: BlockClaudeInterrupted,
            version: 1,
            meta: {
                view: "term",
                [AgentLastRunMetaKey]: {
                    providerId: "claude",
                    state: "running",
                    startTs: Date.now() - 3 * 3600_000,
                },
            },
        } as Block,
        [`block:${AgentBlockId}`]: {
            otype: "block",
            oid: AgentBlockId,
            version: 1,
            meta: { view: "agentcontrol" },
        } as Block,
    };
    return applyMockEnvOverrides(baseEnv, {
        atoms: {
            workspaceId: atom(AgentWorkspaceId) as any,
            staticTabId: atom(TabOrion) as any,
        },
        mockWaveObjs,
        rpc: {
            ListAccountsCommand: (_client: unknown, data: { provider: string }) =>
                Promise.resolve(MockQuotaAccounts[data.provider] ?? []),
            RefreshAccountQuotaCommand: () => Promise.resolve(null),
            ListAccountProvidersCommand: () =>
                Promise.resolve([
                    { id: "claude", name: "Claude", resumeargs: "--resume", icon: "claude" },
                    { id: "codex", name: "Codex", resumeargs: "resume", icon: "codex" },
                ] as ProviderInfo[]),
        },
    });
}

export default function AgentControlPreview() {
    const baseEnv = useWaveEnv();
    const envRef = React.useRef<MockWaveEnv>(null);
    if (envRef.current == null) {
        envRef.current = makeAgentControlEnv(baseEnv);
    }

    React.useEffect(() => {
        seedAgentRuns();
        // the "working" agent keeps emitting output (marks activity every
        // second) while the other one stays quiet -> waiting for input
        const timer = window.setInterval(() => {
            globalStore.set(getBlockLastActivityAtom(BlockClaudeRunning) as any, Date.now());
        }, 1000);
        return () => window.clearInterval(timer);
    }, []);

    const nodeModel = React.useMemo(
        () =>
            makeMockNodeModel({
                nodeId: "preview-agentcontrol-node",
                blockId: AgentBlockId,
                innerRect: { width: "470px", height: "640px" },
                numLeafs: 1,
            }),
        []
    );

    return (
        <WaveEnvContext.Provider value={envRef.current}>
            <div className="flex w-full max-w-[1100px] flex-col gap-2 px-6 py-6">
                <div className="text-xs text-muted font-mono">
                    agentcontrol block (mock: 1 corriendo, 1 esperando input, 1 interrumpido con Reanudar, finished +
                    failed, quotas, filtro por proyecto)
                </div>
                <div className="rounded-md border border-border bg-panel p-4">
                    <div className="h-[640px] w-[470px]">
                        <Block preview={false} nodeModel={nodeModel} />
                    </div>
                </div>
            </div>
        </WaveEnvContext.Provider>
    );
}
