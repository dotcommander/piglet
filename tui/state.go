package tui

// cachedMsg holds a rendered message string with an explicit set flag,
// replacing the "\x00" sentinel so empty renders are distinguishable from
// un-cached slots without special-casing the zero value.
type cachedMsg struct {
	rendered string
	set      bool
}

// widgetState holds the content and placement of a single extension widget.
type widgetState struct {
	Placement string // "above-input" or "below-status"
	Content   string
}
