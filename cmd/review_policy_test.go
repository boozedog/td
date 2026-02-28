package cmd

import (
	"testing"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/features"
	"github.com/marcus/td/internal/models"
)

func TestEvaluateApproveEligibility(t *testing.T) {
	issue := &models.Issue{
		ID:                 "td-test",
		CreatorSession:     "ses_creator",
		ImplementerSession: "ses_impl",
		Status:             models.StatusInReview,
	}

	tests := []struct {
		name                      string
		sessionID                 string
		wasInvolved               bool
		wasImplementationInvolved bool
		balanced                  bool
		minor                     bool
		noImplementer             bool
		wantAllowed               bool
		wantCreatorException      bool
		wantRequiresReason        bool
	}{
		{
			name:                      "strict blocks creator-only approval",
			sessionID:                 "ses_creator",
			wasInvolved:               true,
			wasImplementationInvolved: false,
			balanced:                  false,
			wantAllowed:               false,
		},
		{
			name:                      "balanced allows creator-only approval",
			sessionID:                 "ses_creator",
			wasInvolved:               true,
			wasImplementationInvolved: false,
			balanced:                  true,
			wantAllowed:               true,
			wantCreatorException:      true,
			wantRequiresReason:        true,
		},
		{
			name:                      "balanced blocks creator who implemented",
			sessionID:                 "ses_creator",
			wasInvolved:               true,
			wasImplementationInvolved: true,
			balanced:                  true,
			wantAllowed:               false,
		},
		{
			name:                      "balanced blocks implementer",
			sessionID:                 "ses_impl",
			wasInvolved:               true,
			wasImplementationInvolved: true,
			balanced:                  true,
			wantAllowed:               false,
		},
		{
			name:                      "balanced allows unrelated reviewer",
			sessionID:                 "ses_reviewer",
			wasInvolved:               false,
			wasImplementationInvolved: false,
			balanced:                  true,
			wantAllowed:               true,
		},
		{
			name:                      "balanced blocks involved non-creator",
			sessionID:                 "ses_reviewer",
			wasInvolved:               true,
			wasImplementationInvolved: false,
			balanced:                  true,
			wantAllowed:               false,
		},
		{
			name:                      "minor always allowed",
			sessionID:                 "ses_impl",
			wasInvolved:               true,
			wasImplementationInvolved: true,
			balanced:                  false,
			minor:                     true,
			wantAllowed:               true,
		},
		{
			name:                      "balanced blocks creator when no implementer set",
			sessionID:                 "ses_creator",
			wasInvolved:               true,
			wasImplementationInvolved: false,
			balanced:                  true,
			noImplementer:             true,
			wantAllowed:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := *issue
			i.Minor = tt.minor
			if tt.noImplementer {
				i.ImplementerSession = ""
			}
			got := evaluateApproveEligibility(&i, tt.sessionID, tt.wasInvolved, tt.wasImplementationInvolved, tt.balanced)
			if got.Allowed != tt.wantAllowed {
				t.Fatalf("Allowed=%v, want %v", got.Allowed, tt.wantAllowed)
			}
			if got.CreatorException != tt.wantCreatorException {
				t.Fatalf("CreatorException=%v, want %v", got.CreatorException, tt.wantCreatorException)
			}
			if got.RequiresReason != tt.wantRequiresReason {
				t.Fatalf("RequiresReason=%v, want %v", got.RequiresReason, tt.wantRequiresReason)
			}
		})
	}
}

func TestReviewableByOptions_UsesBalancedReviewPolicyFlag(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "ses_test"

	// Default is ON.
	opts := reviewableByOptions(baseDir, sessionID)
	if !opts.BalancedReviewPolicy {
		t.Fatalf("BalancedReviewPolicy should default to true")
	}

	// Local config override OFF.
	if err := config.SetFeatureFlag(baseDir, features.BalancedReviewPolicy.Name, false); err != nil {
		t.Fatalf("SetFeatureFlag failed: %v", err)
	}
	opts = reviewableByOptions(baseDir, sessionID)
	if opts.BalancedReviewPolicy {
		t.Fatalf("BalancedReviewPolicy should be false when overridden in config")
	}

	// Env override ON should win over config OFF.
	t.Setenv("TD_FEATURE_BALANCED_REVIEW_POLICY", "true")
	opts = reviewableByOptions(baseDir, sessionID)
	if !opts.BalancedReviewPolicy {
		t.Fatalf("BalancedReviewPolicy should be true when env override is set")
	}
}
