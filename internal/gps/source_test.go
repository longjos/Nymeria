package gps

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		cur  time.Duration
		want time.Duration
	}{
		{0, initialBackoff},
		{1 * time.Second, 2 * time.Second},
		{2 * time.Second, 4 * time.Second},
		{4 * time.Second, 8 * time.Second},
		{8 * time.Second, 16 * time.Second},
		{16 * time.Second, 32 * time.Second},
		{32 * time.Second, 60 * time.Second},
		{60 * time.Second, 60 * time.Second},
	}
	for _, tt := range tests {
		if got := nextBackoff(tt.cur); got != tt.want {
			t.Errorf("nextBackoff(%v) = %v, want %v", tt.cur, got, tt.want)
		}
	}
}
