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
	s, err := Default()
	if err != nil {
		t.Fatal(err)
	}
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
			t.Errorf("%s: FlagFitsFrame() = %v, want %v (size %+v)", code, got, want, flagSizes[code])
		}
	}
	if (&Country{Code: "XA", NameJA: "未登録"}).FlagFitsFrame() {
		t.Error("code without a size: FlagFitsFrame() = true, want false")
	}
}

// TestFlagSizesMatchData fails when a country has no generated flag size or
// a size belongs to no country. Run go run ./tools/fetchflags to regenerate.
func TestFlagSizesMatchData(t *testing.T) {
	s, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cover := 0
	for _, c := range s.All() {
		size, ok := flagSizes[c.Code]
		if !ok || size.W <= 0 || size.H <= 0 {
			t.Errorf("%s: flag size = %+v, %v", c.Code, size, ok)
		}
		if c.FlagFitsFrame() {
			cover++
		}
	}
	if len(flagSizes) != s.Len() {
		t.Errorf("%d flag sizes for %d countries", len(flagSizes), s.Len())
	}
	t.Logf("cover %d, contain %d", cover, s.Len()-cover)
}
