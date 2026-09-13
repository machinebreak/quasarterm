// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import logoUrl from "@/app/asset/quasar-logo-white.svg?url";
import type { BlockNodeModel } from "@/app/block/blocktypes";
import { getApi, getSettingsKeyAtom, globalStore, replaceBlock, WOS } from "@/app/store/global";
import type { TabModel } from "@/app/store/tab-model";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { fireAndForget } from "@/util/util";
import { atom, PrimitiveAtom, useAtomValue } from "jotai";
import { useState } from "react";

const MaxRecentProjects = 8;

function projectName(dir: string): string {
    const parts = dir.split(/[\\/]/).filter((part) => part !== "");
    return parts[parts.length - 1] ?? dir;
}

// Shown for new tabs: pick a folder and the tab is bound to that project (the
// term block that replaces this view starts in it, and new terminals in the
// tab fall back to it).
export class ProjectLauncherViewModel implements ViewModel {
    blockId: string;
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    viewType = "projectlauncher";
    viewIcon = atom("folder-open");
    viewName = atom("Open Project");
    viewComponent = ProjectLauncherView;
    noHeader = atom(true);
    busyAtom: PrimitiveAtom<boolean> = atom<boolean>(false) as PrimitiveAtom<boolean>;
    errorAtom: PrimitiveAtom<string> = atom<string>("") as PrimitiveAtom<string>;

    constructor({ blockId, nodeModel, tabModel }: ViewModelInitType) {
        this.blockId = blockId;
        this.nodeModel = nodeModel;
        this.tabModel = tabModel;
    }

    recentProjectsAtom = atom((get) => {
        const recents = get(getSettingsKeyAtom("app:recentprojects")) ?? [];
        return recents.filter((dir) => dir?.trim() !== "");
    });

    async pickFolder() {
        globalStore.set(this.errorAtom, "");
        let dir: string = null;
        try {
            dir = await getApi().openDirectoryDialog?.({ title: "Open a project folder" });
        } catch (e) {
            globalStore.set(this.errorAtom, `Could not open the folder picker: ${e?.message ?? e}`);
            return;
        }
        if (dir == null) {
            return;
        }
        await this.openProject(dir);
    }

    async openProject(dir: string) {
        globalStore.set(this.busyAtom, true);
        globalStore.set(this.errorAtom, "");
        try {
            const tabId = this.tabModel?.tabId;
            const recents = globalStore.get(getSettingsKeyAtom("app:recentprojects")) ?? [];
            const nextRecents = [dir, ...recents.filter((entry) => entry !== dir)].slice(0, MaxRecentProjects);
            fireAndForget(() => RpcApi.SetConfigCommand(TabRpcClient, { "app:recentprojects": nextRecents }));
            if (tabId != null) {
                fireAndForget(() => RpcApi.UpdateTabNameCommand(TabRpcClient, tabId, projectName(dir)));
                await RpcApi.SetMetaCommand(TabRpcClient, {
                    oref: WOS.makeORef("tab", tabId),
                    meta: { "cmd:cwd": dir },
                });
            }
            await replaceBlock(this.blockId, { meta: { view: "term", controller: "shell", "cmd:cwd": dir } }, true);
        } catch (e) {
            globalStore.set(this.busyAtom, false);
            globalStore.set(this.errorAtom, `Could not open the project: ${e?.message ?? e}`);
        }
    }

    async openHomeTerminal() {
        await replaceBlock(this.blockId, { meta: { view: "term", controller: "shell" } }, true);
    }
}

function ProjectLauncherView({ model }: ViewComponentProps<ProjectLauncherViewModel>) {
    const recents = useAtomValue(model.recentProjectsAtom);
    const busy = useAtomValue(model.busyAtom);
    const error = useAtomValue(model.errorAtom);
    const [pathInput, setPathInput] = useState("");

    const submitPath = () => {
        const dir = pathInput.trim();
        if (dir === "") {
            return;
        }
        fireAndForget(() => model.openProject(dir));
    };

    return (
        <div className="w-full h-full overflow-y-auto box-border">
            <div className="min-h-full w-full flex flex-col items-center justify-center gap-6 p-6">
                <img src={logoUrl} alt="" className="w-[150px] h-auto opacity-90 select-none" draggable={false} />

                <div className="flex flex-col items-center gap-1.5 text-center">
                    <div className="text-title font-semibold tracking-tight text-foreground">Open a project</div>
                    <div className="text-secondary text-[12.5px]">Pick a folder and Quasar opens a terminal in it.</div>
                </div>

                <div className="flex flex-col w-full max-w-[560px] gap-2.5">
                    <button
                        className="flex w-full h-10 items-center justify-center gap-2 rounded-lg bg-accent text-background text-[13px] font-medium cursor-pointer transition-colors hover:bg-accenthover disabled:opacity-40 disabled:cursor-default"
                        disabled={busy}
                        onClick={() => fireAndForget(() => model.pickFolder())}
                    >
                        <i className="fa fa-folder-open text-[13px]" />
                        Open Folder…
                    </button>

                    <input
                        autoFocus
                        spellCheck={false}
                        autoComplete="off"
                        className="w-full h-10 px-3.5 rounded-lg bg-hover text-[12.5px] text-foreground outline-none border border-transparent transition-colors placeholder:text-muted hover:border-border/60 focus:border-white/25"
                        placeholder="…or type a folder path and press Enter"
                        value={pathInput}
                        onChange={(e) => setPathInput(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === "Enter") {
                                submitPath();
                            }
                        }}
                    />

                    {recents.length > 0 && (
                        <div className="flex flex-col gap-1 mt-1.5">
                            <div className="px-0.5 text-muted text-xxs font-medium uppercase tracking-wider">
                                Recent projects
                            </div>
                            {recents.map((dir) => (
                                <div
                                    key={dir}
                                    className="flex items-center gap-2.5 px-3 h-9 rounded-lg bg-hover/40 hover:bg-hover transition-colors cursor-pointer"
                                    title={dir}
                                    onClick={() => !busy && fireAndForget(() => model.openProject(dir))}
                                >
                                    <i className="fa fa-folder text-[12px] text-muted shrink-0" />
                                    <span className="text-[12.5px] font-medium shrink-0">{projectName(dir)}</span>
                                    <span className="text-muted text-[11px] truncate min-w-0">{dir}</span>
                                </div>
                            ))}
                        </div>
                    )}

                    {error && <div className="text-[11.5px] text-error">{error}</div>}

                    {recents.length === 0 && (
                        <div className="flex flex-col gap-1.5 mt-1.5">
                            <div className="px-0.5 text-muted text-xxs font-medium uppercase tracking-wider">
                                Quick start
                            </div>
                            <div className="grid grid-cols-3 gap-2">
                                {[
                                    ["1", "Open a project — Quasar starts a terminal in it."],
                                    ["2", "Run an agent — codex, claude, or any CLI."],
                                    ["3", "Track it in Agents & Accounts (right rail)."],
                                ].map(([n, text]) => (
                                    <div key={n} className="flex items-start gap-2 rounded-lg bg-hover/40 p-2.5">
                                        <span className="w-4 h-4 mt-[1px] rounded-full bg-accent/20 text-accent text-[10px] font-semibold flex items-center justify-center shrink-0">
                                            {n}
                                        </span>
                                        <span className="text-[11px] leading-tight text-secondary">{text}</span>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}

                    <button
                        className="self-center mt-1 px-2 py-1 rounded text-muted text-[12px] cursor-pointer transition-colors hover:text-foreground"
                        onClick={() => fireAndForget(() => model.openHomeTerminal())}
                    >
                        Skip — start a terminal in the home folder
                    </button>
                </div>
            </div>
        </div>
    );
}

export default ProjectLauncherView;
