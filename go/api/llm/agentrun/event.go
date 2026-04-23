package agentrun

import "time"

// EventKind mirrors the CHECK constraint on ap_task_event.kind:
// ('stdout','stderr','status','heartbeat','artifact','error').
type EventKind string

const (
	EventStdout    EventKind = "stdout"
	EventStderr    EventKind = "stderr"
	EventStatus    EventKind = "status"
	EventHeartbeat EventKind = "heartbeat"
	EventArtifact  EventKind = "artifact"
	EventError     EventKind = "error"
)

// Event is one line of progress emitted by a Runner.
// The worker pool persists each Event as an ap_task_event row and (in M2)
// fans it out over the workspace WebSocket.
type Event struct {
	Kind    EventKind
	Payload string
	At      time.Time
}
