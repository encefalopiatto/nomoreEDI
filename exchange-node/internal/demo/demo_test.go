package demo

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/audit"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/engine"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
)

// The whole demo, headless, asserting the exact end state of both nodes.
func TestDemoReplay(t *testing.T) {
	spec, err := FindSpec()
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), "run")
	d, err := Run(runDir, spec, true, io.Discard)
	if err != nil {
		t.Fatalf("demo failed: %v", err)
	}
	supplier := d.Supplier.Node
	retailer := d.Retailer.Node

	// v3 and v4 are trusted, v3 with an overlap end date.
	rt, ok := supplier.Trust.RulebookTrust("nordkauf.order.fresh-food")
	if !ok {
		t.Fatal("supplier has no trust record for the rulebook")
	}
	versions := map[int]bool{}
	for _, tv := range rt.Accepting {
		versions[tv.Version] = true
		if tv.Version == 3 && tv.AlsoAcceptedUntil == "" {
			t.Error("v3 must carry an overlap end date after v4 was approved")
		}
	}
	if !versions[3] || !versions[4] {
		t.Fatalf("supplier must trust v3 and v4, has %v", versions)
	}

	// The connection change was applied: route now points at .../transport/in2.
	channels, ok := supplier.Trust.Connections("GLN 4099999000015")
	if !ok || len(channels) == 0 {
		t.Fatal("supplier lost the retailer's connection details")
	}
	addr, _ := channels[0].(map[string]any)["address"].(string)
	if !strings.HasSuffix(addr, filepath.Join("transport", "in2")) {
		t.Fatalf("connection change was not applied, address is %s", addr)
	}

	// Inbox statuses: 88112/88114/88115 green, 88113 red with exactly R-01+R-02.
	wantStatus := map[string]string{
		"ORD-2026-88112": "green", "ORD-2026-88113": "red",
		"ORD-2026-88114": "green", "ORD-2026-88115": "green",
	}
	for msg, want := range wantStatus {
		var rec engine.InboxRecord
		if err := store.ReadJSON(supplier.Home.File("inbox", msg+".json"), &rec); err != nil {
			t.Fatalf("missing inbox record %s: %v", msg, err)
		}
		if rec.Status != want {
			t.Errorf("%s: status %s, want %s", msg, rec.Status, want)
		}
		if msg == "ORD-2026-88113" {
			var ids []string
			for _, v := range rec.Violations {
				ids = append(ids, v.RuleID)
			}
			if strings.Join(ids, ",") != "R-01,R-02" {
				t.Errorf("88113 must fail with exactly R-01,R-02, got %v", ids)
			}
		}
	}

	// The fraud file is quarantined with its reason, and created no proposal.
	if _, err := os.Stat(supplier.Home.File("inbox", "quarantine", "ORD-2026-99999.supermessage.json")); err != nil {
		t.Error("the fraud file must sit in quarantine")
	}
	if pending := supplier.Desk.Queue.ListPending(); len(pending) != 0 {
		t.Errorf("nothing may remain pending (the fraud must not propose), got %d", len(pending))
	}

	// The retailer received a choreography-valid order response.
	var resp engine.InboxRecord
	found := false
	for _, name := range retailer.Home.ListDir("inbox") {
		if store.ReadJSON(retailer.Home.File("inbox", name), &resp) == nil && resp.Type == "order_response" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("retailer never received the order response")
	}
	if !strings.Contains(resp.EchoCheck, "echoes verified") {
		t.Errorf("the response's echoes must verify, got %q", resp.EchoCheck)
	}

	// The delivery spine: the retailer's first order must be confirmed
	// delivered by the supplier's signed receipt, and the supplier's order
	// response must be confirmed by the retailer's.
	assertDelivered := func(n *engine.Node, byNumber, byType string) {
		for _, d := range n.ListDeliveries() {
			if (byNumber != "" && d.MessageNumber == byNumber) || (byType != "" && d.Type == byType) {
				if d.State != engine.DeliveryDelivered {
					t.Errorf("%s (%s) from %s must be delivered-and-confirmed, is %s",
						d.MessageNumber, d.Type, n.Identity.Name, d.State)
				}
				return
			}
		}
		t.Errorf("no delivery record found on %s for %s%s", n.Identity.Name, byNumber, byType)
	}
	assertDelivered(retailer, "ORD-2026-88112", "")
	assertDelivered(supplier, "", "order_response")

	// Both audit chains are intact.
	for _, n := range []*engine.Node{supplier, retailer} {
		if err := audit.Open(n.Home.File("audit", "audit-log.jsonl")).Verify(); err != nil {
			t.Errorf("audit chain broken for %s: %v", n.Identity.Name, err)
		}
	}
}
