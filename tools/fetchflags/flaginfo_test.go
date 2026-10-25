package main

import (
	"bytes"
	"go/format"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

func writeImage(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	writeImage(t, path, image.NewRGBA(image.Rect(0, 0, w, h)))
}

// filled returns a w x h image of c.
func filled(w, h int, c color.Color) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

// fill paints r of img with c.
func fill(img draw.Image, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}

func TestCornerSampleConstants(t *testing.T) {
	if cornerInset != 2 || cornerWindow != 3 {
		t.Errorf("cornerInset, cornerWindow = %d, %d, want 2, 3", cornerInset, cornerWindow)
	}
}

func TestInfos(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, FlagPath(dir, "JP"), 30, 20)
	writePNG(t, FlagPath(dir, "GB"), 32, 16)
	writePNG(t, FlagPath(dir, "NP"), 16, 20)
	got, err := Infos(dir, []string{"JP", "GB", "NP"})
	if err != nil {
		t.Fatalf("Infos: %v", err)
	}
	// image.NewRGBA is transparent, which is white once laid over white.
	want := map[string]Info{
		"JP": {W: 30, H: 20, Corner: 0xFFFFFF},
		"GB": {W: 32, H: 16, Corner: 0xFFFFFF},
		"NP": {W: 16, H: 20, Corner: 0xFFFFFF},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Infos = %v, want %v", got, want)
	}
}

func TestInfosCornerColor(t *testing.T) {
	red := color.NRGBA{R: 0xFF, A: 0xFF}
	blue := color.NRGBA{B: 0xFF, A: 0xFF}
	green := color.NRGBA{G: 0xFF, A: 0xFF}
	white := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	black := color.NRGBA{A: 0xFF}

	tests := []struct {
		name  string
		image func() image.Image
		want  uint32
	}{
		{"single colour", func() image.Image {
			return filled(151, 100, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF})
		}, 0x123456},
		{"left and right differ", func() image.Image {
			img := filled(151, 100, blue)
			fill(img, image.Rect(76, 0, 151, 100), red)
			return img
		}, 0x800080},
		{"mean rounds each component half up", func() image.Image {
			img := filled(151, 100, color.NRGBA{R: 1, G: 2, B: 3, A: 0xFF})
			fill(img, image.Rect(76, 0, 151, 100), color.NRGBA{R: 2, G: 4, B: 8, A: 0xFF})
			return img
		}, 0x020306},
		// 200x100 is wider than the frame: 24 columns are cut off each side.
		{"wider: cropped side columns ignored", func() image.Image {
			img := filled(200, 100, red)
			fill(img, image.Rect(0, 0, 24, 100), blue)
			fill(img, image.Rect(176, 0, 200, 100), blue)
			return img
		}, 0xFF0000},
		// 151x150 is narrower than the frame: 25 rows are cut off top and
		// bottom.
		{"narrower: cropped top rows ignored", func() image.Image {
			img := filled(151, 150, green)
			fill(img, image.Rect(0, 0, 151, 25), blue)
			return img
		}, 0x00FF00},
		{"outermost pixels ignored", func() image.Image {
			img := filled(151, 100, white)
			fill(img, image.Rect(0, 0, 151, 1), black)
			fill(img, image.Rect(0, 0, 1, 100), black)
			fill(img, image.Rect(150, 0, 151, 100), black)
			return img
		}, 0xFFFFFF},
		{"window is averaged", func() image.Image {
			img := filled(151, 100, black)
			// One white pixel of nine in each window: 255/9 = 28.3.
			fill(img, image.Rect(2, 2, 3, 3), white)
			fill(img, image.Rect(148, 2, 149, 3), white)
			return img
		}, 0x1C1C1C},
		{"semi-transparent laid over white", func() image.Image {
			return filled(151, 100, color.NRGBA{R: 0xFF, A: 0x80})
		}, 0xFF7F7F},
		{"transparent is white", func() image.Image {
			return filled(151, 100, color.NRGBA{R: 10, G: 20, B: 30})
		}, 0xFFFFFF},
		{"smallest sample", func() image.Image {
			return filled(8, 6, red)
		}, 0xFF0000},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		writeImage(t, FlagPath(dir, "XA"), tt.image())
		got, err := Infos(dir, []string{"XA"})
		if err != nil {
			t.Errorf("%s: Infos: %v", tt.name, err)
			continue
		}
		if c := got["XA"].Corner; c != tt.want {
			t.Errorf("%s: Corner = %#06x, want %#06x", tt.name, c, tt.want)
		}
	}
}

func TestInfosTooSmall(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, FlagPath(dir, "XA"), filled(7, 5, color.Black))
	writeImage(t, FlagPath(dir, "XB"), filled(20, 4, color.Black))
	writeImage(t, FlagPath(dir, "JP"), filled(30, 20, color.White))
	_, err := Infos(dir, []string{"XA", "XB", "JP"})
	if err == nil {
		t.Fatal("err = nil, want failure")
	}
	for _, code := range []string{"XA", "XB"} {
		if !strings.Contains(err.Error(), code+": ") {
			t.Errorf("error does not list %s: %v", code, err)
		}
	}
	if strings.Contains(err.Error(), "JP") {
		t.Errorf("error lists a readable file: %v", err)
	}
}

func TestInfosReportsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, FlagPath(dir, "JP"), 30, 20)
	if err := os.WriteFile(FlagPath(dir, "TW"), fakePNG, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(FlagPath(dir, "FR"), []byte("<html>not a png</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Infos(dir, []string{"JP", "TW", "XK", "FR"})
	if err == nil {
		t.Fatal("err = nil, want failure")
	}
	msg := err.Error()
	for _, code := range []string{"FR", "TW", "XK"} {
		if !strings.Contains(msg, code) {
			t.Errorf("error does not list %s: %v", code, err)
		}
	}
	if strings.Contains(msg, "JP") {
		t.Errorf("error lists a readable file: %v", err)
	}
	if i, j, k := strings.Index(msg, "FR"), strings.Index(msg, "TW"), strings.Index(msg, "XK"); !(i < j && j < k) {
		t.Errorf("codes not in order: %v", err)
	}
}

func TestWriteInfos(t *testing.T) {
	var buf bytes.Buffer
	err := WriteInfos(&buf, map[string]Info{
		"US": {W: 320, H: 168, Corner: 0x3C3B6E},
		"AD": {W: 320, H: 224, Corner: 0x6E3368},
		"JP": {W: 320, H: 213, Corner: 0xFFFFFF},
		"DE": {W: 320, H: 192, Corner: 0x000000},
		"FR": {W: 320, H: 213, Corner: 0x00002A},
	})
	if err != nil {
		t.Fatalf("WriteInfos: %v", err)
	}
	want := `// Code generated by tools/fetchflags; DO NOT EDIT.

package country

// flagInfos is the pixel size of public/flags/{code}.png and the colour of
// its top corners when shown in the image frame, keyed by ISO code.
var flagInfos = map[string]flagInfo{
	"AD": {W: 320, H: 224, Corner: 0x6E3368},
	"DE": {W: 320, H: 192, Corner: 0x000000},
	"FR": {W: 320, H: 213, Corner: 0x00002A},
	"JP": {W: 320, H: 213, Corner: 0xFFFFFF},
	"US": {W: 320, H: 168, Corner: 0x3C3B6E},
}
`
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		t.Fatalf("output does not parse: %v", err)
	}
	if !bytes.Equal(formatted, buf.Bytes()) {
		t.Error("output is not gofmt-ed")
	}
}

func TestWriteInfosEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteInfos(&buf, nil); err != nil {
		t.Fatalf("WriteInfos: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "// Code generated by tools/fetchflags; DO NOT EDIT.\n") {
		t.Errorf("output = %q", buf.String())
	}
	if _, err := format.Source(buf.Bytes()); err != nil {
		t.Errorf("output does not parse: %v", err)
	}
}

// TestFlagInfosUpToDate fails when internal/country/flags_gen.go does not
// match the flags in public/flags.
func TestFlagInfosUpToDate(t *testing.T) {
	store := country.Default()
	var codes []string
	for _, c := range store.All() {
		codes = append(codes, c.Code)
	}
	infos, err := Infos(filepath.Join("..", "..", "public", "flags"), codes)
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := WriteInfos(&want, infos); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join("..", "..", "internal", "country", "flags_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Error("internal/country/flags_gen.go is out of date; run go run ./tools/fetchflags from the repository root")
	}
	entry := regexp.MustCompile(`(?m)^\t"[A-Z]{2}": \{W: \d+, H: \d+, Corner: 0x[0-9A-F]{6}\},$`)
	if n := len(entry.FindAll(got, -1)); n != len(codes) {
		t.Errorf("flags_gen.go has %d entries, want %d", n, len(codes))
	}
}

func TestWriteInfosFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flags_gen.go")
	if err := writeInfosFile(path, map[string]Info{"JP": {W: 30, H: 20, Corner: 0xFFFFFF}}); err != nil {
		t.Fatalf("writeInfosFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"JP": {W: 30, H: 20, Corner: 0xFFFFFF},`) {
		t.Errorf("file = %s", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary file left behind: %v", err)
	}
}

func TestWriteInfosFileNeedsDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "flags_gen.go")
	if err := writeInfosFile(path, map[string]Info{"JP": {W: 30, H: 20}}); err == nil {
		t.Fatal("err = nil, want failure for a missing directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Errorf("directory created: %v", err)
	}
}
