// SPDX-License-Identifier: Unlicense OR MIT

package gpu

import (
	"image"
	"testing"

	"gioui.org/gpu/internal/driver"
)

type fboTestDevice struct {
	driver.Device
	live int
}

type fboTestTexture struct {
	driver.Texture
	device *fboTestDevice
}

func (d *fboTestDevice) Caps() driver.Caps {
	return driver.Caps{MaxTextureSize: 4096}
}

func (d *fboTestDevice) NewTexture(_ driver.TextureFormat, _, _ int, _, _ driver.TextureFilter, _ driver.BufferBinding) (driver.Texture, error) {
	d.live++
	return &fboTestTexture{device: d}, nil
}

func (t *fboTestTexture) Release() {
	t.device.live--
}

func TestFramebuffersShrinkAndRecreateAfterEmptyFrame(t *testing.T) {
	device := new(fboTestDevice)
	var buffers fboSet
	buffers.resize(device, driver.TextureFormatSRGBA, []image.Point{image.Pt(1000, 1000)})
	buffers.resize(device, driver.TextureFormatSRGBA, []image.Point{image.Pt(100, 100)})
	if size := buffers.fbos[0].size; size.X > 110 || size.Y > 110 {
		t.Fatalf("small frame retained oversized framebuffer: %v", size)
	}
	buffers.resize(device, driver.TextureFormatSRGBA, nil)
	if device.live != 0 {
		t.Fatalf("empty frame retained %d textures", device.live)
	}
	buffers.resize(device, driver.TextureFormatSRGBA, []image.Point{image.Pt(100, 100)})
	if device.live != 1 || buffers.fbos[0].tex == nil {
		t.Fatal("framebuffer was not recreated after release")
	}
	buffers.delete(device, 0)
}

func TestEmptyRenderPassesReleaseFramebuffers(t *testing.T) {
	device := new(fboTestDevice)
	r := renderer{
		ctx:    device,
		pather: &pather{stenciler: &stenciler{ctx: device}},
	}
	for _, buffers := range []*fboSet{&r.layerFBOs, &r.pather.stenciler.fbos, &r.pather.stenciler.intersections} {
		buffers.resize(device, driver.TextureFormatSRGBA, []image.Point{image.Pt(100, 100)})
	}
	r.stencilClips(nil, nil)
	r.intersect(nil)
	r.drawLayers(nil, nil)
	if device.live != 0 {
		t.Fatalf("empty passes retained %d framebuffer textures", device.live)
	}
}
