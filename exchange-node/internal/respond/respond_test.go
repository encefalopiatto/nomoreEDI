package respond

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
)

func specNorm(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile("../../../spec/example-order.supermessage.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := supermessage.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return supermessage.Normalize(doc.M).(map[string]any)
}

func buildDraft(t *testing.T) (*Draft, map[string]any) {
	t.Helper()
	orig := specNorm(t)
	d, err := Build(orig, "order_response", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return d, orig
}

// The echoes the choreography dictates are pre-filled and locked.
func TestDraftEchoesAndHoles(t *testing.T) {
	d, _ := buildDraft(t)
	if got, _ := getPath(d.Content, "order_reference"); got != "ORD-2026-88112" {
		t.Fatalf("order_reference must echo the order number, got %v", got)
	}
	if got, _ := getPath(d.Content, "lines[1].line_number"); got != int64(2) {
		t.Fatalf("line numbers must be echoed, got %v", got)
	}
	foundDecisionHole := false
	for _, h := range d.Holes {
		if h.Path == "lines[0].decision" {
			foundDecisionHole = true
			if len(h.Allowed) != 3 {
				t.Fatalf("decision hole must carry the code list, got %v", h.Allowed)
			}
		}
	}
	if !foundDecisionHole {
		t.Fatal("expected a decision hole per line")
	}
}

func fillOK(d *Draft) {
	ApplyFills(d, map[string]any{
		"lines[0].decision":           "accept",
		"lines[1].decision":           "change",
		"lines[1].confirmed_quantity": 60,
		"lines[0].confirmed_quantity": "", // not needed when accepting
		"confirmed_delivery_date":     "2026-09-01",
	})
}

// A correctly filled draft finishes; three classic mistakes must not.
func TestFinishValidation(t *testing.T) {
	d, orig := buildDraft(t)
	fillOK(d)
	if _, err := Finish(d, orig, nil); err != nil {
		t.Fatalf("a correct response must finish: %v", err)
	}

	// Mutated echo: editing a copied field is refused.
	d2, _ := buildDraft(t)
	fillOK(d2)
	setPath(d2.Content, "order_reference", "ORD-FORGED")
	if _, err := Finish(d2, orig, nil); err == nil || !strings.Contains(err.Error(), "echo") {
		t.Fatalf("a mutated echo must be refused, got %v", err)
	}

	// A decision outside the code list is refused.
	d3, _ := buildDraft(t)
	fillOK(d3)
	setPath(d3.Content, "lines[0].decision", "maybe")
	if _, err := Finish(d3, orig, nil); err == nil || !strings.Contains(err.Error(), "allowed values") {
		t.Fatalf("an out-of-code-list decision must be refused, got %v", err)
	}

	// "change" without a confirmed quantity is refused.
	d4, _ := buildDraft(t)
	ApplyFills(d4, map[string]any{
		"lines[0].decision":           "accept",
		"lines[0].confirmed_quantity": "",
		"lines[1].decision":           "change",
		"lines[1].confirmed_quantity": "",
		"confirmed_delivery_date":     "2026-09-01",
	})
	if _, err := Finish(d4, orig, nil); err == nil || !strings.Contains(err.Error(), "confirmed_quantity") {
		t.Fatalf("'change' without confirmed_quantity must be refused, got %v", err)
	}
}
