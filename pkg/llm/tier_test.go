package llm

import "testing"

func TestParseTier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Tier
		wantErr bool
	}{
		{"heavy canonical", "heavy", TierHeavy, false},
		{"standard canonical", "standard", TierStandard, false},
		{"light canonical", "light", TierLight, false},
		{"uppercase", "HEAVY", TierHeavy, false},
		{"padded", "  light  ", TierLight, false},
		{"empty", "", "", true},
		{"whitespace", "   ", "", true},
		{"unknown", "superheavy", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTier(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("ParseTier(%q) expected error, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ParseTier(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseTier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTierValid(t *testing.T) {
	for _, tier := range AllTiers() {
		if !tier.Valid() {
			t.Errorf("AllTiers() returned invalid tier %q", tier)
		}
	}
	if Tier("foo").Valid() {
		t.Error("Tier(foo).Valid() should be false")
	}
	if Tier("").Valid() {
		t.Error("empty Tier.Valid() should be false")
	}
}

func TestAllTiersOrder(t *testing.T) {
	want := []Tier{TierHeavy, TierStandard, TierLight}
	got := AllTiers()
	if len(got) != len(want) {
		t.Fatalf("AllTiers length = %d, want %d", len(got), len(want))
	}
	for i, tier := range want {
		if got[i] != tier {
			t.Errorf("AllTiers[%d] = %q, want %q", i, got[i], tier)
		}
	}
}
