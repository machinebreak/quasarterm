// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { globalStore } from "@/app/store/jotaiStore";
import { beforeEach, describe, expect, it } from "vitest";
import { deleteCanvasModel, getCanvasModel, type CanvasWindowState } from "./canvas-model";

function overlapRatio(a: CanvasWindowState, b: CanvasWindowState): number {
    const ow = Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x);
    const oh = Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y);
    if (ow <= 0 || oh <= 0) {
        return 0;
    }
    return (ow * oh) / (a.w * a.h);
}

describe("canvas model placement", () => {
    beforeEach(() => {
        deleteCanvasModel("test-tab");
    });

    it("reconcile reports newly added blocks and prunes removed ones", () => {
        const model = getCanvasModel("test-tab");
        model.setBounds({ width: 1600, height: 900 });
        expect(model.reconcile(["block-a"])).toEqual(["block-a"]);
        expect(model.reconcile(["block-a", "block-b"])).toEqual(["block-b"]);
        expect(model.reconcile(["block-a"])).toEqual([]);
        expect(model.getWindow("block-b")).toBeNull();
        expect(model.getWindow("block-a")).not.toBeNull();
    });

    it("places new windows in free slots instead of stacking them", () => {
        const model = getCanvasModel("test-tab");
        model.setBounds({ width: 1600, height: 900 });
        model.reconcile(["block-a"]);
        model.reconcile(["block-a", "block-b"]);
        model.reconcile(["block-a", "block-b", "block-c"]);
        model.reconcile(["block-a", "block-b", "block-c", "block-d"]);
        const wins = globalStore.get(model.windowsAtom);
        const rects = Object.values(wins);
        expect(rects.length).toBe(4);
        for (let i = 0; i < rects.length; i++) {
            for (let j = i + 1; j < rects.length; j++) {
                expect(overlapRatio(rects[i], rects[j])).toBeLessThanOrEqual(0.18);
            }
        }
    });

    it("cascades next to the focused window when the canvas is crowded", () => {
        const model = getCanvasModel("test-tab");
        model.setBounds({ width: 1000, height: 640 });
        model.reconcile(["w1", "w2", "w3", "w4"]);
        model.focusBlock("w2");
        const w2 = model.getWindow("w2");
        model.reconcile(["w1", "w2", "w3", "w4", "w5"]);
        const w5 = model.getWindow("w5");
        expect(w5).not.toBeNull();
        expect(Math.abs(w5.x - (w2.x + 46))).toBeLessThanOrEqual(2);
        expect(Math.abs(w5.y - (w2.y + 40))).toBeLessThanOrEqual(2);
    });

    it("tidy keeps windows inside the canvas with sane sizes", () => {
        const model = getCanvasModel("test-tab");
        model.setBounds({ width: 1200, height: 700 });
        model.reconcile(["t1", "t2", "t3"]);
        model.tidy();
        const wins = globalStore.get(model.windowsAtom);
        for (const win of Object.values(wins)) {
            expect(win.y).toBeGreaterThanOrEqual(0);
            expect(win.x).toBeGreaterThanOrEqual(-1000);
            expect(win.w).toBeGreaterThanOrEqual(320);
            expect(win.h).toBeGreaterThanOrEqual(200);
            expect(win.w).toBeLessThanOrEqual(1216);
            expect(win.h).toBeLessThanOrEqual(716);
        }
    });

    it("keeps existing window state on reconcile", () => {
        const model = getCanvasModel("test-tab");
        model.setBounds({ width: 1600, height: 900 });
        // seed meta for block-a by simulating a persisted window through the store
        const cur = globalStore.get(model.windowsAtom);
        globalStore.set(model.windowsAtom, {
            ...cur,
            "block-a": { x: 100, y: 120, w: 500, h: 360, z: 1, maximized: false },
        });
        model.reconcile(["block-a"]);
        const win = model.getWindow("block-a");
        expect(win.x).toBe(100);
        expect(win.y).toBe(120);
        expect(win.w).toBe(500);
        expect(win.h).toBe(360);
    });
});
