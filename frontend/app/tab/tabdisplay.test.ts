// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { getFolderBasename, getTabDisplayName, isAutoTabName } from "./tabdisplay";

describe("tabdisplay", () => {
    it("detects auto-generated tab names", () => {
        expect(isAutoTabName("T1")).toBe(true);
        expect(isAutoTabName("T42")).toBe(true);
        expect(isAutoTabName("")).toBe(true);
        expect(isAutoTabName(null)).toBe(true);
        expect(isAutoTabName("Backend")).toBe(false);
        expect(isAutoTabName("Tab1")).toBe(false);
        expect(isAutoTabName("T1-x")).toBe(false);
    });

    it("extracts the folder basename from windows and posix paths", () => {
        expect(getFolderBasename("C:\\Users\\Usuario\\Projects\\MoviesAndChill")).toBe("MoviesAndChill");
        expect(getFolderBasename("/home/user/projects/api")).toBe("api");
        expect(getFolderBasename("/home/user/projects/api/")).toBe("api");
        expect(getFolderBasename("MoviesAndChill")).toBe("MoviesAndChill");
        expect(getFolderBasename(null)).toBe(null);
    });

    it("shows the project folder name for auto-generated tab names", () => {
        expect(getTabDisplayName("T1", "MoviesAndChill")).toBe("MoviesAndChill");
        expect(getTabDisplayName("", "MoviesAndChill")).toBe("MoviesAndChill");
        expect(getTabDisplayName("MyTab", "MoviesAndChill")).toBe("MyTab");
        expect(getTabDisplayName("T1", null)).toBe("T1");
        expect(getTabDisplayName("T1", "")).toBe("T1");
        expect(getTabDisplayName("MyTab", null)).toBe("MyTab");
    });
});
