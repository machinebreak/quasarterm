# Changelog

All notable changes to Quasar are documented in this file.
Format based on [Keep a Changelog](https://keepachangelog.com/); versioning follows [SemVer](https://semver.org/).

## [0.1.1] - 2026-09-13

### Fixed

- Dark backgrounds painted by AI CLIs (near-black panels, separators and hint rows) no longer render as opaque slabs over transparent themes — the neutralization range at the terminal write path now covers the whole near-black family (truecolor `≤ #202020`, 256-color grays 232-234), while intentional panel fills like the `#292929` composer stay untouched.
- Terminals with transparency now use the DOM renderer instead of WebGL: the WebGL renderer composites painted backgrounds opaquely, which showed as dark boxes over the blended transparent background.

## [0.1.0] - 2026-09-13

First public release 🎉

### Added

- **AI CLI account manager** — import the account already logged into Codex / Claude, one-click switching, quota monitoring (5h / weekly windows), automatic switch when a quota window is exhausted, and "Switch & Continue" to swap account and resume the latest conversation.
- **Agent Mission Control** — every agent run in the workspace at a glance (running / waiting for input / interrupted / finished), with one-click resume for interrupted runs and per-tab CLI logos with attention indicators.
- **Canvas mode** (experimental) — free-form window canvas with saved layouts and backgrounds.
- **Privacy-first defaults** — no telemetry, no cloud services, no update pings; notifications off by default; everything stored locally (SQLite).
- Windows x64 builds: NSIS installer, MSI package and portable zip.

### Notes

- The installer is not code-signed yet — Windows SmartScreen may ask you to confirm.
- Built on [Wave Terminal](https://github.com/wavetermdev/waveterm) (Apache-2.0); account manager UI inspired by [cockpit-tools](https://github.com/jlcodes99/cockpit-tools).
