// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { CanvasBackground, CanvasBgPresets } from "@/app/canvas/canvas-bg";
import { getApi, getSettingsKeyAtom } from "@/app/store/global";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { cn, fireAndForget, makeIconClass } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useCallback } from "react";
import { ThemesGallery } from "./themes-gallery";
import type { WaveConfigViewModel } from "./waveconfig-model";

interface SettingsContentProps {
    model: WaveConfigViewModel;
}

function setConfigValue(key: string, value: any) {
    fireAndForget(() => RpcApi.SetConfigCommand(TabRpcClient, { [key]: value }));
}

interface SettingsSectionProps {
    icon?: string;
    title: string;
    description?: string;
    children: React.ReactNode;
}

const SettingsSection = memo(({ icon, title, description, children }: SettingsSectionProps) => {
    return (
        <section className="overflow-hidden rounded-xl border border-border/70 bg-panel/40">
            <header className="flex items-center gap-3 border-b border-border/60 px-4 py-3">
                {icon != null && (
                    <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-hover text-[13px] text-secondary">
                        <i className={makeIconClass(icon, true)} />
                    </div>
                )}
                <div className="flex min-w-0 flex-col">
                    <div className="text-[13.5px] font-semibold text-foreground">{title}</div>
                    {description != null && <div className="text-[11.5px] text-secondary">{description}</div>}
                </div>
            </header>
            <div className="flex flex-col px-4">{children}</div>
        </section>
    );
});
SettingsSection.displayName = "SettingsSection";

interface SettingsRowProps {
    label: string;
    description?: string;
    children: React.ReactNode;
}

const SettingsRow = memo(({ label, description, children }: SettingsRowProps) => {
    return (
        <div className="flex items-start justify-between gap-8 border-b border-border/40 py-3.5 last:border-b-0">
            <div className="flex min-w-0 flex-col gap-0.5">
                <div className="text-[13px] font-medium text-foreground">{label}</div>
                {description != null && (
                    <div className="max-w-[430px] text-[11.5px] leading-relaxed text-secondary/90">{description}</div>
                )}
            </div>
            <div className="shrink-0 pt-0.5">{children}</div>
        </div>
    );
});
SettingsRow.displayName = "SettingsRow";

interface SegmentedOption {
    value: string;
    label: string;
    icon: string;
}

interface SegmentedControlProps {
    options: SegmentedOption[];
    value: string;
    onChange: (value: string) => void;
}

const SegmentedControl = memo(({ options, value, onChange }: SegmentedControlProps) => {
    return (
        <div className="flex items-center gap-0.5 rounded-lg border border-border/60 bg-background/40 p-0.5">
            {options.map((option) => (
                <button
                    key={option.value}
                    type="button"
                    className={cn(
                        "flex cursor-pointer items-center gap-1.5 rounded-md px-3 py-1.5 text-[12.5px] transition-colors",
                        value === option.value
                            ? "bg-accent font-medium text-background"
                            : "text-secondary hover:bg-hover hover:text-foreground"
                    )}
                    onClick={() => onChange(option.value)}
                >
                    <i className={`fa fa-solid fa-${option.icon} text-[11px]`} />
                    <span>{option.label}</span>
                </button>
            ))}
        </div>
    );
});
SegmentedControl.displayName = "SegmentedControl";

interface ToggleProps {
    checked: boolean;
    onChange: (checked: boolean) => void;
}

const Toggle = memo(({ checked, onChange }: ToggleProps) => {
    return (
        <button
            type="button"
            role="switch"
            aria-checked={checked}
            className={cn(
                "relative h-[20px] w-[36px] shrink-0 cursor-pointer rounded-full border transition-colors",
                checked ? "border-transparent bg-accent" : "border-border bg-transparent hover:border-white/25"
            )}
            onClick={() => onChange(!checked)}
        >
            <div
                className={cn(
                    "absolute top-1/2 h-[14px] w-[14px] -translate-y-1/2 rounded-full transition-all",
                    checked ? "left-[18px] bg-background" : "left-[3px] bg-muted"
                )}
            />
        </button>
    );
});
Toggle.displayName = "Toggle";

interface CanvasBgChipProps {
    preset: string;
    label: string;
    selected: boolean;
    customImage?: string;
    onSelect: () => void;
}

const CanvasBgChip = memo(({ preset, label, selected, customImage, onSelect }: CanvasBgChipProps) => {
    return (
        <button
            type="button"
            onClick={onSelect}
            className={cn(
                "flex cursor-pointer flex-col gap-1.5 rounded-lg border p-1 transition-colors",
                selected ? "border-accent/80 ring-1 ring-accent/40" : "border-border/60 hover:border-border"
            )}
        >
            <div className="relative h-[52px] w-[88px] overflow-hidden rounded-md bg-black/40">
                <CanvasBackground preset={preset} customImage={customImage} dim={0} />
                {selected && (
                    <div className="absolute top-1 right-1 flex h-4 w-4 items-center justify-center rounded-full bg-accent text-[9px] text-background">
                        <i className="fa fa-solid fa-check" />
                    </div>
                )}
            </div>
            <span className={cn("text-center text-[10.5px]", selected ? "text-foreground" : "text-secondary")}>
                {label}
            </span>
        </button>
    );
});
CanvasBgChip.displayName = "CanvasBgChip";

export const SettingsContent = memo(({ model: _model }: SettingsContentProps) => {
    const tabBarPosition = useAtomValue(getSettingsKeyAtom("app:tabbar")) ?? "top";
    const confirmClose = useAtomValue(getSettingsKeyAtom("tab:confirmclose")) ?? false;
    const transparency = useAtomValue(getSettingsKeyAtom("term:transparency")) ?? 0.5;
    const agentAlerts = useAtomValue(getSettingsKeyAtom("term:agentalerts")) ?? false;
    const canvasMode = useAtomValue(getSettingsKeyAtom("app:canvasmode")) ?? false;
    const canvasBg = useAtomValue(getSettingsKeyAtom("app:canvasbg")) ?? "grid";
    const canvasBgImage = useAtomValue(getSettingsKeyAtom("app:canvasbgimage")) ?? "";
    const canvasBgDim = useAtomValue(getSettingsKeyAtom("app:canvasbgdim")) ?? 0.3;
    const quotaAlert = useAtomValue(getSettingsKeyAtom("accounts:quotaalert")) ?? true;
    const quotaAlertThreshold = useAtomValue(getSettingsKeyAtom("accounts:quotaalertthreshold")) ?? 15;
    const accountsRefreshInterval = useAtomValue(getSettingsKeyAtom("accounts:refreshinterval")) ?? 5;
    const trayQuota = useAtomValue(getSettingsKeyAtom("app:tray")) ?? true;

    const transparencyPct = Math.round(transparency * 100);
    const canvasBgDimPct = Math.round(canvasBgDim * 100);

    const onImportBgImage = useCallback(async () => {
        try {
            const importedPath = await getApi().importBackgroundImage();
            if (importedPath == null) {
                return;
            }
            setConfigValue("app:canvasbgimage", importedPath);
            setConfigValue("app:canvasbg", "custom");
        } catch (e) {
            console.error("failed to import background image", e);
        }
    }, []);

    return (
        <div className="h-full w-full overflow-y-auto">
            <div className="mx-auto flex w-full max-w-[720px] flex-col gap-6 px-6 py-6">
                <SettingsSection
                    icon="table-columns"
                    title="Tabs"
                    description="How terminal tabs are laid out in the window."
                >
                    <SettingsRow
                        label="Tab Bar Position"
                        description="Show tabs in a horizontal bar at the top, or as a vertical list on the left side with the project folder for each tab."
                    >
                        <SegmentedControl
                            options={[
                                { value: "top", label: "Top", icon: "arrow-up" },
                                { value: "left", label: "Side", icon: "table-columns" },
                            ]}
                            value={tabBarPosition}
                            onChange={(value) => setConfigValue("app:tabbar", value)}
                        />
                    </SettingsRow>
                    <SettingsRow
                        label="Confirm Before Closing Tabs"
                        description="Ask for confirmation when a tab is closed."
                    >
                        <Toggle
                            checked={confirmClose}
                            onChange={(checked) => setConfigValue("tab:confirmclose", checked)}
                        />
                    </SettingsRow>
                </SettingsSection>

                <SettingsSection
                    icon="layer-group"
                    title="Canvas"
                    description="Floating, movable terminal windows (Deska-style) with a chill background."
                >
                    <SettingsRow
                        label="Canvas Mode"
                        description="Every tab becomes a free canvas: windows open floating, drag them by their header, resize from any edge, double-click a header to maximize. Also switchable from a tab's right-click menu."
                    >
                        <Toggle
                            checked={canvasMode}
                            onChange={(checked) => setConfigValue("app:canvasmode", checked)}
                        />
                    </SettingsRow>
                    <SettingsRow
                        label="Background"
                        description="Pick a preset, or import your own photo or animated GIF (copied into the app config folder)."
                    >
                        <div className="flex max-w-[310px] flex-wrap justify-end gap-2">
                            {CanvasBgPresets.map((preset) => (
                                <CanvasBgChip
                                    key={preset.id}
                                    preset={preset.id}
                                    label={preset.label}
                                    selected={canvasBg === preset.id}
                                    onSelect={() => setConfigValue("app:canvasbg", preset.id)}
                                />
                            ))}
                            {canvasBgImage !== "" && (
                                <CanvasBgChip
                                    preset="custom"
                                    label="Custom"
                                    customImage={canvasBgImage}
                                    selected={canvasBg === "custom"}
                                    onSelect={() => setConfigValue("app:canvasbg", "custom")}
                                />
                            )}
                            <button
                                type="button"
                                onClick={onImportBgImage}
                                className="flex cursor-pointer flex-col gap-1.5 rounded-lg border border-dashed border-border/70 p-1 text-secondary transition-colors hover:border-accent/60 hover:text-foreground"
                            >
                                <div className="flex h-[52px] w-[88px] items-center justify-center rounded-md bg-hover/40">
                                    <i className="fa fa-solid fa-image text-[16px]" />
                                </div>
                                <span className="text-center text-[10.5px]">Import…</span>
                            </button>
                        </div>
                    </SettingsRow>
                    <SettingsRow
                        label="Background Dim"
                        description="Darken the background so windows and text stand out (0% = none, 80% = dark)."
                    >
                        <div className="flex items-center gap-3">
                            <input
                                type="range"
                                min={0}
                                max={80}
                                step={5}
                                value={canvasBgDimPct}
                                className="w-36 cursor-pointer"
                                style={{ accentColor: "var(--color-accent)" }}
                                onChange={(event) =>
                                    setConfigValue("app:canvasbgdim", Number(event.target.value) / 100)
                                }
                            />
                            <span className="w-11 rounded-md bg-hover py-0.5 text-center text-[11.5px] tabular-nums text-secondary">
                                {canvasBgDimPct}%
                            </span>
                        </div>
                    </SettingsRow>
                </SettingsSection>

                <SettingsSection
                    icon="users"
                    title="Accounts"
                    description="Provider accounts dashboard: quota monitoring, one-click switching and isolated instances."
                >
                    <SettingsRow
                        label="Low Quota Alerts"
                        description="Show a desktop notification when a tracked account drops below the alert threshold."
                    >
                        <Toggle
                            checked={quotaAlert}
                            onChange={(checked) => setConfigValue("accounts:quotaalert", checked)}
                        />
                    </SettingsRow>
                    <SettingsRow
                        label="Alert Threshold"
                        description="Notify when any quota metric falls below this remaining percentage."
                    >
                        <div className="flex items-center gap-3">
                            <input
                                type="range"
                                min={0}
                                max={50}
                                step={1}
                                value={quotaAlertThreshold}
                                className="w-36 cursor-pointer"
                                style={{ accentColor: "var(--color-accent)" }}
                                onChange={(event) =>
                                    setConfigValue("accounts:quotaalertthreshold", Number(event.target.value))
                                }
                            />
                            <span className="w-11 rounded-md bg-hover py-0.5 text-center text-[11.5px] tabular-nums text-secondary">
                                {quotaAlertThreshold}%
                            </span>
                        </div>
                    </SettingsRow>
                    <SettingsRow
                        label="Quota Refresh Interval"
                        description="Minutes between background quota refreshes (minimum 2). Set 0 to disable the background monitor — quotas still refresh while the Accounts panel is open."
                    >
                        <input
                            type="number"
                            min={0}
                            max={120}
                            step={1}
                            value={accountsRefreshInterval}
                            className="w-20 rounded-lg border border-border/70 bg-background/40 px-2.5 py-1.5 text-[12.5px] text-foreground outline-none transition-colors hover:border-border"
                            onChange={(event) =>
                                setConfigValue("accounts:refreshinterval", Math.max(0, Number(event.target.value) || 0))
                            }
                        />
                    </SettingsRow>
                    <SettingsRow
                        label="Tray Quota Indicator"
                        description="Show a system tray icon with the lowest quota per provider. Click it to bring Quasar back, right-click for quota details, refresh and quit."
                    >
                        <Toggle checked={trayQuota} onChange={(checked) => setConfigValue("app:tray", checked)} />
                    </SettingsRow>
                </SettingsSection>

                <SettingsSection
                    icon="palette"
                    title="Themes"
                    description="Change your current theme — every terminal picks it up instantly."
                >
                    <div className="py-3.5">
                        <ThemesGallery />
                    </div>
                </SettingsSection>

                <SettingsSection icon="terminal" title="Terminal" description="Terminal background appearance.">
                    <SettingsRow
                        label="Terminal Background"
                        description="Translucent keeps the previous soft look that blends with the window. Solid uses opaque black, which looks cleaner next to CLI panels (Codex, Claude Code)."
                    >
                        <SegmentedControl
                            options={[
                                { value: "0.5", label: "Translucent", icon: "droplet" },
                                { value: "0", label: "Solid", icon: "square" },
                            ]}
                            value={String(transparency)}
                            onChange={(value) => setConfigValue("term:transparency", Number(value))}
                        />
                    </SettingsRow>
                    <SettingsRow
                        label="Transparency"
                        description="Fine-tune the background transparency (0% fully opaque, 100% fully transparent)."
                    >
                        <div className="flex items-center gap-3">
                            <input
                                type="range"
                                min={0}
                                max={100}
                                step={5}
                                value={transparencyPct}
                                className="w-36 cursor-pointer"
                                style={{ accentColor: "var(--color-accent)" }}
                                onChange={(event) =>
                                    setConfigValue("term:transparency", Number(event.target.value) / 100)
                                }
                            />
                            <span className="w-11 rounded-md bg-hover py-0.5 text-center text-[11.5px] tabular-nums text-secondary">
                                {transparencyPct}%
                            </span>
                        </div>
                    </SettingsRow>
                </SettingsSection>

                <SettingsSection
                    icon="robot"
                    title="Agents"
                    description="Alerts for AI CLI agents (Codex, Claude, ...) running in terminals."
                >
                    <SettingsRow
                        label="Alert when an agent finishes"
                        description="Shows a green/red dot on the tab when an agent CLI finishes (it clears when you open the tab), and a desktop notification when a run takes longer than a few seconds while the window is unfocused."
                    >
                        <Toggle
                            checked={agentAlerts}
                            onChange={(checked) => setConfigValue("term:agentalerts", checked)}
                        />
                    </SettingsRow>
                </SettingsSection>
            </div>
        </div>
    );
});
SettingsContent.displayName = "SettingsContent";
