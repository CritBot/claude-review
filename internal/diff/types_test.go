package diff

import "testing"

func TestSerializeDiff(t *testing.T) {
	p := &Payload{
		Files: []FileDiff{
			{RawDiff: "diff --git a/a.go b/a.go\n+line1\n"},
			{RawDiff: "diff --git a/b.go b/b.go\n+line2\n"},
		},
	}
	got := p.SerializeDiff()
	if got == "" {
		t.Error("expected non-empty serialized diff")
	}
	if len(got) < len("diff --git a/a.go b/a.go") {
		t.Error("serialized diff too short")
	}
}

func TestSerializeFileSubset(t *testing.T) {
	p := &Payload{}
	files := []FileDiff{
		{RawDiff: "diff --git a/a.go b/a.go\n+line1\n"},
	}
	got := p.SerializeFileSubset(files)
	if got == "" {
		t.Error("expected non-empty serialized subset")
	}
}

func TestSerializeDiff_Empty(t *testing.T) {
	p := &Payload{Files: []FileDiff{}}
	got := p.SerializeDiff()
	if got != "" {
		t.Errorf("expected empty string for no files, got %q", got)
	}
}
