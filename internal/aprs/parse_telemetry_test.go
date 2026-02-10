package aprs

import (
	"testing"
)

func TestParseTelemetryPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantSeq int
		wantA   [5]float64
		wantD   byte
		wantC   string
		wantErr bool
	}{
		{
			name:    "standard telemetry",
			payload: "T#123,001,002,003,004,005,10101010",
			wantSeq: 123,
			wantA:   [5]float64{1, 2, 3, 4, 5},
			wantD:   0b10101010,
		},
		{
			name:    "with comment",
			payload: "T#001,100,200,300,400,500,11111111,hello world",
			wantSeq: 1,
			wantA:   [5]float64{100, 200, 300, 400, 500},
			wantD:   0b11111111,
			wantC:   "hello world",
		},
		{
			name:    "all zeros",
			payload: "T#000,000,000,000,000,000,00000000",
			wantSeq: 0,
			wantA:   [5]float64{0, 0, 0, 0, 0},
			wantD:   0,
		},
		{
			name:    "too short",
			payload: "T#123,001",
			wantErr: true,
		},
		{
			name:    "missing T# prefix",
			payload: "X#123,001,002,003,004,005,10101010",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tel, err := parseTelemetryPayload(tt.payload)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tel.Seq != tt.wantSeq {
				t.Errorf("seq: got %d, want %d", tel.Seq, tt.wantSeq)
			}
			if tel.Analog != tt.wantA {
				t.Errorf("analog: got %v, want %v", tel.Analog, tt.wantA)
			}
			if tel.Digital != tt.wantD {
				t.Errorf("digital: got %08b, want %08b", tel.Digital, tt.wantD)
			}
			if tel.Comment != tt.wantC {
				t.Errorf("comment: got %q, want %q", tel.Comment, tt.wantC)
			}
		})
	}
}

func TestIsTelemetryMeta(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"PARM.Vin,Vbat,Temp,Cur,Alt", true},
		{"UNIT.V,V,C,A,m", true},
		{"EQNS.0,1,0,0,1,0,0,1,0,0,1,0,0,1,0", true},
		{"BITS.11111111,My Project", true},
		{"Hello World", false},
		{"PARAM.foo", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsTelemetryMeta(tt.text); got != tt.want {
			t.Errorf("IsTelemetryMeta(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestParseTelemetryPARM(t *testing.T) {
	meta, err := ParseTelemetryPARM("PARM.Vin,Vbat,Temp,Cur,Alt,B1,B2,B3,B4,B5,B6,B7,B8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.MetaType != TelemetryMetaPARM {
		t.Errorf("type: got %q, want %q", meta.MetaType, TelemetryMetaPARM)
	}
	wantNames := [5]string{"Vin", "Vbat", "Temp", "Cur", "Alt"}
	if meta.ParamNames != wantNames {
		t.Errorf("paramNames: got %v, want %v", meta.ParamNames, wantNames)
	}
	wantBits := [8]string{"B1", "B2", "B3", "B4", "B5", "B6", "B7", "B8"}
	if meta.BitLabels != wantBits {
		t.Errorf("bitLabels: got %v, want %v", meta.BitLabels, wantBits)
	}
}

func TestParseTelemetryPARM_partial(t *testing.T) {
	meta, err := ParseTelemetryPARM("PARM.Vin,Vbat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ParamNames[0] != "Vin" || meta.ParamNames[1] != "Vbat" || meta.ParamNames[2] != "" {
		t.Errorf("partial names: got %v", meta.ParamNames)
	}
}

func TestParseTelemetryUNIT(t *testing.T) {
	meta, err := ParseTelemetryUNIT("UNIT.V,V,C,A,m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.MetaType != TelemetryMetaUNIT {
		t.Errorf("type: got %q, want %q", meta.MetaType, TelemetryMetaUNIT)
	}
	want := [5]string{"V", "V", "C", "A", "m"}
	if meta.UnitLabels != want {
		t.Errorf("unitLabels: got %v, want %v", meta.UnitLabels, want)
	}
}

func TestParseTelemetryEQNS(t *testing.T) {
	meta, err := ParseTelemetryEQNS("EQNS.0,0.075,0,0,0.075,0,0,0.5,-30,0,1,0,0,1,0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.MetaType != TelemetryMetaEQNS {
		t.Errorf("type: got %q, want %q", meta.MetaType, TelemetryMetaEQNS)
	}

	// Channel 0: a=0, b=0.075, c=0
	if meta.Equations[0] != [3]float64{0, 0.075, 0} {
		t.Errorf("ch0: got %v", meta.Equations[0])
	}
	// Channel 2: a=0, b=0.5, c=-30
	if meta.Equations[2] != [3]float64{0, 0.5, -30} {
		t.Errorf("ch2: got %v", meta.Equations[2])
	}
}

func TestParseTelemetryEQNS_tooFew(t *testing.T) {
	_, err := ParseTelemetryEQNS("EQNS.0,1,0")
	if err == nil {
		t.Fatal("expected error for too few coefficients")
	}
}

func TestParseTelemetryBITS(t *testing.T) {
	meta, err := ParseTelemetryBITS("BITS.11001100,Weather Station")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.MetaType != TelemetryMetaBITS {
		t.Errorf("type: got %q, want %q", meta.MetaType, TelemetryMetaBITS)
	}
	if meta.BitSense != 0b11001100 {
		t.Errorf("bitSense: got %08b, want 11001100", meta.BitSense)
	}
	if meta.ProjectTitle != "Weather Station" {
		t.Errorf("projectTitle: got %q, want %q", meta.ProjectTitle, "Weather Station")
	}
}

func TestParseTelemetryBITS_noTitle(t *testing.T) {
	meta, err := ParseTelemetryBITS("BITS.10000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.BitSense != 0b10000000 {
		t.Errorf("bitSense: got %08b, want 10000000", meta.BitSense)
	}
	if meta.ProjectTitle != "" {
		t.Errorf("projectTitle: got %q, want empty", meta.ProjectTitle)
	}
}

func TestApplyEquation(t *testing.T) {
	p := &TelemetryParams{}
	p.Equations[0] = [3]float64{0, 0.075, 0}   // linear: 0.075*x
	p.Equations[1] = [3]float64{0.01, 0, -10}   // quadratic: 0.01*x^2 - 10
	p.Equations[2] = [3]float64{0, 0, 0}         // no equation

	if got := p.ApplyEquation(0, 100); got != 7.5 {
		t.Errorf("ch0: got %f, want 7.5", got)
	}
	if got := p.ApplyEquation(1, 10); got != -9 {
		t.Errorf("ch1: got %f, want -9 (0.01*100 + 0*10 - 10)", got)
	}
	// No equation → raw value returned
	if got := p.ApplyEquation(2, 42); got != 42 {
		t.Errorf("ch2: got %f, want 42 (raw passthrough)", got)
	}
	// Out of range channel
	if got := p.ApplyEquation(5, 99); got != 99 {
		t.Errorf("ch5: got %f, want 99", got)
	}
}

func TestParseTelemetryMeta_dispatch(t *testing.T) {
	tests := []struct {
		text     string
		wantType TelemetryMetaType
	}{
		{"PARM.A,B,C,D,E", TelemetryMetaPARM},
		{"UNIT.V,V,C,A,m", TelemetryMetaUNIT},
		{"EQNS.0,1,0,0,1,0,0,1,0,0,1,0,0,1,0", TelemetryMetaEQNS},
		{"BITS.11111111,Test", TelemetryMetaBITS},
	}
	for _, tt := range tests {
		meta, err := ParseTelemetryMeta(tt.text)
		if err != nil {
			t.Errorf("ParseTelemetryMeta(%q): %v", tt.text, err)
			continue
		}
		if meta.MetaType != tt.wantType {
			t.Errorf("ParseTelemetryMeta(%q): type %q, want %q", tt.text, meta.MetaType, tt.wantType)
		}
	}
}

// TestParseMetaInterception verifies that PARM/UNIT/EQNS/BITS messages addressed
// to a callsign are intercepted as TelemetryMeta packets, not regular messages.
func TestParseMetaInterception(t *testing.T) {
	parser := NewParser()

	// Simulate: N0CALL>APRS::N0CALL-1 :PARM.Vin,Vbat,Temp,Cur,Alt
	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Payload:     ":N0CALL-1 :PARM.Vin,Vbat,Temp,Cur,Alt",
	}

	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if pkt.Type != PacketTypeTelemetry {
		t.Errorf("packet type: got %d, want %d (telemetry)", pkt.Type, PacketTypeTelemetry)
	}
	if pkt.TelemetryMeta == nil {
		t.Fatal("TelemetryMeta is nil")
	}
	if pkt.TelemetryMeta.MetaType != TelemetryMetaPARM {
		t.Errorf("meta type: got %q, want %q", pkt.TelemetryMeta.MetaType, TelemetryMetaPARM)
	}
	if pkt.TelemetryMeta.Target != "N0CALL-1" {
		t.Errorf("target: got %q, want %q", pkt.TelemetryMeta.Target, "N0CALL-1")
	}
	if pkt.Message != nil {
		t.Error("Message should be nil for intercepted telemetry meta")
	}
}

// TestParseNormalMessageNotIntercepted verifies that normal messages with similar
// text are NOT intercepted.
func TestParseNormalMessageNotIntercepted(t *testing.T) {
	parser := NewParser()

	frame := APRSFrame{
		Source:      Address{Call: "N0CALL"},
		Destination: Address{Call: "APRS"},
		Payload:     ":N0CALL-1 :Hello World{123",
	}

	pkt, err := parser.Parse(frame)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if pkt.Type != PacketTypeMessage {
		t.Errorf("packet type: got %d, want %d (message)", pkt.Type, PacketTypeMessage)
	}
	if pkt.Message == nil {
		t.Fatal("Message should not be nil")
	}
	if pkt.TelemetryMeta != nil {
		t.Error("TelemetryMeta should be nil for normal messages")
	}
}
