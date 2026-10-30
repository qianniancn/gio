// SPDX-License-Identifier: Unlicense OR MIT

package text

import (
	"cmp"
	"image"
	"image/color"
	stddraw "image/draw"
	"math"
	"slices"
	"weak"

	"gioui.org/f32"
	giofont "gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	nativetext "gioui.org/text/native"
	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

type nativeGlyph struct {
	font *nativetext.Font
	id   uint32
	ppem float32
	key  GlyphID
}

type nativeGlyphKey struct {
	font weak.Pointer[nativetext.Font]
	id   uint32
	ppem fixed.Int26_6
}

type nativeShaper struct {
	system                     *nativetext.System
	fallback                   *shaperImpl
	clearType                  bool
	clearText, clearBackground color.NRGBA
	ids                        map[nativeGlyphKey]weak.Pointer[nativeGlyph]
	bitmaps                    lru[nativeGlyphKey, nativeBitmap]
	nextID                     GlyphID
	layouts                    uint32
}

type nativeBitmap struct {
	img       *image.RGBA
	mask      paint.MaskOp
	left, top int
}

func newNativeShaper(sys *nativetext.System, fallback *shaperImpl, clearType bool, foreground, background color.NRGBA) *nativeShaper {
	if sys == nil || !sys.Available() {
		return nil
	}
	return &nativeShaper{
		system: sys, fallback: fallback, clearType: clearType,
		clearText: foreground, clearBackground: background,
		ids:     make(map[nativeGlyphKey]weak.Pointer[nativeGlyph]),
		bitmaps: lru[nativeGlyphKey, nativeBitmap]{maxBytes: 1 << 20},
		// Zero is reserved for synthetic paragraph breaks.
		nextID: 1,
	}
}

// Layouts and exported glyphs own their native records. The lookup table
// only interns live records; evicted layouts must not keep every past size
// and character alive for the lifetime of the shaper.
func (s *nativeShaper) intern(font *nativetext.Font, id uint32, ppem fixed.Int26_6) *nativeGlyph {
	key := nativeGlyphKey{font: weak.Make(font), id: id, ppem: ppem}
	if g := s.ids[key].Value(); g != nil {
		return g
	}
	g := &nativeGlyph{font: font, id: id, ppem: fixedToFloat(ppem), key: s.nextID}
	s.nextID++
	s.ids[key] = weak.Make(g)
	return g
}

func (s *nativeShaper) LayoutRunes(params Parameters, txt []rune) document {
	s.layouts++
	if s.layouts%256 == 0 {
		for key, ref := range s.ids {
			if ref.Value() == nil {
				delete(s.ids, key)
			}
		}
	}
	original := txt
	hasNewline := len(txt) > 0 && txt[len(txt)-1] == '\n'
	if hasNewline {
		txt = txt[:len(txt)-1]
	}
	if params.MaxLines > 0 && hasNewline {
		params.forceTruncate = true
	}
	style := nativetext.Style{
		Family: string(params.Font.Typeface),
		Size:   fixedToFloat(params.PxPerEm),
		Weight: int(params.Font.Weight) + 400,
		Italic: params.Font.Style == giofont.Italic,
		Lang:   params.Locale.Language,
	}
	if style.Size <= 0 {
		if s.fallback == nil {
			s.fallback = newShaperImpl(true, nil)
		}
		return s.fallback.LayoutRunes(params, original)
	}
	text := replaceControlCharacters(txt)
	for i, r := range text {
		if r == '\t' {
			text[i] = ' '
		}
	}
	l := s.system.Shape(nativetext.Params{
		Text: string(text), Style: style, Width: float32(params.MaxWidth), MaxLines: params.MaxLines,
		TextContinues: params.forceTruncate,
		Ellipsis:      params.Truncator, Align: nativetext.Left, RTL: params.Locale.Direction == system.RTL,
		NoWrap: params.MaxWidth <= 0,
		// Keep every source rune in the glyph stream, even invisible whitespace.
		// Editors and styledtext use these counts to advance through the source.
		KeepSpaces:   true,
		NoBreakWords: params.WrapPolicy == WrapWords,
	})
	// Gio owns alignment; it is deliberately absent from the paragraph cache key.
	doc := document{alignment: params.Alignment, lines: make([]line, 0, len(l.Lines))}
	for li, nl := range l.Lines {
		direction := system.LTR
		if nl.RTL {
			direction = system.RTL
		}
		ln := line{
			width: floatToFixed(nl.Width), ascent: floatToFixed(nl.Ascent),
			descent: floatToFixed(nl.Descent), direction: direction, runeCount: nl.End - nl.Start,
		}
		// Native output can be in visual run order. Gio must iterate source clusters
		// logically, and emit a cluster's rune count exactly once, even for marks.
		glyphs := slices.Clone(nl.Glyphs)
		slices.SortStableFunc(glyphs, func(a, b nativetext.Glyph) int {
			return cmp.Compare(a.Cluster, b.Cluster)
		})
		for start := 0; start < len(glyphs); {
			end := start + 1
			for end < len(glyphs) && glyphs[end].Cluster == glyphs[start].Cluster {
				end++
			}
			cluster := glyphs[start:end]
			slices.SortStableFunc(cluster, func(a, b nativetext.Glyph) int {
				return cmp.Compare(a.X, b.X)
			})
			run := runLayout{
				X: floatToFixed(cluster[0].X), PPEM: params.PxPerEm, Direction: system.LTR,
				Runes:     Range{Offset: cluster[0].Cluster, Count: cluster[0].Runes},
				Glyphs:    make([]glyph, 0, len(cluster)),
				truncator: l.Truncated && li == len(l.Lines)-1 && cluster[0].Cluster == nl.End,
			}
			if cluster[0].RTL {
				run.Direction = system.RTL
			}
			for _, ng := range cluster {
				native := s.intern(ng.Font, ng.ID, params.PxPerEm)
				left, top, w, h, _ := s.system.RasterGlyph(ng.Font, ng.ID, 1)
				g := glyph{
					id: native.key, native: native,
					clusterIndex: ng.Cluster, runeCount: max(ng.Runes, 0), glyphCount: len(cluster),
					advance: floatToFixed(ng.Advance),
					xOffset: floatToFixed(ng.X) - run.X - run.Advance,
					yOffset: floatToFixed(ng.Y - nl.Baseline),
					bounds: fixed.Rectangle26_6{
						Min: fixed.P(left, top), Max: fixed.P(left+w, top+h),
					},
				}
				run.Glyphs = append(run.Glyphs, g)
				run.Advance += g.advance
			}
			ln.runs = append(ln.runs, run)
			start = end
		}
		if len(ln.runs) == 0 {
			ln.runs = []runLayout{{Direction: direction}}
		}
		if li == len(l.Lines)-1 {
			if l.Truncated {
				truncated := max(len(txt)-nl.End, 0)
				if hasNewline {
					truncated++
					hasNewline = false
				}
				ln.setTruncatedCount(truncated)
			}
			if hasNewline {
				ln.insertTrailingSyntheticNewline(len(txt))
			}
		}
		ln.visualOrder = make([]int, len(ln.runs))
		for i := range ln.visualOrder {
			ln.visualOrder[i] = i
		}
		slices.SortStableFunc(ln.visualOrder, func(a, b int) int {
			return cmp.Compare(ln.runs[a].X, ln.runs[b].X)
		})
		for position, index := range ln.visualOrder {
			ln.runs[index].VisualPosition = position
		}
		doc.lines = append(doc.lines, ln)
	}
	lineHeight := params.LineHeight
	if lineHeight == 0 {
		lineHeight = params.PxPerEm
	}
	scale := params.LineHeightScale
	if scale == 0 {
		scale = 1.2
	}
	for i := range doc.lines {
		doc.lines[i].lineHeight = floatToFixed(fixedToFloat(lineHeight) * scale)
	}
	doc.alignWidth = alignWidth(params.MinWidth, doc.lines)
	calculateYOffsets(doc.lines)
	return doc
}

func (s *nativeShaper) Shape(ops *op.Ops, glyphs []Glyph) clip.PathSpec {
	var path clip.Path
	path.Begin(ops)
	return path.End()
}

func (s *nativeShaper) Bitmaps(ops *op.Ops, glyphs []Glyph) (op.CallOp, int) {
	if s.clearType {
		if call, bytes := s.clearTypeBitmaps(ops, glyphs); call != (op.CallOp{}) {
			return call, bytes
		}
	}
	var originX fixed.Int26_6
	var originY int32
	if len(glyphs) > 0 {
		originX, originY = glyphs[0].X, glyphs[0].Y
	}
	macro := op.Record(ops)
	batch := make([]positionedNativeBitmap, 0, len(glyphs))
	var bounds image.Rectangle
	var bytes int
	flush := func() {
		bytes += paintNativeBatch(ops, batch, bounds)
		batch = batch[:0]
		bounds = image.Rectangle{}
	}
	for _, g := range glyphs {
		ng := g.native
		if ng == nil || ng.font == nil {
			continue
		}
		key := nativeGlyphKey{font: weak.Make(ng.font), id: ng.id, ppem: fixed.Int26_6(ng.ppem * 64)}
		bitmap, ok := s.bitmaps.Get(key)
		if !ok {
			left, top, w, h, pixels := s.system.RasterGlyph(ng.font, ng.id, 1)
			if w == 0 || h == 0 || len(pixels) < w*h {
				continue
			}
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			for y := range h {
				for x := range w {
					img.SetRGBA(x, y, color.RGBA{A: pixels[y*w+x]})
				}
			}
			bitmap = nativeBitmap{img: img, mask: paint.NewMaskOp(img), left: left, top: top}
			s.bitmaps.PutCost(key, bitmap, len(img.Pix))
		}
		off := f32.Pt(fixedToFloat(g.X-originX+g.Offset.X)+float32(bitmap.left), float32(g.Y-originY)+fixedToFloat(g.Offset.Y)+float32(bitmap.top))
		rect := image.Rect(int(math.Floor(float64(off.X))), int(math.Floor(float64(off.Y))),
			int(math.Ceil(float64(off.X)))+bitmap.img.Bounds().Dx(),
			int(math.Ceil(float64(off.Y)))+bitmap.img.Bounds().Dy())
		combined := bounds.Union(rect)
		// Bound sparse or very large runs; never allocate a viewport-sized mask.
		if len(batch) > 0 && (combined.Dx() > 2048 || combined.Dy() > 256) {
			flush()
		}
		batch = append(batch, positionedNativeBitmap{bitmap: bitmap, offset: off})
		bounds = bounds.Union(rect)
	}
	flush()
	return macro.Stop(), bytes
}

type positionedNativeBitmap struct {
	bitmap nativeBitmap
	offset f32.Point
}

// A cached text chunk needs one texture and draw instead of one per glyph.
// Fractional placement is baked with bilinear coverage, matching texture sampling.
func paintNativeBatch(ops *op.Ops, glyphs []positionedNativeBitmap, bounds image.Rectangle) int {
	if len(glyphs) == 0 {
		return 0
	}
	var mask paint.MaskOp
	var bytes int
	offset := f32.Pt(float32(bounds.Min.X), float32(bounds.Min.Y))
	if len(glyphs) == 1 {
		mask, offset = glyphs[0].bitmap.mask, glyphs[0].offset
		bytes = len(glyphs[0].bitmap.img.Pix)
	} else {
		img := image.NewRGBA(image.Rectangle{Max: bounds.Size()})
		bytes = len(img.Pix)
		for _, g := range glyphs {
			x, y := float64(g.offset.X)-float64(bounds.Min.X), float64(g.offset.Y)-float64(bounds.Min.Y)
			draw.BiLinear.Transform(img, f64.Aff3{1, 0, x, 0, 1, y}, g.bitmap.img, g.bitmap.img.Bounds(), stddraw.Over, nil)
		}
		mask = paint.NewMaskOp(img)
	}
	transform := op.Affine(f32.AffineId().Offset(offset)).Push(ops)
	size := bounds.Size()
	if len(glyphs) == 1 {
		size = glyphs[0].bitmap.img.Bounds().Size()
	}
	clipping := clip.Rect{Max: size}.Push(ops)
	mask.Add(ops)
	paint.PaintOp{}.Add(ops)
	clipping.Pop()
	transform.Pop()
	return bytes
}

func (s *nativeShaper) clearTypeBitmaps(ops *op.Ops, glyphs []Glyph) (op.CallOp, int) {
	subpixel := s.system.SubpixelEnabled()
	if len(glyphs) == 0 || !subpixel {
		return op.CallOp{}, 0
	}
	var originX fixed.Int26_6
	var originY int32
	if len(glyphs) > 0 {
		originX, originY = glyphs[0].X, glyphs[0].Y
	}
	var bounds image.Rectangle
	type subpixelMask struct {
		left, top, width, height int
		pixels                   []byte
	}
	masks := make([]subpixelMask, len(glyphs))
	for i, g := range glyphs {
		ng := g.native
		if ng == nil || ng.font == nil {
			continue
		}
		left, top, width, height, pixels := s.system.RasterSubpixelGlyph(ng.font, ng.id, 1, fixedToFloat(g.X))
		if width == 0 || height == 0 || len(pixels) < width*height*4 {
			return op.CallOp{}, 0
		}
		mask := subpixelMask{left: left, top: top, width: width, height: height, pixels: pixels}
		masks[i] = mask
		x := int(math.Floor(float64(fixedToFloat(g.X)))) - int(math.Floor(float64(fixedToFloat(originX)))) + left
		y := int(g.Y-originY) + top
		bounds = bounds.Union(image.Rect(x, y, x+width, y+height))
	}
	if bounds.Empty() {
		return op.CallOp{}, 0
	}
	img := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			img.SetRGBA(x, y, color.RGBA{R: s.clearBackground.R, G: s.clearBackground.G, B: s.clearBackground.B, A: 0xff})
		}
	}
	for i, mask := range masks {
		if mask.width == 0 {
			continue
		}
		g := glyphs[i]
		x0 := int(math.Floor(float64(fixedToFloat(g.X)))) - int(math.Floor(float64(fixedToFloat(originX)))) + mask.left - bounds.Min.X
		y0 := int(g.Y-originY) + mask.top - bounds.Min.Y
		for y := 0; y < mask.height; y++ {
			for x := 0; x < mask.width; x++ {
				o := (y*mask.width + x) * 4
				coverage := mask.pixels[o : o+3]
				dst := img.RGBAAt(x0+x, y0+y)
				channels := [3]uint8{s.clearText.R, s.clearText.G, s.clearText.B}
				background := [3]uint8{dst.R, dst.G, dst.B}
				for c := range 3 {
					cov := uint16(coverage[c])
					v := uint16(channels[c])*cov + uint16(background[c])*(255-cov)
					channels[c] = uint8((v + 127) / 255)
				}
				img.SetRGBA(x0+x, y0+y, color.RGBA{R: channels[0], G: channels[1], B: channels[2], A: 0xff})
			}
		}
	}
	macro := op.Record(ops)
	transform := op.Affine(f32.AffineId().Offset(f32.Pt(float32(bounds.Min.X), float32(bounds.Min.Y)))).Push(ops)
	clipStack := clip.Rect{Max: img.Bounds().Size()}.Push(ops)
	paint.NewImageOp(img).Add(ops)
	paint.PaintOp{}.Add(ops)
	clipStack.Pop()
	transform.Pop()
	return macro.Stop(), len(img.Pix)
}
