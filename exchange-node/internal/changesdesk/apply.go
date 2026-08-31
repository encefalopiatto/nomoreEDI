package changesdesk

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/trust"
)

// ApplyResult is what happened when an approved proposal was applied.
type ApplyResult struct {
	FilesWritten []string
	HeldMessages []string       // message numbers to run through the pipeline again
	AckContent   map[string]any // body of the configuration_response to send back
}

// Approve records a human decision. It does not touch trusted/ — Apply does.
func (d Desk) Approve(id, as, reason string, outOfBandConfirmed bool, now time.Time) (*Proposal, error) {
	p, ok := d.Queue.Load(id)
	if !ok {
		return nil, fmt.Errorf("no proposal %s", id)
	}
	if p.State != StateWaiting {
		return nil, fmt.Errorf("proposal %s is %s, not waiting for review", id, p.State)
	}
	if reason == "" {
		return nil, fmt.Errorf("a reason is required — it makes the audit log legible")
	}
	if strings.Contains(p.AnswerKeyNote, "FAILS") {
		return nil, fmt.Errorf("this rulebook fails its own answer key and cannot be approved")
	}
	if d.Policy.NeedsOutOfBandCheck(p.Kind) && !outOfBandConfirmed {
		return nil, fmt.Errorf("approving a %s requires confirming through a channel other than the message itself (phone your contact), then stating that you did", p.Kind)
	}
	p.Decision = &Decision{
		DecidedBy: as, DecidedAt: now.UTC().Format(time.RFC3339),
		Reason: reason, OutOfBandCheck: outOfBandConfirmed,
	}
	p.Transition(StateApproved, as, reason, now)
	if err := d.Queue.Save(p); err != nil {
		return nil, err
	}
	d.Audit.Append("decision_recorded", map[string]any{
		"proposal_id": id, "decision": "approved", "actor": as, "reason": reason,
		"out_of_band_check": outOfBandConfirmed,
	})
	return p, nil
}

// Reject records a human rejection.
func (d Desk) Reject(id, as, reason string, now time.Time) (*Proposal, error) {
	p, ok := d.Queue.Load(id)
	if !ok {
		return nil, fmt.Errorf("no proposal %s", id)
	}
	if p.State != StateWaiting {
		return nil, fmt.Errorf("proposal %s is %s, not waiting for review", id, p.State)
	}
	if reason == "" {
		return nil, fmt.Errorf("a reason is required — it makes the audit log legible")
	}
	p.Decision = &Decision{DecidedBy: as, DecidedAt: now.UTC().Format(time.RFC3339), Reason: reason}
	p.Transition(StateRejected, as, reason, now)
	if err := d.Queue.Save(p); err != nil {
		return nil, err
	}
	d.Audit.Append("decision_recorded", map[string]any{
		"proposal_id": id, "decision": "rejected", "actor": as, "reason": reason,
	})
	return p, nil
}

// Apply is the ONLY code path in the node that writes live configuration.
// It refuses anything that is not approved, and re-verifies signatures and
// fingerprints at apply time — approval alone moves no bytes.
func (d Desk) Apply(p *Proposal, publisherKey ed25519.PublicKey, now time.Time) (*ApplyResult, error) {
	if p.State != StateApproved {
		return nil, fmt.Errorf("refusing to apply proposal %s: its state is %s, not approved", p.ProposalID, p.State)
	}
	res := &ApplyResult{HeldMessages: p.HeldMessages}
	switch p.Kind {
	case KindTrustRulebook:
		if err := d.applyRulebook(p, publisherKey, now, res); err != nil {
			d.Audit.Append("apply_verification_failed", map[string]any{"proposal_id": p.ProposalID, "error": err.Error()})
			return nil, err
		}
	case KindUpdateConnections:
		if err := d.applyConnections(p, res); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown proposal kind %q", p.Kind)
	}

	p.Application = &Application{
		AppliedAt:    now.UTC().Format(time.RFC3339),
		FilesWritten: res.FilesWritten,
	}
	p.Transition(StateApplied, p.Decision.DecidedBy, "applied", now)
	if err := d.Queue.Save(p); err != nil {
		return nil, err
	}
	d.Audit.Append("change_applied", map[string]any{
		"proposal_id": p.ProposalID, "kind": p.Kind, "files": res.FilesWritten,
	})

	res.AckContent = map[string]any{
		"responding_to_proposal": p.ProposalID,
		"regarding":              p.Subject,
		"decision":               "accepted",
		"note":                   "Decision made after human review.",
	}
	if p.Kind == KindTrustRulebook && !p.FirstContact {
		res.AckContent["effective_from"] = orToday(p.EffectiveFrom)
		res.AckContent["we_still_accept_old_until"] = overlapEnd(p.EffectiveFrom, p.OverlapDays, now)
	}
	return res, nil
}

func (d Desk) applyRulebook(p *Proposal, publisherKey ed25519.PublicKey, now time.Time, res *ApplyResult) error {
	// 1. Re-verify: the stored bytes must still carry a valid publisher
	//    signature and still hash to the approved fingerprint.
	raw, err := os.ReadFile(d.Queue.StoredFile(p, "proposed-rulebook.json"))
	if err != nil {
		return fmt.Errorf("the proposed rulebook bytes are missing: %w", err)
	}
	doc, err := supermessage.Parse(raw)
	if err != nil {
		return err
	}
	fp, err := supermessage.Fingerprint(doc.M)
	if err != nil {
		return err
	}
	wantFP, _ := p.Subject["proposed_fingerprint"].(string)
	if fp != wantFP {
		return fmt.Errorf("the stored rulebook no longer matches the approved fingerprint — refusing")
	}
	signingBytes, err := supermessage.RulebookSigningBytes(doc.M)
	if err != nil {
		return err
	}
	sigValue := supermessage.GetString(doc.M, "publisher_signature", "value")
	if publisherKey == nil || !sign.Verify(publisherKey, signingBytes, sigValue) {
		return fmt.Errorf("the publisher signature on the rulebook does not verify — refusing")
	}

	rbID, _ := p.Subject["rulebook_id"].(string)
	version := intOf(p.Subject["proposed_version"])
	rbDir := d.Home.File("trusted", "rulebooks", strings.ReplaceAll(rbID, "/", "_"))
	if err := os.MkdirAll(rbDir, 0o755); err != nil {
		return err
	}

	// 2. Snapshot the current trust record for rollback.
	trustPath := rbDir + "/trust.json"
	if old, err := os.ReadFile(trustPath); err == nil {
		snap := d.Queue.StoredFile(p, "rollback-trust.json")
		store.AtomicWrite(snap, old)
		p.Application = &Application{RollbackSnapshot: snap}
	}

	// 3. Write the rulebook bytes under a name that carries its fingerprint.
	rbFile := fmt.Sprintf("%s/v%d-%s.json", rbDir, version, trust.ShortFP(fp))
	if err := store.AtomicWrite(rbFile, raw); err != nil {
		return err
	}
	res.FilesWritten = append(res.FilesWritten, rbFile)

	// 4. Update the trust record: the new version becomes accepted; the old
	//    highest stays accepted through the overlap window.
	var rt trust.RulebookTrust
	store.ReadJSON(trustPath, &rt)
	rt.RulebookID = rbID
	trustedFrom := orTodayDate(p.EffectiveFrom, now)
	if prev, ok := rt.HighestTrusted(); ok {
		for i := range rt.Accepting {
			if rt.Accepting[i].Version == prev.Version {
				rt.Accepting[i].AlsoAcceptedUntil = overlapEnd(p.EffectiveFrom, p.OverlapDays, now)
			}
		}
	}
	rt.Accepting = append(rt.Accepting, trust.TrustedVersion{
		Version: version, Fingerprint: fp, TrustedFrom: trustedFrom, ApprovedVia: p.ProposalID,
	})
	if err := store.WriteJSON(trustPath, rt); err != nil {
		return err
	}
	res.FilesWritten = append(res.FilesWritten, trustPath)

	// 5. First contact also pins the partner and stores their connections.
	if p.FirstContact {
		partnerPath := d.Home.File("trusted", "partners", store.SafeName(p.ProposedBy.CompanyID)+".json")
		store.WriteJSON(partnerPath, trust.Partner{
			Name: p.ProposedBy.Name, CompanyID: p.ProposedBy.CompanyID,
			KeyFingerprint: p.ProposedBy.KeyFingerprint,
			FirstSeen:      now.UTC().Format(time.RFC3339), ApprovedVia: p.ProposalID,
		})
		res.FilesWritten = append(res.FilesWritten, partnerPath)
		if offered, err := os.ReadFile(d.Queue.StoredFile(p, "offered-connections.json")); err == nil {
			connPath := d.Home.File("trusted", "connections", store.SafeName(p.ProposedBy.CompanyID)+".json")
			store.AtomicWrite(connPath, offered)
			res.FilesWritten = append(res.FilesWritten, connPath)
		}
	}
	return nil
}

func (d Desk) applyConnections(p *Proposal, res *ApplyResult) error {
	offered, err := os.ReadFile(d.Queue.StoredFile(p, "offered-connections.json"))
	if err != nil {
		return fmt.Errorf("the offered connection details are missing: %w", err)
	}
	connPath := d.Home.File("trusted", "connections", store.SafeName(p.ProposedBy.CompanyID)+".json")
	if old, err := os.ReadFile(connPath); err == nil {
		snap := d.Queue.StoredFile(p, "rollback-connections.json")
		store.AtomicWrite(snap, old)
	}
	if err := store.AtomicWrite(connPath, offered); err != nil {
		return err
	}
	res.FilesWritten = append(res.FilesWritten, connPath)
	return nil
}

// DeclineContent is the body of the configuration_response sent after a
// rejection.
func DeclineContent(p *Proposal) map[string]any {
	return map[string]any{
		"responding_to_proposal": p.ProposalID,
		"regarding":              p.Subject,
		"decision":               "declined",
		"reason":                 p.Decision.Reason,
		"note":                   "Decision made after human review. Nothing was changed.",
	}
}

func intOf(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return 0
}

func orTodayDate(s string, now time.Time) string {
	if s == "" {
		return now.UTC().Format("2006-01-02")
	}
	return s
}

func overlapEnd(effectiveFrom string, overlapDays int, now time.Time) string {
	start := now
	if t, err := time.Parse("2006-01-02", effectiveFrom); err == nil {
		start = t
	}
	return start.AddDate(0, 0, overlapDays).Format("2006-01-02")
}
