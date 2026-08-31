package changesdesk

import (
	"fmt"
	"os"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/answerkey"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/audit"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/trust"
)

// Desk wires the changes-desk pieces together for one node home.
type Desk struct {
	Home   *store.Home
	Trust  trust.View
	Queue  Queue
	Policy Policy
	Audit  *audit.Log
}

// Incoming carries the verified facts about one received supermessage that
// the detection logic needs. By the time this exists, the file signature has
// already been verified — strangers never reach the changes desk.
type Incoming struct {
	MessageNumber   string
	SenderName      string
	SenderCompanyID string
	KeyFingerprint  string
	KeyContinuity   string
	DeclaredID      string
	DeclaredVersion int
	DeclaredFP      string
	RulebookNorm    map[string]any // normalized rulebook section (nil if fingerprint-only)
	RulebookRaw     map[string]any // as-received rulebook section (json.Number intact)
	AnswerKeyNorm   map[string]any
	Channels        []any // normalized connections.channels
	EffectiveFrom   string
}

// NoticeRulebook files (or updates) the proposal to trust a new rulebook or
// rulebook version. The carrying message is held until a human decides.
func (d Desk) NoticeRulebook(inc Incoming, firstContact bool, now time.Time) (*Proposal, error) {
	subjectKey := fmt.Sprintf("%s@%s", inc.DeclaredID, inc.DeclaredFP)
	id := ProposalID(KindTrustRulebook, inc.SenderCompanyID, subjectKey)

	if existing, ok := d.Queue.Load(id); ok {
		if existing.State == StateWaiting || existing.State == StateApproved {
			existing.TimesSeen++
			existing.CarriedBy = appendUnique(existing.CarriedBy, inc.MessageNumber)
			existing.HeldMessages = appendUnique(existing.HeldMessages, inc.MessageNumber)
			d.Queue.Save(existing)
			d.Audit.Append("proposal_seen_again", map[string]any{
				"proposal_id": id, "message_number": inc.MessageNumber})
			return existing, nil
		}
		if existing.State == StateRejected {
			// A re-send of something already rejected is noted, never re-queued.
			d.Audit.Append("rejected_proposal_resent", map[string]any{
				"proposal_id": id, "message_number": inc.MessageNumber})
			return existing, nil
		}
	}

	p := &Proposal{
		ProposalID:   id,
		Kind:         KindTrustRulebook,
		FirstContact: firstContact,
		ProposedBy: Proposer{
			Name: inc.SenderName, CompanyID: inc.SenderCompanyID,
			KeyFingerprint: inc.KeyFingerprint, KeyContinuity: inc.KeyContinuity,
			SignatureChecked: true,
		},
		Subject: map[string]any{
			"rulebook_id":          inc.DeclaredID,
			"proposed_version":     inc.DeclaredVersion,
			"proposed_fingerprint": inc.DeclaredFP,
		},
		EffectiveFrom: inc.EffectiveFrom,
		OverlapDays:   d.Policy.OverlapDays,
		FirstSeenAt:   now.UTC().Format(time.RFC3339),
		ExpiresAt:     now.AddDate(0, 0, d.Policy.ExpiryDays).UTC().Format(time.RFC3339),
		TimesSeen:     1,
		CarriedBy:     []string{inc.MessageNumber},
		HeldMessages:  []string{inc.MessageNumber},
	}

	// Diff against the highest trusted version, or list everything as new.
	oldNorm := map[string]any{}
	if rt, ok := d.Trust.RulebookTrust(inc.DeclaredID); ok {
		if highest, ok := rt.HighestTrusted(); ok {
			p.Subject["trusted_version"] = highest.Version
			p.Subject["trusted_fingerprint"] = highest.Fingerprint
			if b, err := d.Trust.TrustedRulebookBytes(inc.DeclaredID, highest.Fingerprint); err == nil {
				if doc, err := supermessage.Parse(b); err == nil {
					oldNorm = supermessage.Normalize(doc.M).(map[string]any)
				}
			}
		}
	}
	p.WhatChanges = DiffRulebooks(oldNorm, inc.RulebookNorm)

	// The rulebook must pass its own answer key before it can even be shown
	// as approvable.
	report := answerkey.Run(inc.AnswerKeyNorm, inc.RulebookNorm)
	if report.AllPass {
		p.AnswerKeyNote = fmt.Sprintf("The rulebook passes its own answer key (%d examples checked).", len(report.Examples))
	} else {
		p.AnswerKeyNote = "WARNING: this rulebook FAILS its own answer key — its rules do not behave as its examples promise. It cannot be approved."
	}

	p.Risk = d.Policy.Classify(p)
	p.IfApproved = d.ifApprovedRulebook(p, firstContact)
	p.IfRejected = "A decline notice goes back to the sender. Nothing changes; held messages are refused."
	p.IfIgnored = fmt.Sprintf("The proposal expires on %s. The old configuration stays.", p.ExpiresAt[:10])
	p.Transition(StateWaiting, "node", "noticed in "+inc.MessageNumber, now)

	if err := d.Queue.Save(p); err != nil {
		return nil, err
	}
	// Store the exact rulebook bytes the proposal is about (canonical form,
	// signature included) and, on first contact, the offered connections.
	if inc.RulebookRaw != nil {
		canon, err := supermessage.Canonical(inc.RulebookRaw)
		if err != nil {
			return nil, err
		}
		if err := store.AtomicWrite(d.Queue.StoredFile(p, "proposed-rulebook.json"), canon); err != nil {
			return nil, err
		}
	}
	if firstContact && inc.Channels != nil {
		store.WriteJSON(d.Queue.StoredFile(p, "offered-connections.json"), map[string]any{"channels": inc.Channels})
	}
	d.Audit.Append("proposal_noticed", map[string]any{
		"proposal_id": id, "kind": p.Kind, "partner": inc.SenderCompanyID,
		"subject": subjectKey, "message_number": inc.MessageNumber, "risk": p.Risk.Class,
	})
	return p, nil
}

func (d Desk) ifApprovedRulebook(p *Proposal, firstContact bool) string {
	if firstContact {
		return fmt.Sprintf("The partner is pinned to key %s, their connection details are stored, rulebook version %v becomes trusted, and the held message is processed.",
			p.ProposedBy.KeyFingerprint, p.Subject["proposed_version"])
	}
	return fmt.Sprintf("Version %v becomes trusted from %s; version %v stays accepted for %d more days (overlap window); held messages are processed.",
		p.Subject["proposed_version"], orToday(p.EffectiveFrom), p.Subject["trusted_version"], p.OverlapDays)
}

// NoticeConnections files a proposal when a known partner's message carries
// connection details that differ from what is on file. The message itself is
// NOT held — traffic keeps flowing to the details on file until approval.
func (d Desk) NoticeConnections(inc Incoming, now time.Time) (*Proposal, error) {
	onFile, has := d.Trust.Connections(inc.SenderCompanyID)
	if !has {
		return nil, nil // first contact handles the initial connection set
	}
	changes := DiffConnections(onFile, inc.Channels)
	if len(changes) == 0 {
		return nil, nil
	}
	newCanon, err := supermessage.Canonical(inc.Channels)
	if err != nil {
		return nil, err
	}
	id := ProposalID(KindUpdateConnections, inc.SenderCompanyID, string(newCanon))
	if existing, ok := d.Queue.Load(id); ok && existing.State != StateExpired {
		if existing.State == StateWaiting {
			existing.TimesSeen++
			existing.CarriedBy = appendUnique(existing.CarriedBy, inc.MessageNumber)
			d.Queue.Save(existing)
		}
		return existing, nil
	}

	p := &Proposal{
		ProposalID: id,
		Kind:       KindUpdateConnections,
		ProposedBy: Proposer{
			Name: inc.SenderName, CompanyID: inc.SenderCompanyID,
			KeyFingerprint: inc.KeyFingerprint, KeyContinuity: inc.KeyContinuity,
			SignatureChecked: true,
		},
		Subject: map[string]any{
			"partner": inc.SenderCompanyID,
			"channel": changedItems(changes),
		},
		WhatChanges: changes,
		FirstSeenAt: now.UTC().Format(time.RFC3339),
		ExpiresAt:   now.AddDate(0, 0, d.Policy.ExpiryDays).UTC().Format(time.RFC3339),
		TimesSeen:   1,
		CarriedBy:   []string{inc.MessageNumber},
		IfApproved:  "The connection details on file are replaced. Your next messages to this partner travel to the new details.",
		IfRejected:  "Nothing changes; messages keep travelling to the details on file. A decline notice goes back.",
	}
	p.IfIgnored = fmt.Sprintf("The proposal expires on %s; messages keep travelling to the details on file.", p.ExpiresAt[:10])
	p.Risk = d.Policy.Classify(p)
	p.Transition(StateWaiting, "node", "noticed in "+inc.MessageNumber, now)
	if err := d.Queue.Save(p); err != nil {
		return nil, err
	}
	store.WriteJSON(d.Queue.StoredFile(p, "offered-connections.json"), map[string]any{"channels": inc.Channels})
	d.Audit.Append("proposal_noticed", map[string]any{
		"proposal_id": id, "kind": p.Kind, "partner": inc.SenderCompanyID, "risk": p.Risk.Class,
	})
	return p, nil
}

// Quarantine files an unverifiable message where it can do no harm, with a
// plain reason beside it.
func (d Desk) Quarantine(fileName string, raw []byte, reason string) {
	base := d.Home.File("inbox", "quarantine", fileName)
	store.AtomicWrite(base, raw)
	os.WriteFile(base+".reason.txt", []byte(reason+"\n"), 0o644)
	d.Audit.Append("verification_failed", map[string]any{"file": fileName, "reason": reason})
}

func changedItems(entries []DiffEntry) string {
	s := ""
	for _, e := range entries {
		if s != "" {
			s += ", "
		}
		s += e.Item
	}
	return s
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

func orToday(s string) string {
	if s == "" {
		return "today"
	}
	return s
}
