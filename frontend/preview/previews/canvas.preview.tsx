// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

// Preview: canvas mode backgrounds + interactive floating windows.
// "Add window" mimics creating a block (widget / New Terminal) — the new
// window should land in a free spot and get focused. "Settings overlay"
// mimics gear-menu Settings (ephemeral block) — centered overlay + backdrop.

import { CanvasBackground, CanvasBgPresets } from "@/app/canvas/canvas-bg";
import { getCanvasModel } from "@/app/canvas/canvas-model";
import { CanvasWindow } from "@/app/canvas/canvas-window";
import { makeMockNodeModel } from "@/preview/mock/mock-node-model";
import { useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useRef, useState } from "react";

const DemoTerminal = memo(({ title }: { title: string }) => {
    return (
        <div className="flex h-full w-full flex-col bg-[#0c0c11] text-[#d6d6e0]">
            <div
                data-role="block-header"
                className="flex h-[30px] shrink-0 items-center gap-2 border-b border-white/10 px-2.5 text-[12px]"
            >
                <i className="fa fa-solid fa-terminal text-[10px] opacity-70" />
                <span className="opacity-90">{title}</span>
                <span className="flex-1" />
                <button type="button" className="block-frame-default-close cursor-pointer opacity-60 hover:opacity-100">
                    <i className="fa fa-solid fa-xmark-large text-[11px]" />
                </button>
            </div>
            <div className="flex-1 overflow-hidden p-3 font-mono text-[11.5px] leading-relaxed">
                <div className="opacity-60">Windows PowerShell</div>
                <div className="opacity-60">Copyright (C) Microsoft Corporation. All rights reserved.</div>
                <div className="mt-2 opacity-90">
                    PS C:\Users\Usuario\Projects&gt; <span className="canvas-cursor">▊</span>
                </div>
            </div>
        </div>
    );
});

const initialDemoBlocks = ["term-1", "term-2", "term-3"];

const CanvasDemo = memo(() => {
    const model = getCanvasModel("preview-canvas-tab");
    const wins = useAtomValue(model.windowsAtom);
    const ephemeralBlockId = useAtomValue(model.ephemeralBlockAtom);
    const [blocks, setBlocks] = useState<string[]>(initialDemoBlocks);
    const nextIdxRef = useRef(4);
    const firstRunRef = useRef(true);

    useEffect(() => {
        model.setBounds({ width: 920, height: 540 });
        model.setWindow("term-1", { x: 36, y: 84, w: 430, h: 300 });
        model.setWindow("term-2", { x: 430, y: 36, w: 450, h: 320 });
        model.setWindow("term-3", { x: 150, y: 260, w: 400, h: 250 });
    }, []);

    useEffect(() => {
        const added = model.reconcile(blocks);
        if (firstRunRef.current) {
            firstRunRef.current = false;
            return;
        }
        if (added.length > 0) {
            model.focusBlock(added[added.length - 1]);
        }
    }, [model, blocks]);

    const onAddWindow = useCallback(() => {
        const id = `term-${nextIdxRef.current}`;
        nextIdxRef.current = nextIdxRef.current + 1;
        setBlocks((prev) => [...prev, id]);
    }, []);

    const onToggleEphemeral = useCallback(() => {
        if (model.getEphemeralBlock() != null) {
            model.setEphemeralBlock(null);
            setBlocks((prev) => prev.filter((b) => b !== "term-eph"));
        } else {
            setBlocks((prev) => (prev.includes("term-eph") ? prev : [...prev, "term-eph"]));
            setTimeout(() => {
                model.setEphemeralBlock("term-eph");
            }, 30);
        }
    }, [model]);

    const onTidy = useCallback(() => {
        model.tidy();
    }, [model]);

    return (
        <div className="flex flex-col gap-2">
            <div className="flex gap-2">
                <button
                    type="button"
                    className="cursor-pointer rounded-md border border-white/15 bg-white/5 px-3 py-1 text-[12px] hover:bg-white/10"
                    onClick={onAddWindow}
                >
                    + Create window (widget flow)
                </button>
                <button
                    type="button"
                    className="cursor-pointer rounded-md border border-white/15 bg-white/5 px-3 py-1 text-[12px] hover:bg-white/10"
                    onClick={onToggleEphemeral}
                >
                    {ephemeralBlockId != null ? "Close Settings overlay" : "Open Settings overlay (gear flow)"}
                </button>
                <button
                    type="button"
                    className="cursor-pointer rounded-md border border-white/15 bg-white/5 px-3 py-1 text-[12px] hover:bg-white/10"
                    onClick={onTidy}
                >
                    Tidy
                </button>
            </div>
            <div className="relative h-[540px] w-[920px] overflow-hidden rounded-xl border border-white/10">
                <CanvasBackground preset="lofi" dim={0.35} />
                <div className="absolute inset-0">
                    {blocks.map((blockId) =>
                        wins[blockId] == null ? null : (
                            <CanvasWindow
                                key={blockId}
                                model={model}
                                blockId={blockId}
                                nodeModel={makeMockNodeModel({ nodeId: `mock-${blockId}`, blockId })}
                                state={wins[blockId]}
                            >
                                <DemoTerminal title={blockId === "term-eph" ? "Settings" : `Terminal ${blockId}`} />
                            </CanvasWindow>
                        )
                    )}
                </div>
                {ephemeralBlockId != null && (
                    <div
                        className="canvas-ephemeral-backdrop"
                        onClick={() => {
                            model.setEphemeralBlock(null);
                            setBlocks((prev) => prev.filter((b) => b !== ephemeralBlockId));
                        }}
                    />
                )}
                <div className="canvas-toolbar">
                    <button type="button" className="canvas-toolbar-btn">
                        <i className="fa fa-solid fa-circle-plus" />
                        <span>New Terminal</span>
                    </button>
                    <button type="button" className="canvas-toolbar-btn">
                        <i className="fa fa-solid fa-layer-group" />
                        <span>Tidy</span>
                    </button>
                </div>
            </div>
        </div>
    );
});
CanvasDemo.displayName = "CanvasDemo";

const CanvasPreview = memo(() => {
    return (
        <div className="flex flex-col gap-6 p-6">
            <div className="flex flex-col gap-2">
                <div className="text-[14px] font-semibold">
                    Floating windows — drag headers, resize edges, dbl-click to maximize
                </div>
                <CanvasDemo />
            </div>
            <div className="flex flex-col gap-2">
                <div className="text-[14px] font-semibold">Built-in backgrounds</div>
                <div className="flex flex-wrap gap-3">
                    {CanvasBgPresets.map((preset) => (
                        <div key={preset.id} className="flex flex-col gap-1">
                            <div className="relative h-[150px] w-[260px] overflow-hidden rounded-lg border border-white/10">
                                <CanvasBackground preset={preset.id} dim={0.2} />
                            </div>
                            <span className="text-[11px] text-secondary">{preset.label}</span>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
});
CanvasPreview.displayName = "CanvasPreview";

export default CanvasPreview;
