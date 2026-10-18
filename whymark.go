package whymark

import (
	"context"
	"errors"
	"io"
	"net/url"
	"time"
)

// An Engine computes pages: a Request, naming a whymark document, gives a
// page. Its pages are streams: a header (see ReadHeader), then the page's
// Markdown, as the page is made.
type Engine interface {
	// Turn makes a new page, answering req: a child of the page
	// req.FromPageID names, or a document's start if none. The same
	// request twice makes two pages, siblings. Turn's error is the
	// engine's, such as a document it can't fetch or a page-id it doesn't
	// know; a document's own errors are shown in its page. Closing the
	// page stops its turn.
	Turn(ctx context.Context, req Request) (io.ReadCloser, error)
	// Saved returns the page id as it was made, and the request it
	// answered: it makes no new page. Its error is ErrNoOutput for a page
	// kept without its output, as earlier versions kept them.
	Saved(ctx context.Context, id string) (io.ReadCloser, Request, error)
	// Pages lists the pages of document, by when they were made.
	Pages(document Document) ([]PageInfo, error)
	// Visit records that the page id was shown to the reader, as a
	// browser can without the engine: keeping pages goes by when they
	// were last visited.
	Visit(id string) error
}

// ErrNoOutput is Saved's error for a page whose output wasn't kept, such
// as one kept by an earlier version: it can still be continued from.
var ErrNoOutput = errors.New("the page's output wasn't kept")

// A Document names a whymark document, as its pages are kept under it:
// its location, with no query or fragment (see DocumentOf).
type Document string

// DocumentOf returns the Document at location, whatever its passage or
// query.
func DocumentOf(location *url.URL) Document {
	u := *location
	u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = "", false, "", ""
	return Document(u.String())
}

// PageInfo is what a list of pages says of each: enough to place it in the
// tree of pages, and label it.
type PageInfo struct {
	ID, Parent     string // Parent is "" for a document's start
	Passage, Title string
	Time           time.Time // when it was made
	// LastVisited is when it was last visited, if the engine keeps track
	// (see Engine.Visit).
	LastVisited time.Time
	// Input is the named fields typed in for the request that made it, if
	// any (see Request.Input).
	Input map[string]string
}
