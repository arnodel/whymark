package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"

	"github.com/arnodel/whymark"
	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/styles/simpletheme"
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
	t.lastChild[t.pages[id].Parent] = id
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
		a.OpenSelected()
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

// treeLayout returns the frames of the tree view: the row of the selected
// page's parent and its siblings, the row of the selected page and its
// siblings, and the row of its children, with the child Down selects
// below it.
func (a *App) treeLayout() (parent []treeFrame, rows [][]treeFrame) {
	t := a.tree
	b := a.bounds
	gap := int(treeGap * a.deviceScale)
	w, h := b.Dx()/2, b.Dy()/2
	center := image.Pt(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)
	sel := image.Rect(center.X-w/2, center.Y-h/2, center.X-w/2+w, center.Y-h/2+h)
	p := t.pages[t.selected].Parent
	if p != "" {
		r := sel.Sub(image.Pt(0, h+gap))
		aunts := t.children[t.pages[p].Parent]
		if k := slices.Index(aunts, p); k >= 0 {
			parent = treeRow(aunts, k, r, gap)
		} else {
			parent = []treeFrame{{p, r}} // not kept by the engine
		}
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

// path returns the pages on the tree's path from the page the view was
// opened from to the selected one: up to the first page they both come
// from, and down again.
func (t *tree) path() map[string]bool {
	ancestors := map[string]bool{}
	for p := t.start; p != ""; p = t.pages[p].Parent {
		ancestors[p] = true
	}
	path := map[string]bool{}
	common := ""
	for p := t.selected; p != ""; p = t.pages[p].Parent {
		path[p] = true
		if ancestors[p] {
			common = p
			break
		}
	}
	for p := t.start; p != common && p != ""; p = t.pages[p].Parent {
		path[p] = true
	}
	return path
}

// shownPath returns the pages of the tree's path from the page the view
// was opened from (see tree.path) that the tree view shows on it, if the
// tree-path experiment is on: a path through the parent to a sibling goes
// along the row's line, which joins siblings, so the parent is on it only
// if it goes on up.
func (a *App) shownPath() map[string]bool {
	if !a.experiment("tree-path") {
		return nil
	}
	t := a.tree
	path := t.path()
	// The same goes for the parent's row, through the grandparent, which
	// isn't shown.
	for _, id := range []string{t.selected, t.pages[t.selected].Parent} {
		parent := t.pages[id].Parent
		if id != "" && slices.ContainsFunc(t.children[parent], func(s string) bool { return s != id && path[s] }) {
			delete(path, parent)
		}
	}
	return path
}

// treeLines returns the lines of the tree view, as rectangles to fill,
// between frames, which are where they're shown: one joining each row of
// siblings, and one from the selected page up to its parent and down to
// the child Down selects, bent while they're not in line. Those on the
// tree's path from the page the view was opened from (see tree.path) are
// in path instead, thicker, if the tree-path experiment is on.
func (a *App) treeLines(frames []treeFrame) (lines, path []image.Rectangle) {
	t := a.tree
	thin := max(1, int(a.deviceScale))
	at := map[string]image.Rectangle{}
	for _, f := range frames {
		at[f.id] = f.rect
	}
	on := a.shownPath()
	// add adds a line to lines, or to path if it's on, as thick as it
	// then is: thick is a function of it, for rectangles centred on it.
	add := func(onPath bool, line func(thick int) []image.Rectangle) {
		if onPath {
			path = append(path, line(3*thin)...)
		} else {
			lines = append(lines, line(thin)...)
		}
	}
	link := func(onPath bool, top, bottom image.Rectangle) {
		add(onPath, func(thick int) []image.Rectangle {
			x1 := top.Min.X + top.Dx()/2 - thick/2
			x2 := bottom.Min.X + bottom.Dx()/2 - thick/2
			y := (top.Max.Y+bottom.Min.Y)/2 - thick/2
			return []image.Rectangle{
				image.Rect(x1, top.Max.Y, x1+thick, y+thick),
				image.Rect(min(x1, x2), y, max(x1, x2)+thick, y+thick),
				image.Rect(x2, y, x2+thick, bottom.Min.Y),
			}
		})
	}
	// Along a row, the path goes from the page lined up under the line
	// above, the parent, the selected page or the child under it, to the
	// page on it.
	along := func(row []string, from string) (k, target int) {
		k = slices.Index(row, from)
		return k, slices.IndexFunc(row, func(id string) bool { return id != from && on[id] })
	}
	sel := at[t.selected]
	parent := t.pages[t.selected].Parent
	siblings, children := t.children[parent], t.children[t.selected]
	var aunts []string
	if parent != "" {
		aunts = t.children[t.pages[parent].Parent]
	}
	child := t.preferredChild(t.selected)
	ak, atarget := along(aunts, parent)
	k, target := along(siblings, t.selected)
	ck, ctarget := along(children, child)
	if parent != "" {
		link(on[parent], at[parent], sel)
	}
	if child != "" {
		link(on[child] || ctarget >= 0, sel, at[child])
	}
	for _, row := range []struct {
		ids       []string
		k, target int
	}{{aunts, ak, atarget}, {siblings, k, target}, {children, ck, ctarget}} {
		siblings, k, target := row.ids, row.k, row.target
		for i := 1; i < len(siblings); i++ {
			l, r := at[siblings[i-1]], at[siblings[i]]
			onPath := k >= 0 && target >= 0 && i > min(k, target) && i <= max(k, target)
			add(onPath, func(thick int) []image.Rectangle {
				y := l.Min.Y + l.Dy()/2 - thick/2
				return []image.Rectangle{image.Rect(l.Max.X, y, r.Min.X, y+thick)}
			})
		}
	}
	return lines, path
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
	window := dst.Clip(b)
	dst = window.Transform(camera)
	frames := a.shownFrames(wall)
	lines, path := a.treeLines(frames)
	for _, l := range lines {
		dst.DrawRect(l.Min.X, l.Min.Y, l.Dx(), l.Dy(), line)
	}
	on := a.shownPath()
	accent := a.Panel.View().HighlightColor()
	for _, l := range path {
		dst.DrawRect(l.Min.X, l.Min.Y, l.Dx(), l.Dy(), accent)
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
		// The pages on the path are framed as it is, and the page the view
		// was opened from, where it leads, thicker.
		clr, frame := line, thick
		if on[f.id] && len(on) > 1 {
			clr = accent
		}
		if f.id == t.start && on[f.id] {
			frame = 3 * thick
		}
		r := f.rect.Inset(-frame)
		dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), frame, clr)
		dst.DrawRect(r.Min.X, r.Max.Y-frame, r.Dx(), frame, clr)
		dst.DrawRect(r.Min.X, r.Min.Y, frame, r.Dy(), clr)
		dst.DrawRect(r.Max.X-frame, r.Min.Y, frame, r.Dy(), clr)
	}
	if a.experiment("tree-labels") {
		for _, f := range frames {
			if f.rect.Overlaps(shown) {
				a.drawTreeLabel(window, f.id, camera.ApplyRect(f.rect), wall)
			}
		}
	}
}

// drawTreeLabel draws the label of page id on its frame, r on the window,
// in a bar along the bottom of what's on screen of it, at the size of the
// window's text whatever the frame's: what was typed for the page, or else
// its title, and how long ago it was made.
func (a *App) drawTreeLabel(dst canvas.Canvas, id string, r image.Rectangle, now time.Time) {
	face, err := a.faceSelector.SelectFace(fonts.TextStyle{Size: 13}, a.deviceScale*72)
	if err != nil {
		return
	}
	theme := simpletheme.Light()
	if a.darkTheme {
		theme = simpletheme.Dark()
	}
	pad := int(6 * a.deviceScale)
	m := face.Metrics()
	h := (m.Ascent + m.Descent).Ceil() + 2*pad
	r = r.Intersect(a.bounds)
	if r.Dx() < 4*h || r.Dy() < 2*h {
		return // too small to say anything
	}
	bar := image.Rect(r.Min.X, r.Max.Y-h, r.Max.X, r.Max.Y)
	dst.DrawRect(bar.Min.X, bar.Min.Y, bar.Dx(), bar.Dy(), theme.BackgroundColor)
	dst.DrawRect(bar.Min.X, bar.Min.Y, bar.Dx(), max(1, int(a.deviceScale)), theme.ThematicBreakColor)
	info := a.tree.pages[id]
	age := ageOf(info.Time, now)
	ageX := bar.Max.X - pad - font.MeasureString(face, age).Ceil()
	baseline := baselineIn(face, bar)
	dst.DrawText(age, face, ageX, baseline, theme.ThematicBreakColor)
	label := truncateEnd(face, treeLabel(info), ageX-pad-(bar.Min.X+pad))
	dst.DrawText(label, face, bar.Min.X+pad, baseline, theme.TextColor)
}

// treeLabel returns what tells page p apart in the tree of pages: what was
// typed for it, or else its title.
func treeLabel(p whymark.PageInfo) string {
	if len(p.Input) > 0 {
		names := slices.Sorted(maps.Keys(p.Input))
		var parts []string
		for _, name := range names {
			value := strings.Join(strings.Fields(p.Input[name]), " ")
			if len(names) > 1 {
				value = name + ": " + value
			}
			parts = append(parts, value)
		}
		return "“" + strings.Join(parts, " · ") + "”"
	}
	if p.Title != "" {
		return p.Title
	}
	return "Untitled page"
}

// ageOf returns how long before now t was, as a person would say it, or ""
// for the zero time.
func ageOf(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case t.IsZero():
		return ""
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
	return t.Format("2 Jan 2006")
}

// truncateEnd returns s, cut short with an ellipsis if it's wider than
// width in face.
func truncateEnd(face font.Face, s string, width int) string {
	if font.MeasureString(face, s).Ceil() <= width {
		return s
	}
	runes := []rune(s)
	for n := len(runes) - 1; n > 0; n-- {
		if t := string(runes[:n]) + "…"; font.MeasureString(face, t).Ceil() <= width {
			return t
		}
	}
	return ""
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
			a.movePage(f.id)
			return
		}
	}
}
