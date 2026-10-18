package browser

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"log"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/arnodel/whymark"
	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
)

// tree is the state of the view of the tree of pages (see ShowTree).
type tree struct {
	document whymark.Document
	pages    map[string]whymark.PageInfo
	// children are each page's children, oldest first, by its id: "" for
	// the document's starts.
	children map[string][]string
	// selected is the page in the middle, and start the page the view was
	// opened from.
	selected, start string
	// views are the pages' Views, by id, as they're shown in their
	// frames: the page's own if this window has shown it, or else loaded
	// (see loadThumb), with a stand-in until then.
	views map[string]*whynot.View
	// loading is the pages being loaded.
	loading map[string]bool
}

// A loadedThumb is a page loaded in the background for the tree view:
// its output and the request it answered, or why it couldn't be.
type loadedThumb struct {
	tree    *tree
	id      string
	output  []byte
	request whymark.Request
	err     error
}

// treeLoads holds the pages loaded in the background, for Update to show
// (see addThumbs).
type treeLoads struct {
	mu     sync.Mutex
	loaded []loadedThumb
}

// treeGap is the space between frames, in logical pixels.
const treeGap = 24

// TreeShowing reports whether the view of the tree of pages is showing.
func (a *App) TreeShowing() bool { return a.tree != nil }

// CanShowTree reports whether ShowTree would do anything: the current
// document must be a page of a whymark document.
func (a *App) CanShowTree() bool {
	return a.treePage() != nil
}

// treePage returns the current document as a whymark page, if it's one
// whose header has arrived, else nil.
func (a *App) treePage() *whymarkPage {
	if a.Panel == nil {
		return nil
	}
	view := a.Panel.View()
	if a.overlaid != nil {
		view = a.overlaid
	}
	if p := a.pageOf(view.Document()); p != nil && p.header.PageID != "" {
		return p
	}
	return nil
}

// ShowTree shows the tree of the current document's pages around the
// current page, at half size: its parent above, its siblings beside it,
// oldest on the left, and its children below. The arrow keys move the
// selection, which stays in the middle, and Enter opens the selected page
// (see OpenSelected). A no-op if the current document isn't a page of a
// whymark document.
func (a *App) ShowTree() {
	p := a.treePage()
	if p == nil || a.tree != nil {
		return
	}
	a.hideOverlay()
	t := &tree{
		document: whymark.DocumentOf(p.header.Location),
		selected: p.header.PageID,
		start:    p.header.PageID,
		views:    map[string]*whynot.View{},
		loading:  map[string]bool{},
	}
	a.tree = t
	a.listPages()
	t.views[t.start] = a.Panel.View()
	for _, entries := range [][]historyEntry{a.history, a.future} {
		for _, e := range entries {
			if q := a.pageOf(e.view.Document()); q != nil && q.header.PageID != "" {
				t.views[q.header.PageID] = e.view
			}
		}
	}
}

// listPages lists the tree's pages again, from the engine.
func (a *App) listPages() {
	t := a.tree
	infos, err := a.engine.Pages(t.document)
	if err != nil {
		log.Printf("listing the pages of %s: %v", t.document, err)
	}
	t.pages = map[string]whymark.PageInfo{}
	t.children = map[string][]string{}
	for _, info := range infos {
		t.pages[info.ID] = info
		t.children[info.Parent] = append(t.children[info.Parent], info.ID)
	}
	for _, id := range []string{t.start, t.selected} {
		if _, ok := t.pages[id]; ok {
			continue
		}
		// Not kept by the engine: a page on its own.
		info := whymark.PageInfo{ID: id}
		t.pages[id] = info
		t.children[""] = append(t.children[""], id)
	}
}

// HideTree closes the view of the tree of pages, leaving the current page
// as it was.
func (a *App) HideTree() { a.tree = nil }

// ToggleTree shows the tree of pages, or hides it if it's showing.
func (a *App) ToggleTree() {
	if a.tree != nil {
		a.HideTree()
		return
	}
	a.ShowTree()
}

// OpenSelected opens the page selected in the tree of pages, as opening
// its page URL does, and closes the tree. A no-op if the tree isn't
// showing.
func (a *App) OpenSelected() {
	t := a.tree
	if t == nil {
		return
	}
	a.HideTree()
	if t.selected == t.start {
		return
	}
	u, err := url.Parse(string(t.document))
	if err != nil {
		log.Printf("opening a page of %s: %v", t.document, err)
		return
	}
	u.Fragment = t.pages[t.selected].Passage + "@" + t.selected
	a.follow(u)
}

// treeKey handles k while the tree of pages shows, reporting whether it
// was one of the tree's keys.
func (a *App) treeKey(k Key) bool {
	t := a.tree
	if k.Shortcut || k.Shift || k.Alt {
		return false
	}
	switch k.Name {
	case "Up":
		if parent := t.pages[t.selected].Parent; parent != "" {
			t.selected = parent
		}
	case "Down":
		if child := t.preferredChild(t.selected); child != "" {
			t.selected = child
		}
	case "Left", "Right":
		siblings := t.children[t.pages[t.selected].Parent]
		i := slices.Index(siblings, t.selected)
		if k.Name == "Left" {
			i--
		} else {
			i++
		}
		if i >= 0 && i < len(siblings) {
			t.selected = siblings[i]
		}
	case "Enter":
		a.OpenSelected()
	case "Escape":
		a.HideTree()
	default:
		return false
	}
	return true
}

// preferredChild returns the child of id Down selects: the one on the way
// back to the page the view was opened from, if any, or else the one
// visited most recently, or else the newest. It's "" without children.
func (t *tree) preferredChild(id string) string {
	children := t.children[id]
	if len(children) == 0 {
		return ""
	}
	for p := t.start; p != ""; p = t.pages[p].Parent {
		if t.pages[p].Parent == id {
			return p
		}
		if p == id {
			break
		}
	}
	best := children[len(children)-1]
	var last time.Time
	for _, c := range children {
		if v := t.pages[c].LastVisited; v.After(last) {
			best, last = c, v
		}
	}
	return best
}

// A treeFrame is where a page is drawn in the tree view.
type treeFrame struct {
	id   string
	rect image.Rectangle
}

// treeRow returns the frames of ids, a row of siblings, with ids[k]'s
// at r and the others beside it.
func treeRow(ids []string, k int, r image.Rectangle, gap int) []treeFrame {
	frames := make([]treeFrame, len(ids))
	for i, id := range ids {
		frames[i] = treeFrame{id, r.Add(image.Pt((i-k)*(r.Dx()+gap), 0))}
	}
	return frames
}

// treeLayout returns the frames of the tree view: the selected page's
// parent, the row of the selected page and its siblings, and the row of
// its children, with the child Down selects below it; and the lines
// joining them, as rectangles to fill.
func (a *App) treeLayout() (parent []treeFrame, rows [][]treeFrame, lines []image.Rectangle) {
	t := a.tree
	b := a.bounds
	gap := int(treeGap * a.deviceScale)
	thick := max(1, int(a.deviceScale))
	w, h := b.Dx()/2, b.Dy()/2
	center := image.Pt(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)
	sel := image.Rect(center.X-w/2, center.Y-h/2, center.X-w/2+w, center.Y-h/2+h)
	down := func(from, to image.Rectangle) image.Rectangle {
		x := from.Min.X + from.Dx()/2 - thick/2
		return image.Rect(x, from.Max.Y, x+thick, to.Min.Y)
	}
	row := func(frames []treeFrame) {
		rows = append(rows, frames)
		for i := 1; i < len(frames); i++ {
			y := frames[i].rect.Min.Y + h/2 - thick/2
			lines = append(lines, image.Rect(frames[i-1].rect.Max.X, y, frames[i].rect.Min.X, y+thick))
		}
	}
	p := t.pages[t.selected].Parent
	if p != "" {
		r := sel.Sub(image.Pt(0, h+gap))
		parent = []treeFrame{{p, r}}
		lines = append(lines, down(r, sel))
	}
	siblings := t.children[p]
	row(treeRow(siblings, slices.Index(siblings, t.selected), sel, gap))
	if child := t.preferredChild(t.selected); child != "" {
		children := t.children[t.selected]
		r := sel.Add(image.Pt(0, h+gap))
		row(treeRow(children, slices.Index(children, child), r, gap))
		lines = append(lines, down(sel, r))
	}
	return parent, rows, lines
}

// treeFrames returns the frames of the tree view that are on screen.
func (a *App) treeFrames() []treeFrame {
	parent, rows, _ := a.treeLayout()
	var frames []treeFrame
	for _, f := range slices.Concat(append([][]treeFrame{parent}, rows...)...) {
		if f.rect.Overlaps(a.bounds) {
			frames = append(frames, f)
		}
	}
	return frames
}

// drawTree draws the tree view.
func (a *App) drawTree(dst canvas.Canvas, now time.Duration) {
	background, line := a.stackColors()
	b := a.bounds
	dst.DrawRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), background)
	_, _, lines := a.treeLayout()
	for _, l := range lines {
		dst.DrawRect(l.Min.X, l.Min.Y, l.Dx(), l.Dy(), line)
	}
	thick := max(1, int(a.deviceScale))
	for _, f := range a.treeFrames() {
		v := a.treeView(f.id)
		if v.Bounds() != b || v.Scale() != a.deviceScale || v.Zoom() != a.zoom {
			a.place(v)
		}
		scale := float64(f.rect.Dx()) / float64(b.Dx())
		v.Draw(dst.Clip(f.rect).Transform(canvas.Transform{
			Scale: scale,
			X:     float64(f.rect.Min.X) - scale*float64(b.Min.X),
			Y:     float64(f.rect.Min.Y) - scale*float64(b.Min.Y),
		}), now)
		r := f.rect.Inset(-thick)
		dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), thick, line)
		dst.DrawRect(r.Min.X, r.Max.Y-thick, r.Dx(), thick, line)
		dst.DrawRect(r.Min.X, r.Min.Y, thick, r.Dy(), line)
		dst.DrawRect(r.Max.X-thick, r.Min.Y, thick, r.Dy(), line)
	}
}

// treeView returns the View of page id shown in its frame: a stand-in
// with its title until it's loaded.
func (a *App) treeView(id string) *whynot.View {
	t := a.tree
	if v := t.views[id]; v != nil {
		return v
	}
	title := t.pages[id].Title
	if title == "" {
		title = "Untitled page"
	}
	v := a.overlayView([]byte("# " + escapeMarkdown(title) + "\n"))
	t.views[id] = v
	return v
}

// loadThumbs starts loading, in the background, the pages of the tree
// view on screen that this window hasn't shown. Update shows them when
// they're loaded (see addThumbs).
func (a *App) loadThumbs() {
	t := a.tree
	if t == nil {
		return
	}
	for _, f := range a.treeFrames() {
		if _, ok := t.views[f.id]; ok || t.loading[f.id] {
			continue
		}
		t.loading[f.id] = true
		engine, id, notify := a.engine, f.id, a.OnDocumentChange
		go func() {
			loaded := loadedThumb{tree: t, id: id}
			r, req, err := engine.Saved(context.Background(), id)
			if err == nil {
				loaded.output, err = io.ReadAll(r)
				r.Close()
			}
			loaded.request, loaded.err = req, err
			a.treeLoads.mu.Lock()
			a.treeLoads.loaded = append(a.treeLoads.loaded, loaded)
			a.treeLoads.mu.Unlock()
			if notify != nil {
				notify()
			}
		}()
	}
}

// addThumbs shows the pages loaded since it last ran in the tree view, if
// it's still showing.
func (a *App) addThumbs() {
	a.treeLoads.mu.Lock()
	loaded := a.treeLoads.loaded
	a.treeLoads.loaded = nil
	a.treeLoads.mu.Unlock()
	for _, l := range loaded {
		t := l.tree
		if t != a.tree {
			continue
		}
		delete(t.loading, l.id)
		if l.err != nil {
			if !errors.Is(l.err, whymark.ErrNoOutput) {
				log.Printf("loading page %s for the tree of pages: %v", l.id, l.err)
			}
			a.treeView(l.id) // the stand-in stays
			continue
		}
		doc, err := a.pageFrom(io.NopCloser(bytes.NewReader(l.output)), l.request, false)
		if err != nil {
			log.Printf("showing page %s in the tree of pages: %v", l.id, err)
			a.treeView(l.id)
			continue
		}
		v := a.NewView(doc)
		a.place(v)
		t.views[l.id] = v
	}
}

// treeInput handles events while the tree view shows: a click on a page
// selects it, and a click on the selected page opens it.
func (a *App) treeInput(events []input.Event) {
	for _, e := range events {
		var at image.Point
		switch e := e.(type) {
		case input.PointerButton:
			if !e.Down || e.Button != input.ButtonPrimary {
				continue
			}
			at = image.Pt(e.X, e.Y)
		case input.TouchStart:
			at = image.Pt(e.X, e.Y)
		default:
			continue
		}
		for _, f := range a.treeFrames() {
			if !at.In(f.rect) {
				continue
			}
			if f.id == a.tree.selected {
				a.OpenSelected()
				return
			}
			a.tree.selected = f.id
			break
		}
	}
}
