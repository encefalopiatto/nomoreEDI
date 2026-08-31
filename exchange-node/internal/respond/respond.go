// Package respond builds answers the way the original message dictates:
// fields the choreography says must be copied are pre-filled and locked,
// fields the responder decides are typed holes limited to the allowed codes.
// Finishing a draft proves the choreography was honored before anything is
// signed or sent.
package respond

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Hole is one field the responder must fill in.
type Hole struct {
	Path    string   `json:"path"`
	Note    string   `json:"note,omitempty"`
	Allowed []string `json:"allowed,omitempty"`
}

// Draft is a response under construction (outbox/drafts/<id>.json).
type Draft struct {
	DraftID      string         `json:"draft_id"`
	ResponseType string         `json:"response_type"`
	BasedOn      string         `json:"based_on"`
	PartnerID    string         `json:"partner_company_id"`
	PartnerName  string         `json:"partner_name"`
	Deadline     string         `json:"deadline,omitempty"`
	Content      map[string]any `json:"content"`
	LockedPaths  []string       `json:"locked_paths"`
	Holes        []Hole         `json:"holes"`
}

const fillMark = "<<FILL"

// Build creates a draft from the archived original message (normalized tree)
// for one of its allowed response types.
func Build(original map[string]any, responseType string, now time.Time) (*Draft, error) {
	about, _ := original["about"].(map[string]any)
	content, _ := original["content"].(map[string]any)
	spec, err := responseSpec(original, responseType)
	if err != nil {
		return nil, err
	}
	msgNo, _ := about["message_number"].(string)
	sender, _ := about["sender"].(map[string]any)

	d := &Draft{
		DraftID:      "draft-" + strings.ToLower(strings.ReplaceAll(responseType, "_", "-")) + "-" + sanitize(msgNo),
		ResponseType: responseType,
		BasedOn:      msgNo,
		Content:      map[string]any{},
	}
	if sender != nil {
		d.PartnerID, _ = sender["company_id"].(string)
		d.PartnerName, _ = sender["name"].(string)
	}
	d.Deadline = computeDeadline(spec, about, now)

	// The echoes: copy exactly what the original says to copy, then lock it.
	for _, mc := range listOfMaps(spec, "must_copy") {
		from, _ := mc["from"].(string)
		to, _ := mc["to"].(string)
		if err := copyPath(original, content, from, d.Content, to); err != nil {
			return nil, fmt.Errorf("cannot copy %q to %q: %w", from, to, err)
		}
		d.LockedPaths = append(d.LockedPaths, expandTargets(d.Content, to)...)
	}

	// The holes: everything the responder decides, typed and bounded.
	rulebook, _ := original["rulebook"].(map[string]any)
	for _, fi := range listOfMaps(spec, "you_fill_in") {
		field, _ := fi["field"].(string)
		note, _ := fi["note"].(string)
		var allowed []string
		if ref, ok := fi["allowed_values_from"].(string); ok {
			allowed = codeListValues(rulebook, ref)
		}
		for _, path := range expandForFill(d.Content, content, field) {
			marker := fillMark + ": " + lastSegment(field)
			if len(allowed) > 0 {
				marker += " — one of: " + strings.Join(allowed, ", ")
			}
			marker += ">>"
			setPath(d.Content, path, marker)
			d.Holes = append(d.Holes, Hole{Path: path, Note: note, Allowed: allowed})
		}
	}
	return d, nil
}

// Finish validates a filled draft against the original's choreography and the
// rulebook's rules for this response type. It returns the final content.
func Finish(d *Draft, original map[string]any, checkRules func(content map[string]any) []string) (map[string]any, error) {
	var problems []string

	// 1. No holes left unfilled (a hole may be legitimately absent if its
	//    note says it is conditional and the condition does not hold).
	for _, h := range d.Holes {
		v, exists := getPath(d.Content, h.Path)
		s, isString := v.(string)
		unfilled := exists && isString && strings.Contains(s, fillMark)
		missing := !exists
		if unfilled || missing {
			if cond := parseCondition(h.Note); cond != nil {
				linePrefix := linePrefixOf(h.Path)
				condVal, _ := getPath(d.Content, linePrefix+cond.field)
				if fmt.Sprintf("%v", condVal) != cond.value {
					if unfilled {
						removePath(d.Content, h.Path)
					}
					continue // condition not met — the hole may stay empty
				}
			}
			problems = append(problems, fmt.Sprintf("%s is not filled in (%s)", h.Path, h.Note))
			continue
		}
		if len(h.Allowed) > 0 && isString && !contains(h.Allowed, s) {
			problems = append(problems, fmt.Sprintf("%s is %q — allowed values are: %s", h.Path, s, strings.Join(h.Allowed, ", ")))
		}
	}

	// 2. The echoes are untouched: byte-for-byte what the original dictated.
	origContent, _ := original["content"].(map[string]any)
	spec, err := responseSpec(original, d.ResponseType)
	if err == nil {
		for _, mc := range listOfMaps(spec, "must_copy") {
			from, _ := mc["from"].(string)
			to, _ := mc["to"].(string)
			expected := map[string]any{}
			copyPath(original, origContent, from, expected, to)
			for _, path := range expandTargets(expected, to) {
				want, _ := getPath(expected, path)
				got, _ := getPath(d.Content, path)
				if fmt.Sprintf("%v", want) != fmt.Sprintf("%v", got) {
					problems = append(problems, fmt.Sprintf("%s must echo the original value %v but is %v — copied fields may not be edited", path, want, got))
				}
			}
		}
	}

	// 3. The rulebook's own rules for this response type.
	if checkRules != nil {
		problems = append(problems, checkRules(d.Content)...)
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("this response is not ready to send:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return d.Content, nil
}

// ApplyFills writes submitted values into a draft's holes. Locked paths are
// silently ignored — echoes cannot be edited, whatever a form sends.
func ApplyFills(d *Draft, fills map[string]any) {
	locked := map[string]bool{}
	for _, p := range d.LockedPaths {
		locked[p] = true
	}
	for path, value := range fills {
		if locked[path] {
			continue
		}
		if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
			removePath(d.Content, path)
			continue
		}
		setPath(d.Content, path, value)
	}
}

// ---- choreography plumbing ----

func responseSpec(original map[string]any, responseType string) (map[string]any, error) {
	htr, _ := original["how_to_respond"].(map[string]any)
	for _, r := range listOfMaps(htr, "allowed_responses") {
		if t, _ := r["response_type"].(string); t == responseType {
			return r, nil
		}
	}
	return nil, fmt.Errorf("the original message does not allow a %q response", responseType)
}

func listOfMaps(m map[string]any, key string) []map[string]any {
	if m == nil {
		return nil
	}
	raw, _ := m[key].([]any)
	var out []map[string]any
	for _, item := range raw {
		if mm, ok := item.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}

func codeListValues(rulebook map[string]any, ref string) []string {
	// ref looks like "code_lists.line_decision"
	parts := strings.Split(ref, ".")
	var cur any = rulebook
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	codes, ok := cur.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	for k := range codes {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func computeDeadline(spec, about map[string]any, now time.Time) string {
	deadline, _ := spec["deadline"].(string)
	re := regexp.MustCompile(`within (\d+) hours`)
	if m := re.FindStringSubmatch(deadline); m != nil {
		hours, _ := strconv.Atoi(m[1])
		sentAt, _ := about["sent_at"].(string)
		if t, err := time.Parse(time.RFC3339, sentAt); err == nil {
			return fmt.Sprintf("%s (answer by %s)", deadline, t.Add(time.Duration(hours)*time.Hour).Format("2006-01-02 15:04 UTC"))
		}
	}
	return deadline
}

// ---- path machinery: dotted paths with one "[]" array pairing ----

// copyPath copies source path values (relative to the whole original message,
// "content." prefix refers to its content) into the target tree.
func copyPath(original, origContent map[string]any, from string, target map[string]any, to string) error {
	from = strings.TrimPrefix(from, "content.")
	if strings.Contains(from, "[]") {
		fromParts := strings.SplitN(from, "[].", 2)
		toParts := strings.SplitN(to, "[].", 2)
		if len(fromParts) != 2 || len(toParts) != 2 {
			return fmt.Errorf("array paths must pair, like lines[].x -> lines[].y")
		}
		srcArr, _ := getPath(origContent, fromParts[0])
		items, ok := srcArr.([]any)
		if !ok {
			return fmt.Errorf("%q is not a list in the original", fromParts[0])
		}
		for i, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			val, exists := getPath(m, fromParts[1])
			if !exists {
				continue
			}
			setPath(target, fmt.Sprintf("%s[%d].%s", toParts[0], i, toParts[1]), val)
		}
		return nil
	}
	val, exists := getPath(origContent, from)
	if !exists {
		return fmt.Errorf("%q is not present in the original", from)
	}
	setPath(target, to, val)
	return nil
}

// expandTargets lists the concrete paths a (possibly []-bearing) target
// pattern produced in the tree.
func expandTargets(tree map[string]any, pattern string) []string {
	if !strings.Contains(pattern, "[]") {
		return []string{pattern}
	}
	parts := strings.SplitN(pattern, "[].", 2)
	arr, _ := getPath(tree, parts[0])
	items, ok := arr.([]any)
	if !ok {
		return nil
	}
	var out []string
	for i := range items {
		out = append(out, fmt.Sprintf("%s[%d].%s", parts[0], i, parts[1]))
	}
	return out
}

// expandForFill lists concrete hole paths for a fill-in field. Array length
// comes from the draft (echoed lines) or the original content.
func expandForFill(draft, origContent map[string]any, field string) []string {
	if !strings.Contains(field, "[]") {
		return []string{field}
	}
	parts := strings.SplitN(field, "[].", 2)
	arr, ok := getPath(draft, parts[0])
	if !ok {
		arr, _ = getPath(origContent, parts[0])
	}
	items, isArr := arr.([]any)
	if !isArr {
		return nil
	}
	var out []string
	for i := range items {
		out = append(out, fmt.Sprintf("%s[%d].%s", parts[0], i, parts[1]))
	}
	return out
}

var indexRe = regexp.MustCompile(`^(\w+)\[(\d+)\]$`)

func getPath(tree map[string]any, path string) (any, bool) {
	var cur any = tree
	for _, seg := range strings.Split(path, ".") {
		if m := indexRe.FindStringSubmatch(seg); m != nil {
			obj, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			arr, ok := obj[m[1]].([]any)
			if !ok {
				return nil, false
			}
			i, _ := strconv.Atoi(m[2])
			if i >= len(arr) {
				return nil, false
			}
			cur = arr[i]
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, exists := obj[seg]
		if !exists {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func setPath(tree map[string]any, path string, value any) {
	segs := strings.Split(path, ".")
	var cur map[string]any = tree
	for i, seg := range segs {
		last := i == len(segs)-1
		if m := indexRe.FindStringSubmatch(seg); m != nil {
			name := m[1]
			idx, _ := strconv.Atoi(m[2])
			arr, _ := cur[name].([]any)
			for len(arr) <= idx {
				arr = append(arr, map[string]any{})
			}
			cur[name] = arr
			if last {
				arr[idx] = value
				return
			}
			next, ok := arr[idx].(map[string]any)
			if !ok {
				next = map[string]any{}
				arr[idx] = next
			}
			cur = next
			continue
		}
		if last {
			cur[seg] = value
			return
		}
		next, ok := cur[seg].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[seg] = next
		}
		cur = next
	}
}

func removePath(tree map[string]any, path string) {
	segs := strings.Split(path, ".")
	parentPath := strings.Join(segs[:len(segs)-1], ".")
	leaf := segs[len(segs)-1]
	var parent any = tree
	if parentPath != "" {
		var ok bool
		parent, ok = getPath(tree, parentPath)
		if !ok {
			return
		}
	}
	if m, ok := parent.(map[string]any); ok {
		delete(m, leaf)
	}
}

// parseCondition understands notes shaped "required when decision is 'change'".
type condition struct{ field, value string }

var condRe = regexp.MustCompile(`required when (\w+) is '([^']+)'`)

func parseCondition(note string) *condition {
	if m := condRe.FindStringSubmatch(note); m != nil {
		return &condition{field: m[1], value: m[2]}
	}
	return nil
}

func linePrefixOf(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[:i+1]
	}
	return ""
}

func lastSegment(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, s)
}
