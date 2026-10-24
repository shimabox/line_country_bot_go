package bot

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, text, lower string
	}{
		{"日本", "日本", "日本"},
		{" 日 本 ", "日本", "日本"},
		{"日　本　", "日本", "日本"},
		{"日\n本\r\n", "日本", "日本"},
		{"\t日本\v\f", "日本", "日本"},
		{" 日本 ", "日本", "日本"},
		{"Japan", "Japan", "japan"},
		{"QUIZ", "QUIZ", "quiz"},
		{"United States", "UnitedStates", "unitedstates"},
		{"ＱＵＩＺ", "ＱＵＩＺ", "ＱＵＩＺ"},
		{"", "", ""},
		{" 　\n", "", ""},
	}
	for _, tt := range tests {
		got := Normalize(tt.in)
		if got.Text != tt.text || got.Lower != tt.lower {
			t.Errorf("Normalize(%q) = %+v, want text=%q lower=%q", tt.in, got, tt.text, tt.lower)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		in             string
		profileEnabled bool
		want           Trigger
	}{
		{"クイズ", false, TriggerQuiz},
		{"quiz", false, TriggerQuiz},
		{"QUIZ", false, TriggerQuiz},
		{" Quiz\n", false, TriggerQuiz},
		{"ク イ ズ", false, TriggerQuiz},
		{"クイズ！", false, TriggerCountry},
		{"君の名は", true, TriggerProfile},
		{"君の名は", false, TriggerCountry},
		{"who", true, TriggerProfile},
		{"WHO", true, TriggerProfile},
		{" W h o ", true, TriggerProfile},
		{"who", false, TriggerCountry},
		{"いいえ", false, TriggerCancel},
		{"いいえ", true, TriggerCancel},
		{" いいえ　", false, TriggerCancel},
		{"はい", false, TriggerCountry},
		{"日本", true, TriggerCountry},
		{"", false, TriggerCountry},
	}
	for _, tt := range tests {
		if got := Classify(Normalize(tt.in), tt.profileEnabled); got != tt.want {
			t.Errorf("Classify(%q, profile=%v) = %v, want %v", tt.in, tt.profileEnabled, got, tt.want)
		}
	}
}

func TestProfile(t *testing.T) {
	full := Profile{
		ImageURL: "https://example.com/p.png",
		Title:    "国名bot",
		Text:     "国名を送ってください",
		LinkText: "ソースコード",
		LinkURL:  "https://example.com/",
	}
	if !full.Enabled() {
		t.Error("full profile disabled")
	}
	if err := full.Validate(); err != nil {
		t.Errorf("Validate full: %v", err)
	}

	if (Profile{}).Enabled() {
		t.Error("empty profile enabled")
	}
	if err := (Profile{}).Validate(); err != nil {
		t.Errorf("Validate empty (disabled on purpose): %v", err)
	}

	partial := full
	partial.LinkURL = ""
	if partial.Enabled() {
		t.Error("profile without link URL enabled")
	}
	if err := partial.Validate(); err == nil {
		t.Error("Validate partial: err = nil, want missing field error")
	}

	noImage := full
	noImage.ImageURL = ""
	if noImage.Enabled() {
		t.Error("profile without image enabled")
	}
	if err := noImage.Validate(); err != nil {
		t.Errorf("Validate without image (disabled on purpose): %v", err)
	}
}
