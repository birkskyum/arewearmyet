package picture

import (
	"strings"
	"testing"
)

func TestAFrameTakesTheMostCellsItsShapeAllows(t *testing.T) {
	frame := Frame{Width: 1920, Height: 1092}
	for _, c := range []struct{ columns, rows, wide, high int }{
		{96, 30, 96, 27}, // the width is the limit
		{96, 14, 49, 14}, // the height is
		{96, 4, 14, 4},
		{400, 400, 128, 36}, // never more cells than there are marks for
		{1, 30, 0, 0},
	} {
		if wide, high := frame.Fit(c.columns, c.rows); wide != c.wide || high != c.high {
			t.Errorf("in %dx%d it takes %dx%d, want %dx%d", c.columns, c.rows, wide, high, c.wide, c.high)
		}
	}
}

func TestAFrameIsMadeSmallerByAveraging(t *testing.T) {
	frame := Frame{Width: 4, Height: 2, Pixels: []byte{
		0, 0, 0, 100, 100, 100, 10, 20, 30, 10, 20, 30,
		100, 100, 100, 200, 200, 200, 10, 20, 30, 10, 20, 30,
	}}
	small := frame.Scaled(2)
	if small.Width != 2 || small.Height != 1 || string(small.Pixels) != string([]byte{100, 100, 100, 10, 20, 30}) {
		t.Errorf("scaled: %+v", small)
	}
	if same := frame.Scaled(4); same.Width != 4 {
		t.Errorf("a frame that fits was scaled to %d", same.Width)
	}
}

func TestThePictureIsHandedToTheTerminalInPiecesAndDrawnInItsCells(t *testing.T) {
	frame := Frame{Width: 640, Height: 364, Pixels: make([]byte, 640*364*3)}
	for i := range frame.Pixels {
		frame.Pixels[i] = byte(i * 31 % 251)
	}
	sequence, err := frame.Hand(77, 49, 14)
	if err != nil {
		t.Fatal(err)
	}
	pieces := strings.Split(strings.TrimSuffix(sequence, "\x1b\\"), "\x1b\\")
	if len(pieces) < 2 || !strings.HasPrefix(pieces[0], "\x1b_Ga=T,U=1,f=100,q=2,i=77,c=49,r=14,m=1;") {
		t.Fatalf("%d pieces, the first starts %q", len(pieces), pieces[0][:min(60, len(pieces[0]))])
	}
	for i, piece := range pieces[1:] {
		more := "\x1b_Gm=1;"
		if i == len(pieces)-2 {
			more = "\x1b_Gm=0;"
		}
		if !strings.HasPrefix(piece, more) || len(piece) > 4096+len(more) {
			t.Fatalf("piece %d of %d bytes starts %q", i+1, len(piece), piece[:8])
		}
	}
	// Every cell says which picture, row and column it is, so a line redrawn alone still fits
	cells := Cells(77, 3, 2)
	want := "\x1b[38;5;77m\U0010EEEE̅̅\U0010EEEE̅̍\U0010EEEE̅̎\x1b[39m\n" +
		"\x1b[38;5;77m\U0010EEEE̍̅\U0010EEEE̍̍\U0010EEEE̍̎\x1b[39m"
	if cells != want {
		t.Errorf("cells:\n%q\nwant\n%q", cells, want)
	}
	if Forget(77) != "\x1b_Ga=d,d=I,q=2,i=77\x1b\\" {
		t.Errorf("forget: %q", Forget(77))
	}
}

func TestOnlyATerminalThatDrawsPicturesIsAskedTo(t *testing.T) {
	for _, name := range []string{"TMUX", "KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR"} {
		t.Setenv(name, "")
	}
	for term, want := range map[string]bool{"xterm-ghostty": true, "xterm-kitty": true, "alacritty": false, "xterm-256color": false, "tmux-256color": false} {
		t.Setenv("TERM", term)
		if InTerminal() != want {
			t.Errorf("TERM=%s: %v", term, InTerminal())
		}
	}
	t.Setenv("TERM", "xterm-ghostty")
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	if InTerminal() {
		t.Error("inside tmux the picture would not be drawn")
	}
}
