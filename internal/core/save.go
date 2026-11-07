package core

import (
	"errors"
	"fmt"
	"strings"
)

// Environment variable names used in the .env file.
const (
	EnvKeyExtensionID  = "CHROME_EXTENSION_ID"
	EnvKeyClientID     = "CHROME_CLIENT_ID"
	EnvKeyClientSecret = "CHROME_CLIENT_SECRET"
	EnvKeyRefreshToken = "CHROME_REFRESH_TOKEN"
)

// EnvValues are the four values written to the .env file.
type EnvValues struct {
	ExtensionID  string
	ClientID     string
	ClientSecret string
	RefreshToken string
}

// SaveReport describes what SaveTokenToEnv did.
type SaveReport struct {
	EnvPath        string
	BackupPath     string
	GitIgnore      GitIgnoreResult
	CreatedNewFile bool
}

// Summary returns a multi-line, human-readable description of the save.
func (r SaveReport) Summary() string {
	lines := []string{fmt.Sprintf("Saved %s to %s and verified it.", EnvKeyRefreshToken, r.EnvPath)}
	if r.CreatedNewFile {
		lines = append(lines, "The file did not exist, so it was created (no backup needed).")
	}
	if r.BackupPath != "" {
		lines = append(lines, "Backup: "+r.BackupPath)
		lines = append(lines, r.GitIgnore.Message())
	}
	return strings.Join(lines, "\n")
}

// SaveTokenToEnv writes the values to envPath. If the file already exists it
// first makes sure the backup name is git-ignored (when inside a repository
// with a .gitignore), then copies the file to <name>.bak, then updates the
// four keys in place and re-reads the file to verify what was written.
func SaveTokenToEnv(envPath string, values EnvValues) (SaveReport, error) {
	report := SaveReport{EnvPath: envPath}

	if values.RefreshToken == "" {
		return report, errors.New("refusing to write: the refresh token value is empty")
	}

	envFile, err := LoadEnv(envPath)
	if err != nil {
		return report, err
	}

	if envFile.Existed {
		backupPath := BackupPathFor(envPath)

		gitIgnoreResult, err := EnsureGitIgnored(backupPath)
		if err != nil {
			return report, fmt.Errorf(
				"stopped before creating the backup so secrets are not left unprotected: %w", err)
		}
		report.GitIgnore = gitIgnoreResult

		if err := CopyFile(envPath, backupPath); err != nil {
			return report, err
		}
		report.BackupPath = backupPath
	} else {
		report.CreatedNewFile = true
	}

	expectedValues := map[string]string{
		EnvKeyExtensionID:  values.ExtensionID,
		EnvKeyClientID:     values.ClientID,
		EnvKeyClientSecret: values.ClientSecret,
		EnvKeyRefreshToken: values.RefreshToken,
	}
	for _, key := range []string{EnvKeyExtensionID, EnvKeyClientID, EnvKeyClientSecret, EnvKeyRefreshToken} {
		envFile.Set(key, expectedValues[key])
	}
	if err := envFile.Save(); err != nil {
		return report, err
	}

	verifiedFile, err := LoadEnv(envPath)
	if err != nil {
		return report, fmt.Errorf("the file was written but could not be re-read for verification: %w", err)
	}
	for key, expected := range expectedValues {
		actual, found := verifiedFile.Get(key)
		if !found || actual != expected {
			return report, fmt.Errorf(
				"verification failed: %s in %q does not match the value that was written", key, envPath)
		}
	}
	return report, nil
}
