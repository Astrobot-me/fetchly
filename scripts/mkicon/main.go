// mkicon builds a multi-size Windows .ico from one large square PNG.
//
// Each size is scaled down by area averaging and stored as PNG data inside
// the .ico, which every browser and Windows Vista+ reads.
//
// Usage: go run scripts/mkicon/main.go -src icon-512.png -o favicon.ico [-sizes 16,32,48,256]
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"
)

func main() {
	src := flag.String("src", "", "square source PNG, at least as large as the biggest size")
	out := flag.String("o", "", "output .ico path")
	sizesFlag := flag.String("sizes", "16,32,48,256", "comma-separated icon sizes (max 256)")
	flag.Parse()
	if *src == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: mkicon -src icon.png -o favicon.ico [-sizes 16,32,48,256]")
		os.Exit(2)
	}
	if err := run(*src, *out, *sizesFlag); err != nil {
		fmt.Fprintln(os.Stderr, "mkicon:", err)
		os.Exit(1)
	}
}

func run(srcPath, outPath, sizesFlag string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	b := img.Bounds()
	if b.Dx() != b.Dy() {
		return fmt.Errorf("source must be square, got %dx%d", b.Dx(), b.Dy())
	}

	var entries [][]byte
	var sizes []int
	for _, s := range strings.Split(sizesFlag, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 || n > 256 || n > b.Dx() {
			return fmt.Errorf("bad size %q (1-256, not larger than the source)", s)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, downscale(img, n)); err != nil {
			return err
		}
		entries = append(entries, buf.Bytes())
		sizes = append(sizes, n)
	}

	var ico bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), image count.
	binary.Write(&ico, binary.LittleEndian, [3]uint16{0, 1, uint16(len(entries))})
	offset := 6 + 16*len(entries)
	for i, data := range entries {
		dim := byte(sizes[i])
		if sizes[i] == 256 {
			dim = 0 // 0 means 256 in an ICONDIRENTRY
		}
		ico.Write([]byte{dim, dim, 0, 0})                         // width, height, palette, reserved
		binary.Write(&ico, binary.LittleEndian, [2]uint16{1, 32}) // planes, bits per pixel
		binary.Write(&ico, binary.LittleEndian, [2]uint32{uint32(len(data)), uint32(offset)})
		offset += len(data)
	}
	for _, data := range entries {
		ico.Write(data)
	}
	return os.WriteFile(outPath, ico.Bytes(), 0o644)
}

// downscale averages each block of source pixels into one output pixel,
// weighting colour by alpha so transparent edges don't darken.
func downscale(src image.Image, n int) *image.NRGBA {
	b := src.Bounds()
	scale := float64(b.Dx()) / float64(n)
	dst := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		y0, y1 := int(float64(y)*scale), int(float64(y+1)*scale)
		for x := 0; x < n; x++ {
			x0, x1 := int(float64(x)*scale), int(float64(x+1)*scale)
			var r, g, bl, a, count float64
			for sy := y0; sy < max(y1, y0+1); sy++ {
				for sx := x0; sx < max(x1, x0+1); sx++ {
					c := color.NRGBAModel.Convert(src.At(b.Min.X+sx, b.Min.Y+sy)).(color.NRGBA)
					al := float64(c.A)
					r += float64(c.R) * al
					g += float64(c.G) * al
					bl += float64(c.B) * al
					a += al
					count++
				}
			}
			if a > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{
					R: uint8(r/a + 0.5), G: uint8(g/a + 0.5), B: uint8(bl/a + 0.5),
					A: uint8(a/count + 0.5),
				})
			}
		}
	}
	return dst
}
