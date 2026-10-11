//go:build ignore

// 从游戏原图生成 UI 缩略图: thumbs/cards|relics/<normalized-id>.png
// 用法: go run gen-thumbs.go <srcDir> <outDir>
package main

import (
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func resizeTo(src image.Image, w int) image.Image {
	b := src.Bounds()
	h := b.Dy() * w / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := float64(b.Dx()), float64(b.Dy())
	for y := 0; y < h; y++ {
		sy := float64(y) * sh / float64(h)
		for x := 0; x < w; x++ {
			sx := float64(x) * sw / float64(w)
			dst.Set(x, y, src.At(b.Min.X+int(sx), b.Min.Y+int(sy)))
		}
	}
	return dst
}

func decode(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if strings.HasSuffix(strings.ToLower(p), ".jpg") || strings.HasSuffix(strings.ToLower(p), ".jpeg") {
		return jpeg.Decode(f)
	}
	return png.Decode(f)
}

func save(img image.Image, p string) error {
	os.MkdirAll(filepath.Dir(p), 0o755)
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func main() {
	srcDir, outDir := os.Args[1], os.Args[2]
	n := 0
	// 卡牌原画: 1024Portraits/<color>/<type>/<name>.png
	filepath.Walk(filepath.Join(srcDir, "images", "1024Portraits"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".png") {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(p), ".png")
		id := norm(base)
		if id == "" {
			return nil
		}
		img, err := decode(p)
		if err != nil {
			return nil
		}
		if err := save(resizeTo(img, 140), filepath.Join(outDir, "cards", id+".png")); err == nil {
			n++
		}
		return nil
	})
	println("cards:", n)
	// 遗物: relics/<lowerCamel>.png
	n = 0
	ents, _ := os.ReadDir(filepath.Join(srcDir, "images", "relics"))
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		id := norm(strings.TrimSuffix(e.Name(), ".png"))
		img, err := decode(filepath.Join(srcDir, "images", "relics", e.Name()))
		if err != nil {
			continue
		}
		if err := save(resizeTo(img, 96), filepath.Join(outDir, "relics", id+".png")); err == nil {
			n++
		}
	}
	println("relics:", n)
	// 药水: 通用瓶子 (glass+liquid 合成)
	n = 0
	glass, err1 := decode(filepath.Join(srcDir, "images", "potion", "potion_m_glass.png"))
	liquid, err2 := decode(filepath.Join(srcDir, "images", "potion", "potion_m_liquid.png"))
	shadow, err3 := decode(filepath.Join(srcDir, "images", "potion", "potion_shadow.png"))
	if err1 == nil && err2 == nil {
		b := glass.Bounds()
		canvas := image.NewRGBA(b)
		if err3 == nil {
			for y := 0; y < b.Dy(); y++ {
				for x := 0; x < b.Dx(); x++ {
					canvas.Set(x, y, shadow.At(x, y))
				}
			}
		}
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				px := liquid.At(x, y)
				if _, _, _, a := px.RGBA(); a > 0 {
					canvas.Set(x, y, px)
				}
			}
		}
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				px := glass.At(x, y)
				if _, _, _, a := px.RGBA(); a > 200 {
					canvas.Set(x, y, px)
				}
			}
		}
		if err := save(resizeTo(canvas, 96), filepath.Join(outDir, "potions", "bottle.png")); err == nil {
			n = 1
		}
	}
	println("potions:", n)
}
