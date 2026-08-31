# The Supermessage — specification v0.1, in plain language

*One file that contains a business message **and** everything needed to understand it, check it, answer it, and stay connected to its sender. Draft v0.1, August 2026. A complete worked example lives in [spec/example-order.supermessage.json](spec/example-order.supermessage.json).*

---

## The idea in one paragraph

Today a message (an order, an invoice) is useless without things that live elsewhere: the retailer's PDF guide, the mapping someone built, the emails where details were clarified, the connection settings. A **supermessage** is one file that carries all of it: the business data itself, the complete rulebook that governs it, instructions for building the response, the sender's connection details, and an answer key of correct and deliberately wrong examples. Open one supermessage, alone, with nothing else, and a small reader program can check it, show it in readable form, build the skeleton of the right response, and know where to send it. Nothing is left to guess, because nothing is taught by example — everything is stated outright.

## What we learned from history, and how this design answers it

The research in [ASSESSMENT.md](ASSESSMENT.md) found three ways this idea has died before, and this specification answers each one directly:

1. **"Software guesses the rules from a received message" fails** — one example can never show every case (returns, credit notes, variable weight, year-end). *Answer: the supermessage doesn't teach by example. It carries the complete rulebook, stated as rules. Guessing is not part of the design.*
2. **"Executable instructions from a stranger" is dangerous** — a file that reconfigures your system is also how fraud works (a fake "my bank account changed" message). *Answer: a supermessage can propose changes but never apply them; everything is signed; rules can only check values, never run programs. See "The three safety rules" below.*
3. **Sending everything in every message is wasteful** — the rulebook can be far bigger than the order it rides with. *Answer: complete by default, shortened only by agreement. See "First contact and repeat sends" below.*

## The six sections of a supermessage

Every supermessage is one file with six sections.

### 1. About
What this file is. The message type (order, invoice, delivery note), a unique message number, who sent it, who it is for, when, and **which rulebook and version it follows**. Plus the signature that proves the file was not altered and really comes from the stated sender.

### 2. Content
The business data itself — the thing an old EDI message would carry: the order lines, quantities, prices, dates, parties, references. Written with plain field names, not cryptic codes.

### 3. Rulebook
The complete set of rules governing this kind of message between these parties:
- **Fields** — every field, its plain meaning, whether it is required, its format, and (where one exists) a link to the standard dictionary entry it corresponds to, so "delivery date" means the same thing to everyone.
- **If-then rules** — "if the product is batch-tracked, the despatch advice must carry a best-before date." "If this is a credit note, it must reference the original invoice." Each rule has a number and a plain-language error message.
- **Scenarios** — the special cases spelled out: returns, credit notes, partial deliveries, variable-weight goods, deposits.
- **Code lists** — the allowed values for coded fields, with meanings.

The rulebook carries its own version number, its publisher, and its publisher's signature — because the rulebook's author (usually the retailer, or Procuros on their behalf) is often not the message's sender (a supplier sending an invoice follows the *retailer's* rulebook).

### 4. How to respond
The choreography: which documents may answer this one (an order may be answered by an order response, then a despatch advice, then an invoice), what the response **must copy** from this message ("echo my order number into your field X, keep my line numbers"), what the responder **fills in itself** (accepted quantities, delivery date, batch numbers), which decisions are allowed (accept / reject / change, from a fixed list), and the deadline for each response.

### 5. Connections
How to reach the sender: which channels they accept (AS2, SFTP, API, email), the addresses, the certificates. Marked clearly as *informational*: a reader uses these details for a first contact, but any **change** to details it already has on file is treated as a proposal (see safety rule 2), never applied silently.

### 6. The answer key
Example messages: at least one fully correct example, and deliberately wrong examples, each labeled with the exact rule numbers it must trigger. This is what makes everything testable: any translation, any checker, any reader built against this rulebook can prove itself against the answer key before anyone relies on it.

## The three safety rules

These are not optional; they are what keeps the supermessage from dying the way its ancestors died.

1. **Everything is signed, with key pairs.** Every company has a key pair: a private key it keeps secret and signs with, and a public key anyone can use to verify. The file as a whole is signed by the sender; the rulebook section is signed by its publisher. The reader trusts signatures, not the messenger — a tampered file or a counterfeit rulebook is detected before anything else happens. A directory (a "phone book" of public keys — operated by Procuros at first, handable to an industry body later) vouches that a public key really belongs to the company it claims.
2. **A message can propose, never apply.** New rulebook version, new delivery address, new bank account — a supermessage can carry the proposal, with its signature and effective date. The receiving software applies it only after verifying the signature, and for anything sensitive (money, connectivity), only after a human confirms. This closes the fraud door that "live configuration in the message" would otherwise open.
3. **Rules check, they never act.** The if-then rules are written in a deliberately limited language that can test values ("is the best-before date present? is the sum of lines equal to the total?") but cannot run programs, reach the network, or touch files. A malicious supermessage cannot harm whoever opens it.

## First contact and repeat sends

- **First contact** (and any archived copy): the supermessage is complete — all six sections, rulebook included in full. Whoever finds this file in ten years can still understand it entirely.
- **Repeat sends**: once the receiver has confirmed "I have rulebook version 3, fingerprint so-and-so," both sides may agree to shorten section 3 to just that fingerprint. The fingerprint guarantees byte-for-byte the same rulebook; the full text can always be fetched or re-sent. This is an optimization both sides opt into — the format's default is complete.
- **Archiving rule**: anything stored for legal purposes (invoices especially) is stored complete, never shortened.

## The reader

The companion to the format: one small program (the "reader") that can, given any supermessage —

1. verify the signatures,
2. check the content against the rulebook and report any broken rule by number, in plain language,
3. display the whole file in human-readable form,
4. generate the skeleton of a chosen response with all the "must copy" fields already filled,
5. hand the response to the sending channel declared in section 5.

The reader is deliberately minimal. All the intelligence lives in the file; the reader just obeys it. That is what makes adoption cheap: implementing the reader (or embedding it in an ERP plugin) is a small, well-defined job, and it never changes when rulebooks change.

A working prototype of the reader — grown into a full exchange node with routing, a human review queue for every proposed change, and a guided two-company demo — exists: see [NODE.md](NODE.md).

## How this fits the build plan

This specification does not replace the order of work in [DECISION.md](DECISION.md) — it completes it. The rulebook library, the checker, the workbench, the test portal, and the change board are how rulebooks get created, proven, and governed. The supermessage is the **traveling form** of the same rulebook: the packaging that lets it leave our walls as a portable, signed thing that anyone's software can open. Concretely: section 3 of a supermessage *is* a rulebook from the library; the checker and the reader validate with the *same* rules; the answer key in section 6 is the same one the test portal uses. One artifact, two homes — the library at rest, the supermessage in motion.

## Decisions settled after the first draft

- **Signing: public–private key pairs.** Each company signs with its own private key; anyone verifies with the matching public key (using a modern, widely used signature algorithm — Ed25519). A public-key directory vouches for who owns which key: Procuros operates it first, and it can be handed to an industry body later. The one thing still open here is that long-term governance handoff, not the scheme.
- **Legal invoices: embed the legal document.** A supermessage carrying an invoice embeds the complete legally mandated invoice (for example, the EN 16931 e-invoice that German, French, and Belgian law require) as a subsection of its content. Two rules make this safe: the embedded legal invoice is the authoritative one wherever the two could disagree, and the rulebook must contain a check that the supermessage's own fields agree with it. Archiving stores the embedded legal invoice. One caveat stays: where the law prescribes a *transmission channel* (France's certified platforms, Italy's exchange system), embedding solves the format but the channel must still be the legal one — that is handled by the operator carrying the message, not by the file.
- **The rule language: CEL.** Rules are written twice in every rulebook, on purpose: once as a plain-language sentence (for people), and once in CEL — a small, widely used checking language, proven safe in very large systems, which can only test values and can never act, reach the network, or touch files (which is exactly safety rule 3). The plain sentence explains; the CEL expression is what actually runs; a rule missing either half is invalid. The worked example shows both halves side by side.
- **The name: supermessage.** Confirmed.
