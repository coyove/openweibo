package avatar

import (
	"crypto/sha1"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"math/rand"
)

const (
	Blocks    = 5
	BlockSize = 8
	Size      = Blocks * BlockSize
)

var bgColor = image.NewUniform(color.RGBA{0xfa, 0xfb, 0xfc, 0xff})

func Create(w io.Writer, id string) {
	canvas := image.NewRGBA(image.Rect(0, 0, Size, Size))
	seed := sha1.Sum([]byte(id))
	rnd := rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(seed[:]))))

	createColor := func() color.RGBA {
		//saturation is the whole color spectrum
		h := rnd.Float64()
		//saturation goes from 40 to 60, it avoids greyish colors
		s := ((rnd.Float64() * 20) + 40) / 100 //  + '%';
		//lightness can be anything from 0 to 100, but probabilities are a bell curve around 50%
		l := ((rnd.Float64() + rnd.Float64() + rnd.Float64() + rnd.Float64()) * 25) / 100 //  + '%';

		r, g, b := HSLToRGB(h, s, l)
		return color.RGBA{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255), A: 255}
	}

	const dataWidth = Blocks/2 + 1

	data := [Blocks * Blocks]float64{}
	row := [Blocks]float64{}

	for y := 0; y < Blocks; y++ {
		for x := 0; x < dataWidth; x++ {
			// this makes foreground and background color to have a 43% (1/2.3) probability
			// spot color has 13% chance
			row[x] = math.Floor(rnd.Float64() * 2.3)
		}
		for x := dataWidth; x < Blocks; x++ {
			row[x] = row[Blocks-1-x]
		}
		for i, v := range row {
			data[y*Blocks+i] = v
		}
	}

	spotColor := image.NewUniform(createColor())
	color := image.NewUniform(createColor())

	for i := range data {
		y, x := i/Blocks*BlockSize, i%Blocks*BlockSize
		var src image.Image
		switch data[i] {
		case 0:
			src = bgColor
		case 1:
			src = color
		case 2:
			src = spotColor
		}
		draw.Draw(canvas, image.Rect(x, y, x+BlockSize, y+BlockSize), src, image.ZP, draw.Src)
	}

	jpeg.Encode(w, canvas, &jpeg.Options{Quality: 70})
}

// https://github.com/gerow/go-color/blob/master/color.go

func hueToRGB(v1, v2, h float64) float64 {
	if h < 0 {
		h += 1
	}
	if h > 1 {
		h -= 1
	}
	switch {
	case 6*h < 1:
		return (v1 + (v2-v1)*6*h)
	case 2*h < 1:
		return v2
	case 3*h < 2:
		return v1 + (v2-v1)*((2.0/3.0)-h)*6
	}
	return v1
}

func HSLToRGB(h, s, l float64) (float64, float64, float64) {
	if s == 0 {
		// it's gray
		return l, l, l
	}

	var v1, v2 float64
	if l < 0.5 {
		v2 = l * (1 + s)
	} else {
		v2 = (l + s) - (s * l)
	}

	v1 = 2*l - v2

	r := hueToRGB(v1, v2, h+(1.0/3.0))
	g := hueToRGB(v1, v2, h)
	b := hueToRGB(v1, v2, h-(1.0/3.0))

	return r, g, b
}
