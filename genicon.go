//go:build ignore

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	const size = 256
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{uint8(70 + x/4), 30, uint8(150 + y/6), 255})
		}
	}
	// 尖塔主体
	top, base, halfW := 24, 216, 88
	white := color.RGBA{245, 245, 250, 255}
	for y := top; y < base; y++ {
		half := (y - top) * halfW / (base - top)
		for x := size/2 - half; x <= size/2+half; x++ {
			img.Set(x, y, white)
		}
	}
	// 塔尖宝石
	for y := top; y < top+28; y++ {
		for x := size/2 - 14; x <= size/2+14; x++ {
			dx, dy := x-size/2, y-(top+14)
			if dx*dx+dy*dy <= 14*14 {
				img.Set(x, y, color.RGBA{255, 210, 80, 255})
			}
		}
	}
	f, err := os.Create("icon.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
