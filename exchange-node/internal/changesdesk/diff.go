package changesdesk

import (
	"fmt"
)

// DiffRulebooks compares two normalized rulebook trees and renders the
// changes in plain language, matched by stable ids (rule number, field name,
// code-list key, response type). Machine diffs classify structure, not
// meaning — so wording changes are always shown in full, never summarized.
func DiffRulebooks(old, new map[string]any) []DiffEntry {
	var out []DiffEntry
	out = append(out, diffKeyedList(old, new, "rules", "rule", describeRule)...)
	out = append(out, diffKeyedList(old, new, "fields", "field", describeField)...)
	out = append(out, diffKeyedList(old, new, "scenarios", "scenario", describeScenario)...)
	out = append(out, diffResponses(old, new)...)
	out = append(out, diffCodeLists(old, new)...)
	return out
}

func itemsByKey(rb map[string]any, listName, keyField string) map[string]map[string]any {
	out := map[string]map[string]any{}
	list, _ := rb[listName].([]any)
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			if k, ok := m[keyField].(string); ok {
				out[k] = m
			}
		}
	}
	return out
}

func diffKeyedList(old, new map[string]any, listName, keyField string,
	describe func(change string, oldItem, newItem map[string]any) (string, string)) []DiffEntry {
	oldItems := itemsByKey(old, listName, keyField)
	newItems := itemsByKey(new, listName, keyField)
	var out []DiffEntry
	// Keep the order of the new list for added/changed, then removals.
	list, _ := new[listName].([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m[keyField].(string)
		oldItem, existed := oldItems[key]
		if !existed {
			words, warn := describe("added", nil, m)
			out = append(out, DiffEntry{Section: listName, Change: "added", Item: key, InPlainWords: words, Warning: warn})
			continue
		}
		if !sameJSON(oldItem, m) {
			words, warn := describe("changed", oldItem, m)
			out = append(out, DiffEntry{Section: listName, Change: "changed", Item: key, InPlainWords: words, Warning: warn})
		}
	}
	for key, oldItem := range oldItems {
		if _, still := newItems[key]; !still {
			words, warn := describe("removed", oldItem, nil)
			out = append(out, DiffEntry{Section: listName, Change: "removed", Item: key, InPlainWords: words, Warning: warn})
		}
	}
	return out
}

func str(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	s, _ := m[k].(string)
	return s
}

func describeRule(change string, oldItem, newItem map[string]any) (string, string) {
	switch change {
	case "added":
		return fmt.Sprintf("New rule: %q", str(newItem, "plain_language")), ""
	case "removed":
		return fmt.Sprintf("Rule removed. It said: %q", str(oldItem, "plain_language")), ""
	default:
		oldCheck, newCheck := str(oldItem, "machine_check"), str(newItem, "machine_check")
		if oldCheck == newCheck {
			return fmt.Sprintf("Wording only (the check itself is unchanged). Old sentence: %q — new sentence: %q",
				str(oldItem, "plain_language"), str(newItem, "plain_language")), ""
		}
		return fmt.Sprintf("The check changed — read both sentences yourself. Old: %q — new: %q",
				str(oldItem, "plain_language"), str(newItem, "plain_language")),
			"The publisher did not explain this change. Ask them before approving."
	}
}

func describeField(change string, oldItem, newItem map[string]any) (string, string) {
	name := str(newItem, "field")
	if name == "" {
		name = str(oldItem, "field")
	}
	switch change {
	case "added":
		req, _ := newItem["required"].(bool)
		if req {
			return fmt.Sprintf("New REQUIRED field %q — tightening: you must now always send it. Meaning: %q", name, str(newItem, "meaning")), ""
		}
		return fmt.Sprintf("New optional field %q. Meaning: %q", name, str(newItem, "meaning")), ""
	case "removed":
		return fmt.Sprintf("Field %q was removed.", name), ""
	default:
		oldReq, _ := oldItem["required"].(bool)
		newReq, _ := newItem["required"].(bool)
		if oldReq != newReq {
			if newReq {
				return fmt.Sprintf("Field %q is now REQUIRED (it was optional) — tightening.", name), ""
			}
			return fmt.Sprintf("Field %q is now optional (it was required).", name), ""
		}
		if str(oldItem, "meaning") != str(newItem, "meaning") {
			return fmt.Sprintf("The MEANING of field %q changed — this is exactly what machines cannot judge; read both. Old: %q — new: %q",
					name, str(oldItem, "meaning"), str(newItem, "meaning")),
				"A meaning shift with unchanged structure. Confirm with the publisher what actually changed."
		}
		return fmt.Sprintf("Field %q changed (format or details). Old format: %q — new format: %q",
			name, str(oldItem, "format"), str(newItem, "format")), ""
	}
}

func describeScenario(change string, oldItem, newItem map[string]any) (string, string) {
	switch change {
	case "added":
		return fmt.Sprintf("New scenario: %q", str(newItem, "plain_language")), ""
	case "removed":
		return fmt.Sprintf("Scenario removed. It said: %q", str(oldItem, "plain_language")), ""
	default:
		return fmt.Sprintf("Scenario rewritten. Old: %q — new: %q",
			str(oldItem, "plain_language"), str(newItem, "plain_language")), ""
	}
}

func diffResponses(oldRB, newRB map[string]any) []DiffEntry {
	oldResp := responsesByType(oldRB)
	newResp := responsesByType(newRB)
	var out []DiffEntry
	for typ, newR := range newResp {
		oldR, existed := oldResp[typ]
		if !existed {
			out = append(out, DiffEntry{Section: "how_to_respond", Change: "added", Item: typ,
				InPlainWords: fmt.Sprintf("A new allowed response type: %q, deadline %q", typ, str(newR, "deadline"))})
			continue
		}
		if str(oldR, "deadline") != str(newR, "deadline") {
			out = append(out, DiffEntry{Section: "how_to_respond", Change: "changed", Item: typ,
				InPlainWords: fmt.Sprintf("Deadline for %q: %q → %q", typ, str(oldR, "deadline"), str(newR, "deadline"))})
		} else if !sameJSON(oldR, newR) {
			out = append(out, DiffEntry{Section: "how_to_respond", Change: "changed", Item: typ,
				InPlainWords: fmt.Sprintf("The construction of the %q response changed (copied fields or fill-in fields differ) — open both versions.", typ)})
		}
	}
	for typ := range oldResp {
		if _, still := newResp[typ]; !still {
			out = append(out, DiffEntry{Section: "how_to_respond", Change: "removed", Item: typ,
				InPlainWords: fmt.Sprintf("Response type %q is no longer allowed.", typ)})
		}
	}
	return out
}

// responsesByType works on either a rulebook that embeds how_to_respond or a
// whole message tree; the demo keeps how_to_respond beside the rulebook, so
// diffs read it from the carrying message when present.
func responsesByType(m map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	htr, ok := m["how_to_respond"].(map[string]any)
	if !ok {
		return out
	}
	list, _ := htr["allowed_responses"].([]any)
	for _, item := range list {
		if r, ok := item.(map[string]any); ok {
			if t, ok := r["response_type"].(string); ok {
				out[t] = r
			}
		}
	}
	return out
}

func diffCodeLists(old, new map[string]any) []DiffEntry {
	oldLists, _ := old["code_lists"].(map[string]any)
	newLists, _ := new["code_lists"].(map[string]any)
	var out []DiffEntry
	for listName, newVal := range newLists {
		newCodes, _ := newVal.(map[string]any)
		oldCodes, _ := oldLists[listName].(map[string]any)
		for code, meaning := range newCodes {
			if _, existed := oldCodes[code]; !existed {
				out = append(out, DiffEntry{Section: "code_lists", Change: "added", Item: listName + "." + code,
					InPlainWords: fmt.Sprintf("New allowed value %q in %s: %v", code, listName, meaning)})
			}
		}
		for code := range oldCodes {
			if _, still := newCodes[code]; !still {
				out = append(out, DiffEntry{Section: "code_lists", Change: "removed", Item: listName + "." + code,
					InPlainWords: fmt.Sprintf("Value %q in %s is no longer allowed.", code, listName)})
			}
		}
	}
	return out
}

// DiffConnections renders channel changes as strict old → new pairs.
func DiffConnections(oldChannels, newChannels []any) []DiffEntry {
	byName := func(chs []any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, c := range chs {
			if m, ok := c.(map[string]any); ok {
				if n, ok := m["channel"].(string); ok {
					out[n] = m
				}
			}
		}
		return out
	}
	oldBy, newBy := byName(oldChannels), byName(newChannels)
	const warning = "This changes where your messages go or how they are secured. Confirm through a channel other than this message (phone your contact) before approving."
	var out []DiffEntry
	for name, newCh := range newBy {
		oldCh, existed := oldBy[name]
		if !existed {
			out = append(out, DiffEntry{Section: "connections", Change: "added", Item: name,
				InPlainWords: fmt.Sprintf("New channel %q: %v", name, plainFields(newCh)), Warning: warning,
				RiskFlags: []string{"touches: channel"}})
			continue
		}
		for field, newVal := range newCh {
			if field == "channel" {
				continue
			}
			if oldVal, has := oldCh[field]; !has || fmt.Sprintf("%v", oldVal) != fmt.Sprintf("%v", newVal) {
				out = append(out, DiffEntry{Section: "connections", Change: "changed", Item: name + "." + field,
					InPlainWords: fmt.Sprintf("Channel %q, %s: %v → %v", name, field, oldCh[field], newVal),
					Warning:      warning, RiskFlags: []string{"touches: channel"}})
			}
		}
	}
	// A channel absent from the message is NOT a removal: section 5 is
	// informational, absence states nothing (spec).
	return out
}

func plainFields(m map[string]any) string {
	s := ""
	for k, v := range m {
		if k == "channel" || k == "_note" {
			continue
		}
		if s != "" {
			s += ", "
		}
		s += fmt.Sprintf("%s=%v", k, v)
	}
	return s
}

func sameJSON(a, b any) bool {
	return fmt.Sprintf("%#v", a) == fmt.Sprintf("%#v", b)
}
