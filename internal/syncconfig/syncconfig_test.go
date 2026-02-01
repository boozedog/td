package syncconfig

import (
	"os"
	"testing"
)

func TestSnapshotThresholdDefault(t *testing.T) {
	// Clear any env var that might be set
	os.Unsetenv("TD_SYNC_SNAPSHOT_THRESHOLD")

	threshold := GetSnapshotThreshold()
	if threshold != 100 {
		t.Fatalf("default threshold: got %d, want 100", threshold)
	}
}

func TestSnapshotThresholdEnvVar(t *testing.T) {
	t.Setenv("TD_SYNC_SNAPSHOT_THRESHOLD", "500")

	threshold := GetSnapshotThreshold()
	if threshold != 500 {
		t.Fatalf("env threshold: got %d, want 500", threshold)
	}
}

func TestSnapshotThresholdEnvVarInvalid(t *testing.T) {
	t.Setenv("TD_SYNC_SNAPSHOT_THRESHOLD", "not-a-number")

	// Invalid env should fall through to default
	threshold := GetSnapshotThreshold()
	if threshold != 100 {
		t.Fatalf("invalid env threshold: got %d, want 100 (default)", threshold)
	}
}

func TestSnapshotThresholdEnvVarZero(t *testing.T) {
	t.Setenv("TD_SYNC_SNAPSHOT_THRESHOLD", "0")

	// Zero should fall through to default (n > 0 check)
	threshold := GetSnapshotThreshold()
	if threshold != 100 {
		t.Fatalf("zero env threshold: got %d, want 100 (default)", threshold)
	}
}

func TestSnapshotThresholdEnvVarNegative(t *testing.T) {
	t.Setenv("TD_SYNC_SNAPSHOT_THRESHOLD", "-5")

	// Negative should fall through to default
	threshold := GetSnapshotThreshold()
	if threshold != 100 {
		t.Fatalf("negative env threshold: got %d, want 100 (default)", threshold)
	}
}

func TestSnapshotThresholdEnvOverridesConfig(t *testing.T) {
	// Even if config has a value, env should take precedence
	t.Setenv("TD_SYNC_SNAPSHOT_THRESHOLD", "42")

	threshold := GetSnapshotThreshold()
	if threshold != 42 {
		t.Fatalf("env override: got %d, want 42", threshold)
	}
}
