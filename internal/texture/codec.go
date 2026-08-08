package texture

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
)

func encodeImageToBase64(img image.Image) string {
	if img == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodeBase64Image(b64Str string) image.Image {
	if b64Str == "" {
		return nil
	}

	cleanStr := b64Str
	if idx := strings.Index(b64Str, ","); idx != -1 {
		cleanStr = b64Str[idx+1:]
	}

	cleanStr = strings.ReplaceAll(cleanStr, "\r", "")
	cleanStr = strings.ReplaceAll(cleanStr, "\n", "")
	cleanStr = strings.ReplaceAll(cleanStr, " ", "")

	data, err := base64.StdEncoding.DecodeString(cleanStr)
	if err != nil {
		return nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}
