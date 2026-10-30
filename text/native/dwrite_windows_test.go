//go:build windows && (amd64 || arm64)

package text

import (
	"runtime"
	"testing"
	"time"
	"weak"
)

func TestDWriteFontOwnedByCaller(t *testing.T) {
	s := Shared()
	if !s.Available() {
		t.Skip("DirectWrite is unavailable")
	}
	makeFont := func() (weak.Pointer[Font], <-chan struct{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		e, ok := s.engine().(*dwrite)
		if !ok {
			t.Skip("DirectWrite is unavailable")
		}
		f := e.font(Style{Size: 23.34375})
		// Dropping lookup keys must not invalidate a font still held by
		// another window or a layout outside the system cache.
		e.forgetFonts()
		runtime.GC()
		if m, _ := e.mode(f, 1); m == 0 {
			t.Fatal("font became unusable after lookup eviction")
		}
		done := make(chan struct{})
		runtime.AddCleanup(f, func(ch chan struct{}) { close(ch) }, done)
		return weak.Make(f), done
	}
	ref, done := makeFont()
	deadline := time.After(5 * time.Second)
	for {
		runtime.GC()
		select {
		case <-done:
			if ref.Value() != nil {
				t.Fatal("font cleanup ran while the font was reachable")
			}
			return
		case <-deadline:
			t.Fatal("lookup tables retained an unowned font")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
