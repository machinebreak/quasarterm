// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import {
    atoms,
    createBlock,
    getApi,
    getOrefMetaKeyAtom,
    getTabMetaKeyAtom,
    globalStore,
    replaceBlock,
} from "@/app/store/global";
import { ObjectService } from "@/app/store/services";
import { getObjectValue, makeORef } from "@/app/store/wos";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { fireAndForget, isBlank } from "@/util/util";

const MaxClosedTabs = 10;
const SnapshotTimeoutMs = 1500;
const NewTabWaitTimeoutMs = 5000;
const LauncherWaitTimeoutMs = 4000;
const ReplaceBlockRetryCount = 6;
const ReplaceBlockRetryDelayMs = 300;

type TabSnapshot = {
    name: string;
    cwd: string;
    flagColor: string;
    blockDefs: BlockDef[];
};

const closedTabs: TabSnapshot[] = [];

function sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
}

async function readBlockDefs(blockIds: string[]): Promise<BlockDef[]> {
    const cached = new Map<string, Block>();
    const missingOrefs: string[] = [];
    for (const blockId of blockIds ?? []) {
        const oref = makeORef("block", blockId);
        if (oref == null) {
            continue;
        }
        const block = getObjectValue<Block>(oref);
        if (block == null) {
            missingOrefs.push(oref);
        } else {
            cached.set(blockId, block);
        }
    }
    // Blocks of tabs that were never opened may not be loaded in the
    // frontend store yet; fetch them in one batch from the server.
    if (missingOrefs.length > 0) {
        try {
            const fetched = await ObjectService.GetObjects(missingOrefs);
            for (const block of fetched ?? []) {
                if (block?.otype === "block") {
                    cached.set(block.oid, block as Block);
                }
            }
        } catch (e) {
            console.log("tabops: error fetching blocks for snapshot", e);
        }
    }
    const blockDefs: BlockDef[] = [];
    for (const blockId of blockIds ?? []) {
        const block = cached.get(blockId);
        if (block?.meta == null) {
            continue;
        }
        blockDefs.push({ meta: block.meta });
    }
    return blockDefs;
}

async function snapshotTab(tabId: string): Promise<TabSnapshot> {
    if (tabId == null) {
        return null;
    }
    const tabORef = makeORef("tab", tabId);
    let tab = getObjectValue<Tab>(tabORef);
    if (tab == null) {
        tab = (await ObjectService.GetObject(tabORef)) as Tab;
    }
    if (tab == null) {
        return null;
    }
    const blockDefs = await readBlockDefs(tab.blockids);
    return {
        name: tab.name,
        cwd: globalStore.get(getTabMetaKeyAtom(tabId, "cmd:cwd")) ?? null,
        flagColor: globalStore.get(getOrefMetaKeyAtom(tabORef, "tab:flagcolor")) ?? null,
        blockDefs,
    };
}

// Snapshots the tab before it is torn down so it can be restored with
// reopenClosedTab (Ctrl/Cmd+Shift+T). A slow snapshot is abandoned after
// SnapshotTimeoutMs so closing is not delayed.
async function takeTabSnapshot(tabId: string): Promise<TabSnapshot> {
    const result = await Promise.race([
        snapshotTab(tabId).catch((e) => {
            console.log("tabops: error snapshotting tab", e);
            return null;
        }),
        sleep(SnapshotTimeoutMs).then(() => null),
    ]);
    return result;
}

async function closeTabAndRecord(closeFn: () => Promise<boolean>, tabId: string): Promise<boolean> {
    const snap = await takeTabSnapshot(tabId);
    const didClose = await closeFn();
    if (didClose && snap != null) {
        closedTabs.unshift(snap);
        if (closedTabs.length > MaxClosedTabs) {
            closedTabs.length = MaxClosedTabs;
        }
    }
    return didClose;
}

function waitForStaticTabChange(prevTabId: string): Promise<string> {
    return new Promise((resolve) => {
        let done = false;
        const finish = (tabId: string) => {
            if (done) {
                return;
            }
            done = true;
            clearTimeout(timer);
            unsub();
            resolve(tabId);
        };
        const check = () => {
            const tabId = globalStore.get(atoms.staticTabId);
            if (tabId != null && tabId !== prevTabId) {
                finish(tabId);
            }
        };
        const unsub = globalStore.sub(atoms.staticTabId, check);
        const timer = setTimeout(() => finish(null), NewTabWaitTimeoutMs);
        check();
    });
}

async function waitForLauncherBlock(tabId: string): Promise<string> {
    const deadline = Date.now() + LauncherWaitTimeoutMs;
    const tabORef = makeORef("tab", tabId);
    while (Date.now() < deadline) {
        let tab = getObjectValue<Tab>(tabORef);
        if (tab == null) {
            try {
                tab = (await ObjectService.GetObject(tabORef)) as Tab;
            } catch (e) {
                tab = null;
            }
        }
        for (const blockId of tab?.blockids ?? []) {
            const block = await findBlock(blockId);
            if (block?.meta?.["view"] === "projectlauncher") {
                return blockId;
            }
        }
        await sleep(150);
    }
    return null;
}

async function findBlock(blockId: string): Promise<Block> {
    const blockORef = makeORef("block", blockId);
    const cached = getObjectValue<Block>(blockORef);
    if (cached != null) {
        return cached;
    }
    try {
        return (await ObjectService.GetObject(blockORef)) as Block;
    } catch (e) {
        return null;
    }
}

async function replaceBlockWithRetry(blockId: string, blockDef: BlockDef, focus: boolean): Promise<void> {
    let lastError: any = null;
    for (let attempt = 0; attempt < ReplaceBlockRetryCount; attempt++) {
        try {
            await replaceBlock(blockId, blockDef, focus);
            return;
        } catch (e) {
            lastError = e;
            await sleep(ReplaceBlockRetryDelayMs);
        }
    }
    throw lastError;
}

async function createTabFromSnapshot(snap: TabSnapshot): Promise<string> {
    if (snap == null) {
        return null;
    }
    const prevTabId = globalStore.get(atoms.staticTabId);
    getApi().createTab();
    const newTabId = await waitForStaticTabChange(prevTabId);
    if (newTabId == null) {
        console.log("tabops: new tab did not become active");
        return null;
    }
    let replaceTarget: string = null;
    if ((snap.blockDefs ?? []).length > 0) {
        replaceTarget = await waitForLauncherBlock(newTabId);
    }
    for (const blockDef of snap.blockDefs ?? []) {
        try {
            if (replaceTarget != null) {
                await replaceBlockWithRetry(replaceTarget, blockDef, true);
                replaceTarget = null;
                continue;
            }
            await createBlock(blockDef);
        } catch (e) {
            replaceTarget = null;
            console.log("tabops: error restoring block", e);
        }
    }
    if (snap.name != null && snap.name !== "") {
        fireAndForget(() => RpcApi.UpdateTabNameCommand(TabRpcClient, newTabId, snap.name));
    }
    if (snap.cwd != null) {
        fireAndForget(() =>
            RpcApi.SetMetaCommand(TabRpcClient, { oref: makeORef("tab", newTabId), meta: { "cmd:cwd": snap.cwd } })
        );
    }
    if (snap.flagColor != null) {
        fireAndForget(() =>
            RpcApi.SetMetaCommand(TabRpcClient, {
                oref: makeORef("tab", newTabId),
                meta: { "tab:flagcolor": snap.flagColor },
            })
        );
    }
    return newTabId;
}

async function duplicateTab(tabId: string): Promise<string> {
    const snap = await snapshotTab(tabId);
    return createTabFromSnapshot(snap);
}

async function reopenClosedTab(): Promise<string> {
    const snap = closedTabs.shift();
    if (snap == null) {
        return null;
    }
    const newTabId = await createTabFromSnapshot(snap);
    if (newTabId == null) {
        closedTabs.unshift(snap);
    }
    return newTabId;
}

function hasClosedTabs(): boolean {
    return closedTabs.length > 0;
}

async function isLastTabInWorkspace(workspaceId: string, tabId: string): Promise<boolean> {
    if (isBlank(workspaceId) || isBlank(tabId)) {
        return false;
    }
    const wsORef = makeORef("workspace", workspaceId);
    let ws = getObjectValue<Workspace>(wsORef);
    if (ws == null) {
        try {
            ws = (await ObjectService.GetObject(wsORef)) as Workspace;
        } catch (e) {
            ws = null;
        }
    }
    const tabIds = ws?.tabids ?? [];
    return tabIds.length > 0 && tabIds.every((id) => id === tabId);
}

// Closing the last tab of a workspace used to shut the whole window down (and
// take the workspace with it). Instead, open a fresh "open a project" launcher
// tab first, then close the original through the normal path (snapshot +
// close, so Ctrl+Shift+T can still bring it back). The app stays alive and
// whatever else was in the workspace survives restarts.
async function closeTabOrReset(workspaceId: string, tabId: string, confirmClose: boolean): Promise<boolean> {
    if (!(await isLastTabInWorkspace(workspaceId, tabId))) {
        return closeTabAndRecord(() => getApi().closeTab(workspaceId, tabId, confirmClose), tabId);
    }
    const prevActiveTabId = globalStore.get(atoms.staticTabId);
    getApi().createTab();
    const newTabId = await waitForStaticTabChange(prevActiveTabId);
    if (newTabId == null) {
        console.log("tabops: could not open a replacement tab; keeping the last tab open");
        return false;
    }
    const didClose = await closeTabAndRecord(() => getApi().closeTab(workspaceId, tabId, confirmClose), tabId);
    if (!didClose) {
        // the user canceled the close confirmation — drop the replacement launcher too
        fireAndForget(() => getApi().closeTab(workspaceId, newTabId, false));
    }
    return didClose;
}

export { closeTabAndRecord, closeTabOrReset, duplicateTab, hasClosedTabs, reopenClosedTab };
export type { TabSnapshot };
