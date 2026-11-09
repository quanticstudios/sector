// Package theme holds sector's colors, type scale and font, taken from
// pitwall's aide-dark tokens so the two look like one product.
package theme

import (
	"bytes"
	_ "embed"
	"image/color"
	"log"
	"os"
	"sync"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	fontapi "github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// Theme is one palette plus a shaper loaded with Geist and a mono face.
type Theme struct {
	Shaper *text.Shaper
	Font   font.Font
	// Mono is JetBrains Mono when the system has it, else Go Mono.
	Mono font.Font

	Bg, Surface, SurfaceSecondary, SurfaceElevated color.NRGBA
	Border, Fg, Muted, Primary                     color.NRGBA
	Red, Yellow, Green, Blue, Purple               color.NRGBA

	// MemUsed and MemCache are the memory graph's two series (violet).
	MemUsed, MemCache color.NRGBA
	// BorderSubtle is a card's ring; TextQuiet the quietest readable text.
	BorderSubtle, TextQuiet color.NRGBA
}

// Type scale (sp) and spacing scale (dp), from pitwall.
const (
	Size11, Size12, Size13, Size14, Size16 unit.Sp = 11, 12, 13, 14, 16

	Space2, Space4, Space8, Space12, Space16, Space24, Space32 unit.Dp = 2, 4, 8, 12, 16, 24, 32

	RadiusCard, RadiusControl, RadiusChip unit.Dp = 12, 8, 6
)

// Dark is aide-dark, the only palette sector ships.
func Dark() *Theme {
	t := &Theme{
		Bg: Hex("#08090c"), Surface: Hex("#14161b"), SurfaceSecondary: Hex("#1c1f26"),
		SurfaceElevated: Hex("#252932"), Border: Hex("#191a1d"), Fg: Hex("#f2f3f5"),
		Muted: Hex("#8e939c"), Primary: Hex("#2997ff"), Red: Hex("#ff6b6b"),
		Yellow: Hex("#ffc533"), Green: Hex("#59d499"), Blue: Hex("#57c1ff"),
		Purple: Hex("#bd93ff"), MemUsed: Hex("#a184f0"), MemCache: Hex("#4a3d78"),
	}
	t.BorderSubtle = Mix(t.Surface, t.Fg, 0.07)
	t.TextQuiet = Mix(t.Bg, t.Muted, 0.85)
	t.Shaper, t.Font, t.Mono = shaper()
	return t
}

// Label draws one line of text at size in c, with the given weight.
func (t *Theme) Label(gtx layout.Context, size unit.Sp, w font.Weight, c color.NRGBA, s string) layout.Dimensions {
	f := t.Font
	f.Weight = w
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	return widget.Label{MaxLines: 1}.Layout(gtx, t.Shaper, f, size, s, m.Stop())
}

// Hex parses "#rrggbb" or "#rrggbbaa" and panics on bad input, because
// every caller passes a literal.
func Hex(s string) color.NRGBA {
	if len(s) != 7 && len(s) != 9 || s[0] != '#' {
		panic("theme: bad hex color " + s)
	}
	n := func(i int) uint8 {
		var v uint8
		for _, c := range s[i : i+2] {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= uint8(c - '0')
			case c >= 'a' && c <= 'f':
				v |= uint8(c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				v |= uint8(c - 'A' + 10)
			default:
				panic("theme: bad hex color " + s)
			}
		}
		return v
	}
	c := color.NRGBA{R: n(1), G: n(3), B: n(5), A: 0xff}
	if len(s) == 9 {
		c.A = n(7)
	}
	return c
}

// Mix is fg laid over bg at alpha a, as an opaque color.
func Mix(bg, fg color.NRGBA, a float32) color.NRGBA {
	l := func(b, f uint8) uint8 { return uint8(float32(b) + (float32(f)-float32(b))*a + 0.5) }
	return color.NRGBA{R: l(bg.R, fg.R), G: l(bg.G, fg.G), B: l(bg.B, fg.B), A: 0xff}
}

// Alpha is c with its alpha set to a (0 to 1).
func Alpha(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(a*255 + 0.5)
	return c
}

// Geist is aide's and pitwall's UI font (OFL), its latin subset as TTF.
//
//go:embed fonts/Geist.ttf
var geistTTF []byte

// varFace pins the variable font to one weight. Each weight needs its own
// parsed font because Gio keys faces by that pointer.
type varFace struct {
	f    *fontapi.Font
	wght float32
}

func (v varFace) Face() *fontapi.Face {
	fc := fontapi.NewFace(v.f)
	fc.SetVariations([]fontapi.Variation{{Tag: ot.MustNewTag("wght"), Value: v.wght}})
	return fc
}

// monoPaths is where a system JetBrains Mono may live; the first that
// parses wins.
var monoPaths = []string{
	"/usr/share/fonts/TTF/JetBrainsMono-Regular.ttf",
	"/usr/share/fonts/TTF/JetBrainsMonoNerdFont-Regular.ttf",
	"/usr/share/fonts/truetype/jetbrains-mono/JetBrainsMono-Regular.ttf",
	"/usr/share/fonts/jetbrains-mono/JetBrainsMono-Regular.ttf",
}

// monoFaces is the system JetBrains Mono, parsed once per process.
var monoFaces = sync.OnceValue(func() []font.FontFace { return loadMono(monoPaths) })

// loadMono parses the first of paths that holds a font, as typeface
// "JetBrains Mono", or returns nil when none does.
func loadMono(paths []string) []font.FontFace {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		faces, err := opentype.ParseCollection(b)
		if err != nil || len(faces) == 0 {
			log.Printf("theme: %s: %v", p, err)
			continue
		}
		faces[0].Font = font.Font{Typeface: "JetBrains Mono"}
		return faces[:1]
	}
	return nil
}

func shaper() (*text.Shaper, font.Font, font.Font) {
	var faces []font.FontFace
	for _, w := range []font.Weight{font.Normal, font.Medium, font.SemiBold, font.Bold} {
		ld, err := ot.NewLoader(bytes.NewReader(geistTTF))
		if err != nil {
			log.Printf("theme: geist: %v", err)
			break
		}
		f, err := fontapi.NewFont(ld)
		if err != nil {
			log.Printf("theme: geist: %v", err)
			break
		}
		faces = append(faces, font.FontFace{Font: font.Font{Typeface: "Geist", Weight: w}, Face: varFace{f: f, wght: float32(400 + w)}})
	}
	ui := font.Font{Typeface: "Geist"}
	if len(faces) == 0 {
		ui = font.Font{Typeface: "Go"}
	}
	mono := font.Font{Typeface: "Go Mono"}
	if m := monoFaces(); m != nil {
		faces = append(faces, m...)
		mono = m[0].Font
	}
	faces = append(faces, gofont.Collection()...)
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces)), ui, mono
}
