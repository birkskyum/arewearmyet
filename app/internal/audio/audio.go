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

// Level is how loud samples are, as their root mean square.
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

// Hear looks for the test tone in a mono recording.
func Hear(samples []int16) Heard { return HearAt(samples, Pitch) }

// Both makes a WAV file with a tone of the given pitch on both speakers, fading in and out.
func Both(pitch, seconds float64) []byte {
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
		value := int16(gain * 12000 * math.Sin(2*math.Pi*pitch*float64(i)/Rate))
		samples[i*2], samples[i*2+1] = value, value
	}
	return WAV(samples, 2)
}

// Quietest is the level of a recording's quietest quarter second, which is its noise with nothing
// playing.
func Quietest(samples []int16) int {
	quietest := -1
	for at := 0; at+window <= len(samples); at += window {
		if level := Level(samples[at : at+window]); quietest < 0 || level < quietest {
			quietest = level
		}
	}
	return max(quietest, 0)
}

// HearAt looks for a tone of the given pitch in a mono recording, a quarter second at a time. The
// windows are short because the speakers' and the microphones' clocks differ a little, which smears
// the tone out of a long measurement. And the tone has to be most of the sound in a window, so room
// noise and the click of the microphones starting don't count.
func HearAt(samples []int16, pitch float64) Heard {
	coefficient := 2 * math.Cos(2*math.Pi*pitch/Rate)
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

// Decibels is a level relative to the loudest a recording can hold, so 0 is full scale, -6 is half
// of it, and silence is far below.
func Decibels(level int) float64 {
	if level <= 0 {
		return -100
	}
	return 20 * math.Log10(float64(level)/32768)
}

// ToneLevel is the level the test tone plays at, the loudness the owner has just heard and
// set their volume by.
var ToneLevel = Level(func() []int16 {
	stereo := Samples(Tone(Left, 1))
	mono := make([]int16, len(stereo)/2)
	for i := range mono {
		mono[i] = stereo[i*2]
	}
	return mono
}())

// Loudest is the level of a recording's loudest fifth of a second, which for speech is the
// level of the voice and not of the pauses.
func Loudest(samples []int16) int {
	loudest := 0
	for at := 0; at+Rate/5 <= len(samples); at += Rate / 5 {
		loudest = max(loudest, Level(samples[at:at+Rate/5]))
	}
	return loudest
}

// Boost returns a recording made as loud as the test tone, so a quiet recording can be
// judged for clearness. It never boosts more than a hundredfold, which would only be noise,
// and never makes a recording quieter.
func Boost(samples []int16) (boosted []int16, gain float64) {
	loudest := Loudest(samples)
	if loudest == 0 {
		return samples, 1
	}
	gain = math.Max(1, math.Min(100, float64(ToneLevel)/float64(loudest)))
	boosted = make([]int16, len(samples))
	for i, sample := range samples {
		boosted[i] = int16(math.Max(-32768, math.Min(32767, float64(sample)*gain)))
	}
	return boosted, gain
}
