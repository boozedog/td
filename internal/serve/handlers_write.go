package serve

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/git"
	"github.com/marcus/td/internal/models"
)

// ============================================================================
// POST /v1/issues — Create Issue
// ============================================================================

// handleCreateIssue creates a new issue from a JSON request body.
func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	var body IssueCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, ErrValidation, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Load configurable title length limits
	titleMin, titleMax := s.titleLengthLimits()

	// Validate
	if errs := ValidateIssueCreate(&body, titleMin, titleMax); len(errs) > 0 {
		WriteValidation(w, errs)
		return
	}

	// Normalize type and priority, apply defaults
	issueType := models.TypeTask
	if body.Type != "" {
		issueType = models.NormalizeType(body.Type)
	}

	issuePriority := models.PriorityP2
	if body.Priority != "" {
		issuePriority = models.NormalizePriority(body.Priority)
	}

	// If parent_id provided, verify it exists
	if body.ParentID != "" {
		normalizedParent := db.NormalizeIssueID(body.ParentID)
		_, err := s.db.GetIssue(normalizedParent)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				WriteError(w, ErrNotFound, fmt.Sprintf("parent issue not found: %s", body.ParentID), http.StatusNotFound)
			} else {
				slog.Error("lookup parent issue", "err", err, "parent_id", body.ParentID)
				WriteError(w, ErrInternal, "failed to verify parent issue", http.StatusInternalServerError)
			}
			return
		}
		body.ParentID = normalizedParent
	}

	// Parse nullable date fields
	var deferUntil *string
	if body.DeferUntil != "" {
		deferUntil = &body.DeferUntil
	}
	var dueDate *string
	if body.DueDate != "" {
		dueDate = &body.DueDate
	}

	// Build the issue model
	issue := &models.Issue{
		Title:          body.Title,
		Description:    body.Description,
		Type:           issueType,
		Priority:       issuePriority,
		Points:         body.Points,
		Labels:         body.Labels,
		ParentID:       body.ParentID,
		Acceptance:     body.Acceptance,
		Sprint:         body.Sprint,
		Minor:          body.Minor,
		CreatorSession: s.sessionID,
		DeferUntil:     deferUntil,
		DueDate:        dueDate,
	}

	// Capture current git branch
	gitState, _ := git.GetState()
	if gitState != nil {
		issue.CreatedBranch = gitState.Branch
	}

	// Create atomically with action log
	if err := s.db.CreateIssueLogged(issue, s.sessionID); err != nil {
		slog.Error("create issue", "err", err)
		WriteError(w, ErrInternal, "failed to create issue", http.StatusInternalServerError)
		return
	}

	// Record session action for bypass prevention
	if err := s.db.RecordSessionAction(issue.ID, s.sessionID, models.ActionSessionCreated); err != nil {
		slog.Warn("failed to record session history", "err", err)
	}

	dto := IssueToDTO(issue)
	WriteSuccess(w, map[string]interface{}{"issue": dto}, http.StatusCreated)
}

// ============================================================================
// PATCH /v1/issues/{id} — Update Issue
// ============================================================================

// handleUpdateIssue applies a partial update to an existing issue.
func (s *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	issueID := r.PathValue("id")
	if issueID == "" {
		WriteError(w, ErrValidation, "issue id is required", http.StatusBadRequest)
		return
	}

	var body IssueUpdateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, ErrValidation, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Load configurable title length limits
	titleMin, titleMax := s.titleLengthLimits()

	// Validate provided fields
	if errs := ValidateIssueUpdate(&body, titleMin, titleMax); len(errs) > 0 {
		WriteValidation(w, errs)
		return
	}

	// Fetch existing issue
	issue, err := s.db.GetIssue(issueID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, ErrNotFound, fmt.Sprintf("issue not found: %s", issueID), http.StatusNotFound)
		} else {
			slog.Error("get issue for update", "err", err, "id", issueID)
			WriteError(w, ErrInternal, "failed to fetch issue", http.StatusInternalServerError)
		}
		return
	}

	// Apply only non-nil fields
	if body.Title != nil {
		issue.Title = *body.Title
	}
	if body.Description != nil {
		issue.Description = *body.Description
	}
	if body.Acceptance != nil {
		issue.Acceptance = *body.Acceptance
	}
	if body.Type != nil && *body.Type != "" {
		issue.Type = models.NormalizeType(*body.Type)
	}
	if body.Priority != nil && *body.Priority != "" {
		issue.Priority = models.NormalizePriority(*body.Priority)
	}
	if body.Points != nil {
		issue.Points = *body.Points
	}
	if body.Labels != nil {
		issue.Labels = body.Labels
	}
	if body.ParentID != nil {
		parentID := *body.ParentID
		if parentID != "" {
			// Verify parent exists
			normalizedParent := db.NormalizeIssueID(parentID)
			_, err := s.db.GetIssue(normalizedParent)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					WriteError(w, ErrNotFound, fmt.Sprintf("parent issue not found: %s", parentID), http.StatusNotFound)
				} else {
					slog.Error("lookup parent issue", "err", err, "parent_id", parentID)
					WriteError(w, ErrInternal, "failed to verify parent issue", http.StatusInternalServerError)
				}
				return
			}
			issue.ParentID = normalizedParent
		} else {
			issue.ParentID = ""
		}
	}
	if body.Sprint != nil {
		issue.Sprint = *body.Sprint
	}
	if body.Minor != nil {
		issue.Minor = *body.Minor
	}
	if body.DeferUntil != nil {
		if *body.DeferUntil == "" {
			issue.DeferUntil = nil
		} else {
			issue.DeferUntil = body.DeferUntil
		}
	}
	if body.DueDate != nil {
		if *body.DueDate == "" {
			issue.DueDate = nil
		} else {
			issue.DueDate = body.DueDate
		}
	}

	// Update atomically with action log
	if err := s.db.UpdateIssueLogged(issue, s.sessionID, models.ActionUpdate); err != nil {
		slog.Error("update issue", "err", err, "id", issueID)
		WriteError(w, ErrInternal, "failed to update issue", http.StatusInternalServerError)
		return
	}

	dto := IssueToDTO(issue)
	WriteSuccess(w, map[string]interface{}{"issue": dto}, http.StatusOK)
}

// ============================================================================
// DELETE /v1/issues/{id} — Soft Delete
// ============================================================================

// handleDeleteIssue soft-deletes an issue.
func (s *Server) handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	issueID := r.PathValue("id")
	if issueID == "" {
		WriteError(w, ErrValidation, "issue id is required", http.StatusBadRequest)
		return
	}

	// Verify issue exists
	_, err := s.db.GetIssue(issueID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, ErrNotFound, fmt.Sprintf("issue not found: %s", issueID), http.StatusNotFound)
		} else {
			slog.Error("get issue for delete", "err", err, "id", issueID)
			WriteError(w, ErrInternal, "failed to fetch issue", http.StatusInternalServerError)
		}
		return
	}

	// Soft delete with action log
	if err := s.db.DeleteIssueLogged(issueID, s.sessionID); err != nil {
		slog.Error("delete issue", "err", err, "id", issueID)
		WriteError(w, ErrInternal, "failed to delete issue", http.StatusInternalServerError)
		return
	}

	WriteSuccess(w, map[string]interface{}{"deleted": true}, http.StatusOK)
}

// ============================================================================
// Helpers
// ============================================================================

// titleLengthLimits returns the configured or default title length limits.
func (s *Server) titleLengthLimits() (min, max int) {
	min, max, _ = config.GetTitleLengthLimits(s.baseDir)
	return min, max
}
