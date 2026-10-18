//go:build !js

package browser

import (
	"context"
	"github.com/arnodel/whymark/engine"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whymark/engine/ai"
)

// cannedAI answers every request with the same text.
type cannedAI string

func (c cannedAI) Ask(ctx context.Context, req ai.Request, w io.Writer) (string, error) {
	if w != nil {
		io.WriteString(w, string(c))
	}
	return string(c), nil
}

// withAI is an option giving the engine provider as its AI, for the
// documents granted reports.
func withAI(provider ai.Provider, granted func(document *url.URL) bool) engine.Option {
	return func(e *engine.Engine) {
		engine.WithCapabilities(&ai.AI{Provider: provider, Models: map[string]string{"balanced": "test-model"}, Default: "balanced"})(e)
		engine.WithGrant(func(document *url.URL, name string) bool { return granted(document) })(e)
	}
}

// localOnly grants local documents, as the viewers do by default.
func localOnly(document *url.URL) bool { return document.Scheme == "file" }

// TestAIPages checks that a page asking the AI shows its answer when the
// engine grants the document the AI, and its without block otherwise.
func TestAIPages(t *testing.T) {
	const doc = "---\nwhymark:\n  version: 1\n  uses:\n    ai: optional\n---\n# Shop\n\n" +
		"```ai\nDescribe the shop.\n```\n```without ai\nThe shop is quiet.\n```\n"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, doc)
	}))
	defer remote.Close()

	for _, c := range []struct {
		name          string
		remoteAllowed bool
		link, want    string
	}{
		{"local", false, "shop.md", "Clocks tick everywhere"},
		{"remote", false, remote.URL + "/shop.md", "The shop is quiet"},
		{"remote, allowed", true, remote.URL + "/shop.md", "Clocks tick everywhere"},
	} {
		dir := t.TempDir()
		start := writeTempMD(t, dir, "start.md", "# Start\n")
		writeTempMD(t, dir, "shop.md", doc)
		granted := localOnly
		if c.remoteAllowed {
			granted = func(*url.URL) bool { return true }
		}
		app := newTestApp(t, dir, start, "# Start\n", withAI(cannedAI("Clocks tick everywhere."), granted))
		app.Follow(c.link)
		if text := pageText(app); !strings.Contains(text, c.want) {
			t.Errorf("%s: the page shows %q, want %q", c.name, text, c.want)
		}
	}
}

// heldAI answers with its text once release is closed.
type heldAI struct {
	text    string
	release chan struct{}
}

func (h heldAI) Ask(ctx context.Context, req ai.Request, w io.Writer) (string, error) {
	<-h.release
	return cannedAI(h.text).Ask(ctx, req, w)
}

// TestTurnInBackground checks that following a link to a page that waits
// for the AI before writing anything returns at once, and that the page,
// its header and so its location arrive with Update.
func TestTurnInBackground(t *testing.T) {
	const doc = "---\nwhymark:\n  version: 1\n  uses:\n    ai: optional\n---\n# Shop\n\n" +
		"```ai\nDescribe the shop.\n```\n```without ai\nThe shop is quiet.\n```\n"
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n")
	writeTempMD(t, dir, "shop.md", doc)
	release := make(chan struct{})
	app := newTestApp(t, dir, start, "# Start\n", withAI(heldAI{"Clocks tick everywhere.", release}, localOnly))
	app.syncTurns = false

	followed := time.Now()
	app.Follow("shop.md")
	if waited := time.Since(followed); waited > time.Second {
		t.Fatalf("following the link waited %v for the AI", waited)
	}
	if strings.Contains(app.Location().Fragment, "@") {
		t.Errorf("the location is %s before the page's header arrived", app.Location())
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(pageText(app), "Clocks tick everywhere") || !strings.Contains(app.Location().Fragment, "@") {
		if time.Now().After(deadline) {
			t.Fatalf("the page shows %q at %s, want the AI's answer and the page's own location", pageText(app), app.Location())
		}
		app.Update()
		time.Sleep(5 * time.Millisecond)
	}
}
