package cmd

import (
	"testing"
	"time"
)

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"just now", 30 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1m ago"},
		{"5 minutes", 5 * time.Minute, "5m ago"},
		{"59 minutes", 59 * time.Minute, "59m ago"},
		{"1 hour", 1 * time.Hour, "1h ago"},
		{"5 hours", 5 * time.Hour, "5h ago"},
		{"23 hours", 23 * time.Hour, "23h ago"},
		{"1 day", 24 * time.Hour, "1d ago"},
		{"3 days", 72 * time.Hour, "3d ago"},
		{"7 days", 168 * time.Hour, "7d ago"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			timestamp := time.Now().Add(-tc.duration)
			got := formatTimeAgo(timestamp)
			if got != tc.want {
				t.Errorf("formatTimeAgo() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatTimeAgoBoundaries(t *testing.T) {
	// Test boundary between just now and minutes
	t.Run("59 seconds is just now", func(t *testing.T) {
		timestamp := time.Now().Add(-59 * time.Second)
		got := formatTimeAgo(timestamp)
		if got != "just now" {
			t.Errorf("formatTimeAgo(59s) = %q, want %q", got, "just now")
		}
	})

	t.Run("61 seconds is 1m ago", func(t *testing.T) {
		timestamp := time.Now().Add(-61 * time.Second)
		got := formatTimeAgo(timestamp)
		if got != "1m ago" {
			t.Errorf("formatTimeAgo(61s) = %q, want %q", got, "1m ago")
		}
	})

	// Test boundary between minutes and hours
	t.Run("59 minutes is minutes", func(t *testing.T) {
		timestamp := time.Now().Add(-59 * time.Minute)
		got := formatTimeAgo(timestamp)
		if got != "59m ago" {
			t.Errorf("formatTimeAgo(59m) = %q, want %q", got, "59m ago")
		}
	})

	t.Run("61 minutes is hours", func(t *testing.T) {
		timestamp := time.Now().Add(-61 * time.Minute)
		got := formatTimeAgo(timestamp)
		if got != "1h ago" {
			t.Errorf("formatTimeAgo(61m) = %q, want %q", got, "1h ago")
		}
	})

	// Test boundary between hours and days
	t.Run("23 hours is hours", func(t *testing.T) {
		timestamp := time.Now().Add(-23 * time.Hour)
		got := formatTimeAgo(timestamp)
		if got != "23h ago" {
			t.Errorf("formatTimeAgo(23h) = %q, want %q", got, "23h ago")
		}
	})

	t.Run("25 hours is days", func(t *testing.T) {
		timestamp := time.Now().Add(-25 * time.Hour)
		got := formatTimeAgo(timestamp)
		if got != "1d ago" {
			t.Errorf("formatTimeAgo(25h) = %q, want %q", got, "1d ago")
		}
	})
}
