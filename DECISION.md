# What to build, and how

*Build recommendation derived from [ASSESSMENT.md](ASSESSMENT.md) and the [research corpus](research/). Four products in strict order, with the exchange language itself deliberately last. August 2026.*

*Parts I and II are written for engineers. [Part III](#part-iii--in-plain-language) says the same thing in plain language, for anyone.*

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

---

## Part III — In plain language

*The same plan as Parts I and II, written so that anyone — not just developers — can understand it.*

### The problem, in one paragraph

Today, when a supplier connects to a retailer, the retailer's requirements live in a PDF document, in emails, and in the heads of integration engineers. A person reads the PDF, builds the translation between the two systems by hand, and then both sides send test files back and forth for weeks until someone at the retailer says "looks good." After go-live, the retailer can quietly change their rules, and things break in production. Our own data says the slow, expensive parts are the **testing** and the **clarifying of unclear requirements** — not the translation work itself.

### The idea: replace the PDF with a rulebook a computer can read

For each retailer and each document type (order, invoice, delivery note), there is one **rulebook file**. It contains, written in a strict format a computer can act on:

- **Which fields exist and which are mandatory.** "An invoice line must have a product number and a price."
- **The if-then rules.** "If the product has a batch number, it must also have a best-before date." "If it's a credit note, it must reference the original invoice."
- **How to answer.** "When you receive an order, your order response must copy the order number and the line numbers exactly, and arrive within 48 hours."
- **Delivery details.** Which address and channel messages should be sent through.
- **An answer key: example messages.** Some correct, some wrong on purpose — each wrong one labeled with the exact error it should trigger.

That last part matters most: because the rulebook contains examples with known right answers, **anything built from the rulebook can be tested automatically.** That is what the PDF could never do.

### The four things this makes possible

**1. The checker.** Every message is compared against the rulebook automatically. Instead of "file rejected, good luck," the sender gets: "line 12 breaks rule 47 — best-before date is missing." Precise, instant, no phone call needed.

**2. The builders.** From one rulebook, software produces: the first draft of the translation between systems, the human-readable documentation (the PDF becomes something we *print out of* the rulebook, never something we read *into* it), and the old-style EDI version for partners who stay on the old system. The answer key checks each of these before anyone relies on them.

**3. The self-service test.** A supplier uploads sample files to a website. They get a pass/fail report with exact reasons, fix their files, and retry — as many times as needed, any hour of the day. When everything passes, they get an official "approved" stamp and go live. Nobody at the retailer reviews test files anymore, and nobody waits weeks for a sign-off email. This attacks the single biggest source of waiting.

**4. The changelog.** When requirements change, a version 2 of the rulebook is published. Everyone connected is notified automatically, there is a transition period where both versions are accepted, and a dashboard shows who has switched and who hasn't. Silent changes — today's second-biggest cause of production failures — stop being possible.

And only after all that is proven: a new message format where every message simply says "I follow rulebook #12345, version 3," and a small piece of software at each company reads the rulebook and handles the rest. That is the endgame — but it only makes sense once the rulebooks exist and have earned trust. The format itself is already specified in draft as the **supermessage** — see [SUPERMESSAGE.md](SUPERMESSAGE.md): specifying the traveling form early costs little and sharpens the rulebook design; *shipping* it still comes last. A runnable prototype of the software that carries it — the exchange node, with a guided two-company demo — exists as well ([NODE.md](NODE.md)); it demonstrates the endgame without changing this build order.

### What the software would look like

Five parts. Two are screens for the outside world, two are screens for Procuros people, one is invisible machinery.

1. **The rulebook library.** A catalog, like a well-organized document archive. Every entry is one rulebook: "Edeka — Invoice — version 3, valid from 1 October." Open it and you see readable pages: the field list, the if-then rules in plain sentences, the examples. A "History" tab shows what changed between versions, like tracked changes in a Word document. Every other part reads from this library.
2. **The checker.** No screen of its own — machinery built into the message pipeline we already run. Every message gets held against its rulebook for a split second. What you see of it: a green tick or a red flag on each message in monitoring, and the red flag says in words what is wrong — "line 12: best-before date missing (rule 47)" — not just "rejected."
3. **The workbench** — for our integration engineers. Draft rulebooks appear here: the software reads our existing customer setups and the retailer's PDF, drafts a rulebook from both, and shows the disagreements — "the live setup does X, the PDF says Y — which is correct?" The engineer clicks through them. Below that, a test report: "applied to six months of real messages, this draft would have wrongly rejected 3 of 10,000 — here they are." The engineer's job shifts from writing translations by hand to refereeing a machine that drafts them.
4. **The test portal** — for suppliers. A plain website: drag a sample file in, get a report seconds later — green and red lines, each red one a plain sentence pointing at the exact spot in the file. Fix, retry, any time of day. All green → an "Approved" badge naming the rulebook version and date, and the connection can go live. No human at the retailer looks at test files again; this replaces the weeks of email ping-pong.
5. **The change board** — for account and support teams. One dashboard: "Rewe invoice version 4 takes effect 1 November. 61 of 74 connected suppliers already pass it; these 13 don't — here is what each is missing." Notifications go out automatically at publication; during the transition window both versions are accepted.

A normal day with it: a new supplier joins a known retailer — draft translation generated and tested against the rulebook's examples, engineer settles the few open questions in an afternoon, supplier self-tests to green, approved, live in days, no waiting on the retailer. A bad invoice is caught *before* it leaves — "price on line 3 doesn't match the agreed price list" — so the retailer never sees it: no rejection, no dispute, no fine. A retailer changes a requirement — version 4 appears in the library with changes highlighted, everyone is notified the same minute, and the change board shows who is ready weeks before the deadline instead of us learning it from production failures.

Physically: one web application (library, workbench, portal, change board are sections of it) plus the invisible checker plugged into the existing message pipeline. Messages keep flowing exactly as today — what is new is that the *rules* about them live in one place where software can read them, test against them, and shout early.

### How we would actually build it

**Step 1 — write one rulebook by hand.** Pick one real, painful flow (say, fresh-food delivery notes with batch numbers). Write its rulebook manually. If a person can't write it comfortably, the format is wrong — better to find out in week one.

**Step 2 — draft rulebooks automatically from what we already have.** Our existing translations already contain the rules, just hidden inside code ("if field X exists, do Y"). A program can read them and pull those rules out. At the same time, AI reads the retailer's PDF and extracts rules from there. Then we compare: where both sources agree, trust the rule; where they disagree, ask an experienced colleague. That comparison also shows us, for the first time, how much knowledge lives only in people's heads.

**Step 3 — test rulebooks against the past.** Run months of old, real messages through the drafted rulebook. If it rejects messages that were actually fine, it is too strict. If it accepts messages that later caused support tickets, it is too loose. Fix and repeat until it is right.

**Step 4 — watch-only mode.** Turn the checker on for live traffic, but let it only *flag*, never block. For a few months we compare its flags with what actually went wrong. Only when it has proven trustworthy does it get the power to reject anything.

**Step 5 — the experiment that decides everything.** For a few flows, let AI build the translation twice: once from just the PDF, once from the rulebook with its answer key. Count the mistakes — especially the *silent* ones, where a message goes through but carries wrong data, because those become wrong payments. If the rulebook version makes clearly fewer silent mistakes, this is a real product with a real edge. If not, we learned it cheaply.

**Step 6 — the self-service test site, then the changelog.** Each builds on the pieces before it.

### Why this order

Each piece is useful on its own even if we never go further. The rulebooks alone preserve knowledge that today walks out the door when people leave. The checker alone catches errors earlier. The self-service test alone removes the waiting. The changelog alone kills silent breakage. The new message format is the only piece that is worthless unless everything before it worked — so it goes last. And thirty years of history says exactly this: new formats never win on their own, but tools that remove waiting and ambiguity do.

**Team:** two or three developers, plus one experienced integration person as the referee for unclear rules. About three months to get through steps 1–5 and have the decisive numbers.
