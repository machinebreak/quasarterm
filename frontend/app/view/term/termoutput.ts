// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

// Terminal output sanitizing: rewrites dark "black background" SGR parameters
// (ESC[40m, ESC[48;5;0m, ESC[48;5;232m, ESC[48;2;0;0;0m, ...) to the default
// background. CLIs (Codex, Claude, ...) use those to paint highlights, panels
// and rules; on a translucent/semi-dark theme they render as opaque dark slabs
// because explicit backgrounds ignore the theme's transparency. Using the
// default background instead makes them blend in like regular text, without
// changing the terminal background color.

const SgrRegex = /\u001b\[([0-9;]*)m/g; // eslint-disable-line no-control-regex
// RGB channels at or below this are treated as "black"-ish. CLIs paint panels,
// rules and hint rows with near-black backgrounds; over a transparent theme
// every one of those renders as an opaque slab, so the whole dark range is
// rewritten to the default background. 0x20 leaves intentional lighter panel
// fills (e.g. #292929 composer rows) untouched.
const MaxBlackRgbChannel = 0x20;

function isBlackBg256(raw: string): boolean {
    const n = raw === "" ? 0 : parseInt(raw, 10);
    // 0/16 = black; 232-234 = the near-black grays (#080808..#1c1c1c)
    return n === 0 || n === 16 || (n >= 232 && n <= 234);
}

function isBlackRgb(rRaw: string, gRaw: string, bRaw: string): boolean {
    const r = parseInt(rRaw, 10);
    const g = parseInt(gRaw, 10);
    const b = parseInt(bRaw, 10);
    if (isNaN(r) || isNaN(g) || isNaN(b)) {
        return false;
    }
    return r <= MaxBlackRgbChannel && g <= MaxBlackRgbChannel && b <= MaxBlackRgbChannel;
}

// Splits an SGR parameter list, drops black-background parameters and rebuilds
// the sequence. Returns null when nothing was removed.
function rewriteSgrSequence(paramsStr: string): string {
    const parts = paramsStr.split(";");
    const kept: string[] = [];
    let removed = false;
    for (let i = 0; i < parts.length; i++) {
        const raw = parts[i];
        const value = raw === "" ? 0 : parseInt(raw, 10);
        if (value === 40) {
            removed = true;
            continue;
        }
        if (value === 48 && parts[i + 1] === "5" && isBlackBg256(parts[i + 2])) {
            removed = true;
            i += 2;
            continue;
        }
        if (value === 48 && parts[i + 1] === "2" && isBlackRgb(parts[i + 2], parts[i + 3], parts[i + 4])) {
            removed = true;
            i += 4;
            continue;
        }
        kept.push(raw);
    }
    if (!removed) {
        return null;
    }
    if (kept.length === 0) {
        return "\u001b[49m";
    }
    return `\u001b[${kept.join(";")}m`;
}

// Rewrites black-background SGR sequences to the default background (ESC[49m).
// Everything else (foregrounds, resets, other sequences) is left untouched.
export function stripBlackBackgroundEscapes(data: string): string {
    if (data == null || !data.includes("\u001b[")) {
        return data;
    }
    return data.replace(SgrRegex, (match, paramsStr: string) => {
        if (paramsStr === "") {
            return match;
        }
        const rewritten = rewriteSgrSequence(paramsStr);
        return rewritten ?? match;
    });
}
