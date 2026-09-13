// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { useWaveEnv } from "@/app/waveenv/waveenv";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { DefaultTermTheme } from "@/app/view/term/termutil";
import { cn, fireAndForget } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useMemo, useState } from "react";
import { filterThemeKeys, sortedThemeKeys, themeDisplayName } from "./themes-util";

function useApplyTheme() {
    const env = useWaveEnv();
    return (key: string) => {
        fireAndForget(() => env.rpc.SetConfigCommand(TabRpcClient, { "term:theme": key }));
    };
}

// ThemePreviewCard renders the Warp-style mini terminal: an `ls` line with a
// directory and an executable colored by the theme, plus a cursor block.
const ThemePreviewCard = memo(
    ({ themeKey, theme, selected, onSelect }: { themeKey: string; theme?: TermThemeType; selected: boolean; onSelect: () => void }) => {
        const background = theme?.background || "#101014";
        const foreground = theme?.foreground || "#e6e6e6";
        const directory = theme?.blue || foreground;
        const executable = theme?.green || foreground;
        const cursor = theme?.cursor || foreground;
        return (
            <button
                type="button"
                onClick={onSelect}
                className={cn(
                    "flex cursor-pointer flex-col gap-1.5 rounded-lg border p-1.5 text-left transition-colors",
                    selected ? "border-accent/80 ring-1 ring-accent/40" : "border-border/60 hover:border-border"
                )}
            >
                <div
                    className="relative flex h-[64px] w-full flex-col justify-center gap-1.5 overflow-hidden rounded-md px-2.5 font-mono text-[11px] leading-none"
                    style={{ background }}
                >
                    <div className="flex flex-row items-center gap-2 whitespace-nowrap">
                        <span style={{ color: foreground }}>ls</span>
                        <span style={{ color: directory }}>dir</span>
                        <span style={{ color: executable }}>executable</span>
                    </div>
                    <div className="flex flex-row items-center gap-2">
                        <span className="inline-block h-[11px] w-[7px] rounded-[1.5px]" style={{ background: cursor }} />
                    </div>
                    {selected && (
                        <div className="absolute top-1.5 right-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-accent text-[9px] text-background">
                            <i className="fa fa-solid fa-check" />
                        </div>
                    )}
                </div>
                <span className={cn("truncate px-0.5 text-[11.5px]", selected ? "text-foreground" : "text-secondary")}>
                    {themeDisplayName(themeKey, theme)}
                </span>
            </button>
        );
    }
);
ThemePreviewCard.displayName = "ThemePreviewCard";

export const ThemesGallery = memo(() => {
    const env = useWaveEnv();
    const fullConfig = useAtomValue(env.atoms.fullConfigAtom);
    const currentTheme = useAtomValue(env.getSettingsKeyAtom("term:theme")) ?? DefaultTermTheme;
    const applyTheme = useApplyTheme();
    const [query, setQuery] = useState("");
    const termthemes = fullConfig?.termthemes ?? {};
    const themeKeys = useMemo(
        () => filterThemeKeys(sortedThemeKeys(termthemes), termthemes, query),
        [termthemes, query]
    );
    return (
        <div className="flex w-full flex-col gap-3">
            <div className="relative w-full max-w-[280px]">
                <i className="fa fa-magnifying-glass absolute left-2.5 top-1/2 -translate-y-1/2 text-[11px] text-muted" />
                <input
                    className="w-full rounded-lg border border-border/70 bg-background/40 py-1.5 pl-7 pr-2.5 text-[12.5px] text-foreground outline-none transition-colors hover:border-border"
                    placeholder="Search themes…"
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                />
            </div>
            {themeKeys.length === 0 ? (
                <span className="text-[11.5px] text-secondary">No themes match that search.</span>
            ) : (
                <div className="grid grid-cols-2 gap-2.5 pr-1 md:grid-cols-3">
                    {themeKeys.map((key) => (
                        <ThemePreviewCard
                            key={key}
                            themeKey={key}
                            theme={termthemes[key]}
                            selected={key === currentTheme}
                            onSelect={() => applyTheme(key)}
                        />
                    ))}
                </div>
            )}
            <span className="text-[11.5px] text-secondary">
                Current theme:{" "}
                <span className="text-foreground">{themeDisplayName(currentTheme, termthemes[currentTheme])}</span> — applies
                to every terminal.
            </span>
        </div>
    );
});
ThemesGallery.displayName = "ThemesGallery";
