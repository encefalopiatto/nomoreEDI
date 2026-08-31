package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/trust"
)

// twoNodes builds a sender and receiver that know each other's keys; the
// sender has the receiver's folder address on file, the receiver does not
// (yet) know the sender as a partner.
func twoNodes(t *testing.T) (*Node, *Node) {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "directory.json")
	a, err := Init(filepath.Join(base, "a"), "Company A", "GLN 1", dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Init(filepath.Join(base, "b"), "Company B", "GLN 2", dir)
	if err != nil {
		t.Fatal(err)
	}
	pinPartner(t, a, b)
	pinPartner(t, b, a)
	return a, b
}

func pinPartner(t *testing.T, on, them *Node) {
	t.Helper()
	if err := store.WriteJSON(on.Home.File("trusted", "partners", store.SafeName(them.Identity.CompanyID)+".json"),
		trust.Partner{Name: them.Identity.Name, CompanyID: them.Identity.CompanyID, KeyFingerprint: "test", FirstSeen: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(on.Home.File("trusted", "connections", store.SafeName(them.Identity.CompanyID)+".json"),
		map[string]any{"channels": []any{map[string]any{
			"channel": "local-folder", "address": them.Home.File("transport", "in"),
		}}}); err != nil {
		t.Fatal(err)
	}
}

// A note travels, the receiver's signed receipt comes back, and only then
// does the sender's record say "delivered".
func TestDeliveryConfirmedByReceipt(t *testing.T) {
	a, b := twoNodes(t)
	a.sendNote(b.Identity.CompanyID, "configuration_response", map[string]any{"note": "hello"})

	deliveries := a.ListDeliveries()
	if len(deliveries) != 1 || deliveries[0].State != DeliveryHandedOver {
		t.Fatalf("after the first attempt the state must be handed_over, got %+v", deliveries)
	}
	msgNo := deliveries[0].MessageNumber

	b.ReceiveAll() // receives the note, sends the receipt
	a.ReceiveAll() // receives the receipt

	d, ok := a.loadDelivery(msgNo)
	if !ok || d.State != DeliveryDelivered {
		t.Fatalf("the receipt must flip the record to delivered, got %+v", d)
	}
	if d.ReceiptNumber == "" {
		t.Fatal("the record must name the receipt that confirmed it")
	}
	// Receipts stay out of the inbox — they are bookkeeping, not business.
	for _, name := range a.Home.ListDir("inbox") {
		if strings.Contains(name, "RCPT") {
			t.Fatal("a receipt must not appear as an inbox message")
		}
	}
}

// Without a route the delivery keeps retrying on schedule and eventually
// gives up as a dead letter — which an operator's retry can revive.
func TestDeliveryRetrySchedule(t *testing.T) {
	a, _ := twoNodes(t)
	clock := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return clock }

	// GLN 3 is nobody: no partner record, no route.
	a.sendNote("GLN 3", "configuration_response", map[string]any{"note": "into the void"})
	d := a.ListDeliveries()[0]
	if d.State != DeliveryFailed || d.NextRetryAt == "" {
		t.Fatalf("no route must mean failed-with-retry, got %+v", d)
	}

	// Before the scheduled time nothing happens; after it, another attempt.
	if n := a.ProcessQueue(); n != 0 {
		t.Fatalf("the queue must respect the schedule, retried %d early", n)
	}
	for i := 0; i < 10; i++ {
		clock = clock.Add(7 * time.Hour)
		a.ProcessQueue()
	}
	d2, _ := a.loadDelivery(d.MessageNumber)
	if d2.State != DeliveryDeadLetter {
		t.Fatalf("repeated failures must end as dead_letter, got %s after %d attempts", d2.State, len(d2.Attempts))
	}

	attemptsBefore := len(d2.Attempts)
	if _, err := a.RetryDeliveryNow(d.MessageNumber); err != nil {
		t.Fatalf("an operator must be able to revive a dead letter: %v", err)
	}
	d3, _ := a.loadDelivery(d.MessageNumber)
	if len(d3.Attempts) != attemptsBefore+1 {
		t.Fatal("the operator's retry must actually make an attempt")
	}
	// The route is still broken, so going back to dead_letter is honest.
}

// Reprocessing runs the archived bytes through validation again without
// touching the original.
func TestReprocessInbound(t *testing.T) {
	a, b := twoNodes(t)
	a.sendNote(b.Identity.CompanyID, "configuration_response", map[string]any{"note": "check me"})
	b.ReceiveAll()

	var msgNo string
	for _, name := range b.Home.ListDir("inbox") {
		var rec InboxRecord
		if store.ReadJSON(b.Home.File("inbox", name), &rec) == nil && rec.Type == "configuration_response" {
			msgNo = rec.MessageNumber
		}
	}
	if msgNo == "" {
		t.Fatal("the note never reached the inbox")
	}
	archivedBefore, err := b.Home.ReadArchived(msgNo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReprocessInbound(msgNo); err != nil {
		t.Fatalf("reprocess failed: %v", err)
	}
	archivedAfter, _ := b.Home.ReadArchived(msgNo)
	if string(archivedBefore) != string(archivedAfter) {
		t.Fatal("reprocessing must never touch the archived original")
	}
	var rec InboxRecord
	if err := store.ReadJSON(b.Home.File("inbox", store.SafeName(msgNo)+".json"), &rec); err != nil {
		t.Fatal("the inbox record must exist again after reprocessing")
	}
}
