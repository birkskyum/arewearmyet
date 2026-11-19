package audio

import (
	"math"
	"testing"
)

// make builds three seconds of mono samples from a function of the sample number.
func build(sample func(i int) float64) []int16 {
	samples := make([]int16, 3*Rate)
	for i := range samples {
		samples[i] = int16(math.Max(-32768, math.Min(32767, sample(i))))
	}
	return samples
}

func sine(i int, pitch, loudness float64) float64 {
	return loudness * math.Sin(2*math.Pi*pitch*float64(i)/Rate)
}

// noise is the same every run, so the tests are too.
func noise(seed *uint32, loudness float64) float64 {
	*seed = *seed*1103515245 + 12345
	return loudness * (float64(*seed>>16&0x7fff)/16384 - 1)
}

func TestHearsTheToneAndNothingElse(t *testing.T) {
	seed := uint32(1)
	cases := []struct {
		name    string
		samples []int16
		clearly bool
	}{
		{"a pure tone", build(func(i int) float64 { return sine(i, Pitch, 3000) }), true},
		{"a quiet tone in room noise", build(func(i int) float64 { return sine(i, Pitch, 600) + noise(&seed, 400) }), true},
		// The microphones' clock runs a little off the speakers'
		{"a tone a little off pitch", build(func(i int) float64 { return sine(i, Pitch+0.7, 250) + noise(&seed, 60) }), true},
		{"a tone that starts late", build(func(i int) float64 {
			if i < Rate {
				return noise(&seed, 40)
			}
			return sine(i, Pitch, 300) + noise(&seed, 40)
		}), true},
		{"noise", build(func(i int) float64 { return noise(&seed, 1500) }), false},
		{"another tone", build(func(i int) float64 { return sine(i, 440, 3000) }), false},
		{"the click of the microphones starting", build(func(i int) float64 {
			if i < Rate/8 {
				return noise(&seed, 9000)
			}
			return noise(&seed, 60)
		}), false},
		{"silence", build(func(i int) float64 { return 0 }), false},
	}
	for _, c := range cases {
		if heard := Hear(c.samples); heard.Clearly() != c.clearly {
			t.Errorf("%s: heard %+v, want clearly=%v", c.name, heard, c.clearly)
		}
	}
}

func TestToneIsOnOneSideAndReadsBack(t *testing.T) {
	for _, side := range []int{Left, Right} {
		stereo := Samples(Tone(side, 1))
		if len(stereo) != 2*Rate {
			t.Fatalf("got %d samples", len(stereo))
		}
		mono := make([]int16, Rate)
		other := make([]int16, Rate)
		for i := range mono {
			mono[i] = stereo[i*2+side]
			other[i] = stereo[i*2+1-side]
		}
		if !Hear(mono).Clearly() || Level(other) != 0 {
			t.Errorf("side %d: tone %+v, other side level %d", side, Hear(mono), Level(other))
		}
	}
}

func TestReadsARecordingWhoseHeaderWasNeverFinished(t *testing.T) {
	file := WAV(build(func(i int) float64 { return sine(i, Pitch, 1000) }), 1)
	// A recorder that was stopped hard leaves the sizes at zero
	for _, at := range []int{4, 5, 6, 7, 40, 41, 42, 43} {
		file[at] = 0
	}
	if samples := Samples(file); len(samples) != 3*Rate || !Hear(samples).Clearly() {
		t.Errorf("read %d samples", len(samples))
	}
	if len(Samples(file[:44])) != 0 {
		t.Error("a header alone gave samples")
	}
}

func TestAQuietRecordingIsBoostedToTheTonesLoudness(t *testing.T) {
	// Speech-like: a quiet stretch, then a louder second
	quiet := build(func(i int) float64 {
		if i < Rate {
			return sine(i, 200, 20)
		}
		return sine(i, 200, 200)
	})
	if got := Decibels(Loudest(quiet)); got > -46 || got < -48 {
		t.Errorf("the quiet recording is at %.1f dB", got)
	}
	boosted, gain := Boost(quiet)
	if gain < 50 || gain > 70 {
		t.Errorf("gain %.1f", gain)
	}
	if difference := Decibels(Loudest(boosted)) - Decibels(ToneLevel); difference > 0.5 || difference < -0.5 {
		t.Errorf("boosted to %.1f dB, the tone is at %.1f", Decibels(Loudest(boosted)), Decibels(ToneLevel))
	}
	// A loud recording is left as it is, and silence isn't turned into noise
	loud := build(func(i int) float64 { return sine(i, 200, 20000) })
	if _, gain := Boost(loud); gain != 1 {
		t.Errorf("a loud recording was changed by %.1f", gain)
	}
	if _, gain := Boost(build(func(i int) float64 { return 0 })); gain != 1 {
		t.Errorf("silence was boosted by %.1f", gain)
	}
	if Decibels(0) > -99 || Decibels(32768) != 0 {
		t.Error("decibels are off")
	}
}
