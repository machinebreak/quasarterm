// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

// A free-floating, draggable, resizable block window rendered inside the
// canvas layout. Drag by the block header, resize from any edge/corner,
// double-click the header to maximize.

import { Block } from "@/app/block/block";
import type { NodeModel } from "@/layout/index";
import { cn } from "@/util/util";
import { useAtomValue } from "jotai";
import * as React from "react";
import { memo, useCallback, useRef, useState } from "react";
import type { CanvasBounds, CanvasModel, CanvasWindowState } from "./canvas-model";
import { MaxExtraZ, MinWinH, MinWinW } from "./canvas-model";
import "./canvas.scss";

const ResizeDirections = ["n", "s", "e", "w", "ne", "nw", "se", "sw"] as const;

export interface CanvasWindowProps {
    model: CanvasModel;
    blockId: string;
    nodeModel: NodeModel;
    state: CanvasWindowState;
    bounds?: CanvasBounds;
    children?: React.ReactNode;
}

export const CanvasWindow = memo(({ model, blockId, nodeModel, state, children }: CanvasWindowProps) => {
    const winRef = useRef<HTMLDivElement>(null);
    const dragRef = useRef<{ px: number; py: number; x: number; y: number } | null>(null);
    const resizeRef = useRef<{ dir: string; px: number; py: number; rect: CanvasWindowState } | null>(null);
    const doubleClickRef = useRef<{ t: number; x: number; y: number } | null>(null);
    const [interaction, setInteraction] = useState<"drag" | "resize" | null>(null);
    const focusedBlockId = useAtomValue(model.focusedBlockAtom);
    const isFocused = focusedBlockId === blockId;
    const ephemeralBlockId = useAtomValue(model.ephemeralBlockAtom);
    const isEphemeral = ephemeralBlockId === blockId;

    const finishInteraction = useCallback(
        (e: React.PointerEvent<HTMLDivElement>) => {
            if (dragRef.current != null || resizeRef.current != null) {
                dragRef.current = null;
                resizeRef.current = null;
                model.setInteracting(null);
                model.persistWindow(blockId);
            }
            setInteraction(null);
            try {
                winRef.current?.releasePointerCapture(e.pointerId);
            } catch (_) {
                // ignore
            }
        },
        [model, blockId]
    );

    const onPointerDown = useCallback(
        (e: React.PointerEvent<HTMLDivElement>) => {
            if (e.button !== 0) {
                return;
            }
            model.focusBlock(blockId);
            if (isEphemeral) {
                return;
            }
            const target = e.target as HTMLElement;
            const onHeader =
                target.closest('[data-role="block-header"]') != null &&
                target.closest("button, input, textarea, select, a, [role='button'], [data-no-drag]") == null;
            if (onHeader) {
                // manual double-click detection (pointer capture retargets the native dblclick)
                const now = Date.now();
                const last = doubleClickRef.current;
                if (last != null && now - last.t < 400 && Math.abs(e.clientX - last.x) < 8 && Math.abs(e.clientY - last.y) < 8) {
                    doubleClickRef.current = null;
                    model.toggleMaximize(blockId);
                    return;
                }
                doubleClickRef.current = { t: now, x: e.clientX, y: e.clientY };
            }
            if (state.maximized) {
                return;
            }
            if (target.closest("[data-canvas-resize]") != null) {
                return;
            }
            if (!onHeader) {
                return;
            }
            dragRef.current = { px: e.clientX, py: e.clientY, x: state.x, y: state.y };
            model.setInteracting(blockId);
            setInteraction("drag");
            try {
                winRef.current?.setPointerCapture(e.pointerId);
            } catch (_) {
                // pointer capture is best-effort
            }
        },
        [model, blockId, state.maximized, state.x, state.y, isEphemeral]
    );

    const startResize = useCallback(
        (dir: string) => (e: React.PointerEvent<HTMLDivElement>) => {
            if (e.button !== 0 || state.maximized || isEphemeral) {
                return;
            }
            e.stopPropagation();
            e.preventDefault();
            model.focusBlock(blockId);
            resizeRef.current = { dir, px: e.clientX, py: e.clientY, rect: { ...state } };
            model.setInteracting(blockId);
            setInteraction("resize");
            try {
                winRef.current?.setPointerCapture(e.pointerId);
            } catch (_) {
                // pointer capture is best-effort
            }
        },
        [model, blockId, state]
    );

    const onPointerMove = useCallback(
        (e: React.PointerEvent<HTMLDivElement>) => {
            if (dragRef.current != null) {
                const d = dragRef.current;
                model.moveWindow(blockId, d.x + (e.clientX - d.px), d.y + (e.clientY - d.py));
                return;
            }
            if (resizeRef.current != null) {
                const r = resizeRef.current;
                const dx = e.clientX - r.px;
                const dy = e.clientY - r.py;
                let { x, y, w, h } = r.rect;
                if (r.dir.includes("e")) {
                    w = r.rect.w + dx;
                }
                if (r.dir.includes("s")) {
                    h = r.rect.h + dy;
                }
                if (r.dir.includes("w")) {
                    x = r.rect.x + dx;
                    w = r.rect.w - dx;
                }
                if (r.dir.includes("n")) {
                    y = r.rect.y + dy;
                    h = r.rect.h - dy;
                }
                if (w < MinWinW) {
                    w = MinWinW;
                    if (r.dir.includes("w")) {
                        x = r.rect.x + r.rect.w - MinWinW;
                    }
                }
                if (h < MinWinH) {
                    h = MinWinH;
                    if (r.dir.includes("n")) {
                        y = r.rect.y + r.rect.h - MinWinH;
                    }
                }
                model.resizeWindow(blockId, { x, y, w, h });
            }
        },
        [model, blockId]
    );

    const onClickCapture = useCallback(
        (e: React.MouseEvent<HTMLDivElement>) => {
            const target = e.target as HTMLElement;
            if (target.closest(".block-frame-default-close") != null) {
                // take over the header close button so canvas bookkeeping stays in sync
                e.stopPropagation();
                e.preventDefault();
                model.closeBlock(blockId);
            }
        },
        [model, blockId]
    );

    const style: React.CSSProperties = isEphemeral
        ? {
              left: "50%",
              top: "50%",
              transform: "translate(-50%, -50%)",
              width: "min(1080px, calc(100% - 140px))",
              height: "min(700px, calc(100% - 140px))",
              zIndex: 26000,
          }
        : state.maximized
          ? {
                left: 8,
                top: 8,
                width: "calc(100% - 16px)",
                height: "calc(100% - 16px)",
                zIndex: MaxExtraZ + state.z,
            }
          : { left: state.x, top: state.y, width: state.w, height: state.h, zIndex: state.z };

    return (
        <div
            ref={winRef}
            className={cn("canvas-window", {
                "canvas-window-focused": isFocused,
                "canvas-window-maximized": state.maximized,
                "canvas-window-ephemeral": isEphemeral,
                "canvas-window-dragging": interaction === "drag",
                "canvas-window-resizing": interaction === "resize",
            })}
            style={style}
            onPointerDown={onPointerDown}
            onPointerMove={onPointerMove}
            onPointerUp={finishInteraction}
            onPointerCancel={finishInteraction}
            onClickCapture={onClickCapture}
            data-canvas-blockid={blockId}
        >
            {children ?? <Block key={blockId} nodeModel={nodeModel} preview={false} />}
            {!state.maximized &&
                !isEphemeral &&
                ResizeDirections.map((dir) => (
                    <div
                        key={dir}
                        data-canvas-resize={dir}
                        className={cn("canvas-resize-handle", `canvas-resize-${dir}`)}
                        onPointerDown={startResize(dir)}
                    />
                ))}
        </div>
    );
});
CanvasWindow.displayName = "CanvasWindow";
