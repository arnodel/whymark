package browser

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"math"
	"slices"
	"time"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
)

// The map of the tree of pages shows its pages as dots, laid out by the
// shape of the tree alone, so that it stays put as the selection moves:
// each page below its parent, the oldest of siblings on the left, and a
// page centred over its children. In a corner of the tree view, it shows
// the part of the tree around the selected page; expanded, it fills the
// document, where it pans and zooms, with a box where the corner's part
// is.

// A treeEase moves a number from one value to another, over a length of
// time, easing out.
type treeEase struct {
	from, to float64
	start    time.Time
	length   time.Duration
}

// at returns e's value at now.
func (e treeEase) at(now time.Time) float64 {
	f, _ := easeOut(e.start, e.length, now)
	return e.from + (e.to-e.from)*f
}

// moving reports whether e is still moving at now.
func (e treeEase) moving(now time.Time) bool {
	_, moving := easeOut(e.start, e.length, now)
	return moving
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

// mapExpanded reports whether the map of the tree of pages is expanded.
func (a *App) mapExpanded() bool { return a.tree != nil && a.tree.mapGrow.to == 1 }

// expandMap expands the map of the tree of pages to fill the window, or
// shrinks it back to its corner, growing or shrinking smoothly.
func (a *App) expandMap(expand bool) {
	t := a.tree
	to := 0.0
	if expand {
		to = 1
	}
	if t.mapGrow.to == to {
		return
	}
	if expand && t.mapGrow.at(time.Now()) == 0 {
		// It opens showing the whole tree, if it's readable so, or else
		// readably, around the selected page.
		least, _, open := a.mapScales()
		t.mapK, t.mapCX, t.mapCY = open, t.mapX[t.selected], float64(t.mapDepth[t.selected])
		if open == least {
			t.mapCX, t.mapCY = float64(t.mapLeaves-1)/2, float64(t.mapRows)/2
		}
		a.clampMap()
	}
	t.mapGrow = t.mapGrow.ease(to, time.Now(), treeZoomTime)
}

// The expanded map's scale is relative to the corner's, whose leaves are
// mapStep apart, and rows mapRowStep, in logical pixels.
const mapStep, mapRowStep = 16, 22

// mapScales returns the scales the expanded map zooms between, least to
// show the whole tree and most to show it no more than one and a half
// times as big as the corner does, and the one it opens at: as little as
// that, but no less than readable.
func (a *App) mapScales() (least, most, open float64) {
	t := a.tree
	big := a.mapArea()
	most = 1.5 // expanding shows more, not just bigger
	least = most
	if n := t.mapLeaves - 1; n > 0 {
		least = min(least, float64(big.Dx())/(mapStep*a.deviceScale*float64(n)))
	}
	if n := t.mapRows; n > 0 {
		least = min(least, float64(big.Dy())/(mapRowStep*a.deviceScale*float64(n)))
	}
	const readable = 0.75
	return least, most, min(most, max(least, readable))
}

// mapSteps returns how far apart the expanded map's leaves and rows are,
// in pixels, at its scale.
func (a *App) mapSteps() (step, rowStep float64) {
	k := a.tree.mapK * a.deviceScale
	return k * mapStep, k * mapRowStep
}

// mapArea returns where the expanded map's dots are.
func (a *App) mapArea() image.Rectangle {
	s := a.deviceScale
	return a.bounds.Inset(int(10 * s))
}

// clampMap keeps the expanded map's scale between its least and most, and
// its middle where the tree stays in view: in the middle of the tree in a
// direction it fits in.
func (a *App) clampMap() {
	t := a.tree
	least, most, _ := a.mapScales()
	t.mapK = max(least, min(most, t.mapK))
	big := a.mapArea()
	clamp := func(c float64, n int, size, step float64) float64 {
		half := size / 2 / step
		if float64(n) <= 2*half {
			return float64(n) / 2
		}
		return max(half, min(float64(n)-half, c))
	}
	step, rowStep := a.mapSteps()
	t.mapCX = clamp(t.mapCX, max(t.mapLeaves-1, 0), float64(big.Dx()), step)
	t.mapCY = clamp(t.mapCY, t.mapRows, float64(big.Dy()), rowStep)
}

// panMap moves the expanded map's view by dx, dy in pixels: towards the
// tree's right and bottom for positive ones.
func (a *App) panMap(dx, dy float64) {
	t := a.tree
	step, rowStep := a.mapSteps()
	t.mapCX += dx / step
	t.mapCY += dy / rowStep
	a.clampMap()
}

// zoomMap zooms the expanded map by factor, keeping where p is on it in
// place.
func (a *App) zoomMap(factor float64, p image.Point) {
	t := a.tree
	big := a.mapArea()
	dx := float64(p.X) - float64(big.Min.X+big.Dx()/2)
	dy := float64(p.Y) - float64(big.Min.Y+big.Dy()/2)
	// Where p is, in the map's x and depth, stays where it is.
	step, rowStep := a.mapSteps()
	ux, uy := t.mapCX+dx/step, t.mapCY+dy/rowStep
	least, most, _ := a.mapScales()
	t.mapK = max(least, min(most, t.mapK*factor))
	step, rowStep = a.mapSteps()
	t.mapCX, t.mapCY = ux-dx/step, uy-dy/rowStep
	a.clampMap()
}

// keepSelectedOnMap pans the expanded map, if need be, so that the
// selected page is in view, with a margin.
func (a *App) keepSelectedOnMap() {
	t := a.tree
	big := a.mapArea()
	keep := func(c, u, size, step float64) float64 {
		half := max(0, size/2/step-1) // a step's margin
		return max(u-half, min(u+half, c))
	}
	step, rowStep := a.mapSteps()
	t.mapCX = keep(t.mapCX, t.mapX[t.selected], float64(big.Dx()), step)
	t.mapCY = keep(t.mapCY, float64(t.mapDepth[t.selected]), float64(big.Dy()), rowStep)
	a.clampMap()
}

// A mapPress is a press on the expanded map, which is a tap until it has
// moved far enough to be a drag, which pans it.
type mapPress struct {
	touch       int // the touch's ID, or -1 for the mouse
	start, last image.Point
	dragged     bool
}

// mapInput handles e on the expanded map: the wheel pans it, or zooms it
// with the shortcut modifier; a drag pans it; a tap on a dot selects the
// page, shrinking it back.
func (a *App) mapInput(e input.Event) {
	m, ok := a.treeMapGeometry()
	if !ok {
		return
	}
	t := a.tree
	shortcut := input.ModCtrl
	if macOS {
		shortcut = input.ModMeta
	}
	press := func(touch int, p image.Point) {
		if !p.In(m.rect) {
			return // the toolbar's, or the dock's
		}
		t.mapPress = &mapPress{touch: touch, start: p, last: p}
	}
	move := func(touch int, p image.Point) {
		pr := t.mapPress
		if pr == nil || pr.touch != touch {
			return
		}
		if d := p.Sub(pr.start); !pr.dragged && float64(d.X*d.X+d.Y*d.Y) > 16*a.deviceScale*a.deviceScale {
			pr.dragged = true
		}
		if pr.dragged {
			a.panMap(float64(pr.last.X-p.X), float64(pr.last.Y-p.Y))
			pr.last = p
		}
	}
	release := func(touch int, p image.Point) {
		pr := t.mapPress
		if pr == nil || pr.touch != touch {
			return
		}
		t.mapPress = nil
		if !pr.dragged {
			if id := m.dotAt(pr.start); id != "" {
				a.movePage(id)
				a.expandMap(false)
			}
		}
	}
	switch e := e.(type) {
	case input.Wheel:
		if e.Mods.Contain(shortcut) {
			a.zoomMap(math.Pow(2, -e.DY/200), image.Pt(e.X, e.Y))
			return
		}
		dx, dy := e.DX, e.DY
		if e.Mods.Contain(input.ModShift) && dx == 0 {
			dx, dy = dy, 0 // a mouse's wheel, sideways
		}
		a.panMap(dx*a.deviceScale, dy*a.deviceScale)
	case input.PointerButton:
		if e.Button != input.ButtonPrimary {
			return
		}
		if e.Down {
			press(-1, image.Pt(e.X, e.Y))
		} else {
			release(-1, image.Pt(e.X, e.Y))
		}
	case input.PointerMove:
		move(-1, image.Pt(e.X, e.Y))
	case input.TouchStart:
		press(e.ID, image.Pt(e.X, e.Y))
	case input.TouchMove:
		move(e.ID, image.Pt(e.X, e.Y))
	case input.TouchEnd:
		if t.mapPress != nil {
			release(e.ID, t.mapPress.last)
		}
	case input.TouchCancel:
		t.mapPress = nil
	}
}

// rankPages ranks the pages of t by when they were made, giving each its
// heat on the map: from 0 for the oldest to 1 for the newest, by rank, so
// that a few pages much older or newer than the rest don't make them all
// alike.
func (t *tree) rankPages() {
	t.mapByAge = slices.SortedFunc(maps.Keys(t.pages), func(x, y string) int {
		return t.pages[x].Time.Compare(t.pages[y].Time)
	})
	t.mapHeat = map[string]float64{}
	for i, id := range t.mapByAge {
		t.mapHeat[id] = 1
		if n := len(t.mapByAge); n > 1 {
			t.mapHeat[id] = float64(i) / float64(n-1)
		}
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
	// icon is whether the corner shows only an icon, in rect: in a window
	// too small for the map, or in the tree-map-icon experiment.
	icon bool
	// grown is how far it's expanded, from 0, in its corner, to 1.
	grown float64
	// box is the part the corner shows, in the map's x and depth.
	box struct{ x0, y0, x1, y1 float64 }
}

// treeMapGeometry returns where the map of the tree of pages is, if it's
// shown, in the tree-map experiment: in its corner, a quarter of the
// window, the part around its focus, at a fixed scale, or an icon if
// that's too small; expanded, the document, at its own scale and middle
// (see expandMap).
func (a *App) treeMapGeometry() (treeMap, bool) {
	t := a.tree
	b := a.bounds
	s := a.deviceScale
	margin, pad := int(12*s), int(10*s)
	if t == nil || !a.experiment("tree-map") || b.Dx() < 4*margin || b.Dy() < 4*margin {
		return treeMap{}, false
	}
	w, h := b.Dx()/4, b.Dy()/4
	now := time.Now()
	corner := image.Rect(b.Max.X-margin-w, b.Max.Y-margin-h, b.Max.X-margin, b.Max.Y-margin)
	full := b // expanded, it's a view of its own
	m := treeMap{tree: t, grown: t.mapGrow.at(now)}
	if m.icon = a.experiment("tree-map-icon") || float64(w) < 120*s || float64(h) < 80*s; m.icon {
		size := int(32 * s)
		corner = image.Rect(b.Max.X-margin-size, b.Max.Y-margin-size, b.Max.X-margin, b.Max.Y-margin)
	}
	lerp := func(x, y float64) float64 { return x + (y-x)*m.grown }
	m.rect = image.Rect(
		int(lerp(float64(corner.Min.X), float64(full.Min.X))), int(lerp(float64(corner.Min.Y), float64(full.Min.Y))),
		int(lerp(float64(corner.Max.X), float64(full.Max.X))), int(lerp(float64(corner.Max.Y), float64(full.Max.Y))))
	m.area = m.rect.Inset(pad)

	// In the corner: a fixed scale, centred on the focus.
	fx, fy := t.focusX.at(now), t.focusY.at(now)
	small := corner.Inset(pad)
	step, rowStep := mapStep*s, mapRowStep*s
	x0 := float64(small.Min.X+small.Dx()/2) - fx*step
	y0 := float64(small.Min.Y+small.Dy()/2) - fy*rowStep
	halfW, halfH := float64(corner.Dx())/2/step, float64(corner.Dy())/2/rowStep
	m.box.x0, m.box.y0, m.box.x1, m.box.y1 = fx-halfW, fy-halfH, fx+halfW, fy+halfH
	// Expanded: scaled from the corner's steps alike in both directions,
	// so that the corner's part keeps its shape, around its middle (see
	// expandMap, panMap and zoomMap).
	big := full.Inset(pad)
	bigStep, bigRowStep := a.mapSteps()
	bx0 := float64(big.Min.X+big.Dx()/2) - t.mapCX*bigStep
	by0 := float64(big.Min.Y+big.Dy()/2) - t.mapCY*bigRowStep
	m.step, m.rowStep = lerp(step, bigStep), lerp(rowStep, bigRowStep)
	m.x0, m.y0 = lerp(x0, bx0), lerp(y0, by0)
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
	if !ok || !a.hasPointer || m.icon && m.grown == 0 {
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
	if m.icon && m.grown == 0 {
		a.drawMapIcon(dst, m.rect)
		return
	}
	accent := a.Panel.View().HighlightColor()
	thin := a.thin()
	r := m.rect
	fill(dst, r, translucent(theme.BackgroundColor, uint8(0xE6+(0xFF-0xE6)*m.grown))) // opaque, expanded
	if m.grown < 1 {
		frame(dst, r, thin, theme.ThematicBreakColor)
	}

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
	heat := a.experiment("tree-map-heat")
	for id := range t.pages {
		clr := theme.ThematicBreakColor
		if heat {
			clr = heatColor(t.mapHeat[id], a.darkTheme)
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
	if heat && m.grown == 1 {
		a.drawHeatScale(dst, m, wall)
	}
	// Expanding, the box around the part the corner shows, which is the
	// corner's own edge as it starts to grow; it keeps the corner's shape,
	// even where it goes past the map's edge.
	if m.grown > 0 && !m.icon {
		box := image.Rect(
			int(m.x0+m.box.x0*m.step), int(m.y0+m.box.y0*m.rowStep),
			int(m.x0+m.box.x1*m.step), int(m.y0+m.box.y1*m.rowStep))
		frame(dst, box, thin, theme.TextColor)
	}

	if hovered != "" {
		a.drawMapCard(dst, m, hovered, wall, now)
	}
}

// drawMapCard draws the card of page id, whose dot on m the pointer is on:
// its picture and its label. In the corner, it's a quarter of the window's
// width, above the map; expanded, a tile's size, beside the pointer.
func (a *App) drawMapCard(dst canvas.Canvas, m treeMap, id string, wall time.Time, now time.Duration) {
	theme := a.treeTheme()
	thin, r := a.thin(), m.rect
	b := a.bounds
	gap := int(8 * a.deviceScale)
	w := b.Dx() / 4
	if m.grown > 0 {
		w = b.Dx() / 2
	}
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
		p.X = max(b.Min.X, min(p.X, b.Max.X-w))
		p.Y = max(b.Min.Y, min(p.Y, b.Max.Y-h))
		card = image.Rect(p.X, p.Y, p.X+w, p.Y+h)
	}
	if !card.In(b) {
		return // no room
	}
	fill(dst, card, theme.BackgroundColor)
	a.drawThumb(dst, id, card, now)
	a.drawTreeLabel(dst, id, card, wall)
	frame(dst, card.Inset(-thin), thin, theme.TextColor)
}

// The heat scales: from cold, the color of the map's lines, through warm,
// to hot, on a dark background and on a light one.
var (
	darkHeat  = [3]color.RGBA{{0x3D, 0x44, 0x4D, 0xFF}, {0xC8, 0x7A, 0x24, 0xFF}, {0xFF, 0xE4, 0x7A, 0xFF}}
	lightHeat = [3]color.RGBA{{0xC8, 0xCC, 0xD2, 0xFF}, {0xE0, 0x7B, 0x00, 0xFF}, {0xB3, 0x26, 0x1E, 0xFF}}
)

// heatColor returns the color of heat h, from 0, cold, to 1, hot.
func heatColor(h float64, dark bool) color.Color {
	scale := lightHeat
	if dark {
		scale = darkHeat
	}
	from, to := scale[0], scale[1]
	if h > 0.5 {
		from, to, h = scale[1], scale[2], h-0.5
	}
	f := 2 * h
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return color.RGBA{mix(from.R, to.R), mix(from.G, to.G), mix(from.B, to.B), 0xFF}
}

// drawHeatScale draws the expanded map's heat scale on dst, in its
// bottom-left corner: the colors, with, below them, how old the pages are
// at the ends, the middle and the quarters of their ranking.
func (a *App) drawHeatScale(dst canvas.Canvas, m treeMap, now time.Time) {
	t := a.tree
	face, _, pad, ok := a.treeLabelFace()
	if !ok || len(t.mapByAge) == 0 {
		return
	}
	theme := a.treeTheme()
	s := a.deviceScale
	fm := face.Metrics()
	text := (fm.Ascent + fm.Descent).Ceil()
	w, barH := int(240*s), int(8*s)
	panel := image.Rect(m.area.Min.X, m.area.Max.Y-2*pad-barH-pad-text, m.area.Min.X+w+2*pad, m.area.Max.Y)
	if panel.Dx() > m.area.Dx() || panel.Min.Y < m.area.Min.Y {
		return // no room
	}
	fill(dst, panel, theme.BackgroundColor)
	frame(dst, panel, a.thin(), theme.ThematicBreakColor)
	bar := image.Rect(panel.Min.X+pad, panel.Min.Y+pad, panel.Max.X-pad, panel.Min.Y+pad+barH)
	for x := bar.Min.X; x < bar.Max.X; x++ {
		h := float64(x-bar.Min.X) / float64(max(1, bar.Dx()-1))
		fill(dst, image.Rect(x, bar.Min.Y, x+1, bar.Max.Y), heatColor(h, a.darkTheme))
	}
	baseline := bar.Max.Y + pad + fm.Ascent.Ceil()
	n := len(t.mapByAge)
	ageAt := func(f float64) string { return shortAge(t.pages[t.mapByAge[int(f*float64(n-1)+0.5)]].Time, now) }
	tick := func(x int) {
		fill(dst, image.Rect(x, bar.Max.Y, x+a.thin(), bar.Max.Y+pad/2), theme.ThematicBreakColor)
	}
	// The ends' labels, flush with them, and between them those of the
	// middle and the quarters, centred on their points, if they say
	// something else and there's room.
	first, last := ageAt(0), ageAt(1)
	dst.DrawText(first, face, bar.Min.X, baseline, theme.TextColor)
	tick(bar.Min.X)
	left := bar.Min.X + font.MeasureString(face, first).Ceil() + pad
	right := bar.Max.X
	if last != first {
		right -= font.MeasureString(face, last).Ceil()
		dst.DrawText(last, face, right, baseline, theme.TextColor)
		tick(bar.Max.X - a.thin())
		right -= pad
	}
	shown := first
	for _, f := range []float64{0.25, 0.5, 0.75} {
		label := ageAt(f)
		if label == shown || label == last {
			continue
		}
		lw := font.MeasureString(face, label).Ceil()
		at := bar.Min.X + int(f*float64(bar.Dx()))
		if x := at - lw/2; x >= left && x+lw <= right {
			dst.DrawText(label, face, x, baseline, theme.TextColor)
			tick(at)
			left, shown = x+lw+pad, label
		}
	}
}

// shortAge returns how long before now t was, in a word or two: "now",
// "5 min", "1 hour", "2 days", "3 weeks", "2 months", "1 year".
func shortAge(t, now time.Time) string {
	d := now.Sub(t)
	unit := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return unit(int(d.Hours()), "hour", "hours")
	case d < 7*24*time.Hour:
		return unit(int(d.Hours()/24), "day", "days")
	case d < 60*24*time.Hour:
		return unit(int(d.Hours()/24/7), "week", "weeks")
	case d < 365*24*time.Hour:
		return unit(int(d.Hours()/24/30), "month", "months")
	}
	return unit(int(d.Hours()/24/365), "year", "years")
}

// drawMapIcon draws the map's icon in r on dst, in a box.
func (a *App) drawMapIcon(dst canvas.Canvas, r image.Rectangle) {
	theme := a.treeTheme()
	fill(dst, r, translucent(theme.BackgroundColor, 0xE6))
	frame(dst, r, a.thin(), theme.ThematicBreakColor)
	in := r.Inset(r.Dx() / 8)
	dst.DrawImage(tint(mapIcon, theme.TextColor), in.Min.X, in.Min.Y, in.Dx(), in.Dy())
}
