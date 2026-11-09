# Chrome Web Store Refresh Token Tool

- Author: [Richard McQuiston](mailto:richard.mcquiston@gmail.com)
- Website: https://richardmcquiston.com/

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/P1V529Z7ZP)

## Table of Contents

| [Overview](#overview) | [Getting Started](#getting-started) | [Features](#features) | [Project Layout](#project-layout) | [Troubleshooting](#troubleshooting) | [Buy Me a Coffee](#buy-me-a-coffee) | [License](#license) | [Copyright](#copyright) |

## Overview

This is a small desktop app that gets a Chrome Web Store refresh token from Google. It then saves the token in a `.env` file. It is for developers who publish Chrome extensions with the Chrome Web Store Application Programming Interface (API).

You sign in once in your browser. The app handles the Open Authorization (OAuth) steps for you. It is written in Go and uses the Fyne toolkit for the window.

[Back to Top](#table-of-contents)

## Getting Started

### Prerequisites

- Go 1.24.7 or newer. This is the version in `go.mod`.
- A C compiler, because Fyne needs one. On Windows, use MinGW-w64 from MSYS2.
- A Google OAuth client of the "Desktop app" type. You need its client ID and client secret.
- The ID of your Chrome extension (32 characters).

### Installation

1. Clone the repository and open the folder.

   ```
   git clone https://github.com/RichardMcQuiston01/refresh-token-tool.git
   cd refresh-token-tool
   ```

2. Download the dependencies.

   ```
   go mod tidy
   ```

3. Build the app.

   ```
   go build -ldflags "-H=windowsgui -s -w" -o chrome-refresh-token.exe .
   ```

   The `-H=windowsgui` flag hides the console window on Windows. The `-s -w` flags make the file smaller. The first build is slow because Fyne takes a while to compile.

   On other systems, leave out `-H=windowsgui` and pick any output name:

   ```
   go build -ldflags "-s -w" -o chrome-refresh-token .
   ```

4. Optional: run the tests. They cover the core logic and need no window system.

   ```
   go test ./internal/...
   ```

### Usage

1. Start the app you built (`chrome-refresh-token.exe` on Windows).
2. Optional: pick a `.env` file with **Browse…**, or type its path and press Enter. The app loads your extension ID, client ID, and client secret from it. The app remembers the path for next time.
3. Fill in any empty fields. To set the client secret, click **Edit**.
4. Click **Get Refresh Token**. Your browser opens a Google page. Approve the request.
5. Return to the app. If you picked a `.env` file, the app saves the token and checks the file. A dialog asks if you want to close the app. If you do not answer, the app closes after 15 seconds.
6. If you did not pick a `.env` file, nothing is saved. Click **Copy** to put the token on your clipboard.

The app waits up to 3 minutes for you to approve. Click **Cancel** to stop sooner.

### Examples

The app writes these four keys to the `.env` file. The values here are made up.

```
CHROME_EXTENSION_ID=abcdefghijklmnopabcdefghijklmnop
CHROME_CLIENT_ID=1234567890-example.apps.googleusercontent.com
CHROME_CLIENT_SECRET=your-client-secret
CHROME_REFRESH_TOKEN=the-token-the-app-writes
```

If `.env` already exists, the app first copies it to `.env.bak` in the same folder. Then it updates the four keys and leaves your other lines alone.

[Back to Top](#table-of-contents)

## Features

- **Masked secrets.** The client secret and refresh token show as dots. **Edit** replaces the secret and never shows it.
- **Reveal on demand.** Double-click the token field to see the token for 5 seconds. **Copy** never reveals it.
- **Safe sign-in.** The app listens on your own computer (`127.0.0.1`, random port) to receive Google's reply. It uses Proof Key for Code Exchange (PKCE) and checks a random state value. It asks Google for offline access so that Google returns a refresh token.
- **Careful saving.** Comments, blank lines, key order, and line endings in your `.env` file stay the same. After the save, the app reads the file again to check the values. It refuses to save an empty token.
- **Backup protection.** Before it saves over an existing file, the app adds `<name>.bak` to your `.gitignore` if the file is inside a Git repository that has one. If it cannot do this, it stops before it makes the backup.
- **Short-lived secrets.** Secrets stay in memory only. The app drops them when it closes. This is a best-effort step, because Go does not let the app erase text in memory.
- **Remembered file.** The last `.env` path is saved in `settings.ini`. It is in a `chrome-refresh-token-tool` folder inside your user config folder. On Windows, that is `%AppData%`.

[Back to Top](#table-of-contents)

## Project Layout

- `main.go` and `revealentry.go`: the window and its buttons.
- `internal/core`: the logic that does not need a window. It handles `.env` files, `.gitignore` checks, the OAuth flow, and settings. It uses only the Go standard library and has tests.

[Back to Top](#table-of-contents)

## Troubleshooting

- **"Google's response contained no refresh_token."** You probably approved this app before. Go to https://myaccount.google.com/permissions, remove access for the app, and try again.
- **"timed out waiting for authorization in the browser"** You did not approve within 3 minutes. Click **Get Refresh Token** again.
- **"Token received but saving failed."** The token is still in the app. Click **Copy** to save it by hand.
- **The build fails and mentions `gcc` or `cgo`.** Fyne needs a C compiler. Install one (see Prerequisites) and make sure it is on your `PATH`. Then build again.
- **"Git repository detected but no .gitignore exists."** The `.bak` backup is not ignored by Git. Add `.env.bak` to a `.gitignore` before you commit.

[Back to Top](#table-of-contents)

## Buy Me a Coffee

If this app, code, or repository has helped you or someone you know, please consider donating. I appreciate any help to offset the costs of development and/or AI Credits.

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/P1V529Z7ZP)

[Back to Top](#table-of-contents)

## License

Apache 2

[Back to Top](#table-of-contents)

## Copyright

(c)2026 Richard McQuiston. All rights reserved.

[Back to Top](#table-of-contents)
