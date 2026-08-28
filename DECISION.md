# What to build, and how

*Build recommendation derived from [ASSESSMENT.md](ASSESSMENT.md) and the [research corpus](research/). Four products in strict order, with the exchange language itself deliberately last. August 2026.*

---

## Part I — What to build, and why

### 1. The contract file — turn what Procuros knows into an artifact

One versioned, machine-readable file per retailer × document type that states everything the PDF, the mapping code, and people's heads currently hold jointly: the shape, the conditional rules ("if batch present, best-before required"), the response rules ("echo my line numbers, reference my PO here, answer within 48h"), the master-data preconditions ("prices must match this PRICAT"), and a set of example messages that must pass and must fail.

**Why:** every other product falls out of this file, and it requires nobody's permission — it is mined from the JSONata mappings and retailer guides Procuros already has. It also converts Procuros' most valuable asset from implicit (knowledge trapped in mappings and heads, which walks out the door and doesn't compound) into explicit (a corpus nobody else in the market has). Even if nothing else ever ships, this alone is worth having.

### 2. The compiler — turn the file into everything else

From one contract, generate: the validator that runs on every live message, the human-readable guide (the PDF becomes an *output*, never again an input), draft mappings with the contract's own test vectors as the pass/fail oracle, and the EDIFACT projection for legacy partners.

**Why:** this is where the two unproven bets get measured instead of debated. If LLM-compiled mappings checked against contract oracles beat today's process on silent errors and cycle time, the moat exists; if they don't, a quarter was spent finding out, not three years. And it attacks the verified cost structure: mapping is only ~15–25% of onboarding effort — the compiler's real target is the testing and ambiguity that eat the other 60–80%.

### 3. The checkmark — self-service conformance

A portal/API where a supplier submits sample messages and gets clause-level pass/fail plus a signed "conformant as of version X" attestation. Go-live stops waiting for a human at the retailer to bless test files.

**Why:** operational experience says partner turnaround sets go-live dates — waiting, not working. This removes the partner from the critical path, which no amount of internal efficiency can fix. It is also the only piece a *retailer* actively wants: their suppliers onboard faster at zero cost to them. PEPPOL proved this exact mechanism at continental scale; it is the most de-risked idea in the whole design.

### 4. The changelog — governed change

Every requirement change becomes a new contract version: diffed, dated, with an overlap window and notified subscribers. Silent changes become protocol violations you can point at.

**Why:** silent partner requirement changes are the #2 verified cause of production failures, master-data drift is #1, and both are relationship-lifetime costs — they never stop, unlike onboarding, which happens once. Nobody in the market sells change control for trade relationships. This is also the piece that makes revenue recurring rather than project-shaped.

### Only then: the language

The wire format — messages carrying a contract hash, declared response affordances, the minimalist runtime in ERP app stores — is the endgame, and it becomes *cheap* once 1–4 exist: at that point "nomoreEDI" is just a nicer serialization of contracts already running in production, adoptable unilaterally because the compiler already speaks EDIFACT to the other side.

### Why this order, in one sentence each

- Thirty years of history says format-first dies (XML, ebXML, SEF) and executable-artifact-first wins (PEPPOL, OpenAPI, FHIR) — so ship the artifact, not the syntax.
- Procuros' own numbers say the money is in testing, ambiguity, and drift — steps 3 and 4 — not in mapping.
- Steps 1–4 are each independently valuable if the endgame never happens; the language is only valuable if everything before it worked. Build the things that are true either way first, and let them pay for the bet.

The first two weeks are concrete: pick three high-friction flows (fresh food with batch/best-before, something with Pfand, one scenario-heavy ORDRSP relationship), mine their contracts from the existing mappings, and run the two-arm silent-error comparison. That result decides whether this is a product line or a feature.

---

## Part II — How to build it

Guiding principle throughout: **anchor on the Canonical model that already exists, emit the artifacts the pipeline already runs, and shadow everything before enforcing anything.**

### The contract format (weeks 1–2, mostly design)

A contract is one JSON document, append-only versioned, addressed by the SHA-256 of its canonicalized form. Five sections:

- **Meta** — publisher, document type (INVOIC/ORDERS/…), direction, base profile + overlay (retailer profile, thin per-supplier overlay inheriting from it).
- **Shape** — JSON Schema over the *Canonical* representation, not over EDIFACT. This is the load-bearing decision: Canonical is Procuros' existing shared semantics, so contracts stay syntax-neutral, and the EDIFACT/X12/CSV specifics live in a separate **projection** section (canonical field ↔ segment/element bindings, from which the human-readable guide and the wire translator are both generated).
- **Rules** — CEL expressions over the canonical doc plus a context object (master-data digests, referenced documents). CEL because it is deterministic, non-Turing-complete, and has production Go/Java runtimes — embed `cel-go` in the pipeline rather than building an interpreter.
- **Responses** — allowed next documents, correlation keys, echo bindings as JSON-Pointer source→target pairs, decision holes with enumerated codes, deadlines.
- **Test vectors** — synthetic pass/fail examples, each fail annotated with the clause IDs it must trigger. Synthetic, generated from shape+rules and checked by the validator — never sampled production traffic, because real prices and volumes in test vectors are a confidentiality problem.

Start by *hand-writing* one contract for one real flow before building any tooling. If a human can't write it comfortably, the format is wrong — learn that in week one.

### The miner (weeks 3–6)

JSONata parses to an AST — that is the whole trick. Walk the destination/decanonicalization transformations: fields written become required/optional shape; `$exists`/ternary branches become conditional rules; lookup tables become code lists; format strings become date/number constraints. In parallel, run an LLM over the retailer's PDF guide for the same flow and diff the two extractions — **agreement is a rule you trust; disagreement is a question for a human**. That diff report is itself the first useful product output: it is the tribal-knowledge audit from the measurement agenda.

Then back-test: replay months of production history for that relationship through the drafted contract. Every false rejection means the contract is too strict; every accepted message that later produced a support ticket means it is too loose. Iterate until stable. This loop is also how contracts are *kept* honest forever.

### The validator, shadow-first (same weeks)

One library/service: canonical doc + contract digest + master-data context in, clause-level verdicts out. Wire it into the existing pipeline as a non-blocking step — **log-only for a quarter**. That yields precision/recall against real outcomes before any message is ever rejected by a contract, and produces the failure-taxonomy data (ASSESSMENT.md §9) as a side effect. Flip to enforcing per relationship only once shadow numbers are clean.

### The compiler experiment (parallel — this is Bets 1 & 2)

Given a contract plus a new partner's sample files, have the LLM generate a mapping — **in the existing JSONata transformation format**, so it drops into today's runtime with no new execution infrastructure. Loop: generate → run against test vectors and validator → feed the clause-level failures back → converge or escalate to a human. Run it two-armed on the same flows (contract-oracle vs. PDF-fed generation) and track three numbers from day one: auto-pass rate, human-touch rate, and silent-wrong rate measured against the human-built mapping in shadow. Those numbers are the investment decision.

### The checkmark (quarter 2)

A thin portal/API over the validator: supplier or ERP plugin submits samples, gets clause-level pass/fail with fix hints, and on full pass a signed attestation `{contract digest, submitter, timestamp, vector coverage}`. Go-live becomes a state machine — attestation present + master-data preconditions verified → live — instead of an email thread. Pilot with one friendly retailer relationship where Procuros controls both sides, so no external buy-in is needed to prove the cycle-time delta.

### The changelog (quarters 2–3)

Falls out of the registry almost for free: a change is a new contract version with a computed structural diff, a declared semantic annotation (the one part machines can't classify), an origin tag (partner notice / internal / defect / regulation), an effective window during which the validator accepts both versions, and a link tying one partner notice to its several releases. Subscribed relationships get notified; a dashboard shows who isn't conformant with v-next. This replaces ad-hoc mapping edits on pilot flows and generates the change-ledger data that currently can't be gotten.

### What deliberately not to build yet

No new wire format, no runtime SDK, no public registry, no signing federation, no standards-body conversations. Every one of those gets dramatically cheaper if the shadow numbers and the two-arm experiment come back good, and is wasted if they don't.

### Sizing, honestly

2–3 engineers plus one senior integration engineer as the domain oracle gets through the miner + shadow validator + compiler experiment in a quarter. The single biggest technical risk is not any component — it is contract quality on messy flows, which is why the back-testing loop and shadow mode are not nice-to-haves; they are the safety net that makes everything else honest.
