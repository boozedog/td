package cmd

import (
	"testing"
)

// TestShowFormatFlagParsing tests that --format flag is defined and works
func TestShowFormatFlagParsing(t *testing.T) {
	// Test that --format flag exists
	if showCmd.Flags().Lookup("format") == nil {
		t.Error("Expected --format flag to be defined")
	}

	// Test that -f shorthand exists
	if showCmd.Flags().ShorthandLookup("f") == nil {
		t.Error("Expected -f shorthand to be defined for --format")
	}

	// Test that --format flag can be set
	if err := showCmd.Flags().Set("format", "json"); err != nil {
		t.Errorf("Failed to set --format flag: %v", err)
	}

	formatValue, err := showCmd.Flags().GetString("format")
	if err != nil {
		t.Errorf("Failed to get --format flag value: %v", err)
	}
	if formatValue != "json" {
		t.Errorf("Expected format value 'json', got %s", formatValue)
	}

	// Reset
	showCmd.Flags().Set("format", "")
}

// TestShowJSONFlagStillWorks tests that --json flag is still available
func TestShowJSONFlagStillWorks(t *testing.T) {
	// Test that --json flag exists
	if showCmd.Flags().Lookup("json") == nil {
		t.Error("Expected --json flag to still be defined")
	}

	// Test that --json flag can be set
	if err := showCmd.Flags().Set("json", "true"); err != nil {
		t.Errorf("Failed to set --json flag: %v", err)
	}

	jsonValue, err := showCmd.Flags().GetBool("json")
	if err != nil {
		t.Errorf("Failed to get --json flag value: %v", err)
	}
	if !jsonValue {
		t.Error("Expected json flag to be true")
	}

	// Reset
	showCmd.Flags().Set("json", "false")
}
