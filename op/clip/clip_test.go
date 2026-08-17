// SPDX-License-Identifier: Unlicense OR MIT

package clip_test

import (
	"image/color"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func TestPathOutline(t *testing.T) {
	t.Run("closed path", func(t *testing.T) {
		defer func() {
			if err := recover(); err != nil {
				t.Error("Outline of a closed path did panic")
			}
		}()
		var p clip.Path
		p.Begin(new(op.Ops))
		p.MoveTo(f32.Pt(300, 200))
		p.LineTo(f32.Pt(150, 200))
		p.MoveTo(f32.Pt(150, 200))
		p.ArcTo(f32.Pt(300, 200), f32.Pt(300, 200), 3*math.Pi/4)
		p.LineTo(f32.Pt(300, 200))
		p.Close()
		clip.Outline{Path: p.End()}.Op()
	})
}

func TestPathBegin(t *testing.T) {
	ops := new(op.Ops)
	var p clip.Path
	p.Begin(ops)
	p.LineTo(f32.Pt(10, 10))
	p.Close()
	stack := clip.Outline{Path: p.End()}.Op().Push(ops)
	paint.Fill(ops, color.NRGBA{A: 255})
	stack.Pop()
	w := newWindow(t, 100, 100)
	if w == nil {
		return
	}
	// The following should not panic.
	_ = w.Frame(ops)
}

func TestPathCache(t *testing.T) {
	var cache clip.PathCache
	p := cache.Begin()
	p.MoveTo(f32.Pt(10, 10))
	p.LineTo(f32.Pt(90, 10))
	p.LineTo(f32.Pt(50, 90))
	p.Close()
	path := p.End()

	w := newWindow(t, 100, 100)
	if w == nil {
		return
	}
	defer w.Release()
	var ops op.Ops
	for range 2 {
		stack := clip.Outline{Path: path}.Op().Push(&ops)
		paint.Fill(&ops, color.NRGBA{A: 0xff})
		stack.Pop()
		if err := w.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		ops.Reset()
	}
}

func BenchmarkPathCache(b *testing.B) {
	build := func(p *clip.Path) {
		p.MoveTo(f32.Pt(0, 0))
		for i := range 64 {
			x := float32(i*8 + 4)
			p.CubeTo(f32.Pt(x, 0), f32.Pt(x, 64), f32.Pt(x+4, 32))
		}
		p.Close()
	}
	for _, cached := range []bool{true, false} {
		name := "Rebuild"
		if cached {
			name = "Cached"
		}
		b.Run(name, func(b *testing.B) {
			var ops op.Ops
			var path clip.PathSpec
			var cache clip.PathCache
			if cached {
				p := cache.Begin()
				build(&p)
				path = p.End()
			}
			b.ReportAllocs()
			for b.Loop() {
				if !cached {
					var p clip.Path
					p.Begin(&ops)
					build(&p)
					path = p.End()
				}
				stack := clip.Outline{Path: path}.Op().Push(&ops)
				paint.PaintOp{}.Add(&ops)
				stack.Pop()
				ops.Reset()
			}
		})
	}
}

func TestTransformChecks(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Error("cross-macro Pop didn't panic")
		}
	}()
	var ops op.Ops
	st := clip.Op{}.Push(&ops)
	op.Record(&ops)
	st.Pop()
}

func TestEmptyPath(t *testing.T) {
	var ops op.Ops
	p := clip.Path{}
	p.Begin(&ops)
	defer clip.Stroke{
		Path:  p.End(),
		Width: 3,
	}.Op().Push(&ops).Pop()
}

func newWindow(t testing.TB, width, height int) *headless.Window {
	w, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("failed to create headless window, skipping: %v", err)
	}
	return w
}
