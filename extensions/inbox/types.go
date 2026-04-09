package inbox

import "time"

// Deliverer injects messages into the agent loop.
// *sdk.Extension satisfies this interface.
type Deliverer interface {
	SendMessage(content string)
	Steer(content string)
	Notify(msg string)
}

// Envelope is an inbound message from an external process.
type Envelope struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Text    string `json:"text"`
	Mode    string `json:"mode,omitzero"`
	Created string `json:"created,omitzero"`
	TTL     int    `json:"ttl,omitzero"`
	Source  string `json:"source,omitzero"`
}

// Ack is written after processing an envelope.
type Ack struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitzero"`
	Ts     string `json:"ts"`
}

// Heartbeat is written periodically to the registry.
type Heartbeat struct {
	PID       int    `json:"pid"`
	CWD       string `json:"cwd"`
	Started   string `json:"started"`
	Heartbeat string `json:"heartbeat"`
}

// Stats tracks delivery counts for the current session.
type Stats struct {
	Delivered  int       `json:"delivered"`
	Failed     int       `json:"failed"`
	Duplicates int       `json:"duplicates"`
	Expired    int       `json:"expired"`
	StartedAt  time.Time `json:"startedAt"`
}

const (
	DefaultScanInterval = 750 * time.Millisecond
	HeartbeatInterval   = 2 * time.Second
	MaxFileBytes        = 32 * 1024
	MaxTextRunes        = 8000
	DedupCap            = 1000
	AckMaxAge           = time.Hour
	PruneInterval       = time.Minute

	ModeQueue     = "queue"
	ModeInterrupt = "interrupt"
)
