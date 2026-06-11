package media

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // registered decoders: only the three allowed formats
	_ "image/png"

	"github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

var ErrNotImage = errors.New("media: payload is not a supported image")

var variantWidths = map[string]int{"thumb": 150, "feed": 600, "orig": 0} // 0 = original size

// ProcessImage decodes, resizes, and re-encodes the upload as WebP variants.
// Re-encoding is the security boundary (§7): it strips EXIF (incl. GPS) and
// destroys polyglot-file payloads. Deviation #3: pure-Go lossless encoder.
func ProcessImage(raw []byte) (map[string][]byte, error) {
	src, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrNotImage
	}
	switch format {
	case "jpeg", "png", "webp":
	default:
		return nil, ErrNotImage
	}

	out := make(map[string][]byte, len(variantWidths))
	for name, width := range variantWidths {
		img := src
		if width > 0 && src.Bounds().Dx() > width {
			img = resizeToWidth(src, width)
		}
		var buf bytes.Buffer
		if err := nativewebp.Encode(&buf, img, nil); err != nil {
			return nil, err
		}
		out[name] = buf.Bytes()
	}
	return out, nil
}

func resizeToWidth(src image.Image, width int) image.Image {
	b := src.Bounds()
	height := b.Dy() * width / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}
