// Package workdir resolves the td database root directory, supporting git
// worktree redirection via .td-root files.
package workdir

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	tdRootFile = ".td-root"
	todosDir   = ".todos"
)

// ResolveBaseDir resolves td's project root with conservative heuristics:
//  1. Honor .td-root in the current directory.
//  2. Use current directory if it already has a .todos directory.
//  3. If inside git, check git root for .td-root or .todos.
//
// If no td markers are found, it returns the original baseDir unchanged.
func ResolveBaseDir(baseDir string) string {
	if baseDir == "" {
		return baseDir
	}
	baseDir = filepath.Clean(baseDir)

	if resolved, ok := readTdRoot(baseDir); ok {
		return resolved
	}
	if hasTodosDir(baseDir) {
		return baseDir
	}

	gitRoot, err := gitTopLevel(baseDir)
	if err != nil || gitRoot == "" {
		return baseDir
	}
	gitRoot = filepath.Clean(gitRoot)

	if resolved, ok := readTdRoot(gitRoot); ok {
		return resolved
	}
	if hasTodosDir(gitRoot) {
		return gitRoot
	}

	return baseDir
}

func readTdRoot(dir string) (string, bool) {
	tdRootPath := filepath.Join(dir, tdRootFile)
	content, err := os.ReadFile(tdRootPath)
	if err != nil {
		return "", false
	}

	resolved := strings.TrimSpace(string(content))
	if resolved == "" {
		return "", false
	}
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(dir, resolved)
	}

	return filepath.Clean(resolved), true
}

func hasTodosDir(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, todosDir))
	return err == nil && fi.IsDir()
}

func gitTopLevel(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
