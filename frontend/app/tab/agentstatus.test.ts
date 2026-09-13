// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import type { AgentRunStatus } from "@/app/store/cliprovider";
import { describe, expect, it } from "vitest";
import { agentAlertColor, agentRunTooltip, formatAgentDuration, isAgentAlertVisible } from "./agentstatus";

const baseRun: AgentRunStatus = {
    providerId: "codex",
    state: "done",
    startTs: 1_000_000,
    endTs: 1_061_000,
    exitCode: 0,
    acknowledged: false,
};

describe("formatAgentDuration", () => {
    it("formats seconds below a minute", () => {
        expect(formatAgentDuration(45_000)).toBe("45s");
    });

    it("formats minutes and seconds", () => {
        expect(formatAgentDuration(134_000)).toBe("2m 14s");
    });

    it("drops seconds on whole minutes", () => {
        expect(formatAgentDuration(120_000)).toBe("2m");
    });

    it("formats hours and minutes", () => {
        expect(formatAgentDuration(3_900_000)).toBe("1h 5m");
    });
});

describe("agentAlertColor", () => {
    it("is green on clean exit, red on failure and slate when unknown", () => {
        expect(agentAlertColor({ ...baseRun, exitCode: 0 })).toBe("#4ade80");
        expect(agentAlertColor({ ...baseRun, exitCode: 1 })).toBe("#f87171");
        expect(agentAlertColor({ ...baseRun, exitCode: null })).toBe("#94a3b8");
    });
});

describe("isAgentAlertVisible", () => {
    it("is visible only for unacknowledged finished runs", () => {
        expect(isAgentAlertVisible(baseRun)).toBe(true);
        expect(isAgentAlertVisible({ ...baseRun, acknowledged: true })).toBe(false);
        expect(isAgentAlertVisible({ ...baseRun, state: "running", endTs: undefined })).toBe(false);
        expect(isAgentAlertVisible(null)).toBe(false);
    });
});

describe("agentRunTooltip", () => {
    it("shows live duration while running", () => {
        const run: AgentRunStatus = { ...baseRun, state: "running", endTs: undefined };
        expect(agentRunTooltip(run, run.startTs + 125_000)).toBe("Codex · corriendo hace 2m 5s");
    });

    it("shows exit code and elapsed time when done", () => {
        expect(agentRunTooltip(baseRun, baseRun.endTs + 60_000)).toBe("Codex · terminó (exit 0) hace 1m");
    });

    it("omits the exit code when the shell did not report one", () => {
        expect(agentRunTooltip({ ...baseRun, exitCode: null }, baseRun.endTs)).toBe("Codex · terminó hace 0s");
    });
});
