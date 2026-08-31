package answerkey

import (
	"os"
	"testing"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
)

// The spec's worked example is the ground truth: its valid example must pass
// and its invalid examples must trigger exactly the promised rule IDs.
func TestSpecExampleAnswerKey(t *testing.T) {
	b, err := os.ReadFile("../../../spec/example-order.supermessage.json")
	if err != nil {
		t.Fatalf("cannot read spec example: %v", err)
	}
	doc, err := supermessage.Parse(b)
	if err != nil {
		t.Fatalf("cannot parse spec example: %v", err)
	}
	norm := supermessage.Normalize(doc.M).(map[string]any)
	rulebook, _ := norm["rulebook"].(map[string]any)
	answerKey, _ := norm["answer_key"].(map[string]any)
	if rulebook == nil || answerKey == nil {
		t.Fatal("spec example is missing rulebook or answer_key")
	}

	rep := Run(answerKey, rulebook)
	for _, ex := range rep.Examples {
		t.Logf("%-7s %-55q expected=%v got=%v pass=%v", ex.Kind, ex.Description, ex.Expected, ex.Got, ex.Pass)
	}
	if !rep.AllPass {
		t.Fatal("the spec example's answer key does not pass — checker and spec have drifted")
	}
}
