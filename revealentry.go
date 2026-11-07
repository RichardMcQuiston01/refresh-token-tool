package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// revealEntry is a text entry that reports double-clicks to a callback instead
// of selecting a word. The app uses it for the refresh token field, which shows
// a mask at rest and reveals the real value briefly on double-click.
type revealEntry struct {
	widget.Entry
	onDoubleTap func()
}

func newRevealEntry(onDoubleTap func()) *revealEntry {
	entry := &revealEntry{onDoubleTap: onDoubleTap}
	entry.ExtendBaseWidget(entry)
	return entry
}

// DoubleTapped overrides the embedded Entry's word-selection behaviour.
func (r *revealEntry) DoubleTapped(_ *fyne.PointEvent) {
	if r.onDoubleTap != nil {
		r.onDoubleTap()
	}
}
