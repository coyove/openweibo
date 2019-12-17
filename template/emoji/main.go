package main

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"strconv"
)

func main() {
	f, _ := os.Open("sprite-32.png")
	img, _ := png.Decode(f)
	x, y := img.Bounds().Dx(), img.Bounds().Dy()
	// 43x43, last 17 are empty

	for i := 0; i < x/32; i++ {
		for j := 0; j < y/32; j++ {
			if !(i == 8 && j == 33) {
				continue
			}

			c := image.NewRGBA(image.Rect(0, 0, 32, 32))
			draw.Draw(c, c.Bounds(), img, image.Pt(j*32, i*32), draw.Src)

			of, _ := os.Create("emoji" + strconv.Itoa(44) + ".png")
			png.Encode(of, c)
			of.Close()

		}
	}
}
