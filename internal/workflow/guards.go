package workflow

import (
	"github.com/marcus/td/internal/models"
)

// BlockedGuard warns when starting a blocked issue without --force
type BlockedGuard struct{}

func (g *BlockedGuard) Name() string {
	return "BlockedGuard"
}

func (g *BlockedGuard) Check(ctx *TransitionContext) GuardResult {
	// Allow if force flag is set
	if ctx.Force {
		return GuardResult{Passed: true}
	}

	// Allow if not transitioning from blocked
	if ctx.FromStatus != models.StatusBlocked {
		return GuardResult{Passed: true}
	}

	// Fail - starting blocked issue without --force
	return GuardResult{
		Passed:  false,
		Message: "cannot start blocked issue without --force",
	}
}

// DifferentReviewerGuard ensures approvals come from different session than implementer
type DifferentReviewerGuard struct{}

func (g *DifferentReviewerGuard) Name() string {
	return "DifferentReviewerGuard"
}

func (g *DifferentReviewerGuard) Check(ctx *TransitionContext) GuardResult {
	// Allow if minor task (self-approval permitted)
	if ctx.Minor || ctx.Issue.Minor {
		return GuardResult{Passed: true}
	}

	// Allow if admin bypass context
	if ctx.Context == ContextAdmin {
		return GuardResult{Passed: true}
	}

	// Allow if session was not involved
	if !ctx.WasInvolved {
		return GuardResult{Passed: true}
	}

	// Check if same session as implementer
	if ctx.Issue.ImplementerSession != "" && ctx.Issue.ImplementerSession == ctx.SessionID {
		return GuardResult{
			Passed:  false,
			Message: "cannot approve your own implementation",
		}
	}

	// Check if session was involved (via WasInvolved flag)
	// The caller sets this based on database.WasSessionInvolved() check
	if ctx.WasInvolved {
		return GuardResult{
			Passed:  false,
			Message: "cannot approve issue you were involved with",
		}
	}

	return GuardResult{Passed: true}
}

// EpicChildrenGuard warns when closing epic with open children
type EpicChildrenGuard struct {
	// OpenChildCount is set by caller before validation
	OpenChildCount int
}

func (g *EpicChildrenGuard) Name() string {
	return "EpicChildrenGuard"
}

func (g *EpicChildrenGuard) Check(ctx *TransitionContext) GuardResult {
	// Only applies to closing transitions
	if ctx.ToStatus != models.StatusClosed {
		return GuardResult{Passed: true}
	}

	// Only applies to epics
	if ctx.Issue.Type != models.TypeEpic {
		return GuardResult{Passed: true}
	}

	// Check if there are open children
	if g.OpenChildCount > 0 {
		return GuardResult{
			Passed:  false,
			Message: "epic has open children",
		}
	}

	return GuardResult{Passed: true}
}

// SelfCloseGuard prevents closing issues you implemented without exception
type SelfCloseGuard struct {
	// SelfCloseException is the reason provided for self-close bypass
	SelfCloseException string
}

func (g *SelfCloseGuard) Name() string {
	return "SelfCloseGuard"
}

func (g *SelfCloseGuard) Check(ctx *TransitionContext) GuardResult {
	// Allow if not closing
	if ctx.ToStatus != models.StatusClosed {
		return GuardResult{Passed: true}
	}

	// Allow if minor task
	if ctx.Minor || ctx.Issue.Minor {
		return GuardResult{Passed: true}
	}

	// Allow if self-close exception provided
	if g.SelfCloseException != "" {
		return GuardResult{Passed: true}
	}

	// Allow if not involved
	if !ctx.WasInvolved {
		return GuardResult{Passed: true}
	}

	// Check if session is the implementer
	if ctx.Issue.ImplementerSession != "" && ctx.Issue.ImplementerSession == ctx.SessionID {
		return GuardResult{
			Passed:  false,
			Message: "cannot close own implementation without --self-close-exception",
		}
	}

	// General involvement check
	if ctx.WasInvolved {
		return GuardResult{
			Passed:  false,
			Message: "cannot close issue you were involved with",
		}
	}

	return GuardResult{Passed: true}
}

// InProgressRequiredGuard ensures issue is in progress before review
type InProgressRequiredGuard struct{}

func (g *InProgressRequiredGuard) Name() string {
	return "InProgressRequiredGuard"
}

func (g *InProgressRequiredGuard) Check(ctx *TransitionContext) GuardResult {
	// Only applies when going to review
	if ctx.ToStatus != models.StatusInReview {
		return GuardResult{Passed: true}
	}

	// Allow from in_progress
	if ctx.FromStatus == models.StatusInProgress {
		return GuardResult{Passed: true}
	}

	// Allow from open (direct submission)
	if ctx.FromStatus == models.StatusOpen {
		return GuardResult{Passed: true}
	}

	return GuardResult{
		Passed:  false,
		Message: "can only submit for review from open or in_progress status",
	}
}
