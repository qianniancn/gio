// SPDX-License-Identifier: Unlicense OR MIT

package app

// CloseRequestEvent is sent when the user asks to close a window.
// Call Window.Perform with system.ActionClose to close it.
type CloseRequestEvent struct{}

func (CloseRequestEvent) ImplementsEvent() {}

// DestroyEvent is the last event sent through
// a window event channel.
type DestroyEvent struct {
	// Err is nil for normal window closures. If a
	// window is prematurely closed, Err is the cause.
	Err error
}

func (DestroyEvent) ImplementsEvent() {}
