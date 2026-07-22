package app

import (
	"net"
	"testing"
)

func TestDesktopAddr(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: ":8080", want: "127.0.0.1:8080"},
		{in: ":0", want: "127.0.0.1:0"},
		{in: "0.0.0.0:8080", want: "0.0.0.0:8080"},
		{in: "[::]:8080", want: "[::]:8080"},
		{in: "localhost:9090", want: "localhost:9090"},
		{in: "192.168.1.5:8080", want: "192.168.1.5:8080"},
		{in: "8080", wantErr: true},
		{in: "", wantErr: true},
		{in: "127.0.0.1:", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := DesktopAddr(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("DesktopAddr(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DesktopAddr(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("DesktopAddr(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestListenWithFallback(t *testing.T) {
	t.Run("free port", func(t *testing.T) {
		ln, err := ListenWithFallback("127.0.0.1:0")
		if err != nil {
			t.Fatalf("ListenWithFallback: %v", err)
		}
		defer ln.Close()

		host, _, err := net.SplitHostPort(ln.Addr().String())
		if err != nil {
			t.Fatalf("SplitHostPort: %v", err)
		}
		if host != "127.0.0.1" {
			t.Fatalf("host = %q, want 127.0.0.1", host)
		}
	})

	t.Run("busy port falls back", func(t *testing.T) {
		busy, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("occupy port: %v", err)
		}
		defer busy.Close()

		busyAddr := busy.Addr().String()
		ln, err := ListenWithFallback(busyAddr)
		if err != nil {
			t.Fatalf("ListenWithFallback on busy %s: %v", busyAddr, err)
		}
		defer ln.Close()

		gotAddr := ln.Addr().String()
		if gotAddr == busyAddr {
			t.Fatalf("expected different port than busy %s, got %s", busyAddr, gotAddr)
		}
		host, _, err := net.SplitHostPort(gotAddr)
		if err != nil {
			t.Fatalf("SplitHostPort: %v", err)
		}
		if host != "127.0.0.1" {
			t.Fatalf("host = %q, want 127.0.0.1", host)
		}
	})

	t.Run("garbage host", func(t *testing.T) {
		ln, err := ListenWithFallback("999.999.999.999:1")
		if err == nil {
			ln.Close()
			t.Fatal("expected error for garbage host")
		}
	})
}

func TestLocalURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "127.0.0.1:9090", want: "http://127.0.0.1:9090"},
		{in: "0.0.0.0:9090", want: "http://127.0.0.1:9090"},
		{in: "[::]:9090", want: "http://127.0.0.1:9090"},
		{in: "192.168.1.5:8080", want: "http://192.168.1.5:8080"},
		{in: "[::1]:8080", want: "http://[::1]:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := LocalURL(tt.in)
			if got != tt.want {
				t.Fatalf("LocalURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
