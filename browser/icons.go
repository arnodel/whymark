package browser

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// decodeIcon loads a PNG from assetsFS (assets/icons/) - decoded once
// at startup (see the icon vars below), not per frame. Returns a plain
// image.Image, not a backend-specific type: each renderer backend
// already has its own way to upload/tint an image (e.g.
// ebitenbackend's own DrawImageOptions.ColorScale, or a Gio app's own
// paint ops), so this package stays backend-agnostic.
func decodeIcon(name string) image.Image {
	data, err := assetsFS.ReadFile("assets/icons/" + name)
	if err != nil {
		panic(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return img
}

// BackIcon/ForwardIcon/ReloadIcon/TOCIcon/TreeIcon/ActionsIcon are the
// toolbar buttons' icons - light, mostly-white silhouettes on a
// transparent background, so a backend can tint them to match a button's
// current state. ActionsIcon, a vertical ellipsis, reads as "more things to do":
// it opens the actions page, where zoom and the theme are too.
var (
	BackIcon    = decodeIcon("arrow_back.png")
	ForwardIcon = decodeIcon("arrow_forward.png")
	ReloadIcon  = decodeIcon("refresh.png")
	TOCIcon     = decodeIcon("toc.png")
	TreeIcon    = decodeIcon("graph.png")
	ActionsIcon = decodeIcon("more_vert.png")
)

// MicIcon and SendIcon are the buttons' beside the prompt: the microphone
// listens where it can (see Listener), and send sends
// the prompt's text, without reaching for Enter.
var (
	MicIcon  = decodeIcon("mic.png")
	SendIcon = decodeIcon("send.png")
)

// mapIcon is the icon in the tree of pages' corner that expands its map,
// when the map itself doesn't show there (see treeMapGeometry).
var mapIcon = decodeIcon("map.png")

// tinted keeps the icons tinted by tint, by icon and color.
var tinted = map[struct {
	icon image.Image
	clr  color.RGBA
}]image.Image{}

// tint returns icon in clr, keeping its shape: always the same image for
// the same icon and color, as a Canvas expects of images it caches.
func tint(icon image.Image, clr color.Color) image.Image {
	key := struct {
		icon image.Image
		clr  color.RGBA
	}{icon, color.RGBAModel.Convert(clr).(color.RGBA)}
	if img, ok := tinted[key]; ok {
		return img
	}
	b := icon.Bounds()
	img := image.NewRGBA(b)
	draw.DrawMask(img, b, image.NewUniform(clr), image.Point{}, icon, b.Min, draw.Src)
	tinted[key] = img
	return img
}
