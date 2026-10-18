//go:build !js

package browser

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/arnodel/whymark"
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

func TestTreeLabels(t *testing.T) {
	app, _, _, _ := treeApp(t)
	app.Relayout(1024, 700, 1, 0)
	app.toggleExperiment("tree-map") // its own label of the selected page
	app.ShowTree()
	texts := func() string {
		r := &recorder{Area: app.Bounds()}
		app.Draw(r, 0)
		var b strings.Builder
		for _, t := range r.Texts {
			b.WriteString(t.S + " ")
		}
		return b.String()
	}
	if !strings.Contains(texts(), "just now") {
		t.Errorf("the tree's labels don't say how long ago its pages were made: %q", texts())
	}
	press(app, "1") // the first experiment: labels
	if strings.Contains(texts(), "just now") {
		t.Errorf("the tree shows labels with them turned off: %q", texts())
	}
	if !app.TreeShowing() {
		t.Error("toggling an experiment closed the tree")
	}
	app.HideTree()
	app.ShowActions()
	if !strings.Contains(string(app.actionsSource()), "| Labels in the tree of pages | **off** |") {
		t.Errorf("the actions page doesn't say labels are off: %s", app.actionsSource())
	}
	app.Follow("whymark:experiment/tree-labels")
	if !app.experiment("tree-labels") || !strings.Contains(string(app.actionsSource()), "| Labels in the tree of pages | **on** |") {
		t.Error("the actions page's link didn't turn labels back on")
	}
}

func TestTreeLabel(t *testing.T) {
	for _, c := range []struct {
		page whymark.PageInfo
		want string
	}{
		{whymark.PageInfo{Title: "Hall"}, "Hall"},
		{whymark.PageInfo{Title: "To do", Input: map[string]string{"task": " buy\n milk "}}, "“buy milk”"},
		{whymark.PageInfo{Input: map[string]string{"b": "2", "a": "1"}}, "“a: 1 · b: 2”"},
		{whymark.PageInfo{}, "Untitled page"},
	} {
		if got := treeLabel(c.page); got != c.want {
			t.Errorf("treeLabel(%+v) = %q, want %q", c.page, got, c.want)
		}
	}
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{5 * time.Minute, "5 min ago"},
		{3 * time.Hour, "3 h ago"},
		{30 * time.Hour, "yesterday"},
		{5 * 24 * time.Hour, "5 days ago"},
		{60 * 24 * time.Hour, "19 Aug 2026"},
	} {
		if got := ageOf(now.Add(-c.ago), now); got != c.want {
			t.Errorf("ageOf(%v ago) = %q, want %q", c.ago, got, c.want)
		}
	}
	if got := ageOf(time.Time{}, now); got != "" {
		t.Errorf("the age of no time is %q", got)
	}
}

func TestTreeBackForward(t *testing.T) {
	app, hall, lamp, shout := treeApp(t)
	app.ShowTree()
	press(app, "Left")
	press(app, "Up")
	for _, step := range []struct {
		do   func()
		want string
	}{
		{app.Back, lamp},
		{app.Back, shout},
		{app.Forward, lamp},
		{app.Forward, hall},
		{app.Back, lamp},
	} {
		step.do()
		if app.tree.selected != step.want {
			t.Fatalf("%s is selected, want %s", app.tree.selected, step.want)
		}
	}
	if !app.CanGoForward() {
		t.Error("can't go forward to a move Back undid")
	}
	press(app, "Right") // a new move: nothing to redo
	if app.CanGoForward() {
		t.Error("can go forward after a new move")
	}
	app.Back()
	app.Back()
	if app.tree.selected != shout {
		t.Fatalf("after undoing every move, %s is selected, want %s", app.tree.selected, shout)
	}
	app.Back() // nothing to undo: closes
	if app.TreeShowing() || pageID(app) != shout {
		t.Error("Back with no move to undo didn't close the tree on the page it was opened from")
	}
}

func TestTreePath(t *testing.T) {
	app, hall, lamp, shout := treeApp(t)
	app.ShowTree()
	press(app, "Left")
	if got := app.tree.path(); len(got) != 3 || !got[lamp] || !got[hall] || !got[shout] {
		t.Errorf("the path from the shout's page to the lamp's is %v", got)
	}
	_, path := app.treeLines(app.shownFrames(time.Now()))
	if len(path) != 1 { // the row's, which joins siblings, not the parent's
		t.Errorf("%d lines on the path, want 1", len(path))
	}
	press(app, "2") // the second experiment: the path
	if _, path := app.treeLines(app.shownFrames(time.Now())); len(path) != 0 {
		t.Errorf("%d lines on the path with it turned off", len(path))
	}
	press(app, "Right")
	if got := app.tree.path(); len(got) != 1 || !got[shout] {
		t.Errorf("the path from a page to itself is %v", got)
	}
}

// TestTreePathUpAndAcross checks the path after going up, then to a
// sibling: it goes along the row, not up.
func TestTreePathUpAndAcross(t *testing.T) {
	app, _, lamp, shout := treeApp(t)
	app.FollowAnchor("cellar") // a child of the shout's page
	cellar := pageID(app)
	app.ShowTree()
	press(app, "Up")
	if got := app.tree.path(); len(got) != 2 || !got[shout] || !got[cellar] {
		t.Errorf("the path after going up is %v", got)
	}
	_, path := app.treeLines(app.shownFrames(time.Now()))
	if len(path) != 3 { // the line down to the child, bent in three
		t.Errorf("after going up, %d lines on the path, want 3", len(path))
	}
	press(app, "Left")
	if app.tree.selected != lamp {
		t.Fatalf("%s is selected, want %s", app.tree.selected, lamp)
	}
	lines, path := app.treeLines(app.shownFrames(time.Now()))
	if len(path) != 1 {
		t.Errorf("after going across, %d lines on the path, want 1, the row's", len(path))
	}
	if len(lines) < 3 {
		t.Errorf("only %d lines off the path", len(lines))
	}
	// The parent is on the tree's path, but not on the path shown.
	if on := app.shownPath(); len(on) != 3 || on[app.tree.pages[lamp].Parent] {
		t.Errorf("the path shown is %v", on)
	}
}

// TestTreeKeepsLastChild checks that going up keeps the row of children
// lined up on the child the reader was on: the move is straight up.
func TestTreeKeepsLastChild(t *testing.T) {
	app, hall, lamp, shout := treeApp(t)
	app.ShowTree()
	press(app, "Left") // the lamp's page, beside the shout's
	before, _ := app.treeRect(lamp)
	press(app, "Up")
	after, _ := app.treeRect(lamp)
	if after.Min.X != before.Min.X {
		t.Errorf("going up moved the lamp's page sideways, from x=%d to x=%d", before.Min.X, after.Min.X)
	}
	if child := app.tree.preferredChild(hall); child != lamp {
		t.Errorf("Down from the hall selects %s, want %s, the page it was on", child, lamp)
	}
	// The way back goes down to the row of children and along it.
	if on := app.shownPath(); len(on) != 2 || !on[hall] || !on[shout] {
		t.Errorf("the path shown is %v", on)
	}
	if _, path := app.treeLines(app.shownFrames(time.Now())); len(path) != 4 {
		t.Errorf("%d lines on the path, want 4: down, bent in three, and along", len(path))
	}
	press(app, "Down")
	if app.tree.selected != lamp {
		t.Errorf("Down selected %s, want %s", app.tree.selected, lamp)
	}
}

// TestTreeParentRow checks that the parent's siblings are shown beside it,
// so that going up slides them straight down rather than showing them.
func TestTreeParentRow(t *testing.T) {
	app, _, lamp, _ := treeApp(t)
	app.FollowAnchor("cellar") // a child of the shout's page
	app.ShowTree()
	before, ok := app.treeRect(lamp)
	if !ok {
		t.Fatal("the parent's sibling isn't shown")
	}
	press(app, "Up")
	after, _ := app.treeRect(lamp)
	if after.Min.X != before.Min.X || after.Min.Y <= before.Min.Y {
		t.Errorf("going up moved the parent's sibling from %v to %v, want straight down", before, after)
	}
}

// TestTreePathAlongParentRow checks the path from a page to a cousin: up
// to the parent, and along its row, not up to the grandparent.
func TestTreePathAlongParentRow(t *testing.T) {
	info := func(id, parent string) whymark.PageInfo { return whymark.PageInfo{ID: id, Parent: parent} }
	app := &App{tree: &tree{
		pages: map[string]whymark.PageInfo{
			"g": info("g", ""), "p": info("p", "g"), "q": info("q", "g"),
			"s": info("s", "p"), "r": info("r", "q"),
		},
		children:  map[string][]string{"": {"g"}, "g": {"p", "q"}, "p": {"s"}, "q": {"r"}},
		selected:  "s",
		start:     "r",
		lastChild: map[string]string{},
	}}
	on := app.shownPath()
	if len(on) != 4 || !on["s"] || !on["p"] || !on["q"] || !on["r"] || on["g"] {
		t.Errorf("the path shown from s to its cousin r is %v, want s, p, q and r", on)
	}
}

// TestTreeBusPath checks the path drawn as in an organisation chart.
func TestTreeBusPath(t *testing.T) {
	app, _, lamp, shout := treeApp(t)
	app.FollowAnchor("cellar") // a child of the shout's page
	app.ShowTree()
	lit := func() int {
		_, path := app.busLines(app.shownFrames(time.Now()))
		return len(path)
	}
	// Up: the line down from the shout's page, along the children's
	// line, and down to the cellar's page.
	press(app, "Up")
	if n := lit(); n != 3 {
		t.Errorf("after going up, %d lines on the path, want 3", n)
	}
	// Across: down from the lamp's page's row line, along it, and down to
	// the shout's page; not up to the hall. Then down to the cellar.
	press(app, "Left")
	if app.tree.selected != lamp {
		t.Fatalf("%s is selected, want %s", app.tree.selected, lamp)
	}
	if n := lit(); n != 3 {
		t.Errorf("after going across, %d lines on the path, want 3", n)
	}
	// Back on the page it was opened from, there's no path.
	press(app, "Right")
	press(app, "Down")
	if app.tree.selected == shout || lit() != 0 {
		t.Errorf("on the page the view was opened from, %d lines on the path", lit())
	}
}

func TestTreeMap(t *testing.T) {
	app, hall, lamp, shout := treeApp(t)
	app.Relayout(1024, 700, 1, 0)
	app.ShowTree()
	m, ok := app.treeMapGeometry()
	if !ok {
		t.Fatal("the map isn't shown")
	}
	// The hall is centred over its children, the lamp's page on the left.
	h, l, s := m.at(hall), m.at(lamp), m.at(shout)
	if l.Y != s.Y || h.Y >= l.Y || l.X >= s.X || h.X != (l.X+s.X)/2 {
		t.Errorf("the map places the hall at %v, the lamp's page at %v and the shout's at %v", h, l, s)
	}
	// It stays put as the selection moves.
	press(app, "Left")
	if m2, _ := app.treeMapGeometry(); m2.at(hall) != h {
		t.Errorf("moving the selection moved the map's hall from %v to %v", h, m2.at(hall))
	}
	// Pointing at a dot shows its page; clicking it selects it.
	app.Frame([]input.Event{input.PointerMove{X: s.X, Y: s.Y}}, 0)
	if got := app.mapHovered(); got != shout {
		t.Errorf("pointing at the shout's dot points at %q", got)
	}
	app.Frame([]input.Event{input.PointerButton{X: h.X, Y: h.Y, Button: input.ButtonPrimary, Down: true}}, 0)
	if app.tree.selected != hall {
		t.Errorf("clicking the hall's dot selected %s", app.tree.selected)
	}
	app.Back()
	if app.tree.selected != lamp {
		t.Errorf("Back after clicking a dot selected %s, want %s", app.tree.selected, lamp)
	}
	press(app, "4") // the fourth experiment: the map
	if _, ok := app.treeMapGeometry(); ok {
		t.Error("the map is shown with it turned off")
	}
}
