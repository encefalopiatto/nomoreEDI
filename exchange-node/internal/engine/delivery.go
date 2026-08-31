package engine

import (
	"fmt"
	"os"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/transport"
)

// Delivery is the honest record of one outbound message's journey
// (outbox/queue/<number>.json). "Delivered" is only ever set when the
// partner's node sent back a signed receipt — never when a file merely
// left this machine.
const (
	DeliveryQueued     = "queued"      // created, no attempt yet
	DeliveryHandedOver = "handed_over" // the transport accepted it; awaiting the partner's receipt
	DeliveryDelivered  = "delivered"   // the partner's signed receipt arrived
	DeliveryFailed     = "failed"      // last attempt failed; a retry is scheduled
	DeliveryDeadLetter = "dead_letter" // gave up after repeated failures; an operator must act
)

type DeliveryAttempt struct {
	At      string `json:"at"`
	Channel string `json:"channel"`
	Outcome string `json:"outcome"` // handed_over | failed
	Detail  string `json:"detail,omitempty"`
}

type Delivery struct {
	MessageNumber string            `json:"message_number"`
	To            string            `json:"to_company_id"`
	ToName        string            `json:"to_name"`
	Type          string            `json:"message_type"`
	State         string            `json:"state"`
	Attempts      []DeliveryAttempt `json:"attempts"`
	CreatedAt     string            `json:"created_at"`
	NextRetryAt   string            `json:"next_retry_at,omitempty"`
	HandedOverAt  string            `json:"handed_over_at,omitempty"`
	DeliveredAt   string            `json:"delivered_at,omitempty"`
	ReceiptNumber string            `json:"receipt_message,omitempty"`
	LastError     string            `json:"last_error,omitempty"`
	ResentBy      string            `json:"resent_by,omitempty"`
}

// Retry waits between attempts: quick at first, then patient.
var retryAfter = []time.Duration{
	1 * time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour,
}

func (n *Node) deliveryPath(msgNo string) string {
	return n.Home.File("outbox", "queue", store.SafeName(msgNo)+".json")
}

func (n *Node) loadDelivery(msgNo string) (*Delivery, bool) {
	var d Delivery
	if store.ReadJSON(n.deliveryPath(msgNo), &d) != nil {
		return nil, false
	}
	return &d, true
}

func (n *Node) saveDelivery(d *Delivery) {
	store.WriteJSON(n.deliveryPath(d.MessageNumber), d)
}

// ListDeliveries returns every outbound delivery record, newest first.
func (n *Node) ListDeliveries() []*Delivery {
	names := n.Home.ListDir("outbox", "queue")
	out := make([]*Delivery, 0, len(names))
	for i := len(names) - 1; i >= 0; i-- {
		var d Delivery
		if store.ReadJSON(n.Home.File("outbox", "queue", names[i]), &d) == nil {
			out = append(out, &d)
		}
	}
	return out
}

// enqueueDelivery files the record for a freshly signed and archived message.
func (n *Node) enqueueDelivery(msgNo, to, toName, msgType string) *Delivery {
	d := &Delivery{
		MessageNumber: msgNo, To: to, ToName: toName, Type: msgType,
		State: DeliveryQueued, CreatedAt: n.Now().UTC().Format(time.RFC3339),
	}
	n.saveDelivery(d)
	return d
}

// TryDeliver makes one delivery attempt now and schedules the next retry on
// failure. It reads the archived bytes — the file that travels is always the
// exact file that was signed.
func (n *Node) TryDeliver(msgNo string) *Delivery {
	d, ok := n.loadDelivery(msgNo)
	if !ok {
		return nil
	}
	if d.State == DeliveryDelivered {
		return d
	}
	raw, err := n.Home.ReadArchived(msgNo)
	if err != nil {
		n.recordAttempt(d, "", "failed", "the archived bytes are missing: "+err.Error())
		return d
	}
	channels, has := n.Trust.Connections(d.To)
	if !has {
		n.recordAttempt(d, "", "failed", "no connection details on file for this partner yet")
		return d
	}
	channel, err := transport.PickChannel(channels)
	if err != nil {
		n.recordAttempt(d, "", "failed", err.Error())
		return d
	}
	fileName := store.SafeName(msgNo) + ".supermessage.json"
	if err := transport.Deliver(channel, fileName, raw); err != nil {
		n.recordAttempt(d, channel.Kind(), "failed", err.Error())
		return d
	}
	n.recordAttempt(d, channel.Kind(), "handed_over", "")
	return d
}

func (n *Node) recordAttempt(d *Delivery, channel, outcome, detail string) {
	now := n.Now().UTC()
	d.Attempts = append(d.Attempts, DeliveryAttempt{
		At: now.Format(time.RFC3339), Channel: channel, Outcome: outcome, Detail: detail,
	})
	if outcome == "handed_over" {
		d.State = DeliveryHandedOver
		d.HandedOverAt = now.Format(time.RFC3339)
		d.NextRetryAt = ""
		d.LastError = ""
		// Receipts are never receipted (that would never end), so for a
		// receipt the hand-over is the end of the line.
		if d.Type == "delivery_receipt" {
			d.State = DeliveryDelivered
			d.DeliveredAt = d.HandedOverAt
		}
		n.Home.Log(d.MessageNumber, "delivery", "handed_over", "via "+channel)
	} else {
		d.LastError = detail
		failures := 0
		for _, a := range d.Attempts {
			if a.Outcome == "failed" {
				failures++
			}
		}
		if failures > len(retryAfter) {
			d.State = DeliveryDeadLetter
			d.NextRetryAt = ""
			n.Home.Log(d.MessageNumber, "delivery", "dead_letter", detail)
			n.Audit.Append("delivery_gave_up", map[string]any{
				"message_number": d.MessageNumber, "partner": d.To, "last_error": detail,
			})
		} else {
			d.State = DeliveryFailed
			d.NextRetryAt = now.Add(retryAfter[failures-1]).Format(time.RFC3339)
			n.Home.Log(d.MessageNumber, "delivery", "failed", detail+" — retrying "+d.NextRetryAt)
		}
	}
	n.saveDelivery(d)
}

// ProcessQueue retries every delivery whose scheduled time has come. The
// pollers call this every tick; approving a proposal calls it immediately,
// because an approval often unblocks a route.
func (n *Node) ProcessQueue() int {
	now := n.Now().UTC()
	retried := 0
	for _, d := range n.ListDeliveries() {
		if d.State != DeliveryFailed || d.NextRetryAt == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, d.NextRetryAt); err == nil && !now.Before(t) {
			n.TryDeliver(d.MessageNumber)
			retried++
		}
	}
	return retried
}

// flushFailed retries every failed delivery immediately, schedule aside —
// used when an approval just created the route queued messages wait for.
func (n *Node) flushFailed() {
	for _, d := range n.ListDeliveries() {
		if d.State == DeliveryFailed {
			n.TryDeliver(d.MessageNumber)
		}
	}
}

// RetryDeliveryNow is the operator's button: one attempt right now,
// whatever the schedule says — also for dead letters.
func (n *Node) RetryDeliveryNow(msgNo string) (*Delivery, error) {
	d, ok := n.loadDelivery(msgNo)
	if !ok {
		return nil, fmt.Errorf("no delivery record for %s", msgNo)
	}
	if d.State == DeliveryDelivered {
		return d, fmt.Errorf("%s is already delivered and confirmed", msgNo)
	}
	if d.State == DeliveryDeadLetter {
		d.State = DeliveryFailed // an operator's retry revives a dead letter
		n.saveDelivery(d)
	}
	return n.TryDeliver(msgNo), nil
}

// ResendDelivery re-sends the exact archived bytes as a fresh attempt, on an
// operator's explicit request. Safe by design: the partner's node ignores a
// message number it has already processed, so the business effect happens
// once even if the file arrives twice.
func (n *Node) ResendDelivery(msgNo, operator string) (*Delivery, error) {
	d, ok := n.loadDelivery(msgNo)
	if !ok {
		return nil, fmt.Errorf("no delivery record for %s", msgNo)
	}
	d.ResentBy = operator
	d.State = DeliveryFailed
	n.saveDelivery(d)
	n.Audit.Append("delivery_resent", map[string]any{
		"message_number": msgNo, "partner": d.To, "actor": operator,
	})
	return n.TryDeliver(msgNo), nil
}

// RetryAllFailed retries every failed and dead-letter delivery, returning a
// per-message outcome — the bulk button behind a confirmation.
func (n *Node) RetryAllFailed(operator string) map[string]string {
	results := map[string]string{}
	for _, d := range n.ListDeliveries() {
		if d.State != DeliveryFailed && d.State != DeliveryDeadLetter {
			continue
		}
		if d.State == DeliveryDeadLetter {
			d.State = DeliveryFailed
			n.saveDelivery(d)
		}
		after := n.TryDeliver(d.MessageNumber)
		results[d.MessageNumber] = after.State
	}
	if len(results) > 0 {
		n.Audit.Append("bulk_retry", map[string]any{"actor": operator, "count": len(results)})
	}
	return results
}

// sendReceipt tells the sender their file arrived and its signature
// verified. This is the end-to-end acknowledgement every transport shares —
// "delivered" on the sender's side means exactly this receipt.
func (n *Node) sendReceipt(senderID, senderName, aboutMsgNo string) {
	n.sendNote(senderID, "delivery_receipt", map[string]any{
		"regarding": aboutMsgNo,
		"result":    "received_and_verified",
		"note":      "Your file arrived and its signature checked out. This receipt is what 'delivered' means.",
	})
	_ = senderName
}

// handleReceipt marks the referenced outbound message as delivered.
func (n *Node) handleReceipt(receiptNo string, content map[string]any) string {
	regarding, _ := content["regarding"].(string)
	d, ok := n.loadDelivery(regarding)
	if !ok {
		n.Home.Log(receiptNo, "receipt", "unmatched", "no delivery record for "+regarding)
		return "receipt noted (no matching delivery record)"
	}
	d.State = DeliveryDelivered
	d.DeliveredAt = n.Now().UTC().Format(time.RFC3339)
	d.ReceiptNumber = receiptNo
	d.NextRetryAt = ""
	n.saveDelivery(d)
	n.Home.Log(regarding, "delivery", "delivered", "receipt "+receiptNo)
	return "delivery of " + regarding + " confirmed"
}

// ReprocessInbound runs an archived inbound message through validation
// again — same bytes, current trusted rulebooks. Useful after a rulebook
// decision changed what should happen. The original is never modified.
func (n *Node) ReprocessInbound(msgNo string) (string, error) {
	raw, err := n.Home.ReadArchived(msgNo)
	if err != nil {
		return "", fmt.Errorf("%s is not in the archive", msgNo)
	}
	if _, err := os.Stat(n.Home.File("archive", "in-"+store.SafeName(msgNo)+".supermessage.json")); err != nil {
		return "", fmt.Errorf("%s was not an inbound message", msgNo)
	}
	doc, err := parseVerified(n, raw)
	if err != nil {
		return "", err
	}
	os.Remove(n.Home.File("inbox", store.SafeName(msgNo)+".json"))
	n.Home.Log(msgNo, "reprocessed", "ok", "operator asked for a fresh check of the archived bytes")
	n.Audit.Append("message_reprocessed", map[string]any{"message_number": msgNo})
	return n.continuePipeline("reprocess-"+msgNo, doc, raw), nil
}

// deadlineInfo captures the response deadline an inbound message carries.
type deadlineInfo struct {
	Display string
	DueAt   string
}

func firstDeadline(norm map[string]any) *deadlineInfo {
	htr, _ := norm["how_to_respond"].(map[string]any)
	if htr == nil {
		return nil
	}
	list, _ := htr["allowed_responses"].([]any)
	about, _ := norm["about"].(map[string]any)
	sentAt, _ := about["sent_at"].(string)
	for _, item := range list {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		deadline, _ := r["deadline"].(string)
		var hours int
		if _, err := fmt.Sscanf(deadline, "within %d hours", &hours); err == nil && hours > 0 {
			if t, err := time.Parse(time.RFC3339, sentAt); err == nil {
				return &deadlineInfo{Display: deadline, DueAt: t.Add(time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339)}
			}
		}
		if deadline != "" {
			return &deadlineInfo{Display: deadline}
		}
	}
	return nil
}

// markResponded flags the original inbound message as answered, so the
// overdue alert lets go of it.
func (n *Node) markResponded(originalMsgNo string) {
	path := n.Home.File("inbox", store.SafeName(originalMsgNo)+".json")
	var rec InboxRecord
	if store.ReadJSON(path, &rec) != nil {
		return
	}
	rec.Responded = true
	store.WriteJSON(path, rec)
}

// parseVerified re-parses archived bytes and re-checks the file signature —
// reprocessing trusts the archive no more than it trusts the wire.
func parseVerified(n *Node, raw []byte) (*supermessage.Doc, error) {
	doc, err := supermessage.Parse(raw)
	if err != nil {
		return nil, err
	}
	about, _ := doc.Section("about")
	senderID := supermessage.GetString(about, "sender", "company_id")
	pub, err := n.Dir.LookUp(senderID)
	if err != nil {
		return nil, err
	}
	signingBytes, err := supermessage.FileSigningBytes(doc.M)
	if err != nil {
		return nil, err
	}
	if !sign.Verify(pub, signingBytes, supermessage.GetString(about, "signature", "value")) {
		return nil, fmt.Errorf("the archived file no longer verifies — do not trust this archive copy")
	}
	return doc, nil
}
