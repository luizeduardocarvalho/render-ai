// Package imageutil holds small shared helpers for decoding uploaded images
// and encoding generated ones, used by both the API layer and the geometry
// package's tests.
package imageutil

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	"image/png"
)

// Decode decodes PNG or JPEG bytes into an image.Image, returning the
// detected format name ("png" or "jpeg").
func Decode(data []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decoding image: %w", err)
	}
	return img, format, nil
}

// EncodePNG encodes img as PNG bytes.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding png: %w", err)
	}
	return buf.Bytes(), nil
}

// ContentTypeForFormat maps an image.Decode format name to a MIME type,
// defaulting to PNG for anything unrecognized.
func ContentTypeForFormat(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	default:
		return "image/png"
	}
}
