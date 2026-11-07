package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// GitIgnoreOutcome describes what EnsureGitIgnored did.
type GitIgnoreOutcome int

const (
	// GitIgnoreNotRepo means no .git entry was found, so nothing was touched.
	GitIgnoreNotRepo GitIgnoreOutcome = iota
	// GitIgnoreNoFile means a repo exists but has no .gitignore to update.
	GitIgnoreNoFile
	// GitIgnoreAlreadyCovered means an existing pattern already ignores the file.
	GitIgnoreAlreadyCovered
	// GitIgnoreAppended means the entry was added to the end of .gitignore.
	GitIgnoreAppended
)

// GitIgnoreResult is the outcome plus the details needed to report it.
type GitIgnoreResult struct {
	Outcome       GitIgnoreOutcome
	GitIgnorePath string
	Entry         string
}

// Message returns a one-line, human-readable description of the result.
func (r GitIgnoreResult) Message() string {
	switch r.Outcome {
	case GitIgnoreNotRepo:
		return "No .git folder found; .gitignore left untouched."
	case GitIgnoreNoFile:
		return fmt.Sprintf("Git repository detected but no .gitignore exists, so %s is NOT ignored by git.", r.Entry)
	case GitIgnoreAlreadyCovered:
		return fmt.Sprintf("%s is already ignored by %s.", r.Entry, r.GitIgnorePath)
	case GitIgnoreAppended:
		return fmt.Sprintf("Added %s to %s.", r.Entry, r.GitIgnorePath)
	default:
		return ""
	}
}

// EnsureGitIgnored makes sure the file at targetPath is ignored by git. It
// looks for a .git entry in the file's directory and then in parent
// directories. If the repository has a .gitignore (preferring the one in the
// file's own directory, then the repository root) and no pattern already
// covers the file's name, the name is appended to the end of that .gitignore.
func EnsureGitIgnored(targetPath string) (GitIgnoreResult, error) {
	entry := filepath.Base(targetPath)
	result := GitIgnoreResult{Entry: entry}

	absoluteTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return result, fmt.Errorf("could not resolve the absolute path of %q: %w", targetPath, err)
	}
	targetDirectory := filepath.Dir(absoluteTarget)

	repoRoot, isRepo := findGitRoot(targetDirectory)
	if !isRepo {
		result.Outcome = GitIgnoreNotRepo
		return result, nil
	}

	gitIgnorePath := ""
	for _, candidateDirectory := range []string{targetDirectory, repoRoot} {
		candidatePath := filepath.Join(candidateDirectory, ".gitignore")
		if info, statErr := os.Stat(candidatePath); statErr == nil && !info.IsDir() {
			gitIgnorePath = candidatePath
			break
		}
	}
	if gitIgnorePath == "" {
		result.Outcome = GitIgnoreNoFile
		return result, nil
	}
	result.GitIgnorePath = gitIgnorePath

	existingBytes, err := os.ReadFile(gitIgnorePath)
	if err != nil {
		return result, fmt.Errorf("could not read %q: %w", gitIgnorePath, err)
	}
	existingText := string(existingBytes)

	if isCoveredByGitIgnore(existingText, entry) {
		result.Outcome = GitIgnoreAlreadyCovered
		return result, nil
	}

	lineEnding := "\n"
	if strings.Contains(existingText, "\r\n") {
		lineEnding = "\r\n"
	}
	prefix := ""
	if existingText != "" && !strings.HasSuffix(existingText, "\n") {
		prefix = lineEnding
	}

	gitIgnoreFile, err := os.OpenFile(gitIgnorePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return result, fmt.Errorf("could not open %q for appending: %w", gitIgnorePath, err)
	}
	if _, err := gitIgnoreFile.WriteString(prefix + entry + lineEnding); err != nil {
		gitIgnoreFile.Close()
		return result, fmt.Errorf("could not append %q to %q: %w", entry, gitIgnorePath, err)
	}
	if err := gitIgnoreFile.Close(); err != nil {
		return result, fmt.Errorf("could not finish writing %q: %w", gitIgnorePath, err)
	}

	result.Outcome = GitIgnoreAppended
	return result, nil
}

// findGitRoot walks upward from directory looking for a .git directory (or a
// .git file, as used by worktrees and submodules).
func findGitRoot(directory string) (string, bool) {
	current := directory
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current, true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

// isCoveredByGitIgnore reports whether the .gitignore text already ignores a
// file with the given base name. Later lines override earlier ones, so a
// trailing negation ("!name") leaves the file uncovered. Patterns containing a
// slash are directory-specific and are skipped as an approximation.
func isCoveredByGitIgnore(gitIgnoreText string, fileName string) bool {
	covered := false
	for _, rawLine := range strings.Split(gitIgnoreText, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negated := strings.HasPrefix(line, "!")
		pattern := strings.TrimPrefix(strings.TrimPrefix(line, "!"), "/")
		if strings.Contains(pattern, "/") {
			continue
		}
		if matched, err := path.Match(pattern, fileName); err == nil && matched {
			covered = !negated
		}
	}
	return covered
}
