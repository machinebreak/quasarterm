# Contributing to Quasar

Quasar is an independent fork of [Wave Terminal](https://github.com/wavetermdev/waveterm).
Issues and pull requests are welcome.

## Reporting bugs / requesting features

Use [Issues](https://github.com/machinebreak/quasarterm/issues) — templates are provided.
For anything security-related, please follow [SECURITY.md](./SECURITY.md) instead of opening a public issue.

## Development setup

Requirements: Node.js 22+, Go 1.25+, [Task](https://taskfile.dev/), [Zig](https://ziglang.org/) (CGO/SQLite on Windows).

```sh
task init
task dev
```

Before opening a PR:

```sh
npx vitest run        # tests
npx tsc --noEmit      # type check
```

Component previews for UI work: `cd frontend/preview && npx vite`.

## Pull requests

- Keep changes focused — one topic per PR.
- Describe **what** changed and **how you tested it**.
- Match the existing code style (Prettier and ESLint configs are in the repo).
- If the change is user-visible, mention it so it can go into the changelog.
