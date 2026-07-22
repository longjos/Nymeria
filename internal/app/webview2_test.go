package app

import "testing"

func TestWebView2VersionUsable(t *testing.T) {
	tests := []struct {
		name string
		pv   string
		want bool
	}{
		{"empty", "", false},
		{"sentinel not installed", "0.0.0.0", false},
		{"real version", "120.0.2210.144", true},
		{"single digit", "1.0.0.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WebView2VersionUsable(tt.pv); got != tt.want {
				t.Errorf("WebView2VersionUsable(%q) = %v, want %v", tt.pv, got, tt.want)
			}
		})
	}
}
