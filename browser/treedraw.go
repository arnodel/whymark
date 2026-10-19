package browser

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"slices"
	"strings"
	"time"

	"golang.org/x/image/font"

	"github.com/arnodel/whymark"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// The tree view's layout and drawing: its frames, where they slide to,
// the lines between them, and their labels.

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

// treeFrames returns the frames of the tree view, on screen or not, where
// they are once they've slid there.
func (a *App) treeFrames() []treeFrame {
	parent, rows := a.treeLayout()
	return slices.Concat(append([][]treeFrame{parent}, rows...)...)
}

// treeRect returns where page id's frame is in the tree view, once it's
// slid there, if it's shown at all, on screen or not.
func (a *App) treeRect(id string) (image.Rectangle, bool) {
	for _, f := range a.treeFrames() {
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
	_, moving := easeOut(s.start, s.length, now)
	return moving
}

// at returns where s has moved r, page id's frame once it's there, at now.
func (s treeSlide) at(id string, r image.Rectangle, now time.Time) image.Rectangle {
	f, moving := easeOut(s.start, s.length, now)
	if !moving {
		return r
	}
	from, ok := s.from[id]
	if !ok {
		from = r.Sub(s.d)
	}
	lerp := func(x, y int) int { return x + int(float64(y-x)*f) }
	return r.Add(image.Pt(lerp(from.Min.X, r.Min.X), lerp(from.Min.Y, r.Min.Y)).Sub(r.Min))
}

// shownFrames returns all the frames of the tree view, where they are at
// now, sliding.
func (a *App) shownFrames(now time.Time) []treeFrame {
	frames := a.treeFrames()
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

// shownPath returns the pages the tree view shows on the way back (see
// tree.path), in the tree-path experiment. A row's line joins siblings, so
// a path from a page to its sibling goes along it, not through their
// parent, which is left out: for the selected page's row, and for its
// parent's.
func (a *App) shownPath() map[string]bool {
	if !a.experiment("tree-path") {
		return nil
	}
	t := a.tree
	path := t.path()
	for _, id := range []string{t.selected, t.pages[t.selected].Parent} {
		parent := t.pages[id].Parent
		if id != "" && slices.ContainsFunc(t.children[parent], func(s string) bool { return s != id && path[s] }) {
			delete(path, parent)
		}
	}
	return path
}

// treeLines returns the lines between frames, at where they're shown, as
// rectangles to fill: one through each row of siblings, and one from the
// selected page up to its parent and down to the child Down selects, bent
// while they slide out of line. Those on the way back (see shownPath) are
// in path instead, thicker.
func (a *App) treeLines(frames []treeFrame) (lines, path []image.Rectangle) {
	t := a.tree
	thin := a.thin()
	at := map[string]image.Rectangle{}
	for _, f := range frames {
		at[f.id] = f.rect
	}
	on := a.shownPath()
	// add adds the rectangles of a line, given how thick it is, to path if
	// it's on it, or else to lines.
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

// busLines returns the lines between frames as treeLines does, drawn as
// in an organisation chart, in the tree-bus experiment: each row hangs
// from a line across the gap above it, with a drop to each page, and a
// line up to its parent, if it's shown. The way back lights a row's lines
// only where it goes along it.
func (a *App) busLines(frames []treeFrame) (lines, path []image.Rectangle) {
	t := a.tree
	thin := a.thin()
	gap := int(treeGap * a.deviceScale)
	at := map[string]image.Rectangle{}
	for _, f := range frames {
		at[f.id] = f.rect
	}
	on := a.shownPath()
	add := func(onPath bool, line func(thick int) image.Rectangle) {
		if onPath {
			path = append(path, line(3*thin))
		} else {
			lines = append(lines, line(thin))
		}
	}
	vertical := func(x, y0, y1 int) func(int) image.Rectangle {
		return func(thick int) image.Rectangle {
			return image.Rect(x-thick/2, min(y0, y1), x-thick/2+thick, max(y0, y1))
		}
	}
	horizontal := func(x0, x1, y int) func(int) image.Rectangle {
		return func(thick int) image.Rectangle {
			return image.Rect(min(x0, x1)-thick/2, y-thick/2, max(x0, x1)-thick/2+thick, y-thick/2+thick)
		}
	}
	middle := func(r image.Rectangle) int { return r.Min.X + r.Dx()/2 }

	// A row's pages, the page lined up under its parent, and its parent,
	// "" for none, which isn't shown for the parent's row.
	type row struct {
		ids          []string
		anchor, over string
		shown        bool
	}
	var rows []row
	parent := t.pages[t.selected].Parent
	if parent != "" {
		grandparent := t.pages[parent].Parent
		rows = append(rows, row{t.children[grandparent], parent, grandparent, false})
	}
	rows = append(rows, row{t.children[parent], t.selected, parent, parent != ""})
	if child := t.preferredChild(t.selected); child != "" {
		rows = append(rows, row{t.children[t.selected], child, t.selected, true})
	}
	for _, r := range rows {
		anchor, ok := at[r.anchor]
		if !ok {
			continue
		}
		y := anchor.Min.Y - gap/2
		up := middle(anchor) // where the line up meets the row's
		if r.shown {
			up = middle(at[r.over])
		}
		// The points the path joins along the row: its pages on it, and
		// the line up, if it goes on up.
		var xs, points []int
		xs = append(xs, up)
		for _, id := range r.ids {
			if f, ok := at[id]; ok {
				xs = append(xs, middle(f))
				if on[id] {
					points = append(points, middle(f))
				}
			}
		}
		if on[r.over] {
			points = append(points, up)
		}
		along := len(points) >= 2
		lines = append(lines, horizontal(slices.Min(xs), slices.Max(xs), y)(thin))
		if along {
			path = append(path, horizontal(slices.Min(points), slices.Max(points), y)(3*thin))
		}
		for _, id := range r.ids {
			if f, ok := at[id]; ok {
				add(along && on[id], vertical(middle(f), y, f.Min.Y))
			}
		}
		if r.shown {
			add(along && on[r.over], vertical(up, at[r.over].Max.Y, y))
		}
	}
	return lines, path
}

// drawTree draws the tree view, through its camera.
func (a *App) drawTree(dst canvas.Canvas, now time.Duration) {
	t := a.tree
	theme := a.treeTheme()
	background, line := theme.BackgroundColor, theme.ThematicBreakColor
	b := a.bounds
	dst.DrawRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), background)
	wall := time.Now()
	camera := t.camera.at(wall)
	shown := camera.Invert().ApplyRect(b)
	window := dst.Clip(b)
	dst = window.Transform(camera)
	frames := a.shownFrames(wall)
	lines, path := a.treeLines(frames)
	if a.experiment("tree-bus") {
		lines, path = a.busLines(frames)
	}
	for _, l := range lines {
		dst.DrawRect(l.Min.X, l.Min.Y, l.Dx(), l.Dy(), line)
	}
	on := a.shownPath()
	accent := a.Panel.View().HighlightColor()
	for _, l := range path {
		dst.DrawRect(l.Min.X, l.Min.Y, l.Dx(), l.Dy(), accent)
	}
	for _, f := range frames {
		if !f.rect.Overlaps(shown) {
			continue
		}
		a.drawThumb(dst, f.id, f.rect, now)
		// The pages on the path are framed as it is, and the page the view
		// was opened from, where it leads, thicker.
		clr, w := line, a.thin()
		if on[f.id] {
			clr = accent
		}
		if f.id == t.start && on[f.id] {
			w *= 3
		}
		frame(dst, f.rect.Inset(-w), w, clr)
	}
	if a.experiment("tree-labels") {
		for _, f := range frames {
			if f.rect.Overlaps(shown) {
				a.drawTreeLabel(window, f.id, camera.ApplyRect(f.rect), wall)
			}
		}
	}
	// The map shows once the camera is still, so that zooming out starts
	// from the page as it was.
	if t.closing == nil && !t.camera.moving(wall) {
		a.drawTreeMap(window, wall, now)
	}
}

// drawThumb draws page id in r, a frame half the window's size or smaller,
// as it is in the window, scaled down.
func (a *App) drawThumb(dst canvas.Canvas, id string, r image.Rectangle, now time.Duration) {
	b := a.bounds
	v := a.treeView(id)
	if v.Bounds() != b || v.Scale() != a.deviceScale || v.Zoom() != a.zoom {
		a.place(v)
	}
	scale := float64(r.Dx()) / float64(b.Dx())
	v.Draw(dst.Clip(r).Transform(canvas.Transform{
		Scale: scale,
		X:     float64(r.Min.X) - scale*float64(b.Min.X),
		Y:     float64(r.Min.Y) - scale*float64(b.Min.Y),
	}), now)
}

// drawTreeLabel draws the label of page id on its frame, r on the window,
// in a bar along the bottom of what's on screen of it, at the size of the
// window's text whatever the frame's: what was typed for the page, or else
// its title, and how long ago it was made.
func (a *App) drawTreeLabel(dst canvas.Canvas, id string, r image.Rectangle, now time.Time) {
	face, h, pad, ok := a.treeLabelFace()
	if !ok {
		return
	}
	theme := a.treeTheme()
	r = r.Intersect(a.bounds)
	if r.Dx() < 4*h || r.Dy() < 2*h {
		return // too small to say anything
	}
	bar := image.Rect(r.Min.X, r.Max.Y-h, r.Max.X, r.Max.Y)
	fill(dst, bar, theme.BackgroundColor)
	fill(dst, image.Rect(bar.Min.X, bar.Min.Y, bar.Max.X, bar.Min.Y+a.thin()), theme.ThematicBreakColor)
	info := a.tree.pages[id]
	x := bar.Min.X + pad
	if clr, ok := markColor(info.Mark, a.darkTheme); ok {
		w := 4 * a.thin()
		fill(dst, image.Rect(bar.Min.X, bar.Min.Y, bar.Min.X+w, bar.Max.Y), clr)
		x += w
	}
	age := ageOf(info.Time, now)
	ageX := bar.Max.X - pad - font.MeasureString(face, age).Ceil()
	baseline := baselineIn(face, bar)
	dst.DrawText(age, face, ageX, baseline, theme.ThematicBreakColor)
	label := truncateEnd(face, treeLabel(info), ageX-pad-x)
	dst.DrawText(label, face, x, baseline, theme.TextColor)
}

// treeLabelFace returns the face of the tree view's labels, the height of
// their bars, and the padding in them.
func (a *App) treeLabelFace() (face font.Face, height, pad int, ok bool) {
	face, err := a.faceSelector.SelectFace(fonts.TextStyle{Size: 13}, a.deviceScale*72)
	if err != nil {
		return nil, 0, 0, false
	}
	pad = int(6 * a.deviceScale)
	m := face.Metrics()
	return face, (m.Ascent + m.Descent).Ceil() + 2*pad, pad, true
}

// treeTheme returns the colors of the tree view.
func (a *App) treeTheme() *simpletheme.Theme {
	if a.darkTheme {
		return simpletheme.Dark()
	}
	return simpletheme.Light()
}

// treeLabel returns what tells page p apart in the tree of pages: its
// summary, or else what was typed for it, or else its title.
func treeLabel(p whymark.PageInfo) string {
	if p.Summary != "" {
		return p.Summary
	}
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

// The marks' colors (see whymark.Marks), on a dark background and on a
// light one.
var (
	darkMarks = map[string]color.RGBA{
		"red": {0xE5, 0x53, 0x4B, 0xFF}, "orange": {0xE8, 0x83, 0x3A, 0xFF}, "yellow": {0xE3, 0xC3, 0x4A, 0xFF},
		"green": {0x57, 0xAB, 0x5A, 0xFF}, "blue": {0x53, 0x9B, 0xF5, 0xFF}, "purple": {0xB0, 0x83, 0xF0, 0xFF},
	}
	lightMarks = map[string]color.RGBA{
		"red": {0xCF, 0x22, 0x2E, 0xFF}, "orange": {0xBC, 0x4C, 0x00, 0xFF}, "yellow": {0x9A, 0x67, 0x00, 0xFF},
		"green": {0x1A, 0x7F, 0x37, 0xFF}, "blue": {0x09, 0x69, 0xDA, 0xFF}, "purple": {0x82, 0x50, 0xDF, 0xFF},
	}
)

// markColor returns the color of mark, if it's one of whymark.Marks.
func markColor(mark string, dark bool) (color.Color, bool) {
	marks := lightMarks
	if dark {
		marks = darkMarks
	}
	clr, ok := marks[mark]
	return clr, ok
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

// thin returns the width of the tree view's thin lines: a pixel, or more
// on a dense display.
func (a *App) thin() int { return max(1, int(a.deviceScale)) }

// translucent returns clr, see-through: opaque for an alpha of 0xFF.
func translucent(clr color.Color, alpha uint8) color.Color {
	c := color.NRGBAModel.Convert(clr).(color.NRGBA)
	c.A = alpha
	return c
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
