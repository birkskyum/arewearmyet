package picture

import (
	"fmt"
	"strings"
	"testing"
)

// ppm makes a picture whose pixels come from a function of their place.
func ppm(width, height int, pixel func(x, y int) (r, g, b byte)) []byte {
	file := []byte(fmt.Sprintf("P6\n%d %d\n255\n", width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b := pixel(x, y)
			file = append(file, r, g, b)
		}
	}
	return file
}

func TestTellsAPictureFromABlackOrFlatFrame(t *testing.T) {
	scene, err := PPM(ppm(192, 108, func(x, y int) (byte, byte, byte) { return byte(x), byte(y * 2), 90 }))
	if err != nil || scene.Blank() {
		t.Errorf("a picture read as blank (%v)", err)
	}
	black, _ := PPM(ppm(192, 108, func(x, y int) (byte, byte, byte) { return 2, 2, 2 }))
	flat, _ := PPM(ppm(192, 108, func(x, y int) (byte, byte, byte) { return 120, 120, 120 }))
	if !black.Blank() || !flat.Blank() {
		t.Error("a black or flat frame read as a picture")
	}
	if _, err := PPM([]byte("P6\n10 10\n255\nshort")); err == nil {
		t.Error("a cut-off picture was read")
	}
	if _, err := PPM([]byte("not a picture")); err == nil {
		t.Error("something else was read as a picture")
	}
}

func TestDrawsTheFrameInCellsThatKeepItsShape(t *testing.T) {
	// Left half red, right half blue
	frame, _ := PPM(ppm(1920, 1080, func(x, y int) (byte, byte, byte) {
		if x < 960 {
			return 255, 0, 0
		}
		return 0, 0, 255
	}))
	drawn := frame.Render(80, 20)
	lines := strings.Split(drawn, "\n")
	cells := strings.Count(lines[0], "▀")
	// 16:9 in cells twice as high as wide: 20 rows are 40 pixels high, so 71 wide
	if len(lines) != 20 || cells != 71 {
		t.Errorf("drew %d rows of %d cells", len(lines), cells)
	}
	if !strings.HasPrefix(lines[0], "\x1b[38;2;255;0;0m\x1b[48;2;255;0;0m▀") || !strings.Contains(lines[0], "\x1b[38;2;0;0;255m\x1b[48;2;0;0;255m▀\x1b[0m") {
		t.Errorf("colours are off: %q", lines[0][:60])
	}
	// A narrow terminal limits it by width instead
	if narrow := strings.Split(frame.Render(40, 20), "\n"); strings.Count(narrow[0], "▀") != 40 || len(narrow) != 11 {
		t.Errorf("narrow: %d rows of %d cells", len(narrow), strings.Count(narrow[0], "▀"))
	}
}
