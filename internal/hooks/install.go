package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const hookMarker = "# claude-review hook"
const hookScript = `
# claude-review hook
claude-review --format json --output /dev/null 2>&1 || {
  echo "claude-review: review failed or issues found. Run 'claude-review' to see details."
  exit 1
}
`

// Install adds claude-review as a pre-commit hook in the current git repo.
func Install() error {
	gitDir, err := findGitDir()
	if err != nil {
		return err
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")

	// Check if hook already contains our marker
	if existing, err := os.ReadFile(hookPath); err == nil {
		if strings.Contains(string(existing), hookMarker) {
			return fmt.Errorf("claude-review hook already installed at %s", hookPath)
		}
		// Append to existing hook
		f, err := os.OpenFile(hookPath, os.O_APPEND|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("opening hook file: %w", err)
		}
		defer f.Close()
		_, err = f.WriteString(hookScript)
		if err != nil {
			return fmt.Errorf("appending to hook: %w", err)
		}
		fmt.Printf("✓ Added claude-review to existing pre-commit hook at %s\n", hookPath)
		return nil
	}

	// Create new hook file
	content := "#!/bin/sh\n" + hookScript
	if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("writing hook file: %w", err)
	}
	fmt.Printf("✓ Installed pre-commit hook at %s\n", hookPath)
	return nil
}

// Remove removes the claude-review section from the pre-commit hook.
func Remove() error {
	gitDir, err := findGitDir()
	if err != nil {
		return err
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		return fmt.Errorf("no pre-commit hook found at %s", hookPath)
	}

	newContent := removeHookSection(string(content))
	if newContent == string(content) {
		return fmt.Errorf("claude-review hook not found in %s", hookPath)
	}

	if strings.TrimSpace(strings.TrimPrefix(newContent, "#!/bin/sh")) == "" {
		// Only our hook was in the file, remove entirely
		return os.Remove(hookPath)
	}
	return os.WriteFile(hookPath, []byte(newContent), 0755)
}

func removeHookSection(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	skip := false
	for _, line := range lines {
		if strings.TrimSpace(line) == hookMarker {
			skip = true
		}
		if !skip {
			result = append(result, line)
		}
		// End of our block: after the closing }
		if skip && strings.TrimSpace(line) == "}" {
			skip = false
		}
	}
	return strings.Join(result, "\n")
}

func findGitDir() (string, error) {
	// Walk up from cwd to find .git
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".git")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not inside a git repository (no .git directory found)")
		}
		dir = parent
	}
}
