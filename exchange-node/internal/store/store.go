// Package store owns the node's home directory: where everything lives on
// disk, atomic file writes, the byte-exact archive, and the message log.
// Everything is plain JSON/JSONL a person can open in a text editor.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Identity is the node's own node.json.
type Identity struct {
	Name      string `json:"name"`
	CompanyID string `json:"company_id"`
	Directory string `json:"directory"` // path to the shared key phone book
}

// Home is one node's state directory.
type Home struct {
	Path string
	Now  func() time.Time // injectable clock (tests, deterministic demo)
}

func Open(path string) *Home {
	return &Home{Path: path, Now: time.Now}
}

// Subdirectories of a node home. Kept in one place so the layout is
// documented by code.
var layout = []string{
	"keys",
	"trusted/rulebooks",
	"trusted/connections",
	"trusted/partners",
	"policy",
	"review/pending",
	"review/decided",
	"held",
	"inbox",
	"inbox/quarantine",
	"outbox/drafts",
	"outbox/queue",
	"outbox/sent",
	"archive",
	"log",
	"audit",
	"transport/in",
}

// EnsureLayout creates the full home directory tree.
func (h *Home) EnsureLayout() error {
	for _, d := range layout {
		if err := os.MkdirAll(filepath.Join(h.Path, d), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (h *Home) File(parts ...string) string {
	return filepath.Join(append([]string{h.Path}, parts...)...)
}

// AtomicWrite writes bytes via a temp file and rename, so a crash can never
// leave a half-written file behind.
func AtomicWrite(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteJSON writes an indented JSON file atomically.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(b, '\n'))
}

// ReadJSON reads a JSON file into v.
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (h *Home) LoadIdentity() (Identity, error) {
	var id Identity
	err := ReadJSON(h.File("node.json"), &id)
	return id, err
}

func (h *Home) SaveIdentity(id Identity) error {
	return WriteJSON(h.File("node.json"), id)
}

// Archive stores a byte-exact copy of a supermessage, complete forever.
func (h *Home) Archive(direction, messageNumber string, raw []byte) error {
	name := fmt.Sprintf("%s-%s.supermessage.json", direction, safeName(messageNumber))
	return AtomicWrite(h.File("archive", name), raw)
}

// ReadArchived returns the archived bytes of a message, trying both directions.
func (h *Home) ReadArchived(messageNumber string) ([]byte, error) {
	for _, dir := range []string{"in", "out"} {
		b, err := os.ReadFile(h.File("archive", fmt.Sprintf("%s-%s.supermessage.json", dir, safeName(messageNumber))))
		if err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("message %s is not in the archive", messageNumber)
}

// LogEvent is one line of log/messages.jsonl: what happened to which message.
type LogEvent struct {
	At            string `json:"at"`
	MessageNumber string `json:"message_number,omitempty"`
	Stage         string `json:"stage"`
	Outcome       string `json:"outcome"`
	Detail        string `json:"detail,omitempty"`
}

// Log appends one event to the message log.
func (h *Home) Log(messageNumber, stage, outcome, detail string) {
	ev := LogEvent{
		At:            h.Now().UTC().Format(time.RFC3339),
		MessageNumber: messageNumber,
		Stage:         stage,
		Outcome:       outcome,
		Detail:        detail,
	}
	b, _ := json.Marshal(ev)
	f, err := os.OpenFile(h.File("log", "messages.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// ReadLog returns all message-log events (oldest first).
func (h *Home) ReadLog() []LogEvent {
	b, err := os.ReadFile(h.File("log", "messages.jsonl"))
	if err != nil {
		return nil
	}
	var out []LogEvent
	for _, line := range splitLines(b) {
		var ev LogEvent
		if json.Unmarshal(line, &ev) == nil {
			out = append(out, ev)
		}
	}
	return out
}

// ListDir returns the sorted file names in a home subdirectory.
func (h *Home) ListDir(parts ...string) []string {
	entries, err := os.ReadDir(h.File(parts...))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// SafeName makes a string usable as a file name.
func SafeName(s string) string { return safeName(s) }

func safeName(s string) string {
	out := []rune(s)
	for i, r := range out {
		switch r {
		case '/', '\\', ':', ' ':
			out[i] = '_'
		}
	}
	return string(out)
}

func splitLines(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				lines = append(lines, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, b[start:])
	}
	return lines
}
