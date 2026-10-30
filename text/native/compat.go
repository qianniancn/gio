package text

type TextParams struct {
	// GammaRatios are Direct2D's alpha correction of a gamma (see
	// GammaRatios).
	GammaRatios [4]float32
	// Contrast enhances the contrast of mask glyphs, and SubpixelContrast
	// that of subpixel glyphs, the more the darker their color: DirectWrite's
	// grayscale enhanced contrast and enhanced contrast.
	Contrast, SubpixelContrast float32
}

// GammaRatios returns Direct2D's alpha correction ratios for text blended
// at a gamma between 1 and 2.2, as Windows Terminal computes them.
func GammaRatios(gamma float32) [4]float32 {
	// Microsoft's ratios, for gamma 1.0, 1.1, ... 2.2, divided by 4.
	table := [13][4]float32{
		{0, 0, 0, 0},
		{0.0166, -0.0807, 0.2227, -0.0751},
		{0.0350, -0.1760, 0.4325, -0.1370},
		{0.0543, -0.2821, 0.6302, -0.1876},
		{0.0739, -0.3963, 0.8167, -0.2287},
		{0.0933, -0.5161, 0.9926, -0.2616},
		{0.1121, -0.6395, 1.1588, -0.2877},
		{0.1300, -0.7649, 1.3159, -0.3080},
		{0.1469, -0.8911, 1.4644, -0.3234},
		{0.1627, -1.0170, 1.6051, -0.3347},
		{0.1773, -1.1420, 1.7385, -0.3426},
		{0.1908, -1.2652, 1.8650, -0.3476},
		{0.2031, -1.3864, 1.9851, -0.3501},
	}
	i := min(max(int(gamma*10+0.5), 10), 22) - 10
	const norm13 = float32(float64(0x10000) / (255 * 255) * 4)
	const norm24 = float32(float64(0x100) / 255 * 4)
	r := table[i]
	return [4]float32{norm13 * r[0] / 4, norm24 * r[1] / 4, norm13 * r[2] / 4, norm24 * r[3] / 4}
}

// ThinBoost is the contrast Direct2D adds for thin glyphs.
const ThinBoost = 0.5

type caretStop struct {
	position struct {
		Index    int
		Affinity uint8
	}
	x       float32
	arrival int
}
