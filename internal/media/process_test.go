package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/image/webp"
)

func testImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	return img
}

// jpegWithEXIF splices a fake APP1/Exif segment after SOI — enough to assert
// that re-encoding drops it.
func jpegWithEXIF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, testImage(800, 600), nil))
	b := buf.Bytes()
	payload := []byte("Exif\x00\x00FAKEMETA")                 // 14 bytes
	seg := append([]byte{0xFF, 0xE1, 0x00, 0x10}, payload...) // len = 14 + 2
	out := append([]byte{0xFF, 0xD8}, append(seg, b[2:]...)...)
	require.True(t, bytes.Contains(out, []byte("FAKEMETA")))
	return out
}

func TestProcessProducesDecodableVariantsAndStripsEXIF(t *testing.T) {
	variants, err := ProcessImage(jpegWithEXIF(t))
	require.NoError(t, err)
	require.Len(t, variants, 3)

	for _, name := range []string{"thumb", "feed", "orig"} {
		data := variants[name]
		require.NotEmpty(t, data, name)
		require.False(t, bytes.Contains(data, []byte("FAKEMETA")),
			"%s: re-encode must strip metadata (§7)", name)
		img, err := webp.Decode(bytes.NewReader(data))
		require.NoError(t, err, "%s must be valid webp", name)
		switch name {
		case "thumb":
			require.LessOrEqual(t, img.Bounds().Dx(), 150)
		case "feed":
			require.LessOrEqual(t, img.Bounds().Dx(), 600)
		case "orig":
			require.Equal(t, 800, img.Bounds().Dx())
		}
	}
}

func TestProcessAcceptsPNGAndDoesNotUpscale(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, testImage(100, 80)))
	variants, err := ProcessImage(buf.Bytes())
	require.NoError(t, err)
	img, err := webp.Decode(bytes.NewReader(variants["feed"]))
	require.NoError(t, err)
	require.Equal(t, 100, img.Bounds().Dx(), "small images are not upscaled")
}

func TestProcessRejectsNonImages(t *testing.T) {
	_, err := ProcessImage([]byte("%PDF-1.4 not an image"))
	require.ErrorIs(t, err, ErrNotImage)
}
