// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

// Maps the first word(s) of a shell command to the account provider whose CLI
// is running. Only providers with a frontend logo are listed here.
const CommandToProvider: { [key: string]: string } = {
    claude: "claude",
    codex: "codex",
    "gh copilot": "github-copilot",
    copilot: "github-copilot",
    "cursor-agent": "cursor",
    cursor: "cursor",
    windsurf: "windsurf",
    kiro: "kiro",
    "kiro-cli": "kiro",
    grok: "grok",
    qoder: "qoder",
    zcode: "zcode",
    zed: "zed",
    codebuddy: "codebuddy",
    devin: "devin",
    opencode: "opencode",
    antigravity: "antigravity",
    workbuddy: "workbuddy",
};

// Strips leading env assignments ("FOO=bar", "env FOO=bar") so
// `env FOO=1 /usr/local/bin/claude --resume` normalizes to "claude --resume".
export function normalizeShellCommand(command: string): string {
    let cmd = command.trim().split(/\r?\n/)[0].trim();
    cmd = cmd.replace(/^env\s+/, "");
    cmd = cmd.replace(/^(?:\w+=(?:"[^"]*"|'[^']*'|\S+)\s+)+/, "");
    return cmd.trim();
}

export function providerIdFromCommand(command: string | null | undefined): string | null {
    if (!command) {
        return null;
    }
    const cmd = normalizeShellCommand(command);
    if (!cmd) {
        return null;
    }
    const tokens = cmd.split(/\s+/);
    const binary = tokens[0].split(/[\\/]/).pop();
    const twoWords = tokens.length > 1 ? `${binary} ${tokens[1]}` : binary;
    return CommandToProvider[twoWords] ?? CommandToProvider[binary] ?? null;
}
