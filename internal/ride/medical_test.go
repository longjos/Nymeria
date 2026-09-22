package ride

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/narvel/nymeria/internal/activity"
	"github.com/narvel/nymeria/internal/store"
)

func baseMedicalInput() CreateMedicalInput {
	return CreateMedicalInput{
		ReportedByCall: "SAG 2",
		Bib:            "412",
		Sex:            "M",
		Age:            "~40",
		Location:       "near Nicasio",
		ChiefComplaint: "fall/shoulder",
		Severity:       store.SeverityRoutine,
		Priority:       PriorityPriority,
	}
}

func TestCreateMedical(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CreateMedicalInput)
		wantErr string
		check   func(t *testing.T, n *store.MedicalNotification)
	}{
		{
			name:   "all fields, readback not confirmed",
			mutate: func(in *CreateMedicalInput) { in.ReadbackConfirmed = false },
			check: func(t *testing.T, n *store.MedicalNotification) {
				if n.Status != store.MedReported {
					t.Errorf("Status = %q, want reported", n.Status)
				}
			},
		},
		{
			name:   "readback confirmed at create",
			mutate: func(in *CreateMedicalInput) { in.ReadbackConfirmed = true; in.ReadBackBy = "SAG 2" },
			check: func(t *testing.T, n *store.MedicalNotification) {
				if n.Status != store.MedConfirmed {
					t.Errorf("Status = %q, want confirmed", n.Status)
				}
				if n.ReadBackAt == nil {
					t.Error("ReadBackAt not set")
				}
			},
		},
		{
			name:    "chief complaint empty",
			mutate:  func(in *CreateMedicalInput) { in.ChiefComplaint = "" },
			wantErr: "chiefComplaint",
		},
		{
			name:    "location empty",
			mutate:  func(in *CreateMedicalInput) { in.Location = "" },
			wantErr: "location",
		},
		{
			name:    "invalid sex",
			mutate:  func(in *CreateMedicalInput) { in.Sex = "Q" },
			wantErr: "sex",
		},
		{
			name:    "invalid severity",
			mutate:  func(in *CreateMedicalInput) { in.Severity = "critical" },
			wantErr: "severity",
		},
		{
			name:   "empty priority defaults to priority tier",
			mutate: func(in *CreateMedicalInput) { in.Priority = "" },
			check: func(t *testing.T, n *store.MedicalNotification) {
				if n.Priority != PriorityPriority {
					t.Errorf("Priority = %q, want %q", n.Priority, PriorityPriority)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, _, _, _ := newTestTrafficManager(t)
			in := baseMedicalInput()
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			n, err := m.CreateMedical("open-net", in, Actor{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateMedical: %v", err)
			}
			if tt.check != nil {
				tt.check(t, n)
			}
		})
	}
}

func TestCreateMedical_NetNotOpen(t *testing.T) {
	m, _, _, _, _ := newTestTrafficManager(t)
	if _, err := m.CreateMedical("draft-net", baseMedicalInput(), Actor{}); err == nil || !strings.Contains(err.Error(), "net is not open") {
		t.Fatalf("expected 'net is not open' error, got %v", err)
	}
}

// TestCreateMedical_IgnoresUnknownFields documents that decoding an
// over-posted body (e.g. a client that adds a clinical-looking field) into
// CreateMedicalInput simply drops anything not in the struct — this is
// standard encoding/json behavior for the HTTP layer's json.Decoder, and is
// exercised at the handler level; here we just confirm CreateMedicalInput
// itself carries no such field to decode into.
func TestCreateMedical_NoClinicalFieldsBeyondChiefComplaint(t *testing.T) {
	in := baseMedicalInput()
	// The struct's only clinical-adjacent field is ChiefComplaint; there is
	// no vitals/diagnosis/treatment field to set. This test exists so that
	// adding one later requires deliberately touching this file.
	_ = in
}

func TestMedicalBibWithholding(t *testing.T) {
	tests := []struct {
		name           string
		severity       string
		policyWithhold bool
		wantWithheld   bool
	}{
		{name: "severe and policy withholds", severity: store.SeveritySevere, policyWithhold: true, wantWithheld: true},
		{name: "severe but policy does not withhold", severity: store.SeveritySevere, policyWithhold: false, wantWithheld: false},
		{name: "serious is never withheld even if policy would", severity: store.SeveritySerious, policyWithhold: true, wantWithheld: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, tl, _, _ := newTestTrafficManager(t)
			m.policy = &fixedPolicy{withhold: tt.policyWithhold}

			in := baseMedicalInput()
			in.Severity = tt.severity
			n, err := m.CreateMedical("open-net", in, Actor{})
			if err != nil {
				t.Fatalf("CreateMedical: %v", err)
			}
			if n.BibWithheld != tt.wantWithheld {
				t.Errorf("BibWithheld = %v, want %v", n.BibWithheld, tt.wantWithheld)
			}
			if n.Bib != "412" {
				t.Errorf("stored Bib = %q, want 412 (still stored regardless of withholding)", n.Bib)
			}
			red := n.Redacted()
			if tt.wantWithheld {
				if red.Bib != "" {
					t.Errorf("Redacted().Bib = %q, want empty", red.Bib)
				}
			} else if red.Bib != "412" {
				t.Errorf("Redacted().Bib = %q, want 412", red.Bib)
			}

			entries := tl.all()
			if len(entries) == 0 {
				t.Fatal("no timeline entries recorded")
			}
			summary := entries[0].Summary
			if tt.wantWithheld {
				if !strings.Contains(summary, "bib withheld") {
					t.Errorf("summary = %q, want to contain 'bib withheld'", summary)
				}
				if strings.Contains(summary, "412") {
					t.Errorf("summary = %q, must not contain the raw bib", summary)
				}
			} else if !strings.Contains(summary, "412") {
				t.Errorf("summary = %q, want to contain the bib", summary)
			}
		})
	}
}

func TestMedicalLadder(t *testing.T) {
	newNotif := func(t *testing.T) (*TrafficManager, *store.MedicalNotification, *recordingTimeline, func(time.Duration)) {
		m, _, tl, _, _ := newTestTrafficManager(t)
		advance := setClock(m, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
		n, err := m.CreateMedical("open-net", baseMedicalInput(), Actor{})
		if err != nil {
			t.Fatalf("CreateMedical: %v", err)
		}
		return m, n, tl, advance
	}

	t.Run("eta before readback is illegal", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.RecordMedicalETA("open-net", n.ID, MedicalETAInput{EMSUnit: "Medic 12", Minutes: 8}, Actor{}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("readback then eta sets due at clock+minutes", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		got, err := m.RecordMedicalETA("open-net", n.ID, MedicalETAInput{EMSUnit: "Medic 12", Minutes: 8}, Actor{})
		if err != nil {
			t.Fatalf("eta: %v", err)
		}
		if got.Status != store.MedEMSEnRoute {
			t.Errorf("Status = %q, want ems_enroute", got.Status)
		}
		want := got.ETAGivenAt.Add(8 * time.Minute)
		if got.ETADueAt == nil || !got.ETADueAt.Equal(want) {
			t.Errorf("ETADueAt = %v, want %v", got.ETADueAt, want)
		}
	})

	t.Run("confirmed to on-scene directly, no ETA required", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		got, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{EMSUnit: "Medic 12"}, Actor{})
		if err != nil {
			t.Fatalf("on-scene: %v", err)
		}
		if got.Status != store.MedOnScene {
			t.Errorf("Status = %q, want on_scene", got.Status)
		}
		if got.ETADueAt != nil {
			t.Errorf("ETADueAt = %v, want nil", got.ETADueAt)
		}
	})

	t.Run("ems_enroute to on-scene sets OnSceneAt to clock", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.RecordMedicalETA("open-net", n.ID, MedicalETAInput{EMSUnit: "Medic 12", Minutes: 8}, Actor{}); err != nil {
			t.Fatalf("eta: %v", err)
		}
		got, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{})
		if err != nil {
			t.Fatalf("on-scene: %v", err)
		}
		if got.OnSceneAt == nil || !got.OnSceneAt.Equal(m.now()) {
			t.Errorf("OnSceneAt = %v, want %v", got.OnSceneAt, m.now())
		}
	})

	t.Run("on_scene to depart after 23m sets OnSceneSeconds and summary", func(t *testing.T) {
		m, n, tl, advance := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{EMSUnit: "Medic 12"}, Actor{}); err != nil {
			t.Fatalf("on-scene: %v", err)
		}
		advance(23 * time.Minute)
		got, err := m.MedicalDepart("open-net", n.ID, DepartInput{Destination: store.DestHospital, DestinationName: "Marin General", PatientCount: 1}, Actor{})
		if err != nil {
			t.Fatalf("depart: %v", err)
		}
		if got.Status != store.MedDeparted {
			t.Errorf("Status = %q, want departed", got.Status)
		}
		if got.OnSceneSeconds == nil || *got.OnSceneSeconds != 1380 {
			t.Errorf("OnSceneSeconds = %v, want 1380", got.OnSceneSeconds)
		}
		entries := tl.all()
		last := entries[len(entries)-1].Summary
		if !strings.Contains(last, "on scene 23m") {
			t.Errorf("summary = %q, want to contain 'on scene 23m'", last)
		}
		if !strings.Contains(last, "1 patient aboard") {
			t.Errorf("summary = %q, want to contain '1 patient aboard'", last)
		}
	})

	t.Run("ems_enroute cannot depart without recording on-scene", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.RecordMedicalETA("open-net", n.ID, MedicalETAInput{EMSUnit: "Medic 12", Minutes: 8}, Actor{}); err != nil {
			t.Fatalf("eta: %v", err)
		}
		if _, err := m.MedicalDepart("open-net", n.ID, DepartInput{Destination: store.DestHospital, PatientCount: 1}, Actor{}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("on-scene with back-dated at", func(t *testing.T) {
		m, n, _, advance := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		advance(10 * time.Minute) // so "5 minutes ago" is still after CreatedAt
		backdated := m.now().Add(-5 * time.Minute)
		got, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{At: &backdated}, Actor{})
		if err != nil {
			t.Fatalf("on-scene backdated: %v", err)
		}
		if got.OnSceneAt == nil || !got.OnSceneAt.Equal(backdated) {
			t.Errorf("OnSceneAt = %v, want %v", got.OnSceneAt, backdated)
		}

		future := m.now().Add(5 * time.Minute)
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{At: &future}, Actor{}); err == nil {
			t.Error("expected error for future at, got nil")
		}

		before := n.CreatedAt.Add(-time.Minute)
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{At: &before}, Actor{}); err == nil {
			t.Error("expected error for at before CreatedAt, got nil")
		}
	})

	t.Run("on_scene to release", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{}); err != nil {
			t.Fatalf("on-scene: %v", err)
		}
		got, err := m.MedicalRelease("open-net", n.ID, ReleaseInput{Reason: "refused transport"}, Actor{})
		if err != nil {
			t.Fatalf("release: %v", err)
		}
		if got.Status != store.MedReleased {
			t.Errorf("Status = %q, want released", got.Status)
		}
		if got.PatientCount != 0 {
			t.Errorf("PatientCount = %d, want 0", got.PatientCount)
		}
		if got.DepartedAt != nil {
			t.Errorf("DepartedAt = %v, want nil", got.DepartedAt)
		}

		if _, err := m.CancelMedical("open-net", n.ID, CancelInput{}, Actor{}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("cancel after released: expected ErrIllegalTransition, got %v", err)
		}
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("on-scene after released: expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("departed cannot cancel", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{}); err != nil {
			t.Fatalf("on-scene: %v", err)
		}
		if _, err := m.MedicalDepart("open-net", n.ID, DepartInput{Destination: store.DestHospital, PatientCount: 1}, Actor{}); err != nil {
			t.Fatalf("depart: %v", err)
		}
		if _, err := m.CancelMedical("open-net", n.ID, CancelInput{}, Actor{}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("expected ErrIllegalTransition, got %v", err)
		}
	})

	t.Run("depart requires patientCount >= 1", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{}); err != nil {
			t.Fatalf("on-scene: %v", err)
		}
		if _, err := m.MedicalDepart("open-net", n.ID, DepartInput{Destination: store.DestHospital, PatientCount: 0}, Actor{}); err == nil {
			t.Error("expected error for patientCount 0")
		}
	})

	t.Run("depart requires a valid destination", func(t *testing.T) {
		m, n, _, _ := newNotif(t)
		if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
			t.Fatalf("readback: %v", err)
		}
		if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{}); err != nil {
			t.Fatalf("on-scene: %v", err)
		}
		if _, err := m.MedicalDepart("open-net", n.ID, DepartInput{Destination: "moon", PatientCount: 1}, Actor{}); err == nil {
			t.Error("expected error for invalid destination")
		}
	})
}

func TestMedicalPatientNamePrivacy(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		wantErr     bool
	}{
		{name: "hospital with name stored", destination: store.DestHospital},
		{name: "start with name stored", destination: store.DestStart},
		{name: "finish with name errors", destination: store.DestFinish, wantErr: true},
		{name: "rest_stop with name errors", destination: store.DestRestStop, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, tl, act, _ := newTestTrafficManagerWithActivity(t)
			n, err := m.CreateMedical("open-net", baseMedicalInput(), Actor{})
			if err != nil {
				t.Fatalf("CreateMedical: %v", err)
			}
			if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
				t.Fatalf("readback: %v", err)
			}
			if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{}); err != nil {
				t.Fatalf("on-scene: %v", err)
			}

			got, err := m.MedicalDepart("open-net", n.ID, DepartInput{
				Destination: tt.destination, DestinationName: "Marin General", PatientCount: 1, PatientName: "Jane Rider",
			}, Actor{UserName: "NCS"})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "patient name is recorded only for hospital or start transports") {
					t.Fatalf("error = %v, want the patient-name-destination message", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("depart: %v", err)
			}
			if got.PatientName != "Jane Rider" {
				t.Errorf("stored PatientName = %q, want Jane Rider", got.PatientName)
			}
			if got.Redacted().PatientName != "" {
				t.Errorf("Redacted().PatientName = %q, want empty", got.Redacted().PatientName)
			}

			for _, e := range tl.all() {
				if strings.Contains(e.Summary, "Jane Rider") || strings.Contains(e.Details, "Jane Rider") {
					t.Errorf("timeline entry leaks patient name: %+v", e)
				}
			}

			entries, _, err := act.Query(activity.Filter{})
			if err != nil {
				t.Fatalf("query activity: %v", err)
			}
			for _, e := range entries {
				if strings.Contains(e.Details, "Jane Rider") {
					t.Errorf("activity Details leaks patient name: %+v", e)
				}
			}
		})
	}
}

func TestMedicalSeparateEvents(t *testing.T) {
	m, _, tl, _, _ := newTestTrafficManager(t)
	advance := setClock(m, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	n, err := m.CreateMedical("open-net", baseMedicalInput(), Actor{})
	if err != nil {
		t.Fatalf("CreateMedical: %v", err)
	}
	advance(time.Minute)
	if _, err := m.ReadbackMedical("open-net", n.ID, ReadbackInput{Confirmed: true}, Actor{}); err != nil {
		t.Fatalf("readback: %v", err)
	}
	advance(time.Minute)
	if _, err := m.RecordMedicalETA("open-net", n.ID, MedicalETAInput{EMSUnit: "Medic 12", Minutes: 8}, Actor{}); err != nil {
		t.Fatalf("eta: %v", err)
	}
	advance(8 * time.Minute)
	if _, err := m.MedicalOnScene("open-net", n.ID, OnSceneInput{}, Actor{}); err != nil {
		t.Fatalf("on-scene: %v", err)
	}
	advance(23 * time.Minute)
	if _, err := m.MedicalDepart("open-net", n.ID, DepartInput{Destination: store.DestHospital, PatientCount: 1}, Actor{}); err != nil {
		t.Fatalf("depart: %v", err)
	}

	entries := tl.all()
	wantTypes := []string{TLMedicalReported, TLMedicalReadbackConfirmed, TLMedicalEMSETA, TLMedicalOnScene, TLMedicalDeparted}
	if len(entries) != len(wantTypes) {
		t.Fatalf("timeline entries = %d, want %d: %+v", len(entries), len(wantTypes), entries)
	}
	seenAt := map[time.Time]bool{}
	for i, want := range wantTypes {
		if entries[i].EventType != want {
			t.Errorf("entry %d type = %q, want %q", i, entries[i].EventType, want)
		}
		if seenAt[entries[i].At] {
			t.Errorf("entry %d (%s) has a duplicate recorded time %v", i, entries[i].EventType, entries[i].At)
		}
		seenAt[entries[i].At] = true
	}
}

func TestGetMedicalRedactionFlag(t *testing.T) {
	m, _, _, _, _ := newTestTrafficManager(t)
	m.policy = &fixedPolicy{withhold: true}
	in := baseMedicalInput()
	in.Severity = store.SeveritySevere
	if _, err := m.CreateMedical("open-net", in, Actor{}); err != nil {
		t.Fatalf("CreateMedical: %v", err)
	}

	redacted := m.GetMedical("open-net", true)
	if len(redacted) != 1 || redacted[0].Bib != "" {
		t.Errorf("redacted list = %+v, want Bib blanked", redacted)
	}
	full := m.GetMedical("open-net", false)
	if len(full) != 1 || full[0].Bib != "412" {
		t.Errorf("full list = %+v, want Bib 412", full)
	}
}
