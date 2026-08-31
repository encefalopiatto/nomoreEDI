# The Exchange Node — a working prototype you can run

*The supermessage format ([SUPERMESSAGE.md](SUPERMESSAGE.md)) describes one file that carries a business message and everything needed to understand, check, and answer it. The exchange node is the small program that brings that file to life. This page tells you what it is and how to watch it work — in plain language, start to finish. The code lives in [exchange-node/](exchange-node/).*

---

## What the exchange node is

Think of it as a very careful mailroom clerk that each company runs. For every arriving supermessage, the clerk:

1. **Checks the signature.** Is this file really from who it claims to be from? If not, it goes to quarantine — unopened, kept as evidence.
2. **Checks the rulebook.** Does the file follow a rulebook we already trust? If it carries a *new* rulebook or a *new version*, the message is put on hold and the rulebook becomes a **proposal** in the review queue — with every rule shown in plain language, a before/after comparison to what we trust today, and proof that the rulebook passes its own answer key.
3. **Waits for a human.** Nothing an incoming file proposes — a new rulebook, a changed delivery address, anything — ever changes the node's configuration by itself. A person reads it, states a reason, and approves or rejects. Every decision lands in a tamper-evident audit diary.
4. **Checks the content.** Once the rulebook is trusted, every message is checked against it, and problems are reported by rule number in plain sentences: "Line 1: product GTIN missing (rule R-01)."
5. **Helps you answer.** The original message dictates how responses are built: copied fields arrive pre-filled and locked; you only fill the real decisions (accept / change / reject), chosen from the allowed list. The node refuses to send a response that breaks the choreography.
6. **Routes files.** Messages travel to the address *on file* for each partner — never to an address a message merely claimed. Address changes are proposals too, and approving one requires confirming through a different channel first (that one habit is what kills the classic "our bank account changed" fraud).

That is the whole idea from the discussion, running: *the software exchanges and routes files, treats them as config files as well, and applies their directions only after human review.*

## What you need

Go 1.22 or newer (one install, nothing else). Then:

```
cd exchange-node
go run ./cmd/exnode demo
```

That single command starts the whole guided walkthrough. For the quick self-contained taste without the demo:

```
go run ./cmd/exnode check ../spec/example-order.supermessage.json
```

— the standalone "reader" opens the spec's example file alone and proves it against its own answer key: the desert-island property, live.

## The demo, act by act

Two fictional companies run on your machine: **Nordkauf** (a retailer, console at http://localhost:7401) and **Molkerei Weide** (a dairy, console at http://localhost:7402). A shared folder stands in for the network. Keys are generated fresh — nothing is pre-trusted. The demo narrates each act in the terminal and waits for you to act in the supplier's console.

- **Act A — First contact.** Nordkauf sends a real order carrying its complete rulebook. Weide has never heard of Nordkauf: the order is *held*, and the review queue shows the rulebook — every rule in plain language, plus its answer-key self-test. You read and approve. Only then is the rulebook trusted, and the order resumes and validates green.
- **Act B — The response.** You answer the order. The order number and line numbers are pre-filled and locked ("copied from the order — you cannot edit this"); you pick accept for line 1 and change-to-60 for line 2. The node checks your response against the choreography before signing and sending it.
- **Act C — The broken order.** A properly signed but *wrong* order arrives (it is literally the spec's "deliberately wrong example"). It validates red with the exact promised errors — R-01 and R-02 — and one click sends a structured rejection carrying those rule numbers back.
- **Act D — A rulebook change, in-band.** Nordkauf adds one rule and sends version 4 inside a new order. The review queue shows the difference in one line: "1 rule added — every order line must state its quantity unit." Nothing changes until you approve; then v4 is trusted, v3 stays accepted through an overlap window, and the held order validates green under v4.
- **Act E — The connection change.** Nordkauf announces a new delivery folder. The order itself flows normally — but the address change is a proposal showing old → new, with a warning to confirm through a different channel. Until you approve, files keep going to the address on file. After you approve, the next file visibly lands in the new folder.
- **Act F — The fraud attempt.** A file claiming to be Nordkauf arrives, carrying a "rulebook v5" and new payment details — signed with an attacker's key. It dies at the front door: quarantined with a plain reason, never shown in the review queue at all. Strangers cannot even propose.

Prefer it fully automatic? `go run ./cmd/exnode demo --auto` plays every human part itself in seconds — that same run is the project's end-to-end test.

## The three safety rules, live

| Safety rule (from SUPERMESSAGE.md) | Where the demo proves it |
|---|---|
| 1 — Everything is signed, with key pairs | Act F: the wrong key is caught before anything else happens |
| 2 — A message can propose, never apply | Acts A, D, E: every change waits in the review queue for a human |
| 3 — Rules check, they never act | Act C: a rule failure produces a precise report, nothing more |

## What is real and what is pretend

Real: the Ed25519 signatures, the fingerprints, the CEL rules from the spec running verbatim, the plain-language diffs, the hash-chained audit diary, the choreography enforcement. Pretend (stated honestly): the key directory is a local JSON file, not an operated service; "the network" is a folder; the ERP is an inbox folder; AS2/SFTP are displayed but not spoken; deadlines are computed and shown, not enforced; reviewer names are an honesty convention, not authentication; the two companies are fictional.

## Where things live on disk

Each company's node home is plain files you can open in any editor:

```
exchange-node/demo/run/<company>/
  trusted/        live configuration — written ONLY when a human approves
  review/         proposals: pending/ awaits you, decided/ keeps who/why/when
  held/           messages parked until their rulebook is decided
  inbox/          delivered messages (and quarantine/ for failed signatures)
  outbox/         drafts you are writing, and everything sent
  archive/        every message, byte-exact, forever
  log/            what happened to each message, step by step
  audit/          the hash-chained diary — `exnode log --home <dir> --verify`
```

A nice property to check by hand: a trusted rulebook's file name carries its fingerprint, and `sha256sum` of the file content *is* that fingerprint.

## Running nodes for real (without the demo script)

```
go run ./cmd/exnode init  --home mycompany --name "My Company" --id "GLN 4012345000001" --directory ./directory.json
go run ./cmd/exnode serve --home mycompany --port 7400
```

Each node is one folder plus one command; the console at the printed address does the rest. `exnode status`, `review`, `respond`, and `log --verify` do the same jobs from the terminal.

## Running the tests

```
cd exchange-node && go test ./...
```

The spec file's answer key is the test suite's ground truth: the valid example must pass, the wrong examples must fail with exactly the promised rule numbers, and the whole demo replays headless asserting the end state of both companies — including the quarantined fraud file and the intact audit chains.

## What this prototype is — and is not

It makes the supermessage tangible: one file, opened by a small program, is enough to check a message, answer it correctly, and manage change safely. It does **not** change the build order in [DECISION.md](DECISION.md): the rulebook library, the compiler, the conformance checkmark, and the change ledger still come first commercially; the traveling format ships last, once the rulebooks have earned trust. This prototype exists so that "last" is a demonstrated design rather than a promise.
