// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import type { AgentRunStatus } from "@/app/store/cliprovider";
import { describe, expect, test } from "vitest";
import { filterAgentEntries, sortAgentEntries } from "./agentcontrol";

type TestEntry = {
    blockId: string;
    tabId: string;
    tabLabel: string;
    folderName: string;
    run: AgentRunStatus;
    waiting: boolean;
};

function entry(blockId: string, run: Partial<AgentRunStatus>): TestEntry {
    return {
        blockId,
        tabId: "t1",
        tabLabel: "T1",
        folderName: "",
        waiting: false,
        run: { providerId: "claude", state: "running", startTs: 0, acknowledged: false, ...run } as AgentRunStatus,
    };
}

describe("sortAgentEntries", () => {
    test("running entries come first, newest start first", () => {
        const sorted = sortAgentEntries([
            entry("b1", { state: "done", startTs: 100, endTs: 200 }),
            entry("b2", { state: "running", startTs: 300 }),
            entry("b3", { state: "running", startTs: 500 }),
        ]);
        expect(sorted.map((e) => e.blockId)).toEqual(["b3", "b2", "b1"]);
    });

    test("done entries sorted by end time desc", () => {
        const sorted = sortAgentEntries([
            entry("b1", { state: "done", startTs: 100, endTs: 200 }),
            entry("b2", { state: "done", startTs: 100, endTs: 900 }),
            entry("b3", { state: "done", startTs: 100, endTs: 500 }),
        ]);
        expect(sorted.map((e) => e.blockId)).toEqual(["b2", "b3", "b1"]);
    });

    test("done entries are capped to the most recent ones", () => {
        const entries: TestEntry[] = [];
        for (let i = 0; i < 12; i++) {
            entries.push(entry(`b${i}`, { state: "done", startTs: i, endTs: i * 10 }));
        }
        const sorted = sortAgentEntries(entries as any);
        expect(sorted.length).toBe(8);
        expect(sorted[0].blockId).toBe("b11");
        expect(sorted[7].blockId).toBe("b4");
    });

    test("empty input", () => {
        expect(sortAgentEntries([])).toEqual([]);
    });

    test("waiting runs bubble above working runs", () => {
        const working = entry("b1", { state: "running", startTs: 900 });
        const waiting = { ...entry("b2", { state: "running", startTs: 100 }), waiting: true };
        const sorted = sortAgentEntries([working, waiting]);
        expect(sorted.map((e) => e.blockId)).toEqual(["b2", "b1"]);
    });

    test("interrupted runs sit between running and done", () => {
        const running = entry("r1", { state: "running", startTs: 500 });
        const interrupted = { ...entry("i1", { state: "done", startTs: 400 }), interrupted: true };
        const done = entry("d1", { state: "done", startTs: 100, endTs: 300 });
        const sorted = sortAgentEntries([done, interrupted, running]);
        expect(sorted.map((e) => e.blockId)).toEqual(["r1", "i1", "d1"]);
    });
});

describe("filterAgentEntries", () => {
    test("filters by folder name, with tab label fallback", () => {
        const a = { ...entry("a", { state: "running", startTs: 1 }), folderName: "proj-a" };
        const b = { ...entry("b", { state: "running", startTs: 2 }), folderName: "" };
        const c = { ...entry("c", { state: "running", startTs: 3 }), folderName: "proj-b" };
        expect(filterAgentEntries([a, b, c], "proj-a").map((e) => e.blockId)).toEqual(["a"]);
        expect(filterAgentEntries([a, b, c], "T1").map((e) => e.blockId)).toEqual(["b"]);
    });

    test("null filter returns everything", () => {
        const a = { ...entry("a", { state: "running", startTs: 1 }), folderName: "proj-a" };
        expect(filterAgentEntries([a], null).length).toBe(1);
    });
});
