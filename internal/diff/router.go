package diff

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var numericRe = regexp.MustCompile(`^\d+$`)

// ResolveURL takes either a full PR/MR URL or a bare PR number.
// For bare numbers, it reads the git remote URL from the current repo
// and constructs the full PR URL automatically.
func ResolveURL(input string) (string, error) {
	// Normalize: strip trailing slashes and fragment
	input = strings.TrimRight(input, "/")
	if u, err := url.Parse(input); err == nil {
		u.Fragment = ""
		u.RawQuery = ""
		input = u.String()
	}

	// If it already looks like a full URL, return as-is
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input, nil
	}

	// Bare number: detect remote and build URL
	if numericRe.MatchString(input) {
		remote, err := runGit("remote", "get-url", "origin")
		if err != nil {
			return "", fmt.Errorf("could not detect git remote: %w\nProvide the full PR URL instead", err)
		}
		return buildPRURL(strings.TrimSpace(remote), input)
	}

	return "", fmt.Errorf("unrecognized input %q — provide a full URL (https://github.com/...) or a PR number", input)
}

// buildPRURL converts a git remote URL + PR number into a browser-accessible PR URL.
func buildPRURL(remote, prNum string) (string, error) {
	// Normalize SSH remotes to HTTPS
	// git@github.com:owner/repo.git  →  https://github.com/owner/repo
	// https://github.com/owner/repo.git  →  https://github.com/owner/repo
	remote = strings.TrimSuffix(remote, ".git")

	if strings.HasPrefix(remote, "git@") {
		// git@github.com:owner/repo → https://github.com/owner/repo
		remote = strings.TrimPrefix(remote, "git@")
		remote = strings.Replace(remote, ":", "/", 1)
		remote = "https://" + remote
	}

	switch {
	case strings.Contains(remote, "github.com"):
		return remote + "/pull/" + prNum, nil
	case strings.Contains(remote, "gitlab"):
		return remote + "/-/merge_requests/" + prNum, nil
	case strings.Contains(remote, "bitbucket.org"):
		return remote + "/pull-requests/" + prNum, nil
	default:
		// Assume GitLab for self-hosted instances
		return remote + "/-/merge_requests/" + prNum, nil
	}
}
