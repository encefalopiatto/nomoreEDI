# A self-describing exchange language to replace EDI — design assessment

*Assessment of the proposal to build a radically self-describing B2B exchange language — "the message is the config file" — intended to replace EDI guidelines, format guides, and bilateral testing entirely. Research base: five adversarially fact-checked research lenses (prior art, PEPPOL/regulation, adversarial critique, steelman architecture, adoption economics) plus a response-choreography lens and internal ground truth from Atlas. August 2026.*

---

## Verdict

**The instinct is right. The mechanism as stated is wrong. The fix is well-defined, and it keeps the disruptive ambition intact.**

The instinct — trade requirements, validation rules, scenario conditionality, response construction, and connectivity should be *executable data that machines enforce*, not PDFs that humans interpret and test against for weeks — is correct, and it attacks exactly the right cost. Procuros' own onboarding decomposition (Atlas, directional): **bilateral partner testing 30–40%, business/master-data alignment 30–40%, mapping 15–25%, syntax 5–15% — and partner turnaround sets the go-live date.** The features of this idea that digitize requirements, compute conformance, and declare responses attack the dominant 60–80%. That is the disruption.

The mechanism as stated — every message literally embeds its full schema/rules/connectivity, receivers auto-build maps from one received message, in-band messages live-modify configuration — fails on five independent grounds, each with verified historical precedent: it has been tried (§2), one message cannot carry a relationship's rules (§3.2), counterparty-shipped executable config is a security and fraud anti-pattern (§3.3), invoices are legally format-locked in exactly our market (§3.4), and self-description attacks only the minority cost layer if the goal is "no maps" (§3.1).

The version that survives scrutiny — call it **self-describing by reference**: signed, versioned, content-addressed *Trade Contracts* that every message points to by hash, executed by a minimalist runtime, with computed conformance replacing partner-in-the-loop testing, response affordances declared per message, and change/enrichment as signed contract-version proposals — is buildable, differentiated, and maps almost feature-for-feature onto the original vision (§8). Its two load-bearing bets are named in §7, with the experiments that decide them.

---

## 1. What the idea claims, precisely

Accumulated across the discussion, the proposal is:

1. **Self-description**: every message carries its own schema, field semantics, and validation/business rules, so receiving software constructs field→meaning mappings automatically from a received message. No bilateral EDI testing against PDF specification guides.
2. **Scenario conditionality**: the format expresses conditional requirements — presence of field X makes Y mandatory/optional/unused — covering all business cases (returns, credit notes, variable weight, …).
3. **Response declarations**: every message declares how to build the legal responses to it (which documents may follow, which fields must be echoed, which the responder fills, deadlines).
4. **In-band lifecycle**: structured change requests to existing mappings, enrichment requests (ask the sender for missing fields), live config changes.
5. **Connectivity in-band**: transport details (AS2, X.400, SFTP, mail, …) travel in the messages.
6. **The message is the config file**: first configuration, live changes, scenarios — everything a minimalist piece of software needs to handle connectivity and processing between parties.
7. **No complex normalization/denormalization.**
8. **Goal**: appealing to every B2B player, easy to migrate to, adopted eventually by giants; complete disruption of the current way of working.

## 2. Thirty years of prior art, adversarially verified

Every component of this idea has been attempted. The pattern across all of them is stable and brutal: **machine-readable self-description reliably eliminates the syntax layer and never the semantics layer** — and formats win by forcing functions, never by merit.

| Prior art | What it tried | What happened | Lesson for this design |
|---|---|---|---|
| **XML, 1998–2003** | "Self-describing data will replace EDI" — the identical pitch | Failed; spawned cXML, xCBL, ebXML, RosettaNet, which recreated the mapping problem in new syntax ([xml.com, 1999](https://www.xml.com/pub/1999/08/edi/index.html)) | Self-description ≠ shared meaning |
| **ebXML CPP/CPA, 2001–02** | The closest ancestor: machine-readable partner profiles incl. **transport bindings** (connectivity-in-profile), machine-formed agreements, *envisioned automated negotiation* | OASIS Standard Dec 2002, ISO 15000. The negotiation layer never shipped; CPAs were hand-written; the market chose dumb AS2 + human mapping. CPPA v3 (2020) reached only Committee Spec with negligible uptake | In-band change requests = CPA negotiation reinvented. Must answer "why now": see §7 |
| **X12 SEF, 1990s** | Complete machine-readable implementation guides **including conditional inter-field dependencies** — feature #2 of this proposal, solved ~1993 | Niche tooling format; the industry kept exchanging PDFs for 30 more years | Format availability was never the bottleneck; *enforcement* is |
| **Stedi Guides, 2022** | Modern SEF: JSON-Schema machine-readable X12 guides powering validation/translation | Works, demonstrably compresses parse/validate — and Stedi still sells managed mapping and onboarding | Best current evidence for what self-description eliminates (syntax) vs what persists (binding, trust) |
| **RosettaNet PIPs, 1999–2013** | Machine-readable two-way choreographies with response obligations and deadlines (Time-to-Acknowledge/Perform, retries — "Business Activity Performance Controls") | Worked in production high-tech supply chains; never spread beyond the vertical; certification testing still required; folded into GS1 US end-2013 | Response declarations (feature #3) are proven implementable — and proven insufficient for adoption |
| **Semantic Web / GoodRelations** | Publish machine-readable semantics; machines align | Publishing semantics ≠ aligning them. schema.org won only because one asymmetric consumer (Google) paid publishers in ranking | Alignment is the cost; publication is cheap |
| **ISO 20022** | Central dictionary + machine-readable e-repository | Banks still needed multi-year human working groups (CBPR+, HVPS+) to restrict optionality before interoperating | Dictionaries permit optionality; **optionality is where mapping lives** |
| **PEPPOL / EN 16931** | The actual EDI-displacement success in Europe | Killed bilateral testing by the *opposite* mechanism: one fixed profile, centrally published executable Schematron validators (~983 rules UBL / ~808 CII, v1.3.16), registry discovery (SML/SMP publishes each receiver's document types, endpoint, certificate), certified access points — plus legal mandates. Variance still crept back through governed channels (country CIUS, XRechnung Leitweg-ID) | **Computed conformance against published artifacts is the proven cure for bilateral testing.** Per-message schema freedom is the disease it cured |
| **OpenAPI** | Machine-readable contracts for APIs | Won — because one side dictates, the contract is executable (codegen/mock/validate), and there is no negotiation | Retail EDI also has one dictating side (the retailer's MIG). The blocker is incentive, not feasibility |
| **HL7 FHIR** | Receiver-published machine-readable profiles + executable validators + automated conformance testing (Inferno) | Works at scale — the living proof of the §5 architecture. Also: needed a US regulatory mandate, profiles proliferated, certification persisted | Both the feasibility proof and the adoption warning |
| **EDIFACT APERAK / X12 824** | In-band, structured business-level rejection messages — feature #4's rejection half | Standardized decades ago; barely implemented | An in-band loop adopts itself only if the runtime generates and consumes it for free; every prior version required per-ERP implementation and died |
| **HATEOAS / Siren / HAL-FORMS / OpenAPI Links** | Messages declaring the next legal actions and how to construct them | Technically adequate, marginally adopted — pre-LLM clients were hardcoded anyway. Current analyses argue agentic clients invert this | Feature #3's syntax exists (OpenAPI Links runtime expressions ≈ echo bindings); the consumer that makes it pay (an agent) only just arrived |
| **TradeLens, embeddings-as-format, SAP CDC agents** | The other AI's suggestions | TradeLens: dead 2023 despite Maersk+IBM. Embeddings: non-deterministic, non-invertible, unauditable — no VAT regime accepts "approximately this invoice". CDC on a retailer's SAP: CISO/SOX/licensing non-starter | Discard all three |

## 3. Why the strong form fails

### 3.1 "No maps" attacks the minority cost

A map does three jobs: **syntax translation** (EDIFACT→structure; commodity, fully solved by self-description), **schema alignment** (their field ↔ canonical field; helped), and **business-semantics reconciliation** — which self-description cannot touch, because it consists of commitments about counterparties' *processes*, not message facts. Verified retail examples: is "delivery date" DTM+2, +17, +63 or +64, and does it mean depot arrival or dispatch; unit price net or gross of ALC allowances; which of four GLNs (NAD+BY/DP/IV, payer) gets invoiced across Edeka/Rewe cooperative legal entities; line-level vs document-level VAT rounding; ORDRSP echoing buyer line numbers; INVOIC validating only against previously loaded PRICAT/GDSN master data; each/case/layer/pallet ambiguity on the same GTIN; batch/best-before (DTM+361) for fresh food; Pfand/RTI deposit lines. A perfectly self-labeled field still needs the parties to *agree what the business means by it*.

Atlas confirms the cost split internally: mapping 15–25%, syntax 5–15%. **A pitch aimed at "no maps" attacks at most a quarter of the problem. The pitch aimed at "no partner-in-the-loop testing and no ambiguous requirements" attacks the dominant three quarters.** Same technology, different target — this reframing is the single most important conclusion in this document.

### 3.2 One message cannot carry the rules

A received message exhibits one path through the optional and conditional structure — no promotions, returns, credit notes, consignment, variable weight, year-end edge cases. Your own scenario-conditionality requirement (feature #2) concedes this: conditional rules are *intensional*, and no instance stream short of exhaustive enumerates them. A map inferred from an instance fails **silently**, and silent failure in order-to-cash means wrong payments at scale — which is precisely why EDI testing exists (it validates money flows, not parsing) and why enterprises would re-impose certification, recreating the onboarding this design abolishes. The rules must therefore travel as a complete declared artifact — at which point you are shipping a *contract*, and the only engineering question left is whether to embed it per message (50–500KB against 2–20KB typical ORDERS payloads, re-parsed and re-trusted on every message) or reference it by content hash (~100 bytes, immutable, cache-forever, first-contact messages may inline it). Reference wins on every axis, and **nothing observable is lost: a naive client receiving one message can still resolve everything needed to process it and respond.** "The message is the config file" survives as the design invariant; literal embedding does not.

### 3.3 Counterparty-shipped executable config is an attack surface

Self-describing, sender-controlled document machinery is where XML's worst CVE classes came from (billion-laughs, XXE). A message that carries executable validation is a DoS vector (pathological rules) and a trust inversion (sender grades its own homework — receivers re-validate with their own rules anyway, yielding dueling rule sets and a meta-dispute layer). A message that carries *live config changes* — new endpoint, new bank details, new invoicee GLN — is the invoice-redirection/BEC fraud pattern with a machine-speed autoloader attached; and most SME endpoints are unattended, so in-band negotiation often has no human responder at all. Every one of these has the same fix: **rules in a deterministic, non-Turing-complete, cost-bounded language (CEL — production-proven for exactly this threat model in Kubernetes admission control since v1.26); contracts signed by an identity anchor and published/resolved, not injected; config changes as signed version proposals with effective dates, applied by policy, never silently.** Connectivity bootstrap additionally requires an out-of-band anchor by construction — you need connectivity to receive the message declaring connectivity. PEPPOL's SML/SMP (DNS-discoverable registry of each participant's document types, endpoint, certificate) is the proven shape; ebXML CPP already carried transport bindings in 2002, so feature #5 is legitimate *as published, signed capability data* — it is only fatal as auto-applied message content.

### 3.4 The law has pre-awarded the invoice

Germany: every business must receive EN 16931 e-invoices since 1 Jan 2025; issuance mandatory 1 Jan 2027 (>€800k turnover) and 2028 (all). France: receive-all + issue (large/mid) 1 Sep 2026, SMEs 1 Sep 2027, exclusively via ~147 certified platforms. Belgium: PEPPOL B2B since 1 Jan 2026. ViDA (adopted 11 Mar 2025): EN 16931 e-invoicing + digital reporting for intra-EU B2B from 1 Jul 2030, domestic harmonization by 2035. A new language either projects into EN 16931 or produces legally void invoices. **The open, unregulated space is exactly where retail EDI lives: ORDERS, ORDRSP, DESADV/SSCC, PRICAT** — no mandate tailwind there, but no legal lockout either, and PEPPOL's ordering/logistics profiles are still thin (its network runs overwhelmingly one-way billing).

### 3.5 Normalization does not disappear; its authorship does

Posting, three-way match, and inventory run in each ERP's fixed internal model; every inbound document must be projected into it. True P2P with no shared intermediate semantics relocates O(N²) pairwise alignment into every receiver — EDI's disease restated; hubs and canonical models exist because N×1 beats N×M. What *can* die is the hand-written normalization layer: with contracts as compilation targets, projections are **generated** (and regenerated on contract change), not authored and maintained by humans. "No complex normalization" is achievable as "no hand-written normalization."

## 4. Internal ground truth (Atlas)

Effort decomposition (directional, not measured time tracking): business/master-data alignment 30–40%; bilateral partner testing 30–40%; JSONata mapping 15–25%; syntax/normalizer config 5–15%. Partner turnaround sets the go-live date even when mapping is quick.

Top post-go-live failure causes (practical ranking): (1) master-data drift — GLNs, ship-to/bill-to, product IDs, VAT data, pack sizes changing after go-live; (2) **silent partner requirement changes** — EDIFACT rules, mandatory fields, filename/SFTP conventions, validation changed without notice; (3) untested business scenarios — returns/credit notes, partial deliveries, substitutions, variable weight, Pfand, mixed VAT, zero/negative values; (4) code-list/reference-data changes; (5) mapping assumptions that held only for test samples; (6) operational/config drift — credentials, certificates, endpoints; (7) volume/timing. Root cause: weak change control after go-live.

Read against the design: **every top failure cause maps onto a specific contract feature.** (1)+(3) → master-data preconditions and scenario rules declared and checkable in-contract; (2) → the versioned change protocol with overlap windows and capability ACKs; (4) → registry-versioned code lists; (5) → conformance against declared rules + test vectors instead of inference from samples; (6) → signed connectivity capability updates. The industry-wide numbers agree: onboarding runs $750–2,500 setup per partner plus often $2,000–5,000/yr, 6–12 weeks calendar (Orderful, Cleo) — while deduction/dispute administration consumes 3–8% of suppliers' retail sales (Amazon ~7% of vendor revenue; Walmart OTIF 3% of COGS). **The economically dominant prize is dispute prevention and change control, not mapping automation.**

## 5. The version that survives: self-describing by reference

One artifact class carries the whole vision. A **Trade Contract** is a versioned, signed, content-addressed bundle, published per receiver-profile (one per retailer × document type, with thin per-supplier overlays — the dictating side authors it, as with OpenAPI; Procuros authors on behalf of the long tail):

1. **Shape**: JSON Schema 2020-12.
2. **Semantics**: per-field URIs into a shared registry that reuses EN 16931 business terms (BT-1…BT-161), UN/CEFACT CCL (~1,330 BIEs), GS1 identifiers and code lists. Zero invented meanings — inventing semantics is how every failed standard died.
3. **Rules**: CEL — deterministic, non-Turing-complete, linear-time, cost-bounded, safe for counterparty-authored logic. This is feature #2 delivered: `has(msg.batch) ? msg.bestBefore != null : true`, cross-field arithmetic (sum of lines = total), master-data preconditions ("prices must match PRICAT digest ≥ X"). EN 16931's ~983 executable Schematron rules prove requirements-as-code works across thousands of heterogeneous implementers; CEL generalizes it beyond XML.
4. **Response declarations** (feature #3): allowed next documents with correlation keys, **echo bindings** as source-pointer→target-field expressions (OpenAPI Links precedent), typed holes for the responder's genuine business decisions (accept/reject/partial/substitute — enumerated codes, UNCL4343-style), deadlines and retry semantics (RosettaNet BAPC precedent: the one choreography mechanism that survived in production). The compilable/human split is crisp: correlation, echoes, structure, deadlines are mechanical; decisions, quantities, dates, SSCCs are holes.
5. **Connectivity** (feature #5): a signed capability section — transports (AS2, SFTP, AS4/PEPPOL, API, mail), endpoints, certificates — published and resolved SMP-style, updated only by signed contract versions with effective dates.
6. **Test vectors**: mandatory valid/invalid instances with expected clause-level error codes. The contract is its own test oracle.
7. **Legacy projection**: bindings from each semantic term to EDIFACT segment/element positions (BT-1 ↔ BGM C106.1004…), from which both the human-readable MIG **and the executable wire translator are generated**. EN 16931's normative UBL/CII bindings prove semantic-model→syntax projection is a solved pattern.

Every message carries `contract: sha256:…` (+ version); first-contact messages may inline the bundle — the TLS-handshake pattern. The runtime stays minimalist precisely because the contract carries the complexity: resolve, verify signature, validate, project into the ERP binding, construct responses by filling declared holes. That runtime — distributed through ERP app stores — is the product form of "minimalist software that handles everything between parties."

**Go-live without partner-in-the-loop testing**: receiver publishes contract → sender's toolchain (LLM-assisted) compiles the ERP↔contract binding, iterating until the contract's validators and test vectors pass — mapping becomes *compilation with tests* → a neutral hosted conformance service returns clause-level verdicts and a signed attestation → effective date. The partner leaves the critical path, which per Atlas is what actually sets go-live dates. Trust is preserved because conformance is computed by a neutral service against the *same published artifact* both sides see — not sender self-grading, and no dueling rule sets.

**Change protocol** (kills Atlas failure causes #2 and #6): v2 = new digest + machine-computed structural diff (Confluent schema-registry precedent) **plus declared semantic-change annotations** — structural diffing cannot classify a meaning shift with unchanged shape (net→gross price basis), so semantic changes must be declared and are the one place human review remains — mandatory overlap windows validating both versions, cutover gated on signed capability-ACKs. Silent breaking changes become protocol violations.

**Enrichment/rejection** (feature #4): structured messages citing contract clause IDs — `{clause, path, verdict, severity, example, ask}`; an enrichment ask ("add GTIN at line level") is formally a contract change proposal, reusing the same protocol. Learn from APERAK/824's corpse: this loop lives only if the runtime emits and consumes it automatically — it must never require per-ERP implementation.

**Topology**: canonical *semantics* survive as the registry (N×1 authoring); hand-written runtime normalization dies (generated). The hub's data model persists; the hub's manual labor is what gets eliminated — including, honestly, Procuros' current moat, replaced by a better one: **the contract corpus.** Procuros' live JSONata mappings parse to ASTs; field paths, conditionals, coercions, and code-list translations can be statically mined into draft contracts, back-tested against message history, covering real Edeka/Rewe-tier requirements from day one. No competitor and no standards body has that corpus. (Generate synthetic test vectors from schemas+rules rather than anonymizing production traffic — real prices and volumes in test vectors are a GDPR/confidentiality landmine.)

## 6. Adoption physics

Verified history is unambiguous: **no B2B message standard has ever spread by spontaneous bilateral adoption.** Walmart's AS2 mandate (Sept 2002, $300/yr subsidized client) was a transport swap that cut Walmart's own VAN costs — semantics untouched. Amazon still requires classic EDI for core Vendor Central flows; SP-API only for greenfield programs. Tradacoms — frozen 1995, unsupported since 2017 — still runs at major UK retailers thirty years after a better successor existed. The fast format resets were all legal mandates (Italy SDI 2019, Mexico CFDI 2014, Belgium 2026), and they reset one regulated document, not order-to-cash. Meanwhile chargebacks partially function as retailer profit centers, so "fewer disputes" pitched at retailer finance can be a *negative* — pitch supply-chain and merchandising cost owners on what the giant itself bleeds: long-tail supplier onboarding speed, dropship expansion, master-data quality, empty-shelf risk from failed orders.

The wedge, in order:
1. **Unilateral adoption via the bridge**: contracts compile to each partner's EDIFACT/X12/Tradacoms guide; Procuros operates the bridge; the legacy side never knows. Any design requiring simultaneous bilateral adoption is dead on arrival — this is the single hardest constraint.
2. **Seed from the live network**: the new format as an alternative serialization of Procuros' canonical flows — every existing customer is a day-one endpoint; the hub is every early adopter's universal counterparty.
3. **Open spec + free hosted conformance validator** (the PEPPOL/OpenAPI playbook); ERP app-store runtimes (SAP, Business Central, Odoo, Xentral) as distribution.
4. **Ride the 2026–2030 mandate wave** as compliance-adjacent tooling — every EU business must touch its document stack; EN 16931 projection makes the invoice leg legal — while the *product's* home turf is unregulated O2C where EDIFACT actually lives.
5. **Giants come last**, pulled by their own cost lines once the long tail speaks the language — never first, never for elegance.

Governance honesty: a Procuros-owned "open" format caps out at a walled garden (cXML/Ariba precedent); rival networks won't adopt a competitor's registry. Plan the handoff: open-source the spec and validators early, court GS1/UN-CEFACT for the registry's semantic layer, keep the corpus and conformance service as the commercial edge. A central registry also concentrates blast radius (one bad validator release breaks thousands of flows at once — EN 16931 Schematron regressions are precedent): versioned pinning, staged rollout, and rollback are launch features, not afterthoughts.

## 7. The two bets that decide it

1. **LLM compilation-with-tests works at finance-grade accuracy.** This is the entire "why now, not 2001" argument — ebXML failed because binding formal agreements to heterogeneous ERPs stayed artisanal; the claim is that LLMs collapse that cost *when the contract supplies the test oracle*. It is currently an untested hypothesis: no lens, benchmark, or pilot quantifies auto-binding accuracy or silent-failure rates, and decades of schema-matching research (OAEI) plateaued well below finance grade *without* executable oracles. **Experiment**: take 10 completed onboardings, reconstruct contracts from the JSONata corpus, auto-compile bindings, measure pass rates against held-out production traffic and — critically — the silent-wrong rate, not just the caught-error rate.
2. **The counterfactual is beaten.** If LLM mapping works, incumbents can point it at legacy PDFs and EDIFACT samples without any new format — Stedi, Orderful, and every AI-mapping startup attack from that side. The defense must be made real: an LLM binding against a *signed executable contract with test vectors* is **provable**; an LLM reading a PDF MIG is plausible. In money flows, provable vs plausible is the product. If contracts don't measurably beat PDF-fed AI mapping on silent-error rates, the format has no moat — the same experiment above answers this by running both arms.

And one bet you don't control: **whether the dictating side will publish.** Retail already has single-side dictation (the MIG); OpenAPI's success conditions are satisfiable *if* retailers publish executable contracts. The bridge means you don't need them to start — Procuros can author their contracts from its own mappings — but the endgame (retailers self-publishing) is an incentive problem, not a technical one, and should be priced as such.

## 8. Disposition of the original vision, feature by feature

| Original feature | Verdict | Surviving form |
|---|---|---|
| Message carries schema/semantics/rules | **Keep, by reference** | `contract: sha256:…` in every message; first contact may inline; observationally identical to "the message is the config file" |
| Build the map from one received message | **Drop** | Underdetermined and silently wrong; replaced by compile-from-contract with test vectors as oracle |
| Scenario conditionality (X present → Y mandatory) | **Keep** | CEL rules in-contract; the *enforcement* is new, the expressibility was never the bottleneck (SEF had it in the 90s) |
| Messages declare how to build responses | **Keep — strongest feature** | Echo bindings + typed decision holes + deadlines; RosettaNet BAPC and OpenAPI Links precedents; attacks the relationship-level pain PDFs encode worst |
| In-band change requests / enrichment | **Keep, hardened** | Signed contract-version proposals citing clause IDs; never auto-applied (BEC vector); runtime-native so it doesn't die APERAK's death |
| Live config changes in-band | **Reshape** | Signed capability updates with effective dates and policy-gated application |
| Connectivity details in the message | **Reshape** | Signed connectivity section, registry-resolved (SMP precedent); bootstrap needs an out-of-band anchor by construction |
| No EDI testing | **Reshape** | No *partner-in-the-loop* testing; conformance becomes a compile step against published artifacts + neutral attestation |
| Replace guidelines/format guides entirely | **Keep** | The MIG PDF is *generated* from the contract projection — the PDF dies as an authoring format and survives only as documentation output |
| No normalization/denormalization | **Reshape** | No *hand-written* normalization; projections generated from contracts; canonical semantics survive as registry |
| Minimalist runtime | **Keep** | Resolve→verify→validate→project→respond; ERP app-store distribution; the wedge |
| P2P, hub-less | **Reshape** | P2P wire, shared semantic registry; pure pairwise contracts are O(N²) — EDI's disease restated |
| Giants adopt because it's better | **Drop** | They never have; design for unilateral adoption + their own cost lines + mandate-wave timing |

## 9. Open questions

**For Atlas (relay when available):**
1. Are trade-partner requirements today captured in any structured machine-readable form, or only PDFs/MIGs + knowledge implicit in JSONata mappings? Roughly what share is tribal?
2. Connectivity mix across live connections (AS2 / SFTP / X.400 / SMTP / VAN / PEPPOL / API), how painful connectivity setup is relative to mapping/testing, and typical connectivity failure modes.
3. What acknowledgement machinery partners actually use (CONTRL, APERAK, custom): when a message is business-rejected (wrong price, unknown GLN), how does that reach the supplier today, and how often does it need human back-and-forth?
4. Median calendar time of an onboarding end-to-end, and what share of that is spent waiting on partner test sign-off specifically.
5. How often do partners change requirements per year (change-management event volume) — sizes the change-protocol value.

**Design questions still open:**
- Registry governance and neutrality path (when to hand the semantic layer to GS1/UN-CEFACT); who pays for registry operations.
- Liability framework: interchange agreements for auto-compiled bindings — who bears the loss when a generated projection mispays; the evidentiary role of the signed contract.
- Semantic-change classification: how much human review the change protocol genuinely requires (structural diffs are computable; meaning shifts are not).
- Direction asymmetry at the edges: who authors contracts for flows where the dictating side is a CSV-and-email SME (answer today: Procuros; answer at scale: TBD).

---

*Method note: findings above marked as verified were adversarially fact-checked against primary or independent sources by dedicated verification agents; corrected figures (EN 16931: 161 business terms, ~983/~808 rules; France ~147 certified platforms as of Aug 2026; Walmart AS2 savings accruing to both sides; Tradacoms "majority share" softened to "still running at major UK retailers") are reflected here. Atlas figures are directional estimates, not measured time tracking. The full research corpus with per-claim sources and verdicts lives in the session workflow outputs.*
