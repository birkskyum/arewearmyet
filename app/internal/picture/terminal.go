package picture

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// A terminal that knows the kitty graphics protocol keeps a picture itself and draws it sharp
// in the cells that hold its placeholder character, so a picture can sit in a screen that is
// otherwise text. Ghostty, Omarchy's terminal, and kitty do this.

// MaxCells is the most columns or rows a picture in the terminal takes.
const MaxCells = len(marks)

// placeholder is the character whose cells the terminal fills with the picture. The marks
// that follow it say which row and column of the picture a cell is, in kitty's own order.
const placeholder = '\U0010EEEE'

var marks = [...]rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F, 0x0346, 0x034A, 0x034B, 0x034C,
	0x0350, 0x0351, 0x0352, 0x0357, 0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484, 0x0485, 0x0486, 0x0487, 0x0592,
	0x0593, 0x0594, 0x0595, 0x0597, 0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1,
	0x05A8, 0x05A9, 0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611, 0x0612, 0x0613, 0x0614, 0x0615,
	0x0616, 0x0617, 0x0657, 0x0658, 0x0659, 0x065A, 0x065B, 0x065D, 0x065E, 0x06D6, 0x06D7, 0x06D8,
	0x06D9, 0x06DA, 0x06DB, 0x06DC, 0x06DF, 0x06E0, 0x06E1, 0x06E2, 0x06E4, 0x06E7, 0x06E8, 0x06EB,
	0x06EC, 0x0730, 0x0732, 0x0733, 0x0735, 0x0736, 0x073A, 0x073D, 0x073F, 0x0740, 0x0741, 0x0743,
	0x0745, 0x0747, 0x0749, 0x074A, 0x07EB, 0x07EC, 0x07ED, 0x07EE, 0x07EF, 0x07F0, 0x07F1, 0x07F3,
	0x0816, 0x0817, 0x0818, 0x0819, 0x081B, 0x081C, 0x081D, 0x081E, 0x081F, 0x0820, 0x0821, 0x0822,
	0x0823, 0x0825, 0x0826, 0x0827, 0x0829, 0x082A, 0x082B, 0x082C,
}

// InTerminal says whether this terminal draws pictures in its cells. Inside tmux or screen it
// doesn't, whatever the terminal around them.
func InTerminal() bool {
	if os.Getenv("TMUX") != "" || strings.HasPrefix(os.Getenv("TERM"), "screen") || strings.HasPrefix(os.Getenv("TERM"), "tmux") {
		return false
	}
	return os.Getenv("TERM") == "xterm-ghostty" || os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("GHOSTTY_RESOURCES_DIR") != ""
}

// Fit is how many cells the frame takes in a space of at most columns by rows, which is as large as
// fits, in the frame's own shape. A cell is about twice as high as it is wide.
func (f Frame) Fit(columns, rows int) (int, int) {
	if f.Width == 0 || f.Height == 0 {
		return 0, 0
	}
	columns, rows = min(columns, MaxCells), min(rows, MaxCells)
	wide := rows * 2 * f.Width / f.Height
	if wide > columns {
		wide = columns
		rows = wide * f.Height / (f.Width * 2)
	}
	if wide < 2 || rows < 1 {
		return 0, 0
	}
	return wide, rows
}

// Scaled is the frame made smaller by a whole factor until it is at most width pixels wide,
// each pixel the average of those it stands for.
func (f Frame) Scaled(width int) Frame {
	factor := 1
	for f.Width/factor > width {
		factor++
	}
	if factor == 1 || f.Width == 0 {
		return f
	}
	small := Frame{Width: f.Width / factor, Height: f.Height / factor}
	small.Pixels = make([]byte, small.Width*small.Height*3)
	for y := 0; y < small.Height; y++ {
		for x := 0; x < small.Width; x++ {
			var sum [3]int
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					at := ((y*factor+dy)*f.Width + x*factor + dx) * 3
					sum[0] += int(f.Pixels[at])
					sum[1] += int(f.Pixels[at+1])
					sum[2] += int(f.Pixels[at+2])
				}
			}
			at := (y*small.Width + x) * 3
			for i := range sum {
				small.Pixels[at+i] = byte(sum[i] / (factor * factor))
			}
		}
	}
	return small
}

// Hand returns what hands the frame to the terminal as picture number id, to be shown
// columns wide and rows high wherever Cells puts its cells. Sent again with the same number,
// it replaces the picture. The terminal is told not to answer.
func (f Frame) Hand(id, columns, rows int) (string, error) {
	file, err := f.Scaled(1280).PNG()
	if err != nil {
		return "", err
	}
	data := base64.StdEncoding.EncodeToString(file)
	var out bytes.Buffer
	for first := true; len(data) > 0; first = false {
		chunk := data[:min(len(data), 4096)]
		data = data[len(chunk):]
		more := 0
		if len(data) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&out, "\x1b_Ga=T,U=1,f=100,q=2,i=%d,c=%d,r=%d,m=%d;%s\x1b\\", id, columns, rows, more, chunk)
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}
	return out.String(), nil
}

// Cells are the lines of cells the terminal fills with picture number id, which is at most 255,
// since the picture's number travels as the cells' colour.
func Cells(id, columns, rows int) string {
	var out strings.Builder
	for row := 0; row < min(rows, MaxCells); row++ {
		if row > 0 {
			out.WriteByte('\n')
		}
		fmt.Fprintf(&out, "\x1b[38;5;%dm", id)
		for column := 0; column < min(columns, MaxCells); column++ {
			out.WriteRune(placeholder)
			out.WriteRune(marks[row])
			out.WriteRune(marks[column])
		}
		out.WriteString("\x1b[39m")
	}
	return out.String()
}

// Forget returns what makes the terminal drop picture number id.
func Forget(id int) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,q=2,i=%d\x1b\\", id)
}
