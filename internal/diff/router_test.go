package diff

import "testing"

func TestBuildPRURL_GitHub(t *testing.T) {
	cases := []struct {
		remote string
		num    string
		want   string
	}{
		{"https://github.com/owner/repo.git", "123", "https://github.com/owner/repo/pull/123"},
		{"git@github.com:owner/repo.git", "42", "https://github.com/owner/repo/pull/42"},
		{"https://github.com/owner/repo", "7", "https://github.com/owner/repo/pull/7"},
	}
	for _, tc := range cases {
		got, err := buildPRURL(tc.remote, tc.num)
		if err != nil {
			t.Errorf("buildPRURL(%q, %q) error: %v", tc.remote, tc.num, err)
			continue
		}
		if got != tc.want {
			t.Errorf("buildPRURL(%q, %q) = %q, want %q", tc.remote, tc.num, got, tc.want)
		}
	}
}

func TestBuildPRURL_GitLab(t *testing.T) {
	got, err := buildPRURL("git@gitlab.com:org/project.git", "55")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://gitlab.com/org/project/-/merge_requests/55"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPRURL_Bitbucket(t *testing.T) {
	got, err := buildPRURL("https://bitbucket.org/team/repo.git", "9")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://bitbucket.org/team/repo/pull-requests/9"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveURL_FullURL(t *testing.T) {
	input := "https://github.com/owner/repo/pull/123?diff=split#top"
	got, err := ResolveURL(input)
	if err != nil {
		t.Fatal(err)
	}
	// Fragment and query should be stripped
	want := "https://github.com/owner/repo/pull/123"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveURL_InvalidInput(t *testing.T) {
	_, err := ResolveURL("not-a-url-or-number")
	if err == nil {
		t.Error("expected error for invalid input")
	}
}
