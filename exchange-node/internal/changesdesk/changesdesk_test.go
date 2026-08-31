package changesdesk

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
)

func specRulebook(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile("../../../spec/example-order.supermessage.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := supermessage.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	norm := supermessage.Normalize(doc.M).(map[string]any)
	return norm["rulebook"].(map[string]any)
}

// v4 = v3 + rule R-06: the diff must say exactly that, nothing more.
func TestDiffV3ToV4IsExactlyOneAddedRule(t *testing.T) {
	v3 := specRulebook(t)
	v4 := supermessage.DeepCopy(v3).(map[string]any)
	v4["version"] = int64(4)
	v4["rules"] = append(v4["rules"].([]any), map[string]any{
		"rule":           "R-06",
		"plain_language": "Every order line must state its quantity unit.",
		"machine_check":  "content.lines.all(l, has(l.quantity_unit))",
		"error_message":  "Line {line_number}: quantity unit missing (rule R-06).",
	})
	diff := DiffRulebooks(v3, v4)
	if len(diff) != 1 {
		t.Fatalf("expected exactly 1 diff entry, got %d: %v", len(diff), diff)
	}
	if diff[0].Change != "added" || diff[0].Item != "R-06" || diff[0].Section != "rules" {
		t.Fatalf("wrong diff entry: %+v", diff[0])
	}
}

// The policy loader's hard floor: no policy file may enable auto-apply.
func TestPolicyHardFloorRejectsAutoApply(t *testing.T) {
	home := store.Open(t.TempDir())
	if err := home.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	p := DefaultPolicy()
	p.AutoApplyEnabled = true
	if err := store.WriteJSON(home.File("policy", "review-policy.json"), p); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(home); err == nil {
		t.Fatal("a policy enabling auto-apply must be rejected at load")
	}
}

// Connection diffs treat absence as silence, never as removal, and always
// carry the out-of-band warning.
func TestConnectionDiff(t *testing.T) {
	oldCh := []any{map[string]any{"channel": "local-folder", "address": "/a"}}
	newCh := []any{map[string]any{"channel": "local-folder", "address": "/b"}}
	diff := DiffConnections(oldCh, newCh)
	if len(diff) != 1 || diff[0].Change != "changed" {
		t.Fatalf("expected one changed entry, got %v", diff)
	}
	if diff[0].Warning == "" {
		t.Fatal("connection changes must carry the confirm-out-of-band warning")
	}
	if len(DiffConnections(oldCh, []any{})) != 0 {
		t.Fatal("an absent channel is not a removal proposal")
	}
	var roundtrip []any
	b, _ := json.Marshal(oldCh)
	json.Unmarshal(b, &roundtrip)
	if len(DiffConnections(oldCh, roundtrip)) != 0 {
		t.Fatal("identical channels must produce no diff")
	}
}
