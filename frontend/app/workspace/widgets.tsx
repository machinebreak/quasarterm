// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Tooltip } from "@/app/element/tooltip";
import { globalStore, refocusNode, withProjectCwd } from "@/app/store/global";
import * as WOS from "@/app/store/wos";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { useWaveEnv, WaveEnv, WaveEnvSubset } from "@/app/waveenv/waveenv";
import { shouldIncludeWidgetForWorkspace } from "@/app/workspace/widgetfilter";
import { modalsModel } from "@/store/modalmodel";
import { fireAndForget, makeIconClass } from "@/util/util";
import {
    autoUpdate,
    FloatingPortal,
    offset,
    shift,
    useDismiss,
    useFloating,
    useInteractions,
} from "@floating-ui/react";
import clsx from "clsx";
import { useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { getCanvasModel } from "../canvas/canvas-model";

export type WidgetsEnv = WaveEnvSubset<{
    isDev: WaveEnv["isDev"];
    electron: {
        openBuilder: WaveEnv["electron"]["openBuilder"];
    };
    rpc: {
        ListAllAppsCommand: WaveEnv["rpc"]["ListAllAppsCommand"];
        SetConfigCommand: WaveEnv["rpc"]["SetConfigCommand"];
    };
    atoms: {
        fullConfigAtom: WaveEnv["atoms"]["fullConfigAtom"];
        hasConfigErrors: WaveEnv["atoms"]["hasConfigErrors"];
        workspaceId: WaveEnv["atoms"]["workspaceId"];
    };
    createBlock: WaveEnv["createBlock"];
    showContextMenu: WaveEnv["showContextMenu"];
}>;

function sortByDisplayOrder(wmap: { [key: string]: WidgetConfigType }): WidgetConfigType[] {
    if (wmap == null) {
        return [];
    }
    const wlist = Object.values(wmap);
    wlist.sort((a, b) => {
        return (a["display:order"] ?? 0) - (b["display:order"] ?? 0);
    });
    return wlist;
}

type WidgetPropsType = {
    widget: WidgetConfigType;
    env: WidgetsEnv;
};

// Widget panels are singletons per tab: clicking the rail icon of an
// already-open widget focuses it instead of spawning a duplicate panel.
// Terminals are exempt (multiple terminal windows are normal).
const WidgetDedupeExemptViews = new Set(["term"]);

// Finds an already-open widget block (by view type) in the current tab.
export function findWidgetBlockInCurrentTab(view: string, env: WaveEnv): string {
    const tabId = globalStore.get(env.atoms.staticTabId);
    if (tabId == null) {
        return null;
    }
    const tab = globalStore.get(env.wos.getWaveObjectAtom<Tab>(WOS.makeORef("tab", tabId)));
    for (const blockId of tab?.blockids ?? []) {
        const block = globalStore.get(env.wos.getWaveObjectAtom<Block>(WOS.makeORef("block", blockId)));
        if (block?.meta?.view === view) {
            return blockId;
        }
    }
    return null;
}

function focusWidgetBlock(blockId: string, env: WaveEnv) {
    try {
        const tabId = globalStore.get(env.atoms.staticTabId);
        const canvasModel = getCanvasModel(tabId);
        canvasModel?.focusBlock(blockId); // raises + focuses in canvas mode; harmless otherwise
    } catch (e) {
        // best effort
    }
    try {
        refocusNode(blockId); // keyboard focus in tiled mode
    } catch (e) {
        // best effort
    }
}

async function handleWidgetSelect(widget: WidgetConfigType, env: WidgetsEnv, fullEnv?: WaveEnv) {
    const view = widget.blockdef?.meta?.view;
    if (fullEnv != null && view != null && !WidgetDedupeExemptViews.has(view)) {
        const existingBlockId = findWidgetBlockInCurrentTab(view, fullEnv);
        if (existingBlockId != null) {
            focusWidgetBlock(existingBlockId, fullEnv);
            return;
        }
    }
    const blockDef = withProjectCwd(widget.blockdef);
    env.createBlock(blockDef, widget.magnified);
}

const Widget = memo(({ widget, env }: WidgetPropsType) => {
    const fullEnv = useWaveEnv();
    return (
        <Tooltip
            content={widget.description || widget.label}
            placement="left"
            divClassName={clsx(
                "flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-secondary text-[17px] cursor-pointer transition-colors hover:bg-hoverbg hover:text-white",
                widget["display:hidden"] && "hidden"
            )}
            divOnClick={() => handleWidgetSelect(widget, env, fullEnv)}
        >
            <div style={{ color: widget.color }}>
                <i className={makeIconClass(widget.icon, true, { defaultIcon: "browser" })}></i>
            </div>
        </Tooltip>
    );
});

function calculateGridSize(appCount: number): number {
    if (appCount <= 4) return 2;
    if (appCount <= 9) return 3;
    if (appCount <= 16) return 4;
    if (appCount <= 25) return 5;
    return 6;
}

function SettingsTooltipContent({ hasConfigErrors }: { hasConfigErrors: boolean }) {
    if (!hasConfigErrors) {
        return "Settings & Help";
    }
    return (
        <div className="flex flex-col p-1">
            <div className="mb-1">Settings &amp; Help</div>
            <div className="flex items-center gap-1 mt-0.5 text-error">
                <i className="fa fa-solid fa-circle-exclamation"></i>
                <span>Config Errors</span>
            </div>
        </div>
    );
}

type FloatingWindowPropsType = {
    isOpen: boolean;
    onClose: () => void;
    referenceElement: HTMLElement;
    hasConfigErrors?: boolean;
};

const AppsFloatingWindow = memo(({ isOpen, onClose, referenceElement }: FloatingWindowPropsType) => {
    const [apps, setApps] = useState<AppInfo[]>([]);
    const [loading, setLoading] = useState(true);
    const env = useWaveEnv<WidgetsEnv>();

    const { refs, floatingStyles, context } = useFloating({
        open: isOpen,
        onOpenChange: onClose,
        placement: "left-start",
        middleware: [offset(-2), shift({ padding: 12 })],
        whileElementsMounted: autoUpdate,
        elements: {
            reference: referenceElement,
        },
    });

    const dismiss = useDismiss(context);
    const { getFloatingProps } = useInteractions([dismiss]);
    const handleOpenBuilder = useCallback(() => {
        env.electron.openBuilder(null);
        onClose();
    }, [onClose, env]);

    useEffect(() => {
        if (!isOpen) return;

        const fetchApps = async () => {
            setLoading(true);
            try {
                const allApps = await env.rpc.ListAllAppsCommand(TabRpcClient);
                const localApps = allApps
                    .filter((app) => !app.appid.startsWith("draft/"))
                    .sort((a, b) => {
                        const aName = a.appid.replace(/^local\//, "");
                        const bName = b.appid.replace(/^local\//, "");
                        return aName.localeCompare(bName);
                    });
                setApps(localApps);
            } catch (error) {
                console.error("Failed to fetch apps:", error);
                setApps([]);
            } finally {
                setLoading(false);
            }
        };

        fetchApps();
    }, [isOpen]);

    if (!isOpen) return null;

    const gridSize = calculateGridSize(apps.length);

    return (
        <FloatingPortal>
            <div
                ref={refs.setFloating}
                style={floatingStyles}
                {...getFloatingProps()}
                className="bg-modalbg border border-border rounded-lg shadow-xl z-50 overflow-hidden"
            >
                <div className="p-4">
                    {loading ? (
                        <div className="flex items-center justify-center p-8">
                            <i className="fa fa-solid fa-spinner fa-spin text-2xl text-muted"></i>
                        </div>
                    ) : apps.length === 0 ? (
                        <div className="text-muted text-sm p-4 text-center">No local apps found</div>
                    ) : (
                        <div
                            className="grid gap-3"
                            style={{
                                gridTemplateColumns: `repeat(${gridSize}, minmax(0, 1fr))`,
                                maxWidth: `${gridSize * 80}px`,
                            }}
                        >
                            {apps.map((app) => {
                                const appMeta = app.manifest?.appmeta;
                                const displayName = app.appid.replace(/^local\//, "");
                                const icon = appMeta?.icon || "cube";
                                const iconColor = appMeta?.iconcolor || "white";

                                return (
                                    <div
                                        key={app.appid}
                                        className="flex flex-col items-center justify-center p-2 rounded hover:bg-hoverbg cursor-pointer transition-colors"
                                        onClick={() => {
                                            const blockDef: BlockDef = {
                                                meta: {
                                                    view: "tsunami",
                                                    controller: "tsunami",
                                                    "tsunami:appid": app.appid,
                                                },
                                            };
                                            env.createBlock(blockDef);
                                            onClose();
                                        }}
                                    >
                                        <div style={{ color: iconColor }} className="text-3xl mb-1">
                                            <i className={makeIconClass(icon, false)}></i>
                                        </div>
                                        <div className="text-xxs text-center text-secondary break-words w-full px-1">
                                            {displayName}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
                <button
                    type="button"
                    className="w-full px-4 py-2 border-t border-border text-xs text-secondary text-center hover:bg-hoverbg hover:text-white transition-colors cursor-pointer flex items-center justify-center gap-2"
                    onClick={handleOpenBuilder}
                >
                    <i className="fa fa-solid fa-hammer"></i>
                    Build/Edit Apps
                </button>
            </div>
        </FloatingPortal>
    );
});

const SettingsFloatingWindow = memo(
    ({ isOpen, onClose, referenceElement, hasConfigErrors }: FloatingWindowPropsType) => {
        const env = useWaveEnv<WidgetsEnv>();
        const fullConfig = useAtomValue(env.atoms.fullConfigAtom);
        const tabBarPosition = fullConfig?.settings?.["app:tabbar"] ?? "top";
        const { refs, floatingStyles, context } = useFloating({
            open: isOpen,
            onOpenChange: onClose,
            placement: "left-start",
            middleware: [offset(-2), shift({ padding: 12 })],
            whileElementsMounted: autoUpdate,
            elements: {
                reference: referenceElement,
            },
        });

        const dismiss = useDismiss(context);
        const { getFloatingProps } = useInteractions([dismiss]);

        if (!isOpen) return null;

        const menuItems = [
            {
                icon: "gear",
                label: "Settings",
                hasError: hasConfigErrors,
                onClick: () => {
                    const blockDef: BlockDef = {
                        meta: {
                            view: "waveconfig",
                        },
                    };
                    env.createBlock(blockDef, false, true);
                    onClose();
                },
            },
            {
                icon: "arrow-up",
                label: "Tab Bar: Top",
                checked: tabBarPosition === "top",
                onClick: () => {
                    fireAndForget(() => env.rpc.SetConfigCommand(TabRpcClient, { "app:tabbar": "top" }));
                    onClose();
                },
            },
            {
                icon: "table-columns",
                label: "Tab Bar: Side",
                checked: tabBarPosition === "left",
                onClick: () => {
                    fireAndForget(() => env.rpc.SetConfigCommand(TabRpcClient, { "app:tabbar": "left" }));
                    onClose();
                },
            },
            {
                icon: "lightbulb",
                label: "Tips",
                onClick: () => {
                    const blockDef: BlockDef = {
                        meta: {
                            view: "tips",
                        },
                    };
                    env.createBlock(blockDef, true, true);
                    onClose();
                },
            },
            {
                icon: "lock",
                label: "Secrets",
                onClick: () => {
                    const blockDef: BlockDef = {
                        meta: {
                            view: "waveconfig",
                            file: "secrets",
                        },
                    };
                    env.createBlock(blockDef, false, true);
                    onClose();
                },
            },
            {
                icon: "book-open",
                label: "Release Notes",
                onClick: () => {
                    modalsModel.pushModal("UpgradeOnboardingPatch", { isReleaseNotes: true });
                    onClose();
                },
            },
            {
                icon: "circle-question",
                label: "Help",
                onClick: () => {
                    const blockDef: BlockDef = {
                        meta: {
                            view: "help",
                        },
                    };
                    env.createBlock(blockDef);
                    onClose();
                },
            },
        ];

        return (
            <FloatingPortal>
                <div
                    ref={refs.setFloating}
                    style={floatingStyles}
                    {...getFloatingProps()}
                    className="bg-modalbg border border-border rounded-lg shadow-xl p-2 z-50"
                >
                    {menuItems.map((item, idx) => (
                        <div
                            key={idx}
                            className="flex items-center gap-3 px-3 py-2 rounded hover:bg-hoverbg cursor-pointer transition-colors text-secondary hover:text-white"
                            onClick={item.onClick}
                        >
                            <div className="text-lg w-5 flex justify-center">
                                <i className={makeIconClass(item.icon, false)}></i>
                            </div>
                            <div className="text-sm whitespace-nowrap">{item.label}</div>
                            {item.checked && <i className="fa fa-solid fa-check text-accent text-[13px] ml-auto"></i>}
                            {item.hasError && (
                                <i className="fa fa-solid fa-circle-exclamation text-error text-[14px] ml-auto"></i>
                            )}
                        </div>
                    ))}
                </div>
            </FloatingPortal>
        );
    }
);

SettingsFloatingWindow.displayName = "SettingsFloatingWindow";

const Widgets = memo(() => {
    const env = useWaveEnv<WidgetsEnv>();
    const fullConfig = useAtomValue(env.atoms.fullConfigAtom);
    const hasConfigErrors = useAtomValue(env.atoms.hasConfigErrors);
    const workspaceId = useAtomValue(env.atoms.workspaceId);
    const containerRef = useRef<HTMLDivElement>(null);

    const featureWaveAppBuilder = fullConfig?.settings?.["feature:waveappbuilder"] ?? false;
    const widgetsMap = fullConfig?.widgets ?? {};
    const filteredWidgets = Object.fromEntries(
        Object.entries(widgetsMap).filter(([_key, widget]) => shouldIncludeWidgetForWorkspace(widget, workspaceId))
    );
    const widgets = sortByDisplayOrder(filteredWidgets);

    const [isAppsOpen, setIsAppsOpen] = useState(false);
    const appsButtonRef = useRef<HTMLDivElement>(null);
    const [isSettingsOpen, setIsSettingsOpen] = useState(false);
    const settingsButtonRef = useRef<HTMLDivElement>(null);

    const handleWidgetsBarContextMenu = (e: React.MouseEvent) => {
        e.preventDefault();
        const menu: ContextMenuItem[] = [
            {
                label: "Edit widgets.json",
                click: () => {
                    fireAndForget(async () => {
                        const blockDef: BlockDef = {
                            meta: {
                                view: "waveconfig",
                                file: "widgets.json",
                            },
                        };
                        await env.createBlock(blockDef, false, true);
                    });
                },
            },
        ];
        env.showContextMenu(menu, e);
    };

    return (
        <>
            <div
                ref={containerRef}
                className="flex flex-col w-12 overflow-hidden py-1.5 -ml-1 select-none shrink-0"
                onContextMenu={handleWidgetsBarContextMenu}
            >
                <div className="flex flex-col items-center gap-1 w-full flex-1 min-h-0 overflow-y-auto">
                    {widgets?.map((data, idx) => (
                        <Widget key={`widget-${idx}`} widget={data} env={env} />
                    ))}
                </div>
                <div className="w-6 h-px my-1.5 shrink-0 bg-border" />
                <div className="flex flex-col items-center gap-1 w-full shrink-0">
                    {env.isDev() || featureWaveAppBuilder ? (
                        <div
                            ref={appsButtonRef}
                            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-secondary text-[17px] cursor-pointer transition-colors hover:bg-hoverbg hover:text-white"
                            onClick={() => setIsAppsOpen(!isAppsOpen)}
                        >
                            <Tooltip content="Local Apps" placement="left" disable={isAppsOpen}>
                                <div>
                                    <i className={makeIconClass("cube", true)}></i>
                                </div>
                            </Tooltip>
                        </div>
                    ) : null}
                    <div
                        ref={settingsButtonRef}
                        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-secondary text-[17px] cursor-pointer transition-colors hover:bg-hoverbg hover:text-white"
                        onClick={() => setIsSettingsOpen(!isSettingsOpen)}
                    >
                        <Tooltip
                            content={<SettingsTooltipContent hasConfigErrors={hasConfigErrors} />}
                            placement="left"
                            disable={isSettingsOpen}
                        >
                            <div className="relative">
                                <i className={makeIconClass("gear", true)}></i>
                                {hasConfigErrors && (
                                    <i className="fa fa-solid fa-circle-exclamation text-error absolute -top-1 -right-1 text-[11px] pointer-events-none"></i>
                                )}
                            </div>
                        </Tooltip>
                    </div>
                    {env.isDev() ? (
                        <div
                            className="flex h-9 w-9 shrink-0 items-center justify-center text-accent/80 text-[20px]"
                            title="Running Wave Dev Build"
                        >
                            <i className="fa fa-brands fa-dev fa-fw" />
                        </div>
                    ) : null}
                </div>
            </div>
            {(env.isDev() || featureWaveAppBuilder) && appsButtonRef.current && (
                <AppsFloatingWindow
                    isOpen={isAppsOpen}
                    onClose={() => setIsAppsOpen(false)}
                    referenceElement={appsButtonRef.current}
                />
            )}
            {settingsButtonRef.current && (
                <SettingsFloatingWindow
                    isOpen={isSettingsOpen}
                    onClose={() => setIsSettingsOpen(false)}
                    referenceElement={settingsButtonRef.current}
                    hasConfigErrors={hasConfigErrors}
                />
            )}
        </>
    );
});

export { Widgets };
