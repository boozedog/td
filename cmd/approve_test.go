package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestApproveCmdHasCommentFlag(t *testing.T) {
	f := approveCmd.Flags().Lookup("comment")
	if f == nil {
		t.Fatalf("expected approveCmd to have --comment flag")
	}
}

func TestApprovalReasonPrecedence(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("reason", "", "")
	cmd.Flags().String("message", "", "")
	cmd.Flags().String("comment", "", "")

	// Lowest precedence: --comment
	if err := cmd.Flags().Set("comment", "c"); err != nil {
		t.Fatalf("set comment: %v", err)
	}
	if got := approvalReason(cmd); got != "c" {
		t.Fatalf("comment only: got %q, want %q", got, "c")
	}

	// Middle precedence: --message overrides comment
	if err := cmd.Flags().Set("message", "m"); err != nil {
		t.Fatalf("set message: %v", err)
	}
	if got := approvalReason(cmd); got != "m" {
		t.Fatalf("message+comment: got %q, want %q", got, "m")
	}

	// Highest precedence: --reason overrides message
	if err := cmd.Flags().Set("reason", "r"); err != nil {
		t.Fatalf("set reason: %v", err)
	}
	if got := approvalReason(cmd); got != "r" {
		t.Fatalf("reason+message+comment: got %q, want %q", got, "r")
	}
}

func TestApprovalReasonEmpty(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("reason", "", "")
	cmd.Flags().String("message", "", "")
	cmd.Flags().String("comment", "", "")

	if got := approvalReason(cmd); got != "" {
		t.Fatalf("empty: got %q, want empty", got)
	}
}
