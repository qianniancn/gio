// SPDX-License-Identifier: Unlicense OR MIT

package gpu

import (
	"image"
	"testing"
)

func TestPackLayersKeepsDepthRunsSeparate(t *testing.T) {
	var renderer renderer
	renderer.layers.maxDims = image.Pt(512, 512)
	layers := []opacityLayer{}
	for _, depth := range []int{0, 0, 1, 1, 0, 0} {
		layers = append(layers, opacityLayer{
			depth: depth, parent: -1, clip: image.Rect(0, 0, 16, 16),
		})
	}
	packed := renderer.packLayers(layers)
	for i, layer := range packed {
		if want := i / 2; layer.place.Idx != want {
			t.Fatalf("layer %d at depth %d uses page %d, want %d", i, layer.depth, layer.place.Idx, want)
		}
	}
}

func BenchmarkPacker(b *testing.B) {
	var p packer
	p.maxDims = image.Point{X: 4096, Y: 4096}
	for i := 0; b.Loop(); i++ {
		p.clear()
		p.newPage()
		for k := range 500 {
			_, ok := p.tryAdd(xy(k))
			if !ok {
				b.Fatal("add failed", i, k, xy(k))
			}
		}
	}
}

func xy(v int) image.Point {
	return image.Point{
		X: ((v / 16) % 16) + 8,
		Y: (v % 16) + 8,
	}
}
