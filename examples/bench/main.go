// Copyright 2016 Hajime Hoshi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten"
	"github.com/hajimehoshi/ebiten/ebitenutil"
	"github.com/hajimehoshi/ebiten/examples/resources/images"
	emath "github.com/hajimehoshi/ebiten/internal/math"
	"github.com/hajimehoshi/ebiten/internal/testflock"
)

var noErr = errors.New("clean")

type benchmark struct {
	name   string
	fn     func(b *testing.B, screen *ebiten.Image)
	result testing.BenchmarkResult
}

func main() {
	flag.Parse() // so -test.benchtime can work
	testflock.Lock()
	defer testflock.Unlock()

	// Run an Ebiten process so that (*Image).At is available.
	f := func(screen *ebiten.Image) error {
		return benchmarks(screen)
	}
	if err := ebiten.Run(f, 320, 240, 1, "Test"); err != nil && err != noErr {
		panic(err)
	}
	for _, b := range benchList {
		fmt.Printf("%s\n", b.name)
		fmt.Println(b.result)
		fmt.Println(b.result.MemString())
	}
}

var (
	px26  *ebiten.Image
	px104 *ebiten.Image
	px416 *ebiten.Image
)

var benchList = []benchmark{
	{
		name: "draw26",
		fn: func(b *testing.B, screen *ebiten.Image) {
			for i := 0; i < b.N; i++ {
				screen.DrawImage(px26, nil)
			}
		},
	},
	{
		name: "draw104",
		fn: func(b *testing.B, screen *ebiten.Image) {
			for i := 0; i < b.N; i++ {
				screen.DrawImage(px104, nil)
			}
		},
	},
	{
		name: "draw416",
		fn: func(b *testing.B, screen *ebiten.Image) {
			for i := 0; i < b.N; i++ {
				screen.DrawImage(px416, nil)
			}
		},
	},
	{
		name: "draw26color1",
		fn: func(b *testing.B, screen *ebiten.Image) {
			ops := []*ebiten.DrawImageOptions{&ebiten.DrawImageOptions{}, &ebiten.DrawImageOptions{}}
			ops[0].ColorM.Scale(1.0, 0.2, 0.2, 1.0)
			ops[1].ColorM.Scale(0.2, 1.0, 0.2, 1.0)
			for i := 0; i < b.N; i++ {
				idx := i
				ops[idx%2].GeoM.Reset()
				ops[idx%2].GeoM.Translate(float64(i*320/b.N), float64(i*200/b.N))
				screen.DrawImage(px26, ops[idx%2])
			}
		},
	},
	{
		name: "draw26color10",
		fn: func(b *testing.B, screen *ebiten.Image) {
			ops := []*ebiten.DrawImageOptions{&ebiten.DrawImageOptions{}, &ebiten.DrawImageOptions{}}
			ops[0].ColorM.Scale(1.0, 0.2, 0.2, 1.0)
			ops[1].ColorM.Scale(0.2, 1.0, 0.2, 1.0)
			for i := 0; i < b.N; i++ {
				idx := i / 10
				ops[idx%2].GeoM.Reset()
				ops[idx%2].GeoM.Translate(float64(i*320/b.N), float64(i*200/b.N))
				screen.DrawImage(px26, ops[idx%2])
			}
		},
	},
}

var (
	benchIdx  = 0
	results   []testing.BenchmarkResult
	didSetup  sync.Once
	benchFunc func(b *testing.B, screen *ebiten.Image)
	benchGo   = make(chan struct{})
	benchDone = make(chan bool)
)

func benchmarks(screen *ebiten.Image) error {
	var setupErr error

	didSetup.Do(func() {
		eimg, _, err := openEbitenImage()
		if err != nil {
			setupErr = fmt.Errorf("can't open image: %s", err)
			return
		}
		px26 = eimg

		w, h := eimg.Size()
		img, err := ebiten.NewImage(w*4, h*4, ebiten.FilterNearest)
		if err != nil {
			setupErr = fmt.Errorf("can't create ne image: %s", err)
			return
		}
		px104 = img

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(4, 4)
		px104.DrawImage(px26, op)

		img, err = ebiten.NewImage(w*16, h*16, ebiten.FilterNearest)
		if err != nil {
			setupErr = fmt.Errorf("can't create ne image: %s", err)
			return
		}
		px416 = img
		op.GeoM.Scale(4, 4)
		px416.DrawImage(px26, op)
	})
	if setupErr != nil {
		return setupErr
	}

	if benchFunc != nil {
		benchGo <- struct{}{}
		if <-benchDone {
			benchFunc = nil
		}
	} else {
		if len(benchList) > benchIdx {
			benchFunc = benchList[benchIdx].fn
			go func(idx int) {
				benchList[idx].result = testing.Benchmark(func(b *testing.B) {
					b.StopTimer()
					ebiten.SetBenchmark(b)
					var elapsed time.Duration
					// run for at least 60 frames, so the FPS value should mean something
					for i := 0; i < 60; i++ {
						<-benchGo
						t0 := time.Now()
						b.StartTimer()
						benchFunc(b, screen)
						b.StopTimer()
						t1 := time.Now()
						elapsed += t1.Sub(t0)
						benchDone <- false
					}
					fmt.Printf("%16s: N %5d, FPS: %5.2f, time %10v/60 frames\n", benchList[idx].name, b.N, ebiten.CurrentFPS(), elapsed)
					ebiten.SetBenchmark(nil)
				})
				<-benchGo
				benchDone <- true
			}(benchIdx)
			benchIdx++
		} else {
			return noErr
		}
	}
	return nil
}

func openEbitenImage() (*ebiten.Image, image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(images.Ebiten_png))
	if err != nil {
		return nil, nil, err
	}

	eimg, err := ebiten.NewImageFromImage(img, ebiten.FilterNearest)
	if err != nil {
		return nil, nil, err
	}
	return eimg, img, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// sameColors compares c1 and c2 and returns a boolean value indicating
// if the two colors are (almost) same.
//
// Pixels read from GPU might include errors (#492), and
// sameColors considers such errors as delta.
func sameColors(c1, c2 color.RGBA, delta int) bool {
	return abs(int(c1.R)-int(c2.R)) <= delta &&
		abs(int(c1.G)-int(c2.G)) <= delta &&
		abs(int(c1.B)-int(c2.B)) <= delta &&
		abs(int(c1.A)-int(c2.A)) <= delta
}

func TestImagePixels(t *testing.T) {
	img0, img, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}

	if got := img0.Bounds().Size(); got != img.Bounds().Size() {
		t.Fatalf("img size: got %d; want %d", got, img.Bounds().Size())
	}

	w, h := img0.Bounds().Size().X, img0.Bounds().Size().Y
	// Check out of range part
	w2, h2 := emath.NextPowerOf2Int(w), emath.NextPowerOf2Int(h)
	for j := -100; j < h2+100; j++ {
		for i := -100; i < w2+100; i++ {
			got := img0.At(i, j)
			want := color.RGBAModel.Convert(img.At(i, j))
			if got != want {
				t.Errorf("img0 At(%d, %d): got %#v; want %#v", i, j, got, want)
			}
		}
	}
}

func TestImageComposition(t *testing.T) {
	img2Color := color.NRGBA{0x24, 0x3f, 0x6a, 0x88}
	img3Color := color.NRGBA{0x85, 0xa3, 0x08, 0xd3}

	// TODO: Rename this to img0
	img1, _, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}

	w, h := img1.Bounds().Size().X, img1.Bounds().Size().Y

	img2, err := ebiten.NewImage(w, h, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}

	img3, err := ebiten.NewImage(w, h, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}

	if err := img2.Fill(img2Color); err != nil {
		t.Fatal(err)
		return
	}
	if err := img3.Fill(img3Color); err != nil {
		t.Fatal(err)
		return
	}
	img_12_3, err := ebiten.NewImage(w, h, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	if err := img2.DrawImage(img1, nil); err != nil {
		t.Fatal(err)
		return
	}
	if err := img3.DrawImage(img2, nil); err != nil {
		t.Fatal(err)
		return
	}
	if err := img_12_3.DrawImage(img3, nil); err != nil {
		t.Fatal(err)
		return
	}

	if err := img2.Fill(img2Color); err != nil {
		t.Fatal(err)
		return
	}
	if err := img3.Fill(img3Color); err != nil {
		t.Fatal(err)
		return
	}
	img_1_23, err := ebiten.NewImage(w, h, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	if err := img3.DrawImage(img2, nil); err != nil {
		t.Fatal(err)
		return
	}
	if err := img3.DrawImage(img1, nil); err != nil {
		t.Fatal(err)
		return
	}
	if err := img_1_23.DrawImage(img3, nil); err != nil {
		t.Fatal(err)
		return
	}

	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			c1 := img_12_3.At(i, j).(color.RGBA)
			c2 := img_1_23.At(i, j).(color.RGBA)
			if !sameColors(c1, c2, 1) {
				t.Errorf("img_12_3.At(%d, %d) = %#v; img_1_23.At(%[1]d, %[2]d) = %#[4]v", i, j, c1, c2)
			}
			if c1.A == 0 {
				t.Fatalf("img_12_3.At(%d, %d).A = 0; nothing is rendered?", i, j)
			}
			if c2.A == 0 {
				t.Fatalf("img_1_23.At(%d, %d).A = 0; nothing is rendered?", i, j)
			}
		}
	}
}

func TestImageSelf(t *testing.T) {
	// Note that mutex usages: without defer, unlocking is not called when panicing.
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("DrawImage must panic but not")
		}
	}()
	img, _, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}
	img.DrawImage(img, nil)
}

func TestImageScale(t *testing.T) {
	for _, scale := range []int{2, 3, 4} {
		img0, _, err := openEbitenImage()
		if err != nil {
			t.Fatal(err)
			return
		}
		w, h := img0.Size()
		img1, err := ebiten.NewImage(w*scale, h*scale, ebiten.FilterNearest)
		if err != nil {
			t.Fatal(err)
			return
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(float64(scale), float64(scale))

		if err := img1.DrawImage(img0, op); err != nil {
			t.Fatal(err)
			return
		}

		for j := 0; j < h*scale; j++ {
			for i := 0; i < w*scale; i++ {
				c0 := img0.At(i/scale, j/scale).(color.RGBA)
				c1 := img1.At(i, j).(color.RGBA)
				if c0 != c1 {
					t.Fatalf("img0.At(%[1]d, %[2]d) should equal to img1.At(%[3]d, %[4]d) (with scale %[5]d) but not: %[6]v vs %[7]v", i/2, j/2, i, j, scale, c0, c1)
				}
			}
		}
	}
}

func TestImage90DegreeRotate(t *testing.T) {
	img0, _, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}
	w, h := img0.Size()
	img1, err := ebiten.NewImage(h, w, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Rotate(math.Pi / 2)
	op.GeoM.Translate(float64(h), 0)
	if err := img1.DrawImage(img0, op); err != nil {
		t.Fatal(err)
		return
	}

	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			c0 := img0.At(i, j).(color.RGBA)
			c1 := img1.At(h-j-1, i).(color.RGBA)
			if c0 != c1 {
				t.Errorf("img0.At(%[1]d, %[2]d) should equal to img1.At(%[3]d, %[4]d) but not: %[5]v vs %[6]v", i, j, h-j-1, i, c0, c1)
			}
		}
	}
}

func TestImageDotByDotInversion(t *testing.T) {
	img0, _, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}
	w, h := img0.Size()
	img1, err := ebiten.NewImage(w, h, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Rotate(math.Pi)
	op.GeoM.Translate(float64(w), float64(h))
	if err := img1.DrawImage(img0, op); err != nil {
		t.Fatal(err)
		return
	}

	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			c0 := img0.At(i, j).(color.RGBA)
			c1 := img1.At(w-i-1, h-j-1).(color.RGBA)
			if c0 != c1 {
				t.Errorf("img0.At(%[1]d, %[2]d) should equal to img1.At(%[3]d, %[4]d) but not: %[5]v vs %[6]v", i, j, w-i-1, h-j-1, c0, c1)
			}
		}
	}
}

func TestImageReplacePixels(t *testing.T) {
	// Create a dummy image so that the shared texture is used and origImg's position is shfited.
	dummyImg, _ := ebiten.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 16, 16)), ebiten.FilterDefault)
	defer dummyImg.Dispose()

	_, origImg, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}
	// Convert to *image.RGBA just in case.
	img := image.NewRGBA(origImg.Bounds())
	draw.Draw(img, img.Bounds(), origImg, image.ZP, draw.Src)

	size := img.Bounds().Size()
	img0, err := ebiten.NewImage(size.X, size.Y, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}

	if err := img0.ReplacePixels(img.Pix); err != nil {
		t.Fatal(err)
		return
	}
	for j := 0; j < img0.Bounds().Size().Y; j++ {
		for i := 0; i < img0.Bounds().Size().X; i++ {
			got := img0.At(i, j)
			want := img.At(i, j)
			if got != want {
				t.Errorf("img0 At(%d, %d): got %#v; want %#v", i, j, got, want)
			}
		}
	}

	p := make([]uint8, 4*size.X*size.Y)
	for i := range p {
		p[i] = 0x80
	}
	if err := img0.ReplacePixels(p); err != nil {
		t.Fatal(err)
		return
	}
	// Even if p is changed after calling ReplacePixel, img0 uses the original values.
	for i := range p {
		p[i] = 0
	}
	for j := 0; j < img0.Bounds().Size().Y; j++ {
		for i := 0; i < img0.Bounds().Size().X; i++ {
			got := img0.At(i, j)
			want := color.RGBA{0x80, 0x80, 0x80, 0x80}
			if got != want {
				t.Errorf("img0 At(%d, %d): got %#v; want %#v", i, j, got, want)
			}
		}
	}
}

func TestImageDispose(t *testing.T) {
	img, err := ebiten.NewImage(16, 16, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	img.Fill(color.White)
	if err := img.Dispose(); err != nil {
		t.Errorf("img.Dipose() returns error: %v", err)
	}

	// The color is transparent (color.RGBA{}).
	// Note that the value's type must be color.RGBA.
	got := img.At(0, 0)
	want := color.RGBA{}
	if got != want {
		t.Errorf("img.At(0, 0) got: %v, want: %v", got, want)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestImageCompositeModeLighter(t *testing.T) {
	img0, _, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}

	w, h := img0.Size()
	img1, err := ebiten.NewImage(w, h, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	if err := img1.Fill(color.RGBA{0x01, 0x02, 0x03, 0x04}); err != nil {
		t.Fatal(err)
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.CompositeMode = ebiten.CompositeModeLighter
	if err := img1.DrawImage(img0, op); err != nil {
		t.Fatal(err)
		return
	}
	for j := 0; j < img1.Bounds().Size().Y; j++ {
		for i := 0; i < img1.Bounds().Size().X; i++ {
			got := img1.At(i, j).(color.RGBA)
			want := img0.At(i, j).(color.RGBA)
			want.R = uint8(min(0xff, int(want.R)+1))
			want.G = uint8(min(0xff, int(want.G)+2))
			want.B = uint8(min(0xff, int(want.B)+3))
			want.A = uint8(min(0xff, int(want.A)+4))
			if got != want {
				t.Errorf("img1 At(%d, %d): got %#v; want %#v", i, j, got, want)
			}
		}
	}
}

func TestNewImageFromEbitenImage(t *testing.T) {
	img, _, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}
	if _, err := ebiten.NewImageFromImage(img, ebiten.FilterNearest); err != nil {
		t.Errorf("ebiten.NewImageFromImage returns error: %v", err)
	}
}

func TestNewImageFromSubImage(t *testing.T) {
	_, img, err := openEbitenImage()
	if err != nil {
		t.Fatal(err)
		return
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	subImg := img.(*image.NRGBA).SubImage(image.Rect(1, 1, w-1, h-1))
	eimg, err := ebiten.NewImageFromImage(subImg, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	sw, sh := subImg.Bounds().Dx(), subImg.Bounds().Dy()
	w2, h2 := eimg.Size()
	if w2 != sw {
		t.Errorf("eimg Width: got %#v; want %#v", w2, sw)
	}
	if h2 != sh {
		t.Errorf("eimg Width: got %#v; want %#v", h2, sh)
	}
	for j := 0; j < h2; j++ {
		for i := 0; i < w2; i++ {
			got := eimg.At(i, j)
			want := color.RGBAModel.Convert(img.At(i+1, j+1))
			if got != want {
				t.Errorf("img0 At(%d, %d): got %#v; want %#v", i, j, got, want)
			}
		}
	}
}

type mutableRGBA struct {
	r, g, b, a uint8
}

func (c *mutableRGBA) RGBA() (r, g, b, a uint32) {
	return uint32(c.r) * 0x101, uint32(c.g) * 0x101, uint32(c.b) * 0x101, uint32(c.a) * 0x101
}

func TestImageFill(t *testing.T) {
	w, h := 10, 10
	img, err := ebiten.NewImage(w, h, ebiten.FilterNearest)
	if err != nil {
		t.Fatal(err)
		return
	}
	clr := &mutableRGBA{0x80, 0x80, 0x80, 0x80}
	if err := img.Fill(clr); err != nil {
		t.Fatal(err)
		return
	}
	clr.r = 0
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			got := img.At(i, j)
			want := color.RGBA{0x80, 0x80, 0x80, 0x80}
			if got != want {
				t.Errorf("img At(%d, %d): got %#v; want %#v", i, j, got, want)
			}
		}
	}
}

// Issue #317, #558
func TestImageEdge(t *testing.T) {
	const (
		img0Width  = 16
		img0Height = 16
		img1Width  = 32
		img1Height = 32
	)
	img0, _ := ebiten.NewImage(img0Width, img0Height, ebiten.FilterNearest)
	pixels := make([]uint8, 4*img0Width*img0Height)
	for j := 0; j < img0Height; j++ {
		for i := 0; i < img0Width; i++ {
			idx := 4 * (i + j*img0Width)
			switch {
			case j < img0Height/2:
				pixels[idx] = 0xff
				pixels[idx+1] = 0
				pixels[idx+2] = 0
				pixels[idx+3] = 0xff
			default:
				pixels[idx] = 0
				pixels[idx+1] = 0xff
				pixels[idx+2] = 0
				pixels[idx+3] = 0xff
			}
		}
	}
	img0.ReplacePixels(pixels)
	img1, _ := ebiten.NewImage(img1Width, img1Height, ebiten.FilterDefault)
	red := color.RGBA{0xff, 0, 0, 0xff}
	transparent := color.RGBA{0, 0, 0, 0}

	angles := []float64{}
	for a := 0; a < 1440; a++ {
		angles = append(angles, float64(a)/1440*2*math.Pi)
	}
	for a := 0; a < 4096; a++ {
		angles = append(angles, float64(a)/4096*2*math.Pi)
	}

	for _, f := range []ebiten.Filter{ebiten.FilterNearest, ebiten.FilterLinear} {
		for _, a := range angles {
			img1.Clear()
			op := &ebiten.DrawImageOptions{}
			w, h := img0.Size()
			r := image.Rect(0, 0, w, h/2)
			op.SourceRect = &r
			op.GeoM.Translate(-float64(img0Width)/2, -float64(img0Height)/2)
			op.GeoM.Rotate(a)
			op.GeoM.Translate(img1Width/2, img1Height/2)
			op.Filter = f
			img1.DrawImage(img0, op)
			for j := 0; j < img1Height; j++ {
				for i := 0; i < img1Width; i++ {
					c := img1.At(i, j)
					if c == transparent {
						continue
					}
					switch f {
					case ebiten.FilterNearest:
						if c == red {
							continue
						}
					case ebiten.FilterLinear:
						_, g, b, _ := c.RGBA()
						if g == 0 && b == 0 {
							continue
						}
					}
					t.Errorf("img1.At(%d, %d) (filter: %d, angle: %f) want: red or transparent, got: %v", i, j, f, a, c)
				}
			}
		}
	}
}

// Issue #419
func TestImageTooManyFill(t *testing.T) {
	const width = 1024

	indexToColor := func(index int) uint8 {
		return uint8((17*index + 0x40) % 256)
	}

	src, _ := ebiten.NewImage(1, 1, ebiten.FilterNearest)
	dst, _ := ebiten.NewImage(width, 1, ebiten.FilterNearest)
	for i := 0; i < width; i++ {
		c := indexToColor(i)
		src.Fill(color.RGBA{c, c, c, 0xff})
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(i), 0)
		dst.DrawImage(src, op)
	}

	for i := 0; i < width; i++ {
		c := indexToColor(i)
		got := color.RGBAModel.Convert(dst.At(i, 0)).(color.RGBA)
		want := color.RGBA{c, c, c, 0xff}
		if !sameColors(got, want, 1) {
			t.Errorf("dst.At(%d, %d): got %#v, want: %#v", i, 0, got, want)
		}
	}
}

func BenchmarkDrawImage(b *testing.B) {
	img0, _ := ebiten.NewImage(16, 16, ebiten.FilterNearest)
	img1, _ := ebiten.NewImage(16, 16, ebiten.FilterNearest)
	op := &ebiten.DrawImageOptions{}
	for i := 0; i < b.N; i++ {
		img0.DrawImage(img1, op)
	}
}

func TestImageLinear(t *testing.T) {
	src, _ := ebiten.NewImage(32, 32, ebiten.FilterDefault)
	dst, _ := ebiten.NewImage(64, 64, ebiten.FilterDefault)
	src.Fill(color.RGBA{0, 0xff, 0, 0xff})
	ebitenutil.DrawRect(src, 8, 8, 16, 16, color.RGBA{0xff, 0, 0, 0xff})

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(8, 8)
	op.GeoM.Scale(2, 2)
	r := image.Rect(8, 8, 24, 24)
	op.SourceRect = &r
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(src, op)

	for j := 0; j < 64; j++ {
		for i := 0; i < 64; i++ {
			c := color.RGBAModel.Convert(dst.At(i, j)).(color.RGBA)
			got := c.G
			want := uint8(0)
			if abs(int(c.G)-int(want)) > 1 {
				t.Errorf("dst At(%d, %d).G: got %#v, want: %#v", i, j, got, want)
			}
		}
	}
}

func TestImageOutside(t *testing.T) {
	src, _ := ebiten.NewImage(5, 10, ebiten.FilterNearest) // internal texture size is 8x16.
	dst, _ := ebiten.NewImage(4, 4, ebiten.FilterNearest)
	src.Fill(color.RGBA{0xff, 0, 0, 0xff})

	cases := []struct {
		X, Y, Width, Height int
	}{
		{-4, -4, 4, 4},
		{5, 0, 4, 4},
		{0, 10, 4, 4},
		{5, 10, 4, 4},
		{8, 0, 4, 4},
		{0, 16, 4, 4},
		{8, 16, 4, 4},
		{8, -4, 4, 4},
		{-4, 16, 4, 4},
		{5, 10, 0, 0},
		{5, 10, -2, -2}, // non-well-formed rectangle
	}
	for _, c := range cases {
		dst.Clear()

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(0, 0)
		op.SourceRect = &image.Rectangle{
			Min: image.Pt(c.X, c.Y),
			Max: image.Pt(c.X+c.Width, c.Y+c.Height),
		}
		dst.DrawImage(src, op)

		for j := 0; j < 4; j++ {
			for i := 0; i < 4; i++ {
				got := color.RGBAModel.Convert(dst.At(i, j)).(color.RGBA)
				want := color.RGBA{0, 0, 0, 0}
				if got != want {
					t.Errorf("src(x: %d, y: %d, w: %d, h: %d), dst At(%d, %d): got %#v, want: %#v", c.X, c.Y, c.Width, c.Height, i, j, got, want)
				}
			}
		}
	}
}

func TestImageOutsideUpperLeft(t *testing.T) {
	src, _ := ebiten.NewImage(4, 4, ebiten.FilterNearest)
	dst1, _ := ebiten.NewImage(16, 16, ebiten.FilterNearest)
	dst2, _ := ebiten.NewImage(16, 16, ebiten.FilterNearest)
	src.Fill(color.RGBA{0xff, 0, 0, 0xff})

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Rotate(math.Pi / 4)
	r := image.Rect(-4, -4, 8, 8)
	op.SourceRect = &r
	dst1.DrawImage(src, op)

	op = &ebiten.DrawImageOptions{}
	op.GeoM.Translate(4, 4)
	op.GeoM.Rotate(math.Pi / 4)
	dst2.DrawImage(src, op)

	for j := 0; j < 16; j++ {
		for i := 0; i < 16; i++ {
			got := color.RGBAModel.Convert(dst1.At(i, j)).(color.RGBA)
			want := color.RGBAModel.Convert(dst2.At(i, j)).(color.RGBA)
			if got != want {
				t.Errorf("got: dst1.At(%d, %d): %#v, want: dst2.At(%d, %d): %#v", i, j, got, i, j, want)
			}
		}
	}
}

func TestImageSize(t *testing.T) {
	const (
		w = 17
		h = 31
	)
	img, _ := ebiten.NewImage(w, h, ebiten.FilterDefault)
	gotW, gotH := img.Size()
	if gotW != w {
		t.Errorf("got: %d, want: %d", gotW, w)
	}
	if gotH != h {
		t.Errorf("got: %d, want: %d", gotH, h)
	}
}

func TestImageSize1(t *testing.T) {
	src, _ := ebiten.NewImage(1, 1, ebiten.FilterNearest)
	dst, _ := ebiten.NewImage(1, 1, ebiten.FilterNearest)
	src.Fill(color.White)
	dst.DrawImage(src, nil)
	got := color.RGBAModel.Convert(src.At(0, 0)).(color.RGBA)
	want := color.RGBAModel.Convert(color.White).(color.RGBA)
	if !sameColors(got, want, 1) {
		t.Errorf("got: %#v, want: %#v", got, want)
	}
}

func TestImageSize4096(t *testing.T) {
	src, _ := ebiten.NewImage(4096, 4096, ebiten.FilterNearest)
	dst, _ := ebiten.NewImage(4096, 4096, ebiten.FilterNearest)
	pix := make([]byte, 4096*4096*4)
	for i := 0; i < 4096; i++ {
		j := 4095
		idx := 4 * (i + j*4096)
		pix[idx] = uint8(i + j)
		pix[idx+1] = uint8((i + j) >> 8)
		pix[idx+2] = uint8((i + j) >> 16)
		pix[idx+3] = 0xff
	}
	for j := 0; j < 4096; j++ {
		i := 4095
		idx := 4 * (i + j*4096)
		pix[idx] = uint8(i + j)
		pix[idx+1] = uint8((i + j) >> 8)
		pix[idx+2] = uint8((i + j) >> 16)
		pix[idx+3] = 0xff
	}
	src.ReplacePixels(pix)
	dst.DrawImage(src, nil)
	for i := 4095; i < 4096; i++ {
		j := 4095
		got := color.RGBAModel.Convert(dst.At(i, j)).(color.RGBA)
		want := color.RGBA{uint8(i + j), uint8((i + j) >> 8), uint8((i + j) >> 16), 0xff}
		if got != want {
			t.Errorf("At(%d, %d): got: %#v, want: %#v", i, j, got, want)
		}
	}
	for j := 4095; j < 4096; j++ {
		i := 4095
		got := color.RGBAModel.Convert(dst.At(i, j)).(color.RGBA)
		want := color.RGBA{uint8(i + j), uint8((i + j) >> 8), uint8((i + j) >> 16), 0xff}
		if got != want {
			t.Errorf("At(%d, %d): got: %#v, want: %#v", i, j, got, want)
		}
	}
}

func TestImageCopy(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("copying image and using it should panic")
		}
	}()

	img0, _ := ebiten.NewImage(256, 256, ebiten.FilterDefault)
	img1 := *img0
	img1.Fill(color.Transparent)
}

func TestImageStretch(t *testing.T) {
	img0, _ := ebiten.NewImage(16, 17, ebiten.FilterDefault)

	pix := make([]byte, 4*16*17)
	for i := 0; i < 16*16; i++ {
		pix[4*i] = 0xff
		pix[4*i+3] = 0xff
	}
	for i := 0; i < 16; i++ {
		pix[4*(16*16+i)+1] = 0xff
		pix[4*(16*16+i)+3] = 0xff
	}
	img0.ReplacePixels(pix)

	// TODO: 4096 doesn't pass on MacBook Pro (#611).
	const h = 2048
	img1, _ := ebiten.NewImage(16, h, ebiten.FilterDefault)
	for i := 1; i < h; i++ {
		img1.Clear()
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(1, float64(i)/16)
		r := image.Rect(0, 0, 16, 16)
		op.SourceRect = &r
		img1.DrawImage(img0, op)
		for j := -1; j <= 1; j++ {
			got := img1.At(0, i+j).(color.RGBA)
			want := color.RGBA{}
			if j < 0 {
				want = color.RGBA{0xff, 0, 0, 0xff}
			}
			if got != want {
				t.Errorf("At(%d, %d) (i=%d): got: %#v, want: %#v", 0, i+j, i, got, want)
			}
		}
	}
}

func TestSprites(t *testing.T) {
	const (
		width  = 512
		height = 512
	)

	src, _ := ebiten.NewImage(4, 4, ebiten.FilterNearest)
	src.Fill(color.RGBA{0xff, 0xff, 0xff, 0xff})
	dst, _ := ebiten.NewImage(width, height, ebiten.FilterNearest)
	for j := 0; j < height/4; j++ {
		for i := 0; i < width/4; i++ {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(i*4), float64(j*4))
			dst.DrawImage(src, op)
		}
	}

	for j := 0; j < height/4; j++ {
		for i := 0; i < width/4; i++ {
			got := color.RGBAModel.Convert(dst.At(i*4, j*4)).(color.RGBA)
			want := color.RGBA{0xff, 0xff, 0xff, 0xff}
			if !sameColors(got, want, 1) {
				t.Errorf("dst.At(%d, %d): got %#v, want: %#v", i*4, j*4, got, want)
			}
		}
	}
}
