package core

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDirectoryName = "chrome-refresh-token-tool"
	configFileName      = "settings.ini"
	configSectionName   = "settings"
	lastEnvFileKey      = "last_env_file"
)

// ConfigFilePath returns where the INI settings file lives, e.g.
// %AppData%\chrome-refresh-token-tool\settings.ini on Windows.
func ConfigFilePath() (string, error) {
	baseDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine the user config directory: %w", err)
	}
	return filepath.Join(baseDirectory, configDirectoryName, configFileName), nil
}

// ReadLastEnvPath returns the saved .env path, or "" if none has been saved.
func ReadLastEnvPath(configPath string) (string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("could not open settings file %q: %w", configPath, err)
	}
	defer file.Close()

	currentSection := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && currentSection == configSectionName && strings.TrimSpace(key) == lastEnvFileKey {
			return strings.TrimSpace(value), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("could not read settings file %q: %w", configPath, err)
	}
	return "", nil
}

// WriteLastEnvPath saves envPath as the last used .env file.
func WriteLastEnvPath(configPath string, envPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return fmt.Errorf("could not create settings directory for %q: %w", configPath, err)
	}
	content := fmt.Sprintf("[%s]\n%s=%s\n", configSectionName, lastEnvFileKey, envPath)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("could not write settings file %q: %w", configPath, err)
	}
	return nil
}
