// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { DefaultTermTheme } from "@/app/view/term/termutil";
import { ThemesGallery } from "@/app/view/waveconfig/themes-gallery";
import { useWaveEnv, WaveEnv, WaveEnvContext } from "@/app/waveenv/waveenv";
import { atom } from "jotai";
import * as React from "react";
import termThemesJSON from "../../../pkg/wconfig/defaultconfig/termthemes.json";
import { applyMockEnvOverrides } from "../mock/mockwaveenv";

const termThemes = termThemesJSON as unknown as { [key: string]: TermThemeType };

const themesConfigAtom = atom<FullConfigType>({
    settings: { "term:theme": "catppuccin-mocha" },
    termthemes: termThemes,
} as unknown as FullConfigType);

function makeThemesEnv(baseEnv: WaveEnv): WaveEnv {
    return applyMockEnvOverrides(baseEnv, {
        atoms: {
            fullConfigAtom: themesConfigAtom,
        },
    });
}

export default function ThemesPreview() {
    const baseEnv = useWaveEnv();
    const envRef = React.useRef<WaveEnv>(null);
    if (envRef.current == null) {
        envRef.current = makeThemesEnv(baseEnv);
    }
    return (
        <WaveEnvContext.Provider value={envRef.current}>
            <div className="flex w-full max-w-[820px] flex-col gap-3 px-6 py-6">
                <div className="flex flex-col gap-1">
                    <span className="text-[15px] font-semibold text-foreground">Themes</span>
                    <span className="text-xs font-mono text-muted">
                        settings → themes gallery (real termthemes.json, default theme: {DefaultTermTheme})
                    </span>
                </div>
                <div className="rounded-xl border border-border/70 bg-panel/40 p-4">
                    <ThemesGallery />
                </div>
            </div>
        </WaveEnvContext.Provider>
    );
}
