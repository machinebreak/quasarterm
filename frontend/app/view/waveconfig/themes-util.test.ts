// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { filterThemeKeys, sortedThemeKeys, themeDisplayName } from "./themes-util";

const themes: { [key: string]: TermThemeType } = {
    dracula: { "display:name": "Dracula", "display:order": 3 } as TermThemeType,
    "default-dark": { "display:name": "Default Dark", "display:order": 1 } as TermThemeType,
    nord: { "display:name": "Nord", "display:order": 11 } as TermThemeType,
    "catppuccin-mocha": { "display:name": "Catppuccin Mocha", "display:order": 8 } as TermThemeType,
};

describe("sortedThemeKeys", () => {
    it("sorts by display order", () => {
        expect(sortedThemeKeys(themes)).toEqual(["default-dark", "dracula", "catppuccin-mocha", "nord"]);
    });

    it("falls back to the key when a theme has no name", () => {
        const withUnnamed = { ...themes, zeta: { "display:order": 4 } as TermThemeType };
        expect(sortedThemeKeys(withUnnamed)).toEqual(["default-dark", "dracula", "zeta", "catppuccin-mocha", "nord"]);
        expect(themeDisplayName("zeta", withUnnamed.zeta)).toBe("zeta");
    });

    it("handles an empty theme map", () => {
        expect(sortedThemeKeys({})).toEqual([]);
    });
});

describe("filterThemeKeys", () => {
    const keys = sortedThemeKeys(themes);

    it("returns everything for an empty query", () => {
        expect(filterThemeKeys(keys, themes, "   ")).toEqual(keys);
    });

    it("matches on the display name, case-insensitively", () => {
        expect(filterThemeKeys(keys, themes, "mocha")).toEqual(["catppuccin-mocha"]);
        expect(filterThemeKeys(keys, themes, "NORD")).toEqual(["nord"]);
    });

    it("matches on the theme key too", () => {
        expect(filterThemeKeys(keys, themes, "catppuccin")).toEqual(["catppuccin-mocha"]);
        expect(filterThemeKeys(keys, themes, "dracula")).toEqual(["dracula"]);
    });

    it("returns nothing when there is no match", () => {
        expect(filterThemeKeys(keys, themes, "solarized")).toEqual([]);
    });
});
