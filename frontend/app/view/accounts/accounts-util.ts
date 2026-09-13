// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

export function accountMatchesFilter(account: Account, query: string, tags: string[]): boolean {
    const trimmed = query.trim().toLowerCase();
    if (trimmed !== "") {
        const haystack = [account.label, account.email, account.id, ...(account.tags ?? [])]
            .filter((part) => part != null && part !== "")
            .join(" ")
            .toLowerCase();
        if (!haystack.includes(trimmed)) {
            return false;
        }
    }
    if (tags.length > 0) {
        const accountTags = account.tags ?? [];
        if (!tags.every((tag) => accountTags.includes(tag))) {
            return false;
        }
    }
    return true;
}

export function providerTags(accounts: Account[]): string[] {
    const seen = new Set<string>();
    for (const account of accounts) {
        for (const tag of account.tags ?? []) {
            seen.add(tag);
        }
    }
    return Array.from(seen).sort((a, b) => a.localeCompare(b));
}

export function mergeAccountExports(parts: string[]): string {
    const merged = { version: 1, exported: Date.now(), accounts: [] as unknown[] };
    for (const part of parts) {
        try {
            const parsed = JSON.parse(part);
            if (Array.isArray(parsed?.accounts)) {
                merged.accounts.push(...parsed.accounts);
            }
        } catch {
            // skip malformed export fragments
        }
    }
    return JSON.stringify(merged, null, 2);
}

export function sanitizeFilename(name: string): string {
    const cleaned = (name || "account")
        .replace(/[\\/:*?"<>|]+/g, "-")
        .replace(/\s+/g, "-")
        .replace(/-+/g, "-")
        .replace(/^-|-$/g, "");
    return cleaned.substring(0, 48) || "account";
}
