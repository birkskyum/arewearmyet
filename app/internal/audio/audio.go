// Package audio makes the test tone and listens for it in a recording.
package audio

import (
	"bytes"
	"encoding/binary"
	"math"
)

const (
	// Rate is the sample rate of everything here.
	Rate = 48000
	// Pitch is the test tone's frequency in Hz.
	Pitch = 880
	// window is a quarter second of samples.
	window = Rate / 4
)

// Left and Right name the speaker a tone plays on.
const (
	Left = iota
	Right
)

// Tone is a stereo WAV file with the test tone on one side and silence on the other. It fades
// in and out over 20 ms, so the speakers don't click.
func Tone(side int, seconds float64) []byte {
	frames := int(seconds * Rate)
	fade := Rate / 50
	samples := make([]int16, frames*2)
	for i := 0; i < frames; i++ {
		gain := 1.0
		if i < fade {
			gain = float64(i) / float64(fade)
		} else if frames-i < fade {
			gain = float64(frames-i) / float64(fade)
		}
		samples[i*2+side] = int16(gain * 12000 * math.Sin(2*math.Pi*Pitch*float64(i)/Rate))
	}
	return WAV(samples, 2)
}

// WAV wraps 16-bit samples in a WAV header.
func WAV(samples []int16, channels int) []byte {
	var out bytes.Buffer
	size := uint32(len(samples) * 2)
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, 36+size)
	out.WriteString("WAVEfmt ")
	binary.Write(&out, binary.LittleEndian, uint32(16))
	binary.Write(&out, binary.LittleEndian, uint16(1))
	binary.Write(&out, binary.LittleEndian, uint16(channels))
	binary.Write(&out, binary.LittleEndian, uint32(Rate))
	binary.Write(&out, binary.LittleEndian, uint32(Rate*channels*2))
	binary.Write(&out, binary.LittleEndian, uint16(channels*2))
	binary.Write(&out, binary.LittleEndian, uint16(16))
	out.WriteString("data")
	binary.Write(&out, binary.LittleEndian, size)
	binary.Write(&out, binary.LittleEndian, samples)
	return out.Bytes()
}

// Samples reads the 16-bit samples out of a WAV file, also one whose recorder was stopped
// before it wrote the sizes into the header. Without a header the data is taken as raw samples.
func Samples(file []byte) []int16 {
	start := 0
	if len(file) >= 12 && string(file[:4]) == "RIFF" {
		start = len(file)
		for at := 12; at+8 <= len(file); {
			size := int(binary.LittleEndian.Uint32(file[at+4 : at+8]))
			if string(file[at:at+4]) == "data" {
				start = at + 8
				break
			}
			at += 8 + size + size%2
		}
	}
	data := file[start:]
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

// Level is how loud samples are: their root mean square.
func Level(samples []int16) int {
	if len(samples) == 0 {
		return 0
	}
	var energy float64
	for _, s := range samples {
		energy += float64(s) * float64(s)
	}
	return int(math.Sqrt(energy / float64(len(samples))))
}

// Heard says how well a recording holds the test tone.
type Heard struct {
	// Windows is how many quarter seconds the tone was most of the sound for.
	Windows int
	// Best is the highest share of the sound the tone reached, in percent.
	Best int
	// Level is how loud the recording was while the tone was heard.
	Level int
}

// Clearly says whether the tone was there for at least three quarters of a second.
func (h Heard) Clearly() bool { return h.Windows >= 3 }

// Hear looks for the test tone in a mono recording, a quarter second at a time. Short windows,
// because the speakers' and the microphones' clocks differ a little, which smears the tone out
// of a long measurement; and the tone has to be most of the sound in a window, so room noise
// and the click of the microphones starting don't count.
func Hear(samples []int16) Heard {
	coefficient := 2 * math.Cos(2*math.Pi*Pitch/Rate)
	var heard Heard
	var level float64
	for at := 0; at+window <= len(samples); at += window {
		var energy, s1, s2 float64
		for _, sample := range samples[at : at+window] {
			x := float64(sample)
			energy += x * x
			s1, s2 = x+coefficient*s1-s2, s1
		}
		if energy == 0 {
			continue
		}
		share := 100 * (s1*s1 + s2*s2 - coefficient*s1*s2) / (window / 2 * energy)
		if share >= 50 {
			heard.Windows++
			level += math.Sqrt(energy / window)
		}
		if int(share) > heard.Best {
			heard.Best = int(share)
		}
	}
	if heard.Windows > 0 {
		heard.Level = int(level / float64(heard.Windows))
	}
	return heard
}
