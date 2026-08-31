// Package answerkey runs a rulebook's own example messages (section 6 of a
// supermessage) through the checker. Valid examples must pass; deliberately
// wrong examples must trigger exactly the rule IDs they promise. A rulebook
// that fails its own answer key is never trusted.
package answerkey

import (
	"fmt"
	"sort"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/checker"
)

type ExampleResult struct {
	Kind        string   `json:"kind"` // "valid" or "invalid"
	Description string   `json:"description"`
	Expected    []string `json:"expected_errors"`
	Got         []string `json:"got_errors"`
	Pass        bool     `json:"pass"`
}

type Report struct {
	RulebookID string          `json:"rulebook_id"`
	Examples   []ExampleResult `json:"examples"`
	AllPass    bool            `json:"all_pass"`
	Note       string          `json:"note,omitempty"`
}

// Run executes the answer key of a supermessage (normalized tree) against its
// own rulebook. The answer key may live beside the rulebook in the message.
func Run(answerKey map[string]any, rulebook map[string]any) Report {
	rep := Report{AllPass: true}
	if id, ok := rulebook["id"].(string); ok {
		rep.RulebookID = id
	}
	if answerKey == nil {
		rep.AllPass = false
		rep.Note = "this file carries no answer key — nothing proves the rules behave as written"
		return rep
	}
	docType := checker.BaseDocType(rulebook)

	for _, kind := range []string{"valid", "invalid"} {
		listKey := kind + "_examples"
		items, _ := answerKey[listKey].([]any)
		for _, item := range items {
			ex, ok := item.(map[string]any)
			if !ok {
				continue
			}
			desc, _ := ex["description"].(string)
			content, _ := ex["content"].(map[string]any)
			var expected []string
			if raw, ok := ex["expected_errors"].([]any); ok {
				for _, e := range raw {
					if s, ok := e.(string); ok {
						expected = append(expected, s)
					}
				}
			}
			sort.Strings(expected)

			result := ExampleResult{Kind: kind, Description: desc, Expected: expected}
			checked, err := checker.Check(content, rulebook, docType)
			if err != nil {
				result.Got = []string{fmt.Sprintf("checker error: %v", err)}
				result.Pass = false
			} else {
				result.Got = checked.ViolatedIDs()
				if kind == "valid" {
					result.Pass = len(result.Got) == 0
				} else {
					result.Pass = equalStringSets(result.Got, expected)
				}
			}
			if !result.Pass {
				rep.AllPass = false
			}
			rep.Examples = append(rep.Examples, result)
		}
	}
	if len(rep.Examples) == 0 {
		rep.AllPass = false
		rep.Note = "the answer key is empty — nothing proves the rules behave as written"
	}
	return rep
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
