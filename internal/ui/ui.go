package ui

import (
	"fmt"

	a "9fans.net/go/acme"
)

type Window = a.Win

// WindowOpen shows the window with a given name if it exists,
// otherwise creates it
func WindowOpen(name string) (*a.Win, error) {
	if w := a.Show(name); w != nil {
		return w, nil
	}
	w, err := a.New()
	if err != nil {
		return nil, fmt.Errorf("failed to open prompt window: %w", err)
	}

	if err := w.Name(name); err != nil {
		w.Del(true)
		return nil, fmt.Errorf("failed to set prompt window name: %w", err)
	}
	return w, nil
}

// WindowDirty marks the window as "dirty"
// (dirty=true) or "clean" (dirty=false).
func WindowDirty(w *a.Win, dirty bool) {
	if dirty {
		w.Ctl("dirty")
	} else {
		w.Ctl("clean")
	}
}

// DotToAddr positions the cursor in a window to the given addr.
// addr follows the ':' in a fileaddr (e.g., '$', '#0', '<linenum>',
// '<range>')
func DotToAddr(w *a.Win, addr string) {
	w.Addr(addr)
	w.Ctl("dot=addr")
	w.Ctl("show")
}

// WindowClear clears a window's contents
func WindowClear(w *a.Win) {
	w.Clear()
}

// TagSet sets the tag of the window with the given name.
func TagSet(w *a.Win, tag string) error {
	if _, err := w.Write("tag", []byte(tag)); err != nil {
		w.Del(true)
		return fmt.Errorf("failed to set tag: %w", err)
	}
	return nil
}

// BodyWrite writes the bytes s to the window body at address addr.
// addr is what follows the ":" in a file address, e.g., "$" for EOF,
// "1" for BOF, "," for the full file, "1,20" for lines 1-20.
func BodyWrite(w *a.Win, addr string, s []byte) error {
	if err := w.Addr(addr); err != nil {
		return fmt.Errorf("failed to seek to end of body: %w", err)
	}

	_, err := w.Write("data", s)
	if err != nil {
		return fmt.Errorf("failed to append data: %w", err)
	}

	return nil
}

// BodyRead reads the full contents from the window of
// the given name.
func BodyRead(w *a.Win) ([]byte, error) {
	data, err := w.ReadAll("body")
	if err != nil {
		return []byte(""), fmt.Errorf("failed to read from window: %w", err)
	}

	return data, nil
}
