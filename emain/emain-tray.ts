// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { Menu, Tray, app, nativeImage } from "electron";
import path from "path";
import { fireAndForget } from "../frontend/util/util";
import { RpcApi } from "@/app/store/wshclientapi";
import { log } from "./emain-log";
import { getElectronAppBasePath } from "./emain-platform";
import { ElectronWshClient } from "./emain-wsh";
import { createNewWaveWindow, getAllWaveWindows } from "./emain-window";

const trayPollMs = 60_000;

let tray: Tray = null;
let pollTimer: NodeJS.Timeout = null;
let trayEnabled = false;
let pollTick = 0;
let cachedAccounts: Account[] = [];

function makeTrayIcon() {
    const iconPath = path.join(getElectronAppBasePath(), "public/logos/quasar-appicon.png");
    const image = nativeImage.createFromPath(iconPath);
    if (image.isEmpty()) {
        return image;
    }
    return image.resize({ width: 16, height: 16 });
}

function quotaLines(accounts: Account[]): string[] {
    const byProvider = new Map<string, Account[]>();
    for (const account of accounts) {
        const list = byProvider.get(account.provider) ?? [];
        list.push(account);
        byProvider.set(account.provider, list);
    }
    const lines: string[] = [];
    for (const [provider, list] of byProvider) {
        const account = list.find((entry) => entry.isactive) ?? list[0];
        if (account == null) {
            continue;
        }
        const label = account.label || account.email || account.id;
        const metrics = account.quota?.metrics ?? [];
        if (metrics.length === 0) {
            lines.push(`${provider} · ${label} — no quota data`);
            continue;
        }
        let lowest: QuotaMetric = metrics[0];
        for (const metric of metrics) {
            if (metric.remainingpercent < lowest.remainingpercent) {
                lowest = metric;
            }
        }
        lines.push(`${provider} · ${label} — ${lowest.name} ${Math.round(lowest.remainingpercent)}%`);
    }
    return lines;
}

function tooltipText(accounts: Account[]): string {
    if (accounts.length === 0) {
        return "Quasar — no accounts";
    }
    const lines = quotaLines(accounts).slice(0, 3);
    return `Quasar — ${lines.join(" · ")}`;
}

function buildContextMenu(accounts: Account[]): Menu {
    const items: Electron.MenuItemConstructorOptions[] = [];
    const lines = quotaLines(accounts);
    if (lines.length === 0) {
        items.push({ label: "No accounts yet", enabled: false });
    } else {
        for (const line of lines.slice(0, 12)) {
            items.push({ label: line, enabled: false });
        }
    }
    items.push({ type: "separator" });
    items.push({
        label: "Refresh quotas",
        click: () => fireAndForget(refreshAllQuotas),
    });
    items.push({ label: "Show Quasar", click: () => focusFirstWindow() });
    items.push({ type: "separator" });
    items.push({ label: "Quit Quasar", click: () => app.quit() });
    return Menu.buildFromTemplate(items);
}

function focusFirstWindow() {
    for (const ww of getAllWaveWindows()) {
        if (ww != null && !ww.isDestroyed()) {
            if (ww.isMinimized()) {
                ww.restore();
            }
            ww.show();
            ww.focus();
            return;
        }
    }
    fireAndForget(createNewWaveWindow);
}

async function refreshAllQuotas() {
    try {
        for (const account of cachedAccounts) {
            try {
                await RpcApi.RefreshAccountQuotaCommand(ElectronWshClient, {
                    provider: account.provider,
                    accountid: account.id,
                });
            } catch (e) {
                log(`[tray] quota refresh failed for ${account.id}: ${e}`);
            }
        }
    } finally {
        await pollTray();
    }
}

async function pollTray() {
    if (!trayEnabled) {
        return;
    }
    try {
        pollTick++;
        if (pollTick % 5 === 1) {
            const fullConfig = await RpcApi.GetFullConfigCommand(ElectronWshClient);
            const setting = fullConfig?.settings?.["app:tray"];
            applyTrayEnabled(setting ?? true);
            if (!trayEnabled) {
                return;
            }
        }
        cachedAccounts = await RpcApi.ListAccountsCommand(ElectronWshClient, { provider: "" });
        if (tray != null) {
            tray.setToolTip(tooltipText(cachedAccounts));
            tray.setContextMenu(buildContextMenu(cachedAccounts));
        }
    } catch (e) {
        log(`[tray] poll failed: ${e}`);
    }
}

function applyTrayEnabled(enabled: boolean) {
    trayEnabled = enabled;
    if (enabled && tray == null) {
        try {
            tray = new Tray(makeTrayIcon());
            tray.on("click", () => focusFirstWindow());
            tray.setToolTip("Quasar");
            log("[tray] created");
        } catch (e) {
            tray = null;
            log(`[tray] failed to create: ${e}`);
        }
    } else if (!enabled && tray != null) {
        tray.destroy();
        tray = null;
        log("[tray] destroyed");
    }
}

export function initTray(fullConfig?: FullConfigType) {
    try {
        const setting = fullConfig?.settings?.["app:tray"];
        applyTrayEnabled(setting ?? true);
        if (pollTimer == null) {
            pollTimer = setInterval(() => fireAndForget(pollTray), trayPollMs);
            fireAndForget(pollTray);
        }
    } catch (e) {
        log(`[tray] init failed: ${e}`);
    }
}
