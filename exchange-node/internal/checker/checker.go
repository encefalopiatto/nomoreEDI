// Package checker evaluates a rulebook's machine checks (CEL) against a
// message's content and reports broken rules by ID, with the rulebook's own
// plain-language sentences. Rules can only check values — CEL cannot act,
// reach the network, or touch files (safety rule 3 of the spec).
package checker

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"cel.dev/cel-go/cel"
)

// Rule is one entry of a rulebook's "rules" list.
type Rule struct {
	ID           string
	Plain        string
	MachineCheck string
	ErrorMessage string
	AppliesTo    string // empty = the rulebook's base document type
}

// Violation is one broken rule, rendered for humans.
type Violation struct {
	RuleID   string   `json:"rule"`
	Plain    string   `json:"plain_language"`
	Messages []string `json:"messages"`
}

// Skipped is a rule that does not apply to the document type being checked.
type Skipped struct {
	RuleID    string `json:"rule"`
	Plain     string `json:"plain_language"`
	AppliesTo string `json:"applies_to"`
}

// Result of checking one content tree against one rulebook.
type Result struct {
	CheckedRules []string    `json:"checked_rules"`
	SkippedRules []Skipped   `json:"skipped_rules"`
	Violations   []Violation `json:"violations"`
}

func (r Result) Passed() bool { return len(r.Violations) == 0 }

// ViolatedIDs returns the sorted set of broken rule IDs.
func (r Result) ViolatedIDs() []string {
	ids := make([]string, 0, len(r.Violations))
	for _, v := range r.Violations {
		ids = append(ids, v.RuleID)
	}
	sort.Strings(ids)
	return ids
}

// ExtractRules reads the rules list out of a (normalized) rulebook tree.
func ExtractRules(rulebook map[string]any) []Rule {
	raw, _ := rulebook["rules"].([]any)
	rules := make([]Rule, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		get := func(k string) string { s, _ := m[k].(string); return s }
		rules = append(rules, Rule{
			ID:           get("rule"),
			Plain:        get("plain_language"),
			MachineCheck: get("machine_check"),
			ErrorMessage: get("error_message"),
			AppliesTo:    get("applies_to"),
		})
	}
	return rules
}

// BaseDocType is the document type a rulebook's applies_to-less rules govern.
func BaseDocType(rulebook map[string]any) string {
	if s, ok := rulebook["applies_to_default"].(string); ok && s != "" {
		return s
	}
	return "order"
}

var lineAllPattern = regexp.MustCompile(`(?s)^content\.lines\.all\((\w+),\s*(.+)\)$`)

// Check runs every applicable rule of the rulebook against normalized content.
// docType selects which rules apply: a rule with applies_to runs only when it
// matches; a rule without applies_to runs for the rulebook's base type.
func Check(content map[string]any, rulebook map[string]any, docType string) (Result, error) {
	res := Result{}
	base := BaseDocType(rulebook)
	env, err := cel.NewEnv(
		cel.Variable("content", cel.DynType),
		cel.CrossTypeNumericComparisons(true),
	)
	if err != nil {
		return res, err
	}

	for _, rule := range ExtractRules(rulebook) {
		applies := rule.AppliesTo
		if applies == "" {
			applies = base
		}
		if applies != docType {
			res.SkippedRules = append(res.SkippedRules, Skipped{RuleID: rule.ID, Plain: rule.Plain, AppliesTo: applies})
			continue
		}
		res.CheckedRules = append(res.CheckedRules, rule.ID)

		ok, evalErr := evalBool(env, rule.MachineCheck, map[string]any{"content": content})
		if evalErr != nil {
			// A rule that cannot be evaluated cannot be shown to hold: fail it,
			// and say why in the message.
			res.Violations = append(res.Violations, Violation{
				RuleID: rule.ID,
				Plain:  rule.Plain,
				Messages: []string{fmt.Sprintf("%s (the check could not be evaluated: %v)",
					strings.ReplaceAll(rule.ErrorMessage, "{line_number}", "?"), evalErr)},
			})
			continue
		}
		if ok {
			continue
		}
		res.Violations = append(res.Violations, Violation{
			RuleID:   rule.ID,
			Plain:    rule.Plain,
			Messages: renderMessages(rule, content),
		})
	}
	return res, nil
}

// evalBool compiles and runs one CEL expression, expecting a boolean.
func evalBool(env *cel.Env, expr string, input map[string]any) (bool, error) {
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return false, iss.Err()
	}
	prg, err := env.Program(ast)
	if err != nil {
		return false, err
	}
	out, _, err := prg.Eval(input)
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("the check did not produce a yes/no answer")
	}
	return b, nil
}

// renderMessages fills the rulebook's own error_message template. When the
// check has the shape content.lines.all(v, predicate), the predicate is
// re-evaluated per line so {line_number} names the actual failing lines.
// This is a rendering courtesy, not spec machinery.
func renderMessages(rule Rule, content map[string]any) []string {
	m := lineAllPattern.FindStringSubmatch(strings.TrimSpace(rule.MachineCheck))
	if m == nil {
		return []string{strings.ReplaceAll(rule.ErrorMessage, "{line_number}", "?")}
	}
	varName, pred := m[1], m[2]
	lines, _ := content["lines"].([]any)
	env, err := cel.NewEnv(
		cel.Variable("content", cel.DynType),
		cel.Variable(varName, cel.DynType),
		cel.CrossTypeNumericComparisons(true),
	)
	if err != nil {
		return []string{strings.ReplaceAll(rule.ErrorMessage, "{line_number}", "?")}
	}
	var msgs []string
	for i, line := range lines {
		ok, evalErr := evalBool(env, pred, map[string]any{"content": content, varName: line})
		if evalErr == nil && ok {
			continue
		}
		lineNo := fmt.Sprintf("%d", i+1)
		if lm, isMap := line.(map[string]any); isMap {
			if n, has := lm["line_number"]; has {
				lineNo = fmt.Sprintf("%v", n)
			}
		}
		msgs = append(msgs, strings.ReplaceAll(rule.ErrorMessage, "{line_number}", lineNo))
	}
	if len(msgs) == 0 {
		msgs = []string{strings.ReplaceAll(rule.ErrorMessage, "{line_number}", "?")}
	}
	return msgs
}
