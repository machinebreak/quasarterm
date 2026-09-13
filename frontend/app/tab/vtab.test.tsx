// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { VTab, VTabItem } from "./vtab";

const OriginalCss = globalThis.CSS;
const HexColorRegex = /^#([\da-f]{3}|[\da-f]{4}|[\da-f]{6}|[\da-f]{8})$/i;

function renderVTab(tab: VTabItem): string {
    return renderToStaticMarkup(
        <VTab
            tab={tab}
            active={false}
            isDragging={false}
            isReordering={false}
            onSelect={() => null}
            onDragStart={() => null}
            onDragOver={() => null}
            onDrop={() => null}
            onDragEnd={() => null}
        />
    );
}

describe("VTab badges", () => {
    beforeAll(() => {
        globalThis.CSS = {
            supports: (_property: string, value: string) => HexColorRegex.test(value),
        } as typeof CSS;
    });

    afterAll(() => {
        globalThis.CSS = OriginalCss;
    });

    it("renders shared badges and a validated flag badge", () => {
        const markup = renderVTab({
            id: "tab-1",
            name: "Build Logs",
            badges: [{ badgeid: "badge-1", icon: "bell", color: "#f59e0b", priority: 2 }],
            flagColor: "#429DFF",
        });

        expect(markup).toContain("#429DFF");
        expect(markup).toContain("#f59e0b");
        expect(markup).toContain("rounded-full");
    });

    it("ignores invalid flag colors", () => {
        const markup = renderVTab({
            id: "tab-2",
            name: "Deploy",
            badges: [{ badgeid: "badge-2", icon: "bell", color: "#4ade80", priority: 2 }],
            flagColor: "definitely-not-a-color",
        });

        expect(markup).not.toContain("definitely-not-a-color");
        expect(markup).not.toContain("fa-flag");
        expect(markup).toContain("#4ade80");
    });
});

describe("VTab project folder", () => {
    it("shows the folder name for auto-generated tab names", () => {
        const markup = renderVTab({
            id: "tab-3",
            name: "T1",
            folderName: "MoviesAndChill",
            folderPath: "C:\\Users\\Usuario\\MoviesAndChill",
        });

        expect(markup).toContain("MoviesAndChill");
        expect(markup).toContain("fa-folder");
        expect(markup).toContain('title="C:\\Users\\Usuario\\MoviesAndChill"');
    });

    it("keeps the custom name but still shows the folder icon and path", () => {
        const markup = renderVTab({
            id: "tab-4",
            name: "backend",
            folderName: "MoviesAndChill",
            folderPath: "C:\\Users\\Usuario\\MoviesAndChill",
        });

        expect(markup).toContain("backend");
        expect(markup).toContain("fa-folder");
    });

    it("does not show folder decoration for plain tabs", () => {
        const markup = renderVTab({ id: "tab-5", name: "T2" });

        expect(markup).not.toContain("fa-folder");
        expect(markup).not.toContain("title=");
    });
});

describe("VTab CLI logo", () => {
    it("shows the agent logo while a known CLI runs in the tab", () => {
        const markup = renderVTab({ id: "tab-8", name: "backend", cliProviderId: "claude" });

        expect(markup).toContain("<img");
        expect(markup).toContain('title="Claude"');
    });

    it("shows the logo next to the name for codex tabs", () => {
        const markup = renderVTab({ id: "tab-9", name: "quasar", cliProviderId: "codex" });

        expect(markup).toContain("<img");
        expect(markup).toContain('title="Codex"');
    });

    it("shows nothing when the CLI has no logo", () => {
        const markup = renderVTab({ id: "tab-10", name: "aider run", cliProviderId: "aider" });

        expect(markup).not.toContain("<img");
    });

    it("shows nothing when no CLI is running", () => {
        const markup = renderVTab({ id: "tab-11", name: "shell" });

        expect(markup).not.toContain("<img");
    });
});

describe("VTab agent finished marker", () => {
    const baseRun = {
        providerId: "codex",
        state: "done" as const,
        startTs: Date.now() - 61_000,
        endTs: Date.now(),
        exitCode: 0,
        acknowledged: false,
    };

    it("shows a green dot when the agent finished successfully", () => {
        const markup = renderVTab({ id: "tab-20", name: "backend", agentRun: baseRun });

        expect(markup).toContain('data-testid="agent-dot"');
        expect(markup).toContain("#4ade80");
    });

    it("shows a red dot when the agent failed", () => {
        const markup = renderVTab({
            id: "tab-21",
            name: "backend",
            agentRun: { ...baseRun, exitCode: 1 },
        });

        expect(markup).toContain('data-testid="agent-dot"');
        expect(markup).toContain("#f87171");
    });

    it("hides the dot once the tab was visited", () => {
        const markup = renderVTab({
            id: "tab-22",
            name: "backend",
            agentRun: { ...baseRun, acknowledged: true },
        });

        expect(markup).not.toContain('data-testid="agent-dot"');
    });

    it("shows no dot while the agent is still running", () => {
        const markup = renderVTab({
            id: "tab-23",
            name: "backend",
            agentRun: { ...baseRun, state: "running", endTs: undefined },
        });

        expect(markup).not.toContain('data-testid="agent-dot"');
    });
});
