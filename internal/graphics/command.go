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

package graphics

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/internal/affine"
	emath "github.com/hajimehoshi/ebiten/internal/math"
	"github.com/hajimehoshi/ebiten/internal/opengl"
)

// command represents a drawing command.
//
// A command for drawing that is created when Image functions are called like DrawImage,
// or Fill.
// A command is not immediately executed after created. Instaed, it is queued after created,
// and executed only when necessary.
type command interface {
	Exec(indexOffsetInBytes int) error
	NumVertices() int
	NumIndices() int
	AddNumVertices(n int)
	AddNumIndices(n int)
	CanMerge(dst, src *Image, color *affine.ColorM, mode opengl.CompositeMode, filter Filter) bool
}

// commandQueue is a command queue for drawing commands.
type commandQueue struct {
	// commands is a queue of drawing commands.
	commands []command

	// vertices represents a vertices data in OpenGL's array buffer.
	vertices []float32

	indices  []uint16
	nindices int

	tmpNumIndices int
	nextIndex     int
}

// theCommandQueue is the command queue for the current process.
var theCommandQueue = &commandQueue{}

// appendVertices appends vertices to the queue.
func (q *commandQueue) appendVertices(vertices []float32) {
	q.vertices = append(q.vertices, vertices...)
}

func (q *commandQueue) appendIndices(indices []uint16, offset uint16) {
	if len(q.indices) < q.nindices+len(indices) {
		n := q.nindices + len(indices) - len(q.indices)
		q.indices = append(q.indices, make([]uint16, n)...)
	}
	for i := range indices {
		q.indices[q.nindices+i] = indices[i] + offset
	}
	q.nindices += len(indices)
}

func (q *commandQueue) doEnqueueDrawImageCommand(dst, src *Image, nvertices, nindices int, color *affine.ColorM, mode opengl.CompositeMode, filter Filter, forceNewCommand bool) {
	if nindices > indicesNum {
		panic("not implemented for too many indices")
	}
	if !forceNewCommand && 0 < len(q.commands) {
		if last := q.commands[len(q.commands)-1]; last.CanMerge(dst, src, color, mode, filter) {
			last.AddNumVertices(nvertices)
			last.AddNumIndices(nindices)
			return
		}
	}
	c := &drawImageCommand{
		dst:       dst,
		src:       src,
		nvertices: nvertices,
		nindices:  nindices,
		color:     color,
		mode:      mode,
		filter:    filter,
	}
	q.commands = append(q.commands, c)
}

// EnqueueDrawImageCommand enqueues a drawing-image command.
func (q *commandQueue) EnqueueDrawImageCommand(dst, src *Image, vertices []float32, indices []uint16, color *affine.ColorM, mode opengl.CompositeMode, filter Filter) {
	if len(indices) > indicesNum {
		panic("not reached")
	}
	vertexFloats := VertexSizeInFloats()
	// temporary hack: populate color naively
	for i := 0; i < len(vertices); i += vertexFloats {
		vs := vertices[i : i+vertexFloats]
		for j := 6; j < vertexFloats; j++ {
			vs[j] = 1.0
		}
	}

	split := false
	if q.tmpNumIndices+len(indices) > indicesNum {
		q.tmpNumIndices = 0
		q.nextIndex = 0
		split = true
	}

	q.appendVertices(vertices)
	q.appendIndices(indices, uint16(q.nextIndex))
	q.nextIndex += len(vertices) / vertexFloats
	q.tmpNumIndices += len(indices)

	q.doEnqueueDrawImageCommand(dst, src, len(vertices), len(indices), color, mode, filter, split)
}

// Enqueue enqueues a drawing command other than a draw-image command.
//
// For a draw-image command, use EnqueueDrawImageCommand.
func (q *commandQueue) Enqueue(command command) {
	q.commands = append(q.commands, command)
}

// Flush flushes the command queue.
func (q *commandQueue) Flush() error {
	// glViewport must be called at least at every frame on iOS.
	opengl.GetContext().ResetViewportSize()
	es := q.indices
	vs := q.vertices
	for len(q.commands) > 0 {
		nv := 0
		ne := 0
		nc := 0
		for _, c := range q.commands {
			if c.NumIndices() > indicesNum {
				panic("not reached")
			}
			if ne+c.NumIndices() > indicesNum {
				break
			}
			nv += c.NumVertices()
			ne += c.NumIndices()
			nc++
		}
		if 0 < ne {
			// Note that the vertices passed to BufferSubData is not under GC management
			// in opengl package due to unsafe-way.
			// See BufferSubData in context_mobile.go.
			opengl.GetContext().ElementArrayBufferSubData(es[:ne])
			opengl.GetContext().ArrayBufferSubData(vs[:nv])
			es = es[ne:]
			vs = vs[nv:]
		}
		indexOffsetInBytes := 0
		for _, c := range q.commands[:nc] {
			if err := c.Exec(indexOffsetInBytes); err != nil {
				return err
			}
			// TODO: indexOffsetInBytes should be reset if the command type is different
			// from the previous one. This fix is needed when another drawing command is
			// introduced than drawImageCommand.
			indexOffsetInBytes += c.NumIndices() * 2 // 2 is uint16 size in bytes
		}
		if 0 < nc {
			// Call glFlush to prevent black flicking (especially on Android (#226) and iOS).
			opengl.GetContext().Flush()
		}
		q.commands = q.commands[nc:]
	}
	q.commands = q.commands[:0]
	q.vertices = q.vertices[:0]
	q.nindices = 0
	q.tmpNumIndices = 0
	q.nextIndex = 0
	return nil
}

// FlushCommands flushes the command queue.
func FlushCommands() error {
	return theCommandQueue.Flush()
}

// drawImageCommand represents a drawing command to draw an image on another image.
type drawImageCommand struct {
	dst       *Image
	src       *Image
	nvertices int
	nindices  int
	color     *affine.ColorM
	mode      opengl.CompositeMode
	filter    Filter
}

// VertexSizeInBytes returns the size in bytes of one vertex.
func VertexSizeInBytes() int {
	return theArrayBufferLayout.totalBytes()
}

func VertexSizeInFloats() int {
	return theArrayBufferLayout.totalFloats()
}

// Exec executes the drawImageCommand.
func (c *drawImageCommand) Exec(indexOffsetInBytes int) error {
	f, err := c.dst.createFramebufferIfNeeded()
	if err != nil {
		return err
	}
	f.setAsViewport()

	opengl.GetContext().BlendFunc(c.mode)

	if c.nindices == 0 {
		return nil
	}
	proj := f.projectionMatrix()
	theOpenGLState.useProgram(proj, c.src.texture.native, c.dst, c.src, c.color, c.filter)
	opengl.GetContext().DrawElements(opengl.Triangles, c.nindices, indexOffsetInBytes)

	// glFlush() might be necessary at least on MacBook Pro (a smilar problem at #419),
	// but basically this pass the tests (esp. TestImageTooManyFill).
	// As glFlush() causes performance problems, this should be avoided as much as possible.
	// Let's wait and see, and file a new issue when this problem is newly found.
	return nil
}

func (c *drawImageCommand) NumVertices() int {
	return c.nvertices
}

func (c *drawImageCommand) NumIndices() int {
	return c.nindices
}

func (c *drawImageCommand) AddNumVertices(n int) {
	c.nvertices += n
}

func (c *drawImageCommand) AddNumIndices(n int) {
	c.nindices += n
}

// CanMerge returns a boolean value indicating whether the other drawImageCommand can be merged
// with the drawImageCommand c.
func (c *drawImageCommand) CanMerge(dst, src *Image, color *affine.ColorM, mode opengl.CompositeMode, filter Filter) bool {
	if c.dst != dst {
		return false
	}
	if c.src != src {
		return false
	}
	if !c.color.Equals(color) {
		return false
	}
	if c.mode != mode {
		return false
	}
	if c.filter != filter {
		return false
	}
	return true
}

// replacePixelsCommand represents a command to replace pixels of an image.
type replacePixelsCommand struct {
	dst    *Image
	pixels []byte
	x      int
	y      int
	width  int
	height int
}

// Exec executes the replacePixelsCommand.
func (c *replacePixelsCommand) Exec(indexOffsetInBytes int) error {
	// glFlush is necessary on Android.
	// glTexSubImage2D didn't work without this hack at least on Nexus 5x and NuAns NEO [Reloaded] (#211).
	opengl.GetContext().Flush()
	opengl.GetContext().BindTexture(c.dst.texture.native)
	opengl.GetContext().TexSubImage2D(c.pixels, c.x, c.y, c.width, c.height)
	return nil
}

func (c *replacePixelsCommand) NumVertices() int {
	return 0
}

func (c *replacePixelsCommand) NumIndices() int {
	return 0
}

func (c *replacePixelsCommand) AddNumVertices(n int) {
}

func (c *replacePixelsCommand) AddNumIndices(n int) {
}

func (c *replacePixelsCommand) CanMerge(dst, src *Image, color *affine.ColorM, mode opengl.CompositeMode, filter Filter) bool {
	return false
}

// disposeCommand represents a command to dispose an image.
type disposeCommand struct {
	target *Image
}

// Exec executes the disposeCommand.
func (c *disposeCommand) Exec(indexOffsetInBytes int) error {
	if c.target.framebuffer != nil &&
		c.target.framebuffer.native != opengl.GetContext().ScreenFramebuffer() {
		opengl.GetContext().DeleteFramebuffer(c.target.framebuffer.native)
	}
	if c.target.texture != nil {
		opengl.GetContext().DeleteTexture(c.target.texture.native)
	}
	return nil
}

func (c *disposeCommand) NumVertices() int {
	return 0
}

func (c *disposeCommand) NumIndices() int {
	return 0
}

func (c *disposeCommand) AddNumVertices(n int) {
}

func (c *disposeCommand) AddNumIndices(n int) {
}

func (c *disposeCommand) CanMerge(dst, src *Image, color *affine.ColorM, mode opengl.CompositeMode, filter Filter) bool {
	return false
}

// newImageCommand represents a command to create an empty image with given width and height.
type newImageCommand struct {
	result *Image
	width  int
	height int
}

func checkSize(width, height int) {
	if width < 1 {
		panic(fmt.Sprintf("graphics: width (%d) must be equal or more than 1.", width))
	}
	if height < 1 {
		panic(fmt.Sprintf("graphics: height (%d) must be equal or more than 1.", height))
	}
	m := MaxImageSize()
	if width > m {
		panic(fmt.Sprintf("graphics: width (%d) must be less than or equal to %d", width, m))
	}
	if height > m {
		panic(fmt.Sprintf("graphics: height (%d) must be less than or equal to %d", height, m))
	}
}

// Exec executes a newImageCommand.
func (c *newImageCommand) Exec(indexOffsetInBytes int) error {
	w := emath.NextPowerOf2Int(c.width)
	h := emath.NextPowerOf2Int(c.height)
	checkSize(w, h)
	native, err := opengl.GetContext().NewTexture(w, h)
	if err != nil {
		return err
	}
	c.result.texture = &texture{
		native: native,
	}
	return nil
}

func (c *newImageCommand) NumVertices() int {
	return 0
}

func (c *newImageCommand) NumIndices() int {
	return 0
}

func (c *newImageCommand) AddNumVertices(n int) {
}

func (c *newImageCommand) AddNumIndices(n int) {
}

func (c *newImageCommand) CanMerge(dst, src *Image, color *affine.ColorM, mode opengl.CompositeMode, filter Filter) bool {
	return false
}

// newScreenFramebufferImageCommand is a command to create a special image for the screen.
type newScreenFramebufferImageCommand struct {
	result *Image
	width  int
	height int
}

// Exec executes a newScreenFramebufferImageCommand.
func (c *newScreenFramebufferImageCommand) Exec(indexOffsetInBytes int) error {
	checkSize(c.width, c.height)
	// The (default) framebuffer size can't be converted to a power of 2.
	// On browsers, c.width and c.height are used as viewport size and
	// Edge can't treat a bigger viewport than the drawing area (#71).
	c.result.framebuffer = newScreenFramebuffer(c.width, c.height)
	return nil
}

func (c *newScreenFramebufferImageCommand) NumVertices() int {
	return 0
}

func (c *newScreenFramebufferImageCommand) NumIndices() int {
	return 0
}

func (c *newScreenFramebufferImageCommand) AddNumVertices(n int) {
}

func (c *newScreenFramebufferImageCommand) AddNumIndices(n int) {
}

func (c *newScreenFramebufferImageCommand) CanMerge(dst, src *Image, color *affine.ColorM, mode opengl.CompositeMode, filter Filter) bool {
	return false
}
