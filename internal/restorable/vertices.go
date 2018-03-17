// Copyright 2017 The Ebiten Authors
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

package restorable

import (
	"github.com/hajimehoshi/ebiten/internal/affine"
	"github.com/hajimehoshi/ebiten/internal/graphics"
	"image/color"
)

var (
	quadFloat32Num     = graphics.QuadVertexSizeInBytes() / 4
	theVerticesBackend = &verticesBackend{}
)

type verticesBackend struct {
	backend []float32
	head    int
}

func (v *verticesBackend) get() []float32 {
	const num = 256
	if v.backend == nil {
		v.backend = make([]float32, quadFloat32Num*num)
	}
	s := v.backend[v.head : v.head+quadFloat32Num]
	v.head += quadFloat32Num
	if v.head+quadFloat32Num > len(v.backend) {
		v.backend = nil
		v.head = 0
	}
	return s
}

func (i *Image) vertices(sx0, sy0, sx1, sy1 int, tint *color.RGBA, geo *affine.GeoM) []float32 {
	if sx0 >= sx1 || sy0 >= sy1 {
		return nil
	}
	if sx1 <= 0 || sy1 <= 0 {
		return nil
	}

	// TODO: This function should be in graphics package?
	vs := theVerticesBackend.get()

	x0, y0 := 0.0, 0.0
	x1, y1 := float64(sx1-sx0), float64(sy1-sy0)
	if tint == nil {
		tint = &color.RGBA{255, 255, 255, 255}
	}
	r := float32(tint.R) / 255
	g := float32(tint.G) / 255
	b := float32(tint.B) / 255
	a := float32(tint.A) / 255

	// it really feels like we should be able to cache this computation
	// but it may not matter.
	width, height := i.Size()
	w := 1
	h := 1
	for w < width {
		w *= 2
	}
	for h < height {
		h *= 2
	}
	wf := float32(w)
	hf := float32(h)
	u0, v0, u1, v1 := float32(sx0)/wf, float32(sy0)/hf, float32(sx1)/wf, float32(sy1)/hf

	x, y := geo.Apply32(x0, y0)
	// Vertex coordinates
	vs[0] = x
	vs[1] = y

	// Texture coordinates: first 2 values indicates the actual coodinate, and
	// the second indicates diagonally opposite coodinates.
	// The second is needed to calculate source rectangle size in shader programs.
	vs[2] = u0
	vs[3] = v0
	vs[4] = u1
	vs[5] = v1

	vs[6] = r
	vs[7] = g
	vs[8] = b
	vs[9] = a

	// and the same for the other three coordinates
	x, y = geo.Apply32(x1, y0)
	vs[10] = x
	vs[11] = y
	vs[12] = u1
	vs[13] = v0
	vs[14] = u0
	vs[15] = v1
	vs[16] = r
	vs[17] = g
	vs[18] = b
	vs[19] = a

	x, y = geo.Apply32(x0, y1)
	vs[20] = x
	vs[21] = y
	vs[22] = u0
	vs[23] = v1
	vs[24] = u1
	vs[25] = v0
	vs[26] = r
	vs[27] = g
	vs[28] = b
	vs[29] = a

	x, y = geo.Apply32(x1, y1)
	vs[30] = x
	vs[31] = y
	vs[32] = u1
	vs[33] = v1
	vs[34] = u0
	vs[35] = v0
	vs[36] = r
	vs[37] = g
	vs[38] = b
	vs[39] = a

	return vs
}
