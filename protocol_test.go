package whymark

import "testing"

// TestIsDocument checks that a whymark document is told by its front
// matter's whymark key, whatever its settings.
func TestIsDocument(t *testing.T) {
	for source, want := range map[string]bool{
		"---\nwhymark:\n  version: 1\n---\n# Hall\n":               true,
		"---\r\nwhymark:\r\n  version: 1\r\n---\r\n# Hall\r\n":     true,
		"---\nwhymark:\n  uses:\n    ai: sometimes\n---\n# Hall\n": true, // the engine says what's wrong
		"---\ntitle: Notes\n---\n# Notes\n":                        false,
		"---\nwhymark:\n---\n# Notes\n":                            false,
		"# Notes\n\nwhymark: not front matter\n":                   false,
		"---\nwhymark:\n  version: 1\n":                            false, // no end to the front matter
	} {
		if got := IsDocument([]byte(source)); got != want {
			t.Errorf("IsDocument(%q) = %v, want %v", source, got, want)
		}
	}
}
