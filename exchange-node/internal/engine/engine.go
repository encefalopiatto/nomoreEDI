// Package engine is the exchange node itself: the pipeline every incoming
// file walks (verify → resolve rulebook → validate → deliver), the hold and
// resume machinery around human review, and the outbound side (responses,
// rejections, configuration acknowledgements). The web console, the CLI, and
// the tests all drive the same Node methods.
package engine

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/audit"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/changesdesk"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/checker"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/directory"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/respond"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/transport"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/trust"
)

// Node is one running exchange node over one home directory.
type Node struct {
	Home     *store.Home
	Identity store.Identity
	Dir      *directory.Directory
	Priv     ed25519.PrivateKey
	Desk     changesdesk.Desk
	Trust    trust.View
	Sender   transport.Sender
	Audit    *audit.Log
	Now      func() time.Time
}

// InboxRecord is one delivered message as the console shows it.
type InboxRecord struct {
	MessageNumber string              `json:"message_number"`
	FromName      string              `json:"from_name"`
	FromID        string              `json:"from_company_id"`
	Type          string              `json:"message_type"`
	ReceivedAt    string              `json:"received_at"`
	Status        string              `json:"status"` // green | red
	Violations    []checker.Violation `json:"violations,omitempty"`
	SkippedRules  []checker.Skipped   `json:"skipped_rules,omitempty"`
	Rulebook      map[string]any      `json:"rulebook,omitempty"`
	Content       map[string]any      `json:"content"`
	EchoCheck     string              `json:"echo_check,omitempty"`
}

// Init creates a fresh node home: keys, identity, default policy, layout,
// and publishes the public key to the shared directory (the phone-book stub).
func Init(path, name, companyID, directoryPath string) (*Node, error) {
	h := store.Open(path)
	if err := h.EnsureLayout(); err != nil {
		return nil, err
	}
	pub, err := sign.Generate(h.File("keys", "private.key"), h.File("keys", "public.key"))
	if err != nil {
		return nil, err
	}
	id := store.Identity{Name: name, CompanyID: companyID, Directory: directoryPath}
	if err := h.SaveIdentity(id); err != nil {
		return nil, err
	}
	dir, err := directory.Open(directoryPath)
	if err != nil {
		return nil, err
	}
	if err := dir.Publish(companyID, name, pub); err != nil {
		return nil, err
	}
	if err := store.WriteJSON(h.File("policy", "review-policy.json"), changesdesk.DefaultPolicy()); err != nil {
		return nil, err
	}
	return Open(path)
}

// Open loads an existing node home.
func Open(path string) (*Node, error) {
	h := store.Open(path)
	id, err := h.LoadIdentity()
	if err != nil {
		return nil, fmt.Errorf("%s is not a node home (missing node.json): %w", path, err)
	}
	dir, err := directory.Open(id.Directory)
	if err != nil {
		return nil, err
	}
	priv, err := sign.LoadPrivate(h.File("keys", "private.key"))
	if err != nil {
		return nil, err
	}
	auditLog := audit.Open(h.File("audit", "audit-log.jsonl"))
	policy, err := changesdesk.LoadPolicy(h)
	if err != nil {
		return nil, err
	}
	tv := trust.View{Home: h}
	n := &Node{
		Home: h, Identity: id, Dir: dir, Priv: priv,
		Trust:  tv,
		Desk:   changesdesk.Desk{Home: h, Trust: tv, Queue: changesdesk.Queue{Home: h}, Policy: policy, Audit: auditLog},
		Sender: transport.Folder{},
		Audit:  auditLog,
		Now:    time.Now,
	}
	return n, nil
}

// ReceiveAll drains the transport in-folder through the pipeline.
func (n *Node) ReceiveAll() []string {
	files, err := transport.Collect(n.Home.File("transport", "in"))
	if err != nil {
		return []string{fmt.Sprintf("could not read the in-folder: %v", err)}
	}
	var outcomes []string
	for name, raw := range files {
		outcomes = append(outcomes, fmt.Sprintf("%s: %s", name, n.Ingest(name, raw)))
	}
	return outcomes
}

// Ingest walks one received file through the full pipeline and returns a
// one-line outcome for logs and screens.
func (n *Node) Ingest(fileName string, raw []byte) string {
	doc, err := supermessage.Parse(raw)
	if err != nil {
		n.Desk.Quarantine(fileName, raw, "not readable as a supermessage: "+err.Error())
		return "quarantined (unreadable)"
	}
	about, _ := doc.Section("about")
	msgNo := supermessage.GetString(about, "message_number")
	senderID := supermessage.GetString(about, "sender", "company_id")
	senderName := supermessage.GetString(about, "sender", "name")

	// Duplicate delivery of the same message number is a no-op.
	if _, err := n.Home.ReadArchived(msgNo); err == nil && msgNo != "" {
		n.Home.Log(msgNo, "received", "duplicate", "already archived; ignored")
		return "duplicate (already processed)"
	}
	n.Home.Archive("in", msgNo, raw)
	n.Home.Log(msgNo, "received", "ok", "from "+senderName)

	// The gatekeeper: the file must be signed with the key the directory
	// lists for the company it CLAIMS to be from. Strangers stop here.
	pub, err := n.Dir.LookUp(senderID)
	if err != nil {
		n.Desk.Quarantine(fileName, raw, fmt.Sprintf("sender %q is not in the key directory", senderID))
		n.Home.Log(msgNo, "signature_check", "quarantined", "sender unknown to the key directory")
		return "quarantined (unknown sender)"
	}
	signingBytes, err := supermessage.FileSigningBytes(doc.M)
	if err != nil {
		return "internal error: " + err.Error()
	}
	sigValue := supermessage.GetString(about, "signature", "value")
	if !sign.Verify(pub, signingBytes, sigValue) {
		reason := fmt.Sprintf("this file claims to be from %s but is not signed with %s's key. Nothing inside it was trusted or shown for approval. Kept as evidence.", senderName, senderName)
		n.Desk.Quarantine(fileName, raw, reason)
		n.Home.Log(msgNo, "signature_check", "quarantined", "signature does not verify")
		return "quarantined (signature check failed)"
	}
	n.Home.Log(msgNo, "signature_check", "ok", "signed by "+sign.KeyFingerprint(pub))

	return n.continuePipeline(fileName, doc, raw)
}

// continuePipeline runs the post-signature stages. Held messages re-enter here.
func (n *Node) continuePipeline(fileName string, doc *supermessage.Doc, raw []byte) string {
	about, _ := doc.Section("about")
	msgNo := supermessage.GetString(about, "message_number")
	senderID := supermessage.GetString(about, "sender", "company_id")
	senderName := supermessage.GetString(about, "sender", "name")
	msgType := supermessage.GetString(about, "message_type")
	pub, _ := n.Dir.LookUp(senderID)

	norm := supermessage.Normalize(doc.M).(map[string]any)

	// Protocol-level notes (acknowledgements, rejections) carry no rulebook:
	// verify, show, done.
	if msgType == "configuration_response" || msgType == "rejection_notice" {
		content, _ := norm["content"].(map[string]any)
		if content == nil {
			content = map[string]any{}
		}
		n.deliver(InboxRecord{
			MessageNumber: msgNo, FromName: senderName, FromID: senderID, Type: msgType,
			ReceivedAt: n.Now().UTC().Format(time.RFC3339), Status: "green",
			Content: content,
		})
		return "delivered (" + msgType + ")"
	}

	// Resolve the rulebook this message claims to follow.
	fr, _ := about["follows_rulebook"].(map[string]any)
	declaredID := supermessage.GetString(about, "follows_rulebook", "id")
	declaredFP := supermessage.GetString(about, "follows_rulebook", "fingerprint")
	declaredVersion := 0
	if v, ok := fr["version"].(json.Number); ok {
		if i, err := v.Int64(); err == nil {
			declaredVersion = int(i)
		}
	}

	rbSection, hasRulebook := doc.Section("rulebook")
	if hasRulebook {
		computedFP, err := supermessage.Fingerprint(rbSection)
		if err != nil || computedFP != declaredFP {
			n.Desk.Quarantine(fileName, raw, "the rulebook inside this file does not match the fingerprint the file declares — the file was tampered with or corrupted")
			n.Home.Log(msgNo, "rulebook_check", "quarantined", "fingerprint mismatch")
			return "quarantined (rulebook fingerprint mismatch)"
		}
		// Prototype simplification: the rulebook publisher's key is the
		// sender's directory key (the retailer publishes its own rulebooks).
		rbSigning, _ := supermessage.RulebookSigningBytes(rbSection)
		rbSig := supermessage.GetString(rbSection, "publisher_signature", "value")
		if !sign.Verify(pub, rbSigning, rbSig) {
			n.Desk.Quarantine(fileName, raw, "the rulebook inside this file is not signed by its claimed publisher")
			n.Home.Log(msgNo, "rulebook_check", "quarantined", "publisher signature failed")
			return "quarantined (rulebook publisher signature failed)"
		}
	}

	status := n.Trust.StatusOf(declaredID, declaredVersion, declaredFP, n.Now())
	_, partnerKnown := n.Trust.Partner(senderID)

	switch status {
	case trust.StatusCounterfeit:
		n.Audit.Append("same_version_different_content", map[string]any{"message_number": msgNo, "rulebook": declaredID})
		n.Desk.Quarantine(fileName, raw, "this file cites a rulebook version we know — but with DIFFERENT content. That is a counterfeit or corrupted rulebook, never a proposal.")
		n.Home.Log(msgNo, "rulebook_check", "quarantined", "counterfeit rulebook")
		return "quarantined (counterfeit rulebook)"

	case trust.StatusDowngrade:
		n.Audit.Append("downgrade_attempt", map[string]any{"message_number": msgNo, "rulebook": declaredID, "version": declaredVersion})
		n.sendNote(senderID, "configuration_response", map[string]any{
			"regarding": map[string]any{"message_number": msgNo, "rulebook_id": declaredID, "version": declaredVersion},
			"decision":  "declined",
			"reason":    "this message follows a retired rulebook version; please use the current version",
		})
		n.Home.Log(msgNo, "rulebook_check", "refused", "downgrade attempt")
		return "refused (retired rulebook version)"

	case trust.StatusUnknown, trust.StatusUpdate:
		if !hasRulebook {
			n.hold(msgNo, raw)
			n.sendNote(senderID, "configuration_response", map[string]any{
				"regarding": map[string]any{"message_number": msgNo, "rulebook_id": declaredID, "fingerprint": declaredFP},
				"decision":  "please_resend_complete",
				"reason":    "we do not know this rulebook yet — send the message again with the full rulebook included",
			})
			n.Home.Log(msgNo, "rulebook_check", "held", "unknown rulebook cited by fingerprint only")
			return "held (unknown rulebook, asked for the full text)"
		}
		inc := n.incomingFrom(doc, norm, declaredID, declaredVersion, declaredFP)
		firstContact := !partnerKnown
		p, err := n.Desk.NoticeRulebook(inc, firstContact, n.Now())
		if err != nil {
			return "internal error: " + err.Error()
		}
		n.hold(msgNo, raw)
		n.Home.Log(msgNo, "rulebook_check", "held", "awaiting review of proposal "+p.ProposalID)
		return "held (rulebook awaiting your approval: " + p.ProposalID + ")"
	}

	// Rulebook trusted. A known partner's changed connection details become a
	// proposal on the side; the message itself keeps flowing.
	if partnerKnown {
		inc := n.incomingFrom(doc, norm, declaredID, declaredVersion, declaredFP)
		if p, _ := n.Desk.NoticeConnections(inc, n.Now()); p != nil && p.TimesSeen == 1 {
			n.Home.Log(msgNo, "connections_check", "proposal", p.ProposalID)
		}
	}

	// Validate content with the TRUSTED stored bytes (identical to the
	// embedded copy by fingerprint, but trust flows from our own store).
	trustedBytes, err := n.Trust.TrustedRulebookBytes(declaredID, declaredFP)
	if err != nil {
		return "internal error: trusted rulebook bytes missing: " + err.Error()
	}
	rbDoc, _ := supermessage.Parse(trustedBytes)
	rbNorm := supermessage.Normalize(rbDoc.M).(map[string]any)
	content, _ := norm["content"].(map[string]any)

	result, err := checker.Check(content, rbNorm, msgType)
	if err != nil {
		return "internal error: checker: " + err.Error()
	}

	rec := InboxRecord{
		MessageNumber: msgNo, FromName: senderName, FromID: senderID, Type: msgType,
		ReceivedAt: n.Now().UTC().Format(time.RFC3339),
		Violations: result.Violations, SkippedRules: result.SkippedRules,
		Rulebook: map[string]any{"id": declaredID, "version": declaredVersion, "fingerprint": declaredFP},
		Content:  content,
	}
	rec.Status = "green"
	if !result.Passed() {
		rec.Status = "red"
	}

	// A response to something we sent also gets its echoes checked.
	if ref, ok := content["order_reference"].(string); ok && msgType != "order" {
		rec.EchoCheck = n.checkEchoes(ref, msgType, content)
	}

	n.deliver(rec)
	n.Home.Log(msgNo, "content_validated", rec.Status, strings.Join(result.ViolatedIDs(), ", "))
	if rec.Status == "red" {
		return "delivered RED (" + strings.Join(result.ViolatedIDs(), ", ") + ")"
	}
	return "delivered green"
}

func (n *Node) incomingFrom(doc *supermessage.Doc, norm map[string]any, id string, version int, fp string) changesdesk.Incoming {
	about, _ := doc.Section("about")
	rbRaw, _ := doc.Section("rulebook")
	rbNorm, _ := norm["rulebook"].(map[string]any)
	akNorm, _ := norm["answer_key"].(map[string]any)
	senderID := supermessage.GetString(about, "sender", "company_id")
	pub, _ := n.Dir.LookUp(senderID)
	var channels []any
	if conns, ok := norm["connections"].(map[string]any); ok {
		channels, _ = conns["channels"].([]any)
	}
	effective := ""
	if rbNorm != nil {
		effective, _ = rbNorm["valid_from"].(string)
	}
	return changesdesk.Incoming{
		MessageNumber:   supermessage.GetString(about, "message_number"),
		SenderName:      supermessage.GetString(about, "sender", "name"),
		SenderCompanyID: senderID,
		KeyFingerprint:  sign.KeyFingerprint(pub),
		KeyContinuity:   n.keyContinuity(senderID),
		DeclaredID:      id, DeclaredVersion: version, DeclaredFP: fp,
		RulebookNorm: rbNorm, RulebookRaw: rbRaw, AnswerKeyNorm: akNorm,
		Channels: channels, EffectiveFrom: effective,
	}
}

// keyContinuity summarizes how long and how much this sender's key has been
// seen verifying real traffic — the reviewer's context line.
func (n *Node) keyContinuity(senderID string) string {
	count := 0
	first := ""
	for _, ev := range n.Home.ReadLog() {
		if ev.Stage == "signature_check" && ev.Outcome == "ok" {
			count++
			if first == "" {
				first = ev.At
			}
		}
	}
	if count == 0 {
		return "this is the first verified message from this sender"
	}
	return fmt.Sprintf("this key has signed %d verified messages since %s", count, first[:10])
}

func (n *Node) hold(msgNo string, raw []byte) {
	store.AtomicWrite(n.Home.File("held", store.SafeName(msgNo)+".supermessage.json"), raw)
}

func (n *Node) deliver(rec InboxRecord) {
	store.WriteJSON(n.Home.File("inbox", store.SafeName(rec.MessageNumber)+".json"), rec)
}

// checkEchoes verifies an incoming response against the choreography of the
// original message we sent.
func (n *Node) checkEchoes(originalNumber, responseType string, content map[string]any) string {
	origRaw, err := n.Home.ReadArchived(originalNumber)
	if err != nil {
		return "could not check echoes: the original " + originalNumber + " is not in the archive"
	}
	origDoc, err := supermessage.Parse(origRaw)
	if err != nil {
		return "could not check echoes: " + err.Error()
	}
	origNorm := supermessage.Normalize(origDoc.M).(map[string]any)
	d := &respond.Draft{ResponseType: responseType, Content: content}
	if _, err := respond.Finish(d, origNorm, nil); err != nil {
		return "ECHO PROBLEM: " + err.Error()
	}
	return "echoes verified against " + originalNumber
}

// ---- decisions (the same API the web console, CLI and tests call) ----

// Approve applies a proposal after recording the human decision, sends the
// acknowledgement, and releases any held messages back through the pipeline.
func (n *Node) Approve(proposalID, as, reason string, outOfBandConfirmed bool) (string, error) {
	p, err := n.Desk.Approve(proposalID, as, reason, outOfBandConfirmed, n.Now())
	if err != nil {
		return "", err
	}
	publisherKey, err := n.Dir.LookUp(p.ProposedBy.CompanyID)
	if err != nil {
		return "", err
	}
	res, err := n.Desk.Apply(p, publisherKey, n.Now())
	if err != nil {
		return "", err
	}
	n.sendNote(p.ProposedBy.CompanyID, "configuration_response", res.AckContent)

	released := 0
	for _, msgNo := range res.HeldMessages {
		path := n.Home.File("held", store.SafeName(msgNo)+".supermessage.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		os.Remove(path)
		doc, err := supermessage.Parse(raw)
		if err != nil {
			continue
		}
		n.Home.Log(msgNo, "resumed", "ok", "released by approval of "+proposalID)
		n.continuePipeline("resumed-"+msgNo, doc, raw)
		released++
	}
	return fmt.Sprintf("applied. Files written: %s. Held messages released: %d.",
		strings.Join(res.FilesWritten, ", "), released), nil
}

// Reject records the rejection, notifies the sender, and refuses held messages.
func (n *Node) Reject(proposalID, as, reason string) (string, error) {
	p, err := n.Desk.Reject(proposalID, as, reason, n.Now())
	if err != nil {
		return "", err
	}
	n.sendNote(p.ProposedBy.CompanyID, "configuration_response", changesdesk.DeclineContent(p))
	for _, msgNo := range p.HeldMessages {
		os.Remove(n.Home.File("held", store.SafeName(msgNo)+".supermessage.json"))
		n.Home.Log(msgNo, "refused", "rejected", "its rulebook proposal was rejected")
	}
	return "rejected. The sender was told; nothing was changed.", nil
}

// ---- outbound ----

// StartResponse builds a draft for one of the allowed responses to an
// archived message.
func (n *Node) StartResponse(messageNumber, responseType string) (*respond.Draft, error) {
	origNorm, err := n.archivedNorm(messageNumber)
	if err != nil {
		return nil, err
	}
	d, err := respond.Build(origNorm, responseType, n.Now())
	if err != nil {
		return nil, err
	}
	if err := store.WriteJSON(n.Home.File("outbox", "drafts", d.DraftID+".json"), d); err != nil {
		return nil, err
	}
	n.Home.Log(messageNumber, "response_drafted", "ok", d.DraftID)
	return d, nil
}

// FinishResponse validates a filled draft, signs it, sends it, archives it.
func (n *Node) FinishResponse(draftID string) (string, error) {
	var d respond.Draft
	draftPath := n.Home.File("outbox", "drafts", draftID+".json")
	if err := store.ReadJSON(draftPath, &d); err != nil {
		return "", fmt.Errorf("no draft %s: %w", draftID, err)
	}
	origNorm, err := n.archivedNorm(d.BasedOn)
	if err != nil {
		return "", err
	}
	// The rulebook's rules for this response type run as part of finishing.
	rbNorm, _ := origNorm["rulebook"].(map[string]any)
	checkRules := func(content map[string]any) []string {
		if rbNorm == nil {
			return nil
		}
		res, err := checker.Check(content, rbNorm, d.ResponseType)
		if err != nil {
			return []string{"rule check failed: " + err.Error()}
		}
		var out []string
		for _, v := range res.Violations {
			out = append(out, v.Messages...)
		}
		return out
	}
	content, err := respond.Finish(&d, origNorm, checkRules)
	if err != nil {
		return "", err
	}

	about, _ := origNorm["about"].(map[string]any)
	fr, _ := about["follows_rulebook"].(map[string]any)
	msg := n.newMessage(d.ResponseType, d.PartnerID, d.PartnerName, content)
	if fr != nil {
		msg["about"].(map[string]any)["follows_rulebook"] = map[string]any{
			"id": fr["id"], "version": fr["version"], "fingerprint": fr["fingerprint"],
			"included_below": false,
		}
	}
	msgNo, err := n.signAndSend(d.PartnerID, msg)
	if err != nil {
		return "", err
	}
	os.Remove(draftPath)
	n.Home.Log(msgNo, "sent", "ok", d.ResponseType+" answering "+d.BasedOn)
	return msgNo, nil
}

// FillDraft writes values into a draft's holes (the web form's submit).
func (n *Node) FillDraft(draftID string, fills map[string]any) error {
	var d respond.Draft
	path := n.Home.File("outbox", "drafts", draftID+".json")
	if err := store.ReadJSON(path, &d); err != nil {
		return err
	}
	respond.ApplyFills(&d, fills)
	return store.WriteJSON(path, &d)
}

// SendRejection answers a red inbox message with a structured rejection
// carrying the exact broken rule IDs and sentences.
func (n *Node) SendRejection(messageNumber string) (string, error) {
	var rec InboxRecord
	if err := store.ReadJSON(n.Home.File("inbox", store.SafeName(messageNumber)+".json"), &rec); err != nil {
		return "", fmt.Errorf("no inbox record for %s", messageNumber)
	}
	if rec.Status != "red" {
		return "", fmt.Errorf("%s passed its checks — there is nothing to reject", messageNumber)
	}
	content := map[string]any{
		"regarding":    messageNumber,
		"failed_rules": rec.Violations,
		"note":         "This message broke the rules of the rulebook it declares. Details name the exact rules.",
	}
	msg := n.newMessage("rejection_notice", rec.FromID, rec.FromName, content)
	msgNo, err := n.signAndSend(rec.FromID, msg)
	if err != nil {
		return "", err
	}
	n.Home.Log(msgNo, "sent", "ok", "rejection of "+messageNumber)
	return msgNo, nil
}

// SendFile signs and sends a prepared supermessage tree (the demo's sender side).
func (n *Node) SendFile(receiverID string, msg map[string]any) (string, error) {
	return n.signAndSend(receiverID, msg)
}

// sendNote sends a small protocol note (acknowledgement, decline, ask).
func (n *Node) sendNote(receiverID, msgType string, content map[string]any) {
	receiverName := receiverID
	if p, ok := n.Trust.Partner(receiverID); ok {
		receiverName = p.Name
	}
	msg := n.newMessage(msgType, receiverID, receiverName, content)
	if _, err := n.signAndSend(receiverID, msg); err != nil {
		// No route yet (e.g. declining a first contact): keep it in outbox/ready.
		b := supermessage.MarshalPretty(msg)
		store.AtomicWrite(n.Home.File("outbox", "ready", fmt.Sprintf("%s-%s.supermessage.json", msgType, n.Now().Format("150405"))), b)
	}
}

func (n *Node) newMessage(msgType, receiverID, receiverName string, content map[string]any) map[string]any {
	return map[string]any{
		"supermessage_version": "0.1",
		"about": map[string]any{
			"message_type":   msgType,
			"message_number": n.nextNumber(msgType),
			"sent_at":        n.Now().UTC().Format(time.RFC3339),
			"sender":         map[string]any{"name": n.Identity.Name, "company_id": n.Identity.CompanyID},
			"receiver":       map[string]any{"name": receiverName, "company_id": receiverID},
			"signature": map[string]any{
				"signed_by": n.Identity.Name,
				"method":    "ed25519-public-key-signature",
				"value":     "",
			},
		},
		"content": content,
	}
}

// nextNumber produces a readable unique outbound message number.
func (n *Node) nextNumber(msgType string) string {
	prefix := map[string]string{
		"order_response":         "ORSP",
		"despatch_advice":        "DESP",
		"configuration_response": "CFG",
		"rejection_notice":       "REJ",
	}[msgType]
	if prefix == "" {
		prefix = "MSG"
	}
	count := len(n.Home.ListDir("outbox", "sent")) + 1
	return fmt.Sprintf("%s-%s-%04d", prefix, n.Now().Format("20060102"), count)
}

// signAndSend signs a message tree and delivers it via the partner's
// connection details ON FILE — never via anything a message claimed.
func (n *Node) signAndSend(receiverID string, msg map[string]any) (string, error) {
	signingBytes, err := supermessage.FileSigningBytes(msg)
	if err != nil {
		return "", err
	}
	about := msg["about"].(map[string]any)
	about["signature"].(map[string]any)["value"] = sign.Sign(n.Priv, signingBytes)
	msgNo, _ := about["message_number"].(string)

	address, err := n.routeTo(receiverID)
	if err != nil {
		return "", err
	}
	b := supermessage.MarshalPretty(msg)
	fileName := store.SafeName(msgNo) + ".supermessage.json"
	if err := n.Sender.Send(address, fileName, b); err != nil {
		return "", err
	}
	n.Home.Archive("out", msgNo, b)
	store.AtomicWrite(n.Home.File("outbox", "sent", fileName), b)
	return msgNo, nil
}

// routeTo picks the folder-channel address from the connections on file.
func (n *Node) routeTo(receiverID string) (string, error) {
	channels, ok := n.Trust.Connections(receiverID)
	if !ok {
		return "", fmt.Errorf("no connection details on file for %s", receiverID)
	}
	for _, c := range channels {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if ch, _ := m["channel"].(string); ch == "local-folder" {
			if addr, ok := m["address"].(string); ok {
				return addr, nil
			}
		}
	}
	return "", fmt.Errorf("no usable channel on file for %s (the prototype speaks local-folder)", receiverID)
}

func (n *Node) archivedNorm(messageNumber string) (map[string]any, error) {
	raw, err := n.Home.ReadArchived(messageNumber)
	if err != nil {
		return nil, err
	}
	doc, err := supermessage.Parse(raw)
	if err != nil {
		return nil, err
	}
	return supermessage.Normalize(doc.M).(map[string]any), nil
}
