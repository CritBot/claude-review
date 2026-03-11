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

var gitlabMRRe = regexp.MustCompile(`([^/]+)/([^/]+)/-/merge_requests/(\d+)`)

type gitlabMR struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
}

type gitlabMRDiff struct {
	Diffs []struct {
		OldPath     string `json:"old_path"`
		NewPath     string `json:"new_path"`
		Diff        string `json:"diff"`
		NewFile     bool   `json:"new_file"`
		RenamedFile bool   `json:"renamed_file"`
		DeletedFile bool   `json:"deleted_file"`
	} `json:"diffs"`
}

// FetchGitLabMR fetches the diff for a GitLab merge request URL.
func FetchGitLabMR(rawURL, token, gitlabHost string) (*Payload, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	m := gitlabMRRe.FindStringSubmatch(u.Path)
	if len(m) != 4 {
		return nil, fmt.Errorf("could not parse GitLab MR URL: %s", rawURL)
	}
	namespace, project, mrNumStr := m[1], m[2], m[3]
	mrNum, _ := strconv.Atoi(mrNumStr)

	// URL-encode the project path
	projectID := url.PathEscape(namespace + "/" + project)
	baseURL := gitlabHost
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	client := &http.Client{}
	apiBase := fmt.Sprintf("%s/api/v4", baseURL)

	// Fetch MR metadata
	mr, err := fetchGitLabJSON[gitlabMR](client, token,
		fmt.Sprintf("%s/projects/%s/merge_requests/%d", apiBase, projectID, mrNum))
	if err != nil {
		return nil, fmt.Errorf("fetching MR metadata: %w", err)
	}

	// Fetch MR changes
	changes, err := fetchGitLabJSON[gitlabMRDiff](client, token,
		fmt.Sprintf("%s/projects/%s/merge_requests/%d/changes", apiBase, projectID, mrNum))
	if err != nil {
		return nil, fmt.Errorf("fetching MR changes: %w", err)
	}

	fileDiffs := make([]FileDiff, 0, len(changes.Diffs))
	totalAdd, totalDel := 0, 0

	for _, d := range changes.Diffs {
		if d.Diff == "" {
			continue
		}
		rawDiff := fmt.Sprintf("diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n%s",
			d.OldPath, d.NewPath, d.OldPath, d.NewPath, d.Diff)

		fd, err := parseFileDiff(rawDiff)
		if err != nil {
			continue
		}
		fd.FilePath = d.NewPath

		switch {
		case d.NewFile:
			fd.ChangeType = ChangeAdded
		case d.DeletedFile:
			fd.ChangeType = ChangeDeleted
		case d.RenamedFile:
			fd.ChangeType = ChangeRenamed
			fd.OldFilePath = d.OldPath
		default:
			fd.ChangeType = ChangeModified
		}

		for _, h := range fd.Hunks {
			for _, l := range h.Lines {
				switch l.Type {
				case LineAdded:
					totalAdd++
				case LineRemoved:
					totalDel++
				}
			}
		}
		fileDiffs = append(fileDiffs, fd)
	}

	return &Payload{
		Source:         SourceGitLabMR,
		Files:          fileDiffs,
		PRTitle:        mr.Title,
		PRDescription:  mr.Description,
		BaseRef:        mr.TargetBranch,
		HeadRef:        mr.SourceBranch,
		TotalAdditions: totalAdd,
		TotalDeletions: totalDel,
		PRURL:          rawURL,
	}, nil
}

func fetchGitLabJSON[T any](client *http.Client, token, apiURL string) (T, error) {
	var zero T
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("User-Agent", "claude-review/1.0")
	if token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
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
		return zero, fmt.Errorf("GitLab API authentication failed (HTTP %d). Set GITLAB_TOKEN env var", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return zero, fmt.Errorf("GitLab API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, fmt.Errorf("parsing GitLab API response: %w", err)
	}
	return result, nil
}

// IsGitLabURL returns true if the URL looks like a GitLab MR.
func IsGitLabURL(rawURL string) bool {
	return strings.Contains(rawURL, "/-/merge_requests/")
}
