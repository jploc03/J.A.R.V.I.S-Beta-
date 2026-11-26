//go:build darwin

package main

// Tela no Mac: ver a tela (screencapture) e digitar/teclas (System Events).
// Cliques ainda não são suportados no Mac.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type macScreen struct{}

func init() { screen = macScreen{} }

func (macScreen) Capture(maxW, maxH int) ([]byte, int, int, error) {
	tmp := filepath.Join(os.TempDir(), "jarvis-shot.png")
	defer os.Remove(tmp)
	if err := exec.Command("screencapture", "-x", "-t", "png", tmp).Run(); err != nil {
		return nil, 0, 0, errors.New("couldn't take a screenshot (allow Screen Recording for Jarvis in System Settings → Privacy & Security)")
	}
	f, err := os.Open(tmp)
	if err != nil {
		return nil, 0, 0, err
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil, 0, 0, errors.New("couldn't read the screenshot (allow Screen Recording for Jarvis)")
	}
	b := src.Bounds()
	tw, th := fitSize(b.Dx(), b.Dy(), maxW, maxH)
	dst := downscale(src, tw, th)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), tw, th, nil
}

func fitSize(w, h, maxW, maxH int) (int, int) {
	s := 1.0
	if float64(maxW)/float64(w) < s {
		s = float64(maxW) / float64(w)
	}
	if float64(maxH)/float64(h) < s {
		s = float64(maxH) / float64(h)
	}
	return max(1, int(float64(w)*s)), max(1, int(float64(h)*s))
}

// downscale: média de blocos (boa qualidade para reduzir).
func downscale(src image.Image, tw, th int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		y0 := b.Min.Y + y*b.Dy()/th
		y1 := max(y0+1, b.Min.Y+(y+1)*b.Dy()/th)
		for x := 0; x < tw; x++ {
			x0 := b.Min.X + x*b.Dx()/tw
			x1 := max(x0+1, b.Min.X+(x+1)*b.Dx()/tw)
			var r, g, bl, n uint32
			for yy := y0; yy < y1; yy += 2 {
				for xx := x0; xx < x1; xx += 2 {
					cr, cg, cb, _ := src.At(xx, yy).RGBA()
					r, g, bl, n = r+cr, g+cg, bl+cb, n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), 255
		}
	}
	return dst
}

var macKeyCodes = map[string]int{"return": 36, "enter": 36, "tab": 48, "space": 49, "delete": 51, "backspace": 51, "escape": 53, "esc": 53,
	"left": 123, "right": 124, "down": 125, "up": 126, "home": 115, "end": 119, "page_up": 116, "page_down": 121}

func (macScreen) Do(action string, input map[string]any) (string, error) {
	osa := func(script string) error {
		if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
			return fmt.Errorf("macOS blocked it (allow Accessibility for Jarvis in System Settings → Privacy & Security): %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	switch action {
	case "type":
		t, _ := input["text"].(string)
		t = strings.ReplaceAll(strings.ReplaceAll(t, `\`, `\\`), `"`, `\"`)
		return "OK", osa(`tell application "System Events" to keystroke "` + t + `"`)
	case "key":
		t, _ := input["text"].(string)
		parts := strings.Split(strings.ToLower(t), "+")
		key := parts[len(parts)-1]
		var mods []string
		for _, m := range parts[:len(parts)-1] {
			switch m {
			case "ctrl", "control":
				mods = append(mods, "control down")
			case "cmd", "super", "meta", "win":
				mods = append(mods, "command down")
			case "alt", "option":
				mods = append(mods, "option down")
			case "shift":
				mods = append(mods, "shift down")
			}
		}
		using := ""
		if len(mods) > 0 {
			using = " using {" + strings.Join(mods, ", ") + "}"
		}
		if code, ok := macKeyCodes[key]; ok {
			return "OK", osa(fmt.Sprintf(`tell application "System Events" to key code %d%s`, code, using))
		}
		return "OK", osa(`tell application "System Events" to keystroke "` + key + `"` + using)
	case "wait":
		return "OK", nil
	}
	return "", errors.New("on Mac, Jarvis can look at the screen and type, but can't click yet — ask the student to click")
}

func (macScreen) EscPressed() bool { return false }
