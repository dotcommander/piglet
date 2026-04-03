package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDeliverer struct {
	mu            sync.Mutex
	sent          []string
	steered       []string
	notifications []string
}

func (m *mockDeliverer) SendMessage(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, content)
}

func (m *mockDeliverer) Steer(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steered = append(m.steered, content)
}

func (m *mockDeliverer) Notify(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, msg)
}

func newTestScanner(t *testing.T, d *mockDeliverer) *Scanner {
	t.Helper()
	dir := t.TempDir()
	s := New(dir, "/tmp", 99999, d)
	_ = os.MkdirAll(s.processDir(), 0750)
	_ = os.MkdirAll(s.acksDir(), 0750)
	_ = os.MkdirAll(s.registryDir(), 0750)
	return s
}

func writeEnvelope(t *testing.T, dir string, name string, env Envelope) {
	t.Helper()
	data, err := json.Marshal(env)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0640))
}

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		env    Envelope
		ok     bool
		reason string
	}{
		{
			name: "good",
			env:  Envelope{Version: 1, ID: "abc", Text: "hello"},
			ok:   true,
		},
		{
			name: "good_with_mode",
			env:  Envelope{Version: 1, ID: "abc", Text: "hello", Mode: "interrupt"},
			ok:   true,
		},
		{
			name:   "missing_id",
			env:    Envelope{Version: 1, Text: "hello"},
			reason: "missing_id",
		},
		{
			name:   "missing_text",
			env:    Envelope{Version: 1, ID: "abc"},
			reason: "missing_text",
		},
		{
			name:   "text_too_long",
			env:    Envelope{Version: 1, ID: "abc", Text: string(make([]rune, MaxTextRunes+1))},
			reason: "text_too_long",
		},
		{
			name:   "invalid_mode",
			env:    Envelope{Version: 1, ID: "abc", Text: "hi", Mode: "blast"},
			reason: "invalid_mode",
		},
		{
			name:   "bad_version",
			env:    Envelope{Version: 2, ID: "abc", Text: "hi"},
			reason: "unsupported_version",
		},
		{
			name:   "zero_version",
			env:    Envelope{ID: "abc", Text: "hi"},
			reason: "unsupported_version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ok, reason := Validate(&tt.env)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.reason, reason)
		})
	}
}

func TestScanDelivers(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	writeEnvelope(t, s.processDir(), "msg-1.json", Envelope{
		Version: 1, ID: "msg-1", Text: "hello from test", Source: "test",
	})

	s.scan()

	d.mu.Lock()
	defer d.mu.Unlock()
	require.Len(t, d.sent, 1)
	assert.Equal(t, "hello from test", d.sent[0])
	assert.Contains(t, d.notifications[0], "test")

	ackData, err := os.ReadFile(filepath.Join(s.acksDir(), "msg-1.json"))
	require.NoError(t, err)
	var ack Ack
	require.NoError(t, json.Unmarshal(ackData, &ack))
	assert.Equal(t, "delivered", ack.Status)

	_, err = os.Stat(filepath.Join(s.processDir(), "msg-1.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestScanInterrupt(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	writeEnvelope(t, s.processDir(), "int-1.json", Envelope{
		Version: 1, ID: "int-1", Text: "urgent", Mode: "interrupt",
	})

	s.scan()

	d.mu.Lock()
	defer d.mu.Unlock()
	require.Len(t, d.steered, 1)
	assert.Equal(t, "urgent", d.steered[0])
	assert.Empty(t, d.sent)
}

func TestScanDedup(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	env := Envelope{Version: 1, ID: "dup-1", Text: "once"}

	writeEnvelope(t, s.processDir(), "dup-1.json", env)
	s.scan()

	writeEnvelope(t, s.processDir(), "dup-1b.json", env)
	s.scan()

	d.mu.Lock()
	defer d.mu.Unlock()
	assert.Len(t, d.sent, 1, "should deliver only once")

	stats := s.Stats()
	assert.Equal(t, 1, stats.Duplicates)

	// Ack file reflects last write — the duplicate ack overwrites the delivered one.
	ackData, err := os.ReadFile(filepath.Join(s.acksDir(), "dup-1.json"))
	require.NoError(t, err)
	var ack Ack
	require.NoError(t, json.Unmarshal(ackData, &ack))
	assert.Equal(t, "duplicate", ack.Status)
}

func TestScanExpired(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	writeEnvelope(t, s.processDir(), "exp-1.json", Envelope{
		Version: 1,
		ID:      "exp-1",
		Text:    "old news",
		TTL:     1,
		Created: time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339),
	})

	s.scan()

	d.mu.Lock()
	defer d.mu.Unlock()
	assert.Empty(t, d.sent)

	stats := s.Stats()
	assert.Equal(t, 1, stats.Expired)

	ackData, err := os.ReadFile(filepath.Join(s.acksDir(), "exp-1.json"))
	require.NoError(t, err)
	var ack Ack
	require.NoError(t, json.Unmarshal(ackData, &ack))
	assert.Equal(t, "expired", ack.Reason)
}

func TestScanSymlink(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	realFile := filepath.Join(s.inboxDir, "real.json")
	writeEnvelope(t, s.inboxDir, "real.json", Envelope{
		Version: 1, ID: "sym-1", Text: "via symlink",
	})

	linkPath := filepath.Join(s.processDir(), "sym-1.json")
	require.NoError(t, os.Symlink(realFile, linkPath))

	s.scan()

	d.mu.Lock()
	defer d.mu.Unlock()
	assert.Empty(t, d.sent, "symlinks should be rejected")
}

func TestScanInvalidJSON(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	require.NoError(t, os.WriteFile(
		filepath.Join(s.processDir(), "bad.json"),
		[]byte("not json{{{"),
		0640,
	))

	s.scan()

	d.mu.Lock()
	defer d.mu.Unlock()
	assert.Empty(t, d.sent)
	assert.Equal(t, 1, s.Stats().Failed)

	_, err := os.Stat(filepath.Join(s.processDir(), "bad.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestHeartbeat(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	s.writeHeartbeat()

	data, err := os.ReadFile(filepath.Join(s.registryDir(), "99999.json"))
	require.NoError(t, err)
	var hb Heartbeat
	require.NoError(t, json.Unmarshal(data, &hb))
	assert.Equal(t, 99999, hb.PID)
	assert.Equal(t, "/tmp", hb.CWD)
	assert.NotEmpty(t, hb.Heartbeat)
	assert.NotEmpty(t, hb.Started)
}

func TestStopCleansHeartbeat(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	s.writeHeartbeat()
	hbPath := filepath.Join(s.registryDir(), "99999.json")
	_, err := os.Stat(hbPath)
	require.NoError(t, err)

	// Stop without Start — just test heartbeat cleanup.
	s.cancel = func() {} // no-op cancel since we didn't call Start
	s.Stop()

	_, err = os.Stat(hbPath)
	assert.True(t, os.IsNotExist(err), "heartbeat should be removed on stop")
}

func TestDefaultModeIsQueue(t *testing.T) {
	t.Parallel()
	d := &mockDeliverer{}
	s := newTestScanner(t, d)

	writeEnvelope(t, s.processDir(), "nomode.json", Envelope{
		Version: 1, ID: "nomode-1", Text: "default mode",
	})

	s.scan()

	d.mu.Lock()
	defer d.mu.Unlock()
	require.Len(t, d.sent, 1)
	assert.Equal(t, "default mode", d.sent[0])
	assert.Empty(t, d.steered)
}
