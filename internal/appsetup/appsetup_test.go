package appsetup

import (
	"net/url"
	"testing"
)

// TestIsLocal checks which documents count as local, and so get the AI
// without -remote-ai.
func TestIsLocal(t *testing.T) {
	for location, want := range map[string]bool{
		"file:///home/me/todo.md":     true,
		"whymark:welcome":             true,
		"https://example.com/todo.md": false,
		"http://localhost/todo.md":    false,
	} {
		u, err := url.Parse(location)
		if err != nil {
			t.Fatal(err)
		}
		if got := isLocal(u); got != want {
			t.Errorf("isLocal(%s) = %v, want %v", location, got, want)
		}
	}
}
