package images

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestDecodePNG(t *testing.T) {
	decoded, err := Decode(testPNG(t, 640, 480))
	require.NoError(t, err)
	require.Equal(t, "image/png", decoded.MIMEType)
	require.Equal(t, 640, decoded.Width)
	require.Equal(t, 480, decoded.Height)
}

func TestDecodeJPEG(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 20, 10)), nil))
	decoded, err := Decode(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", decoded.MIMEType)
}

func TestDecodeRejectsInvalidData(t *testing.T) {
	_, err := Decode([]byte("not an image"))
	require.Error(t, err)
	_, err = Decode([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	require.Error(t, err)
}

func TestDecodeAll(t *testing.T) {
	decoded, data, err := DecodeAll(bytes.NewReader(testPNG(t, 10, 10)))
	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.Equal(t, 10, decoded.Width)
}

func TestThumbnailDimensions(t *testing.T) {
	decoded, err := Decode(testPNG(t, 640, 480))
	require.NoError(t, err)
	for _, size := range []int{100, 300} {
		data, err := Thumbnail(decoded.Image, size, size)
		require.NoError(t, err)
		thumbnail, err := Decode(data)
		require.NoError(t, err)
		require.Equal(t, "image/jpeg", thumbnail.MIMEType)
		require.Equal(t, size, thumbnail.Width)
		require.Equal(t, size, thumbnail.Height)
	}
}
