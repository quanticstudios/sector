package theme

import "testing"

func TestHexAndMix(t *testing.T) {
	if c := Hex("#2997ff"); c.R != 0x29 || c.G != 0x97 || c.B != 0xff || c.A != 0xff {
		t.Fatalf("Hex = %v", c)
	}
	if c := Mix(Hex("#000000"), Hex("#ffffff"), 0.5); c.R != 128 {
		t.Fatalf("Mix = %v", c)
	}
	if Dark().Shaper == nil {
		t.Fatal("no shaper")
	}
}

func TestLoadMonoFallsBack(t *testing.T) {
	if f := loadMono([]string{"/nonexistent/mono.ttf"}); f != nil {
		t.Fatalf("loadMono of a missing file = %v, want nil", f)
	}
	if m := Dark().Mono.Typeface; m != "JetBrains Mono" && m != "Go Mono" {
		t.Fatalf("Mono = %q", m)
	}
}
