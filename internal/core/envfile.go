// Package core holds the non-GUI logic: .env handling, .gitignore safety,
// the Google OAuth loopback flow, and persisted settings.
package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnvFile is a line-preserving view of a .env file. Comments, blank lines,
// ordering and the original line-ending style survive a load/modify/save cycle.
type EnvFile struct {
	Path    string
	Existed bool
	lines   []string
	eol     string
	perm    os.FileMode
}

// LoadEnv reads path. A missing file is not an error: it yields an empty
// EnvFile with Existed == false so the caller can create it on Save.
func LoadEnv(path string) (*EnvFile, error) {
	envFile := &EnvFile{Path: path, eol: "\n", perm: 0o600}

	rawBytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return envFile, nil
		}
		return nil, fmt.Errorf("could not read env file %q: %w", path, err)
	}
	envFile.Existed = true

	if info, statErr := os.Stat(path); statErr == nil {
		envFile.perm = info.Mode().Perm()
	}

	text := strings.TrimPrefix(string(rawBytes), "\xef\xbb\xbf")
	if strings.Contains(text, "\r\n") {
		envFile.eol = "\r\n"
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text != "" {
		envFile.lines = strings.Split(text, "\n")
	}
	return envFile, nil
}

// parseEnvLine extracts KEY and VALUE from a line. ok is false for blank
// lines, comments and anything without a KEY=VALUE shape.
func parseEnvLine(line string) (key string, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	trimmed = strings.TrimPrefix(trimmed, "export ")

	separatorIndex := strings.Index(trimmed, "=")
	if separatorIndex <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(trimmed[:separatorIndex])
	value = strings.TrimSpace(trimmed[separatorIndex+1:])
	if len(value) >= 2 && value[0] == value[len(value)-1] && (value[0] == '"' || value[0] == '\'') {
		value = value[1 : len(value)-1]
	}
	return key, value, true
}

// Get returns the value for key. If the key appears more than once the last
// occurrence wins, matching common dotenv behaviour.
func (e *EnvFile) Get(key string) (string, bool) {
	var foundValue string
	var found bool
	for _, line := range e.lines {
		lineKey, lineValue, ok := parseEnvLine(line)
		if ok && lineKey == key {
			foundValue, found = lineValue, true
		}
	}
	return foundValue, found
}

// Set replaces every existing KEY= line in place, or appends the key if absent.
func (e *EnvFile) Set(key string, value string) {
	replaced := false
	for index, line := range e.lines {
		lineKey, _, ok := parseEnvLine(line)
		if ok && lineKey == key {
			e.lines[index] = key + "=" + value
			replaced = true
		}
	}
	if !replaced {
		e.lines = append(e.lines, key+"="+value)
	}
}

// Save writes the file atomically (temp file in the same directory, then rename).
func (e *EnvFile) Save() error {
	content := ""
	if len(e.lines) > 0 {
		content = strings.Join(e.lines, e.eol) + e.eol
	}

	directory := filepath.Dir(e.Path)
	tempFile, err := os.CreateTemp(directory, ".env-write-*")
	if err != nil {
		return fmt.Errorf("could not create a temporary file in %q: %w", directory, err)
	}
	tempPath := tempFile.Name()

	if _, err := tempFile.WriteString(content); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("could not write temporary env file %q: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("could not close temporary env file %q: %w", tempPath, err)
	}
	if err := os.Chmod(tempPath, e.perm); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("could not set permissions on temporary env file %q: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, e.Path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("could not replace %q with the updated file: %w", e.Path, err)
	}
	return nil
}

// BackupPathFor returns the backup location for an env file: the same
// directory, with ".bak" appended (".env" becomes ".env.bak").
func BackupPathFor(envPath string) string {
	return filepath.Join(filepath.Dir(envPath), filepath.Base(envPath)+".bak")
}

// CopyFile copies source to destination with owner-only permissions,
// overwriting any existing destination.
func CopyFile(sourcePath string, destinationPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("could not read %q to copy it: %w", sourcePath, err)
	}
	if err := os.WriteFile(destinationPath, data, 0o600); err != nil {
		return fmt.Errorf("could not write backup %q: %w", destinationPath, err)
	}
	return nil
}
