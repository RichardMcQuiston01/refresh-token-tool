package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("could not create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read %s: %v", path, err)
	}
	return string(data)
}

// ---- envfile ----

func TestEnvSetPreservesCommentsAndOrder(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	writeTestFile(t, envPath, "# deploy secrets\nCHROME_CLIENT_ID=old\n\nOTHER=\"keep me\"\n")

	envFile, err := LoadEnv(envPath)
	if err != nil {
		t.Fatal(err)
	}
	envFile.Set("CHROME_CLIENT_ID", "new")
	envFile.Set("CHROME_REFRESH_TOKEN", "tok")
	if err := envFile.Save(); err != nil {
		t.Fatal(err)
	}

	want := "# deploy secrets\nCHROME_CLIENT_ID=new\n\nOTHER=\"keep me\"\nCHROME_REFRESH_TOKEN=tok\n"
	if got := readTestFile(t, envPath); got != want {
		t.Fatalf("unexpected file content:\n got: %q\nwant: %q", got, want)
	}
}

func TestEnvGetStripsQuotesAndExport(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	writeTestFile(t, envPath, "export A='one'\nB=\"two\"\nC=three\n")
	envFile, err := LoadEnv(envPath)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"A": "one", "B": "two", "C": "three"} {
		if got, ok := envFile.Get(key); !ok || got != want {
			t.Errorf("Get(%s) = %q, %v; want %q", key, got, ok, want)
		}
	}
}

func TestEnvPreservesCRLF(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	writeTestFile(t, envPath, "A=1\r\nB=2\r\n")
	envFile, _ := LoadEnv(envPath)
	envFile.Set("B", "3")
	if err := envFile.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, envPath); got != "A=1\r\nB=3\r\n" {
		t.Fatalf("CRLF not preserved: %q", got)
	}
}

func TestLoadEnvMissingFile(t *testing.T) {
	envFile, err := LoadEnv(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil || envFile.Existed {
		t.Fatalf("expected empty, non-existing EnvFile; got existed=%v err=%v", envFile != nil && envFile.Existed, err)
	}
}

// ---- gitignore ----

func makeRepo(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGitIgnoreOutcomes(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(t *testing.T, root string)
		wantOutcome GitIgnoreOutcome
		wantContent string // expected .gitignore content in root ("" = skip check)
	}{
		{
			name:        "not a repo",
			setup:       func(t *testing.T, root string) {},
			wantOutcome: GitIgnoreNotRepo,
		},
		{
			name:        "repo without gitignore",
			setup:       func(t *testing.T, root string) { makeRepo(t, root) },
			wantOutcome: GitIgnoreNoFile,
		},
		{
			name: "appends and adds missing trailing newline",
			setup: func(t *testing.T, root string) {
				makeRepo(t, root)
				writeTestFile(t, filepath.Join(root, ".gitignore"), "node_modules")
			},
			wantOutcome: GitIgnoreAppended,
			wantContent: "node_modules\n.env.bak\n",
		},
		{
			name: "already covered by wildcard",
			setup: func(t *testing.T, root string) {
				makeRepo(t, root)
				writeTestFile(t, filepath.Join(root, ".gitignore"), ".env*\n")
			},
			wantOutcome: GitIgnoreAlreadyCovered,
			wantContent: ".env*\n",
		},
		{
			name: "negation leaves it uncovered",
			setup: func(t *testing.T, root string) {
				makeRepo(t, root)
				writeTestFile(t, filepath.Join(root, ".gitignore"), "*.bak\n!.env.bak\n")
			},
			wantOutcome: GitIgnoreAppended,
			wantContent: "*.bak\n!.env.bak\n.env.bak\n",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			testCase.setup(t, root)
			result, err := EnsureGitIgnored(filepath.Join(root, ".env.bak"))
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != testCase.wantOutcome {
				t.Fatalf("outcome = %v, want %v", result.Outcome, testCase.wantOutcome)
			}
			if testCase.wantContent != "" {
				if got := readTestFile(t, filepath.Join(root, ".gitignore")); got != testCase.wantContent {
					t.Fatalf(".gitignore = %q, want %q", got, testCase.wantContent)
				}
			}
		})
	}
}

func TestGitIgnoreFindsParentRepoRoot(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root)
	writeTestFile(t, filepath.Join(root, ".gitignore"), "dist\n")
	subDirectory := filepath.Join(root, "chrome-extension")
	if err := os.MkdirAll(subDirectory, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := EnsureGitIgnored(filepath.Join(subDirectory, ".env.bak"))
	if err != nil || result.Outcome != GitIgnoreAppended {
		t.Fatalf("outcome=%v err=%v", result.Outcome, err)
	}
	if got := readTestFile(t, filepath.Join(root, ".gitignore")); got != "dist\n.env.bak\n" {
		t.Fatalf("root .gitignore = %q", got)
	}
}

// ---- save ----

func TestSaveTokenToEnvFullFlow(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root)
	writeTestFile(t, filepath.Join(root, ".gitignore"), "dist\n")
	envPath := filepath.Join(root, ".env")
	original := "# keep\nCHROME_CLIENT_ID=old-id\nUNRELATED=1\n"
	writeTestFile(t, envPath, original)

	report, err := SaveTokenToEnv(envPath, EnvValues{
		ExtensionID: "ext", ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := readTestFile(t, filepath.Join(root, ".env.bak")); got != original {
		t.Errorf("backup content = %q, want original %q", got, original)
	}
	if got := readTestFile(t, filepath.Join(root, ".gitignore")); got != "dist\n.env.bak\n" {
		t.Errorf(".gitignore = %q", got)
	}
	if report.GitIgnore.Outcome != GitIgnoreAppended {
		t.Errorf("git outcome = %v", report.GitIgnore.Outcome)
	}

	saved := readTestFile(t, envPath)
	for _, want := range []string{
		"# keep\n", "CHROME_CLIENT_ID=id\n", "UNRELATED=1\n",
		"CHROME_EXTENSION_ID=ext\n", "CHROME_CLIENT_SECRET=secret\n", "CHROME_REFRESH_TOKEN=refresh\n",
	} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved file missing %q:\n%s", want, saved)
		}
	}
}

func TestSaveTokenToEnvNewFileHasNoBackup(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	report, err := SaveTokenToEnv(envPath, EnvValues{ExtensionID: "e", ClientID: "c", ClientSecret: "s", RefreshToken: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CreatedNewFile || report.BackupPath != "" {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(BackupPathFor(envPath)); err == nil {
		t.Fatal("backup should not exist for a newly created file")
	}
}

func TestSaveTokenToEnvRejectsEmptyToken(t *testing.T) {
	if _, err := SaveTokenToEnv(filepath.Join(t.TempDir(), ".env"), EnvValues{}); err == nil {
		t.Fatal("expected an error for an empty refresh token")
	}
}

// ---- config ----

func TestConfigRoundTrip(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "sub", "settings.ini")

	if got, err := ReadLastEnvPath(configPath); err != nil || got != "" {
		t.Fatalf("missing config: got %q, %v", got, err)
	}
	wantPath := `C:\Users\McQ\repo\chrome-extension\.env`
	if err := WriteLastEnvPath(configPath, wantPath); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadLastEnvPath(configPath); err != nil || got != wantPath {
		t.Fatalf("got %q, %v; want %q", got, err, wantPath)
	}
}

// ---- oauth ----

func fakeBrowser(t *testing.T, callbackQuery func(state string) string) BrowserOpener {
	t.Helper()
	return func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		query := parsed.Query()
		if query.Get("code_challenge") == "" || query.Get("code_challenge_method") != "S256" {
			t.Errorf("PKCE parameters missing from %s", authURL)
		}
		target := query.Get("redirect_uri") + "/?" + callbackQuery(query.Get("state"))
		go func() {
			response, err := http.Get(target)
			if err == nil {
				response.Body.Close()
			}
		}()
		return nil
	}
}

func useFakeTokenServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	original := TokenEndpoint
	TokenEndpoint = server.URL
	t.Cleanup(func() {
		TokenEndpoint = original
		server.Close()
	})
}

func TestRunOAuthSuccess(t *testing.T) {
	useFakeTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		for _, field := range []string{"code", "code_verifier", "client_id", "client_secret", "redirect_uri"} {
			if r.Form.Get(field) == "" {
				t.Errorf("token request missing %s", field)
			}
		}
		if r.Form.Get("code") != "abc" || r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("unexpected token form: %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"refresh_token":"1//refresh","access_token":"x"}`))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	token, err := RunOAuth(ctx, OAuthRequest{ClientID: "id", ClientSecret: "secret"},
		fakeBrowser(t, func(state string) string { return "code=abc&state=" + url.QueryEscape(state) }))
	if err != nil || token != "1//refresh" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestRunOAuthIgnoresWrongState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	_, err := RunOAuth(ctx, OAuthRequest{ClientID: "id", ClientSecret: "secret"},
		fakeBrowser(t, func(string) string { return "code=abc&state=forged" }))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout after forged state, got %v", err)
	}
}

func TestRunOAuthGoogleError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := RunOAuth(ctx, OAuthRequest{ClientID: "id", ClientSecret: "secret"},
		fakeBrowser(t, func(state string) string { return "error=access_denied&state=" + url.QueryEscape(state) }))
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("expected access_denied error, got %v", err)
	}
}

func TestRunOAuthNoRefreshToken(t *testing.T) {
	useFakeTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"x"}`))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := RunOAuth(ctx, OAuthRequest{ClientID: "id", ClientSecret: "secret"},
		fakeBrowser(t, func(state string) string { return "code=abc&state=" + url.QueryEscape(state) }))
	if err == nil || !strings.Contains(err.Error(), "no refresh_token") {
		t.Fatalf("expected no-refresh-token error, got %v", err)
	}
}

func TestRunOAuthTokenEndpointError(t *testing.T) {
	useFakeTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := RunOAuth(ctx, OAuthRequest{ClientID: "id", ClientSecret: "secret"},
		fakeBrowser(t, func(state string) string { return "code=abc&state=" + url.QueryEscape(state) }))
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("expected HTTP 401 invalid_client error, got %v", err)
	}
}

func TestRunOAuthCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	_, err := RunOAuth(ctx, OAuthRequest{ClientID: "id", ClientSecret: "secret"}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancelled error, got %v", err)
	}
}
