// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { BuilderAppPanel } from "@/builder/builder-apppanel";
import { BuilderBuildPanel } from "@/builder/builder-buildpanel";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { atoms } from "@/store/global";
import { cn } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useState } from "react";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import { debounce } from "throttle-debounce";

const DefaultLayoutPercentages = {
    app: 80,
    build: 20,
};

const BuilderWorkspace = memo(() => {
    const builderId = useAtomValue(atoms.builderId);
    const [layout, setLayout] = useState<Record<string, number>>(null);
    const [isLoading, setIsLoading] = useState(true);
    const focusType = useAtomValue(BuilderFocusManager.getInstance().focusType);
    const isAppFocused = focusType === "app";

    useEffect(() => {
        const loadLayout = async () => {
            if (!builderId) {
                setLayout(DefaultLayoutPercentages);
                setIsLoading(false);
                return;
            }

            try {
                const rtInfo = await RpcApi.GetRTInfoCommand(TabRpcClient, {
                    oref: `builder:${builderId}`,
                });
                if (rtInfo?.["builder:layout"]) {
                    setLayout(rtInfo["builder:layout"] as Record<string, number>);
                } else {
                    setLayout(DefaultLayoutPercentages);
                }
            } catch (error) {
                console.error("Failed to load builder layout:", error);
                setLayout(DefaultLayoutPercentages);
            } finally {
                setIsLoading(false);
            }
        };

        loadLayout();
    }, [builderId]);

    const saveLayout = useCallback(
        debounce(500, (newLayout: Record<string, number>) => {
            if (!builderId) return;

            RpcApi.SetRTInfoCommand(TabRpcClient, {
                oref: `builder:${builderId}`,
                data: {
                    "builder:layout": newLayout,
                },
            }).catch((error) => {
                console.error("Failed to save builder layout:", error);
            });
        }),
        [builderId]
    );

    const handleVerticalLayout = useCallback(
        (sizes: number[]) => {
            const newLayout = { ...layout, app: sizes[0], build: sizes[1] };
            setLayout(newLayout);
            saveLayout(newLayout);
        },
        [layout, saveLayout]
    );

    if (isLoading || !layout) {
        return null;
    }

    return (
        <div className="flex-1 overflow-hidden">
            <PanelGroup direction="vertical" onLayout={handleVerticalLayout}>
                <Panel defaultSize={layout.app} minSize={20}>
                    <div
                        className={cn(
                            "flex flex-col relative h-full",
                            isAppFocused ? "border-2 border-accent" : "border-2 border-transparent"
                        )}
                        style={{
                            borderBottomRightRadius: 8,
                        }}
                    >
                        <BuilderAppPanel />
                    </div>
                </Panel>
                <PanelResizeHandle className="h-0.5 bg-transparent hover:bg-gray-500/20 transition-colors" />
                <Panel defaultSize={layout.build} minSize={20} maxSize={50} style={{ borderBottomRightRadius: 8 }}>
                    <BuilderBuildPanel />
                </Panel>
            </PanelGroup>
        </div>
    );
});

BuilderWorkspace.displayName = "BuilderWorkspace";

export { BuilderWorkspace };
