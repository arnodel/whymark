package whymark

import (
	"bufio"
	"bytes"
	"net/url"
	"testing"
)

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

func TestHeaderRoundTrip(t *testing.T) {
	location, _ := url.Parse("file:///d.md#hall")
	h := Header{Location: location, PageID: "p1", Title: "Hall", Summary: "Took the lamp", Mark: "yellow", Resume: true}
	var b bytes.Buffer
	if err := WriteHeader(&b, h); err != nil {
		t.Fatal(err)
	}
	got, err := ReadHeader(bufio.NewReader(&b))
	if err != nil {
		t.Fatal(err)
	}
	if got.Location.String() != h.Location.String() || got.PageID != h.PageID || got.Title != h.Title ||
		got.Summary != h.Summary || got.Mark != h.Mark || got.Resume != h.Resume {
		t.Errorf("read back %+v, want %+v", got, h)
	}
}
