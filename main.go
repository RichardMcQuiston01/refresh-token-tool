// Command chrometoken is a small desktop tool that mints a Chrome Web Store API
// refresh token via Google's OAuth loopback flow and stores it in a .env file.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"chrometoken/internal/core"
)

const (
	appTitle       = "Chrome Web Store Refresh Token"
	maskText       = "••••••••••••••••"
	revealDuration = 5 * time.Second
	autoCloseDelay = 15 * time.Second
	oauthTimeout   = 3 * time.Minute
)

// appState holds the widgets and the secret values. Secrets live only in
// memory and are never copied into a visible widget except the refresh token,
// which is shown briefly on double-click.
type appState struct {
	fyneApp fyne.App
	window  fyne.Window

	extensionIDEntry    *widget.Entry
	clientIDEntry       *widget.Entry
	clientSecretDisplay *widget.Entry
	tokenEntry          *revealEntry
	envPathEntry        *widget.Entry
	statusLabel         *widget.Label
	getTokenButton      *widget.Button
	cancelButton        *widget.Button

	clientSecret         string
	refreshToken         string
	tokenDisplayExpected string
	revealGeneration     int
	revealTimer          *time.Timer
	cancelOAuth          context.CancelFunc
}

func main() {
	fyneApp := app.NewWithID("com.mcq.chrome-refresh-token-tool")
	window := fyneApp.NewWindow(appTitle)

	state := &appState{fyneApp: fyneApp, window: window}
	content := state.buildContent()
	window.SetContent(content)
	window.Resize(fyne.NewSize(700, content.MinSize().Height))
	window.SetOnClosed(state.wipeSecrets)

	state.restoreLastEnvFile()
	window.ShowAndRun()
}

// ---- layout ----

func (a *appState) buildContent() fyne.CanvasObject {
	a.extensionIDEntry = widget.NewEntry()
	a.extensionIDEntry.SetPlaceHolder("32-character Chrome extension ID")

	a.clientIDEntry = widget.NewEntry()
	a.clientIDEntry.SetPlaceHolder("xxxx.apps.googleusercontent.com")

	// The client secret is never rendered: a disabled entry shows only a mask.
	a.clientSecretDisplay = widget.NewEntry()
	a.clientSecretDisplay.SetPlaceHolder("(not set)")
	a.clientSecretDisplay.Disable()
	editSecretButton := widget.NewButton("Edit", a.editClientSecret)

	// The token entry is read-only: any edit is reverted to the expected text.
	a.tokenEntry = newRevealEntry(a.revealToken)
	a.tokenEntry.SetPlaceHolder("(no token yet)")
	a.tokenEntry.OnChanged = func(text string) {
		if text != a.tokenDisplayExpected {
			a.tokenEntry.SetText(a.tokenDisplayExpected)
		}
	}
	copyButton := widget.NewButton("Copy", a.copyToken)

	a.envPathEntry = widget.NewEntry()
	a.envPathEntry.SetPlaceHolder("Optional: path to a .env file to read from and write to")
	a.envPathEntry.OnSubmitted = func(path string) { a.useEnvFile(strings.TrimSpace(path)) }
	browseButton := widget.NewButton("Browse…", a.browseForEnvFile)

	form := widget.NewForm(
		widget.NewFormItem("Extension ID", a.extensionIDEntry),
		widget.NewFormItem("Client ID", a.clientIDEntry),
		widget.NewFormItem("Client Secret", container.NewBorder(nil, nil, nil, editSecretButton, a.clientSecretDisplay)),
		widget.NewFormItem("Refresh Token", container.NewBorder(nil, nil, nil, copyButton, a.tokenEntry)),
		widget.NewFormItem(".env file", container.NewBorder(nil, nil, nil, browseButton, a.envPathEntry)),
	)

	hint := widget.NewLabel(fmt.Sprintf(
		"Double-click the token field to reveal it for %d seconds. Copy works without revealing it.",
		int(revealDuration/time.Second)))
	hint.Wrapping = fyne.TextWrapWord

	a.statusLabel = widget.NewLabel("Ready.")
	a.statusLabel.Wrapping = fyne.TextWrapWord

	a.getTokenButton = widget.NewButton("Get Refresh Token", a.startOAuth)
	a.getTokenButton.Importance = widget.HighImportance
	a.cancelButton = widget.NewButton("Cancel", func() {
		if a.cancelOAuth != nil {
			a.cancelOAuth()
		}
	})
	a.cancelButton.Hide()

	return container.NewVBox(
		form,
		hint,
		container.NewGridWithColumns(2, a.getTokenButton, a.cancelButton),
		a.statusLabel,
	)
}

func (a *appState) setStatus(message string) {
	a.statusLabel.SetText(message)
}

// ---- client secret (masked, replace-only) ----

func (a *appState) refreshSecretDisplay() {
	if a.clientSecret == "" {
		a.clientSecretDisplay.SetText("")
		return
	}
	a.clientSecretDisplay.SetText(maskText)
}

func (a *appState) editClientSecret() {
	newSecretEntry := widget.NewEntry()
	newSecretEntry.Password = true
	newSecretEntry.SetPlaceHolder("Paste the new client secret")

	items := []*widget.FormItem{widget.NewFormItem("New secret", newSecretEntry)}
	editDialog := dialog.NewForm("Replace Client Secret", "Save", "Cancel", items, func(confirmed bool) {
		if !confirmed {
			return
		}
		newSecret := strings.TrimSpace(newSecretEntry.Text)
		if newSecret == "" {
			a.setStatus("Client secret was not changed: the new value was empty.")
			return
		}
		a.clientSecret = newSecret
		a.refreshSecretDisplay()
		a.setStatus("Client secret replaced.")
	}, a.window)
	editDialog.Resize(fyne.NewSize(440, editDialog.MinSize().Height))
	editDialog.Show()
	a.window.Canvas().Focus(newSecretEntry)
}

// ---- refresh token (masked, reveal on double-click, copy without reveal) ----

func (a *appState) setTokenDisplay(text string) {
	a.tokenDisplayExpected = text
	a.tokenEntry.SetText(text)
}

func (a *appState) hideToken() {
	if a.refreshToken == "" {
		a.setTokenDisplay("")
		return
	}
	a.setTokenDisplay(maskText)
}

func (a *appState) revealToken() {
	if a.refreshToken == "" {
		return
	}
	a.setTokenDisplay(a.refreshToken)

	a.revealGeneration++
	generation := a.revealGeneration
	if a.revealTimer != nil {
		a.revealTimer.Stop()
	}
	a.revealTimer = time.AfterFunc(revealDuration, func() {
		fyne.Do(func() {
			if generation == a.revealGeneration {
				a.hideToken()
			}
		})
	})
}

func (a *appState) copyToken() {
	if a.refreshToken == "" {
		a.setStatus("Nothing to copy yet: no refresh token has been generated.")
		return
	}
	a.fyneApp.Clipboard().SetContent(a.refreshToken)
	a.setStatus("Refresh token copied to the clipboard.")
}

// ---- .env file selection ----

func (a *appState) browseForEnvFile() {
	openDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			a.setStatus("Could not open the file picker: " + err.Error())
			return
		}
		if reader == nil {
			return // cancelled
		}
		chosenPath := normalizeDialogPath(reader.URI().Path())
		_ = reader.Close()
		a.envPathEntry.SetText(chosenPath)
		a.useEnvFile(chosenPath)
	}, a.window)
	openDialog.Show()
}

// normalizeDialogPath converts a Fyne URI path to a native OS path. On Windows
// a file URI path looks like "/C:/Users/..." and needs its leading slash removed.
func normalizeDialogPath(uriPath string) string {
	if runtime.GOOS == "windows" && len(uriPath) >= 3 && uriPath[0] == '/' && uriPath[2] == ':' {
		uriPath = uriPath[1:]
	}
	return filepath.FromSlash(uriPath)
}

// useEnvFile loads values from the chosen file into the form and remembers the
// path for next launch.
func (a *appState) useEnvFile(path string) {
	if path == "" {
		return
	}
	envFile, err := core.LoadEnv(path)
	if err != nil {
		a.setStatus("Could not load the env file: " + err.Error())
		return
	}

	if !envFile.Existed {
		a.setStatus("That file does not exist yet; it will be created when the token is saved.")
	} else {
		if value, found := envFile.Get(core.EnvKeyExtensionID); found {
			a.extensionIDEntry.SetText(value)
		}
		if value, found := envFile.Get(core.EnvKeyClientID); found {
			a.clientIDEntry.SetText(value)
		}
		if value, found := envFile.Get(core.EnvKeyClientSecret); found {
			a.clientSecret = value
			a.refreshSecretDisplay()
		}
		a.setStatus("Loaded values from " + path)
	}

	a.rememberEnvPath(path)
}

func (a *appState) rememberEnvPath(path string) {
	configPath, err := core.ConfigFilePath()
	if err != nil {
		a.setStatus(a.statusLabel.Text + "\nNote: could not remember this file: " + err.Error())
		return
	}
	if err := core.WriteLastEnvPath(configPath, path); err != nil {
		a.setStatus(a.statusLabel.Text + "\nNote: could not remember this file: " + err.Error())
	}
}

func (a *appState) restoreLastEnvFile() {
	configPath, err := core.ConfigFilePath()
	if err != nil {
		a.setStatus("Could not locate the settings file: " + err.Error())
		return
	}
	lastPath, err := core.ReadLastEnvPath(configPath)
	if err != nil {
		a.setStatus("Could not read saved settings: " + err.Error())
		return
	}
	if lastPath == "" {
		return
	}
	if _, statErr := os.Stat(lastPath); statErr != nil && errors.Is(statErr, fs.ErrNotExist) {
		a.setStatus("The last used .env file no longer exists: " + lastPath)
		return
	}
	a.envPathEntry.SetText(lastPath)
	a.useEnvFile(lastPath)
}

// ---- OAuth flow ----

func (a *appState) startOAuth() {
	extensionID := strings.TrimSpace(a.extensionIDEntry.Text)
	clientID := strings.TrimSpace(a.clientIDEntry.Text)
	clientSecret := a.clientSecret
	if extensionID == "" || clientID == "" || clientSecret == "" {
		a.setStatus("Extension ID, Client ID and Client Secret are all required before requesting a token.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), oauthTimeout)
	a.cancelOAuth = cancel
	a.getTokenButton.Disable()
	a.cancelButton.Show()
	a.setStatus("Waiting for you to authorize in the browser…")

	go func() {
		token, err := core.RunOAuth(ctx,
			core.OAuthRequest{ClientID: clientID, ClientSecret: clientSecret},
			a.openInBrowser)
		cancel()
		fyne.Do(func() {
			a.finishOAuth(core.EnvValues{
				ExtensionID:  extensionID,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				RefreshToken: token,
			}, err)
		})
	}()
}

// openInBrowser is called from the OAuth goroutine, so it hops to the UI thread.
func (a *appState) openInBrowser(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("the authorization URL was malformed: %w", err)
	}
	var openErr error
	fyne.DoAndWait(func() {
		openErr = a.fyneApp.OpenURL(parsedURL)
	})
	return openErr
}

func (a *appState) finishOAuth(values core.EnvValues, oauthErr error) {
	a.cancelButton.Hide()
	a.getTokenButton.Enable()
	a.cancelOAuth = nil

	if oauthErr != nil {
		a.setStatus("Authorization failed: " + oauthErr.Error())
		return
	}

	a.refreshToken = values.RefreshToken
	a.hideToken()

	envPath := strings.TrimSpace(a.envPathEntry.Text)
	if envPath == "" {
		a.setStatus("Refresh token received. No .env file is selected, so nothing was saved — use Copy.")
		return
	}

	report, err := core.SaveTokenToEnv(envPath, values)
	if err != nil {
		a.setStatus("Token received but saving failed: " + err.Error() +
			"\nThe token is still available via Copy.")
		return
	}
	a.rememberEnvPath(envPath)
	a.setStatus(report.Summary())
	a.showCloseDialog()
}

// ---- auto-close confirmation ----

// showCloseDialog asks whether to close the app. With no answer inside
// autoCloseDelay it closes anyway, so secrets do not linger in memory.
func (a *appState) showCloseDialog() {
	remainingSeconds := int(autoCloseDelay / time.Second)
	countdownLabel := widget.NewLabel(countdownText(remainingSeconds))
	message := widget.NewLabel("The refresh token was saved and verified.\nClose the application now?")

	stopCountdown := make(chan struct{})
	var stopOnce sync.Once
	cancelCountdown := func() { stopOnce.Do(func() { close(stopCountdown) }) }

	closeDialog := dialog.NewCustomConfirm("Saved — close application?", "Close now", "Keep open",
		container.NewVBox(message, countdownLabel),
		func(closeNow bool) {
			cancelCountdown()
			if closeNow {
				a.quit()
			}
		}, a.window)
	closeDialog.Show()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for remainingSeconds > 0 {
			select {
			case <-stopCountdown:
				return
			case <-ticker.C:
				remainingSeconds--
				secondsLeft := remainingSeconds
				fyne.Do(func() { countdownLabel.SetText(countdownText(secondsLeft)) })
			}
		}
		select {
		case <-stopCountdown:
			return
		default:
		}
		fyne.Do(a.quit)
	}()
}

func countdownText(seconds int) string {
	return fmt.Sprintf("Closing automatically in %d seconds…", seconds)
}

func (a *appState) quit() {
	a.wipeSecrets()
	a.fyneApp.Quit()
}

// wipeSecrets drops references to the secrets. Go strings are immutable, so
// this is best-effort: it makes them unreachable rather than zeroing memory.
func (a *appState) wipeSecrets() {
	a.clientSecret = ""
	a.refreshToken = ""
	a.tokenDisplayExpected = ""
}
