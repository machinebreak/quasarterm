// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

type TermThemes = { [key: string]: TermThemeType };

export function sortedThemeKeys(termthemes: TermThemes): string[] {
    return Object.keys(termthemes ?? {}).sort((a, b) => {
        const orderA = termthemes[a]?.["display:order"] ?? 0;
        const orderB = termthemes[b]?.["display:order"] ?? 0;
        if (orderA !== orderB) {
            return orderA - orderB;
        }
        return themeDisplayName(a, termthemes[a]).localeCompare(themeDisplayName(b, termthemes[b]));
    });
}

export function themeDisplayName(key: string, theme?: TermThemeType): string {
    return theme?.["display:name"] ?? key;
}

export function filterThemeKeys(keys: string[], termthemes: TermThemes, query: string): string[] {
    const trimmed = query.trim().toLowerCase();
    if (trimmed === "") {
        return keys;
    }
    return keys.filter((key) => {
        const name = themeDisplayName(key, termthemes[key]).toLowerCase();
        return name.includes(trimmed) || key.toLowerCase().includes(trimmed);
    });
}
