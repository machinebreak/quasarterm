// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import type { AgentRunStatus } from "@/app/store/cliprovider";
import { providerDisplayName } from "@/app/view/accounts/providericons";
import { useEffect, useState } from "react";

// Runs shorter than this do not trigger a desktop notification (avoids noise
// from quick CLI checks like `codex --version`).
export const AgentNotifyMinDurationMs = 5000;

// A running agent whose terminal produced no output for this long is assumed
// to be waiting for user input (working agents animate a spinner, so silence
// means the CLI is sitting at a prompt).
export const AgentWaitingIdleMs = 3000;

// "45s" / "2m 14s" / "1h 5m"
export function formatAgentDuration(ms: number): string {
    const totalSec = Math.max(0, Math.round(ms / 1000));
    if (totalSec < 60) {
        return `${totalSec}s`;
    }
    const minutes = Math.floor(totalSec / 60);
    const seconds = totalSec % 60;
    if (minutes < 60) {
        return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`;
    }
    const hours = Math.floor(minutes / 60);
    const remMinutes = minutes % 60;
    return remMinutes > 0 ? `${hours}h ${remMinutes}m` : `${hours}h`;
}

// One-line status used in the tab tooltip ("Codex · corriendo hace 2m").
export function agentRunTooltip(run: AgentRunStatus, nowTs: number, waiting?: boolean): string {
    const name = providerDisplayName(run.providerId) ?? run.providerId;
    if (run.state === "running") {
        const state = waiting ? "esperando input" : "corriendo";
        return `${name} · ${state} hace ${formatAgentDuration(nowTs - run.startTs)}`;
    }
    const exit = run.exitCode == null ? "" : ` (exit ${run.exitCode})`;
    return `${name} · terminó${exit} hace ${formatAgentDuration(nowTs - (run.endTs ?? run.startTs))}`;
}

// The finished marker is visible until the tab is visited.
export function isAgentAlertVisible(run: AgentRunStatus | null | undefined): boolean {
    return run != null && run.state === "done" && !run.acknowledged;
}

// Green when the agent exited cleanly, red on failure, slate when the shell
// did not report an exit code.
export function agentAlertColor(run: AgentRunStatus): string {
    if (run.exitCode == null) {
        return "#94a3b8";
    }
    return run.exitCode === 0 ? "#4ade80" : "#f87171";
}

// Re-renders the caller every intervalMs while enabled (used to keep the
// "hace Xm" durations live in tooltips).
export function useNowTick(enabled: boolean, intervalMs = 1000): number {
    const [nowTs, setNowTs] = useState(() => Date.now());
    useEffect(() => {
        if (!enabled) {
            return;
        }
        const timer = window.setInterval(() => setNowTs(Date.now()), intervalMs);
        return () => window.clearInterval(timer);
    }, [enabled, intervalMs]);
    return nowTs;
}
