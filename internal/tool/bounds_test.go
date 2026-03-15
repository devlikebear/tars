package tool

import "testing"

func TestResolvePositiveBoundedInt(t *testing.T) {
	t.Parallel()

	override := func(v int) *int { return &v }

	tests := []struct {
		name         string
		defaultValue int
		maxValue     int
		override     *int
		want         int
	}{
		{
			name:         "uses default when override is nil",
			defaultValue: 10,
			maxValue:     20,
			override:     nil,
			want:         10,
		},
		{
			name:         "falls back to default when override is not positive",
			defaultValue: 10,
			maxValue:     20,
			override:     override(0),
			want:         10,
		},
		{
			name:         "clamps to max when override is too large",
			defaultValue: 10,
			maxValue:     20,
			override:     override(25),
			want:         20,
		},
		{
			name:         "uses override when within bounds",
			defaultValue: 10,
			maxValue:     20,
			override:     override(15),
			want:         15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolvePositiveBoundedInt(tt.defaultValue, tt.maxValue, tt.override); got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}
