// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

// Canvas ("Deska") mode: renders all blocks of the tab as free-floating
// windows on a chill background instead of the tiled layout. Enable via the
// app:canvasmode setting.

import { useOnResize } from "@/app/hook/useDimensions";
import { Block } from "@/app/block/block";
import * as services from "@/app/store/services";
import { getLayoutModelForStaticTab, type LayoutNode, type NodeModel } from "@/layout/index";
import type { TileLayoutContents } from "@/layout/lib/types";
import { getBlockMetaKeyAtom, getSettingsKeyAtom } from "@/store/global";
import { fireAndForget } from "@/util/util";
import { type Atom, atom as jotaiAtom, useAtomValue } from "jotai";
import * as React from "react";
import { memo, useCallback, useEffect, useMemo, useRef } from "react";
import { CanvasBackground } from "./canvas-bg";
import { getCanvasModel, type CanvasModel, type CanvasWindowState } from "./canvas-model";
import { CanvasWindow } from "./canvas-window";
import "./canvas.scss";

const NoEphemeralAtom: Atom<LayoutNode | null> = jotaiAtom<LayoutNode | null>(null);

export interface CanvasLayoutProps {
    tabAtom: Atom<Tab>;
    tabId: string;
}

// The project launcher ("Open a project") is a setup screen, not a real
// window: render it full-canvas — no frame, no drag, no clipping — instead of
// a small floating window.
const CanvasBlockWindow = memo(
    ({ model, blockId, state }: { model: CanvasModel; blockId: string; state: CanvasWindowState }) => {
        const view = useAtomValue(getBlockMetaKeyAtom(blockId, "view"));
        const nodeModel = model.getNodeModel(blockId);
        if (view === "projectlauncher") {
            return (
                <div key={blockId} className="canvas-welcome">
                    <Block key={blockId} nodeModel={nodeModel} preview={false} />
                </div>
            );
        }
        return <CanvasWindow key={blockId} model={model} blockId={blockId} nodeModel={nodeModel} state={state} />;
    }
);
CanvasBlockWindow.displayName = "CanvasBlockWindow";

export const CanvasLayout = memo(({ tabAtom, tabId }: CanvasLayoutProps) => {
    const tabData = useAtomValue(tabAtom);
    const blockIds = useMemo(() => tabData?.blockids ?? [], [tabData]);
    const model = getCanvasModel(tabId);
    const wins = useAtomValue(model.windowsAtom);
    const ephemeralBlockId = useAtomValue(model.ephemeralBlockAtom);
    const containerRef = useRef<HTMLDivElement>(null);
    const firstReconcileRef = useRef(true);
    const [layoutModel] = React.useState(() => getLayoutModelForStaticTab());
    const ephemeralNode = useAtomValue(layoutModel?.ephemeralNode ?? NoEphemeralAtom);

    const bgPreset = useAtomValue(getSettingsKeyAtom("app:canvasbg")) ?? "grid";
    const bgImage = useAtomValue(getSettingsKeyAtom("app:canvasbgimage")) ?? "";
    const bgDim = useAtomValue(getSettingsKeyAtom("app:canvasbgdim"));

    // Keep the tab's layout tree in sync in the background (backend layout
    // actions, orphan cleanup, close flows) even though we don't render tiles.
    useEffect(() => {
        if (!tabId) {
            return;
        }
        model.setContainerRef(containerRef);
        try {
            const lm = getLayoutModelForStaticTab();
            if (lm == null) {
                return;
            }
            const contents: TileLayoutContents = {
                tabId,
                // The canvas renders blocks itself (CanvasWindow), but the layout
                // model's render callbacks must be left intact: TileLayout bakes
                // them into the leaf content when it mounts (`[nodeModel]` memo),
                // so a stub renderer left behind here makes the tab render empty
                // after switching back to the normal tiled mode.
                renderContent:
                    lm.renderContent ??
                    ((nodeModel: NodeModel) => <Block key={nodeModel.blockId} nodeModel={nodeModel} preview={false} />),
                renderPreview: lm.renderPreview,
                onNodeDelete: (data: TabLayoutData) => services.ObjectService.DeleteBlock(data.blockId),
            };
            lm.registerTileLayout(contents);
        } catch (e) {
            console.warn("canvas: failed to register with layout model", e);
        }
    }, [tabId, model]);

    useOnResize(containerRef, (rect) => {
        model.setBounds({ width: rect.width, height: rect.height });
        model.reclampAll();
    });

    useEffect(() => {
        const added = model.reconcile(blockIds);
        if (firstReconcileRef.current) {
            firstReconcileRef.current = false;
            return;
        }
        if (added.length > 0) {
            // surface freshly created windows: raise + focus them
            model.focusBlock(added[added.length - 1]);
        }
    }, [model, blockIds]);

    // track the current ephemeral (overlay) block, e.g. Settings opened from the gear menu
    useEffect(() => {
        const ephBlockId = ephemeralNode?.data?.blockId ?? null;
        model.setEphemeralBlock(ephBlockId != null && blockIds.includes(ephBlockId) ? ephBlockId : null);
    }, [model, ephemeralNode, blockIds]);

    const onPointerDown = useCallback(
        (e: React.PointerEvent<HTMLDivElement>) => {
            const target = e.target as HTMLElement;
            if (target.closest(".canvas-window") != null || target.closest(".canvas-toolbar") != null) {
                return;
            }
            model.clearFocus();
        },
        [model]
    );

    const onNewTerminal = useCallback(() => {
        fireAndForget(() => model.createTerminalWindow());
    }, [model]);

    const onTidy = useCallback(() => {
        model.tidy();
    }, [model]);

    const onBackdropClick = useCallback(() => {
        const ephBlockId = model.getEphemeralBlock();
        if (ephBlockId != null) {
            model.closeBlock(ephBlockId);
        }
    }, [model]);

    return (
        <div ref={containerRef} className="canvas-layout" onPointerDown={onPointerDown}>
            <CanvasBackground preset={bgPreset} customImage={bgImage} dim={bgDim} />
            <div className="canvas-windows">
                {blockIds.map((blockId) => {
                    const state = wins[blockId];
                    if (state == null) {
                        return null;
                    }
                    return (
                        <CanvasBlockWindow key={blockId} model={model} blockId={blockId} state={state} />
                    );
                })}
            </div>
            {ephemeralBlockId != null && <div className="canvas-ephemeral-backdrop" onClick={onBackdropClick} />}
            {blockIds.length === 0 && (
                <div className="canvas-empty-hint">
                    <div className="canvas-empty-hint-title">Empty canvas</div>
                    <div className="canvas-empty-hint-sub">
                        Open a floating terminal with the button below. Drag window headers to move them, drag edges to
                        resize, double-click a header to maximize.
                    </div>
                </div>
            )}
            <div className="canvas-toolbar">
                <button type="button" className="canvas-toolbar-btn" title="New Terminal" onClick={onNewTerminal}>
                    <i className="fa fa-solid fa-circle-plus" />
                    <span>New Terminal</span>
                </button>
                {blockIds.length > 0 && (
                    <button type="button" className="canvas-toolbar-btn" title="Tidy windows" onClick={onTidy}>
                        <i className="fa fa-solid fa-layer-group" />
                        <span>Tidy</span>
                    </button>
                )}
            </div>
        </div>
    );
});
CanvasLayout.displayName = "CanvasLayout";
