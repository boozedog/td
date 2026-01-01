package monitor

import (
	"sort"
	"strings"
	"time"

	"github.com/marcus/td/internal/config"
	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
	"github.com/marcus/td/internal/query"
	"github.com/marcus/td/internal/session"
)

// StatsData holds statistics for the stats modal
type StatsData struct {
	ExtendedStats *models.ExtendedStats
	Error         error
}

// StatsDataMsg carries fetched stats data
type StatsDataMsg struct {
	Data  *StatsData
	Error error
}

// FetchData retrieves all data needed for the monitor display
func FetchData(database *db.DB, sessionID string, startedAt time.Time, searchQuery string, includeClosed bool, sortMode SortMode) RefreshDataMsg {
	msg := RefreshDataMsg{
		Timestamp: time.Now(),
	}

	// Auto-detect current session for reviewable calculation
	// This allows the monitor to see reviewable issues when a new session starts
	currentSessionID := sessionID
	if sess, err := session.GetOrCreate(database.BaseDir()); err == nil {
		currentSessionID = sess.ID
	}

	// Get focused issue
	focusedID, _ := config.GetFocus(database.BaseDir())
	if focusedID != "" {
		if issue, err := database.GetIssue(focusedID); err == nil {
			msg.FocusedIssue = issue
		}
	}

	// Get in-progress issues
	inProgress, _ := database.ListIssues(db.ListIssuesOptions{
		Status: []models.Status{models.StatusInProgress},
		SortBy: "priority",
	})
	msg.InProgress = inProgress

	// Get activity feed
	msg.Activity = fetchActivity(database, 50)

	// Get task list (uses current session for reviewable calculation)
	msg.TaskList = fetchTaskList(database, currentSessionID, searchQuery, includeClosed, sortMode)

	// Get recent handoffs since monitor started
	msg.RecentHandoffs = fetchRecentHandoffs(database, startedAt)

	// Get active sessions (activity in last 5 minutes)
	msg.ActiveSessions = fetchActiveSessions(database)

	return msg
}

// fetchActivity combines logs, actions, and comments into a unified activity feed
func fetchActivity(database *db.DB, limit int) []ActivityItem {
	var items []ActivityItem

	// Fetch logs
	logs, _ := database.GetRecentLogsAll(limit)
	for _, log := range logs {
		items = append(items, ActivityItem{
			Timestamp: log.Timestamp,
			SessionID: log.SessionID,
			Type:      "log",
			IssueID:   log.IssueID,
			Message:   log.Message,
			LogType:   log.Type,
		})
	}

	// Fetch actions
	actions, _ := database.GetRecentActionsAll(limit)
	for _, action := range actions {
		items = append(items, ActivityItem{
			Timestamp: action.Timestamp,
			SessionID: action.SessionID,
			Type:      "action",
			IssueID:   action.EntityID,
			Message:   formatActionMessage(action),
			Action:    action.ActionType,
		})
	}

	// Fetch comments
	comments, _ := database.GetRecentCommentsAll(limit)
	for _, comment := range comments {
		items = append(items, ActivityItem{
			Timestamp: comment.CreatedAt,
			SessionID: comment.SessionID,
			Type:      "comment",
			IssueID:   comment.IssueID,
			Message:   comment.Text,
		})
	}

	// Sort by timestamp descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})

	// Limit total items
	if len(items) > limit {
		items = items[:limit]
	}

	return items
}

// isTDQQuery checks if the query uses TDQ syntax (operators, functions, etc.)
func isTDQQuery(q string) bool {
	// Check for TDQ operators and patterns
	tdqPatterns := []string{
		" = ", " != ", " ~ ", " !~ ",
		" < ", " > ", " <= ", " >= ",
		" AND ", " OR ", "NOT ",
		"has(", "is(", "any(", "blocks(", "blocked_by(", "descendant_of(",
		"log.", "comment.", "handoff.", "file.",
		"@me", "EMPTY",
		"sort:", // Sort prefix is considered TDQ
	}
	upper := strings.ToUpper(q)
	for _, pattern := range tdqPatterns {
		if strings.Contains(upper, strings.ToUpper(pattern)) {
			return true
		}
	}
	return false
}

// fetchTaskList retrieves categorized issues for the task list panel
func fetchTaskList(database *db.DB, sessionID string, searchQuery string, includeClosed bool, sortMode SortMode) TaskListData {
	var data TaskListData

	// Get default sort from SortMode (used for non-TDQ queries)
	sortBy, sortDesc := sortMode.ToDBOptions()

	// Helper to extract issues from ranked results
	extractIssues := func(results []db.SearchResult) []models.Issue {
		issues := make([]models.Issue, len(results))
		for i, r := range results {
			issues[i] = r.Issue
		}
		return issues
	}

	// Check if this is a TDQ query
	useTDQ := searchQuery != "" && isTDQQuery(searchQuery)

	if useTDQ {
		// Use TDQ to filter issues across all categories
		allIssues, err := query.Execute(database, searchQuery, sessionID, query.ExecuteOptions{})
		if err != nil {
			// Fall back to simple search on TDQ parse error
			useTDQ = false
		} else {
			// Categorize the TDQ results
			for _, issue := range allIssues {
				switch issue.Status {
				case models.StatusOpen:
					// Check if blocked by dependencies
					deps, _ := database.GetDependencies(issue.ID)
					isBlocked := false
					for _, depID := range deps {
						depIssue, err := database.GetIssue(depID)
						if err == nil && depIssue.Status != models.StatusClosed {
							isBlocked = true
							break
						}
					}
					if isBlocked {
						data.Blocked = append(data.Blocked, issue)
					} else {
						data.Ready = append(data.Ready, issue)
					}
				case models.StatusInProgress:
					// In-progress issues show in Ready (active work)
					data.Ready = append(data.Ready, issue)
				case models.StatusBlocked:
					data.Blocked = append(data.Blocked, issue)
				case models.StatusInReview:
					if issue.ImplementerSession != sessionID {
						data.Reviewable = append(data.Reviewable, issue)
					}
				case models.StatusClosed:
					if includeClosed {
						data.Closed = append(data.Closed, issue)
					}
				}
			}
			return data
		}
	}

	// Standard search (simple text or when TDQ fails)
	// Ready issues: open status, not blocked, sorted by priority
	var openIssues []models.Issue
	if searchQuery != "" && !useTDQ {
		results, _ := database.SearchIssuesRanked(searchQuery, db.ListIssuesOptions{
			Status: []models.Status{models.StatusOpen},
		})
		openIssues = extractIssues(results)
	} else if searchQuery == "" {
		openIssues, _ = database.ListIssues(db.ListIssuesOptions{
			Status:   []models.Status{models.StatusOpen},
			SortBy:   sortBy,
			SortDesc: sortDesc,
		})
	}

	// Separate open issues into ready vs blocked-by-dependency
	var blockedByDep []models.Issue
	for _, issue := range openIssues {
		deps, _ := database.GetDependencies(issue.ID)
		isBlocked := false
		for _, depID := range deps {
			depIssue, err := database.GetIssue(depID)
			if err == nil && depIssue.Status != models.StatusClosed {
				isBlocked = true
				break
			}
		}
		if isBlocked {
			blockedByDep = append(blockedByDep, issue)
		} else {
			data.Ready = append(data.Ready, issue)
		}
	}

	// Reviewable issues: in_review status, different implementer than current session
	if searchQuery != "" && !useTDQ {
		results, _ := database.SearchIssuesRanked(searchQuery, db.ListIssuesOptions{
			ReviewableBy: sessionID,
		})
		data.Reviewable = extractIssues(results)
	} else if searchQuery == "" {
		data.Reviewable, _ = database.ListIssues(db.ListIssuesOptions{
			ReviewableBy: sessionID,
			SortBy:       sortBy,
			SortDesc:     sortDesc,
		})
	}

	// Blocked issues: explicit blocked status + issues blocked by dependencies
	if searchQuery != "" && !useTDQ {
		results, _ := database.SearchIssuesRanked(searchQuery, db.ListIssuesOptions{
			Status: []models.Status{models.StatusBlocked},
		})
		data.Blocked = append(extractIssues(results), blockedByDep...)
	} else if searchQuery == "" {
		blocked, _ := database.ListIssues(db.ListIssuesOptions{
			Status:   []models.Status{models.StatusBlocked},
			SortBy:   sortBy,
			SortDesc: sortDesc,
		})
		data.Blocked = append(blocked, blockedByDep...)
	} else {
		data.Blocked = blockedByDep
	}

	// Closed issues (if toggle enabled)
	if includeClosed {
		if searchQuery != "" && !useTDQ {
			results, _ := database.SearchIssuesRanked(searchQuery, db.ListIssuesOptions{
				Status: []models.Status{models.StatusClosed},
			})
			data.Closed = extractIssues(results)
		} else if searchQuery == "" {
			data.Closed, _ = database.ListIssues(db.ListIssuesOptions{
				Status:   []models.Status{models.StatusClosed},
				SortBy:   sortBy,
				SortDesc: sortDesc,
			})
		}
	}

	return data
}

// fetchActiveSessions retrieves sessions with activity in the last 5 minutes
func fetchActiveSessions(database *db.DB) []string {
	since := time.Now().Add(-5 * time.Minute)
	sessions, err := database.GetActiveSessions(since)
	if err != nil {
		return nil
	}
	return sessions
}

// fetchRecentHandoffs retrieves handoffs since the given time
func fetchRecentHandoffs(database *db.DB, since time.Time) []RecentHandoff {
	var result []RecentHandoff

	handoffs, err := database.GetRecentHandoffs(10, since)
	if err != nil {
		return result
	}

	for _, h := range handoffs {
		result = append(result, RecentHandoff{
			IssueID:   h.IssueID,
			SessionID: h.SessionID,
			Timestamp: h.Timestamp,
		})
	}

	return result
}

// formatActionMessage creates a human-readable message for an action
func formatActionMessage(action models.ActionLog) string {
	switch action.ActionType {
	case models.ActionCreate:
		return "created issue"
	case models.ActionUpdate:
		return "updated issue"
	case models.ActionDelete:
		return "deleted issue"
	case models.ActionRestore:
		return "restored issue"
	case models.ActionStart:
		return "started work"
	case models.ActionReview:
		return "submitted for review"
	case models.ActionApprove:
		return "approved"
	case models.ActionReject:
		return "rejected"
	case models.ActionBlock:
		return "marked as blocked"
	case models.ActionUnblock:
		return "unblocked"
	case models.ActionClose:
		return "closed"
	case models.ActionReopen:
		return "reopened"
	case models.ActionAddDep:
		return "added dependency"
	case models.ActionRemoveDep:
		return "removed dependency"
	case models.ActionLinkFile:
		return "linked file"
	case models.ActionUnlinkFile:
		return "unlinked file"
	default:
		return string(action.ActionType)
	}
}

// FetchStats retrieves extended statistics for the stats modal
func FetchStats(database *db.DB) StatsDataMsg {
	stats, err := database.GetExtendedStats()
	if err != nil {
		return StatsDataMsg{
			Data:  &StatsData{Error: err},
			Error: err,
		}
	}
	return StatsDataMsg{
		Data: &StatsData{ExtendedStats: stats},
	}
}
