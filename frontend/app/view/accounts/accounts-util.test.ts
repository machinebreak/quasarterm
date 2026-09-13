// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { accountMatchesFilter, mergeAccountExports, providerTags, sanitizeFilename } from "./accounts-util";

function makeAccount(overrides: Partial<Account>): Account {
    return {
        id: "acc-1",
        provider: "claude",
        label: "",
        email: "",
        plantype: "",
        remoteid: "",
        createdat: 0,
        lastused: 0,
        isactive: false,
        quotaerror: "",
        ...overrides,
    } as Account;
}

describe("accountMatchesFilter", () => {
    const work = makeAccount({ id: "a1", label: "Work Max", email: "work@example.com", tags: ["work", "prod"] });
    const personal = makeAccount({ id: "a2", label: "Personal", email: "me@home.dev", tags: ["home"] });

    it("matches everything with an empty query and no tag filters", () => {
        expect(accountMatchesFilter(work, "", [])).toBe(true);
        expect(accountMatchesFilter(personal, "   ", [])).toBe(true);
    });

    it("matches label, email and id case-insensitively", () => {
        expect(accountMatchesFilter(work, "work max", [])).toBe(true);
        expect(accountMatchesFilter(work, "EXAMPLE", [])).toBe(true);
        expect(accountMatchesFilter(work, "a1", [])).toBe(true);
        expect(accountMatchesFilter(work, "home", [])).toBe(false);
    });

    it("matches tags too", () => {
        expect(accountMatchesFilter(work, "prod", [])).toBe(true);
    });

    it("requires every selected tag filter to be present", () => {
        expect(accountMatchesFilter(work, "", ["work"])).toBe(true);
        expect(accountMatchesFilter(work, "", ["work", "prod"])).toBe(true);
        expect(accountMatchesFilter(work, "", ["work", "home"])).toBe(false);
        expect(accountMatchesFilter(personal, "", ["work"])).toBe(false);
    });

    it("combines query and tag filters", () => {
        expect(accountMatchesFilter(work, "max", ["prod"])).toBe(true);
        expect(accountMatchesFilter(work, "max", ["home"])).toBe(false);
    });
});

describe("providerTags", () => {
    it("collects unique tags sorted alphabetically", () => {
        const accounts = [
            makeAccount({ id: "a1", tags: ["work", "zz"] }),
            makeAccount({ id: "a2", tags: ["home", "work"] }),
            makeAccount({ id: "a3" }),
        ];
        expect(providerTags(accounts)).toEqual(["home", "work", "zz"]);
    });
});

describe("mergeAccountExports", () => {
    it("merges account arrays from multiple exports", () => {
        const part1 = JSON.stringify({ version: 1, accounts: [{ id: "a1", provider: "claude" }] });
        const part2 = JSON.stringify({ version: 1, accounts: [{ id: "a2", provider: "claude" }] });
        const merged = JSON.parse(mergeAccountExports([part1, part2]));
        expect(merged.version).toBe(1);
        expect(merged.accounts).toHaveLength(2);
        expect(merged.accounts[0].id).toBe("a1");
        expect(merged.accounts[1].id).toBe("a2");
    });

    it("skips malformed fragments instead of failing", () => {
        const merged = JSON.parse(mergeAccountExports(["not json", JSON.stringify({ accounts: [] })]));
        expect(merged.accounts).toHaveLength(0);
    });
});

describe("sanitizeFilename", () => {
    it("strips path-hostile characters", () => {
        expect(sanitizeFilename('Work: "Max" <prod>')).toBe("Work-Max-prod");
    });

    it("falls back to account for empty labels", () => {
        expect(sanitizeFilename("")).toBe("account");
        expect(sanitizeFilename("...")).toBe("...");
    });
});
