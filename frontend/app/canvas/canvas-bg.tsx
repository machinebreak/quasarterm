// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

// Canvas mode backgrounds: built-in chill presets (plain/dotted grid/quiet
// peaks/lofi night/aurora drift) plus imported custom images (png/jpg/gif...).

import { boundNumber, cn, isBlank } from "@/util/util";
import { getWebServerEndpoint } from "@/util/endpoints";
import { formatRemoteUri } from "@/util/waveutil";
import * as React from "react";
import { memo, useState } from "react";

export type CanvasBgPresetId = "default" | "grid" | "peaks" | "lofi" | "aurora" | "custom";

export const CanvasBgPresets: { id: CanvasBgPresetId; label: string }[] = [
    { id: "default", label: "Plain" },
    { id: "grid", label: "Dotted Grid" },
    { id: "peaks", label: "Quiet Peaks" },
    { id: "lofi", label: "Lo-Fi Night" },
    { id: "aurora", label: "Aurora Drift" },
];

export function customBgImageUrl(path: string): string {
    if (isBlank(path)) {
        return null;
    }
    try {
        return (
            getWebServerEndpoint() + `/wave/stream-file?path=${encodeURIComponent(formatRemoteUri(path, "local"))}&no404=1`
        );
    } catch (e) {
        console.warn("canvas: failed to resolve custom background image", e);
        return null;
    }
}

function mulberry32(seed: number): () => number {
    let a = seed >>> 0;
    return () => {
        a = (a + 0x6d2b79f5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

type Star = { x: number; y: number; r: number; o: number; dur?: number; delay?: number };

const peaksStars: Star[] = (() => {
    const rnd = mulberry32(1337);
    return Array.from({ length: 90 }, () => ({
        x: Math.round(rnd() * 1600),
        y: Math.round(rnd() * 480),
        r: Math.round((0.6 + rnd() * 1.3) * 10) / 10,
        o: Math.round((0.25 + rnd() * 0.6) * 100) / 100,
    }));
})();

const lofiStars: Star[] = (() => {
    const rnd = mulberry32(4242);
    return Array.from({ length: 54 }, () => ({
        x: Math.round(rnd() * 1600),
        y: Math.round(rnd() * 430),
        r: Math.round((0.6 + rnd() * 1.5) * 10) / 10,
        o: Math.round((0.35 + rnd() * 0.55) * 100) / 100,
        dur: Math.round((3.5 + rnd() * 5) * 10) / 10,
        delay: Math.round(rnd() * 60) / 10,
    }));
})();

const auroraStars: Star[] = (() => {
    const rnd = mulberry32(90125);
    return Array.from({ length: 70 }, () => ({
        x: Math.round(rnd() * 1600),
        y: Math.round(rnd() * 620),
        r: Math.round((0.5 + rnd() * 1.2) * 10) / 10,
        o: Math.round((0.25 + rnd() * 0.55) * 100) / 100,
    }));
})();

const Scene_Peaks = memo(({ bgId }: { bgId: string }) => (
    <svg
        className="canvas-scene canvas-scene-svg"
        viewBox="0 0 1600 900"
        preserveAspectRatio="xMidYMid slice"
        aria-hidden="true"
    >
        <defs>
            <linearGradient id={`${bgId}-sky`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#060917" />
                <stop offset="42%" stopColor="#131a3c" />
                <stop offset="72%" stopColor="#2b2954" />
                <stop offset="100%" stopColor="#54405f" />
            </linearGradient>
            <radialGradient id={`${bgId}-moonglow`} cx="0.5" cy="0.5" r="0.5">
                <stop offset="0%" stopColor="#f2f0ff" stopOpacity="0.85" />
                <stop offset="35%" stopColor="#c6c2ee" stopOpacity="0.28" />
                <stop offset="100%" stopColor="#c6c2ee" stopOpacity="0" />
            </radialGradient>
            <linearGradient id={`${bgId}-haze`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#8a6a9e" stopOpacity="0" />
                <stop offset="100%" stopColor="#8a6a9e" stopOpacity="0.32" />
            </linearGradient>
        </defs>
        <rect width="1600" height="900" fill={`url(#${bgId}-sky)`} />
        {peaksStars.map((star, i) => (
            <circle key={i} cx={star.x} cy={star.y} r={star.r} fill="#e8ecff" opacity={star.o} />
        ))}
        <circle cx="1170" cy="215" r="170" fill={`url(#${bgId}-moonglow)`} />
        <circle cx="1170" cy="215" r="50" fill="#eef0ff" />
        <circle cx="1188" cy="202" r="9" fill="#cfd3f7" opacity="0.55" />
        <circle cx="1156" cy="228" r="6" fill="#cfd3f7" opacity="0.45" />
        <circle cx="1178" cy="234" r="4" fill="#cfd3f7" opacity="0.4" />
        <path
            d="M0 596 L150 486 L300 566 L470 420 L640 574 L790 498 L950 590 L1140 480 L1310 588 L1450 520 L1600 574 L1600 900 L0 900 Z"
            fill="#262c55"
        />
        <rect y="540" width="1600" height="200" fill={`url(#${bgId}-haze)`} />
        <path
            d="M0 668 L190 540 L360 640 L560 500 L760 656 L940 560 L1140 668 L1330 570 L1600 660 L1600 900 L0 900 Z"
            fill="#181e41"
        />
        <rect y="612" width="1600" height="200" fill={`url(#${bgId}-haze)`} />
        <path
            d="M0 760 L220 620 L430 730 L660 588 L900 748 L1120 646 L1360 756 L1600 660 L1600 900 L0 900 Z"
            fill="#0d1230"
        />
        <ellipse cx="800" cy="742" rx="760" ry="60" fill="#0d1230" opacity="0.55" />
    </svg>
));
Scene_Peaks.displayName = "Scene_Peaks";

const Scene_Lofi = memo(({ bgId }: { bgId: string }) => (
    <svg
        className="canvas-scene canvas-scene-svg"
        viewBox="0 0 1600 900"
        preserveAspectRatio="xMidYMid slice"
        aria-hidden="true"
    >
        <defs>
            <linearGradient id={`${bgId}-sky`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#12172f" />
                <stop offset="38%" stopColor="#2a2a52" />
                <stop offset="66%" stopColor="#5c4059" />
                <stop offset="88%" stopColor="#a15f56" />
                <stop offset="100%" stopColor="#c9775b" />
            </linearGradient>
            <radialGradient id={`${bgId}-sun`} cx="0.5" cy="0.5" r="0.5">
                <stop offset="0%" stopColor="#ffd9a0" stopOpacity="0.95" />
                <stop offset="45%" stopColor="#ff9f6e" stopOpacity="0.45" />
                <stop offset="100%" stopColor="#ff9f6e" stopOpacity="0" />
            </radialGradient>
            <filter id={`${bgId}-soft`} x="-60%" y="-60%" width="220%" height="220%">
                <feGaussianBlur stdDeviation="20" />
            </filter>
            <linearGradient id={`${bgId}-fog`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#d88a6a" stopOpacity="0" />
                <stop offset="100%" stopColor="#d88a6a" stopOpacity="0.28" />
            </linearGradient>
        </defs>
        <rect width="1600" height="900" fill={`url(#${bgId}-sky)`} />
        {lofiStars.map((star, i) => (
            <circle
                key={i}
                className="lofi-twinkle"
                style={{ animationDuration: `${star.dur}s`, animationDelay: `${star.delay}s` }}
                cx={star.x}
                cy={star.y}
                r={star.r}
                fill="#ffeedd"
                opacity={star.o}
            />
        ))}
        <g className="lofi-cloud lofi-cloud-1" filter={`url(#${bgId}-soft)`}>
            <ellipse cx="330" cy="212" rx="250" ry="40" fill="#343a6b" opacity="0.5" />
            <ellipse cx="520" cy="238" rx="180" ry="30" fill="#3c3f72" opacity="0.4" />
        </g>
        <g className="lofi-cloud lofi-cloud-2" filter={`url(#${bgId}-soft)`}>
            <ellipse cx="1180" cy="168" rx="280" ry="42" fill="#2e3463" opacity="0.5" />
            <ellipse cx="950" cy="196" rx="170" ry="28" fill="#3a3f70" opacity="0.35" />
        </g>
        <g className="lofi-cloud lofi-cloud-3" filter={`url(#${bgId}-soft)`}>
            <ellipse cx="760" cy="300" rx="300" ry="34" fill="#54406b" opacity="0.4" />
        </g>
        <circle className="lofi-sun" cx="1090" cy="560" r="220" fill={`url(#${bgId}-sun)`} />
        <circle className="lofi-sun-core" cx="1090" cy="560" r="62" fill="#ffe3b0" />
        <rect y="480" width="1600" height="220" fill={`url(#${bgId}-fog)`} />
        <path
            d="M0 640 L200 560 L420 636 L610 552 L840 640 L1040 566 L1240 644 L1440 574 L1600 630 L1600 900 L0 900 Z"
            fill="#1c1c3a"
        />
        <path
            d="M0 748 L240 656 L470 738 L700 648 L950 746 L1180 664 L1420 752 L1600 690 L1600 900 L0 900 Z"
            fill="#0f1027"
        />
        <g className="lofi-cabin">
            <circle cx="1163" cy="712" r="4.5" fill="#ffce7a" opacity="0.95" />
            <circle cx="1186" cy="706" r="3.5" fill="#ffce7a" opacity="0.85" />
            <circle cx="352" cy="756" r="4" fill="#ffce7a" opacity="0.8" />
        </g>
    </svg>
));
Scene_Lofi.displayName = "Scene_Lofi";

const Scene_Aurora = memo(({ bgId }: { bgId: string }) => (
    <svg
        className="canvas-scene canvas-scene-svg"
        viewBox="0 0 1600 900"
        preserveAspectRatio="xMidYMid slice"
        aria-hidden="true"
    >
        <defs>
            <linearGradient id={`${bgId}-sky`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#04070f" />
                <stop offset="55%" stopColor="#0a1024" />
                <stop offset="100%" stopColor="#101a36" />
            </linearGradient>
            <linearGradient id={`${bgId}-green`} x1="0" y1="0" x2="1" y2="0">
                <stop offset="0%" stopColor="#39f0a4" stopOpacity="0" />
                <stop offset="35%" stopColor="#39f0a4" stopOpacity="0.85" />
                <stop offset="70%" stopColor="#63f5c8" stopOpacity="0.6" />
                <stop offset="100%" stopColor="#63f5c8" stopOpacity="0" />
            </linearGradient>
            <linearGradient id={`${bgId}-teal`} x1="0" y1="0" x2="1" y2="0">
                <stop offset="0%" stopColor="#46c7f0" stopOpacity="0" />
                <stop offset="40%" stopColor="#46c7f0" stopOpacity="0.6" />
                <stop offset="100%" stopColor="#46c7f0" stopOpacity="0" />
            </linearGradient>
            <linearGradient id={`${bgId}-violet`} x1="0" y1="0" x2="1" y2="0">
                <stop offset="0%" stopColor="#a374ff" stopOpacity="0" />
                <stop offset="45%" stopColor="#a374ff" stopOpacity="0.5" />
                <stop offset="100%" stopColor="#c9a6ff" stopOpacity="0" />
            </linearGradient>
            <filter id={`${bgId}-blur`} x="-40%" y="-40%" width="180%" height="180%">
                <feGaussianBlur stdDeviation="24" />
            </filter>
        </defs>
        <rect width="1600" height="900" fill={`url(#${bgId}-sky)`} />
        {auroraStars.map((star, i) => (
            <circle key={i} cx={star.x} cy={star.y} r={star.r} fill="#dfe8ff" opacity={star.o} />
        ))}
        <g className="aurora-band aurora-band-1" filter={`url(#${bgId}-blur)`}>
            <path
                d="M-150 350 C 260 220, 620 470, 1020 300 S 1650 210, 1950 340"
                fill="none"
                stroke={`url(#${bgId}-green)`}
                strokeWidth="100"
                strokeLinecap="round"
            />
        </g>
        <g className="aurora-band aurora-band-2" filter={`url(#${bgId}-blur)`}>
            <path
                d="M-150 470 C 300 330, 700 560, 1100 400 S 1700 320, 2000 440"
                fill="none"
                stroke={`url(#${bgId}-teal)`}
                strokeWidth="70"
                strokeLinecap="round"
            />
        </g>
        <g className="aurora-band aurora-band-3" filter={`url(#${bgId}-blur)`}>
            <path
                d="M-150 590 C 350 460, 750 640, 1150 500 S 1750 440, 2050 540"
                fill="none"
                stroke={`url(#${bgId}-violet)`}
                strokeWidth="90"
                strokeLinecap="round"
            />
        </g>
        <path
            d="M0 780 L260 690 L520 770 L800 668 L1080 776 L1330 700 L1600 768 L1600 900 L0 900 Z"
            fill="#05070f"
        />
    </svg>
));
Scene_Aurora.displayName = "Scene_Aurora";

export interface CanvasBackgroundProps {
    preset: string;
    customImage?: string;
    dim?: number;
    className?: string;
}

let canvasBgIdSeq = 0;

export const CanvasBackground = memo(({ preset, customImage, dim, className }: CanvasBackgroundProps) => {
    const [bgId] = useState(() => `cbg${++canvasBgIdSeq}`);
    const dimVal = boundNumber(dim, 0, 1) ?? 0.3;
    let scene: React.ReactNode;
    switch (preset) {
        case "grid":
            scene = <div className="canvas-scene canvas-scene-grid" />;
            break;
        case "peaks":
            scene = <Scene_Peaks bgId={bgId} />;
            break;
        case "lofi":
            scene = <Scene_Lofi bgId={bgId} />;
            break;
        case "aurora":
            scene = <Scene_Aurora bgId={bgId} />;
            break;
        case "custom": {
            const url = customBgImageUrl(customImage);
            scene =
                url == null ? (
                    <div className="canvas-scene canvas-scene-default" />
                ) : (
                    <div className="canvas-scene canvas-scene-custom" style={{ backgroundImage: `url("${url}")` }} />
                );
            break;
        }
        default:
            scene = <div className="canvas-scene canvas-scene-default" />;
            break;
    }
    return (
        <div className={cn("canvas-bg", className)}>
            {scene}
            {dimVal > 0 && (
                <div className="canvas-bg-dim" style={{ "--canvas-dim": dimVal } as React.CSSProperties} />
            )}
        </div>
    );
});
CanvasBackground.displayName = "CanvasBackground";
