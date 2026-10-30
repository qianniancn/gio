// SPDX-License-Identifier: Unlicense OR MIT

package text

import (
	"image"
	"strings"
	"sync/atomic"
	"unsafe"

	giofont "gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"golang.org/x/image/math/fixed"
)

// entry holds a single key-value pair for an LRU cache.
type entry[K comparable, V any] struct {
	next, prev *entry[K, V]
	key        K
	v          V
	bytes      int
}

// lru is a generic least-recently-used cache.
type lru[K comparable, V any] struct {
	m          map[K]*entry[K, V]
	head, tail *entry[K, V]
	bytes      int
	maxBytes   int
}

// Get fetches the value associated with the given key, if any.
func (l *lru[K, V]) Get(k K) (V, bool) {
	if lt, ok := l.m[k]; ok {
		l.remove(lt)
		l.insert(lt)
		return lt.v, true
	}
	var v V
	return v, false
}

// Put inserts the given value with the given key, evicting old
// cache entries if necessary.
func (l *lru[K, V]) Put(k K, v V) {
	l.PutCost(k, v, 0)
}

// PutCost bounds retained payload as well as entry count. A zero
// maxBytes keeps a count-only policy.
func (l *lru[K, V]) PutCost(k K, v V, bytes int) {
	if l.m == nil {
		l.m = make(map[K]*entry[K, V])
		l.head = new(entry[K, V])
		l.tail = new(entry[K, V])
		l.head.prev = l.tail
		l.tail.next = l.head
	}
	val := l.m[k]
	if val != nil {
		l.remove(val)
		l.bytes -= val.bytes
		delete(l.m, k)
	}
	// A large one-off value must not evict the useful working set.
	if l.maxBytes > 0 && bytes > l.maxBytes {
		return
	}
	if val == nil {
		val = new(entry[K, V])
	}
	val.key, val.v, val.bytes = k, v, bytes
	l.m[k] = val
	l.insert(val)
	l.bytes += bytes
	for len(l.m) > maxSize || l.maxBytes > 0 && l.bytes > l.maxBytes {
		oldest := l.tail.next
		l.remove(oldest)
		delete(l.m, oldest.key)
		l.bytes -= oldest.bytes
	}
}

// remove cuts e out of the lru linked list.
func (l *lru[K, V]) remove(e *entry[K, V]) {
	e.next.prev = e.prev
	e.prev.next = e.next
}

// insert adds e to the lru linked list.
func (l *lru[K, V]) insert(e *entry[K, V]) {
	e.next = l.head
	e.prev = l.head.prev
	e.prev.next = e
	e.next.prev = e
}

type bitmapCache = lru[GlyphID, bitmap]

type bitmap struct {
	img  paint.ImageOp
	size image.Point
}

type layoutCache struct {
	lru[layoutKey, document]
}

func (c *layoutCache) Put(k layoutKey, v document) {
	bytes := layoutBytes(k, v)
	if c.maxBytes > 0 && bytes > c.maxBytes {
		return
	}
	// Paragraphs are substrings of the input document. Own the cached keys
	// so a short paragraph cannot retain an arbitrarily large source string.
	k.str = strings.Clone(k.str)
	k.truncator = strings.Clone(k.truncator)
	k.font.Typeface = giofont.Typeface(strings.Clone(string(k.font.Typeface)))
	k.locale.Language = strings.Clone(k.locale.Language)
	c.PutCost(k, v, bytes)
}

// layoutBytes accounts for owned strings, slice capacities and entry metadata.
// Shared fonts, allocator rounding and map bucket overhead are not included.
func layoutBytes(k layoutKey, d document) int {
	bytes := int(unsafe.Sizeof(entry[layoutKey, document]{})) + int(unsafe.Sizeof(k))
	bytes += len(k.str) + len(k.truncator) + len(k.font.Typeface) + len(k.locale.Language)
	bytes += cap(d.lines) * int(unsafe.Sizeof(line{}))
	for _, l := range d.lines {
		bytes += cap(l.runs)*int(unsafe.Sizeof(runLayout{})) + cap(l.visualOrder)*int(unsafe.Sizeof(int(0)))
		for _, r := range l.runs {
			bytes += cap(r.Glyphs) * int(unsafe.Sizeof(glyph{}))
		}
	}
	return bytes
}

type glyphValue[V any] struct {
	v      V
	glyphs []glyphInfo
}

type glyphLRU[V any] struct {
	seed  uint64
	cache lru[uint64, glyphValue[V]]
}

var seed atomic.Uint32

// hashGlyphs computes a hash key based on the ID and X offset of
// every glyph in the slice.
func (c *glyphLRU[V]) hashGlyphs(gs []Glyph) uint64 {
	if c.seed == 0 {
		c.seed = uint64(seed.Add(3900798947))
	}
	if len(gs) == 0 {
		return 0
	}

	h := c.seed
	firstX := gs[0].X
	for _, g := range gs {
		h += uint64(g.X - firstX)
		h *= 6585573582091643
		h += uint64(g.ID)
		h *= 3650802748644053
	}

	return h
}

func (c *glyphLRU[V]) Get(key uint64, gs []Glyph) (V, bool) {
	if v, ok := c.cache.Get(key); ok && gidsEqual(v.glyphs, gs) {
		return v.v, true
	}
	var v V
	return v, false
}

func (c *glyphLRU[V]) Put(key uint64, glyphs []Glyph, v V) {
	c.PutCost(key, glyphs, v, 0)
}

func (c *glyphLRU[V]) PutCost(key uint64, glyphs []Glyph, v V, bytes int) {
	gids := make([]glyphInfo, len(glyphs))
	firstX := fixed.I(0)
	for i, glyph := range glyphs {
		if i == 0 {
			firstX = glyph.X
		}
		// Cache glyph X offsets relative to the first glyph.
		gids[i] = glyphInfo{ID: glyph.ID, X: glyph.X - firstX, native: glyph.native}
	}
	val := glyphValue[V]{
		glyphs: gids,
		v:      v,
	}
	c.cache.PutCost(key, val, bytes)
}

type pathCache = glyphLRU[clip.PathSpec]

type bitmapShapeCache = glyphLRU[op.CallOp]

type glyphInfo struct {
	ID GlyphID
	X  fixed.Int26_6
	// Preserve native IDs while their cached drawing operations are reusable.
	native *nativeGlyph
}

type layoutKey struct {
	ppem               fixed.Int26_6
	maxWidth, minWidth int
	maxLines           int
	str                string
	truncator          string
	locale             system.Locale
	font               giofont.Font
	forceTruncate      bool
	wrapPolicy         WrapPolicy
	lineHeight         fixed.Int26_6
	lineHeightScale    float32
}

const maxSize = 1000

func gidsEqual(a []glyphInfo, glyphs []Glyph) bool {
	if len(a) != len(glyphs) {
		return false
	}
	firstX := fixed.Int26_6(0)
	for i := range a {
		if i == 0 {
			firstX = glyphs[i].X
		}
		// Cache glyph X offsets relative to the first glyph.
		if a[i].ID != glyphs[i].ID || a[i].X != (glyphs[i].X-firstX) {
			return false
		}
	}
	return true
}
