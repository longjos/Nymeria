package aprs

import (
	"testing"
)

func TestParseTacticalMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want map[string]string
	}{
		{
			name: "single pair",
			body: "W4ABC-9=SHELTER-1",
			want: map[string]string{"W4ABC-9": "SHELTER-1"},
		},
		{
			name: "multiple pairs",
			body: "W4ABC-9=SHELTER-1;N5XYZ=NET-CTRL",
			want: map[string]string{
				"W4ABC-9": "SHELTER-1",
				"N5XYZ":   "NET-CTRL",
			},
		},
		{
			name: "three pairs",
			body: "W4ABC-9=SHELTER-1;N5XYZ=NET-CTRL;KD0ABC-5=EOC",
			want: map[string]string{
				"W4ABC-9":  "SHELTER-1",
				"N5XYZ":    "NET-CTRL",
				"KD0ABC-5": "EOC",
			},
		},
		{
			name: "with spaces",
			body: " W4ABC-9 = SHELTER-1 ; N5XYZ = NET-CTRL ",
			want: map[string]string{
				"W4ABC-9": "SHELTER-1",
				"N5XYZ":   "NET-CTRL",
			},
		},
		{
			name: "trailing semicolon",
			body: "W4ABC-9=SHELTER-1;",
			want: map[string]string{"W4ABC-9": "SHELTER-1"},
		},
		{
			name: "lowercase callsign uppercased",
			body: "w4abc-9=SHELTER-1",
			want: map[string]string{"W4ABC-9": "SHELTER-1"},
		},
		{
			name: "alias preserves case",
			body: "W4ABC=Shelter One",
			want: map[string]string{"W4ABC": "Shelter One"},
		},
		{
			name: "empty body",
			body: "",
			want: nil,
		},
		{
			name: "whitespace only",
			body: "   ",
			want: nil,
		},
		{
			name: "no equals sign",
			body: "W4ABC-9",
			want: nil,
		},
		{
			name: "equals but no value",
			body: "W4ABC-9=",
			want: nil,
		},
		{
			name: "equals but no key",
			body: "=SHELTER-1",
			want: nil,
		},
		{
			name: "mixed valid and invalid",
			body: "W4ABC-9=SHELTER-1;badpair;N5XYZ=NET-CTRL",
			want: map[string]string{
				"W4ABC-9": "SHELTER-1",
				"N5XYZ":   "NET-CTRL",
			},
		},
		{
			name: "only semicolons",
			body: ";;;",
			want: nil,
		},
		{
			name: "alias with equals sign in value",
			body: "W4ABC=A=B",
			want: map[string]string{"W4ABC": "A=B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTacticalMessage(tt.body)

			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf("length mismatch: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}

			for k, wantV := range tt.want {
				gotV, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("key %q: got %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}
