package app

import (
	"context"
	"testing"
	"time"
)

func TestSleepWaits(t *testing.T) {
	start := time.Now()
	sleep(context.Background(), 20*time.Millisecond)
	if got := time.Since(start); got < 20*time.Millisecond {
		t.Errorf("slept %v, want at least 20ms", got)
	}
}

func TestSleepReturnsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	sleep(ctx, time.Hour)
	if got := time.Since(start); got > time.Second {
		t.Errorf("canceled context: slept %v", got)
	}

	ctx, cancel = context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	start = time.Now()
	sleep(ctx, time.Hour)
	if got := time.Since(start); got > time.Second {
		t.Errorf("context canceled while sleeping: slept %v", got)
	}
}
