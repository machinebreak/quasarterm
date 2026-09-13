// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { Block } from "@/app/block/block";
import { useWaveEnv, WaveEnv, WaveEnvContext } from "@/app/waveenv/waveenv";
import * as React from "react";
import { makeMockNodeModel } from "../mock/mock-node-model";
import { applyMockEnvOverrides } from "../mock/mockwaveenv";

const AccountsBlockId = "preview-accounts-block";

const MockProviders: ProviderInfo[] = [
    {
        id: "codex",
        name: "Codex",
        description: "OpenAI Codex CLI accounts",
        clicommand: "codex",
        resumeargs: "resume",
        icon: "codex",
        order: 1,
        available: true,
        wakesupported: true,
    },
    {
        id: "claude",
        name: "Claude",
        description: "Anthropic Claude Code accounts",
        clicommand: "claude",
        resumeargs: "--resume",
        icon: "claude",
        order: 2,
        available: true,
        wakesupported: true,
    },
    {
        id: "cursor",
        name: "Cursor",
        description: "Cursor editor accounts",
        clicommand: "cursor",
        resumeargs: "",
        icon: "cursor",
        order: 3,
        available: true,
    },
];

const MockWakeTasks: WakeTask[] = [
    {
        id: "wake-1",
        provider: "codex",
        accountid: "cx-2",
        intervalminutes: 240,
        model: "gpt-5-codex",
        enabled: true,
        createdat: 0,
        lastrun: Date.now() - 42 * 60000,
        laststatus: "ok",
        lastdurationms: 9400,
    } as WakeTask,
    {
        id: "wake-2",
        provider: "codex",
        accountid: "cx-3",
        intervalminutes: 60,
        enabled: false,
        createdat: 0,
        lastrun: Date.now() - 3 * 3600_000,
        laststatus: "error",
        lasterror: "exit status 1: Error: not logged in",
        lastdurationms: 2100,
    } as WakeTask,
];

const MockWakeRuns: WakeRun[] = [
    {
        taskid: "wake-1",
        provider: "codex",
        accountid: "cx-2",
        startedat: Date.now() - 42 * 60000,
        durationms: 9400,
        status: "ok",
        output: "OK",
    } as WakeRun,
    {
        taskid: "wake-1",
        provider: "codex",
        accountid: "cx-2",
        startedat: Date.now() - 4 * 3600_000,
        durationms: 11200,
        status: "ok",
        output: "OK",
    } as WakeRun,
    {
        taskid: "wake-2",
        provider: "codex",
        accountid: "cx-3",
        startedat: Date.now() - 3 * 3600_000,
        durationms: 2100,
        status: "error",
        error: "exit status 1: Error: not logged in",
    } as WakeRun,
];

function quota(metrics: QuotaMetric[], planType: string): Quota {
    return {
        updatedat: Date.now() - 4 * 60000,
        plantype: planType,
        allowed: true,
        limitreached: false,
        metrics,
    } as Quota;
}

const MockAccounts: Account[] = [
    {
        id: "cx-1",
        provider: "codex",
        label: "Max personal",
        email: "max@example.com",
        plantype: "pro",
        remoteid: "r-1",
        tags: ["personal", "5h"],
        createdat: 0,
        lastused: Date.now() - 3600_000,
        isactive: true,
        quota: quota(
            [
                { name: "5h", remainingpercent: 72, resetat: Math.floor(Date.now() / 1000) + 3 * 3600 },
                { name: "Weekly", remainingpercent: 41, resetat: Math.floor(Date.now() / 1000) + 4 * 86400 },
            ],
            "pro"
        ),
        quotaerror: "",
    } as Account,
    {
        id: "cx-2",
        provider: "codex",
        label: "Work bench",
        email: "bench@corp.dev",
        plantype: "team",
        remoteid: "r-2",
        tags: ["work"],
        createdat: 0,
        lastused: Date.now() - 26 * 3600_000,
        isactive: false,
        quota: quota(
            [
                { name: "5h", remainingpercent: 12, resetat: Math.floor(Date.now() / 1000) + 1800 },
                { name: "Weekly", remainingpercent: 8, resetat: Math.floor(Date.now() / 1000) + 2 * 86400 },
            ],
            "team"
        ),
        quotaerror: "",
    } as Account,
    {
        id: "cx-3",
        provider: "codex",
        label: "Old trial",
        email: "trial@example.com",
        plantype: "free",
        remoteid: "r-3",
        tags: [],
        createdat: 0,
        lastused: 0,
        isactive: false,
        quota: null as Quota,
        quotaerror: "quota refresh failed: token expired",
    } as Account,
    {
        id: "cl-1",
        provider: "claude",
        label: "Claude Max",
        email: "me@claude.ai",
        plantype: "max",
        remoteid: "r-4",
        tags: ["personal"],
        createdat: 0,
        lastused: Date.now() - 7200_000,
        isactive: true,
        quota: quota(
            [
                { name: "Session", remainingpercent: 63, resetat: Math.floor(Date.now() / 1000) + 4 * 3600 },
                { name: "Weekly", remainingpercent: 55, resetat: Math.floor(Date.now() / 1000) + 3 * 86400 },
            ],
            "max"
        ),
        quotaerror: "",
    } as Account,
];

const MockInstances: Instance[] = [
    {
        id: "inst-1",
        provider: "codex",
        accountid: "cx-2",
        name: "Work bench #1",
        profiledir: "C:\\Users\\demo\\AppData\\Local\\quasar-dev\\Data\\accounts\\instances\\codex\\inst-1",
        status: "running",
        blockid: "preview-block-xyz",
        createdat: 0,
        dir: "C:\\Users\\demo\\codex-work",
        args: ["--model", "gpt-5-codex"],
    } as Instance,
    {
        id: "inst-2",
        provider: "codex",
        accountid: "cx-1",
        name: "Max personal #1",
        profiledir: "C:\\Users\\demo\\AppData\\Local\\quota-dev\\accounts\\instances\\codex\\inst-2",
        status: "stopped",
        createdat: 0,
    } as Instance,
];

function makeAccountsEnv(baseEnv: WaveEnv): WaveEnv {
    return applyMockEnvOverrides(baseEnv, {
        rpc: {
            ListAccountProvidersCommand: () => Promise.resolve(MockProviders),
            ListAccountsCommand: (_client: unknown, data: CommandListAccountsData) =>
                Promise.resolve(MockAccounts.filter((account) => account.provider === data.provider)),
            ListAccountInstancesCommand: (_client: unknown, provider: string) =>
                Promise.resolve(MockInstances.filter((instance) => instance.provider === provider)),
            GetAccountAutoSwitchCommand: () => Promise.resolve(true),
            RefreshAccountQuotaCommand: (_client: unknown, data: CommandRefreshAccountQuotaData) =>
                Promise.resolve(MockAccounts.find((account) => account.id === data.accountid) ?? null),
            SetAccountTagsCommand: () => Promise.resolve(undefined),
            SetAccountLabelCommand: () => Promise.resolve(undefined),
            DeleteAccountCommand: () => Promise.resolve(undefined),
            ExportAccountCommand: () =>
                Promise.resolve(JSON.stringify({ version: 1, exported: Date.now(), accounts: [] })),
            SetAccountAutoSwitchCommand: () => Promise.resolve(undefined),
            ListWakeTasksCommand: (_client: unknown, provider: string) =>
                Promise.resolve(MockWakeTasks.filter((task) => task.provider === provider)),
            ListWakeRunsCommand: (_client: unknown, data: CommandListWakeRunsData) =>
                Promise.resolve(MockWakeRuns.filter((run) => run.taskid === data.taskid)),
            CreateWakeTaskCommand: () => Promise.resolve(MockWakeTasks[0]),
            UpdateWakeTaskCommand: () => Promise.resolve(MockWakeTasks[0]),
            DeleteWakeTaskCommand: () => Promise.resolve(undefined),
            RunWakeTaskCommand: (_client: unknown, taskID: string) =>
                Promise.resolve(MockWakeRuns.find((run) => run.taskid === taskID && run.status === "ok")),
            GetCodexApiStatusCommand: () =>
                Promise.resolve({
                    enabled: true,
                    running: true,
                    port: 8318,
                    baseurl: "http://127.0.0.1:8318/v1",
                    apikey: "",
                    error: "",
                } as LocalAPIStatus),
            SetCodexApiSettingsCommand: () =>
                Promise.resolve({
                    enabled: true,
                    running: true,
                    port: 8318,
                    baseurl: "http://127.0.0.1:8318/v1",
                    apikey: "",
                    error: "",
                } as LocalAPIStatus),
        },
        mockWaveObjs: {
            [`block:${AccountsBlockId}`]: {
                otype: "block",
                oid: AccountsBlockId,
                version: 1,
                meta: { view: "accounts" },
            },
        },
    });
}

export default function AccountsPreview() {
    const baseEnv = useWaveEnv();
    const envRef = React.useRef<WaveEnv>(null);
    if (envRef.current == null) {
        envRef.current = makeAccountsEnv(baseEnv);
    }
    const nodeModel = React.useMemo(
        () =>
            makeMockNodeModel({
                nodeId: "preview-accounts-node",
                blockId: AccountsBlockId,
                innerRect: { width: "1120px", height: "680px" },
                numLeafs: 1,
            }),
        []
    );
    return (
        <WaveEnvContext.Provider value={envRef.current}>
            <div className="flex w-full max-w-[1160px] flex-col gap-2 px-6 py-6">
                <div className="text-xs text-muted font-mono">
                    accounts block (mock RPC — dashboard, search, tags, batch bar, instances)
                </div>
                <div className="rounded-md border border-border bg-panel p-4">
                    <div className="h-[700px]">
                        <Block preview={false} nodeModel={nodeModel} />
                    </div>
                </div>
            </div>
        </WaveEnvContext.Provider>
    );
}
