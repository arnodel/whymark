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
	// camera is how the tree is drawn on the window, slide how its frames
	// are moving to their places, and closing what to do once the camera
	// has zoomed in on a page, closing the view.
	camera  treeCamera
	slide   treeSlide
	closing func()
}

// treeZoomTime and treeSlideTime are how long the tree view takes to zoom
// in or out, and to slide to a new selection.
var treeZoomTime, treeSlideTime = 300 * time.Millisecond, 200 * time.Millisecond

// A treeCamera moves the tree view from one transform to another, over a
// length of time, easing out.
type treeCamera struct {
	from, to canvas.Transform
	start    time.Time
	length   time.Duration
}

// at returns c's transform at now.
func (c treeCamera) at(now time.Time) canvas.Transform {
	if !c.moving(now) {
		return c.to
	}
	f := float64(now.Sub(c.start)) / float64(c.length)
	f = 1 - (1-f)*(1-f)*(1-f)
	lerp := func(x, y float64) float64 { return x + (y-x)*f }
	return canvas.Transform{
		Scale: lerp(c.from.Scale, c.to.Scale),
		X:     lerp(c.from.X, c.to.X),
		Y:     lerp(c.from.Y, c.to.Y),
	}
}

// moving reports whether c is still moving at now.
func (c treeCamera) moving(now time.Time) bool {
	return c.length > 0 && now.Before(c.start.Add(c.length))
}

// move starts moving t's camera from from to to, over length.
func (t *tree) move(from, to canvas.Transform, length time.Duration) {
	t.camera = treeCamera{from: from, to: to, start: time.Now(), length: length}
}

// zoomOn returns the camera showing r, a frame, as the whole window: a
// page's frame is half its size, so its page is then drawn as it is.
func (a *App) zoomOn(r image.Rectangle) canvas.Transform {
	b := a.bounds
	s := float64(b.Dx()) / float64(r.Dx())
	return canvas.Transform{Scale: s, X: float64(b.Min.X) - s*float64(r.Min.X), Y: float64(b.Min.Y) - s*float64(r.Min.Y)}
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
// oldest on the left, and its children below, zooming out from the
// current page. The arrow keys move the selection, which slides to the
// middle, and Enter opens the selected page (see OpenSelected). A no-op if
// the current document isn't a page of a whymark document.
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
	r, _ := a.treeRect(t.start)
	t.move(a.zoomOn(r), canvas.Identity, treeZoomTime)
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

// HideTree closes the view of the tree of pages at once, leaving the
// current page as it was.
func (a *App) HideTree() { a.tree = nil }

// closeTree closes the view of the tree of pages by zooming in on page
// id's frame, then does act: at once if the frame isn't on screen. A
// no-op if it's already closing.
func (a *App) closeTree(id string, act func()) {
	t := a.tree
	if t.closing != nil {
		return
	}
	camera := t.camera.at(time.Now())
	r, ok := a.treeRect(id)
	if !ok || treeZoomTime <= 0 || !camera.ApplyRect(r).Overlaps(a.bounds) {
		a.HideTree()
		act()
		return
	}
	t.closing = act
	t.move(camera, a.zoomOn(r), treeZoomTime)
}

// finishTree closes the view of the tree of pages if it has finished
// zooming in on a page (see closeTree).
func (a *App) finishTree() {
	if t := a.tree; t != nil && t.closing != nil && !t.camera.moving(time.Now()) {
		a.HideTree()
		t.closing()
	}
}

// ToggleTree shows the tree of pages, or closes it if it's showing,
// zooming in on the current page.
func (a *App) ToggleTree() {
	if a.tree != nil {
		a.closeTree(a.tree.start, func() {})
		return
	}
	a.ShowTree()
}

// OpenSelected opens the page selected in the tree of pages, zooming in on
// it, as opening its page URL does, and closes the tree. A no-op if the
// tree isn't showing.
func (a *App) OpenSelected() {
	t := a.tree
	if t == nil {
		return
	}
	id := t.selected
	if id == t.start {
		a.closeTree(id, func() {})
		return
	}
	u, err := url.Parse(string(t.document))
	if err != nil {
		log.Printf("opening a page of %s: %v", t.document, err)
		return
	}
	u.Fragment = t.pages[id].Passage + "@" + id
	a.closeTree(id, func() { a.follow(u) })
}

// selectPage selects page id in the tree view. Its frames slide to their
// new places from where they were; those it didn't show slide in with the
// page selected before.
func (a *App) selectPage(id string) {
	t := a.tree
	if id == t.selected {
		return
	}
	now := time.Now()
	from := map[string]image.Rectangle{}
	for _, f := range a.shownFrames(now) {
		from[f.id] = f.rect
	}
	old := t.selected
	t.selected = id
	var d image.Point
	if after, ok := a.treeRect(old); ok {
		d = after.Min.Sub(from[old].Min)
	}
	t.slide = treeSlide{from: from, d: d, start: now, length: treeSlideTime}
}

// treeKey handles k while the tree of pages shows, reporting whether it
// was one of the tree's keys.
func (a *App) treeKey(k Key) bool {
	t := a.tree
	if k.Shortcut || k.Shift || k.Alt {
		return false
	}
	if t.closing != nil {
		// Zooming in on a page: the tree is done with.
		return slices.Contains([]string{"Up", "Down", "Left", "Right", "Enter", "Escape"}, k.Name)
	}
	switch k.Name {
	case "Up":
		if parent := t.pages[t.selected].Parent; parent != "" {
			a.selectPage(parent)
		}
	case "Down":
		if child := t.preferredChild(t.selected); child != "" {
			a.selectPage(child)
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
			a.selectPage(siblings[i])
		}
	case "Enter":
		a.OpenSelected()
	case "Escape":
		a.closeTree(t.start, func() {})
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
// its children, with the child Down selects below it.
func (a *App) treeLayout() (parent []treeFrame, rows [][]treeFrame) {
	t := a.tree
	b := a.bounds
	gap := int(treeGap * a.deviceScale)
	w, h := b.Dx()/2, b.Dy()/2
	center := image.Pt(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)
	sel := image.Rect(center.X-w/2, center.Y-h/2, center.X-w/2+w, center.Y-h/2+h)
	p := t.pages[t.selected].Parent
	if p != "" {
		parent = []treeFrame{{p, sel.Sub(image.Pt(0, h+gap))}}
	}
	siblings := t.children[p]
	rows = append(rows, treeRow(siblings, slices.Index(siblings, t.selected), sel, gap))
	if child := t.preferredChild(t.selected); child != "" {
		children := t.children[t.selected]
		rows = append(rows, treeRow(children, slices.Index(children, child), sel.Add(image.Pt(0, h+gap)), gap))
	}
	return parent, rows
}

// treeFrames returns the frames of the tree view that overlap area, where
// they are once they've slid there.
func (a *App) treeFrames(area image.Rectangle) []treeFrame {
	parent, rows := a.treeLayout()
	var frames []treeFrame
	for _, f := range slices.Concat(append([][]treeFrame{parent}, rows...)...) {
		if f.rect.Overlaps(area) {
			frames = append(frames, f)
		}
	}
	return frames
}

// treeRect returns where page id's frame is in the tree view, once it's
// slid there, if it's shown at all, on screen or not.
func (a *App) treeRect(id string) (image.Rectangle, bool) {
	for _, f := range a.treeFrames(image.Rect(-1<<30, -1<<30, 1<<30, 1<<30)) {
		if f.id == id {
			return f.rect, true
		}
	}
	return image.Rectangle{}, false
}

// A treeSlide moves the frames of the tree view to their places after the
// selection changed, over a length of time, easing out: from where they
// were, or, for those that weren't shown, from d before their places.
type treeSlide struct {
	from   map[string]image.Rectangle
	d      image.Point
	start  time.Time
	length time.Duration
}

// moving reports whether s is still moving at now.
func (s treeSlide) moving(now time.Time) bool {
	return s.length > 0 && now.Before(s.start.Add(s.length))
}

// at returns where s has moved r, page id's frame once it's there, at now.
func (s treeSlide) at(id string, r image.Rectangle, now time.Time) image.Rectangle {
	if !s.moving(now) {
		return r
	}
	from, ok := s.from[id]
	if !ok {
		from = r.Sub(s.d)
	}
	f := float64(now.Sub(s.start)) / float64(s.length)
	f = 1 - (1-f)*(1-f)*(1-f)
	lerp := func(x, y int) int { return x + int(float64(y-x)*f) }
	return r.Add(image.Pt(lerp(from.Min.X, r.Min.X), lerp(from.Min.Y, r.Min.Y)).Sub(r.Min))
}

// shownFrames returns all the frames of the tree view, where they are at
// now, sliding.
func (a *App) shownFrames(now time.Time) []treeFrame {
	frames := a.treeFrames(image.Rect(-1<<30, -1<<30, 1<<30, 1<<30))
	for i, f := range frames {
		frames[i].rect = a.tree.slide.at(f.id, f.rect, now)
	}
	return frames
}

// treeLines returns the lines of the tree view, as rectangles to fill,
// between frames, which are where they're shown: one joining each row of
// siblings, and one from the selected page up to its parent and down to
// the child Down selects, bent while they're not in line.
func (a *App) treeLines(frames []treeFrame) []image.Rectangle {
	t := a.tree
	thick := max(1, int(a.deviceScale))
	at := map[string]image.Rectangle{}
	for _, f := range frames {
		at[f.id] = f.rect
	}
	var lines []image.Rectangle
	link := func(top, bottom image.Rectangle) {
		x1 := top.Min.X + top.Dx()/2 - thick/2
		x2 := bottom.Min.X + bottom.Dx()/2 - thick/2
		y := (top.Max.Y+bottom.Min.Y)/2 - thick/2
		lines = append(lines,
			image.Rect(x1, top.Max.Y, x1+thick, y+thick),
			image.Rect(min(x1, x2), y, max(x1, x2)+thick, y+thick),
			image.Rect(x2, y, x2+thick, bottom.Min.Y))
	}
	sel := at[t.selected]
	parent := t.pages[t.selected].Parent
	if parent != "" {
		link(at[parent], sel)
	}
	if child := t.preferredChild(t.selected); child != "" {
		link(sel, at[child])
	}
	for _, siblings := range [][]string{t.children[parent], t.children[t.selected]} {
		for i := 1; i < len(siblings); i++ {
			l, r := at[siblings[i-1]], at[siblings[i]]
			y := l.Min.Y + l.Dy()/2 - thick/2
			lines = append(lines, image.Rect(l.Max.X, y, r.Min.X, y+thick))
		}
	}
	return lines
}

// drawTree draws the tree view, through its camera.
func (a *App) drawTree(dst canvas.Canvas, now time.Duration) {
	t := a.tree
	background, line := a.stackColors()
	b := a.bounds
	dst.DrawRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), background)
	wall := time.Now()
	camera := t.camera.at(wall)
	shown := camera.Invert().ApplyRect(b)
	dst = dst.Clip(b).Transform(camera)
	frames := a.shownFrames(wall)
	for _, l := range a.treeLines(frames) {
		dst.DrawRect(l.Min.X, l.Min.Y, l.Dx(), l.Dy(), line)
	}
	thick := max(1, int(a.deviceScale))
	for _, f := range frames {
		if !f.rect.Overlaps(shown) {
			continue
		}
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
	for _, f := range a.treeFrames(a.bounds) {
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
	if a.tree.closing != nil {
		return
	}
	now := time.Now()
	camera := a.tree.camera.at(now).Invert()
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
		x, y := camera.Apply(float64(at.X), float64(at.Y))
		at = image.Pt(int(x), int(y))
		for _, f := range a.shownFrames(now) {
			if !at.In(f.rect) {
				continue
			}
			if f.id == a.tree.selected {
				a.OpenSelected()
				return
			}
			a.selectPage(f.id)
			return
		}
	}
}
