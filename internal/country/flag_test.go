package country

import "testing"

func TestFitsFrame(t *testing.T) {
	tests := []struct {
		name string
		w, h int
		want bool
	}{
		{"1.50 (3:2)", 300, 200, true},
		{"1.43 (10:7)", 320, 224, true},
		{"1.37", 320, 234, true},
		{"1.60 (8:5)", 320, 200, true},
		{"1.67 (5:3)", 320, 192, true},
		{"1.33 (4:3)", 320, 240, false},
		{"1.70", 320, 188, false},
		{"1.75 (7:4)", 280, 160, false},
		{"2.00 (2:1)", 320, 160, false},
		{"1.00 (1:1)", 320, 320, false},
		{"0.82 (portrait)", 82, 100, false},
		{"same as frame", 151, 100, true},
		// Wider than the frame: 151x90 loses exactly 10% of the width.
		{"wide boundary 10%", 151, 90, true},
		{"wide just over 10%", 152, 90, false},
		// Narrower than the frame: 1359x1000 loses exactly 10% of the height.
		{"narrow boundary 10%", 1359, 1000, true},
		{"narrow just over 10%", 1358, 1000, false},
		{"zero width", 0, 100, false},
		{"zero height", 100, 0, false},
		{"negative", -300, -200, false},
	}
	for _, tt := range tests {
		if got := fitsFrame(tt.w, tt.h); got != tt.want {
			t.Errorf("%s: fitsFrame(%d, %d) = %v, want %v", tt.name, tt.w, tt.h, got, tt.want)
		}
	}
}

func TestFlagFitsFrame(t *testing.T) {
	s := Default()
	tests := map[string]bool{
		"JP": true,  // 3:2
		"BR": true,  // 10:7
		"AR": true,  // 8:5
		"DE": true,  // 5:3
		"NO": true,  // 11:8
		"GB": false, // 2:1
		"CH": false, // 1:1
		"NP": false, // portrait
		"US": false, // 19:10
		"CV": false, // 17:10
		"MX": false, // 7:4
		"SM": false, // 4:3
	}
	byCode := map[string]*Country{}
	for _, c := range s.All() {
		byCode[c.Code] = c
	}
	for code, want := range tests {
		c := byCode[code]
		if c == nil {
			t.Errorf("%s not in the data", code)
			continue
		}
		if got := c.FlagFitsFrame(); got != want {
			t.Errorf("%s: FlagFitsFrame() = %v, want %v (size %+v)", code, got, want, flagInfos[code])
		}
	}
	if (&Country{Code: "XA", NameJA: "未登録"}).FlagFitsFrame() {
		t.Error("code without a size: FlagFitsFrame() = true, want false")
	}
}

// TestFlagInfosMatchData fails when a country has no generated flag info or
// an entry belongs to no country. Run go run ./tools/fetchflags to regenerate.
func TestFlagInfosMatchData(t *testing.T) {
	s := Default()
	cover := 0
	for _, c := range s.All() {
		info, ok := flagInfos[c.Code]
		if !ok || info.W <= 0 || info.H <= 0 {
			t.Errorf("%s: flag info = %+v, %v", c.Code, info, ok)
		}
		if c.FlagFitsFrame() {
			cover++
			if c.FlagCornerColor() == "" {
				t.Errorf("%s: cover flag without a corner colour", c.Code)
			}
		}
	}
	if len(flagInfos) != s.Len() {
		t.Errorf("%d flag infos for %d countries", len(flagInfos), s.Len())
	}
	t.Logf("cover %d, contain %d", cover, s.Len()-cover)
}

func TestFlagCornerColor(t *testing.T) {
	byCode := map[string]*Country{}
	for _, c := range Default().All() {
		byCode[c.Code] = c
	}
	tests := map[string]string{
		"JP": "#FFFFFF", // white field
		"DE": "#000000", // black top band
		"BR": "#009440", // green field
		// Left and right differ: the mean of the two.
		"FR": "#671C3D", // #002654 and #CE1126
		"IT": "#675F3F", // #009246 and #CE2B37
	}
	for code, want := range tests {
		c := byCode[code]
		if c == nil {
			t.Errorf("%s not in the data", code)
			continue
		}
		if got := c.FlagCornerColor(); got != want {
			t.Errorf("%s: FlagCornerColor() = %q, want %q", code, got, want)
		}
	}
	if got := (&Country{Code: "XA", NameJA: "未登録"}).FlagCornerColor(); got != "" {
		t.Errorf("code without a flag: FlagCornerColor() = %q, want \"\"", got)
	}
}

func TestFrameCrop(t *testing.T) {
	tests := []struct {
		name           string
		w, h           int
		wantDX, wantDY int
	}{
		{"same as frame", 151, 100, 0, 0},
		{"same as frame x2", 302, 200, 0, 0},
		{"wider (8:5)", 320, 200, 9, 0},
		{"slightly narrower", 320, 213, 0, 1},
		{"narrower", 1359, 1000, 0, 50},
		{"zero width", 0, 100, 0, 0},
		{"zero height", 100, 0, 0, 0},
		{"negative", -300, -200, 0, 0},
	}
	for _, tt := range tests {
		dx, dy := FrameCrop(tt.w, tt.h)
		if dx != tt.wantDX || dy != tt.wantDY {
			t.Errorf("%s: FrameCrop(%d, %d) = %d, %d, want %d, %d", tt.name, tt.w, tt.h, dx, dy, tt.wantDX, tt.wantDY)
		}
	}
}

func TestHexColor(t *testing.T) {
	tests := []struct {
		rgb  uint32
		want string
	}{
		{0x000000, "#000000"},
		{0xFFFFFF, "#FFFFFF"},
		{0x002654, "#002654"},
		{0x000001, "#000001"},
		{0x0A0B0C, "#0A0B0C"},
		{0xABCDEF, "#ABCDEF"},
		{0x6E3368, "#6E3368"},
	}
	for _, tt := range tests {
		if got := hexColor(tt.rgb); got != tt.want {
			t.Errorf("hexColor(%#06x) = %q, want %q", tt.rgb, got, tt.want)
		}
	}
}
