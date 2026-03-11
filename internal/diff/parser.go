package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+),?(\d+)? \+(\d+),?(\d+)? @@`)
	diffGitRe    = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)
)

var languageByExt = map[string]string{
	".go": "go", ".ts": "typescript", ".tsx": "typescript",
	".js": "javascript", ".jsx": "javascript", ".py": "python",
	".rb": "ruby", ".rs": "rust", ".java": "java", ".kt": "kotlin",
	".cs": "csharp", ".cpp": "cpp", ".c": "c", ".h": "c",
	".php": "php", ".swift": "swift", ".sh": "bash", ".yml": "yaml",
	".yaml": "yaml", ".json": "json", ".md": "markdown", ".sql": "sql",
}

// Parse converts a raw unified diff string into a Payload.
func Parse(rawDiff string, source Source) (*Payload, error) {
	if strings.TrimSpace(rawDiff) == "" {
		return nil, fmt.Errorf("no changes to review")
	}

	files, err := parseFiles(rawDiff)
	if err != nil {
		return nil, err
	}

	totalAdd, totalDel := 0, 0
	for _, f := range files {
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				switch l.Type {
				case LineAdded:
					totalAdd++
				case LineRemoved:
					totalDel++
				}
			}
		}
	}

	return &Payload{
		Source:         source,
		Files:          files,
		TotalAdditions: totalAdd,
		TotalDeletions: totalDel,
	}, nil
}

func parseFiles(rawDiff string) ([]FileDiff, error) {
	sections := splitOnDiffGit(rawDiff)
	files := make([]FileDiff, 0, len(sections))

	for _, section := range sections {
		if strings.TrimSpace(section) == "" {
			continue
		}
		fd, err := parseFileDiff(section)
		if err != nil {
			continue // skip unparseable sections (binary files, etc.)
		}
		files = append(files, fd)
	}
	return files, nil
}

func splitOnDiffGit(raw string) []string {
	var sections []string
	var current strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "diff --git ") && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}
	return sections
}

func parseFileDiff(section string) (FileDiff, error) {
	lines := strings.Split(section, "\n")
	if len(lines) == 0 {
		return FileDiff{}, fmt.Errorf("empty section")
	}

	fd := FileDiff{RawDiff: section}

	// Parse diff --git header
	if m := diffGitRe.FindStringSubmatch(lines[0]); len(m) == 3 {
		fd.FilePath = m[2]
		fd.OldFilePath = m[1]
		if fd.OldFilePath == fd.FilePath {
			fd.OldFilePath = ""
		}
	}

	// Detect language from extension
	if idx := strings.LastIndex(fd.FilePath, "."); idx >= 0 {
		ext := fd.FilePath[idx:]
		fd.Language = languageByExt[ext]
	}

	// Parse change type and file paths from --- / +++ lines
	fd.ChangeType = ChangeModified
	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "--- /dev/null"):
			fd.ChangeType = ChangeAdded
		case strings.HasPrefix(line, "+++ /dev/null"):
			fd.ChangeType = ChangeDeleted
		case strings.HasPrefix(line, "rename from "):
			fd.ChangeType = ChangeRenamed
		case strings.HasPrefix(line, "Binary files "):
			return FileDiff{}, fmt.Errorf("binary file, skipping")
		}
	}

	// Parse hunks
	hunks, err := parseHunks(lines)
	if err != nil {
		return FileDiff{}, err
	}
	fd.Hunks = hunks

	return fd, nil
}

func parseHunks(lines []string) ([]Hunk, error) {
	var hunks []Hunk
	var current *Hunk
	newLine, oldLine := 0, 0

	for _, line := range lines {
		if m := hunkHeaderRe.FindStringSubmatch(line); len(m) >= 4 {
			if current != nil {
				hunks = append(hunks, *current)
			}
			oldStart, _ := strconv.Atoi(m[1])
			newStart, _ := strconv.Atoi(m[3])
			oldLen := 1
			newLen := 1
			if len(m) >= 5 && m[2] != "" {
				oldLen, _ = strconv.Atoi(m[2])
			}
			if len(m) >= 5 && m[4] != "" {
				newLen, _ = strconv.Atoi(m[4])
			}
			current = &Hunk{
				OldStart: oldStart,
				OldLines: oldLen,
				NewStart: newStart,
				NewLines: newLen,
			}
			newLine = newStart
			oldLine = oldStart
			continue
		}

		if current == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			current.Lines = append(current.Lines, Line{
				Type:       LineAdded,
				Content:    line[1:],
				LineNumber: newLine,
			})
			newLine++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			current.Lines = append(current.Lines, Line{
				Type:          LineRemoved,
				Content:       line[1:],
				OldLineNumber: oldLine,
			})
			oldLine++
		default:
			current.Lines = append(current.Lines, Line{
				Type:          LineContext,
				Content:       line,
				LineNumber:    newLine,
				OldLineNumber: oldLine,
			})
			newLine++
			oldLine++
		}
	}

	if current != nil {
		hunks = append(hunks, *current)
	}
	return hunks, nil
}

// SplitForAgents divides files evenly across n agents, returning n slices of FileDiff.
func SplitForAgents(files []FileDiff, n int) [][]FileDiff {
	if n <= 0 {
		n = 1
	}
	result := make([][]FileDiff, n)
	for i, f := range files {
		result[i%n] = append(result[i%n], f)
	}
	return result
}
