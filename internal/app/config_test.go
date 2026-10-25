package app

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigReplyDelay(t *testing.T) {
	tests := []struct {
		value   string
		want    time.Duration
		warning bool
	}{
		{"", 1000 * time.Millisecond, false},
		{"0", 0, false},
		{"1", time.Millisecond, false},
		{"800", 800 * time.Millisecond, false},
		{" 900 ", 900 * time.Millisecond, false},
		{"1200", 1200 * time.Millisecond, false},
		{"1201", 1200 * time.Millisecond, true},
		{"60000", 1200 * time.Millisecond, true},
		{"9999999999999", 1200 * time.Millisecond, true},
		{"-1", 1000 * time.Millisecond, true},
		{"abc", 1000 * time.Millisecond, true},
		{"1.5", 1000 * time.Millisecond, true},
		{"1s", 1000 * time.Millisecond, true},
		{"99999999999999999999", 1000 * time.Millisecond, true},
	}
	for _, tt := range tests {
		cfg := LoadConfig(func(k string) string {
			if k == EnvReplyDelayMS {
				return tt.value
			}
			return ""
		})
		if cfg.ReplyDelay != tt.want {
			t.Errorf("%s=%q: ReplyDelay = %v, want %v", EnvReplyDelayMS, tt.value, cfg.ReplyDelay, tt.want)
		}
		if got := cfg.ReplyDelayWarning != nil; got != tt.warning {
			t.Errorf("%s=%q: warning = %v, want warning %v", EnvReplyDelayMS, tt.value, cfg.ReplyDelayWarning, tt.warning)
		}
		if cfg.ReplyDelayWarning != nil && !strings.Contains(cfg.ReplyDelayWarning.Error(), EnvReplyDelayMS) {
			t.Errorf("%s=%q: warning %q does not name the variable", EnvReplyDelayMS, tt.value, cfg.ReplyDelayWarning)
		}
	}
}
