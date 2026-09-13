# Quasar

Quasar is a modern terminal with an integrated AI account manager. It is a personal fork of
[Wave Terminal](https://github.com/wavetermdev/waveterm) (Apache-2.0) with the AI assistant,
cloud connectivity and Wave branding removed, plus a built-in account manager inspired by
[cockpit-tools](https://github.com/jlcodes99/cockpit-tools).

## Features

- Full terminal emulator with blocks, editor, preview and durable SSH sessions
- Integrated AI CLI account manager:
  - Import the account currently logged in to the Codex / Claude CLI
  - Switch accounts with one click
  - Quota monitoring (5h / weekly windows)
  - "Switch & Continue": swap account and resume the latest conversation in the terminal
  - Automatic account switch when a quota window is exhausted
- No telemetry, no cloud services, no update pings

## Quick start

1. **Open a project** — pick a folder in the launcher; Quasar opens a terminal in it.
2. **Run an agent** — start `codex`, `claude`, or any CLI in the terminal; Quasar detects it and shows its logo in the tab.
3. **Track it** — the **Agents** panel (right rail) shows running / waiting / interrupted agents, and **Accounts** manages CLI logins and quotas.

## Development

Requirements:

- Node.js 22+
- Go 1.25+
- [Task](https://taskfile.dev/)
- [Zig](https://ziglang.org/) (for CGO / SQLite on Windows)

```sh
task init
task dev
```

Package a production build for the current platform:

```sh
task package
```

## Bugs & requests

Open an issue at [github.com/machinebreak/orion-term/issues](https://github.com/machinebreak/orion-term/issues).

## License

Apache-2.0. Wave Terminal copyright notices are preserved in `LICENSE` and `NOTICE`.
