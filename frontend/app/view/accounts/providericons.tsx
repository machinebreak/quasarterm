// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import antigravityIcon from "@/app/asset/providers/antigravity.svg?url";
import claudeIcon from "@/app/asset/providers/claude.png?url";
import codebuddyIcon from "@/app/asset/providers/codebuddy.png?url";
import codexIcon from "@/app/asset/providers/codex.svg?url";
import cursorIcon from "@/app/asset/providers/cursor.svg?url";
import devinIcon from "@/app/asset/providers/devin.png?url";
import copilotIcon from "@/app/asset/providers/github-copilot.svg?url";
import grokIcon from "@/app/asset/providers/grok.svg?url";
import kiroIcon from "@/app/asset/providers/kiro.svg?url";
import opencodeIcon from "@/app/asset/providers/opencode.svg?url";
import qoderIcon from "@/app/asset/providers/qoder.png?url";
import traeCnIcon from "@/app/asset/providers/trae-cn.png?url";
import traeSoloCnIcon from "@/app/asset/providers/trae-solo-cn.png?url";
import traeSoloIcon from "@/app/asset/providers/trae-solo.png?url";
import traeIcon from "@/app/asset/providers/trae.png?url";
import windsurfIcon from "@/app/asset/providers/windsurf.svg?url";
import workbuddyIcon from "@/app/asset/providers/workbuddy.png?url";
import zcodeIcon from "@/app/asset/providers/zcode.png?url";
import zedIcon from "@/app/asset/providers/zed.png?url";

export const ProviderIcons: { [key: string]: string } = {
    antigravity: antigravityIcon,
    claude: claudeIcon,
    codebuddy: codebuddyIcon,
    codex: codexIcon,
    "github-copilot": copilotIcon,
    cursor: cursorIcon,
    devin: devinIcon,
    grok: grokIcon,
    kiro: kiroIcon,
    opencode: opencodeIcon,
    qoder: qoderIcon,
    trae: traeIcon,
    "trae-cn": traeCnIcon,
    "trae-solo": traeSoloIcon,
    "trae-solo-cn": traeSoloCnIcon,
    windsurf: windsurfIcon,
    workbuddy: workbuddyIcon,
    zcode: zcodeIcon,
    zed: zedIcon,
};

const ProviderAccent: { [key: string]: string } = {
    codex: "#10a37f",
    claude: "#d97757",
    "github-copilot": "#8b949e",
    antigravity: "#4285f4",
    cursor: "#8b5cf6",
    windsurf: "#09b6a2",
    kiro: "#f59e0b",
    qoder: "#6366f1",
    zed: "#4f46e5",
    zcode: "#0ea5e9",
};

const ProviderNames: { [key: string]: string } = {
    claude: "Claude",
    codex: "Codex",
    "github-copilot": "GitHub Copilot",
    antigravity: "Antigravity",
    cursor: "Cursor",
    windsurf: "Windsurf",
    kiro: "Kiro",
    grok: "Grok",
    qoder: "Qoder",
    zed: "Zed",
    zcode: "ZCode",
    codebuddy: "CodeBuddy",
    devin: "Devin",
    opencode: "opencode",
    workbuddy: "WorkBuddy",
    trae: "Trae",
};

export function providerAccent(id: string): string {
    return ProviderAccent[id] ?? "var(--accent-color)";
}

export function getProviderIconUrl(providerId: string): string | undefined {
    return ProviderIcons[providerId];
}

export function providerDisplayName(id: string): string {
    return ProviderNames[id] ?? id;
}

export function ProviderIcon({ provider, size = 20 }: { provider: ProviderInfo; size?: number }) {
    const icon = ProviderIcons[provider.icon] ?? ProviderIcons[provider.id];
    if (icon) {
        return <img src={icon} alt={provider.name} style={{ width: size, height: size }} draggable={false} />;
    }
    return (
        <div
            className="flex items-center justify-center rounded font-bold"
            style={{
                width: size,
                height: size,
                background: providerAccent(provider.id),
                color: "#fff",
                fontSize: size * 0.45,
            }}
        >
            {provider.name.substring(0, 1)}
        </div>
    );
}
