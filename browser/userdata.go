package browser

import (
	"sync"

	"github.com/arnodel/whymark"
)

// UserData is what the browser keeps for its user, beyond one window and
// one run: where they've been, and later what they've chosen. The program
// keeps it on the user's behalf, as it sees fit (see NewApp); a whymark
// document's own state is its engine's, in its pages.
type UserData interface {
	// LatestPage returns the page-id of the latest page of document, a
	// document asking to be resumed (its pages' resume hint), or "".
	LatestPage(document whymark.Document) (string, error)
	// SetLatestPage records the page id as the latest of document.
	SetLatestPage(document whymark.Document, id string) error
}

// NewMemoryUserData returns UserData kept in memory, for as long as the
// program runs.
func NewMemoryUserData() UserData {
	return &memoryUserData{latest: map[whymark.Document]string{}}
}

type memoryUserData struct {
	mu     sync.Mutex
	latest map[whymark.Document]string
}

func (m *memoryUserData) LatestPage(document whymark.Document) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.latest[document], nil
}

func (m *memoryUserData) SetLatestPage(document whymark.Document, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latest[document] = id
	return nil
}
