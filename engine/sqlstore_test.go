//go:build !js

package engine

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/arnodel/whymark"
)

func newSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "whymark.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newEngineOn returns an engine on store, as in the program's next run.
func newEngineOn(t *testing.T, store Store) *Engine {
	e, _ := testEngine(t, nil, WithStore(store))
	return e
}

// TestEnginePages checks the list of a document's pages: every page, with
// its parent, passage, title and input, by when it was made, from the
// Store and from a new engine on it alike.
func TestEnginePages(t *testing.T) {
	files := map[string]string{"cave.md": "---\nwhymark:\n  version: 1\n---\n# Entrance\n\n[In](#hall)\n\n# Hall\n\n[Out](#entrance)\n"}
	store := newSQLiteStore(t)
	e, file := testEngine(t, files, WithStore(store))
	start := take(t, e, whymark.Request{Location: file("cave.md")})
	hall := follow(t, e, start, "#hall")
	again := follow(t, e, start, "#hall") // a sibling of hall
	typed := take(t, e, whymark.Request{Location: file("cave.md"), FromPageID: start.PageID, Input: map[string]string{"word": "xyzzy"}})

	document := whymark.DocumentOf(file("cave.md"))
	for _, engine := range []*Engine{e, newEngineOn(t, store)} {
		pages, err := engine.Pages(document)
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) != 4 {
			t.Fatalf("%d pages, want 4: %+v", len(pages), pages)
		}
		want := []struct{ id, parent, passage, title, input string }{
			{start.PageID, "", "entrance", "Entrance", ""},
			{hall.PageID, start.PageID, "hall", "Hall", ""},
			{again.PageID, start.PageID, "hall", "Hall", ""},
			{typed.PageID, start.PageID, "entrance", "Entrance", "xyzzy"},
		}
		for i, w := range want {
			p := pages[i]
			if p.ID != w.id || p.Parent != w.parent || p.Passage != w.passage || p.Title != w.title || p.Input["word"] != w.input {
				t.Errorf("page %d is %+v, want %+v", i, p, w)
			}
		}
	}
	if pages, _ := e.Pages("file:///elsewhere.md"); len(pages) != 0 {
		t.Errorf("another document has pages: %+v", pages)
	}
}

// TestVisits checks last_visited_at: a page's creation is its first visit,
// Visit moves it on but never back, and saving the page again keeps it.
func TestVisits(t *testing.T) {
	store := newSQLiteStore(t)
	made := time.Date(2026, 10, 18, 9, 15, 3, 120000000, time.FixedZone("CEST", 2*3600))
	page := SavedPage{Document: "file:///d.md", Location: "file:///d.md", Time: made, Title: "D"}
	if err := store.SavePage("p1", page); err != nil {
		t.Fatal(err)
	}
	visited := func() time.Time {
		t.Helper()
		pages, err := store.Pages("file:///d.md")
		if err != nil || len(pages) != 1 {
			t.Fatalf("pages %+v, %v", pages, err)
		}
		if !pages[0].Time.Equal(made) {
			t.Errorf("made at %v, want %v", pages[0].Time, made)
		}
		return pages[0].LastVisited
	}
	if v := visited(); !v.Equal(made) {
		t.Errorf("a new page was last visited at %v, want when it was made, %v", v, made)
	}
	later := made.Add(time.Hour)
	store.Visit("p1", later)
	store.Visit("p1", made.Add(time.Minute)) // an earlier visit, reported late
	if v := visited(); !v.Equal(later) {
		t.Errorf("after visits, last visited at %v, want %v", v, later)
	}
	if err := store.SavePage("p1", page); err != nil {
		t.Fatal(err)
	}
	if v := visited(); !v.Equal(later) {
		t.Errorf("saving the page again moved its last visit to %v", v)
	}
	// Times sort as text: a whole second before a fraction of it.
	if a, b := timeText(made.Truncate(time.Second)), timeText(made); a >= b {
		t.Errorf("%q sorts after %q", a, b)
	}
}

// TestSQLiteStoreAddsColumns checks that a database made before pages had
// summaries and marks gets them, empty.
func TestSQLiteStoreAddsColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "whymark.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE pages (id TEXT PRIMARY KEY, document TEXT NOT NULL,
		parent TEXT NOT NULL DEFAULT '', passage TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL, last_visited_at TEXT NOT NULL, location TEXT NOT NULL,
		state TEXT, notes TEXT, input TEXT, output TEXT NOT NULL DEFAULT '');
		INSERT INTO pages (id, document, created_at, last_visited_at, location)
		VALUES ('old', 'file:///d.md', '2026-10-01T00:00:00.000000000Z', '2026-10-01T00:00:00.000000000Z', 'file:///d.md')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pages, err := store.Pages("file:///d.md")
	if err != nil || len(pages) != 1 || pages[0].Summary != "" || pages[0].Mark != "" {
		t.Fatalf("the old page is listed as %+v, %v", pages, err)
	}
	if err := store.SavePage("new", SavedPage{Document: "file:///d.md", Location: "file:///d.md", Summary: "s", Mark: "red"}); err != nil {
		t.Fatal(err)
	}
}
