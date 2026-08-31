// Package demo is the guided two-company walkthrough: the fictional retailer
// Nordkauf and the fictional dairy Molkerei Weide exchange real, signed
// supermessages on one machine, and every act shows one property of the
// design. --auto plays all human parts itself (that mode is also the
// end-to-end test).
package demo

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/engine"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/store"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/transport"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/trust"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/webconsole"
)

const (
	retailerGLN  = "GLN 4099999000015"
	supplierGLN  = "GLN 4088888000023"
	retailerPort = 7401
	supplierPort = 7402
)

// Demo holds the two running nodes and the shared world.
type Demo struct {
	RunDir   string
	SpecPath string
	Auto     bool
	Out      io.Writer
	In       *bufio.Reader

	Retailer *webconsole.Server
	Supplier *webconsole.Server

	specRaw map[string]any // the spec file, json.Number intact
	rbV3    map[string]any // retailer-signed rulebook v3 (raw tree)
	rbV3FP  string
	rbV4    map[string]any
	rbV4FP  string
}

// FindSpec walks upward from the working directory to the repo's spec file.
func FindSpec() (string, error) {
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "spec", "example-order.supermessage.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("cannot find spec/example-order.supermessage.json — run from inside the repo or pass --spec")
}

// Run plays the whole demo. In guided mode it starts the two web consoles and
// waits for the human; in auto mode it plays every part itself.
func Run(runDir, specPath string, auto bool, out io.Writer) (*Demo, error) {
	d := &Demo{RunDir: runDir, SpecPath: specPath, Auto: auto, Out: out, In: bufio.NewReader(os.Stdin)}
	if err := d.setup(); err != nil {
		return d, err
	}
	if !auto {
		d.startConsoles()
	}
	acts := []func() error{d.actA, d.actB, d.actC, d.actD, d.actE, d.actF}
	for _, act := range acts {
		if err := act(); err != nil {
			return d, err
		}
	}
	d.epilogue()
	return d, nil
}

func (d *Demo) say(format string, args ...any) {
	fmt.Fprintf(d.Out, format+"\n", args...)
}

func (d *Demo) banner(title, text string) {
	d.say("\n════════════════════════════════════════════════════════")
	d.say("  %s", title)
	d.say("════════════════════════════════════════════════════════")
	if text != "" {
		d.say("%s", text)
	}
}

// ---- setup ----

func (d *Demo) setup() error {
	d.banner("ACT 0 — Setup", "Two companies, two mailrooms, one shared folder standing in\nfor the network. Keys are generated fresh — nothing is pre-trusted.")

	os.RemoveAll(d.RunDir)
	if err := os.MkdirAll(d.RunDir, 0o755); err != nil {
		return err
	}
	dirPath := filepath.Join(d.RunDir, "directory.json")

	retailer, err := engine.Init(filepath.Join(d.RunDir, "nordkauf"), "Nordkauf Handels GmbH (fictional)", retailerGLN, dirPath)
	if err != nil {
		return err
	}
	supplier, err := engine.Init(filepath.Join(d.RunDir, "weide"), "Molkerei Weide GmbH (fictional)", supplierGLN, dirPath)
	if err != nil {
		return err
	}
	d.Retailer = &webconsole.Server{Node: retailer}
	d.Supplier = &webconsole.Server{Node: supplier}

	// Load the spec file — the demo's single source of truth.
	raw, err := os.ReadFile(d.SpecPath)
	if err != nil {
		return err
	}
	doc, err := supermessage.Parse(raw)
	if err != nil {
		return err
	}
	d.specRaw = doc.M

	// The retailer signs its own rulebook v3 (replacing the spec's
	// placeholder signature) and, being its publisher, trusts it.
	d.rbV3, d.rbV3FP, err = d.signRulebook(retailer, deepCopyMap(d.specRaw["rulebook"].(map[string]any)))
	if err != nil {
		return err
	}
	if err := bootstrapOwnRulebook(retailer, d.rbV3, 3, d.rbV3FP); err != nil {
		return err
	}

	// Rulebook v4 = v3 plus one rule (used in act D). The retailer trusts it
	// too, from its effective date.
	v4 := deepCopyMap(d.specRaw["rulebook"].(map[string]any))
	v4["version"] = json.Number("4")
	v4["valid_from"] = "2026-09-15"
	v4["rules"] = append(v4["rules"].([]any), map[string]any{
		"rule":           "R-06",
		"plain_language": "Every order line must state its quantity unit.",
		"machine_check":  "content.lines.all(l, has(l.quantity_unit))",
		"error_message":  "Line {line_number}: quantity unit missing (rule R-06).",
	})
	d.rbV4, d.rbV4FP, err = d.signRulebook(retailer, v4)
	if err != nil {
		return err
	}
	if err := bootstrapOwnRulebook(retailer, d.rbV4, 4, d.rbV4FP); err != nil {
		return err
	}

	// Out-of-band bootstrap, narrated: a human at Nordkauf typed the
	// supplier's address into their system. (First contact the other way
	// happens IN band, in act A.)
	if err := bootstrapPartnerRoute(retailer, supplier.Identity.Name, supplierGLN,
		supplier.Home.File("transport", "in")); err != nil {
		return err
	}

	d.say("Created %s and %s.", retailer.Identity.Name, supplier.Identity.Name)
	d.say("Key directory (the phone-book stub): %s", dirPath)
	d.say("The supplier trusts NOTHING yet: no partners, no rulebooks.")
	return nil
}

// signRulebook signs a rulebook tree with the node's key and returns it with
// its fingerprint.
func (d *Demo) signRulebook(n *engine.Node, rb map[string]any) (map[string]any, string, error) {
	rb["publisher_signature"] = map[string]any{
		"method":                     "ed25519-public-key-signature",
		"public_key_directory_entry": n.Identity.CompanyID,
		"value":                      "",
	}
	signingBytes, err := supermessage.RulebookSigningBytes(rb)
	if err != nil {
		return nil, "", err
	}
	rb["publisher_signature"].(map[string]any)["value"] = sign.Sign(n.Priv, signingBytes)
	fp, err := supermessage.Fingerprint(rb)
	if err != nil {
		return nil, "", err
	}
	return rb, fp, nil
}

// bootstrapOwnRulebook makes a publisher trust its own rulebook (a publisher
// obviously accepts what it publishes; nobody reviews their own rulebook).
func bootstrapOwnRulebook(n *engine.Node, rb map[string]any, version int, fp string) error {
	canon, err := supermessage.Canonical(rb)
	if err != nil {
		return err
	}
	id, _ := rb["id"].(string)
	dir := n.Home.File("trusted", "rulebooks", store.SafeName(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := store.AtomicWrite(fmt.Sprintf("%s/v%d-%s.json", dir, version, trust.ShortFP(fp)), canon); err != nil {
		return err
	}
	var rt trust.RulebookTrust
	store.ReadJSON(dir+"/trust.json", &rt)
	rt.RulebookID = id
	rt.Accepting = append(rt.Accepting, trust.TrustedVersion{
		Version: version, Fingerprint: fp, TrustedFrom: "2026-01-01", ApprovedVia: "own-publication",
	})
	return store.WriteJSON(dir+"/trust.json", rt)
}

// bootstrapPartnerRoute pins a partner and their folder address directly —
// the demo's out-of-band first configuration on the retailer side.
func bootstrapPartnerRoute(n *engine.Node, name, companyID, address string) error {
	if err := store.WriteJSON(n.Home.File("trusted", "partners", store.SafeName(companyID)+".json"),
		trust.Partner{Name: name, CompanyID: companyID, KeyFingerprint: "bootstrap", FirstSeen: "setup", ApprovedVia: "out-of-band setup"}); err != nil {
		return err
	}
	return store.WriteJSON(n.Home.File("trusted", "connections", store.SafeName(companyID)+".json"),
		map[string]any{"channels": []any{map[string]any{"channel": "local-folder", "address": address}}})
}

func (d *Demo) startConsoles() {
	for _, s := range []*webconsole.Server{d.Retailer, d.Supplier} {
		srv := s
		port := retailerPort
		if srv == d.Supplier {
			port = supplierPort
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			d.say("(console for %s could not start on port %d: %v)", srv.Node.Identity.Name, port, err)
			continue
		}
		go http.Serve(ln, srv.Handler())
		go func() {
			for {
				time.Sleep(time.Second)
				srv.WithLock(func(n *engine.Node) { n.ReceiveAll() })
			}
		}()
	}
	d.say("\nOpen the two consoles in your browser:")
	d.say("  Retailer  (Nordkauf):        http://localhost:%d", retailerPort)
	d.say("  Supplier  (Molkerei Weide):  http://localhost:%d", supplierPort)
}

// ---- message builders ----

// buildOrder assembles a signed order from the spec's parts.
func (d *Demo) buildOrder(orderNumber string, content map[string]any, rb map[string]any, rbVersion int, rbFP string, includeRulebook bool, channelAddress string) map[string]any {
	retailer := d.Retailer.Node
	msg := map[string]any{
		"supermessage_version": "0.1",
		"about": map[string]any{
			"message_type":   "order",
			"message_number": orderNumber,
			"sent_at":        time.Now().UTC().Format(time.RFC3339),
			"sender":         map[string]any{"name": retailer.Identity.Name, "company_id": retailer.Identity.CompanyID},
			"receiver":       map[string]any{"name": d.Supplier.Node.Identity.Name, "company_id": supplierGLN},
			"follows_rulebook": map[string]any{
				"id": rb["id"], "version": json.Number(fmt.Sprintf("%d", rbVersion)),
				"fingerprint": rbFP, "included_below": includeRulebook,
			},
			"signature": map[string]any{
				"signed_by": retailer.Identity.Name,
				"method":    "ed25519-public-key-signature",
				"value":     "",
			},
		},
		"content": content,
		"connections": map[string]any{
			"channels": []any{map[string]any{"channel": "local-folder", "address": channelAddress}},
		},
	}
	if includeRulebook {
		msg["rulebook"] = rb
		msg["how_to_respond"] = deepCopyMap(d.specRaw["how_to_respond"].(map[string]any))
		msg["answer_key"] = deepCopyMap(d.specRaw["answer_key"].(map[string]any))
	} else {
		// Responses still need the choreography; the spec carries it per
		// message, so every order ships it.
		msg["how_to_respond"] = deepCopyMap(d.specRaw["how_to_respond"].(map[string]any))
	}
	return msg
}

func (d *Demo) retailerInAddress() string  { return d.Retailer.Node.Home.File("transport", "in") }
func (d *Demo) retailerIn2Address() string { return d.Retailer.Node.Home.File("transport", "in2") }

// ---- the acts ----

func (d *Demo) actA() error {
	d.banner("ACT A — First contact",
		"Nordkauf sends order ORD-2026-88112 carrying its complete rulebook.\nThe supplier has never heard of Nordkauf: the message is HELD and a\nproposal appears in the supplier's review queue. Nothing is trusted\nuntil a human reads it and approves.")

	content := deepCopyMap(mapAt(d.specRaw, "content"))
	msg := d.buildOrder("ORD-2026-88112", content, d.rbV3, 3, d.rbV3FP, true, d.retailerInAddress())
	var sendErr error
	d.Retailer.WithLock(func(n *engine.Node) { _, sendErr = n.SendFile(supplierGLN, msg) })
	if sendErr != nil {
		return sendErr
	}
	d.say("→ Order sent into the supplier's folder.")

	d.receiveSupplier()
	d.say("→ Supplier verified the signature, met an unknown rulebook, HELD the order.")

	if d.Auto {
		if err := d.approveFirstPending(d.Supplier, "first contact reviewed; rulebook read; approving", false); err != nil {
			return err
		}
	} else {
		d.say("\nYOUR TURN — in the SUPPLIER console (http://localhost:%d):", supplierPort)
		d.say("read the rulebook proposal (every rule in plain language, plus its")
		d.say("answer-key self-test) and click APPROVE with a reason.")
		d.waitFor("waiting for your approval...", func() bool { return pendingCount(d.Supplier) == 0 })
	}
	d.receiveSupplier()
	d.receiveRetailer() // the configuration acknowledgement travels back
	d.say("→ Approved. Rulebook v3 is now trusted; the held order resumed and")
	d.say("  validated GREEN (R-05 noted as 'applies to the despatch advice').")
	return nil
}

func (d *Demo) actB() error {
	d.banner("ACT B — The response",
		"The order itself dictates how to answer: order number and line numbers\nare copied and LOCKED; the supplier only fills the real decisions.\nLine 1: accept. Line 2: change to 60 pieces.")

	if d.Auto {
		var err error
		d.Supplier.WithLock(func(n *engine.Node) {
			_, err = n.StartResponse("ORD-2026-88112", "order_response")
			if err != nil {
				return
			}
			err = n.FillDraft("draft-order-response-ORD-2026-88112", map[string]any{
				"lines[0].decision":           "accept",
				"lines[1].decision":           "change",
				"lines[1].confirmed_quantity": 60,
				"confirmed_delivery_date":     "2026-09-01",
			})
			if err != nil {
				return
			}
			_, err = n.FinishResponse("draft-order-response-ORD-2026-88112")
		})
		if err != nil {
			return err
		}
	} else {
		d.say("\nYOUR TURN — in the SUPPLIER console: on the green order click")
		d.say("'Respond: order response', fill the decisions (try to edit a locked")
		d.say("field — you cannot), then 'Check & send'.")
		d.waitFor("waiting for your response...", func() bool {
			return inboxHas(d.Retailer, "order_response")
		})
	}
	d.receiveRetailer()
	d.say("→ The retailer received the order response; the echoes verified")
	d.say("  against the original ORD-2026-88112 automatically.")
	return nil
}

func (d *Demo) actC() error {
	d.banner("ACT C — The broken order",
		"Nordkauf now sends ORD-2026-88113 — literally the spec answer key's\n'invalid example': GTIN missing, quantity zero. Properly signed, so it\nis NOT fraud; it reaches validation and fails on exact rules.")

	invalid := mapAt(d.specRaw, "answer_key")["invalid_examples"].([]any)[0].(map[string]any)
	content := deepCopyMap(invalid["content"].(map[string]any))
	content["order_number"] = "ORD-2026-88113"
	msg := d.buildOrder("ORD-2026-88113", content, d.rbV3, 3, d.rbV3FP, false, d.retailerInAddress())
	var sendErr error
	d.Retailer.WithLock(func(n *engine.Node) { _, sendErr = n.SendFile(supplierGLN, msg) })
	if sendErr != nil {
		return sendErr
	}
	d.receiveSupplier()
	d.say("→ Supplier validated it RED: 'Line 1: product GTIN missing (rule R-01)'")
	d.say("  and 'quantity must be greater than zero (rule R-02)' — the exact")
	d.say("  errors the answer key promised. That promise is also our test suite.")

	if d.Auto {
		var err error
		d.Supplier.WithLock(func(n *engine.Node) { _, err = n.SendRejection("ORD-2026-88113") })
		if err != nil {
			return err
		}
	} else {
		d.say("\nYOUR TURN — in the SUPPLIER console: click 'Send rejection' on the red order.")
		d.waitFor("waiting for the rejection...", func() bool { return inboxHas(d.Retailer, "rejection_notice") })
	}
	d.receiveRetailer()
	d.say("→ A structured rejection carrying the rule numbers reached Nordkauf.")
	return nil
}

func (d *Demo) actD() error {
	d.banner("ACT D — Rulebook version 4 arrives in-band",
		"Nordkauf adds one rule (R-06: every line must state its quantity unit)\nand sends a new order following v4, rulebook included. The supplier's\nchanges desk shows the plain-language DIFF; the order is held until a\nhuman approves; nothing changes silently.")

	content := deepCopyMap(mapAt(d.specRaw, "content"))
	content["order_number"] = "ORD-2026-88114"
	msg := d.buildOrder("ORD-2026-88114", content, d.rbV4, 4, d.rbV4FP, true, d.retailerInAddress())
	var sendErr error
	d.Retailer.WithLock(func(n *engine.Node) { _, sendErr = n.SendFile(supplierGLN, msg) })
	if sendErr != nil {
		return sendErr
	}
	d.receiveSupplier()
	d.say("→ Held. The review card reads: '1 rule added — R-06 …' (0 removed, 0 changed).")

	if d.Auto {
		if err := d.approveFirstPending(d.Supplier, "diff reviewed: one additive rule; we already send quantity units", false); err != nil {
			return err
		}
	} else {
		d.say("\nYOUR TURN — in the SUPPLIER console: read the v3 → v4 diff and APPROVE.")
		d.waitFor("waiting for your approval...", func() bool { return pendingCount(d.Supplier) == 0 })
	}
	d.receiveSupplier()
	d.receiveRetailer()
	d.say("→ v4 trusted (v3 stays accepted through the overlap window);")
	d.say("  ORD-2026-88114 resumed and validated GREEN under v4.")
	return nil
}

func (d *Demo) actE() error {
	d.banner("ACT E — The connection change",
		"Nordkauf's next order announces a NEW folder address ('we moved our\nSFTP endpoint'). The supplier keeps sending to the address ON FILE\nuntil a human confirms the change — out-of-band — and approves.\nThis is the anti-fraud rule at work on a legitimate change.")

	content := deepCopyMap(mapAt(d.specRaw, "content"))
	content["order_number"] = "ORD-2026-88115"
	msg := d.buildOrder("ORD-2026-88115", content, d.rbV4, 4, d.rbV4FP, false, d.retailerIn2Address())
	var sendErr error
	d.Retailer.WithLock(func(n *engine.Node) { _, sendErr = n.SendFile(supplierGLN, msg) })
	if sendErr != nil {
		return sendErr
	}
	os.MkdirAll(d.retailerIn2Address(), 0o755)
	d.receiveSupplier()
	d.say("→ The order itself validated green and was delivered — but the changed")
	d.say("  address became a PROPOSAL (old → new), applied to nothing.")

	if d.Auto {
		if err := d.approveFirstPending(d.Supplier, "phoned Nordkauf logistics; the move is real", true); err != nil {
			return err
		}
	} else {
		d.say("\nYOUR TURN — in the SUPPLIER console: open the connection-change card,")
		d.say("note the warning, tick 'I confirmed through a different channel',")
		d.say("give a reason, APPROVE.")
		d.waitFor("waiting for your approval...", func() bool { return pendingCount(d.Supplier) == 0 })
	}
	// The acknowledgement already travelled via the NEW address:
	files, _ := transport.Collect(d.retailerIn2Address())
	for name, raw := range files {
		d.Retailer.WithLock(func(n *engine.Node) { n.Ingest(name, raw) })
	}
	d.say("→ The supplier's acknowledgement landed in the NEW folder:")
	d.say("  %s", d.retailerIn2Address())
	return nil
}

func (d *Demo) actF() error {
	d.banner("ACT F — The fraud attempt (finale)",
		"A file arrives claiming to be Nordkauf, carrying a 'rulebook v5' and\nnew payment details — signed with a freshly generated attacker key.\nThis is the fake 'my bank account changed' letter. Watch where it dies.")

	_, attackerPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	rbV5 := deepCopyMap(d.rbV4)
	rbV5["version"] = json.Number("5")
	fraud := map[string]any{
		"supermessage_version": "0.1",
		"about": map[string]any{
			"message_type":     "order",
			"message_number":   "ORD-2026-99999",
			"sent_at":          time.Now().UTC().Format(time.RFC3339),
			"sender":           map[string]any{"name": d.Retailer.Node.Identity.Name, "company_id": retailerGLN},
			"receiver":         map[string]any{"name": d.Supplier.Node.Identity.Name, "company_id": supplierGLN},
			"follows_rulebook": map[string]any{"id": rbV5["id"], "version": json.Number("5"), "fingerprint": "sm0-forged", "included_below": true},
			"signature":        map[string]any{"signed_by": d.Retailer.Node.Identity.Name, "method": "ed25519-public-key-signature", "value": ""},
		},
		"rulebook": rbV5,
		"content":  map[string]any{"order_number": "ORD-2026-99999", "note": "please also update our bank account"},
		"connections": map[string]any{
			"channels": []any{map[string]any{"channel": "local-folder", "address": "/tmp/attacker-owned-folder"}},
		},
	}
	signingBytes, err := supermessage.FileSigningBytes(fraud)
	if err != nil {
		return err
	}
	fraud["about"].(map[string]any)["signature"].(map[string]any)["value"] = sign.Sign(attackerPriv, signingBytes)
	// The attacker drops the file straight into the supplier's folder.
	if err := (transport.Folder{}).Send(d.Supplier.Node.Home.File("transport", "in"),
		"ORD-2026-99999.supermessage.json", supermessage.MarshalPretty(fraud)); err != nil {
		return err
	}
	d.receiveSupplier()
	d.say("→ QUARANTINED at the front door: \"claims to be from Nordkauf but is")
	d.say("  not signed with Nordkauf's key. Nothing inside it was trusted or")
	d.say("  shown for approval. Kept as evidence.\"")
	d.say("  It never reached the review queue — strangers cannot even propose.")
	return nil
}

func (d *Demo) epilogue() {
	d.banner("EPILOGUE — where everything lives", "")
	d.say("Every artifact of this exchange is a plain file you can open:")
	d.say("  %s/", d.RunDir)
	d.say("    nordkauf/  and  weide/   — the two node homes")
	d.say("      trusted/               — live config; written ONLY by approvals")
	d.say("      review/decided/        — every proposal with who/why/when")
	d.say("      inbox/  archive/       — messages, byte-exact forever")
	d.say("      inbox/quarantine/      — the fraud file and its reason")
	d.say("      audit/audit-log.jsonl  — the hash-chained diary ('log --verify')")
	if !d.Auto {
		d.say("\nThe consoles stay up — explore, then Ctrl-C to finish.")
	}
}

// ---- helpers ----

func (d *Demo) receiveSupplier() {
	d.Supplier.WithLock(func(n *engine.Node) { n.ReceiveAll() })
}

func (d *Demo) receiveRetailer() {
	d.Retailer.WithLock(func(n *engine.Node) { n.ReceiveAll() })
}

// approveFirstPending plays the human in --auto mode.
func (d *Demo) approveFirstPending(s *webconsole.Server, reason string, outOfBand bool) error {
	var err error
	s.WithLock(func(n *engine.Node) {
		pending := n.Desk.Queue.ListPending()
		if len(pending) == 0 {
			err = fmt.Errorf("expected a pending proposal, found none")
			return
		}
		_, err = n.Approve(pending[0].ProposalID, "demo-operator", reason, outOfBand)
	})
	return err
}

func pendingCount(s *webconsole.Server) int {
	count := 0
	s.WithLock(func(n *engine.Node) { count = len(n.Desk.Queue.ListPending()) })
	return count
}

func inboxHas(s *webconsole.Server, msgType string) bool {
	found := false
	s.WithLock(func(n *engine.Node) {
		for _, name := range n.Home.ListDir("inbox") {
			var rec engine.InboxRecord
			if store.ReadJSON(n.Home.File("inbox", name), &rec) == nil && rec.Type == msgType {
				found = true
			}
		}
	})
	return found
}

func (d *Demo) waitFor(hint string, done func() bool) {
	d.say("  (%s)", hint)
	for !done() {
		time.Sleep(500 * time.Millisecond)
	}
}

func mapAt(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func deepCopyMap(m map[string]any) map[string]any {
	return supermessage.DeepCopy(m).(map[string]any)
}
