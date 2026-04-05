package proctor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

// encodePNG helps tests round-trip a grayscale image through PNG decoding so
// VerifyNonceInRegion exercises its real decode path.
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// pasteGray draws src onto dst at the given offset. dst is treated as a
// white-background canvas and src's dark pixels become dark pixels on dst.
func pasteGray(dst *image.Gray, src *image.Gray, offX, offY int) {
	sb := src.Bounds()
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			dst.SetGray(offX+x, offY+y, src.GrayAt(sb.Min.X+x, sb.Min.Y+y))
		}
	}
}

// invertGray returns a copy of src with every pixel's luminance inverted.
func invertGray(src *image.Gray) *image.Gray {
	out := image.NewGray(src.Bounds())
	for i, p := range src.Pix {
		out.Pix[i] = 0xFF - p
	}
	return out
}

func TestRenderNonceBoundsABCScale1(t *testing.T) {
	img := RenderNonce("ABC", 1)
	b := img.Bounds()
	// 3 glyphs * 5 wide + 2 spacings = 17 wide; 7 tall.
	if b.Dx() != 17 || b.Dy() != 7 {
		t.Fatalf("unexpected bounds for ABC scale=1: got %dx%d, want 17x7", b.Dx(), b.Dy())
	}
	// Top-left of A is the second column of its first row: bit pattern
	// 0b01110 means (0,0) is white but (1,0) is black.
	if img.GrayAt(0, 0).Y != 0xFF {
		t.Fatalf("expected (0,0) white in 'A' glyph, got %d", img.GrayAt(0, 0).Y)
	}
	if img.GrayAt(1, 0).Y != 0x00 {
		t.Fatalf("expected (1,0) black in 'A' glyph, got %d", img.GrayAt(1, 0).Y)
	}
	// Middle row of A is fully filled (0b11111) at columns 0..4.
	for x := 0; x < 5; x++ {
		if img.GrayAt(x, 3).Y != 0x00 {
			t.Fatalf("expected (%d,3) black in 'A' mid row, got %d", x, img.GrayAt(x, 3).Y)
		}
	}
	// The 1px spacing column between A and B must be fully white at x=5.
	for y := 0; y < 7; y++ {
		if img.GrayAt(5, y).Y != 0xFF {
			t.Fatalf("expected (5,%d) white in A/B gap, got %d", y, img.GrayAt(5, y).Y)
		}
	}
}

func TestRenderNonceScale2Doubles(t *testing.T) {
	img := RenderNonce("ABC", 2)
	b := img.Bounds()
	if b.Dx() != 34 || b.Dy() != 14 {
		t.Fatalf("unexpected bounds for ABC scale=2: got %dx%d, want 34x14", b.Dx(), b.Dy())
	}
	// At scale 2, the black pixel that lived at (1,0) now fills the 2x2
	// block (2..3, 0..1).
	for sy := 0; sy < 2; sy++ {
		for sx := 0; sx < 2; sx++ {
			x := 2 + sx
			y := 0 + sy
			if img.GrayAt(x, y).Y != 0x00 {
				t.Fatalf("scale=2 block (%d,%d) should be black, got %d", x, y, img.GrayAt(x, y).Y)
			}
		}
	}
	// (0,0) was white at scale 1 so its 2x2 block must also be white.
	for sy := 0; sy < 2; sy++ {
		for sx := 0; sx < 2; sx++ {
			if img.GrayAt(sx, sy).Y != 0xFF {
				t.Fatalf("scale=2 block (%d,%d) should be white, got %d", sx, sy, img.GrayAt(sx, sy).Y)
			}
		}
	}
}

func TestRenderNonceEmptyString(t *testing.T) {
	img := RenderNonce("", 1)
	b := img.Bounds()
	if b.Dx() != 0 || b.Dy() != 0 {
		t.Fatalf("empty nonce should produce empty image, got %dx%d", b.Dx(), b.Dy())
	}
	// Scale shouldn't crash or produce ink either.
	img2 := RenderNonce("", 3)
	if img2.Bounds().Dx() != 0 || img2.Bounds().Dy() != 0 {
		t.Fatalf("empty nonce with scale=3 should be empty, got %v", img2.Bounds())
	}
}

func TestRenderNonceNonPositiveScale(t *testing.T) {
	img := RenderNonce("A", 0)
	b := img.Bounds()
	if b.Dx() != 5 || b.Dy() != 7 {
		t.Fatalf("scale<=0 must clamp to 1, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestRenderNonceDeterministic(t *testing.T) {
	a := RenderNonce("XK7Q2M", 2)
	b := RenderNonce("XK7Q2M", 2)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatalf("RenderNonce is not deterministic")
	}
	if a.Bounds() != b.Bounds() {
		t.Fatalf("RenderNonce bounds differ: %v vs %v", a.Bounds(), b.Bounds())
	}
}

func TestVerifyNonceRoundTripExactRegion(t *testing.T) {
	nonce := "XK7Q2M"
	rendered := RenderNonce(nonce, 2)
	pngBytes := encodePNG(t, rendered)

	region := rendered.Bounds()
	if err := VerifyNonceInRegion(pngBytes, nonce, region, 1.0, 2); err != nil {
		t.Fatalf("round-trip exact region failed: %v", err)
	}
}

func TestVerifyNonceInsideWhiteCanvas(t *testing.T) {
	nonce := "PROCTOR"
	rendered := RenderNonce(nonce, 3)

	canvasW := rendered.Bounds().Dx() + 200
	canvasH := rendered.Bounds().Dy() + 200
	canvas := image.NewGray(image.Rect(0, 0, canvasW, canvasH))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0xFF
	}
	offX, offY := 47, 89
	pasteGray(canvas, rendered, offX, offY)

	pngBytes := encodePNG(t, canvas)
	// A generous region that starts above/left of the paste and runs to the
	// canvas edge.
	region := image.Rect(10, 10, canvasW, canvasH)
	if err := VerifyNonceInRegion(pngBytes, nonce, region, 1.0, 3); err != nil {
		t.Fatalf("expected full match in white canvas, got %v", err)
	}
}

func TestVerifyNonceWithNoisyBackground(t *testing.T) {
	nonce := "A1B2C3"
	rendered := RenderNonce(nonce, 2)

	canvasW := rendered.Bounds().Dx() + 120
	canvasH := rendered.Bounds().Dy() + 80
	canvas := image.NewGray(image.Rect(0, 0, canvasW, canvasH))

	// Deterministic pseudo-random noise so the test is reproducible.
	rng := rand.New(rand.NewSource(1337))
	for i := range canvas.Pix {
		// Keep the noise on the lighter side so the binarization still
		// classifies most of it as background. Mixing in occasional dark
		// speckles stresses the "match only ink pixels" strategy.
		if rng.Intn(10) == 0 {
			canvas.Pix[i] = byte(rng.Intn(110)) // dark speckle
		} else {
			canvas.Pix[i] = byte(200 + rng.Intn(55)) // light noise
		}
	}
	offX, offY := 60, 33
	pasteGray(canvas, rendered, offX, offY)

	pngBytes := encodePNG(t, canvas)
	region := image.Rect(0, 0, canvasW, canvasH)
	if err := VerifyNonceInRegion(pngBytes, nonce, region, 0.95, 2); err != nil {
		t.Fatalf("expected nonce to survive noise: %v", err)
	}
}

func TestVerifyNonceWrongNonceFails(t *testing.T) {
	rendered := RenderNonce("ABCDEF", 2)
	pngBytes := encodePNG(t, rendered)
	region := rendered.Bounds()

	err := VerifyNonceInRegion(pngBytes, "ZZZZZZ", region, 0.9, 2)
	if err == nil {
		t.Fatalf("expected mismatch error for wrong nonce")
	}
	if !strings.Contains(err.Error(), "similarity=") {
		t.Fatalf("error should describe best similarity, got %v", err)
	}
	if !strings.Contains(err.Error(), "ZZZZZZ") {
		t.Fatalf("error should name the nonce, got %v", err)
	}
}

func TestVerifyNonceRegionTooSmall(t *testing.T) {
	nonce := "ABCDEF"
	rendered := RenderNonce(nonce, 2)
	// Embed it in a canvas, but pass a tiny region that cannot fit the
	// rendered nonce.
	canvas := image.NewGray(image.Rect(0, 0, 200, 200))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0xFF
	}
	pasteGray(canvas, rendered, 10, 10)
	pngBytes := encodePNG(t, canvas)

	region := image.Rect(0, 0, 5, 5)
	err := VerifyNonceInRegion(pngBytes, nonce, region, 1.0, 2)
	if err == nil {
		t.Fatalf("expected error for tiny region")
	}
	if !strings.Contains(err.Error(), "smaller than template") {
		t.Fatalf("expected size-mismatch error, got %v", err)
	}
}

func TestVerifyNonceInvertedColors(t *testing.T) {
	nonce := "NIGHT1"
	rendered := RenderNonce(nonce, 3)

	// Place the rendered nonce on a dark background, then invert the whole
	// thing so the glyph pixels are white on black.
	canvasW := rendered.Bounds().Dx() + 80
	canvasH := rendered.Bounds().Dy() + 40
	canvas := image.NewGray(image.Rect(0, 0, canvasW, canvasH))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0xFF
	}
	pasteGray(canvas, rendered, 20, 15)
	inverted := invertGray(canvas)

	pngBytes := encodePNG(t, inverted)
	region := image.Rect(0, 0, canvasW, canvasH)
	if err := VerifyNonceInRegion(pngBytes, nonce, region, 1.0, 3); err != nil {
		t.Fatalf("inverted-colors match should succeed: %v", err)
	}
}

func TestVerifyNonceEmptyAndBadInputs(t *testing.T) {
	if err := VerifyNonceInRegion(nil, "", image.Rect(0, 0, 10, 10), 1.0, 1); err == nil {
		t.Fatalf("expected error for empty nonce")
	}
	if err := VerifyNonceInRegion(nil, "A", image.Rect(0, 0, 10, 10), 1.5, 1); err == nil {
		t.Fatalf("expected error for tolerance out of range")
	}
	if err := VerifyNonceInRegion([]byte("not a png"), "A", image.Rect(0, 0, 10, 10), 1.0, 1); err == nil {
		t.Fatalf("expected decode error for garbage bytes")
	}
}

// TestRenderAllGlyphsMatch exercises every character in the supported font.
// For each character we render it alone, encode to PNG, and verify.
func TestRenderAllGlyphsMatch(t *testing.T) {
	// Assemble the supported character set from font5x7 so the test tracks
	// the implementation automatically.
	chars := make([]rune, 0, len(font5x7))
	for r := range font5x7 {
		chars = append(chars, r)
	}

	for _, r := range chars {
		r := r
		name := string(r)
		if r == ' ' {
			name = "space"
		}
		t.Run("glyph_"+name, func(t *testing.T) {
			rendered := RenderNonce(string(r), 2)
			// Space has no ink pixels, so we cannot template-match it. Just
			// assert bounds.
			if r == ' ' {
				b := rendered.Bounds()
				if b.Dx() != glyphWidth*2 || b.Dy() != glyphHeight*2 {
					t.Fatalf("space glyph bounds wrong: %v", b)
				}
				return
			}
			pngBytes := encodePNG(t, rendered)
			region := rendered.Bounds()
			if err := VerifyNonceInRegion(pngBytes, string(r), region, 1.0, 2); err != nil {
				t.Fatalf("glyph %q failed to self-verify: %v", name, err)
			}
		})
	}
}

// TestVerifyNonceProctorFormat exercises the "proctor:NONCE" shape that the
// surface integrations render on overlays.
func TestVerifyNonceProctorFormat(t *testing.T) {
	text := "PROCTOR:XK7Q2M"
	rendered := RenderNonce(text, 2)

	canvasW := rendered.Bounds().Dx() + 40
	canvasH := rendered.Bounds().Dy() + 40
	canvas := image.NewGray(image.Rect(0, 0, canvasW, canvasH))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0xFF
	}
	pasteGray(canvas, rendered, 12, 8)
	pngBytes := encodePNG(t, canvas)

	region := image.Rect(0, 0, canvasW, canvasH)
	if err := VerifyNonceInRegion(pngBytes, text, region, 1.0, 2); err != nil {
		t.Fatalf("proctor: format match failed: %v", err)
	}
}

// TestVerifyNonceRegionClipping passes a region that extends far beyond the
// image bounds; the implementation must clip gracefully and still find the
// nonce.
func TestVerifyNonceRegionClipping(t *testing.T) {
	nonce := "CLIP01"
	rendered := RenderNonce(nonce, 2)

	canvasW := rendered.Bounds().Dx() + 20
	canvasH := rendered.Bounds().Dy() + 20
	canvas := image.NewGray(image.Rect(0, 0, canvasW, canvasH))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0xFF
	}
	pasteGray(canvas, rendered, 10, 10)
	pngBytes := encodePNG(t, canvas)

	// Far bigger than the image.
	region := image.Rect(-100, -100, 10000, 10000)
	if err := VerifyNonceInRegion(pngBytes, nonce, region, 1.0, 2); err != nil {
		t.Fatalf("oversize region should be clipped and match, got %v", err)
	}
}

// TestLuminanceHandlesColor ensures the luminance helper uses the standard
// grayscale conversion so callers rendering colored PNGs still get sensible
// binarization.
func TestLuminanceHandlesColor(t *testing.T) {
	black := luminance(color.RGBA{R: 0, G: 0, B: 0, A: 255})
	if black >= 128 {
		t.Fatalf("black luminance should be <128, got %d", black)
	}
	white := luminance(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if white < 128 {
		t.Fatalf("white luminance should be >=128, got %d", white)
	}
}
