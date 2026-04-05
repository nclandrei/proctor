package proctor

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// Pixel-verification primitives for proctor's nonce ritual.
//
// proctor plants a short alphanumeric nonce on the verified surface (browser
// page, simulator overlay, terminal prompt) and must later prove that the nonce
// actually appears in the captured screenshot. A trusted OCR stack is overkill
// for 6-char fixed-vocabulary nonces, so this package ships a deterministic
// 5x7 bitmap font renderer plus a template matcher. Callers render the
// reference via RenderNonce and scan the captured PNG via VerifyNonceInRegion.
//
// Zero external dependencies: only image, image/color, image/png, bytes, fmt,
// errors.

// glyphWidth is the pixel width of every glyph in the built-in 5x7 font.
const glyphWidth = 5

// glyphHeight is the pixel height of every glyph in the built-in 5x7 font.
const glyphHeight = 7

// glyphSpacing is the gap in logical pixels inserted between adjacent glyphs
// when rendering a string. One column of whitespace disambiguates touching
// glyphs on noisy backgrounds.
const glyphSpacing = 1

// font5x7 is the built-in monochrome bitmap font. Each glyph is seven bytes,
// one per row, top-to-bottom. Only the low five bits of each byte are used;
// bit 4 (MSB of the pixel field) is the leftmost pixel, bit 0 is the
// rightmost. A set bit draws black (0x00) on a white (0xFF) background.
//
// The covered character set is deliberately constrained to what proctor
// nonces use today: uppercase letters, digits, colon, dash, underscore, and
// space. An unknown rune is rendered as a solid block so that verification
// still has something to match against and the caller can tell at a glance
// that the nonce contains an unsupported character.
var font5x7 = map[rune][glyphHeight]byte{
	' ': {
		0b00000,
		0b00000,
		0b00000,
		0b00000,
		0b00000,
		0b00000,
		0b00000,
	},
	':': {
		0b00000,
		0b00000,
		0b00100,
		0b00000,
		0b00100,
		0b00000,
		0b00000,
	},
	'-': {
		0b00000,
		0b00000,
		0b00000,
		0b11111,
		0b00000,
		0b00000,
		0b00000,
	},
	'_': {
		0b00000,
		0b00000,
		0b00000,
		0b00000,
		0b00000,
		0b00000,
		0b11111,
	},
	'0': {
		0b01110,
		0b10001,
		0b10011,
		0b10101,
		0b11001,
		0b10001,
		0b01110,
	},
	'1': {
		0b00100,
		0b01100,
		0b00100,
		0b00100,
		0b00100,
		0b00100,
		0b01110,
	},
	'2': {
		0b01110,
		0b10001,
		0b00001,
		0b00010,
		0b00100,
		0b01000,
		0b11111,
	},
	'3': {
		0b11111,
		0b00010,
		0b00100,
		0b00010,
		0b00001,
		0b10001,
		0b01110,
	},
	'4': {
		0b00010,
		0b00110,
		0b01010,
		0b10010,
		0b11111,
		0b00010,
		0b00010,
	},
	'5': {
		0b11111,
		0b10000,
		0b11110,
		0b00001,
		0b00001,
		0b10001,
		0b01110,
	},
	'6': {
		0b00110,
		0b01000,
		0b10000,
		0b11110,
		0b10001,
		0b10001,
		0b01110,
	},
	'7': {
		0b11111,
		0b00001,
		0b00010,
		0b00100,
		0b01000,
		0b01000,
		0b01000,
	},
	'8': {
		0b01110,
		0b10001,
		0b10001,
		0b01110,
		0b10001,
		0b10001,
		0b01110,
	},
	'9': {
		0b01110,
		0b10001,
		0b10001,
		0b01111,
		0b00001,
		0b00010,
		0b01100,
	},
	'A': {
		0b01110,
		0b10001,
		0b10001,
		0b11111,
		0b10001,
		0b10001,
		0b10001,
	},
	'B': {
		0b11110,
		0b10001,
		0b10001,
		0b11110,
		0b10001,
		0b10001,
		0b11110,
	},
	'C': {
		0b01110,
		0b10001,
		0b10000,
		0b10000,
		0b10000,
		0b10001,
		0b01110,
	},
	'D': {
		0b11110,
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b11110,
	},
	'E': {
		0b11111,
		0b10000,
		0b10000,
		0b11110,
		0b10000,
		0b10000,
		0b11111,
	},
	'F': {
		0b11111,
		0b10000,
		0b10000,
		0b11110,
		0b10000,
		0b10000,
		0b10000,
	},
	'G': {
		0b01110,
		0b10001,
		0b10000,
		0b10111,
		0b10001,
		0b10001,
		0b01111,
	},
	'H': {
		0b10001,
		0b10001,
		0b10001,
		0b11111,
		0b10001,
		0b10001,
		0b10001,
	},
	'I': {
		0b01110,
		0b00100,
		0b00100,
		0b00100,
		0b00100,
		0b00100,
		0b01110,
	},
	'J': {
		0b00111,
		0b00010,
		0b00010,
		0b00010,
		0b00010,
		0b10010,
		0b01100,
	},
	'K': {
		0b10001,
		0b10010,
		0b10100,
		0b11000,
		0b10100,
		0b10010,
		0b10001,
	},
	'L': {
		0b10000,
		0b10000,
		0b10000,
		0b10000,
		0b10000,
		0b10000,
		0b11111,
	},
	'M': {
		0b10001,
		0b11011,
		0b10101,
		0b10101,
		0b10001,
		0b10001,
		0b10001,
	},
	'N': {
		0b10001,
		0b10001,
		0b11001,
		0b10101,
		0b10011,
		0b10001,
		0b10001,
	},
	'O': {
		0b01110,
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b01110,
	},
	'P': {
		0b11110,
		0b10001,
		0b10001,
		0b11110,
		0b10000,
		0b10000,
		0b10000,
	},
	'Q': {
		0b01110,
		0b10001,
		0b10001,
		0b10001,
		0b10101,
		0b10010,
		0b01101,
	},
	'R': {
		0b11110,
		0b10001,
		0b10001,
		0b11110,
		0b10100,
		0b10010,
		0b10001,
	},
	'S': {
		0b01111,
		0b10000,
		0b10000,
		0b01110,
		0b00001,
		0b00001,
		0b11110,
	},
	'T': {
		0b11111,
		0b00100,
		0b00100,
		0b00100,
		0b00100,
		0b00100,
		0b00100,
	},
	'U': {
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b01110,
	},
	'V': {
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b10001,
		0b01010,
		0b00100,
	},
	'W': {
		0b10001,
		0b10001,
		0b10001,
		0b10101,
		0b10101,
		0b10101,
		0b01010,
	},
	'X': {
		0b10001,
		0b10001,
		0b01010,
		0b00100,
		0b01010,
		0b10001,
		0b10001,
	},
	'Y': {
		0b10001,
		0b10001,
		0b10001,
		0b01010,
		0b00100,
		0b00100,
		0b00100,
	},
	'Z': {
		0b11111,
		0b00001,
		0b00010,
		0b00100,
		0b01000,
		0b10000,
		0b11111,
	},
}

// unknownGlyph is rendered for runes not in font5x7. A solid block keeps the
// image size predictable and still gives the template matcher a strong
// signal, which is good enough because proctor nonces are drawn from a fixed
// vocabulary and unknown chars always indicate a caller mistake.
var unknownGlyph = [glyphHeight]byte{
	0b11111,
	0b11111,
	0b11111,
	0b11111,
	0b11111,
	0b11111,
	0b11111,
}

// colorBlack is the "ink" color for rendered glyphs.
var colorBlack = color.Gray{Y: 0x00}

// colorWhite is the background color for rendered glyphs.
var colorWhite = color.Gray{Y: 0xFF}

// glyphFor returns the bitmap for the given rune, falling back to
// unknownGlyph for characters outside the supported set.
func glyphFor(r rune) [glyphHeight]byte {
	if g, ok := font5x7[r]; ok {
		return g
	}
	return unknownGlyph
}

// RenderNonce renders a text nonce as a monochrome bitmap using the built-in
// 5x7 font. scale enlarges each logical pixel to scale-by-scale physical
// pixels via nearest-neighbor upscaling. The returned image is grayscale with
// black ink on a white background.
//
// An empty text returns an empty image (bounds 0x0). A non-positive scale is
// clamped to 1 so callers never get a zero-area image for non-empty text.
func RenderNonce(text string, scale int) *image.Gray {
	if scale < 1 {
		scale = 1
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return image.NewGray(image.Rect(0, 0, 0, 0))
	}

	logicalWidth := len(runes)*glyphWidth + (len(runes)-1)*glyphSpacing
	logicalHeight := glyphHeight
	pixelWidth := logicalWidth * scale
	pixelHeight := logicalHeight * scale

	img := image.NewGray(image.Rect(0, 0, pixelWidth, pixelHeight))
	// Start with a white background; ink is painted on top.
	for i := range img.Pix {
		img.Pix[i] = colorWhite.Y
	}

	for charIdx, r := range runes {
		glyph := glyphFor(r)
		xOffset := charIdx * (glyphWidth + glyphSpacing)
		for row := 0; row < glyphHeight; row++ {
			rowBits := glyph[row]
			for col := 0; col < glyphWidth; col++ {
				// bit 4 is the leftmost pixel.
				if rowBits&(1<<(glyphWidth-1-col)) == 0 {
					continue
				}
				// Scale this logical pixel into a scale x scale block.
				baseX := (xOffset + col) * scale
				baseY := row * scale
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						img.SetGray(baseX+sx, baseY+sy, colorBlack)
					}
				}
			}
		}
	}

	return img
}

// VerifyNonceInRegion scans the given region of the PNG-encoded pngData for
// the rendered nonce. It renders a reference via RenderNonce(nonce, scale),
// binarizes the region, and template-matches by sliding the reference across
// the region one pixel at a time. The best similarity is compared against
// tolerance.
//
// Similarity is defined as (ink pixels matched) / (total ink pixels in the
// template). Only the template's set pixels contribute, so a background that
// is mostly one color does not inflate the score. Both a direct and an
// inverted binarization of the region are tried so that light-on-dark and
// dark-on-light surfaces are handled symmetrically.
//
// Returns nil if the best similarity is >= tolerance. Otherwise returns an
// error describing the best similarity achieved and the region offset at
// which it was observed.
func VerifyNonceInRegion(pngData []byte, nonce string, region image.Rectangle, tolerance float64, scale int) error {
	if nonce == "" {
		return errors.New("pixelverify: nonce is empty")
	}
	if tolerance < 0 || tolerance > 1 {
		return fmt.Errorf("pixelverify: tolerance %.2f out of range [0,1]", tolerance)
	}
	if scale < 1 {
		scale = 1
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return fmt.Errorf("pixelverify: decode png: %w", err)
	}

	template := RenderNonce(nonce, scale)
	tplBounds := template.Bounds()
	tplW := tplBounds.Dx()
	tplH := tplBounds.Dy()
	if tplW == 0 || tplH == 0 {
		return errors.New("pixelverify: template is empty")
	}

	// Collect the template's ink pixel offsets once; every search position
	// reuses the same set.
	inkOffsets := collectInkOffsets(template)
	totalInk := len(inkOffsets)
	if totalInk == 0 {
		return errors.New("pixelverify: template has no ink pixels")
	}

	// Clip the caller-supplied region to the actual image bounds before
	// walking it so callers can pass generous rectangles.
	clipped := region.Intersect(img.Bounds())
	regionW := clipped.Dx()
	regionH := clipped.Dy()
	if regionW < tplW || regionH < tplH {
		return fmt.Errorf(
			"pixelverify: region %dx%d smaller than template %dx%d",
			regionW, regionH, tplW, tplH,
		)
	}

	// Binarize both polarities of the region up front so the inner search
	// loop is tight. direct[y*regionW+x] == true means that pixel was dark in
	// the original (luminance < 128). inverted is the complement.
	direct, inverted := binarizeRegion(img, clipped)

	bestDirect, bestDirectX, bestDirectY := scanRegion(direct, regionW, regionH, tplW, tplH, inkOffsets)
	bestInverted, bestInvertedX, bestInvertedY := scanRegion(inverted, regionW, regionH, tplW, tplH, inkOffsets)

	best := bestDirect
	bestX := bestDirectX
	bestY := bestDirectY
	polarity := "direct"
	if bestInverted > best {
		best = bestInverted
		bestX = bestInvertedX
		bestY = bestInvertedY
		polarity = "inverted"
	}

	similarity := float64(best) / float64(totalInk)
	if similarity >= tolerance {
		return nil
	}
	// Translate back into image coordinates so the caller sees an offset they
	// can correlate with their own region.
	absX := clipped.Min.X + bestX
	absY := clipped.Min.Y + bestY
	return fmt.Errorf(
		"pixelverify: nonce %q not found (best similarity=%.2f at (%d,%d) polarity=%s tolerance=%.2f)",
		nonce, similarity, absX, absY, polarity, tolerance,
	)
}

// collectInkOffsets returns the (dx,dy) positions of every ink pixel in the
// rendered template. Using a flat slice keeps the hot loop in scanRegion
// branch-free and cache friendly.
func collectInkOffsets(tpl *image.Gray) []image.Point {
	b := tpl.Bounds()
	offsets := make([]image.Point, 0, b.Dx()*b.Dy()/4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if tpl.GrayAt(x, y).Y < 128 {
				offsets = append(offsets, image.Point{X: x - b.Min.X, Y: y - b.Min.Y})
			}
		}
	}
	return offsets
}

// binarizeRegion walks the rectangle and returns two bitmaps (one per pixel)
// of length regionW*regionH. direct marks pixels whose luminance is below
// 128; inverted is the logical NOT.
func binarizeRegion(img image.Image, region image.Rectangle) (direct, inverted []bool) {
	w := region.Dx()
	h := region.Dy()
	direct = make([]bool, w*h)
	inverted = make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.At(region.Min.X+x, region.Min.Y+y)
			lum := luminance(c)
			idx := y*w + x
			if lum < 128 {
				direct[idx] = true
			} else {
				inverted[idx] = true
			}
		}
	}
	return direct, inverted
}

// luminance returns a 0..255 luminance value using Rec. 601 coefficients.
// The conversion goes through color.GrayModel so we honor image packages that
// already know how to desaturate their own colors.
func luminance(c color.Color) int {
	g := color.GrayModel.Convert(c).(color.Gray)
	return int(g.Y)
}

// scanRegion slides the ink mask over the binarized region and returns the
// best match count and its top-left offset. Returns (0,0,0) if the template
// could not be placed anywhere inside the region.
func scanRegion(region []bool, regionW, regionH, tplW, tplH int, inkOffsets []image.Point) (best, bestX, bestY int) {
	if regionW < tplW || regionH < tplH {
		return 0, 0, 0
	}
	maxDY := regionH - tplH
	maxDX := regionW - tplW
	for dy := 0; dy <= maxDY; dy++ {
		for dx := 0; dx <= maxDX; dx++ {
			matches := 0
			for _, off := range inkOffsets {
				rx := dx + off.X
				ry := dy + off.Y
				if region[ry*regionW+rx] {
					matches++
				}
			}
			if matches > best {
				best = matches
				bestX = dx
				bestY = dy
			}
		}
	}
	return best, bestX, bestY
}
