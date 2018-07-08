// Copyright 2018 The Ebiten Authors
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

// +build example jsgo

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten"
	"github.com/hajimehoshi/ebiten/examples/resources/images"
	"github.com/hajimehoshi/ebiten/internal/testflock"
)

var noErr = errors.New("clean")

type benchmark struct {
	name   string
	fn     func(b *testing.B, screen *ebiten.Image)
	result testing.BenchmarkResult
}

func main() {
	// You can't specify minimum bench time, or whether you want allocation,
	// directly in code, but because benchmarking per-frame is weird, we really
	// want to control those. So what if we faked up command line parameters
	// which have special meaning to the testing package?
	newArgs := []string{os.Args[0], "-test.benchmem"}
	timeGiven := false
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.benchtime") {
			timeGiven = true
			break
		}
	}
	if !timeGiven {
		newArgs = append(newArgs, "-test.benchtime=180ms")
	}
	os.Args = append(newArgs, os.Args[1:]...)
	flag.Parse()
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
			op := &ebiten.DrawImageOptions{}
			for i := 0; i < b.N; i++ {
				op.GeoM.Reset()
				op.GeoM.Translate(float64(i*320/b.N), float64(i*200/b.N))
				screen.DrawImage(px26, op)
			}
		},
	},
	{
		name: "draw104",
		fn: func(b *testing.B, screen *ebiten.Image) {
			op := &ebiten.DrawImageOptions{}
			for i := 0; i < b.N; i++ {
				op.GeoM.Reset()
				op.GeoM.Translate(float64(i*320/b.N), float64(i*200/b.N))
				screen.DrawImage(px104, op)
			}
		},
	},
	{
		name: "draw416",
		fn: func(b *testing.B, screen *ebiten.Image) {
			op := &ebiten.DrawImageOptions{}
			for i := 0; i < b.N; i++ {
				op.GeoM.Reset()
				op.GeoM.Translate(float64(i*320/b.N), float64(i*200/b.N))
				screen.DrawImage(px416, op)
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
	{
		name: "draw26colorNew",
		fn: func(b *testing.B, screen *ebiten.Image) {
			op := &ebiten.DrawImageOptions{}
			tints := []color.RGBA{color.RGBA{255, 51, 51, 255}, color.RGBA{51, 255, 51, 255}}
			for i := 0; i < b.N; i++ {
				idx := i % 2
				op.GeoM.Reset()
				op.GeoM.Translate(float64(i*320/b.N), float64(i*200/b.N))
				op.Tints = tints[idx : idx+1]
				screen.DrawImage(px26, op)
			}
		},
	},
	{
		name: "draw26colorNew4",
		fn: func(b *testing.B, screen *ebiten.Image) {
			op := &ebiten.DrawImageOptions{}
			tints := []color.RGBA{
				color.RGBA{255, 51, 51, 255},
				color.RGBA{51, 255, 51, 255},
				color.RGBA{51, 51, 255, 255},
				color.RGBA{255, 255, 0, 255},
				color.RGBA{255, 51, 51, 255},
				color.RGBA{51, 255, 51, 255},
				color.RGBA{51, 51, 255, 255},
				color.RGBA{255, 255, 0, 255},
			}
			for i := 0; i < b.N; i++ {
				idx := i % 4
				op.GeoM.Reset()
				op.GeoM.Translate(float64(i*320/b.N), float64(i*200/b.N))
				op.Tints = tints[idx : idx+4]
				screen.DrawImage(px26, op)
			}
		},
	},
}

var (
	benchIdx  = 0
	results   []testing.BenchmarkResult
	setup     sync.Once
	benchFunc func(b *testing.B, screen *ebiten.Image)
	benchGo   = make(chan struct{})
	benchDone = make(chan bool)
)

func benchmarks(screen *ebiten.Image) error {
	var setupErr error

	// We need to run this exactly once, but it has to happen inside an
	// actual update loop so ebiten is fully up and running.
	setup.Do(func() {
		eimg, _, err := openEbitenImage()
		if err != nil {
			setupErr = fmt.Errorf("can't open image: %s", err)
			return
		}
		px26 = eimg

		w, h := eimg.Size()
		img, err := ebiten.NewImage(w*4, h*4, ebiten.FilterNearest)
		if err != nil {
			setupErr = fmt.Errorf("can't create new image: %s", err)
			return
		}
		px104 = img

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(4, 4)
		px104.DrawImage(px26, op)

		img, err = ebiten.NewImage(w*16, h*16, ebiten.FilterNearest)
		if err != nil {
			setupErr = fmt.Errorf("can't create new image: %s", err)
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
		if benchIdx >= len(benchList) {
			return noErr
		}
		benchFunc = benchList[benchIdx].fn
		go func(idx int) {
			benchList[idx].result = testing.Benchmark(func(b *testing.B) {
				b.StopTimer()
				// ebiten will start/stop the benchmark's timer during the
				// actual frame update, including time spent calling the user
				// update function, but also any rendering calls during the
				// update.
				ebiten.SetBenchmark(b)
				var elapsed time.Duration
				// run for at least 60 frames, so the FPS value should mean something
				for i := 0; i < 60; i++ {
					<-benchGo
					t0 := time.Now()
					benchFunc(b, screen)
					t1 := time.Now()
					elapsed += t1.Sub(t0)
					benchDone <- false
				}
				b.Logf("%16s: N %5d, FPS: %5.2f, time %10v/60 frames\n", benchList[idx].name, b.N, ebiten.CurrentFPS(), elapsed)
				ebiten.SetBenchmark(nil)
			})
			<-benchGo
			benchDone <- true
		}(benchIdx)
		benchIdx++
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
