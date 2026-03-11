package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var bitbucketPRRe = regexp.MustCompile(`bitbucket\.org/([^/]+)/([^/]+)/pull-requests/(\d+)`)

type bitbucketPR struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Source      struct {
		Branch struct{ Name string `json:"name"` } `json:"branch"`
	} `json:"source"`
	Destination struct {
		Branch struct{ Name string `json:"name"` } `json:"branch"`
	} `json:"destination"`
}

type bitbucketDiffstat struct {
	Values []struct {
		Status string `json:"status"` // added, removed, modified, renamed
		Old    *struct {
			Path string `json:"path"`
		} `json:"old"`
		New *struct {
			Path string `json:"path"`
		} `json:"new"`
	} `json:"values"`
	Next string `json:"next"` // pagination cursor URL
}

// FetchBitbucketPR fetches the diff for a Bitbucket pull request URL.
// token should be a Bitbucket app password: "username:app_password"
func FetchBitbucketPR(rawURL, token string) (*Payload, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	m := bitbucketPRRe.FindStringSubmatch(u.Host + u.Path)
	if len(m) != 4 {
		return nil, fmt.Errorf("could not parse Bitbucket PR URL: %s", rawURL)
	}
	workspace, repoSlug, prID := m[1], m[2], m[3]

	client := &http.Client{}
	apiBase := "https://api.bitbucket.org/2.0"

	// Fetch PR metadata
	pr, err := fetchBitbucketJSON[bitbucketPR](client, token,
		fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%s", apiBase, workspace, repoSlug, prID))
	if err != nil {
		return nil, fmt.Errorf("fetching PR metadata: %w", err)
	}

	// Fetch unified diff
	diffURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%s/diff", apiBase, workspace, repoSlug, prID)
	rawDiff, err := fetchBitbucketRaw(client, token, diffURL)
	if err != nil {
		return nil, fmt.Errorf("fetching PR diff: %w", err)
	}

	payload, err := Parse(rawDiff, SourceBitbucketPR)
	if err != nil {
		return nil, err
	}
	payload.PRTitle = pr.Title
	payload.PRDescription = pr.Description
	payload.HeadRef = pr.Source.Branch.Name
	payload.BaseRef = pr.Destination.Branch.Name
	payload.PRURL = rawURL
	return payload, nil
}

func fetchBitbucketJSON[T any](client *http.Client, token, apiURL string) (T, error) {
	var zero T
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "claude-review/1.0")
	if token != "" {
		parts := strings.SplitN(token, ":", 2)
		if len(parts) == 2 {
			req.SetBasicAuth(parts[0], parts[1])
		}
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
		return zero, fmt.Errorf("Bitbucket API authentication failed (HTTP %d). Set BITBUCKET_TOKEN as 'username:app_password'", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return zero, fmt.Errorf("Bitbucket API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, fmt.Errorf("parsing Bitbucket API response: %w", err)
	}
	return result, nil
}

func fetchBitbucketRaw(client *http.Client, token, apiURL string) (string, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "claude-review/1.0")
	if token != "" {
		parts := strings.SplitN(token, ":", 2)
		if len(parts) == 2 {
			req.SetBasicAuth(parts[0], parts[1])
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Bitbucket diff API HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

// IsBitbucketURL returns true if the URL looks like a Bitbucket PR.
func IsBitbucketURL(rawURL string) bool {
	return strings.Contains(rawURL, "bitbucket.org") && strings.Contains(rawURL, "/pull-requests/")
}
