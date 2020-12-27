package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
)

func main() {
	ac := []string{"ac0", "ac1", "ac2", "ac3", "ac4", "ac5", "ac6", "ac8", "ac9", "ac10", "ac11", "ac12", "ac13", "ac14", "ac15", "ac17", "ac23", "ac21", "ac33", "ac34", "ac35", "ac36", "ac37", "ac22", "ac24", "ac25", "ac26", "ac27", "ac28", "ac29", "ac16", "ac18", "ac19", "ac20", "ac30", "ac32", "ac40", "ac44", "ac38", "ac43", "ac31", "ac39", "ac41", "ac7", "ac42", "a2_02", "a2_05", "a2_03", "a2_04", "a2_07", "a2_08", "a2_09", "a2_10", "a2_14", "a2_16", "a2_15", "a2_17", "a2_21", "a2_23", "a2_24", "a2_25", "a2_27", "a2_28", "a2_30", "a2_31", "a2_32", "a2_33", "a2_36", "a2_51", "a2_53", "a2_54", "a2_55", "a2_47", "a2_48", "a2_45", "a2_49", "a2_18", "a2_19", "a2_52", "a2_26", "a2_11", "a2_12", "a2_13", "a2_20", "a2_22", "a2_42", "a2_37", "a2_38", "a2_39", "a2_41", "a2_40"}

	for _, key := range ac {
		resp, _ := http.Get("https://img4.nga.178.com/ngabbs/post/smile/" + key + ".png")
		buf, _ := ioutil.ReadAll(resp.Body)
		ioutil.WriteFile(key+".png", buf, 777)
		fmt.Println(key, len(buf))
	}
	return

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
