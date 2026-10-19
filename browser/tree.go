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
	// moves are the pages selected before, oldest first, for Back to
	// return to, and undone the moves Back undid, most recent last, for
	// Forward to make again.
	moves, undone []string
	// lastChild is the child of each page last selected, by the page's
	// id, so that going up keeps the row of children where it was (see
	// preferredChild).
	lastChild map[string]string
	// mapX and mapDepth place each page on the map (see layoutMap), which
	// is mapLeaves wide and mapRows deep.
	mapX               map[string]float64
	mapDepth           map[string]int
	mapHeat            map[string]float64 // see layoutMap
	mapByAge           []string           // the pages, oldest first
	mapLeaves, mapRows int
	// focusX and focusY are where the map's corner is centred, following
	// the selected page.
	focusX, focusY treeEase
	// mapGrow is how far the map is expanded, from 0 to 1 (see expandMap).
	mapGrow treeEase
	// mapK is the expanded map's scale, relative to the corner's, mapCX
	// and mapCY where its middle is, and mapPress the press on it, if any.
	mapK, mapCX, mapCY float64
	mapPress           *mapPress
	// views are the pages' Views, by id, as they're shown in their
	// frames: the page's own if this window has shown it, or else loaded
	// (see loadThumbs), with a stand-in until then.
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

// easeOut returns how far a move that started at start and lasts length
// has gone at now, from 0 to 1, slowing down towards its end, and whether
// it's still going.
func easeOut(start time.Time, length time.Duration, now time.Time) (float64, bool) {
	if length <= 0 || !now.Before(start.Add(length)) {
		return 1, false
	}
	f := float64(now.Sub(start)) / float64(length)
	return 1 - (1-f)*(1-f)*(1-f), true
}

// A treeCamera moves the tree view from one transform to another, over a
// length of time, easing out.
type treeCamera struct {
	from, to canvas.Transform
	start    time.Time
	length   time.Duration
}

// at returns c's transform at now.
func (c treeCamera) at(now time.Time) canvas.Transform {
	f, moving := easeOut(c.start, c.length, now)
	if !moving {
		return c.to
	}
	lerp := func(x, y float64) float64 { return x + (y-x)*f }
	return canvas.Transform{
		Scale: lerp(c.from.Scale, c.to.Scale),
		X:     lerp(c.from.X, c.to.X),
		Y:     lerp(c.from.Y, c.to.Y),
	}
}

// moving reports whether c is still moving at now.
func (c treeCamera) moving(now time.Time) bool {
	_, moving := easeOut(c.start, c.length, now)
	return moving
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
// middle, and Enter opens the selected page (see openSelected). A no-op if
// the current document isn't a page of a whymark document.
func (a *App) ShowTree() {
	p := a.treePage()
	if p == nil || a.tree != nil {
		return
	}
	a.hideOverlay()
	t := &tree{
		document:  whymark.DocumentOf(p.header.Location),
		selected:  p.header.PageID,
		start:     p.header.PageID,
		views:     map[string]*whynot.View{},
		loading:   map[string]bool{},
		lastChild: map[string]string{},
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
	t.rankPages()
	t.layoutMap()
	t.focusMap(0)
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

// openSelected opens the page selected in the tree of pages, zooming in on
// it, as opening its page URL does, and closes the tree. A no-op if the
// tree isn't showing.
func (a *App) openSelected() {
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
	t.lastChild[t.pages[id].Parent] = id
	t.focusMap(treeSlideTime)
	if t.mapGrow.to == 1 {
		a.keepSelectedOnMap()
	}
	var d image.Point
	if after, ok := a.treeRect(old); ok {
		d = after.Min.Sub(from[old].Min)
	}
	t.slide = treeSlide{from: from, d: d, start: now, length: treeSlideTime}
}

// movePage selects page id in the tree view, as a move the reader made,
// which Back undoes.
func (a *App) movePage(id string) {
	t := a.tree
	if id == t.selected {
		return
	}
	t.moves = append(t.moves, t.selected)
	t.undone = nil
	a.selectPage(id)
}

// treeBack undoes the last move in the tree view, or closes it, zooming in
// on the page it was opened from, if there's none to undo.
func (a *App) treeBack() {
	t := a.tree
	if t.closing != nil {
		return
	}
	if len(t.moves) == 0 {
		a.closeTree(t.start, func() {})
		return
	}
	prev := t.moves[len(t.moves)-1]
	t.moves = t.moves[:len(t.moves)-1]
	t.undone = append(t.undone, t.selected)
	a.selectPage(prev)
}

// treeForward makes the last move Back undid again.
func (a *App) treeForward() {
	t := a.tree
	if t.closing != nil || len(t.undone) == 0 {
		return
	}
	next := t.undone[len(t.undone)-1]
	t.undone = t.undone[:len(t.undone)-1]
	t.moves = append(t.moves, t.selected)
	a.selectPage(next)
}

// treeKeys are the keys only the tree of pages handles (see treeKey and
// experimentKey), for a program to watch for as it does the commands'.
func treeKeys() []Key {
	return append([]Key{{Name: "Enter"}, {Name: "M"}}, experimentKeys()...)
}

// treeKey handles k while the tree of pages shows, reporting whether it
// was one of the tree's keys.
func (a *App) treeKey(k Key) bool {
	t := a.tree
	if a.mapExpanded() && k.Shortcut && slices.Contains([]string{"=", "-", "0"}, k.Name) {
		// Zooming the map, around the selected page, rather than pages.
		if m, ok := a.treeMapGeometry(); ok {
			switch k.Name {
			case "=":
				a.zoomMap(1.25, m.at(t.selected))
			case "-":
				a.zoomMap(1/1.25, m.at(t.selected))
			default:
				_, _, t.mapK = a.mapScales()
				a.keepSelectedOnMap()
			}
		}
		return true
	}
	if k.Shortcut || k.Shift || k.Alt {
		return false
	}
	if t.closing != nil {
		// Zooming in on a page: the tree is done with.
		return slices.Contains([]string{"Up", "Down", "Left", "Right", "Enter", "Escape"}, k.Name)
	}
	if a.mapExpanded() && (k.Name == "Enter" || k.Name == "Escape") {
		a.expandMap(false)
		return true
	}
	switch k.Name {
	case "M":
		if _, ok := a.treeMapGeometry(); ok {
			a.expandMap(!a.mapExpanded())
		}
	case "Up":
		if parent := t.pages[t.selected].Parent; parent != "" {
			a.movePage(parent)
		}
	case "Down":
		if child := t.preferredChild(t.selected); child != "" {
			a.movePage(child)
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
			a.movePage(siblings[i])
		}
	case "Enter":
		a.openSelected()
	case "Escape":
		a.closeTree(t.start, func() {})
	default:
		return false
	}
	return true
}

// preferredChild returns the child of id Down selects, and the row of its
// children is lined up on: the one last selected, so that going up and
// down again changes nothing, if any; or else the one on the way back to
// the page the view was opened from, if any; or else the one visited most
// recently, or else the newest. It's "" without children.
func (t *tree) preferredChild(id string) string {
	children := t.children[id]
	if len(children) == 0 {
		return ""
	}
	if c, ok := t.lastChild[id]; ok && slices.Contains(children, c) {
		return c
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
	var ids []string
	for _, f := range a.treeFrames() {
		if f.rect.Overlaps(a.bounds) {
			ids = append(ids, f.id)
		}
	}
	if id := a.mapHovered(); id != "" {
		ids = append(ids, id) // for the map's card
	}
	for _, id := range ids {
		if _, ok := t.views[id]; ok || t.loading[id] {
			continue
		}
		t.loading[id] = true
		engine, notify := a.engine, a.OnDocumentChange
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
// selects it, a click on the selected page opens it, and a click on the
// map in the corner expands it (see mapInput for the expanded map).
func (a *App) treeInput(events []input.Event) {
	// The pointer may now be on a dot of the map whose page isn't loaded:
	// a program that draws only when asked has no Update to start it.
	defer a.loadThumbs()
	if a.tree.closing != nil {
		return
	}
	now := time.Now()
	camera := a.tree.camera.at(now).Invert()
	for _, e := range events {
		if a.mapExpanded() {
			a.mapInput(e)
			continue
		}
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
		if m, ok := a.treeMapGeometry(); ok && at.In(m.rect) {
			a.expandMap(true) // the map is over the tiles
			return
		}
		x, y := camera.Apply(float64(at.X), float64(at.Y))
		at = image.Pt(int(x), int(y))
		for _, f := range a.shownFrames(now) {
			if !at.In(f.rect) {
				continue
			}
			if f.id == a.tree.selected {
				a.openSelected()
				return
			}
			a.movePage(f.id)
			return
		}
	}
}
