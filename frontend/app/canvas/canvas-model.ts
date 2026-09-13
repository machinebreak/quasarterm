// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

// Canvas ("Deska") mode: free-floating, movable, resizable block windows.
// The model keeps window geometry in a per-tab store (persisted to block meta
// canvas:x/y/w/h) and builds lightweight NodeModels so the existing Block
// component tree can render inside each floating window.

import { globalStore } from "@/app/store/jotaiStore";
import * as services from "@/app/store/services";
import { uxCloseBlock } from "@/app/store/keymodel";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { getBlockMetaKeyAtom } from "@/store/global";
import {
    LayoutTreeActionType,
    getLayoutModelForStaticTab,
    type LayoutTreeFocusNodeAction,
    type NodeModel,
} from "@/layout/index";
import * as WOS from "@/store/wos";
import { boundNumber, isBlank } from "@/util/util";
import { atom, type PrimitiveAtom } from "jotai";
import { createRef, type CSSProperties, type RefObject } from "react";

export type CanvasWindowState = {
    x: number;
    y: number;
    w: number;
    h: number;
    z: number;
    maximized: boolean;
};

export type CanvasBounds = {
    width: number;
    height: number;
};

const DefaultWinW = 660;
const DefaultWinH = 420;
const MinWinW = 320;
const MinWinH = 200;
const MinVisibleX = 180; // px of a window that must stay inside the canvas horizontally
const MinVisibleY = 44; // px of the title bar that must stay inside the canvas vertically
const CascadeStartX = 64;
const CascadeStartY = 52;
const CascadeStepX = 46;
const CascadeStepY = 40;
const CascadeWrap = 8;
const MaxExtraZ = 10000;

function clampNum(val: number, min: number, max: number): number {
    return Math.min(Math.max(val, min), max);
}

function cascadeSlot(index: number): { x: number; y: number } {
    const i = index % CascadeWrap;
    return { x: CascadeStartX + i * CascadeStepX, y: CascadeStartY + i * CascadeStepY };
}

class CanvasModel {
    tabId: string;
    windowsAtom: PrimitiveAtom<Record<string, CanvasWindowState>>;
    focusedBlockAtom: PrimitiveAtom<string | null>;
    interactingBlockAtom: PrimitiveAtom<string | null>;
    ephemeralBlockAtom: PrimitiveAtom<string | null>;

    private bounds: CanvasBounds;
    private zCounter: number;
    private nodeModels: Map<string, NodeModel>;
    private containerRef: RefObject<HTMLDivElement>;

    constructor(tabId: string) {
        this.tabId = tabId;
        this.windowsAtom = atom<Record<string, CanvasWindowState>>({});
        this.focusedBlockAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;
        this.interactingBlockAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;
        this.ephemeralBlockAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;
        this.bounds = { width: 1280, height: 720 };
        this.zCounter = 10;
        this.nodeModels = new Map();
        this.containerRef = createRef<HTMLDivElement>();
    }

    setContainerRef(ref: RefObject<HTMLDivElement>): void {
        if (ref?.current != null) {
            this.containerRef = ref;
        }
    }

    setBounds(bounds: CanvasBounds): void {
        if (bounds.width > 0 && bounds.height > 0) {
            this.bounds = bounds;
        }
    }

    getBounds(): CanvasBounds {
        return this.bounds;
    }

    getWindow(blockId: string): CanvasWindowState | null {
        return globalStore.get(this.windowsAtom)[blockId] ?? null;
    }

    private nextZ(): number {
        this.zCounter = this.zCounter + 1;
        return this.zCounter;
    }

    // Add windows for new blocks and drop windows for blocks that no longer exist.
    // Returns the ids of newly added windows so callers can focus the freshest one.
    reconcile(blockIds: string[]): string[] {
        const cur = globalStore.get(this.windowsAtom);
        const next: Record<string, CanvasWindowState> = {};
        const added: string[] = [];
        let changed = false;
        const seen = new Set(blockIds);
        blockIds.forEach((blockId, idx) => {
            if (cur[blockId] != null) {
                next[blockId] = cur[blockId];
                return;
            }
            next[blockId] = this.makeInitialWindow(blockId, idx, { ...cur, ...next });
            added.push(blockId);
            changed = true;
        });
        for (const blockId of Object.keys(cur)) {
            if (seen.has(blockId)) {
                continue;
            }
            changed = true;
            this.nodeModels.delete(blockId);
            const focused = globalStore.get(this.focusedBlockAtom);
            if (focused === blockId) {
                globalStore.set(this.focusedBlockAtom, null);
            }
            const ephemeral = globalStore.get(this.ephemeralBlockAtom);
            if (ephemeral === blockId) {
                globalStore.set(this.ephemeralBlockAtom, null);
            }
        }
        if (changed) {
            globalStore.set(this.windowsAtom, next);
        }
        return added;
    }

    private makeInitialWindow(
        blockId: string,
        index: number,
        existing: Record<string, CanvasWindowState>
    ): CanvasWindowState {
        const metaX = globalStore.get(getBlockMetaKeyAtom(blockId, "canvas:x"));
        const metaY = globalStore.get(getBlockMetaKeyAtom(blockId, "canvas:y"));
        const metaW = globalStore.get(getBlockMetaKeyAtom(blockId, "canvas:w"));
        const metaH = globalStore.get(getBlockMetaKeyAtom(blockId, "canvas:h"));
        const bounds = this.bounds;
        let w = clampNum(boundNumber(metaW, 0, 100000) ?? DefaultWinW, MinWinW, Math.max(MinWinW, bounds.width - 16));
        let h = clampNum(boundNumber(metaH, 0, 100000) ?? DefaultWinH, MinWinH, Math.max(MinWinH, bounds.height - 16));
        let x: number;
        let y: number;
        if (metaX != null && metaY != null) {
            ({ x, y } = this.clampPos(metaX, metaY, w, h));
        } else {
            const slot = this.findFreeSlot(w, h, existing, index);
            ({ x, y } = this.clampPos(slot.x, slot.y, w, h));
        }
        return { x, y, w, h, z: this.nextZ(), maximized: false };
    }

    // Pick a spawn position that is not hidden under existing windows: prefer a
    // clearly free slot, then fall back to a desktop-style cascade next to the
    // focused window, and finally the classic cascade.
    private findFreeSlot(
        w: number,
        h: number,
        existing: Record<string, CanvasWindowState>,
        index: number
    ): { x: number; y: number } {
        const candidates: { x: number; y: number }[] = [];
        for (let i = 0; i < CascadeWrap; i++) {
            candidates.push(cascadeSlot(i));
        }
        for (let i = 0; i < CascadeWrap; i++) {
            candidates.push({ x: cascadeSlot(i).x + 340, y: cascadeSlot(i).y + 210 });
        }
        for (let i = 0; i < CascadeWrap; i++) {
            candidates.push({ x: cascadeSlot(i).x + 150, y: cascadeSlot(i).y + 420 });
        }
        const obstacles = Object.values(existing).filter((win) => !win.maximized);
        for (const candidate of candidates) {
            const pos = this.clampPos(candidate.x, candidate.y, w, h);
            if (this.overlapRatio(pos.x, pos.y, w, h, obstacles) <= 0.18) {
                return pos;
            }
        }
        const focusedId = globalStore.get(this.focusedBlockAtom);
        const focusedWin = focusedId != null ? existing[focusedId] : null;
        if (focusedWin != null && !focusedWin.maximized) {
            return { x: focusedWin.x + CascadeStepX, y: focusedWin.y + CascadeStepY };
        }
        // no focused window to cascade from: pick the least-covered spot on the canvas
        const bounds = this.bounds;
        let bestPos: { x: number; y: number } = null;
        let bestOverlap = Number.POSITIVE_INFINITY;
        const maxGx = Math.max(24, bounds.width - MinVisibleX);
        const maxGy = Math.max(24, bounds.height - MinVisibleY);
        for (let gx = 24; gx <= maxGx; gx += 150) {
            for (let gy = 24; gy <= maxGy; gy += 120) {
                const pos = this.clampPos(gx, gy, w, h);
                const overlap = this.overlapRatio(pos.x, pos.y, w, h, obstacles);
                if (overlap < bestOverlap) {
                    bestOverlap = overlap;
                    bestPos = pos;
                }
            }
        }
        if (bestPos != null) {
            return bestPos;
        }
        return cascadeSlot(index);
    }

    private overlapRatio(x: number, y: number, w: number, h: number, obstacles: CanvasWindowState[]): number {
        let worst = 0;
        const area = w * h;
        for (const other of obstacles) {
            const overlapW = Math.min(x + w, other.x + other.w) - Math.max(x, other.x);
            const overlapH = Math.min(y + h, other.y + other.h) - Math.max(y, other.y);
            if (overlapW > 0 && overlapH > 0) {
                worst = Math.max(worst, (overlapW * overlapH) / area);
            }
        }
        return worst;
    }

    private clampPos(x: number, y: number, w: number, h: number): { x: number; y: number } {
        const bounds = this.bounds;
        const minX = Math.min(0, bounds.width) + MinVisibleX - w;
        const maxX = Math.max(minX, bounds.width - MinVisibleX);
        const minY = 0;
        const maxY = Math.max(minY, bounds.height - MinVisibleY);
        return { x: clampNum(x, minX, maxX), y: clampNum(y, minY, maxY) };
    }

    setWindow(blockId: string, patch: Partial<CanvasWindowState>): void {
        const cur = globalStore.get(this.windowsAtom);
        const win = cur[blockId];
        if (win == null) {
            return;
        }
        globalStore.set(this.windowsAtom, { ...cur, [blockId]: { ...win, ...patch } });
    }

    // move a window (clamped so its header stays reachable)
    moveWindow(blockId: string, x: number, y: number): void {
        const win = this.getWindow(blockId);
        if (win == null) {
            return;
        }
        this.setWindow(blockId, this.clampPos(x, y, win.w, win.h));
    }

    // resize a window (clamped to min size and canvas bounds)
    resizeWindow(blockId: string, rect: { x: number; y: number; w: number; h: number }): void {
        const win = this.getWindow(blockId);
        if (win == null) {
            return;
        }
        const bounds = this.bounds;
        const w = clampNum(rect.w, MinWinW, Math.max(MinWinW, bounds.width + 120));
        const h = clampNum(rect.h, MinWinH, Math.max(MinWinH, bounds.height + 120));
        const pos = this.clampPos(rect.x, rect.y, w, h);
        this.setWindow(blockId, { ...pos, w, h });
    }

    // re-clamp all windows after the canvas container resized
    reclampAll(): void {
        const cur = globalStore.get(this.windowsAtom);
        const bounds = this.bounds;
        const next: Record<string, CanvasWindowState> = {};
        let changed = false;
        for (const [blockId, win] of Object.entries(cur)) {
            if (win.maximized) {
                next[blockId] = win;
                continue;
            }
            const w = clampNum(win.w, MinWinW, Math.max(MinWinW, bounds.width));
            const h = clampNum(win.h, MinWinH, Math.max(MinWinH, bounds.height));
            const pos = this.clampPos(win.x, win.y, w, h);
            if (w !== win.w || h !== win.h || pos.x !== win.x || pos.y !== win.y) {
                next[blockId] = { ...win, ...pos, w, h };
                changed = true;
            } else {
                next[blockId] = win;
            }
        }
        if (changed) {
            globalStore.set(this.windowsAtom, next);
        }
    }

    setInteracting(blockId: string | null): void {
        globalStore.set(this.interactingBlockAtom, blockId);
    }

    bringToFront(blockId: string): void {
        const cur = globalStore.get(this.windowsAtom);
        const win = cur[blockId];
        if (win == null) {
            return;
        }
        if (win.maximized) {
            return;
        }
        globalStore.set(this.windowsAtom, { ...cur, [blockId]: { ...win, z: this.nextZ() } });
    }

    focusBlock(blockId: string): void {
        globalStore.set(this.focusedBlockAtom, blockId);
        this.bringToFront(blockId);
        this.mirrorTreeFocus(blockId);
    }

    clearFocus(): void {
        globalStore.set(this.focusedBlockAtom, null);
    }

    toggleMaximize(blockId: string): void {
        const win = this.getWindow(blockId);
        if (win == null) {
            return;
        }
        if (!win.maximized) {
            globalStore.set(this.focusedBlockAtom, blockId);
            this.mirrorTreeFocus(blockId);
        }
        this.setWindow(blockId, { maximized: !win.maximized });
        globalStore.set(this.windowsAtom, {
            ...globalStore.get(this.windowsAtom),
            [blockId]: { ...globalStore.get(this.windowsAtom)[blockId], z: this.nextZ() },
        });
    }

    // mirror canvas focus into the layout tree so close/keyboard flows stay consistent
    private mirrorTreeFocus(blockId: string): void {
        try {
            const lm = getLayoutModelForStaticTab();
            if (lm == null) {
                return;
            }
            const node = lm.getNodeByBlockId(blockId);
            if (node != null) {
                const focusAction: LayoutTreeFocusNodeAction = {
                    type: LayoutTreeActionType.FocusNode,
                    nodeId: node.id,
                };
                lm.treeReducer(focusAction);
            }
        } catch (e) {
            console.warn("canvas: failed to mirror focus", e);
        }
    }

    persistWindow(blockId: string): void {
        const win = this.getWindow(blockId);
        if (win == null) {
            return;
        }
        try {
            const oref = WOS.makeORef("block", blockId);
            services.ObjectService.UpdateObjectMeta(oref, {
                "canvas:x": Math.round(win.x),
                "canvas:y": Math.round(win.y),
                "canvas:w": Math.round(win.w),
                "canvas:h": Math.round(win.h),
            }).catch((e: unknown) => console.warn("canvas: failed to persist window geometry", e));
        } catch (e) {
            console.warn("canvas: failed to persist window geometry", e);
        }
    }

    // arrange all windows in a tidy cascade and persist
    tidy(): void {
        const cur = globalStore.get(this.windowsAtom);
        const bounds = this.bounds;
        const next: Record<string, CanvasWindowState> = {};
        let idx = 0;
        for (const [blockId, win] of Object.entries(cur)) {
            if (win.maximized) {
                next[blockId] = win;
                continue;
            }
            const w = clampNum(win.w, MinWinW, Math.max(MinWinW, bounds.width - 16));
            const h = clampNum(win.h, MinWinH, Math.max(MinWinH, bounds.height - 16));
            const slot = cascadeSlot(idx);
            const { x, y } = this.clampPos(slot.x, slot.y, w, h);
            next[blockId] = { ...win, x, y, w, h };
            idx = idx + 1;
        }
        globalStore.set(this.windowsAtom, next);
        for (const blockId of Object.keys(next)) {
            this.persistWindow(blockId);
        }
    }

    setEphemeralBlock(blockId: string | null): void {
        const cur = globalStore.get(this.ephemeralBlockAtom);
        if (cur !== blockId) {
            globalStore.set(this.ephemeralBlockAtom, blockId);
        }
    }

    getEphemeralBlock(): string | null {
        return globalStore.get(this.ephemeralBlockAtom);
    }

    // convert an ephemeral overlay back into a regular floating window
    addEphemeralToLayout(blockId: string): void {
        try {
            const lm = getLayoutModelForStaticTab();
            lm?.addEphemeralNodeToLayout();
        } catch (e) {
            console.warn("canvas: failed to add ephemeral node to layout", e);
        }
        globalStore.set(this.ephemeralBlockAtom, null);
        const win = this.getWindow(blockId);
        if (win != null) {
            const bounds = this.bounds;
            const w = clampNum(win.w, MinWinW, Math.max(MinWinW, bounds.width - 16));
            const h = clampNum(win.h, MinWinH, Math.max(MinWinH, bounds.height - 16));
            const pos = this.clampPos(win.x, win.y, w, h);
            this.setWindow(blockId, { ...pos, w, h });
        }
    }

    closeBlock(blockId: string): void {
        const tabAtom = WOS.getWaveObjectAtom<Tab>(WOS.makeORef("tab", this.tabId));
        const tabData = globalStore.get(tabAtom);
        const count = tabData?.blockids?.length ?? 0;
        if (count <= 1) {
            // closing the last window closes the tab (respects tab:confirmclose)
            uxCloseBlock(blockId);
            return;
        }
        const cur = globalStore.get(this.windowsAtom);
        const next = { ...cur };
        delete next[blockId];
        globalStore.set(this.windowsAtom, next);
        this.nodeModels.delete(blockId);
        if (globalStore.get(this.ephemeralBlockAtom) === blockId) {
            globalStore.set(this.ephemeralBlockAtom, null);
        }
        try {
            const lm = getLayoutModelForStaticTab();
            // ephemeral (overlay) blocks live outside the tree: close through their ephemeral node
            const ephemeralNode = lm != null ? globalStore.get(lm.ephemeralNode) : null;
            if (ephemeralNode?.data?.blockId === blockId) {
                lm.closeNode(ephemeralNode.id).catch((e: unknown) => console.warn("canvas: closeNode failed", e));
                return;
            }
            const node = lm?.getNodeByBlockId(blockId);
            if (node != null) {
                lm.closeNode(node.id).catch((e: unknown) => console.warn("canvas: closeNode failed", e));
            } else {
                services.ObjectService.DeleteBlock(blockId).catch((e: unknown) =>
                    console.warn("canvas: DeleteBlock failed", e)
                );
            }
        } catch (e) {
            console.warn("canvas: failed to close block", e);
        }
    }

    async createTerminalWindow(): Promise<void> {
        await RpcApi.CreateBlockCommand(TabRpcClient, {
            tabid: this.tabId,
            blockdef: { meta: { view: "term", controller: "shell" } },
            rtopts: { termsize: { rows: 25, cols: 80 } },
            focused: true,
        });
    }

    getNodeModel(blockId: string): NodeModel {
        let nm = this.nodeModels.get(blockId);
        if (nm != null) {
            return nm;
        }
        const model = this;
        nm = {
            additionalProps: atom({ treeKey: "canvas" }),
            innerRect: atom<CSSProperties>(null),
            blockNum: atom((get) => {
                const wins = get(model.windowsAtom);
                const idx = Object.keys(wins).indexOf(blockId);
                return idx < 0 ? 0 : idx + 1;
            }),
            numLeafs: atom((get) => Math.max(2, Object.keys(get(model.windowsAtom)).length)),
            nodeId: `canvas-${blockId}`,
            blockId: blockId,
            addEphemeralNodeToLayout: () => {
                model.addEphemeralToLayout(blockId);
            },
            animationTimeS: atom(0),
            isResizing: atom((get) => get(model.interactingBlockAtom) === blockId),
            isFocused: atom((get) => get(model.focusedBlockAtom) === blockId),
            isMagnified: atom((get) => get(model.windowsAtom)[blockId]?.maximized ?? false),
            anyMagnified: atom((get) => Object.values(get(model.windowsAtom)).some((win) => win.maximized)),
            isEphemeral: atom((get) => get(model.ephemeralBlockAtom) === blockId),
            ready: atom(true),
            disablePointerEvents: atom(false),
            toggleMagnify: () => {
                model.toggleMaximize(blockId);
            },
            focusNode: () => {
                model.focusBlock(blockId);
            },
            onClose: () => {
                model.closeBlock(blockId);
            },
            dragHandleRef: createRef(),
            displayContainerRef: model.containerRef,
        };
        this.nodeModels.set(blockId, nm);
        return nm;
    }
}

const canvasModelCache = new Map<string, CanvasModel>();

export function getCanvasModel(tabId: string): CanvasModel {
    if (isBlank(tabId)) {
        return null;
    }
    let model = canvasModelCache.get(tabId);
    if (model == null) {
        model = new CanvasModel(tabId);
        canvasModelCache.set(tabId, model);
    }
    return model;
}

export function deleteCanvasModel(tabId: string): void {
    canvasModelCache.delete(tabId);
}

export { CanvasModel, DefaultWinW, DefaultWinH, MinWinW, MinWinH, MaxExtraZ };
