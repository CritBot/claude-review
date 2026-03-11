package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var githubPRRe = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

type githubPRFile struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"` // added, removed, modified, renamed
	Patch       string `json:"patch"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	PreviousFilename string `json:"previous_filename"`
}

type githubPR struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// FetchGitHubPR fetches the diff for a GitHub pull request URL.
func FetchGitHubPR(rawURL, token string) (*Payload, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	m := githubPRRe.FindStringSubmatch(u.Host + u.Path)
	if len(m) != 4 {
		return nil, fmt.Errorf("could not parse GitHub PR URL: %s", rawURL)
	}
	owner, repo, prNumStr := m[1], m[2], m[3]
	prNum, _ := strconv.Atoi(prNumStr)

	client := &http.Client{}

	// Fetch PR metadata
	pr, err := fetchGitHubJSON[githubPR](client, token,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, prNum))
	if err != nil {
		return nil, fmt.Errorf("fetching PR metadata: %w", err)
	}

	// Fetch PR files (paginated)
	var allFiles []githubPRFile
	page := 1
	for {
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/files?per_page=100&page=%d",
			owner, repo, prNum, page)
		files, err := fetchGitHubJSON[[]githubPRFile](client, token, apiURL)
		if err != nil {
			return nil, fmt.Errorf("fetching PR files (page %d): %w", page, err)
		}
		allFiles = append(allFiles, files...)
		if len(files) < 100 {
			break
		}
		page++
	}

	// Convert to FileDiff objects
	fileDiffs := make([]FileDiff, 0, len(allFiles))
	totalAdd, totalDel := 0, 0

	for _, f := range allFiles {
		if f.Patch == "" {
			continue // binary files
		}
		rawDiff := fmt.Sprintf("diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n%s",
			f.Filename, f.Filename, f.Filename, f.Filename, f.Patch)

		fd, err := parseFileDiff(rawDiff)
		if err != nil {
			continue
		}
		fd.FilePath = f.Filename
		if f.PreviousFilename != "" {
			fd.OldFilePath = f.PreviousFilename
		}
		switch f.Status {
		case "added":
			fd.ChangeType = ChangeAdded
		case "removed":
			fd.ChangeType = ChangeDeleted
		case "renamed":
			fd.ChangeType = ChangeRenamed
		default:
			fd.ChangeType = ChangeModified
		}
		fileDiffs = append(fileDiffs, fd)
		totalAdd += f.Additions
		totalDel += f.Deletions
	}

	return &Payload{
		Source:         SourceGitHubPR,
		Files:          fileDiffs,
		PRTitle:        pr.Title,
		PRDescription:  pr.Body,
		BaseRef:        pr.Base.Ref,
		HeadRef:        pr.Head.Ref,
		TotalAdditions: totalAdd,
		TotalDeletions: totalDel,
		PRURL:          rawURL,
	}, nil
}

func fetchGitHubJSON[T any](client *http.Client, token, apiURL string) (T, error) {
	var zero T
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "claude-review/1.0")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return zero, fmt.Errorf("GitHub API authentication failed (HTTP %d). Set GITHUB_TOKEN env var for private repos or to avoid rate limits", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return zero, fmt.Errorf("GitHub API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, fmt.Errorf("parsing GitHub API response: %w", err)
	}
	return result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// IsGitHubURL returns true if the URL looks like a GitHub PR.
func IsGitHubURL(rawURL string) bool {
	return strings.Contains(rawURL, "github.com") && strings.Contains(rawURL, "/pull/")
}
