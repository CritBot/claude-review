package diff

type Source string

const (
	SourceLocalStaged Source = "local-staged"
	SourceLocalRange  Source = "local-range"
	SourceGitHubPR    Source = "github-pr"
	SourceGitLabMR    Source = "gitlab-mr"
	SourceBitbucketPR Source = "bitbucket-pr"
)

type LineType string

const (
	LineAdded   LineType = "added"
	LineRemoved LineType = "removed"
	LineContext LineType = "context"
)

type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeModified ChangeType = "modified"
	ChangeDeleted  ChangeType = "deleted"
	ChangeRenamed  ChangeType = "renamed"
)

type Line struct {
	Type          LineType
	Content       string
	LineNumber    int // New file line number (0 for removed lines)
	OldLineNumber int // Old file line number (0 for added lines)
}

type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []Line
}

type FileDiff struct {
	FilePath    string
	OldFilePath string // Set when renamed
	ChangeType  ChangeType
	Language    string
	Hunks       []Hunk
	RawDiff     string // Original unified diff for this file
}

type Payload struct {
	Source          Source
	Files           []FileDiff
	PRTitle         string
	PRDescription   string
	BaseRef         string
	HeadRef         string
	TotalAdditions  int
	TotalDeletions  int
	PRURL           string
}

// SerializeDiff returns a compact text representation suitable for sending to the LLM.
func (p *Payload) SerializeDiff() string {
	out := ""
	for _, f := range p.Files {
		out += f.RawDiff + "\n"
	}
	return out
}

// SerializeFileSubset returns the diff text for a subset of files (for splitting across agents).
func (p *Payload) SerializeFileSubset(files []FileDiff) string {
	out := ""
	for _, f := range files {
		out += f.RawDiff + "\n"
	}
	return out
}
