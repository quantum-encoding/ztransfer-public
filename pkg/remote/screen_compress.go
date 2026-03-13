//go:build darwin || linux

package remote

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	// Register PNG decoder
	_ "image/png"
)

// CompressScreenshot takes raw PNG screenshot bytes and returns a compressed
// JPEG at the given quality (1-100) and optional downscale factor.
// A downscale of 2 halves both dimensions (e.g. 2940x1912 → 1470x956).
func CompressScreenshot(pngData []byte, quality int, downscale int) ([]byte, error) {
	if quality <= 0 {
		quality = 65
	}
	if downscale <= 0 {
		downscale = 1
	}

	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}

	if downscale > 1 {
		img = downscaleImage(img, downscale)
	}

	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	return buf.Bytes(), nil
}

// downscaleImage reduces image dimensions by the given factor using
// nearest-neighbor sampling. Fast and good enough for screen content.
func downscaleImage(src image.Image, factor int) image.Image {
	bounds := src.Bounds()
	newW := bounds.Dx() / factor
	newH := bounds.Dy() / factor

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	for y := 0; y < newH; y++ {
		srcY := bounds.Min.Y + y*factor
		for x := 0; x < newW; x++ {
			srcX := bounds.Min.X + x*factor
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

// CaptureScreenJPEG captures a screenshot and returns compressed JPEG bytes.
// This is the fast path for the viewer — captures PNG via the OS tool,
// then compresses to JPEG in-process.
func CaptureScreenJPEG(quality int, downscale int) ([]byte, error) {
	pngData, err := captureScreen(ScreenRequest{Format: "png"})
	if err != nil {
		return nil, err
	}

	return CompressScreenshot(pngData, quality, downscale)
}
