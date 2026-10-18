package browser

import (
	"image"
	"image/color"
	"slices"
	"time"

	"github.com/arnodel/whynot/canvas"
)

// The map of the tree of pages shows every page of the tree as a dot, in a
// corner of the tree view, laid out by the shape of the tree alone, so
// that it stays put as the selection moves: each page below its parent,
// the oldest of siblings on the left, and a page centred over its
// children.

// layoutMap places each page of t on the map: its x, counted in leaves
// from the left, and its depth.
func (t *tree) layoutMap() {
	t.mapX = map[string]float64{}
	t.mapDepth = map[string]int{}
	t.mapLeaves, t.mapRows = 0, 0
	var place func(id string, depth int) float64
	place = func(id string, depth int) float64 {
		t.mapDepth[id] = depth
		t.mapRows = max(t.mapRows, depth)
		children := t.children[id]
		var x float64
		if len(children) == 0 {
			x = float64(t.mapLeaves)
			t.mapLeaves++
		} else {
			first := place(children[0], depth+1)
			last := first
			for _, c := range children[1:] {
				last = place(c, depth+1)
			}
			x = (first + last) / 2
		}
		t.mapX[id] = x
		return x
	}
	for _, root := range t.children[""] {
		place(root, 0)
	}
}

// A treeMap is where the map of the tree of pages is on the window.
type treeMap struct {
	tree *tree
	// rect is the map, and area where its dots are.
	rect, area image.Rectangle
	// x0, y0 are where the map's x and depth 0 are, step and rowStep how
	// far apart its leaves and its rows are, and dot how big a dot is.
	x0, y0, step, rowStep float64
	dot                   int
}

// treeMapGeometry returns where the map of the tree of pages is, if it's
// shown: in the tree-map experiment, in a window big enough for it. A map
// too big for its space shows the part around the selected page.
func (a *App) treeMapGeometry() (treeMap, bool) {
	t := a.tree
	b := a.bounds
	s := a.deviceScale
	w, h := b.Dx()/4, b.Dy()/4
	if t == nil || !a.experiment("tree-map") || float64(w) < 120*s || float64(h) < 60*s {
		return treeMap{}, false
	}
	margin, pad := int(12*s), int(10*s)
	rect := image.Rect(b.Max.X-margin-w, b.Max.Y-margin-h, b.Max.X-margin, b.Max.Y-margin)
	m := treeMap{tree: t, rect: rect}
	m.area = rect.Inset(pad)
	// fit returns the step between n+1 places in size, at least least,
	// and where the first is: centred, or if they don't fit, with sel's in
	// the middle, as far as the ends allow.
	fit := func(n int, size, least, sel float64) (step, first float64) {
		step = 40 * s
		if n > 0 {
			step = min(40*s, max(least, size/float64(n)))
		}
		content := step * float64(n)
		if content <= size {
			return step, (size - content) / 2
		}
		return step, max(size-content, min(0, size/2-sel*step))
	}
	minStep := 6 * s
	var sx, sy float64
	m.step, sx = fit(t.mapLeaves-1, float64(m.area.Dx()), minStep, t.mapX[t.selected])
	m.rowStep, sy = fit(t.mapRows, float64(m.area.Dy()), minStep, float64(t.mapDepth[t.selected]))
	m.x0, m.y0 = float64(m.area.Min.X)+sx, float64(m.area.Min.Y)+sy
	m.dot = int(max(3*s, min(7*s, min(m.step, m.rowStep)/2)))
	return m, true
}

// at returns where page id's dot is centred.
func (m treeMap) at(id string) image.Point {
	return image.Pt(int(m.x0+m.tree.mapX[id]*m.step), int(m.y0+float64(m.tree.mapDepth[id])*m.rowStep))
}

// dotAt returns the page whose dot is nearest p, if it's near enough to
// point at, or "".
func (m treeMap) dotAt(p image.Point) string {
	if !p.In(m.area.Inset(-m.dot)) {
		return ""
	}
	reach := max(float64(m.dot), min(m.step, m.rowStep)/2)
	best, bestD := "", reach*reach
	for id := range m.tree.pages {
		d := m.at(id).Sub(p)
		if dd := float64(d.X*d.X + d.Y*d.Y); dd <= bestD {
			best, bestD = id, dd
		}
	}
	return best
}

// mapHovered returns the page whose dot the pointer is on, if the map is
// shown, or "".
func (a *App) mapHovered() string {
	m, ok := a.treeMapGeometry()
	if !ok || !a.hasPointer {
		return ""
	}
	return m.dotAt(a.pointer)
}

// drawTreeMap draws the map of the tree of pages on dst, the window, if
// it's shown, with a card for the page whose dot the pointer is on: its
// picture and its label.
func (a *App) drawTreeMap(dst canvas.Canvas, wall time.Time, now time.Duration) {
	m, ok := a.treeMapGeometry()
	if !ok {
		return
	}
	t := a.tree
	theme := a.treeTheme()
	accent := a.Panel.View().HighlightColor()
	thin := max(1, int(a.deviceScale))
	r := m.rect
	bg := color.NRGBAModel.Convert(theme.BackgroundColor).(color.NRGBA)
	bg.A = 0xE6
	dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), bg)
	frame(dst, r, thin, theme.ThematicBreakColor)

	in := dst.Clip(m.area.Inset(-m.dot))
	path := t.path()
	// The lines from each page with children down to them: down to the
	// middle of the gap, across, and down to each.
	for id, children := range t.children {
		if id == "" || len(children) == 0 {
			continue
		}
		p := m.at(id)
		mid := p.Y + int(m.rowStep/2)
		xs := []int{p.X}
		for _, c := range children {
			xs = append(xs, m.at(c).X)
		}
		fill(in, image.Rect(p.X, p.Y, p.X+thin, mid), theme.ThematicBreakColor)
		fill(in, image.Rect(slices.Min(xs), mid, slices.Max(xs)+thin, mid+thin), theme.ThematicBreakColor)
		for _, c := range children {
			q := m.at(c)
			fill(in, image.Rect(q.X, mid, q.X+thin, q.Y), theme.ThematicBreakColor)
		}
	}
	// The way back, over them.
	if a.experiment("tree-path") {
		w := 2 * thin
		for id := range path {
			parent := t.pages[id].Parent
			if parent == "" || !path[parent] {
				continue
			}
			p, q := m.at(parent), m.at(id)
			mid := p.Y + int(m.rowStep/2)
			fill(in, image.Rect(p.X-w/2, p.Y, p.X-w/2+w, mid), accent)
			fill(in, image.Rect(min(p.X, q.X)-w/2, mid-w/2, max(p.X, q.X)-w/2+w, mid-w/2+w), accent)
			fill(in, image.Rect(q.X-w/2, mid, q.X-w/2+w, q.Y), accent)
		}
	}
	// The dots: brighter for the pages the tiles show, the page the view
	// was opened from in the accent color, and the selected page ringed.
	shown := map[string]bool{}
	for _, f := range a.treeFrames(a.bounds) {
		shown[f.id] = true
	}
	hovered := m.dotAt(a.pointer)
	if !a.hasPointer {
		hovered = ""
	}
	for id := range t.pages {
		clr := theme.ThematicBreakColor
		if shown[id] {
			clr = theme.TextColor
		}
		if id == t.start && a.experiment("tree-path") {
			clr = accent
		}
		c := m.at(id)
		if id == t.selected || id == hovered {
			frame(in, image.Rect(c.X-m.dot, c.Y-m.dot, c.X+m.dot, c.Y+m.dot), thin, theme.TextColor)
		}
		d := m.dot / 2
		fill(in, image.Rect(c.X-d, c.Y-d, c.X-d+m.dot, c.Y-d+m.dot), clr)
	}
	if hovered == "" {
		return
	}
	// The card: above the map, the size of a frame scaled to its width.
	gap := int(8 * a.deviceScale)
	h := r.Dx() * a.bounds.Dy() / max(1, a.bounds.Dx())
	card := image.Rect(r.Min.X, r.Min.Y-gap-h, r.Max.X, r.Min.Y-gap)
	if card.Min.Y < a.bounds.Min.Y {
		return // no room
	}
	fill(dst, card, theme.BackgroundColor)
	a.drawThumb(dst, hovered, card, now)
	frame(dst, card, thin, theme.TextColor)
	a.drawTreeLabel(dst, hovered, card, wall)
}

// fill fills r on dst with clr.
func fill(dst canvas.Canvas, r image.Rectangle, clr color.Color) {
	r = r.Canon()
	dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), clr)
}

// frame draws a frame thick wide just inside r on dst, in clr.
func frame(dst canvas.Canvas, r image.Rectangle, thick int, clr color.Color) {
	fill(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+thick), clr)
	fill(dst, image.Rect(r.Min.X, r.Max.Y-thick, r.Max.X, r.Max.Y), clr)
	fill(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+thick, r.Max.Y), clr)
	fill(dst, image.Rect(r.Max.X-thick, r.Min.Y, r.Max.X, r.Max.Y), clr)
}
