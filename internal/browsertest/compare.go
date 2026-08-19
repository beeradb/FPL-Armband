package browsertest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// JustAntialiasing is the largest per-channel difference a comparison ignores.
//
// It is 1 — the least significant bit of an 8-bit channel — and it is not a fuzz factor
// added to make a failing test pass. The first version of this comparison had no tolerance
// at all, on the reasoning that pinned inputs must produce identical pixels. That is right
// about the DATA and wrong about the RENDERER: text rasterisation in a real browser is not
// bit-reproducible between runs, and the suite duly failed with 66 pixels of 1,014,000
// differing by exactly 1.
//
// ⚠️ If this needs raising, that is evidence about the harness. Find what is moving.
const JustAntialiasing = 1

// NoiseFloor is how many pixels may differ by more than JustAntialiasing before a
// comparison calls it a change.
//
// The two guards do different jobs and neither replaces the other: the threshold forgives a
// shade, this forgives an isolated speck. Raising the threshold instead would have been the
// wrong fix — a uniform two-level shift across a whole panel is a real regression, and a
// threshold of 2 swallows it silently while a floor still catches it, because it has area.
//
// Sized from observed noise. Before the compositor was waited for, a run differed in 66
// pixels at delta 1; after, in a single pixel at delta 2.
const NoiseFloor = 32

// Diff is the result of comparing two screenshots.
type Diff struct {
	Differing, Total int
	Worst            int
	// Resized is set when the two images are not the same size, and is separate from the
	// pixel counts on purpose: it must not be comparable against the noise floor. It used
	// to travel as a differing count of 1, which failed correctly against a zero-tolerance
	// caller and then passed silently the moment a floor of 32 was added.
	Resized bool
	// Image is a PNG showing where they differ: the new render, dimmed, with the changed
	// pixels lit. Nil when they match.
	Image []byte
}

// OK reports whether the two images should be treated as the same.
func (d Diff) OK() bool { return !d.Resized && d.Differing <= NoiseFloor }

// Compare decodes two PNGs and reports where they differ, producing a third that shows it.
//
// Pure standard library: image/png is in the toolchain and a screenshot differ is not worth
// a dependency.
func Compare(wantPNG, gotPNG []byte) (Diff, error) {
	want, err := png.Decode(bytes.NewReader(wantPNG))
	if err != nil {
		return Diff{}, fmt.Errorf("decoding the golden: %w", err)
	}
	got, err := png.Decode(bytes.NewReader(gotPNG))
	if err != nil {
		return Diff{}, fmt.Errorf("decoding the screenshot: %w", err)
	}

	wb, gb := want.Bounds(), got.Bounds()
	if wb != gb {
		return Diff{Resized: true, Image: gotPNG}, nil
	}

	out := image.NewRGBA(wb)
	d := Diff{Total: wb.Dx() * wb.Dy()}
	for y := wb.Min.Y; y < wb.Max.Y; y++ {
		for x := wb.Min.X; x < wb.Max.X; x++ {
			wr, wg, wbl, _ := want.At(x, y).RGBA()
			gr, gg, gbl, _ := got.At(x, y).RGBA()
			delta := max3(abs(int(wr)-int(gr)), abs(int(wg)-int(gg)), abs(int(wbl)-int(gbl))) >> 8
			if delta <= JustAntialiasing {
				// Unchanged pixels are dimmed rather than dropped, so the diff reads as
				// the page with the changes lit up on it.
				r, g, b, _ := got.At(x, y).RGBA()
				out.Set(x, y, color.RGBA{uint8(r >> 10), uint8(g >> 10), uint8(b >> 10), 255})
				continue
			}
			d.Differing++
			if delta > d.Worst {
				d.Worst = delta
			}
			out.Set(x, y, color.RGBA{255, 0, 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return d, err
	}
	d.Image = buf.Bytes()
	return d, nil
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func max3(a, b, c int) int {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}
