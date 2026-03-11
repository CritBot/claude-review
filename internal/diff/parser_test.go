package diff

import (
	"testing"
)

func TestParse_ModifiedFile(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

 import "fmt"
+import "os"

 func main() {
`
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(p.Files))
	}
	f := p.Files[0]
	if f.FilePath != "main.go" {
		t.Errorf("expected FilePath=main.go, got %q", f.FilePath)
	}
	if f.ChangeType != ChangeModified {
		t.Errorf("expected ChangeModified, got %q", f.ChangeType)
	}
	if p.TotalAdditions != 1 {
		t.Errorf("expected 1 addition, got %d", p.TotalAdditions)
	}
}

func TestParse_AddedFile(t *testing.T) {
	raw := `diff --git a/new.go b/new.go
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func hello() {}
`
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		t.Fatal(err)
	}
	f := p.Files[0]
	if f.ChangeType != ChangeAdded {
		t.Errorf("expected ChangeAdded, got %q", f.ChangeType)
	}
	if p.TotalAdditions != 3 {
		t.Errorf("expected 3 additions, got %d", p.TotalAdditions)
	}
}

func TestParse_DeletedFile(t *testing.T) {
	raw := `diff --git a/old.go b/old.go
--- a/old.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package main
-func bye() {}
`
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		t.Fatal(err)
	}
	f := p.Files[0]
	if f.ChangeType != ChangeDeleted {
		t.Errorf("expected ChangeDeleted, got %q", f.ChangeType)
	}
	if p.TotalDeletions != 2 {
		t.Errorf("expected 2 deletions, got %d", p.TotalDeletions)
	}
}

func TestParse_MultipleFiles(t *testing.T) {
	raw := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,1 +1,2 @@
 package a
+// added
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,2 +1,1 @@
 package b
-// removed
`
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(p.Files))
	}
	if p.TotalAdditions != 1 {
		t.Errorf("expected 1 addition, got %d", p.TotalAdditions)
	}
	if p.TotalDeletions != 1 {
		t.Errorf("expected 1 deletion, got %d", p.TotalDeletions)
	}
}

func TestParse_BinaryFile(t *testing.T) {
	raw := `diff --git a/image.png b/image.png
Binary files a/image.png and b/image.png differ
`
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		t.Fatal(err)
	}
	// Binary files should be skipped
	if len(p.Files) != 0 {
		t.Errorf("expected binary file to be skipped, got %d files", len(p.Files))
	}
}

func TestParse_EmptyDiff(t *testing.T) {
	_, err := Parse("", SourceLocalStaged)
	if err == nil {
		t.Error("expected error for empty diff")
	}
}

func TestParse_RenamedFile(t *testing.T) {
	raw := `diff --git a/old.go b/new.go
rename from old.go
rename to new.go
--- a/old.go
+++ b/new.go
@@ -1,1 +1,1 @@
 package main
`
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(p.Files))
	}
	if p.Files[0].ChangeType != ChangeRenamed {
		t.Errorf("expected ChangeRenamed, got %q", p.Files[0].ChangeType)
	}
}

func TestParse_LineNumbers(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -10,3 +10,4 @@
 line10
+added11
 line11
 line12
`
	p, err := Parse(raw, SourceLocalStaged)
	if err != nil {
		t.Fatal(err)
	}
	hunk := p.Files[0].Hunks[0]
	// Find the added line and check its line number
	for _, l := range hunk.Lines {
		if l.Type == LineAdded {
			if l.LineNumber != 11 {
				t.Errorf("expected added line at line 11, got %d", l.LineNumber)
			}
		}
	}
}

func TestParse_LanguageDetection(t *testing.T) {
	cases := []struct {
		file string
		lang string
	}{
		{"src/auth.go", "go"},
		{"src/app.ts", "typescript"},
		{"lib/helper.py", "python"},
		{"unknown.xyz", ""},
	}
	for _, tc := range cases {
		raw := "diff --git a/" + tc.file + " b/" + tc.file + "\n--- a/" + tc.file + "\n+++ b/" + tc.file + "\n@@ -1 +1,2 @@\n line\n+added\n"
		p, _ := Parse(raw, SourceLocalStaged)
		if len(p.Files) > 0 && p.Files[0].Language != tc.lang {
			t.Errorf("file %s: expected lang=%q, got %q", tc.file, tc.lang, p.Files[0].Language)
		}
	}
}

func TestSplitForAgents(t *testing.T) {
	files := []FileDiff{{FilePath: "a"}, {FilePath: "b"}, {FilePath: "c"}, {FilePath: "d"}, {FilePath: "e"}}
	splits := SplitForAgents(files, 3)
	if len(splits) != 3 {
		t.Fatalf("expected 3 splits, got %d", len(splits))
	}
	total := 0
	for _, s := range splits {
		total += len(s)
	}
	if total != 5 {
		t.Errorf("expected 5 total files across splits, got %d", total)
	}
}
