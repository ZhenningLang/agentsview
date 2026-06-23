# AgentsView Desktop (Tauri)

This directory contains an experimental Tauri desktop wrapper for AgentsView.

The wrapper does not reimplement the web app. Instead, it:

1. Builds the existing Go `agentsview` binary.
1. Packages it as a Tauri sidecar.
1. Starts it with `serve --no-browser` on a local port.
1. Loads the local URL in a native webview.
1. Adds a macOS menu-bar (tray) icon as a quick-access entry point.

## Requirements

- Rust toolchain (`rustc`, `cargo`)
- Node.js and npm
- Go (with CGO enabled; same requirements as the main project)

## Usage

```bash
npm install
npm run tauri:dev
npm run tauri:build
npm run tauri:build:macos-app
npm run tauri:build:windows
```

The `prepare-sidecar` step runs automatically for `tauri:dev` and `tauri:build`.
It builds `agentsview` and copies it to
`src-tauri/binaries/agentsview-<target-triple>`.

The macOS `.app` bundle is written to
`src-tauri/target/release/bundle/macos/AgentsView.app`.

## Menu Bar (macOS)

The app installs a menu-bar (status bar) icon that acts as a persistent
quick-access entry point:

- Left-click the icon to open/focus the AgentsView window.
- Right-click for a menu: `Open AgentsView` / `Quit AgentsView`.
- Clicking the window's red close button **hides the window to the menu
  bar** instead of quitting — the app and its local backend keep running.
- To fully quit (and stop the backend), use the tray's `Quit AgentsView`
  item or `Cmd+Q`.

## Environment Notes (Desktop)

When launched from Finder/Explorer, desktop apps usually do not inherit your
shell profile (`.zshrc`, `.bashrc`), which can hide CLIs like `claude`, `codex`,
and `gemini` from `PATH`.

On macOS/Linux, the Tauri wrapper loads login-shell env (`$SHELL -lic 'env -0'`)
for the sidecar (with a short timeout to avoid startup hangs). On Windows this
probing is skipped by default.

Optional escape hatch:

- Add overrides in `~/.agentsview/desktop.env`:
  - Example: `PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin`
  - Example: `ANTHROPIC_API_KEY=...`
- On Windows, this file resolves to `%USERPROFILE%\\.agentsview\\desktop.env`.
- Force a custom PATH with `AGENTSVIEW_DESKTOP_PATH`.
- Skip login-shell env loading with `AGENTSVIEW_DESKTOP_SKIP_LOGIN_SHELL_ENV=1`.
