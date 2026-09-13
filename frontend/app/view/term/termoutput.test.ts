// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { stripBlackBackgroundEscapes } from "./termoutput";

describe("stripBlackBackgroundEscapes", () => {
    it("rewrites the ANSI black background", () => {
        expect(stripBlackBackgroundEscapes("\u001b[40mhi\u001b[49m")).toBe("\u001b[49mhi\u001b[49m");
    });

    it("keeps other parameters of combined sequences", () => {
        expect(stripBlackBackgroundEscapes("\u001b[40;33mtext")).toBe("\u001b[33mtext");
        expect(stripBlackBackgroundEscapes("\u001b[1;40mtext")).toBe("\u001b[1mtext");
    });

    it("rewrites 256-color black backgrounds (palette 0 and 16)", () => {
        expect(stripBlackBackgroundEscapes("\u001b[48;5;0mx")).toBe("\u001b[49mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;5;16mx")).toBe("\u001b[49mx");
    });

    it("rewrites the 256-color near-black grays (232-234)", () => {
        expect(stripBlackBackgroundEscapes("\u001b[48;5;232mx")).toBe("\u001b[49mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;5;233mx")).toBe("\u001b[49mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;5;234mx")).toBe("\u001b[49mx");
    });

    it("keeps darker-but-intentional 256-color backgrounds", () => {
        expect(stripBlackBackgroundEscapes("\u001b[48;5;235mx")).toBe("\u001b[48;5;235mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;5;236mx")).toBe("\u001b[48;5;236mx");
    });

    it("rewrites pure and near-black truecolor backgrounds", () => {
        expect(stripBlackBackgroundEscapes("\u001b[48;2;0;0;0mx")).toBe("\u001b[49mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;2;3;2;1mx")).toBe("\u001b[49mx");
        expect(stripBlackBackgroundEscapes("\u001b[37;48;2;0;0;0mtext")).toBe("\u001b[37mtext");
    });

    it("rewrites dark-gray truecolor backgrounds (up to the neutral threshold)", () => {
        expect(stripBlackBackgroundEscapes("\u001b[48;2;3;2;1mx")).toBe("\u001b[49mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;2;18;18;18mx")).toBe("\u001b[49mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;2;30;30;30mx")).toBe("\u001b[49mx");
    });

    it("keeps intentional panel grays and colored backgrounds", () => {
        expect(stripBlackBackgroundEscapes("\u001b[48;2;41;41;41mx")).toBe("\u001b[48;2;41;41;41mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;2;64;64;64mx")).toBe("\u001b[48;2;64;64;64mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;2;74;34;29mx")).toBe("\u001b[48;2;74;34;29mx");
        expect(stripBlackBackgroundEscapes("\u001b[48;2;0;0;255mx")).toBe("\u001b[48;2;0;0;255mx");
    });

    it("does not touch resets, foregrounds or non-SGR sequences", () => {
        const input = "\u001b[0m\u001b[m\u001b[31mred\u001b[38;2;0;0;0mdark-fg\u001b[1;1H\u001b[?25l";
        expect(stripBlackBackgroundEscapes(input)).toBe(input);
    });

    it("passes through data without escapes", () => {
        expect(stripBlackBackgroundEscapes("plain text")).toBe("plain text");
        expect(stripBlackBackgroundEscapes("")).toBe("");
    });
});
