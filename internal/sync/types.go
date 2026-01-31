package sync

import "time"

// Event represents a single sync action from a device.
type Event struct {
	ClientActionID  int64
	DeviceID        string
	SessionID       string
	ActionType      string
	EntityType      string
	EntityID        string
	Payload         []byte // JSON
	ClientTimestamp time.Time
	ServerSeq       int64
}

// PushResult is the server response to a push request.
type PushResult struct {
	Accepted int
	Acks     []Ack
	Rejected []Rejection
}

// Ack confirms a client action was accepted with a server sequence number.
type Ack struct {
	ClientActionID int64
	ServerSeq      int64
}

// Rejection explains why a client action was refused.
type Rejection struct {
	ClientActionID int64
	Reason         string
}

// PullResult is the server response to a pull request.
type PullResult struct {
	Events        []Event
	LastServerSeq int64
	HasMore       bool
}

// ApplyResult summarises the outcome of applying a batch of events.
type ApplyResult struct {
	LastAppliedSeq int64
	Applied        int
	Failed         []FailedEvent
}

// FailedEvent records a single event that could not be applied.
type FailedEvent struct {
	ServerSeq int64
	Error     error
}

// EntityValidator returns true if the given entity type is allowed.
type EntityValidator func(entityType string) bool
