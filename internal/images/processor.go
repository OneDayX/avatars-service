package images

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

var SupportedMIMEs = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

type Decoded struct {
	Image    image.Image
	MIMEType string
	Width    int
	Height   int
}

func Decode(data []byte) (Decoded, error) {
	mimeType := http.DetectContentType(data)
	if !SupportedMIMEs[mimeType] {
		return Decoded{}, fmt.Errorf("unsupported media type %q", mimeType)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Decoded{}, fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return Decoded{}, fmt.Errorf("image has invalid dimensions")
	}
	return Decoded{Image: img, MIMEType: mimeType, Width: bounds.Dx(), Height: bounds.Dy()}, nil
}

func DecodeAll(r io.Reader) (Decoded, []byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Decoded{}, nil, err
	}
	decoded, err := Decode(data)
	return decoded, data, err
}

func Thumbnail(img image.Image, width, height int) ([]byte, error) {
	resized := imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)
	var output bytes.Buffer
	if err := imaging.Encode(&output, resized, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w", err)
	}
	return output.Bytes(), nil
}
