package aprs

import (
	"math"
	"testing"
)

func TestParseDFReport(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantDF     bool
		wantBRG    float64
		wantNumber int
		wantRange  float64
		wantQual   int
		wantCSE    float64
		wantSPD    float64 // km/h
		wantComment string
	}{
		{
			name:       "standard DF report with CSE/SPD",
			raw:        "N0CALL>APRS:!4903.50N/07201.75W\\088/036/270/729",
			wantDF:     true,
			wantBRG:    270,
			wantNumber: 7,
			wantRange:  math.Pow(2, 2), // R=2 → 4 miles
			wantQual:   9,
			wantCSE:    88,
			wantSPD:    36 * 1.852,
		},
		{
			name:       "DF with zero NRQ",
			raw:        "N0CALL>APRS:!4903.50N/07201.75W\\000/000/180/000",
			wantDF:     true,
			wantBRG:    180,
			wantNumber: 0,
			wantRange:  0, // R=0 → 0
			wantQual:   0,
			wantCSE:    0,
			wantSPD:    0,
		},
		{
			name:       "DF with timestamp",
			raw:        "N0CALL>APRS:@092345z4903.50N/07201.75W\\088/036/090/839",
			wantDF:     true,
			wantBRG:    90,
			wantNumber: 8,
			wantRange:  math.Pow(2, 3), // R=3 → 8 miles
			wantQual:   9,
			wantCSE:    88,
			wantSPD:    36 * 1.852,
		},
		{
			name:       "DF bearing 360",
			raw:        "N0CALL>APRS:!4903.50N/07201.75W\\000/000/360/519",
			wantDF:     true,
			wantBRG:    360,
			wantNumber: 5,
			wantRange:  math.Pow(2, 1), // R=1 → 2 miles
			wantQual:   9,
		},
		{
			name:       "DF with trailing free-text",
			raw:        "N0CALL>APRS:!4903.50N/07201.75W\\088/036/270/729Fox hunt signal",
			wantDF:     true,
			wantBRG:    270,
			wantNumber: 7,
			wantRange:  math.Pow(2, 2),
			wantQual:   9,
			wantComment: "Fox hunt signal",
		},
		{
			name:       "DF with R=9 (max range)",
			raw:        "N0CALL>APRS:!4903.50N/07201.75W\\000/000/045/199",
			wantDF:     true,
			wantBRG:    45,
			wantNumber: 1,
			wantRange:  math.Pow(2, 9), // R=9 → 512 miles
			wantQual:   9,
		},
		{
			name:   "non-DF position should NOT parse DF",
			raw:    "N0CALL>APRS:!4903.50N/07201.75W>088/036",
			wantDF: false,
		},
		{
			name:   "house symbol should NOT parse DF",
			raw:    "N0CALL>APRS:!4903.50N/07201.75W-088/036/270/729",
			wantDF: false,
		},
		{
			name:       "DF with messaging DTI",
			raw:        "N0CALL>APRS:=4903.50N/07201.75W\\088/036/270/729",
			wantDF:     true,
			wantBRG:    270,
			wantNumber: 7,
			wantRange:  math.Pow(2, 2),
			wantQual:   9,
		},
	}

	parser := NewParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := ParseFrame(tt.raw)
			if err != nil {
				t.Fatalf("ParseFrame error: %v", err)
			}

			pkt, err := parser.Parse(frame)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			if tt.wantDF {
				if pkt.DF == nil {
					t.Fatal("expected DF data, got nil")
				}
				if !approxEqual(pkt.DF.Bearing, tt.wantBRG, 0.01) {
					t.Errorf("bearing = %v, want %v", pkt.DF.Bearing, tt.wantBRG)
				}
				if pkt.DF.Number != tt.wantNumber {
					t.Errorf("number = %v, want %v", pkt.DF.Number, tt.wantNumber)
				}
				if !approxEqual(pkt.DF.Range, tt.wantRange, 0.01) {
					t.Errorf("range = %v, want %v", pkt.DF.Range, tt.wantRange)
				}
				if pkt.DF.Quality != tt.wantQual {
					t.Errorf("quality = %v, want %v", pkt.DF.Quality, tt.wantQual)
				}
				if tt.wantCSE != 0 && pkt.Position != nil {
					if !approxEqual(pkt.Position.Course, tt.wantCSE, 0.01) {
						t.Errorf("course = %v, want %v", pkt.Position.Course, tt.wantCSE)
					}
				}
				if tt.wantSPD != 0 && pkt.Position != nil {
					if !approxEqual(pkt.Position.Speed, tt.wantSPD, 0.1) {
						t.Errorf("speed = %v, want %v", pkt.Position.Speed, tt.wantSPD)
					}
				}
				if tt.wantComment != "" && pkt.Position != nil {
					if pkt.Position.Comment != tt.wantComment {
						t.Errorf("comment = %q, want %q", pkt.Position.Comment, tt.wantComment)
					}
				}
				// DF packets should still be PacketTypePosition
				if pkt.Type != PacketTypePosition {
					t.Errorf("packet type = %v, want PacketTypePosition", pkt.Type)
				}
			} else {
				if pkt.DF != nil {
					t.Errorf("expected no DF data, got %+v", pkt.DF)
				}
			}
		})
	}
}

func TestParseDFFromComment(t *testing.T) {
	tests := []struct {
		name        string
		comment     string
		wantDF      bool
		wantBRG     float64
		wantNRQ     string
		wantRemain  string
	}{
		{
			name:    "standard BRG/NRQ",
			comment: "/270/729",
			wantDF:  true,
			wantBRG: 270,
			wantNRQ: "729",
		},
		{
			name:    "BRG/NRQ with trailing text",
			comment: "/090/839Fox hunt",
			wantDF:  true,
			wantBRG: 90,
			wantNRQ: "839",
			wantRemain: "Fox hunt",
		},
		{
			name:    "too short",
			comment: "/27",
			wantDF:  false,
		},
		{
			name:    "no leading slash",
			comment: "270/729",
			wantDF:  false,
		},
		{
			name:    "non-numeric bearing",
			comment: "/abc/729",
			wantDF:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			df, remain := parseDFComment(tt.comment)
			if tt.wantDF {
				if df == nil {
					t.Fatal("expected DF data, got nil")
				}
				if !approxEqual(df.Bearing, tt.wantBRG, 0.01) {
					t.Errorf("bearing = %v, want %v", df.Bearing, tt.wantBRG)
				}
				if tt.wantRemain != "" && remain != tt.wantRemain {
					t.Errorf("remain = %q, want %q", remain, tt.wantRemain)
				}
			} else {
				if df != nil {
					t.Errorf("expected nil DF, got %+v", df)
				}
			}
		})
	}
}
