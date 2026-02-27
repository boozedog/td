package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateInstanceID(t *testing.T) {
	id, err := GenerateInstanceID()
	if err != nil {
		t.Fatalf("GenerateInstanceID() error: %v", err)
	}

	if !strings.HasPrefix(id, "srv_") {
		t.Errorf("expected prefix 'srv_', got %q", id)
	}

	// srv_ (4 chars) + 6 hex chars = 10 total
	if len(id) != 10 {
		t.Errorf("expected length 10, got %d (%q)", len(id), id)
	}

	// Generate another and verify uniqueness
	id2, err := GenerateInstanceID()
	if err != nil {
		t.Fatalf("GenerateInstanceID() second call error: %v", err)
	}
	if id == id2 {
		t.Errorf("expected unique IDs, got %q twice", id)
	}
}

func TestWriteReadPortFile(t *testing.T) {
	baseDir := t.TempDir()
	todosDir := filepath.Join(baseDir, ".todos")
	if err := os.MkdirAll(todosDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second).UTC()
	info := &PortInfo{
		Port:       54321,
		PID:        os.Getpid(),
		StartedAt:  now,
		InstanceID: "srv_abc123",
	}

	// Write
	if err := WritePortFile(baseDir, info); err != nil {
		t.Fatalf("WritePortFile() error: %v", err)
	}

	// Verify file exists
	pfPath := filepath.Join(todosDir, portFileName)
	if _, err := os.Stat(pfPath); err != nil {
		t.Fatalf("port file not created: %v", err)
	}

	// Read back
	got, err := ReadPortFile(baseDir)
	if err != nil {
		t.Fatalf("ReadPortFile() error: %v", err)
	}

	if got.Port != info.Port {
		t.Errorf("Port = %d, want %d", got.Port, info.Port)
	}
	if got.PID != info.PID {
		t.Errorf("PID = %d, want %d", got.PID, info.PID)
	}
	if got.InstanceID != info.InstanceID {
		t.Errorf("InstanceID = %q, want %q", got.InstanceID, info.InstanceID)
	}
	if !got.StartedAt.Equal(info.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, info.StartedAt)
	}
}

func TestWritePortFileJSON(t *testing.T) {
	baseDir := t.TempDir()
	todosDir := filepath.Join(baseDir, ".todos")
	if err := os.MkdirAll(todosDir, 0755); err != nil {
		t.Fatal(err)
	}

	info := &PortInfo{
		Port:       54321,
		PID:        91234,
		StartedAt:  time.Date(2026, 2, 27, 5, 10, 11, 0, time.UTC),
		InstanceID: "srv_8f3b2c",
	}

	if err := WritePortFile(baseDir, info); err != nil {
		t.Fatalf("WritePortFile() error: %v", err)
	}

	// Read raw JSON and verify format
	data, err := os.ReadFile(filepath.Join(todosDir, portFileName))
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if raw["port"].(float64) != 54321 {
		t.Errorf("JSON port = %v, want 54321", raw["port"])
	}
	if raw["pid"].(float64) != 91234 {
		t.Errorf("JSON pid = %v, want 91234", raw["pid"])
	}
	if raw["instance_id"].(string) != "srv_8f3b2c" {
		t.Errorf("JSON instance_id = %v, want srv_8f3b2c", raw["instance_id"])
	}
}

func TestDeletePortFile(t *testing.T) {
	baseDir := t.TempDir()
	todosDir := filepath.Join(baseDir, ".todos")
	if err := os.MkdirAll(todosDir, 0755); err != nil {
		t.Fatal(err)
	}

	info := &PortInfo{
		Port:       12345,
		PID:        os.Getpid(),
		StartedAt:  time.Now().UTC(),
		InstanceID: "srv_aabbcc",
	}

	if err := WritePortFile(baseDir, info); err != nil {
		t.Fatalf("WritePortFile() error: %v", err)
	}

	// Delete
	if err := DeletePortFile(baseDir); err != nil {
		t.Fatalf("DeletePortFile() error: %v", err)
	}

	// Verify gone
	pfPath := filepath.Join(todosDir, portFileName)
	if _, err := os.Stat(pfPath); !os.IsNotExist(err) {
		t.Errorf("port file still exists after delete")
	}

	// Delete again should be a no-op (not an error)
	if err := DeletePortFile(baseDir); err != nil {
		t.Errorf("DeletePortFile() on missing file: %v", err)
	}
}

func TestReadPortFileMissing(t *testing.T) {
	baseDir := t.TempDir()
	_, err := ReadPortFile(baseDir)
	if err == nil {
		t.Fatal("expected error for missing port file")
	}
}

func TestReadPortFileInvalidJSON(t *testing.T) {
	baseDir := t.TempDir()
	todosDir := filepath.Join(baseDir, ".todos")
	if err := os.MkdirAll(todosDir, 0755); err != nil {
		t.Fatal(err)
	}

	pfPath := filepath.Join(todosDir, portFileName)
	if err := os.WriteFile(pfPath, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadPortFile(baseDir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse port file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadPortFileMissingFields(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "missing port",
			data: `{"pid": 123, "instance_id": "srv_abc123"}`,
			want: "port",
		},
		{
			name: "missing pid",
			data: `{"port": 8080, "instance_id": "srv_abc123"}`,
			want: "pid",
		},
		{
			name: "missing instance_id",
			data: `{"port": 8080, "pid": 123}`,
			want: "instance_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			todosDir := filepath.Join(baseDir, ".todos")
			if err := os.MkdirAll(todosDir, 0755); err != nil {
				t.Fatal(err)
			}

			pfPath := filepath.Join(todosDir, portFileName)
			if err := os.WriteFile(pfPath, []byte(tt.data), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := ReadPortFile(baseDir)
			if err == nil {
				t.Fatal("expected error for missing field")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should mention %q", err.Error(), tt.want)
			}
		})
	}
}

func TestIsPortFileStaleDeadPID(t *testing.T) {
	// Use a PID that is extremely unlikely to be alive.
	// PID 2^30 is outside typical PID ranges on most systems.
	info := &PortInfo{
		Port:       19999,
		PID:        1<<30 + 7,
		StartedAt:  time.Now().UTC(),
		InstanceID: "srv_dead01",
	}

	if !IsPortFileStale(info) {
		t.Error("expected stale for dead PID")
	}
}

func TestIsServerHealthyNoServer(t *testing.T) {
	// Port 1 is privileged and almost certainly not running a health server
	if IsServerHealthy(1) {
		t.Error("expected IsServerHealthy(1) = false")
	}
}

func TestPortFileLockFileCreated(t *testing.T) {
	baseDir := t.TempDir()
	todosDir := filepath.Join(baseDir, ".todos")
	if err := os.MkdirAll(todosDir, 0755); err != nil {
		t.Fatal(err)
	}

	info := &PortInfo{
		Port:       33333,
		PID:        os.Getpid(),
		StartedAt:  time.Now().UTC(),
		InstanceID: "srv_lock01",
	}

	if err := WritePortFile(baseDir, info); err != nil {
		t.Fatalf("WritePortFile() error: %v", err)
	}

	// Lock file should have been created
	lockPath := filepath.Join(todosDir, portLockFileName)
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file not created: %v", err)
	}
}

func TestWriteReadDeleteRoundtrip(t *testing.T) {
	baseDir := t.TempDir()
	todosDir := filepath.Join(baseDir, ".todos")
	if err := os.MkdirAll(todosDir, 0755); err != nil {
		t.Fatal(err)
	}

	info := &PortInfo{
		Port:       44444,
		PID:        os.Getpid(),
		StartedAt:  time.Now().Truncate(time.Second).UTC(),
		InstanceID: "srv_round1",
	}

	// Write
	if err := WritePortFile(baseDir, info); err != nil {
		t.Fatal(err)
	}

	// Read
	got, err := ReadPortFile(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != info.Port || got.PID != info.PID || got.InstanceID != info.InstanceID {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, info)
	}

	// Delete
	if err := DeletePortFile(baseDir); err != nil {
		t.Fatal(err)
	}

	// Read should fail
	_, err = ReadPortFile(baseDir)
	if err == nil {
		t.Error("expected error after delete")
	}
}
