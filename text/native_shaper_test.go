// SPDX-License-Identifier: Unlicense OR MIT

package text

import (
	"bytes"
	"image"
	"runtime"
	"testing"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/internal/ops"
	"gioui.org/io/system"
	"gioui.org/op"
	"golang.org/x/image/math/fixed"
)

func TestNativeGlyphSurvivesLayoutEviction(t *testing.T) {
	shaper := NewShaper()
	if _, ok := shaper.shaper.(*nativeShaper); !ok {
		t.Skip("native text backend is unavailable")
	}
	shaper.LayoutString(Parameters{PxPerEm: fixed.I(19), MaxWidth: 500}, "Retained glyph")
	glyphs := []Glyph{}
	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		glyphs = append(glyphs, g)
	}
	// An editor may retain glyphs after the shaper's layout cache evicts
	// their source. They must still own everything needed for rasterization.
	shaper.layoutCache = layoutCache{}
	shaper.txt = document{}
	runtime.GC()
	var operations op.Ops
	shaper.Bitmaps(glyphs).Add(&operations)
	var reader ops.Reader
	reader.Reset(&operations.Internal)
	for encoded, ok := reader.Decode(); ok; encoded, ok = reader.Decode() {
		if ops.OpType(encoded.Data[0]) == ops.TypeMask {
			return
		}
	}
	t.Fatal("retained glyphs could not recreate their bitmaps")
}

func TestNativeDrawingCacheKeepsGlyphIdentity(t *testing.T) {
	shaper := NewShaper()
	if _, ok := shaper.shaper.(*nativeShaper); !ok {
		t.Skip("native text backend is unavailable")
	}
	shape := func() []Glyph {
		shaper.LayoutString(Parameters{PxPerEm: fixed.I(17), MaxWidth: 500}, "Reusable drawing")
		glyphs := []Glyph{}
		for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
			glyphs = append(glyphs, g)
		}
		return glyphs
	}
	first := shaper.Bitmaps(shape())
	shaper.layoutCache = layoutCache{}
	shaper.txt = document{}
	runtime.GC()
	if again := shaper.Bitmaps(shape()); first != again {
		t.Fatal("live drawing cache lost its native glyph identities")
	}
}

func TestNativeBitmapCacheEviction(t *testing.T) {
	shaper := NewShaper()
	backend, ok := shaper.shaper.(*nativeShaper)
	if !ok {
		t.Skip("native text backend is unavailable")
	}
	backend.bitmaps.maxBytes = 1 << 10
	shaper.bitmapShapeCache.cache.maxBytes = 16 << 10
	shape := func(size int) op.CallOp {
		shaper.LayoutString(Parameters{PxPerEm: fixed.I(size), MaxWidth: 1000}, "Native bitmap eviction 123")
		var glyphs []Glyph
		for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
			glyphs = append(glyphs, g)
		}
		return shaper.Bitmaps(glyphs)
	}
	pixels := func(call op.CallOp) []byte {
		var operations op.Ops
		call.Add(&operations)
		var reader ops.Reader
		reader.Reset(&operations.Internal)
		var result []byte
		for encoded, ok := reader.Decode(); ok; encoded, ok = reader.Decode() {
			for _, ref := range encoded.Refs {
				if img, ok := ref.(*image.RGBA); ok {
					result = append(result, img.Pix...)
				}
			}
		}
		return result
	}
	first := shape(16)
	want := pixels(first)
	if len(want) == 0 {
		t.Fatal("native text produced no bitmap")
	}
	for size := 17; size < 48; size++ {
		shape(size)
	}
	if backend.bitmaps.bytes > backend.bitmaps.maxBytes ||
		shaper.bitmapShapeCache.cache.bytes > shaper.bitmapShapeCache.cache.maxBytes {
		t.Fatal("bitmap caches exceeded their byte budgets")
	}
	if !bytes.Equal(pixels(first), want) {
		t.Fatal("eviction invalidated an outstanding paint operation")
	}
	if !bytes.Equal(pixels(shape(16)), want) {
		t.Fatal("cached layout could not recreate an evicted bitmap")
	}
}

func TestNativeShaperBitmapBatch(t *testing.T) {
	shaper := NewShaper()
	if _, ok := shaper.shaper.(*nativeShaper); !ok {
		t.Skip("native text backend is unavailable")
	}
	shaper.LayoutString(Parameters{PxPerEm: fixed.I(16), MaxWidth: 500}, "Native text 123")
	var glyphs []Glyph
	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		glyphs = append(glyphs, g)
	}
	var operations op.Ops
	shaper.Bitmaps(glyphs).Add(&operations)
	var reader ops.Reader
	reader.Reset(&operations.Internal)
	masks := 0
	for encoded, ok := reader.Decode(); ok; encoded, ok = reader.Decode() {
		if ops.OpType(encoded.Data[0]) == ops.TypeMask {
			masks++
		}
	}
	if masks != 1 {
		t.Fatalf("text chunk emitted %d masks, want one batch", masks)
	}
}

func TestNativeShaperRuneAccounting(t *testing.T) {
	for _, sample := range []string{" ", "\t", "code   ", "hello\n", "\n", "a\n\nb", "a\u0301", "\U0001f600", "العَرَبِيَّة", "क्षि"} {
		t.Run(sample, func(t *testing.T) {
			shaper := NewShaper()
			if _, ok := shaper.shaper.(*nativeShaper); !ok {
				t.Skip("native text backend is unavailable")
			}
			shaper.LayoutString(Parameters{PxPerEm: fixed.I(16), MaxWidth: 200}, sample)
			var runes int
			for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
				runes += int(g.Runes)
			}
			if want := utf8.RuneCountInString(sample); runes != want {
				t.Fatalf("glyph stream accounts for %d runes, want %d", runes, want)
			}
		})
	}
}

func TestNativeShaperTruncation(t *testing.T) {
	for _, sample := range []string{"hello world long text", "hello\nworld\n", "\n\n", "long\n"} {
		for _, truncator := range []string{"…", "..."} {
			t.Run(sample+truncator, func(t *testing.T) {
				shaper := NewShaper()
				if _, ok := shaper.shaper.(*nativeShaper); !ok {
					t.Skip("native text backend is unavailable")
				}
				shaper.LayoutString(Parameters{
					PxPerEm: fixed.I(16), MaxWidth: 50, MaxLines: 1, Truncator: truncator,
				}, sample)
				var runes, clusters int
				for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
					runes += int(g.Runes)
					if g.Flags&FlagTruncator != 0 && g.Flags&FlagClusterBreak != 0 {
						clusters++
					}
				}
				if runes != utf8.RuneCountInString(sample) || clusters != 1 {
					t.Fatalf("runes=%d want=%d, truncator clusters=%d want=1", runes, utf8.RuneCountInString(sample), clusters)
				}
			})
		}
	}
}

func TestNativeShaperLayoutParameters(t *testing.T) {
	shaper := NewShaper()
	if _, ok := shaper.shaper.(*nativeShaper); !ok {
		t.Skip("native text backend is unavailable")
	}
	params := Parameters{PxPerEm: fixed.I(16), MinWidth: 120, MaxWidth: 200, LineHeight: fixed.I(30), LineHeightScale: 1}
	for _, alignment := range []Alignment{Start, Middle, End} {
		t.Run(alignment.String(), func(t *testing.T) {
			params.Alignment = alignment
			shaper.LayoutString(params, "abc\ndef")
			first, _ := shaper.NextGlyph()
			var second Glyph
			for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
				if g.Y != first.Y {
					second = g
					break
				}
			}
			if second.Y-first.Y != 30 {
				t.Errorf("baseline gap=%d, want 30", second.Y-first.Y)
			}
			if alignment == Start && first.X != 0 || alignment != Start && first.X <= 0 {
				t.Errorf("unexpected first glyph X=%v for %s", first.X, alignment)
			}
		})
	}
}

func TestNativeShaperParagraphLineLimit(t *testing.T) {
	for _, limit := range []int{0, 1, 2, 3, 4} {
		shaper := NewShaper()
		if _, ok := shaper.shaper.(*nativeShaper); !ok {
			t.Skip("native text backend is unavailable")
		}
		const sample = "a\nb\nc"
		shaper.LayoutString(Parameters{PxPerEm: fixed.I(16), MaxWidth: 200, MaxLines: limit}, sample)
		var lines, runes int
		for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
			runes += int(g.Runes)
			if g.Flags&FlagLineBreak != 0 {
				lines++
			}
		}
		want := 3
		if limit > 0 {
			want = min(want, limit)
		}
		if lines != want || runes != 5 {
			t.Errorf("MaxLines=%d: lines=%d want=%d, runes=%d want=5", limit, lines, want, runes)
		}
	}
}

func TestNativeShaperChineseAndLatin(t *testing.T) {
	shaper := NewShaper()
	backend, ok := shaper.shaper.(*nativeShaper)
	if !ok {
		t.Skip("native text backend is unavailable on this platform")
	}
	shaper.LayoutString(Parameters{
		Font: font.Font{}, PxPerEm: fixed.I(28), MaxWidth: 360,
		Locale: system.Locale{Language: "zh-CN"},
	}, "中文文本 Gio rendering 123")

	var glyphs []Glyph
	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		if g.ID != 0 {
			glyphs = append(glyphs, g)
		}
	}
	if len(glyphs) < 8 {
		t.Fatalf("got %d drawable glyphs, want Chinese and Latin glyphs", len(glyphs))
	}
	shaper.Shape(glyphs)
	if len(backend.ids) == 0 {
		t.Fatal("native glyph IDs were not retained for drawing")
	}
	var rasterized int
	var partialCoverage int
	for _, glyph := range glyphs {
		ng := glyph.native
		_, _, width, height, pixels := backend.system.RasterGlyph(ng.font, ng.id, ng.ppem)
		if width > 0 && height > 0 && len(pixels) > 0 {
			rasterized++
			for _, coverage := range pixels {
				if coverage > 0 && coverage < 255 {
					partialCoverage++
				}
			}
		}
	}
	if rasterized == 0 {
		t.Fatal("native text engine did not rasterize any glyphs")
	}
	if partialCoverage == 0 {
		t.Fatal("native rasterizer returned no antialiased edge coverage")
	}
}

func TestNativeShaperArabic(t *testing.T) {
	shaper := NewShaper()
	if _, ok := shaper.shaper.(*nativeShaper); !ok {
		t.Skip("native text backend is unavailable on this platform")
	}
	shaper.LayoutString(Parameters{
		Font: font.Font{}, PxPerEm: fixed.I(18), MaxWidth: 600,
		Locale: system.Locale{Language: "ar", Direction: system.RTL},
	}, "العربية: مرحباً بالعالم، اختبار النص والأرقام 12345.")

	var glyphs []Glyph
	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		if g.ID != 0 {
			glyphs = append(glyphs, g)
		}
	}
	if len(glyphs) < 20 {
		t.Fatalf("got %d drawable glyphs for Arabic sample", len(glyphs))
	}
	t.Logf("Arabic glyph count=%d first=%+v last=%+v", len(glyphs), glyphs[0], glyphs[len(glyphs)-1])
	if path := shaper.Shape(glyphs); !path.Empty() {
		t.Fatal("native shaper should render glyphs from platform bitmaps")
	}
}
