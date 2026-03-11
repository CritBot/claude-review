package diff

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetStaged returns the diff of staged changes (git diff --cached).
func GetStaged() (*Payload, error) {
	raw, err := runGit("diff", "--cached", "--unified=3")
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("no staged changes found\n\nStage changes with 'git add' first, or use 'claude-review diff HEAD~1' to review the last commit")
	}
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		return nil, err
	}
	p.HeadRef, _ = runGit("rev-parse", "--abbrev-ref", "HEAD")
	p.HeadRef = strings.TrimSpace(p.HeadRef)
	return p, nil
}

// GetRange returns the diff for a git range (e.g. "HEAD~1", "main..feature").
func GetRange(rangeRef string) (*Payload, error) {
	raw, err := runGit("diff", "--unified=3", rangeRef)
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %w", rangeRef, err)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("no changes found for range: %s", rangeRef)
	}
	p, err := Parse(raw, SourceLocalRange)
	if err != nil {
		return nil, err
	}
	p.HeadRef = rangeRef
	p.BaseRef, _ = inferBaseRef(rangeRef)
	return p, nil
}

// GetFiles returns the diff for specific files only.
func GetFiles(files []string) (*Payload, error) {
	args := append([]string{"diff", "--cached", "--unified=3", "--"}, files...)
	raw, err := runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("git diff for files: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		// Try unstaged
		args[1] = "diff"
		args = append(args[:2], append([]string{"--unified=3", "--"}, files...)...)
		raw, err = runGit(args...)
		if err != nil || strings.TrimSpace(raw) == "" {
			return nil, fmt.Errorf("no changes found for specified files")
		}
	}
	return Parse(raw, SourceLocalStaged)
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s", string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

func inferBaseRef(rangeRef string) (string, string) {
	if idx := strings.Index(rangeRef, ".."); idx >= 0 {
		return rangeRef[:idx], rangeRef[idx+2:]
	}
	return "", rangeRef
}
