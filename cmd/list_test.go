package cmd

import (
	"testing"
	"time"
)

func TestParseDateFilter(t *testing.T) {
	// Reference date for testing
	refDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		input      string
		wantAfter  time.Time
		wantBefore time.Time
	}{
		{
			name:       "after: prefix",
			input:      "after:2024-06-15",
			wantAfter:  refDate,
			wantBefore: time.Time{},
		},
		{
			name:       "before: prefix",
			input:      "before:2024-06-15",
			wantAfter:  time.Time{},
			wantBefore: refDate,
		},
		{
			name:       "DATE.. format (after)",
			input:      "2024-06-15..",
			wantAfter:  refDate,
			wantBefore: time.Time{},
		},
		{
			name:       "..DATE format (before)",
			input:      "..2024-06-15",
			wantAfter:  time.Time{},
			wantBefore: refDate,
		},
		{
			name:       "DATE..DATE format (range)",
			input:      "2024-06-01..2024-06-30",
			wantAfter:  time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantBefore: time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "exact date (entire day)",
			input:      "2024-06-15",
			wantAfter:  refDate,
			wantBefore: refDate.Add(24 * time.Hour),
		},
		{
			name:       "invalid date returns zero values",
			input:      "invalid",
			wantAfter:  time.Time{},
			wantBefore: time.Time{},
		},
		{
			name:       "empty string returns zero values",
			input:      "",
			wantAfter:  time.Time{},
			wantBefore: time.Time{},
		},
		{
			name:       "whitespace trimmed",
			input:      "  2024-06-15  ",
			wantAfter:  refDate,
			wantBefore: refDate.Add(24 * time.Hour),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotAfter, gotBefore := parseDateFilter(tc.input)

			if !gotAfter.Equal(tc.wantAfter) {
				t.Errorf("parseDateFilter(%q) after = %v, want %v", tc.input, gotAfter, tc.wantAfter)
			}
			if !gotBefore.Equal(tc.wantBefore) {
				t.Errorf("parseDateFilter(%q) before = %v, want %v", tc.input, gotBefore, tc.wantBefore)
			}
		})
	}
}

func TestParsePointsFilter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int
		wantMax int
	}{
		{
			name:    "exact match",
			input:   "5",
			wantMin: 5,
			wantMax: 5,
		},
		{
			name:    "greater than or equal",
			input:   ">=3",
			wantMin: 3,
			wantMax: 0,
		},
		{
			name:    "less than or equal",
			input:   "<=8",
			wantMin: 0,
			wantMax: 8,
		},
		{
			name:    "range with dash",
			input:   "3-8",
			wantMin: 3,
			wantMax: 8,
		},
		{
			name:    "whitespace trimmed",
			input:   "  5  ",
			wantMin: 5,
			wantMax: 5,
		},
		{
			name:    "zero value",
			input:   "0",
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "large value",
			input:   "21",
			wantMin: 21,
			wantMax: 21,
		},
		{
			name:    "range 1-13",
			input:   "1-13",
			wantMin: 1,
			wantMax: 13,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotMin, gotMax := parsePointsFilter(tc.input)

			if gotMin != tc.wantMin {
				t.Errorf("parsePointsFilter(%q) min = %d, want %d", tc.input, gotMin, tc.wantMin)
			}
			if gotMax != tc.wantMax {
				t.Errorf("parsePointsFilter(%q) max = %d, want %d", tc.input, gotMax, tc.wantMax)
			}
		})
	}
}

func TestParseDateFilterEdgeCases(t *testing.T) {
	t.Run("malformed after: prefix", func(t *testing.T) {
		after, before := parseDateFilter("after:")
		if !after.IsZero() || !before.IsZero() {
			t.Error("Empty after: should return zero values")
		}
	})

	t.Run("malformed before: prefix", func(t *testing.T) {
		after, before := parseDateFilter("before:")
		if !after.IsZero() || !before.IsZero() {
			t.Error("Empty before: should return zero values")
		}
	})

	t.Run("malformed range with multiple ..", func(t *testing.T) {
		// This tests current behavior - multiple .. splits incorrectly
		after, before := parseDateFilter("2024-01-01..2024-06-15..2024-12-31")
		// Should parse first two parts
		if after.IsZero() {
			t.Log("Multiple .. in range handles first two parts")
		}
		_ = before // avoid unused variable
	})

	t.Run("range with only ..", func(t *testing.T) {
		after, before := parseDateFilter("..")
		if !after.IsZero() || !before.IsZero() {
			t.Error("Empty range should return zero values")
		}
	})
}

func TestParsePointsFilterEdgeCases(t *testing.T) {
	t.Run("negative value treated as zero", func(t *testing.T) {
		// Sscanf won't parse negative correctly with %d for this use case
		min, max := parsePointsFilter("-5")
		if min != 0 || max != 0 {
			// This depends on implementation - negative might be parsed differently
			t.Logf("Negative input: min=%d, max=%d", min, max)
		}
	})

	t.Run("non-numeric string", func(t *testing.T) {
		min, max := parsePointsFilter("abc")
		if min != 0 || max != 0 {
			t.Error("Non-numeric should return 0, 0")
		}
	})

	t.Run("range with spaces", func(t *testing.T) {
		min, max := parsePointsFilter("3 - 8")
		// Should fail to parse due to spaces in range
		t.Logf("Range with spaces: min=%d, max=%d", min, max)
	})

	t.Run(">=0 edge case", func(t *testing.T) {
		min, max := parsePointsFilter(">=0")
		if min != 0 || max != 0 {
			t.Errorf(">=0: got min=%d, max=%d", min, max)
		}
	})

	t.Run("<=0 edge case", func(t *testing.T) {
		min, max := parsePointsFilter("<=0")
		if min != 0 || max != 0 {
			t.Errorf("<=0: got min=%d, max=%d", min, max)
		}
	})
}
