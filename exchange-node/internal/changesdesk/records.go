// Package changesdesk is the self-adjusting half of the node: it treats
// incoming supermessages as configuration proposals, files them for human
// review, and — only after approval — applies them to the live configuration.
//
// The one invariant of the whole node lives here: Apply (in apply.go) is the
// ONLY code that writes to trusted/, and it refuses any proposal that is not
// in the approved state. Messages propose; humans apply.
package changesdesk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
)

// Proposal kinds.
const (
	KindTrustRulebook     = "trust_rulebook"
	KindUpdateConnections = "update_connections"
)

// Proposal states.
const (
	StateWaiting  = "waiting_for_review"
	StateApproved = "approved"
	StateRejected = "rejected"
	StateApplied  = "applied"
	StateExpired  = "expired"
)

type StateChange struct {
	State string `json:"state"`
	At    string `json:"at"`
	By    string `json:"by"`
	Note  string `json:"note,omitempty"`
}

type Proposer struct {
	Name             string `json:"name"`
	CompanyID        string `json:"company_id"`
	KeyFingerprint   string `json:"key_fingerprint"`
	KeyContinuity    string `json:"key_continuity"`
	SignatureChecked bool   `json:"signature_checked"`
}

type DiffEntry struct {
	Section      string   `json:"section"`
	Change       string   `json:"change"` // added | removed | changed
	Item         string   `json:"item"`
	InPlainWords string   `json:"in_plain_words"`
	RiskFlags    []string `json:"risk_flags,omitempty"`
	Warning      string   `json:"warning,omitempty"`
}

type Risk struct {
	Class   string   `json:"class"` // review | two-keys
	Because []string `json:"because"`
}

type Decision struct {
	DecidedBy      string `json:"decided_by"`
	DecidedAt      string `json:"decided_at"`
	Reason         string `json:"reason"`
	OutOfBandCheck bool   `json:"out_of_band_check"`
}

type Application struct {
	AppliedAt        string   `json:"applied_at"`
	FilesWritten     []string `json:"files_written"`
	AcknowledgedWith string   `json:"acknowledged_with,omitempty"`
	RollbackSnapshot string   `json:"rollback_snapshot,omitempty"` // path inside the proposal dir
}

// Proposal is one filed configuration proposal (review/pending/<id>/record.json).
type Proposal struct {
	ProposalID    string         `json:"proposal_id"`
	Kind          string         `json:"kind"`
	State         string         `json:"state"`
	StateHistory  []StateChange  `json:"state_history"`
	ProposedBy    Proposer       `json:"proposed_by"`
	Subject       map[string]any `json:"subject"`
	FirstContact  bool           `json:"first_contact,omitempty"`
	WhatChanges   []DiffEntry    `json:"what_changes"`
	AnswerKeyNote string         `json:"answer_key_note,omitempty"`
	Risk          Risk           `json:"risk"`
	EffectiveFrom string         `json:"effective_from,omitempty"`
	OverlapDays   int            `json:"overlap_days,omitempty"`
	FirstSeenAt   string         `json:"first_seen_at"`
	ExpiresAt     string         `json:"expires_at"`
	TimesSeen     int            `json:"times_seen"`
	CarriedBy     []string       `json:"carried_by"`
	HeldMessages  []string       `json:"held_messages,omitempty"`
	Decision      *Decision      `json:"decision,omitempty"`
	Application   *Application   `json:"application,omitempty"`

	// The three futures, spelled out for the reviewer.
	IfApproved string `json:"if_approved"`
	IfRejected string `json:"if_rejected"`
	IfIgnored  string `json:"if_ignored"`
}

// ProposalID derives the content-addressed id: the same proposal arriving
// five hundred times is one queue entry.
func ProposalID(kind, partner, subjectKey string) string {
	sum := sha256.Sum256([]byte(kind + "|" + partner + "|" + subjectKey))
	return "p-" + hex.EncodeToString(sum[:6])
}

// Queue reads and writes proposal records under review/pending and
// review/decided. It never touches trusted/.
type Queue struct {
	Home *store.Home
}

func (q Queue) dir(state string) string {
	if state == StateWaiting || state == StateApproved {
		return q.Home.File("review", "pending")
	}
	return q.Home.File("review", "decided")
}

func (q Queue) recordDir(id string, pending bool) string {
	if pending {
		return q.Home.File("review", "pending", id)
	}
	return q.Home.File("review", "decided", id)
}

// Load finds a proposal by id in pending, then decided.
func (q Queue) Load(id string) (*Proposal, bool) {
	for _, pending := range []bool{true, false} {
		var p Proposal
		if store.ReadJSON(q.recordDir(id, pending)+"/record.json", &p) == nil {
			return &p, true
		}
	}
	return nil, false
}

// Save writes a proposal record into the folder matching its state.
func (q Queue) Save(p *Proposal) error {
	pending := p.State == StateWaiting || p.State == StateApproved
	dir := q.recordDir(p.ProposalID, pending)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := store.WriteJSON(dir+"/record.json", p); err != nil {
		return err
	}
	// If it just left the pending state, move the whole folder across.
	if !pending {
		old := q.recordDir(p.ProposalID, true)
		if _, err := os.Stat(old); err == nil {
			mergeMove(old, dir)
		}
	}
	return nil
}

// ListPending returns all proposals awaiting review or awaiting apply.
func (q Queue) ListPending() []*Proposal {
	entries, err := os.ReadDir(q.Home.File("review", "pending"))
	if err != nil {
		return nil
	}
	var out []*Proposal
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if p, ok := q.Load(e.Name()); ok {
			out = append(out, p)
		}
	}
	return out
}

// Transition records a state change with its author and note.
func (p *Proposal) Transition(state, by, note string, now time.Time) {
	p.State = state
	p.StateHistory = append(p.StateHistory, StateChange{
		State: state, At: now.UTC().Format(time.RFC3339), By: by, Note: note,
	})
}

// StoredFile returns the path of an extra file stored inside the proposal's
// folder (the proposed rulebook bytes, rollback snapshots, carrying messages).
func (q Queue) StoredFile(p *Proposal, name string) string {
	pending := p.State == StateWaiting || p.State == StateApproved
	return q.recordDir(p.ProposalID, pending) + "/" + name
}

// mergeMove moves the contents of one directory into another.
func mergeMove(from, to string) {
	entries, err := os.ReadDir(from)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == "record.json" {
			os.Remove(from + "/" + e.Name())
			continue
		}
		os.Rename(from+"/"+e.Name(), to+"/"+e.Name())
	}
	os.Remove(from)
}

// ExpireOverdue sweeps pending proposals past their expiry date.
func (q Queue) ExpireOverdue(now time.Time) []string {
	var expired []string
	for _, p := range q.ListPending() {
		if p.State != StateWaiting || p.ExpiresAt == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil && now.After(t) {
			p.Transition(StateExpired, "node", "expiry date passed; nothing was changed", now)
			q.Save(p)
			expired = append(expired, p.ProposalID)
		}
	}
	return expired
}

// Describe renders a one-sentence summary for queue listings.
func (p *Proposal) Describe() string {
	switch p.Kind {
	case KindTrustRulebook:
		if p.FirstContact {
			return fmt.Sprintf("First contact: %s asks you to trust rulebook %v (version %v) and their connection details",
				p.ProposedBy.Name, p.Subject["rulebook_id"], p.Subject["proposed_version"])
		}
		return fmt.Sprintf("%s published rulebook %v version %v (you trust version %v)",
			p.ProposedBy.Name, p.Subject["rulebook_id"], p.Subject["proposed_version"], p.Subject["trusted_version"])
	case KindUpdateConnections:
		return fmt.Sprintf("%s wants to change connection details (%v)", p.ProposedBy.Name, p.Subject["channel"])
	}
	return p.Kind
}
