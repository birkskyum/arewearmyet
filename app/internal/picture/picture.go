// Package picture reads a camera frame and draws it in the terminal.
package picture

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
)

// Frame is one picture, three bytes a pixel: red, green, blue.
type Frame struct {
	Width, Height int
	Pixels        []byte
}

// PPM reads a binary PPM file (P6, 255 levels), the format libcamera's cam writes.
func PPM(file []byte) (Frame, error) {
	var frame Frame
	var depth int
	header := bytes.NewReader(file)
	if _, err := fmt.Fscanf(header, "P6\n%d %d\n%d\n", &frame.Width, &frame.Height, &depth); err != nil {
		return frame, fmt.Errorf("not a PPM picture: %w", err)
	}
	start := len(file) - header.Len()
	if depth != 255 || frame.Width <= 0 || frame.Height <= 0 || len(file)-start < frame.Width*frame.Height*3 {
		return frame, fmt.Errorf("a PPM picture of %dx%d with %d levels and %d bytes can't be read", frame.Width, frame.Height, depth, len(file)-start)
	}
	frame.Pixels = file[start : start+frame.Width*frame.Height*3]
	return frame, nil
}

// Brightness is the mean of every colour value, 0 to 255, and Contrast how far they spread.
// A black or blank frame has almost no contrast.
func (f Frame) Brightness() (mean, contrast int) {
	if len(f.Pixels) == 0 {
		return 0, 0
	}
	var sum, squares float64
	// Every seventh value is plenty, and keeps a full frame fast
	n := 0.0
	for i := 0; i < len(f.Pixels); i += 7 {
		v := float64(f.Pixels[i])
		sum += v
		squares += v * v
		n++
	}
	m := sum / n
	return int(m), int(math.Sqrt(math.Max(0, squares/n-m*m)))
}

// Blank says whether the frame is black or one flat colour, which a camera that doesn't work gives.
func (f Frame) Blank() bool {
	mean, contrast := f.Brightness()
	return mean <= 8 || contrast <= 4
}

// Render draws the frame at most columns wide and rows high in terminal cells. Each cell
// holds two pixels, one above the other, as an upper half block in the top pixel's colour on
// the bottom pixel's.
func (f Frame) Render(columns, rows int) string {
	if f.Width == 0 || f.Height == 0 || columns < 2 || rows < 1 {
		return ""
	}
	// A cell is about twice as high as it is wide, so two stacked pixels in it are square
	height := rows * 2
	width := height * f.Width / f.Height
	if width > columns {
		width = columns
		height = width * f.Height / f.Width
		height -= height % 2
	}
	if width < 1 || height < 2 {
		return ""
	}
	// Each drawn pixel is the average of the block of frame pixels it covers
	average := func(x, y int) (r, g, b int) {
		x0, x1 := x*f.Width/width, (x+1)*f.Width/width
		y0, y1 := y*f.Height/height, (y+1)*f.Height/height
		count := 0
		for py := y0; py < y1; py += 1 + (y1-y0)/4 {
			for px := x0; px < x1; px += 1 + (x1-x0)/4 {
				at := (py*f.Width + px) * 3
				r += int(f.Pixels[at])
				g += int(f.Pixels[at+1])
				b += int(f.Pixels[at+2])
				count++
			}
		}
		if count == 0 {
			return 0, 0, 0
		}
		return r / count, g / count, b / count
	}
	var out strings.Builder
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x++ {
			tr, tg, tb := average(x, y)
			br, bg, bb := average(x, y+1)
			fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", tr, tg, tb, br, bg, bb)
		}
		out.WriteString("\x1b[0m")
		if y+2 < height {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// PNG is the frame as a PNG file, for an image viewer to show at full size.
func (f Frame) PNG() ([]byte, error) {
	if f.Width == 0 || f.Height == 0 {
		return nil, fmt.Errorf("there is no picture")
	}
	img := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
	for i := 0; i < f.Width*f.Height; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = f.Pixels[i*3], f.Pixels[i*3+1], f.Pixels[i*3+2], 255
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
