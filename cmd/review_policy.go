package cmd

import (
	"fmt"

	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/features"
	"github.com/marcus/td/internal/models"
)

type approveEligibility struct {
	Allowed          bool
	CreatorException bool
	RequiresReason   bool
	RejectionMessage string
}

func balancedReviewPolicyEnabled(baseDir string) bool {
	return features.IsEnabled(baseDir, features.BalancedReviewPolicy.Name)
}

func reviewableByOptions(baseDir, sessionID string) db.ListIssuesOptions {
	return db.ListIssuesOptions{
		ReviewableBy:         sessionID,
		BalancedReviewPolicy: balancedReviewPolicyEnabled(baseDir),
	}
}

func evaluateApproveEligibility(issue *models.Issue, sessionID string, wasInvolved, wasImplementationInvolved, balancedPolicy bool) approveEligibility {
	if issue == nil {
		return approveEligibility{
			Allowed:          false,
			RejectionMessage: "cannot approve: issue not found",
		}
	}

	// Minor tasks intentionally bypass all self-review restrictions.
	if issue.Minor {
		return approveEligibility{Allowed: true}
	}

	isCreator := issue.CreatorSession != "" && issue.CreatorSession == sessionID
	isImplementer := issue.ImplementerSession != "" && issue.ImplementerSession == sessionID

	if !balancedPolicy {
		if wasInvolved || isCreator || isImplementer {
			return approveEligibility{
				Allowed:          false,
				RejectionMessage: fmt.Sprintf("cannot approve: you were involved with %s (created, started, or previously worked on)", issue.ID),
			}
		}
		return approveEligibility{Allowed: true}
	}

	// Balanced policy still hard-blocks implementation self-approval.
	if isImplementer || wasImplementationInvolved {
		return approveEligibility{
			Allowed:          false,
			RejectionMessage: fmt.Sprintf("cannot approve: you were involved with implementation of %s", issue.ID),
		}
	}

	// Creator-only exception: creator can approve if a different session implemented.
	hasDifferentImplementer := issue.ImplementerSession != "" && issue.ImplementerSession != sessionID
	if isCreator && hasDifferentImplementer {
		return approveEligibility{
			Allowed:          true,
			CreatorException: true,
			RequiresReason:   true,
		}
	}

	// Non-creator sessions still require zero prior involvement.
	if wasInvolved {
		return approveEligibility{
			Allowed:          false,
			RejectionMessage: fmt.Sprintf("cannot approve: you were involved with %s (created, started, or previously worked on)", issue.ID),
		}
	}

	return approveEligibility{Allowed: true}
}
