package browser

import (
	"slices"
	"strconv"
)

// An experiment is a feature being tried out, which can be turned on and
// off while the browser runs: from the actions page, or while the tree of
// pages shows, with the digit key of its place in experiments.
type experiment struct {
	name, text string
	// on is whether it's on until it's turned off.
	on bool
}

// experiments are the features being tried out.
var experiments = []experiment{
	{name: "tree-labels", text: "Labels in the tree of pages", on: true},
	{name: "tree-path", text: "The way back in the tree of pages", on: true},
	{name: "tree-bus", text: "Rows hanging from a line in the tree of pages"},
	{name: "tree-map", text: "A map of the whole tree of pages", on: true},
}

// experiment reports whether the experiment name is on.
func (a *App) experiment(name string) bool {
	if on, ok := a.experimentsOn[name]; ok {
		return on
	}
	i := slices.IndexFunc(experiments, func(e experiment) bool { return e.name == name })
	return i >= 0 && experiments[i].on
}

// toggleExperiment turns the experiment name on if it's off, and off if
// it's on.
func (a *App) toggleExperiment(name string) {
	if a.experimentsOn == nil {
		a.experimentsOn = map[string]bool{}
	}
	a.experimentsOn[name] = !a.experiment(name)
}

// experimentKeys are the keys that toggle experiments while the tree of
// pages shows: "1" for the first, and so on.
func experimentKeys() []Key {
	var keys []Key
	for i := range min(len(experiments), 9) {
		keys = append(keys, Key{Name: strconv.Itoa(i + 1)})
	}
	return keys
}

// experimentKey toggles the experiment whose key k is (see
// experimentKeys), reporting whether there was one.
func (a *App) experimentKey(k Key) bool {
	n, err := strconv.Atoi(k.Name)
	if err != nil || k.Shortcut || k.Shift || k.Alt || n < 1 || n > min(len(experiments), 9) {
		return false
	}
	a.toggleExperiment(experiments[n-1].name)
	return true
}
