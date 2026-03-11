package memory

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// descriptionHash returns a stable hash for a finding pattern used to detect false positives.
func descriptionHash(file, category, description string) string {
	// Normalize: lowercase, trim whitespace, strip variable parts like line numbers
	normalized := strings.ToLower(strings.TrimSpace(file + "|" + category + "|" + description))
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h[:12])
}
