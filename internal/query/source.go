package query

import (
	"github.com/marcus/td/internal/db"
	"github.com/marcus/td/internal/models"
)

// QuerySource abstracts the database operations needed by the TDQ query engine.
// Both db.DB (client-side) and snapshot adapters (server-side) implement this.
type QuerySource interface {
	ListIssues(opts db.ListIssuesOptions) ([]models.Issue, error)
	GetIssue(id string) (*models.Issue, error)
	GetLogs(issueID string, limit int) ([]models.Log, error)
	GetComments(issueID string) ([]models.Comment, error)
	GetLatestHandoff(issueID string) (*models.Handoff, error)
	GetLinkedFiles(issueID string) ([]models.IssueFile, error)
	GetDependencies(issueID string) ([]string, error)
	GetRejectedInProgressIssueIDs() (map[string]bool, error)
	GetIssuesWithOpenDeps() (map[string]bool, error)
}
