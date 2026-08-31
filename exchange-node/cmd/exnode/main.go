// exnode is the exchange node: one small program per company that receives,
// checks, answers, and routes supermessages — and treats incoming files as
// configuration proposals a human must approve. Run `exnode demo` for the
// guided two-company walkthrough.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/answerkey"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/audit"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/demo"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/engine"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/sign"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/supermessage"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/transport"
	"github.com/encefalopiatto/nomoreEDI/exchange-node/internal/webconsole"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "receive":
		err = withNode(os.Args[2:], func(n *engine.Node, _ []string) error {
			for _, line := range n.ReceiveAll() {
				fmt.Println(line)
			}
			return nil
		})
	case "status":
		err = withNode(os.Args[2:], cmdStatus)
	case "review":
		err = withNode(os.Args[2:], cmdReview)
	case "respond":
		err = withNode(os.Args[2:], cmdRespond)
	case "reject-message":
		err = withNode(os.Args[2:], func(n *engine.Node, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: exnode reject-message --home <dir> <message-number>")
			}
			sent, err := n.SendRejection(args[0])
			if err != nil {
				return err
			}
			fmt.Println("rejection sent:", sent)
			return nil
		})
	case "log":
		err = cmdLog(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "check":
		err = cmdCheck(os.Args[2:])
	case "demo":
		err = cmdDemo(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`exnode — the exchange node

  exnode demo [--auto] [--dir <run-dir>] [--spec <spec-file>]
        the guided two-company walkthrough (start here)

  exnode check <file.supermessage.json>
        the standalone reader: fingerprint + answer-key self-test of one file

  exnode init --home <dir> --name <company name> --id <company id> --directory <phone-book.json>
  exnode serve --home <dir> --port <port>       start a node with its web console
  exnode receive --home <dir>                   process the transport in-folder once
  exnode status --home <dir>                    what needs attention
  exnode review --home <dir> list|show <id>|approve <id> --as <name> --reason <text> [--confirmed-out-of-band]|reject <id> --as <name> --reason <text>
  exnode respond --home <dir> start <message-number> <response-type> | finish <draft-id>
  exnode reject-message --home <dir> <message-number>
  exnode log --home <dir> [--verify]            the audit diary (and its chain check)`)
}

func withNode(args []string, fn func(n *engine.Node, rest []string) error) error {
	fs := flag.NewFlagSet("node", flag.ContinueOnError)
	home := fs.String("home", "", "node home directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *home == "" {
		return fmt.Errorf("--home is required")
	}
	n, err := engine.Open(*home)
	if err != nil {
		return err
	}
	return fn(n, fs.Args())
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	home := fs.String("home", "", "node home directory to create")
	name := fs.String("name", "", "company name")
	id := fs.String("id", "", "company id (e.g. GLN 40...)")
	dir := fs.String("directory", "", "path of the shared key directory file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *home == "" || *name == "" || *id == "" || *dir == "" {
		return fmt.Errorf("init needs --home, --name, --id and --directory")
	}
	n, err := engine.Init(*home, *name, *id, *dir)
	if err != nil {
		return err
	}
	fmt.Printf("Node home created for %s (%s).\nPublic key published to %s.\n",
		n.Identity.Name, n.Identity.CompanyID, *dir)
	return nil
}

func cmdStatus(n *engine.Node, _ []string) error {
	state := webconsole.State(n)
	fmt.Printf("%s (%s)\n", n.Identity.Name, n.Identity.CompanyID)
	fmt.Printf("  needs your decision: %d proposal(s)\n", count(state["review"]))
	fmt.Printf("  held messages:       %d\n", count(state["held"]))
	fmt.Printf("  inbox:               %d message(s)\n", count(state["inbox"]))
	fmt.Printf("  open drafts:         %d\n", count(state["drafts"]))
	fmt.Printf("  quarantined:         %d\n", count(state["quarantine"]))
	return nil
}

func cmdReview(n *engine.Node, args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		pending := n.Desk.Queue.ListPending()
		if len(pending) == 0 {
			fmt.Println("Nothing waits for you.")
			return nil
		}
		for _, p := range pending {
			fmt.Printf("%s  [%s]  %s\n", p.ProposalID, p.Risk.Class, p.Describe())
		}
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: review show <id>")
		}
		p, ok := n.Desk.Queue.Load(args[1])
		if !ok {
			return fmt.Errorf("no proposal %s", args[1])
		}
		fmt.Println(string(supermessage.MarshalPretty(p)))
	case "approve", "reject":
		fs := flag.NewFlagSet("decision", flag.ContinueOnError)
		as := fs.String("as", "operator", "your name (goes in the audit log)")
		reason := fs.String("reason", "", "why (required)")
		oob := fs.Bool("confirmed-out-of-band", false, "you confirmed through a different channel")
		if len(args) < 2 {
			return fmt.Errorf("usage: review %s <id> --as <name> --reason <text>", args[0])
		}
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		var msg string
		var err error
		if args[0] == "approve" {
			msg, err = n.Approve(args[1], *as, *reason, *oob)
		} else {
			msg, err = n.Reject(args[1], *as, *reason)
		}
		if err != nil {
			return err
		}
		fmt.Println(msg)
	default:
		return fmt.Errorf("unknown review action %q", args[0])
	}
	return nil
}

func cmdRespond(n *engine.Node, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: respond start <message-number> <response-type> | finish <draft-id>")
	}
	switch args[0] {
	case "start":
		if len(args) < 3 {
			return fmt.Errorf("usage: respond start <message-number> <response-type>")
		}
		d, err := n.StartResponse(args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("Draft %s created. Open it, fill the <<FILL>> holes, then: respond finish %s\n",
			n.Home.File("outbox", "drafts", d.DraftID+".json"), d.DraftID)
	case "finish":
		sent, err := n.FinishResponse(args[1])
		if err != nil {
			return err
		}
		fmt.Println("checked against the choreography, signed, and sent:", sent)
	default:
		return fmt.Errorf("unknown respond action %q", args[0])
	}
	return nil
}

func cmdLog(args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	home := fs.String("home", "", "node home directory")
	verify := fs.Bool("verify", false, "re-check the whole hash chain")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *home == "" {
		return fmt.Errorf("--home is required")
	}
	l := audit.Open(filepath.Join(*home, "audit", "audit-log.jsonl"))
	if *verify {
		if err := l.Verify(); err != nil {
			return err
		}
		fmt.Println("the audit chain verifies: no line has been altered")
		return nil
	}
	for _, rec := range l.Read() {
		fmt.Printf("%v  %v  %v\n", rec["at"], rec["event"], rec["proposal_id"])
	}
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	home := fs.String("home", "", "node home directory")
	port := fs.Int("port", 7400, "console port (localhost only)")
	transportPort := fs.Int("transport-port", 0, "listen for HTTPS-pushed supermessages from partners on this port (0 = off)")
	tlsCert := fs.String("tls-cert", "", "TLS certificate file for the transport listener (plain HTTP without it — put TLS in front)")
	tlsKey := fs.String("tls-key", "", "TLS key file for the transport listener")
	sftpPort := fs.Int("sftp-port", 0, "run an SFTP drop folder for partners on this port (0 = off)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *home == "" {
		return fmt.Errorf("--home is required")
	}
	n, err := engine.Open(*home)
	if err != nil {
		return err
	}
	s := &webconsole.Server{Node: n}

	// The heartbeat: collect incoming files and retry due deliveries.
	go func() {
		for {
			time.Sleep(time.Second)
			s.WithLock(func(node *engine.Node) {
				node.ReceiveAll()
				node.ProcessQueue()
			})
		}
	}()

	// Partners can push supermessages straight to this node over HTTP(S).
	if *transportPort > 0 {
		addr := fmt.Sprintf(":%d", *transportPort)
		go func() {
			err := transport.StartInboundHTTP(addr, *tlsCert, *tlsKey, func(name string, b []byte) string {
				var outcome string
				s.WithLock(func(node *engine.Node) { outcome = node.Ingest(name, b) })
				return outcome
			})
			fmt.Fprintln(os.Stderr, "the HTTPS transport listener stopped:", err)
		}()
		scheme := "http"
		if *tlsCert != "" {
			scheme = "https"
		}
		fmt.Printf("  receiving pushed supermessages at %s://<this-host>:%d/exchange/inbound\n", scheme, *transportPort)
	}

	// Or they can upload into an SFTP drop folder served by this node.
	if *sftpPort > 0 {
		users := map[string]string{}
		usersPath := filepath.Join(*home, "transport", "sftp-users.json")
		if err := readJSONFile(usersPath, &users); err != nil || len(users) == 0 {
			return fmt.Errorf("the SFTP drop folder needs accounts: create %s containing {\"username\": \"password\"} per partner", usersPath)
		}
		hostKeyPath := filepath.Join(*home, "keys", "ssh_host_key.pem")
		if _, err := os.Stat(hostKeyPath); err != nil {
			if _, err := sign.Generate(hostKeyPath, hostKeyPath+".pub"); err != nil {
				return err
			}
			fmt.Println("  generated a fresh SFTP host key:", hostKeyPath)
		}
		hostPriv, err := sign.LoadPrivate(hostKeyPath)
		if err != nil {
			return err
		}
		signer, err := ssh.NewSignerFromKey(hostPriv)
		if err != nil {
			return err
		}
		fmt.Printf("  SFTP host key fingerprint (give this to partners): %s\n", ssh.FingerprintSHA256(signer.PublicKey()))
		inDir := filepath.Join(*home, "transport", "in")
		go func() {
			err := transport.StartInboundSFTP(fmt.Sprintf(":%d", *sftpPort), users, signer, inDir)
			fmt.Fprintln(os.Stderr, "the SFTP listener stopped:", err)
		}()
		fmt.Printf("  SFTP drop folder for partners on port %d\n", *sftpPort)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	fmt.Printf("%s — console at http://localhost:%d (Ctrl-C to stop)\n", n.Identity.Name, *port)
	return http.Serve(ln, s.Handler())
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// cmdCheck is the standalone reader: it proves a supermessage file against
// itself — fingerprint recomputed, answer key executed.
func cmdCheck(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: exnode check <file.supermessage.json>")
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	doc, err := supermessage.Parse(raw)
	if err != nil {
		return err
	}
	fmt.Printf("File: %s\n", args[0])
	about, _ := doc.Section("about")
	fmt.Printf("Type: %s · number %s · from %s\n",
		supermessage.GetString(about, "message_type"),
		supermessage.GetString(about, "message_number"),
		supermessage.GetString(about, "sender", "name"))

	rb, ok := doc.Section("rulebook")
	if !ok {
		fmt.Println("No rulebook included (fingerprint-only message).")
		return nil
	}
	fp, err := supermessage.Fingerprint(rb)
	if err != nil {
		return err
	}
	declared := supermessage.GetString(about, "follows_rulebook", "fingerprint")
	fmt.Printf("Computed rulebook fingerprint: %s\n", fp)
	if declared == fp {
		fmt.Println("Declared fingerprint matches.")
	} else {
		fmt.Printf("NOTE: the declared fingerprint (%s) does not match — a placeholder or tampering.\n", declared)
	}
	sigVal := supermessage.GetString(rb, "publisher_signature", "value")
	if strings.HasPrefix(sigVal, "SIG-") || sigVal == "" {
		fmt.Println("Publisher signature is a placeholder — skipped (content-vs-rulebook checking only).")
	}

	norm := supermessage.Normalize(doc.M).(map[string]any)
	rbNorm, _ := norm["rulebook"].(map[string]any)
	akNorm, _ := norm["answer_key"].(map[string]any)
	rep := answerkey.Run(akNorm, rbNorm)
	fmt.Println("\nAnswer-key self-test (the file proving itself):")
	for _, ex := range rep.Examples {
		status := "PASS"
		if !ex.Pass {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %-7s %q — expected %v, got %v\n", status, ex.Kind, ex.Description, ex.Expected, ex.Got)
	}
	if rep.AllPass {
		fmt.Println("All examples behave exactly as the rulebook promises.")
		return nil
	}
	return fmt.Errorf("the answer key does not pass — the rules do not behave as the examples promise")
}

func cmdDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	auto := fs.Bool("auto", false, "play every human part automatically (headless)")
	dir := fs.String("dir", "", "run directory (default: exchange-node/demo/run)")
	spec := fs.String("spec", "", "path to the spec example (default: found in the repo)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	specPath := *spec
	if specPath == "" {
		var err error
		specPath, err = demo.FindSpec()
		if err != nil {
			return err
		}
	}
	runDir := *dir
	if runDir == "" {
		runDir = filepath.Join(filepath.Dir(filepath.Dir(specPath)), "exchange-node", "demo", "run")
	}
	_, err := demo.Run(runDir, specPath, *auto, os.Stdout)
	if err != nil {
		return err
	}
	if !*auto {
		select {} // keep the consoles alive until Ctrl-C
	}
	return nil
}

func count(v any) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map {
		return rv.Len()
	}
	return 0
}
