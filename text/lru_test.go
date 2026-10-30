// SPDX-License-Identifier: Unlicense OR MIT

package text

import (
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"gioui.org/op/clip"
)

func TestLayoutLRU(t *testing.T) {
	c := new(layoutCache)
	put := func(i int) {
		c.Put(layoutKey{str: strconv.Itoa(i)}, document{})
	}
	get := func(i int) bool {
		_, ok := c.Get(layoutKey{str: strconv.Itoa(i)})
		return ok
	}
	testLRU(t, put, get)
}

func TestLayoutCacheOwnsText(t *testing.T) {
	source := strings.Repeat("large document ", 100)
	key := layoutKey{str: source[:5]}
	var cache layoutCache
	cache.Put(key, document{})
	for stored := range cache.m {
		if stored.str != key.str {
			t.Fatal("cached paragraph changed")
		}
		if unsafe.StringData(stored.str) == unsafe.StringData(source) {
			t.Fatal("cached paragraph retains the source document")
		}
	}
}

func TestPathLRU(t *testing.T) {
	c := new(pathCache)
	shaped := []Glyph{{ID: 1}}
	put := func(i int) {
		c.Put(uint64(i), shaped, clip.PathSpec{})
	}
	get := func(i int) bool {
		_, ok := c.Get(uint64(i), shaped)
		return ok
	}
	testLRU(t, put, get)
}

func TestLRUReplacement(t *testing.T) {
	var c lru[int, int]
	for i := range maxSize {
		c.Put(i, i)
	}
	c.Put(0, -1)
	c.Put(maxSize, maxSize)
	if _, ok := c.Get(1); ok {
		t.Fatal("replacement did not refresh recency")
	}
	c.Put(maxSize+1, maxSize+1)
	if v, ok := c.Get(0); !ok || v != -1 {
		t.Fatal("evicting an old node removed the replacement")
	}
}

func TestLRUByteBudget(t *testing.T) {
	c := lru[int, int]{maxBytes: 10}
	c.PutCost(1, 1, 4)
	c.PutCost(2, 2, 4)
	c.Get(1)
	c.PutCost(3, 3, 5)
	if _, ok := c.Get(2); ok {
		t.Fatal("byte pressure did not evict the least recent entry")
	}
	c.PutCost(1, 10, 5)
	c.PutCost(4, 4, 11)
	if _, ok := c.Get(4); ok {
		t.Fatal("oversized entry was cached")
	}
	if v, ok := c.Get(1); !ok || v != 10 {
		t.Fatal("oversized entry evicted the working set")
	}
	if c.bytes != 10 || len(c.m) != 2 {
		t.Fatalf("replacement accounting: bytes=%d entries=%d", c.bytes, len(c.m))
	}
}

func testLRU(t *testing.T, put func(i int), get func(i int) bool) {
	for i := range maxSize {
		put(i)
	}
	for i := range maxSize {
		if !get(i) {
			t.Fatalf("key %d was evicted", i)
		}
	}
	put(maxSize)
	for i := 1; i < maxSize+1; i++ {
		if !get(i) {
			t.Fatalf("key %d was evicted", i)
		}
	}
	if i := 0; get(i) {
		t.Fatalf("key %d was not evicted", i)
	}
}
