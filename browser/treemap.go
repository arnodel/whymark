package browser

import (
	"image"
	"image/color"
	"slices"
	"time"

	"github.com/arnodel/whynot/canvas"
)

// The map of the tree of pages shows its pages as dots, laid out by the
// shape of the tree alone, so that it stays put as the selection moves:
// each page below its parent, the oldest of siblings on the left, and a
// page centred over its children. In a corner of the tree view, it shows
// the part of the tree around the selected page; expanded, it fills the
// window, with the whole tree, and a box where the corner's part is.

// A treeEase moves a number from one value to another, over a length of
// time, easing out.
type treeEase struct {
	from, to float64
	start    time.Time
	length   time.Duration
}

// at returns e's value at now.
func (e treeEase) at(now time.Time) float64 {
	if !e.moving(now) {
		return e.to
	}
	f := float64(now.Sub(e.start)) / float64(e.length)
	f = 1 - (1-f)*(1-f)*(1-f)
	return e.from + (e.to-e.from)*f
}

// moving reports whether e is still moving at now.
func (e treeEase) moving(now time.Time) bool {
	return e.length > 0 && now.Before(e.start.Add(e.length))
}

// ease returns e moving on, from where it is at now, to to, over length.
func (e treeEase) ease(to float64, now time.Time, length time.Duration) treeEase {
	return treeEase{from: e.at(now), to: to, start: now, length: length}
}

// focusMap makes the map's focus, which the corner's part is centred on,
// the selected page: moving there over length.
func (t *tree) focusMap(length time.Duration) {
	now := time.Now()
	t.focusX = t.focusX.ease(t.mapX[t.selected], now, length)
	t.focusY = t.focusY.ease(float64(t.mapDepth[t.selected]), now, length)
}

// MapExpanded reports whether the map of the tree of pages is expanded.
func (a *App) MapExpanded() bool { return a.tree != nil && a.tree.mapGrow.to == 1 }

// expandMap expands the map of the tree of pages to fill the window, or
// shrinks it back to its corner, growing or shrinking smoothly.
func (a *App) expandMap(expand bool) {
	t := a.tree
	to := 0.0
	if expand {
		to = 1
	}
	if t.mapGrow.to != to {
		t.mapGrow = t.mapGrow.ease(to, time.Now(), treeZoomTime)
	}
}

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
	// grown is how far it's expanded, from 0, in its corner, to 1, and
	// box, in its own x and depth, the part the corner shows.
	grown float64
	box   struct{ x0, y0, x1, y1 float64 }
}

// treeMapGeometry returns where the map of the tree of pages is, if it's
// shown: in the tree-map experiment, in a window big enough for it. In its
// corner, it shows the part around its focus, at a fixed scale; expanded,
// the whole tree, or the part around the selected page, if it's too big.
func (a *App) treeMapGeometry() (treeMap, bool) {
	t := a.tree
	b := a.bounds
	s := a.deviceScale
	w, h := b.Dx()/4, b.Dy()/4
	if t == nil || !a.experiment("tree-map") || float64(w) < 120*s || float64(h) < 60*s {
		return treeMap{}, false
	}
	now := time.Now()
	margin, pad := int(12*s), int(10*s)
	corner := image.Rect(b.Max.X-margin-w, b.Max.Y-margin-h, b.Max.X-margin, b.Max.Y-margin)
	full := b.Inset(margin)
	m := treeMap{tree: t, grown: t.mapGrow.at(now)}
	lerp := func(x, y float64) float64 { return x + (y-x)*m.grown }
	m.rect = image.Rect(
		int(lerp(float64(corner.Min.X), float64(full.Min.X))), int(lerp(float64(corner.Min.Y), float64(full.Min.Y))),
		int(lerp(float64(corner.Max.X), float64(full.Max.X))), int(lerp(float64(corner.Max.Y), float64(full.Max.Y))))
	m.area = m.rect.Inset(pad)

	// In the corner: a fixed scale, centred on the focus.
	fx, fy := t.focusX.at(now), t.focusY.at(now)
	small := corner.Inset(pad)
	step, rowStep := 16*s, 22*s
	x0 := float64(small.Min.X+small.Dx()/2) - fx*step
	y0 := float64(small.Min.Y+small.Dy()/2) - fy*rowStep
	halfW, halfH := float64(small.Dx())/2/step, float64(small.Dy())/2/rowStep
	m.box.x0, m.box.y0, m.box.x1, m.box.y1 = fx-halfW, fy-halfH, fx+halfW, fy+halfH
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
	// Expanded: the whole tree, as far as it fits.
	big := full.Inset(pad)
	bigStep, sx := fit(t.mapLeaves-1, float64(big.Dx()), 6*s, fx)
	bigRowStep, sy := fit(t.mapRows, float64(big.Dy()), 6*s, fy)
	m.step, m.rowStep = lerp(step, bigStep), lerp(rowStep, bigRowStep)
	m.x0, m.y0 = lerp(x0, float64(big.Min.X)+sx), lerp(y0, float64(big.Min.Y)+sy)
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
	bg.A = uint8(0xE6 + (0xFF-0xE6)*m.grown) // opaque, expanded
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
	// The dots: the page the view was opened from in the accent color,
	// and the selected page ringed.
	hovered := m.dotAt(a.pointer)
	if !a.hasPointer {
		hovered = ""
	}
	for id := range t.pages {
		clr := theme.ThematicBreakColor
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
	// Expanding, the box around the part the corner shows, which is the
	// corner's own edge as it starts to grow.
	if m.grown > 0 {
		box := image.Rect(
			int(m.x0+m.box.x0*m.step), int(m.y0+m.box.y0*m.rowStep),
			int(m.x0+m.box.x1*m.step), int(m.y0+m.box.y1*m.rowStep)).Intersect(m.area)
		frame(dst, box, thin, theme.TextColor)
	}

	if hovered == "" {
		return
	}
	// The card: a frame's size scaled to a quarter of the window's width,
	// above the map in its corner, or else beside the pointer.
	b := a.bounds
	gap := int(8 * a.deviceScale)
	w := b.Dx() / 4
	h := w * b.Dy() / max(1, b.Dx())
	card := image.Rect(r.Max.X-w, r.Min.Y-gap-h, r.Max.X, r.Min.Y-gap)
	if m.grown > 0 {
		p := a.pointer.Add(image.Pt(2*gap, 2*gap))
		if p.X+w > b.Max.X {
			p.X = a.pointer.X - 2*gap - w
		}
		if p.Y+h > b.Max.Y {
			p.Y = a.pointer.Y - 2*gap - h
		}
		card = image.Rect(p.X, p.Y, p.X+w, p.Y+h)
	}
	if !card.In(b) {
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
