# CHANGELOG

All notable changes to this project are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

The project has no Git tags yet. The version numbers and dates below are approximate and come from the Git history.

## [Unreleased]

### Added

- README: build command for Linux and macOS, and a Troubleshooting entry for C compiler errors.
- `.gitignore`: rules for the built binary, `.env.*` backup files, and local Claude Code settings.

### Changed

- `.gitignore` now uses the standard Go template.
- README: Usage no longer assumes a Windows file name.

## [0.1.0] - 2026-11-06

### Added

- Desktop app, built with Go and Fyne, that gets a Chrome Web Store refresh token from Google.
- OAuth sign-in with Proof Key for Code Exchange (PKCE) and a random state check. A local listener on `127.0.0.1` receives Google's reply. The app waits up to 3 minutes.
- Saving of `CHROME_EXTENSION_ID`, `CHROME_CLIENT_ID`, `CHROME_CLIENT_SECRET`, and `CHROME_REFRESH_TOKEN` to a `.env` file. Comments, key order, and line endings stay the same. The app reads the file back to check it and refuses to save an empty token.
- Backup of an existing `.env` file to `.env.bak`, and automatic `.gitignore` entry for the backup inside a Git repository.
- Masked secret fields, **Edit** for the client secret, **Copy** for the token, and double-click to reveal the token for 5 seconds.
- Memory of the last `.env` path in `settings.ini` in the user config folder.
- Auto-close prompt after a successful save.
- Unit tests in `internal/core` for the `.env`, `.gitignore`, OAuth, settings, and save logic.
- Go module file (`go.mod`, Go 1.24.7).

### Changed

- README expanded with prerequisites, build and usage steps, features, project layout, and troubleshooting.
- `.gitignore` reduced to the entries the project needs.

## [0.0.1] - 2026-11-04

### Added

- Initial project files: Apache 2 `LICENSE`, `COPYRIGHT`, `README.md`, placeholder `CHANGELOG.md` and `ROADMAP.md`, and the donate image.
- Prettier settings, a starter `.gitignore`, and GitHub ruleset files for branch and tag protection.
- Project Git workflow skill for Claude Code.
