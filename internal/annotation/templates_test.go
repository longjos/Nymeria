package annotation

import "testing"

func TestAllTemplates(t *testing.T) {
	templates := AllTemplates()
	if len(templates) < 15 {
		t.Errorf("expected at least 15 templates, got %d", len(templates))
	}
}

func TestTemplatesByPack(t *testing.T) {
	tests := []struct {
		pack     string
		wantMin  int
	}{
		{"event", 5},
		{"sar", 5},
		{"disaster", 5},
		{"nonexistent", 0},
	}

	for _, tt := range tests {
		got := TemplatesByPack(tt.pack)
		if len(got) < tt.wantMin {
			t.Errorf("TemplatesByPack(%q): got %d, want >= %d", tt.pack, len(got), tt.wantMin)
		}
		for _, tmpl := range got {
			if tmpl.Pack != tt.pack {
				t.Errorf("TemplatesByPack(%q) returned template with pack %q", tt.pack, tmpl.Pack)
			}
		}
	}
}

func TestTemplateCategoryGeometryValid(t *testing.T) {
	for _, tmpl := range AllTemplates() {
		if err := ValidateCategory(tmpl.Category); err != nil {
			t.Errorf("template %q has invalid category %q: %v", tmpl.ID, tmpl.Category, err)
		}
		if err := ValidateCategoryGeometry(tmpl.Category, tmpl.Type); err != nil {
			t.Errorf("template %q: category %q incompatible with type %q: %v", tmpl.ID, tmpl.Category, tmpl.Type, err)
		}
		if err := ValidatePriority(tmpl.DefaultPriority); err != nil {
			t.Errorf("template %q has invalid priority %q: %v", tmpl.ID, tmpl.DefaultPriority, err)
		}
	}
}
