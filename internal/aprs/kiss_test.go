package aprs

import (
	"bytes"
	"io"
	"testing"
)

func TestKISSEscape(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		{
			name: "no special bytes",
			in:   []byte{0x01, 0x02, 0x03},
			want: []byte{0x01, 0x02, 0x03},
		},
		{
			name: "escape FEND",
			in:   []byte{0x01, FEND, 0x02},
			want: []byte{0x01, FESC, TFEND, 0x02},
		},
		{
			name: "escape FESC",
			in:   []byte{0x01, FESC, 0x02},
			want: []byte{0x01, FESC, TFESC, 0x02},
		},
		{
			name: "escape both FEND and FESC",
			in:   []byte{FEND, 0x55, FESC},
			want: []byte{FESC, TFEND, 0x55, FESC, TFESC},
		},
		{
			name: "empty data",
			in:   []byte{},
			want: []byte{},
		},
		{
			name: "all FEND bytes",
			in:   []byte{FEND, FEND, FEND},
			want: []byte{FESC, TFEND, FESC, TFEND, FESC, TFEND},
		},
		{
			name: "all FESC bytes",
			in:   []byte{FESC, FESC},
			want: []byte{FESC, TFESC, FESC, TFESC},
		},
		{
			name: "FEND then FESC adjacent",
			in:   []byte{FEND, FESC},
			want: []byte{FESC, TFEND, FESC, TFESC},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KISSEscape(tt.in)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("KISSEscape(%x) = %x, want %x", tt.in, got, tt.want)
			}
		})
	}
}

func TestKISSUnescape(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		{
			name: "no escape sequences",
			in:   []byte{0x01, 0x02, 0x03},
			want: []byte{0x01, 0x02, 0x03},
		},
		{
			name: "unescape TFEND",
			in:   []byte{0x01, FESC, TFEND, 0x02},
			want: []byte{0x01, FEND, 0x02},
		},
		{
			name: "unescape TFESC",
			in:   []byte{0x01, FESC, TFESC, 0x02},
			want: []byte{0x01, FESC, 0x02},
		},
		{
			name: "unescape both",
			in:   []byte{FESC, TFEND, 0x55, FESC, TFESC},
			want: []byte{FEND, 0x55, FESC},
		},
		{
			name: "empty data",
			in:   []byte{},
			want: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KISSUnescape(tt.in)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("KISSUnescape(%x) = %x, want %x", tt.in, got, tt.want)
			}
		})
	}
}

func TestKISSEscapeUnescapeRoundtrip(t *testing.T) {
	// Test roundtrip: data with all possible byte values
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	escaped := KISSEscape(data)
	unescaped := KISSUnescape(escaped)
	if !bytes.Equal(data, unescaped) {
		t.Errorf("roundtrip failed: original %d bytes, escaped %d bytes, unescaped %d bytes", len(data), len(escaped), len(unescaped))
	}
}

func TestKISSEncode(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []byte
	}{
		{
			name: "simple data",
			data: []byte{0x01, 0x02, 0x03},
			want: []byte{FEND, CmdData, 0x01, 0x02, 0x03, FEND},
		},
		{
			name: "data with FEND byte",
			data: []byte{0x01, FEND, 0x02},
			want: []byte{FEND, CmdData, 0x01, FESC, TFEND, 0x02, FEND},
		},
		{
			name: "data with FESC byte",
			data: []byte{0x01, FESC, 0x02},
			want: []byte{FEND, CmdData, 0x01, FESC, TFESC, 0x02, FEND},
		},
		{
			name: "empty data",
			data: []byte{},
			want: []byte{FEND, CmdData, FEND},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KISSEncode(tt.data)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("KISSEncode(%x) = %x, want %x", tt.data, got, tt.want)
			}
		})
	}
}

func TestKISSDecode(t *testing.T) {
	tests := []struct {
		name    string
		frame   []byte
		want    []byte
		wantErr bool
	}{
		{
			name:  "simple frame",
			frame: []byte{FEND, CmdData, 0x01, 0x02, 0x03, FEND},
			want:  []byte{0x01, 0x02, 0x03},
		},
		{
			name:  "frame with escaped FEND",
			frame: []byte{FEND, CmdData, 0x01, FESC, TFEND, 0x02, FEND},
			want:  []byte{0x01, FEND, 0x02},
		},
		{
			name:  "frame with escaped FESC",
			frame: []byte{FEND, CmdData, 0x01, FESC, TFESC, 0x02, FEND},
			want:  []byte{0x01, FESC, 0x02},
		},
		{
			name:  "empty data frame",
			frame: []byte{FEND, CmdData, FEND},
			want:  []byte{},
		},
		{
			name:  "no leading FEND (tolerant)",
			frame: []byte{CmdData, 0x01, 0x02, FEND},
			want:  []byte{0x01, 0x02},
		},
		{
			name:    "too short frame",
			frame:   []byte{FEND},
			wantErr: true,
		},
		{
			name:    "empty input",
			frame:   []byte{},
			wantErr: true,
		},
		{
			name:    "wrong command byte",
			frame:   []byte{FEND, 0x05, 0x01, 0x02, FEND},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := KISSDecode(tt.frame)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("KISSDecode(%x) = %x, want %x", tt.frame, got, tt.want)
			}
		})
	}
}

func TestKISSEncodeDecodeRoundtrip(t *testing.T) {
	data := []byte{0x01, FEND, 0x55, FESC, 0xFF, 0x00}
	encoded := KISSEncode(data)
	decoded, err := KISSDecode(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, decoded) {
		t.Errorf("roundtrip: got %x, want %x", decoded, data)
	}
}

func TestKISSFrameReader(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantFrames [][]byte
		wantErr    bool
	}{
		{
			name: "single frame",
			input: []byte{
				FEND, CmdData, 0x01, 0x02, FEND,
			},
			wantFrames: [][]byte{
				{0x01, 0x02},
			},
		},
		{
			name: "multiple frames",
			input: []byte{
				FEND, CmdData, 0x01, 0x02, FEND,
				FEND, CmdData, 0x03, 0x04, FEND,
			},
			wantFrames: [][]byte{
				{0x01, 0x02},
				{0x03, 0x04},
			},
		},
		{
			name: "multiple FENDs between frames",
			input: []byte{
				FEND, FEND, FEND,
				CmdData, 0x01, 0x02, FEND,
				FEND, FEND,
				CmdData, 0x03, 0x04, FEND,
			},
			wantFrames: [][]byte{
				{0x01, 0x02},
				{0x03, 0x04},
			},
		},
		{
			name: "frame with escape sequences",
			input: []byte{
				FEND, CmdData, 0x01, FESC, TFEND, FESC, TFESC, 0x02, FEND,
			},
			wantFrames: [][]byte{
				{0x01, FEND, FESC, 0x02},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewKISSFrameReader(bytes.NewReader(tt.input))
			var frames [][]byte
			for {
				frame, err := r.ReadFrame()
				if err == io.EOF {
					break
				}
				if err != nil {
					if tt.wantErr {
						return
					}
					t.Fatalf("unexpected error: %v", err)
				}
				frames = append(frames, frame)
			}
			if len(frames) != len(tt.wantFrames) {
				t.Fatalf("got %d frames, want %d", len(frames), len(tt.wantFrames))
			}
			for i, f := range frames {
				if !bytes.Equal(f, tt.wantFrames[i]) {
					t.Errorf("frame[%d] = %x, want %x", i, f, tt.wantFrames[i])
				}
			}
		})
	}
}

func TestKISSFrameReaderSplitReads(t *testing.T) {
	// Simulate a frame split across multiple small reads using a pipe
	data := []byte{FEND, CmdData, 0xAA, 0xBB, 0xCC, FEND}

	// Use a reader that delivers one byte at a time
	r := NewKISSFrameReader(&oneByteReader{data: data})
	frame, err := r.ReadFrame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []byte{0xAA, 0xBB, 0xCC}
	if !bytes.Equal(frame, want) {
		t.Errorf("frame = %x, want %x", frame, want)
	}
}

// oneByteReader delivers one byte at a time from data.
type oneByteReader struct {
	data []byte
	pos  int
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}
