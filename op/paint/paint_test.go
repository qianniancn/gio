// SPDX-License-Identifier: Unlicense OR MIT

package paint

import (
	"testing"

	"gioui.org/internal/ops"
	"gioui.org/op"
)

func TestOpaqueGroupNeedsNoCompositingOps(t *testing.T) {
	var operations op.Ops
	opaque := PushOpacity(&operations, 1)
	translucent := PushOpacity(&operations, .5)
	translucent.Pop()
	opaque.Pop()
	var reader ops.Reader
	reader.Reset(&operations.Internal)
	count := 0
	for _, ok := reader.Decode(); ok; _, ok = reader.Decode() {
		count++
	}
	if count != 2 {
		t.Fatalf("opacity operations = %d, want only the translucent push/pop", count)
	}
}

func TestOpaqueGroupStillChecksMacroBoundary(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("cross-macro opacity Pop did not panic")
		}
	}()
	var operations op.Ops
	stack := PushOpacity(&operations, 1)
	op.Record(&operations)
	stack.Pop()
}
