package api

import (
	"bytes"
	"image"

	"render-ai/backend/internal/imageutil"
)

// decodeImageConfig reads just the dimensions/format of an uploaded image
// without fully decoding pixels, validating that it is a real PNG/JPEG.
func decodeImageConfig(data []byte) (image.Config, string, error) {
	return image.DecodeConfig(bytes.NewReader(data))
}

func contentTypeForFormat(format string) string {
	return imageutil.ContentTypeForFormat(format)
}
