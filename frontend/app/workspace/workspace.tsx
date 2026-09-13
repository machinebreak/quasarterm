// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { ErrorBoundary } from "@/app/element/errorboundary";
import { CenteredDiv } from "@/app/element/quickelems";
import { ModalsRenderer } from "@/app/modals/modalsrenderer";
import { TabBar } from "@/app/tab/tabbar";
import { TabContent } from "@/app/tab/tabcontent";
import { VTabBar } from "@/app/tab/vtabbar";
import { Widgets } from "@/app/workspace/widgets";
import { WorkspaceLayoutModel } from "@/app/workspace/workspace-layout-model";
import { atoms, getSettingsKeyAtom } from "@/store/global";
import { isMacOS } from "@/util/platformutil";
import { useAtomValue } from "jotai";
import { memo, useEffect, useRef } from "react";
import {
    ImperativePanelGroupHandle,
    ImperativePanelHandle,
    Panel,
    PanelGroup,
    PanelResizeHandle,
} from "react-resizable-panels";

const MacOSTabBarSpacer = memo(() => {
    return (
        <div
            className="w-full shrink-0"
            style={
                {
                    height: "calc(8px * var(--zoomfactor-inv))",
                    WebkitAppRegion: "drag",
                    backdropFilter: "blur(20px)",
                    background: "rgba(0, 0, 0, 0.35)",
                } as React.CSSProperties
            }
        />
    );
});
MacOSTabBarSpacer.displayName = "MacOSTabBarSpacer";

const WorkspaceElem = memo(() => {
    const workspaceLayoutModel = WorkspaceLayoutModel.getInstance();
    const tabId = useAtomValue(atoms.staticTabId);
    const ws = useAtomValue(atoms.workspace);
    const tabBarPosition = useAtomValue(getSettingsKeyAtom("app:tabbar")) ?? "top";
    const showLeftTabBar = tabBarPosition === "left";
    const widgetsSidebarVisible = useAtomValue(workspaceLayoutModel.widgetsSidebarVisibleAtom);
    const windowWidth = window.innerWidth;
    const vtabInitialPct = workspaceLayoutModel.getVTabInitialPercentage(windowWidth);
    const outerPanelGroupRef = useRef<ImperativePanelGroupHandle>(null);
    const vtabPanelRef = useRef<ImperativePanelHandle>(null);
    const panelContainerRef = useRef<HTMLDivElement>(null);
    const vtabPanelWrapperRef = useRef<HTMLDivElement>(null);

    // showLeftTabBar is passed as a seed value only; subsequent changes are handled by setShowLeftTabBar below.
    // Do NOT add showLeftTabBar as a dep here — re-registering refs on config changes would redundantly re-run commitLayouts.
    useEffect(() => {
        if (
            vtabPanelRef.current &&
            outerPanelGroupRef.current &&
            panelContainerRef.current &&
            vtabPanelWrapperRef.current
        ) {
            workspaceLayoutModel.registerRefs(
                vtabPanelRef.current,
                outerPanelGroupRef.current,
                panelContainerRef.current,
                vtabPanelWrapperRef.current,
                showLeftTabBar
            );
        }
    }, []);

    useEffect(() => {
        window.addEventListener("resize", workspaceLayoutModel.handleWindowResize);
        return () => window.removeEventListener("resize", workspaceLayoutModel.handleWindowResize);
    }, []);

    useEffect(() => {
        workspaceLayoutModel.setShowLeftTabBar(showLeftTabBar);
    }, [showLeftTabBar]);

    useEffect(() => {
        const handleFocus = () => workspaceLayoutModel.syncVTabWidthFromMeta();
        window.addEventListener("focus", handleFocus);
        return () => window.removeEventListener("focus", handleFocus);
    }, []);

    const vtabHandleClass = `bg-transparent hover:bg-zinc-500/20 transition-colors ${showLeftTabBar ? "w-0.5" : "w-0 pointer-events-none"}`;

    return (
        <div className="flex flex-col w-full flex-grow overflow-hidden">
            {!(showLeftTabBar && isMacOS()) && <TabBar key={ws.oid} workspace={ws} noTabs={showLeftTabBar} />}
            {showLeftTabBar && isMacOS() && <MacOSTabBarSpacer />}
            <div ref={panelContainerRef} className="flex flex-row flex-grow overflow-hidden">
                <ErrorBoundary key={tabId}>
                    <PanelGroup
                        direction="horizontal"
                        onLayout={workspaceLayoutModel.handleOuterPanelLayout}
                        ref={outerPanelGroupRef}
                    >
                        <Panel
                            ref={vtabPanelRef}
                            collapsible
                            defaultSize={vtabInitialPct}
                            order={0}
                            className="overflow-hidden"
                        >
                            <div ref={vtabPanelWrapperRef} className="w-full h-full">
                                {showLeftTabBar && <VTabBar workspace={ws} />}
                            </div>
                        </Panel>
                        <PanelResizeHandle className={vtabHandleClass} />
                        <Panel order={1} defaultSize={100 - vtabInitialPct}>
                            {tabId === "" ? (
                                <CenteredDiv>No Active Tab</CenteredDiv>
                            ) : (
                                <div className="flex flex-row h-full">
                                    <TabContent key={tabId} tabId={tabId} noTopPadding={showLeftTabBar && isMacOS()} />
                                    {widgetsSidebarVisible && <Widgets />}
                                </div>
                            )}
                        </Panel>
                    </PanelGroup>
                    <ModalsRenderer />
                </ErrorBoundary>
            </div>
        </div>
    );
});

WorkspaceElem.displayName = "WorkspaceElem";

export { WorkspaceElem as Workspace };
