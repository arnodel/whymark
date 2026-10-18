//go:build !js

package browser

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
)

// pageID returns the page-id of the current whymark page.
func pageID(app *App) string {
	return app.pageOf(app.Panel.View().Document()).header.PageID
}

// treeApp returns an App that played the cellar into a small tree: the
// hall, and two children of it, taking the lamp, then shouting, which is
// the current page.
func treeApp(t *testing.T) (app *App, hall, lamp, shout string) {
	t.Helper()
	instantTree(t)
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Play](cellar.md)\n")
	writeTempMD(t, dir, "cellar.md", cellarDoc(false))
	app = newTestApp(t, dir, start, "# Start\n\n[Play](cellar.md)\n", withStore(t, dir))
	app.Follow("cellar.md")
	hall = pageID(app)
	app.FollowAnchor("take-the-lamp")
	lamp = pageID(app)
	app.Back()
	app.Follow("?shout=yes")
	shout = pageID(app)
	return app, hall, lamp, shout
}

// instantTree makes the tree view's zooms and slides take no time, for
// the test t.
func instantTree(t *testing.T) {
	zoom, slide := treeZoomTime, treeSlideTime
	treeZoomTime, treeSlideTime = 0, 0
	t.Cleanup(func() { treeZoomTime, treeSlideTime = zoom, slide })
}

// press presses the key named name, with no modifiers.
func press(app *App, name string) { app.PressKey(Key{Name: name}) }

func TestTreeMoves(t *testing.T) {
	app, hall, lamp, shout := treeApp(t)
	if !app.CanShowTree() {
		t.Fatal("can't show the tree of a whymark page")
	}
	app.ShowTree()
	for _, step := range []struct{ key, want string }{
		{"Left", lamp},   // the older sibling
		{"Left", lamp},   // the oldest: nothing
		{"Right", shout}, // back
		{"Up", hall},     // the parent
		{"Up", hall},     // a start: nothing
		{"Left", hall},   // no siblings
		{"Down", shout},  // the child on the way back to where the view opened
	} {
		press(app, step.key)
		if app.tree.selected != step.want {
			t.Fatalf("after %s, %s is selected, want %s", step.key, app.tree.selected, step.want)
		}
	}
	press(app, "Escape")
	if app.TreeShowing() || pageID(app) != shout {
		t.Error("Escape didn't close the tree, keeping the page")
	}
	app.PressKey(Key{Name: "G", Shortcut: true})
	if !app.TreeShowing() {
		t.Error("Cmd+G didn't show the tree")
	}
	app.PressKey(Key{Name: "G", Shortcut: true})
	if app.TreeShowing() {
		t.Error("Cmd+G didn't hide the tree")
	}
}

func TestTreeOpen(t *testing.T) {
	app, _, lamp, shout := treeApp(t)
	app.ShowTree()
	press(app, "Left")
	press(app, "Enter")
	if app.TreeShowing() || pageID(app) != lamp || !strings.Contains(pageText(app), "You take the lamp") {
		t.Fatalf("Enter shows page %s (%q), want the lamp's, %s", pageID(app), pageText(app), lamp)
	}
	if !strings.HasSuffix(app.Location().Fragment, "@"+lamp) {
		t.Errorf("the page opened is at %s", app.Location())
	}
	app.Back()
	if pageID(app) != shout {
		t.Errorf("Back after opening a page shows %s, want %s", pageID(app), shout)
	}
}

func TestTreeDrawAndClick(t *testing.T) {
	app, _, lamp, _ := treeApp(t)
	app.Relayout(600, 800, 1, 0) // room for the children's tops
	app.ShowTree()
	press(app, "Up") // the hall, with both children below
	// The children this window has shown keep their Views; the others
	// are loaded in the background.
	deadline := time.Now().Add(5 * time.Second)
	for app.Update(); len(app.tree.loading) > 0 && time.Now().Before(deadline); app.Update() {
		time.Sleep(10 * time.Millisecond)
	}
	r := &recorder{Area: app.Bounds()}
	app.Draw(r, 0)
	var text strings.Builder
	for _, t := range r.Texts {
		text.WriteString(t.S + " ")
	}
	// The hall, and the top of the shout's page below it. The lamp's
	// page, beside it, shows its right edge.
	for _, want := range []string{"dark below", "echoes"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("the tree doesn't show %q: %q", want, text.String())
		}
	}
	lampView := &recorder{Area: app.Bounds()}
	app.tree.views[lamp].Draw(lampView, 0)
	if len(lampView.Texts) == 0 || lampView.Texts[0].S != "You" {
		t.Errorf("the lamp's page, loaded, shows %v", lampView.Texts)
	}

	// A click in the middle of id's frame, wherever the selection put it.
	click := func(id string) {
		for _, f := range app.treeFrames(app.Bounds()) {
			if f.id == id {
				p := f.rect.Min.Add(f.rect.Size().Div(2))
				app.Frame([]input.Event{input.PointerButton{X: p.X, Y: p.Y, Button: input.ButtonPrimary, Down: true}}, 0)
				return
			}
		}
		t.Fatalf("page %s isn't on screen", id)
	}
	click(lamp)
	if app.tree.selected != lamp {
		t.Fatalf("a click on the lamp's page selected %s", app.tree.selected)
	}
	click(lamp)
	if app.TreeShowing() || pageID(app) != lamp {
		t.Errorf("a click on the selected page didn't open it")
	}
}

func TestTreeNotForMarkdown(t *testing.T) {
	dir := t.TempDir()
	loc := writeTempMD(t, dir, "a.md", "# A\n")
	app := newTestApp(t, dir, loc, "# A\n")
	if app.CanShowTree() {
		t.Error("can show the tree of a Markdown document")
	}
	app.ShowTree()
	if app.TreeShowing() {
		t.Error("ShowTree showed the tree of a Markdown document")
	}
}

func TestTreeAnimates(t *testing.T) {
	app, hall, lamp, shout := treeApp(t)
	treeZoomTime, treeSlideTime = 50*time.Millisecond, 50*time.Millisecond
	settle := func() {
		time.Sleep(60 * time.Millisecond)
		app.Update()
	}

	// It zooms out from the current page, shown as it was.
	app.ShowTree()
	r, _ := app.treeRect(shout)
	if c := app.tree.camera.from; c != app.zoomOn(r) || c.Scale != 2 {
		t.Errorf("the tree zooms out from %v, want %v", c, app.zoomOn(r))
	}
	if !app.Animating() {
		t.Error("the tree isn't animating as it zooms out")
	}
	settle()
	if app.tree.camera.moving(time.Now()) || app.tree.camera.at(time.Now()) != canvas.Identity {
		t.Error("the tree didn't stop zooming out")
	}

	// Moving slides the frames from where they were: the page selected
	// before from the middle, and the parent, which stays, not at all.
	before, _ := app.treeRect(shout)
	hallBefore, _ := app.treeRect(hall)
	press(app, "Left")
	start := app.tree.slide.start
	for _, f := range []struct {
		id   string
		want image.Rectangle
	}{{shout, before}, {hall, hallBefore}} {
		r, _ := app.treeRect(f.id)
		if got := app.tree.slide.at(f.id, r, start); got != f.want {
			t.Errorf("page %s starts sliding at %v, want %v", f.id, got, f.want)
		}
	}
	settle()

	// Opening zooms in on the page, then opens it.
	press(app, "Enter")
	if !app.TreeShowing() || pageID(app) != shout {
		t.Fatal("Enter opened the page before zooming in on it")
	}
	settle()
	if app.TreeShowing() || pageID(app) != lamp {
		t.Errorf("after zooming in, page %s shows, want %s", pageID(app), lamp)
	}
}
