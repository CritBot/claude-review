package agents

import (
	"testing"
)

func TestParseJSONResponse_DirectArray(t *testing.T) {
	raw := `[{"file":"main.go","line":10,"severity":"high","description":"bug"}]`
	var findings []rawFinding
	if err := parseJSONResponse(raw, &findings); err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].File != "main.go" {
		t.Errorf("expected file=main.go, got %q", findings[0].File)
	}
}

func TestParseJSONResponse_MarkdownFenced(t *testing.T) {
	raw := "Here are the findings:\n```json\n[{\"file\":\"auth.go\",\"line\":5,\"severity\":\"critical\",\"description\":\"sql injection\"}]\n```"
	var findings []rawFinding
	if err := parseJSONResponse(raw, &findings); err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].File != "auth.go" {
		t.Errorf("unexpected findings: %+v", findings)
	}
}

func TestParseJSONResponse_PreambleText(t *testing.T) {
	raw := `I found the following issues in the code:

[{"file":"db.go","line":42,"severity":"medium","description":"n+1 query"}]

Hope that helps!`
	var findings []rawFinding
	if err := parseJSONResponse(raw, &findings); err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestParseJSONResponse_EmptyArray(t *testing.T) {
	var findings []rawFinding
	if err := parseJSONResponse("[]", &findings); err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected empty findings, got %d", len(findings))
	}
}

func TestParseJSONResponse_Garbage(t *testing.T) {
	var findings []rawFinding
	err := parseJSONResponse("This is not JSON at all.", &findings)
	if err == nil {
		t.Error("expected error for non-JSON input")
	}
}

func TestNormalizeSeverity(t *testing.T) {
	cases := map[string]Severity{
		"critical":   SeverityCritical,
		"CRITICAL":   SeverityCritical,
		"high":       SeverityHigh,
		"medium":     SeverityMedium,
		"suggestion": SeveritySuggestion,
		"unknown":    SeveritySuggestion,
		"":           SeveritySuggestion,
	}
	for input, expected := range cases {
		got := normalizeSeverity(input)
		if got != expected {
			t.Errorf("normalizeSeverity(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestClamp(t *testing.T) {
	if clamp(1.5, 0, 1) != 1 {
		t.Error("expected clamp(1.5, 0, 1) = 1")
	}
	if clamp(-0.1, 0, 1) != 0 {
		t.Error("expected clamp(-0.1, 0, 1) = 0")
	}
	if clamp(0.85, 0, 1) != 0.85 {
		t.Error("expected clamp(0.85, 0, 1) = 0.85")
	}
}

func TestDeterministicSort(t *testing.T) {
	findings := []Finding{
		{ID: "a", Severity: SeverityMedium, Confidence: 0.9},
		{ID: "b", Severity: SeverityCritical, Confidence: 0.8},
		{ID: "c", Severity: SeverityHigh, Confidence: 0.95},
	}
	sorted := deterministicSort(findings)
	if sorted[0].ID != "b" {
		t.Errorf("expected critical first, got %q", sorted[0].ID)
	}
	if sorted[1].ID != "c" {
		t.Errorf("expected high second, got %q", sorted[1].ID)
	}
	if sorted[2].ID != "a" {
		t.Errorf("expected medium third, got %q", sorted[2].ID)
	}
}

func TestFallbackFilter(t *testing.T) {
	findings := []Finding{
		{ID: "a", File: "a.go", Line: 1, Severity: SeverityHigh, Confidence: 0.9},
		{ID: "b", File: "b.go", Line: 2, Severity: SeverityMedium, Confidence: 0.7}, // below 0.8
		{ID: "c", File: "a.go", Line: 1, Severity: SeverityHigh, Confidence: 0.85},  // dup of a
	}
	result := fallbackFilter(findings, 0.8)
	if len(result) != 1 {
		t.Errorf("expected 1 finding after filter+dedup, got %d", len(result))
	}
	if result[0].ID != "a" {
		t.Errorf("expected finding 'a' to survive, got %q", result[0].ID)
	}
}
