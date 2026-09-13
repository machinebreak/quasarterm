// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { makeMockWaveEnv } from "@/preview/mock/mockwaveenv";
import { describe, expect, it } from "vitest";
import { findWidgetBlockInCurrentTab } from "./widgets";

describe("findWidgetBlockInCurrentTab", () => {
    it("finds an already-open widget block by view", () => {
        const env = makeMockWaveEnv();
        const blockId = findWidgetBlockInCurrentTab("sysinfo", env);
        expect(blockId).not.toBeNull();
    });

    it("returns null when the widget is not open in the current tab", () => {
        const env = makeMockWaveEnv();
        expect(findWidgetBlockInCurrentTab("agentcontrol", env)).toBeNull();
    });
});
