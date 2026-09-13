// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import {
    beginBlockAgentRun,
    finishBlockAgentRun,
    getBlockAgentRunAtom,
    getBlockLastActivityAtom,
    isAnyBlockAgentWaiting,
    parsePersistedAgentRun,
} from "./cliprovider";
import { globalStore } from "./jotaiStore";

describe("agent runs", () => {
    it("records the start and finish of an agent run with exit code", () => {
        const blockId = "test-agent-block-1";
        beginBlockAgentRun(blockId, "codex");
        const running = globalStore.get(getBlockAgentRunAtom(blockId));
        expect(running?.state).toBe("running");
        expect(running?.providerId).toBe("codex");
        expect(running?.acknowledged).toBe(false);

        const finished = finishBlockAgentRun(blockId, 0);
        expect(finished?.state).toBe("done");
        expect(finished?.exitCode).toBe(0);
        expect(finished?.endTs).toBeGreaterThanOrEqual(finished!.startTs);
        expect(finished?.acknowledged).toBe(false);

        const stored = globalStore.get(getBlockAgentRunAtom(blockId));
        expect(stored?.state).toBe("done");
    });

    it("keeps the start time when the same provider is already running", () => {
        const blockId = "test-agent-block-2";
        beginBlockAgentRun(blockId, "claude");
        const first = globalStore.get(getBlockAgentRunAtom(blockId));
        beginBlockAgentRun(blockId, "claude");
        const second = globalStore.get(getBlockAgentRunAtom(blockId));
        expect(second?.startTs).toBe(first?.startTs);
    });

    it("replaces the previous run when a different provider starts", () => {
        const blockId = "test-agent-block-3";
        beginBlockAgentRun(blockId, "codex");
        finishBlockAgentRun(blockId, 0);
        beginBlockAgentRun(blockId, "claude");
        const run = globalStore.get(getBlockAgentRunAtom(blockId));
        expect(run?.providerId).toBe("claude");
        expect(run?.state).toBe("running");
    });

    it("returns null when finishing without a running agent", () => {
        expect(finishBlockAgentRun("test-agent-block-4", 1)).toBeNull();
        beginBlockAgentRun("test-agent-block-4", "codex");
        finishBlockAgentRun("test-agent-block-4", 0);
        expect(finishBlockAgentRun("test-agent-block-4", 0)).toBeNull();
    });
});

describe("parsePersistedAgentRun", () => {
    it("parses a valid persisted run object", () => {
        const parsed = parsePersistedAgentRun({ providerId: "claude", state: "running", startTs: 123 });
        expect(parsed?.providerId).toBe("claude");
        expect(parsed?.state).toBe("running");
        expect(parsed?.startTs).toBe(123);
    });

    it("parses a JSON string value", () => {
        const parsed = parsePersistedAgentRun(
            JSON.stringify({ providerId: "codex", state: "done", startTs: 5, endTs: 9 })
        );
        expect(parsed?.state).toBe("done");
        expect(parsed?.endTs).toBe(9);
    });

    it("returns null for malformed values", () => {
        expect(parsePersistedAgentRun(null)).toBeNull();
        expect(parsePersistedAgentRun("not json")).toBeNull();
        expect(parsePersistedAgentRun({ state: "running" })).toBeNull();
        expect(parsePersistedAgentRun({ providerId: "claude", startTs: 0 })).toBeNull();
    });
});

describe("isAnyBlockAgentWaiting", () => {
    it("is true when a running agent has been idle", () => {
        const blockId = "test-agent-block-waiting-1";
        beginBlockAgentRun(blockId, "claude");
        globalStore.set(getBlockLastActivityAtom(blockId) as any, Date.now() - 60_000);
        expect(isAnyBlockAgentWaiting([blockId], Date.now())).toBe(true);
    });

    it("is false while the agent is producing output", () => {
        const blockId = "test-agent-block-waiting-2";
        beginBlockAgentRun(blockId, "claude");
        globalStore.set(getBlockLastActivityAtom(blockId) as any, Date.now());
        expect(isAnyBlockAgentWaiting([blockId], Date.now())).toBe(false);
    });

    it("is false for finished or unknown blocks", () => {
        expect(isAnyBlockAgentWaiting(["nonexistent-block"], Date.now())).toBe(false);
        const blockId = "test-agent-block-waiting-3";
        beginBlockAgentRun(blockId, "codex");
        finishBlockAgentRun(blockId, 0);
        globalStore.set(getBlockLastActivityAtom(blockId) as any, Date.now() - 60_000);
        expect(isAnyBlockAgentWaiting([blockId], Date.now())).toBe(false);
    });
});
