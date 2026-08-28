# Steelman architecture: Trade Contracts

*Research lens `architecture` — structured output of the research agent, followed by adversarial fact-check verdicts. Confidence labels are the agent's own; verdicts are from an independent verification agent with web access.*

## Summary

Steelman architecture: "Trade Contracts" — versioned, content-addressed artifacts published per receiver-profile (one per retailer document type, plus thin per-supplier overlays), never per-message blueprints. A contract bundles: (1) JSON Schema 2020-12 structure; (2) per-field semantic bindings as URIs into a shared registry reusing EN 16931 business terms, UN/CEFACT CCL BIEs, and GS1 identifiers/code lists — inventing zero new meanings; (3) business rules in CEL (deterministic, non-Turing-complete, cost-bounded, safe for counterparty-supplied rules, proven in Kubernetes admission control; JSONLogic lacks typed decimals/dates, JSON Schema alone cannot express cross-field arithmetic, Schematron proves the pattern in EN 16931 but is XML/XPath-bound); (4) mandatory test vectors — valid and invalid instances with expected clause-level error codes. Messages reference the contract by SHA-256 digest (~100 bytes) rather than embedding 50–500KB per message; content-addressing gives immutability, cache-forever resolution, offline verification, archival pinning; first-contact messages MAY inline the contract. Go-live without bilateral testing: receiver publishes contract → sender's LLM toolchain compiles ERP→contract mapping, iterating until the contract's validators and test vectors pass (compilation-with-tests) → self-service conformance API returns clause-level verdicts plus signed attestation → effective date set. Peppol's SML/SMP already proves registry-discovery-without-bilateral-agreements scales. Changes: v2 is a new digest plus computed diff auto-classified breaking/non-breaking; overlap windows validate both versions; cutover gates on signed capability-ACKs — eliminating silent breaking changes. Rejections and enrichment asks are structured messages citing clause IDs (BR-style), unlike syntax-only 997/CONTRL; an enrichment ask is formally a contract change-proposal. Why now: ebXML CPP/CPA (2001–02) had machine-readable partner agreements, but ERP binding stayed artisanal; LLMs collapse that cost because the contract is its own test oracle. Legacy bridge: contracts carry EDIFACT projections (BT↔segment bindings) compiled to MIGs and wire translators — Procuros operates the bridge, so adoption is unilateral. Topology: keep canonical SEMANTICS as registry (N-by-1 authoring); pure pairwise contracts are O(N²), EDI's disease restated; runtime normalization becomes generated, not hand-written. Bootstrap: statically mine Procuros' live JSONata mappings into per-partner contracts; anonymized traffic becomes test vectors. Hardest: unformalizable semantics (party roles, price basis, packaging hierarchies), retailer incentives, registry governance, tax/audit recognition.

## Key points

### 1. Artifact model: one versioned, content-addressed Trade Contract per receiver-profile (plus thin supplier overlays), referenced from messages by SHA-256 digest. Embedding schema+rules per message costs 50-500KB against ~100 bytes for a digest (100-1000x bloat on high-volume ORDERS/DESADV flows); hashes give immutability, infinite cacheability, offline verification, archival pinning.

- **Relevance:** Rescues 'self-describing' from per-message bloat: the message plus resolvable registry is self-describing; per-message embedding is the naive version critics will attack.
- **Evidence:** OCI/Docker image digests and git demonstrate content-addressed artifact distribution at scale; typical EDIFACT ORDERS payloads are 2-20KB, so an embedded contract would dominate every message.
- **Confidence:** `model-knowledge`

### 2. Semantic bindings must reuse existing dictionaries, not invent meanings: EN 16931 defines 176 business terms with stable IDs (BT-1=Invoice Number) and business groups; UN/CEFACT CCL provides ~1330 Business Information Entities (D21A); GS1 supplies identifiers/code lists. Contract fields carry URIs into these.

- **Relevance:** Stable, citable clause/term IDs are the substrate for machine-readable rejections, diffs, and LLM grounding; inventing new semantics would repeat every failed standard.
- **Evidence:** EN 16931 semantic model organizes 176 BTs into BGs with unique IDs and cardinalities; CCL D21A holds ~600 core components, ~1330 BIEs.
- **Confidence:** `verified-web`
- **Source:** https://peppolvalidator.com/en16931-business-terms and https://unece.org/trade/uncefact/unccl

### 3. Rule language: CEL. Non-Turing-complete, linear-time (no loops/recursion), cost-bounded, Go/Java/C++ runtimes, production-proven for evaluating third-party-supplied rules in-process (Kubernetes ValidatingAdmissionPolicy). Rejected: JSON Schema alone (no cross-field arithmetic like sum(lines)=total), JSONLogic (untyped decimals/dates, thin ecosystem), Schematron (proves the pattern but XML/XPath-bound).

- **Relevance:** Solves the 'counterparty rules must be safe to evaluate' constraint with an off-the-shelf, battle-tested component, and makes the design pick concrete enough to attack.
- **Evidence:** CEL designed for safe evaluation in security-sensitive environments; embedded in the Kubernetes API server since 1.26 to run user-supplied admission rules in-process.
- **Confidence:** `verified-web`
- **Source:** https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/

### 4. Machine-executable business rules at continental scale are proven, not speculative: CEN/TC 434 maintains official Schematron validation artefacts for EN 16931 (~955 rules), used design-time and run-time, republished with each Peppol BIS release.

- **Relevance:** Direct precedent that 'requirements shipped as executable code' works across thousands of heterogeneous implementers — the proposal generalizes this from invoices to all trade documents.
- **Evidence:** Official Schematron artefacts maintained by CEN/TC 434 on GitHub (ConnectingEurope/eInvoicing-EN16931), pre-compiled to XSLT 2.0, published with Peppol BIS releases.
- **Confidence:** `verified-web`
- **Source:** https://github.com/ConnectingEurope/eInvoicing-EN16931

### 5. Computed conformance replaces bilateral testing: publish contract → toolchain compiles mapping → local validation against test vectors → conformance API returns clause-level verdicts → signed conformance attestation → effective date. Peppol proves the registry pattern: SML/SMP dynamic discovery lets any participant exchange with any other without bilateral agreements or pairwise testing.

- **Relevance:** The end-to-end go-live story; Peppol is the existence proof that no-bilateral-setup networks scale in exactly this industry and geography.
- **Evidence:** Peppol four-corner model: sending AP resolves receiver capabilities via SML then SMP without prior bilateral configuration; OpenPeppol Testbed handles conformance self-assessment.
- **Confidence:** `verified-web`
- **Source:** https://peppol.org/learn-more/peppol-interoperability-framework/

### 6. Change protocol kills silent breaking changes: contract v2 = new digest + machine-computed diff auto-classified breaking/non-breaking (added-optional=minor; removed field, tightened constraint, changed semantics=major); mandatory effective-date overlap window where both versions validate; sender cutover gated on receivers' signed capability-ACKs listing supported digests.

- **Relevance:** Addresses EDI's worst operational pain — retailers announcing requirement changes via PDF/email and suppliers failing in production; classification is computed, not declared, so it cannot be gamed.
- **Evidence:** Semver-style computed compatibility checking is standard in schema registries (Confluent Avro/Protobuf compatibility modes); no equivalent exists in EDIFACT/X12 workflows, where MIG changes circulate as PDFs.
- **Confidence:** `model-knowledge`

### 7. Enrichment/rejection protocol: structured messages citing contract clause IDs — {clause, field-path, verdict, severity, example, ask} — where an enrichment ask ('add GTIN at line level') is formally a contract change-proposal, reusing the change protocol. Today's X12 997 and EDIFACT CONTRL acknowledge syntax only; business-level rejections travel by email/phone/portal.

- **Relevance:** Turns the receiver's 'this message is wrong/insufficient' from an out-of-band human process into an in-band machine loop; unifies enrichment with change management.
- **Evidence:** 997/CONTRL report envelope/syntax acceptance at transaction-set level and cannot reference business rules; EN 16931's BR-xx rule IDs show clause-level error codes are feasible.
- **Confidence:** `model-knowledge`

### 8. Why now, not 2001: ebXML CPP/CPA (OASIS+UN/CEFACT, TC formed June 2001, v2.0 ratified December 2002) already specified machine-readable partner profiles and agreements, yet never displaced EDI — because binding a formal agreement to each party's heterogeneous ERP internals stayed manual expert labor. LLM compilation-with-tests (contract validators + test vectors as oracle) removes exactly that cost.

- **Relevance:** The single strongest 'this time is different' argument; also the biggest risk if LLM mapping accuracy on long-tail semantics disappoints.
- **Evidence:** CPP/CPA history verified (OASIS ratification Dec 2002); the failure-mechanism analysis is inference — sources document the standard's existence, not its non-adoption causes.
- **Confidence:** `verified-web`
- **Source:** https://www.oasis-open.org/2002/12/01/ebxml-collaboration-protocol-profile-and-agreement-ratified-as-oasis-open-standard/

### 9. Legacy bridge enables unilateral adoption: contracts carry an optional EDIFACT projection section binding each clause to segment/element positions (e.g., BT-1 to BGM C106.1004, delivery date to DTM+2 format 102), from which both human-readable MIGs and executable wire translators are generated. A bridge operator — Procuros — translates for partners still on EDIFACT/X12/CSV.

- **Relevance:** Satisfies the unilateral-adoption constraint and positions Procuros' existing business as the bridge, making the protocol a wedge rather than a rip-and-replace bet.
- **Evidence:** EN 16931 maintains normative syntax bindings from one semantic model to two syntaxes (UBL, CII), proving semantic-model-to-syntax projection is a solved pattern; EDIFACT is one more target syntax.
- **Confidence:** `model-knowledge`

### 10. Topology verdict: canonical SEMANTICS survive as the shared registry (each party authors one contract: O(N)); true pairwise P2P contracts mean O(N squared) authoring and re-create EDI's bilateral disease. What dies is the hand-written runtime normalization layer: mappings compile contract-to-contract or contract-to-ERP directly, any intermediate representation generated rather than human-maintained.

- **Relevance:** Honest answer to hub-vs-P2P: the hub's data model persists as semantics; the hub's labor (hand-written maps) is what gets eliminated — including Procuros' own current moat.
- **Evidence:** N-by-M integration collapse via a shared intermediate model is why VANs, Peppol BIS profiles, and canonical hubs all converged on N-by-1; nothing in the proposal changes that combinatorics.
- **Confidence:** `model-knowledge`

### 11. Bootstrap: Procuros' hundreds of live JSONata partner mappings are statically analyzable (JSONata parses to an AST) — extract per-partner field paths, conditionals, format coercions, code-list translations to auto-synthesize draft contracts (schema + CEL rules); anonymized production traffic supplies test vectors; contracts back-tested against message history before publication. Day-one corpus covering real Edeka/Rewe-tier requirements.

- **Relevance:** The unique defensible asset: competitors and standards bodies lack a live corpus of battle-tested mappings plus real traffic to validate against; this makes Procuros the credible author.
- **Evidence:** JSONata is a declarative expression language with a public parser/AST; mining declarative mappings for schema and rule inference is tractable, unlike mining imperative integration code.
- **Confidence:** `model-knowledge`

### 12. Hardest unsolved parts: (a) semantics that resist formalization — party-role disambiguation (buyer vs invoicee vs ship-to), price basis/unit-of-measure, DESADV packaging hierarchies, German Pfand/deposits — two parties can both pass conformance yet disagree on meaning; (b) retail giants externalize testing cost onto suppliers, so their incentive must be supplier-onboarding speed; (c) registry governance/neutrality; (d) tax/audit legal recognition (GoBD, e-invoicing mandates).

- **Relevance:** These, not the protocol mechanics, are where the design most likely fails; the synthesizer should weight the incentive asymmetry heaviest.
- **Evidence:** EN 16931 needed a decade plus an EU directive to force one document type into one semantic model, and 955 executable rules still did not eliminate interpretation disputes.
- **Confidence:** `model-knowledge`

## Verification verdicts

### `PLAUSIBLE` — Content-addressed Trade Contract referenced by SHA-256 digest; embedding schema+rules costs 50-500KB vs ~100 bytes digest (100-1000x bloat); typical EDIFACT ORDERS 2-20KB

Design opinion, not web-checkable. Anchors are sound: a sha256: digest string is ~71-100 bytes; OCI/git content addressing is real precedent; 2-20KB ORDERS payloads and 50-500KB for full schema+rule sets are consistent with practice (the EN 16931 UBL schematron alone is several hundred KB) but no independent source quantifies them.

*Sources: https://github.com/ConnectingEurope/eInvoicing-EN16931*

### `CORRECTED` — EN 16931 defines 176 business terms with stable IDs (BT-1=Invoice Number) and business groups; UN/CEFACT CCL D21A ~1330 BIEs; GS1 supplies identifiers/code lists

The claim's own cited source (peppolvalidator.com) states 161 BTs across 30 BGs; the official CEN/TC 434 validation artefacts reference BT IDs only up to BT-161 and BGs up to BG-32. UNECE forum material confirms D21A: 1,330 ABIEs, ~600-class Object Class Library.

**Correction:** EN 16931-1 defines 161 business terms (BT-1 through BT-161, BT-1=Invoice number, 1..1) organized into business groups up to BG-32; the '176' figure appears only in SEO blogs. CCL part checks out: D21A holds 1,330 ABIEs over ~600 object classes. GS1 part is trivially true.

*Sources: https://peppolvalidator.com/en16931-business-terms ; https://unece.org/sites/default/files/2021-05/M-T_LIB-Maintenance-CCL-RDM.pdf ; rule-ID census of github.com/ConnectingEurope/eInvoicing-EN16931*

### `CONFIRMED` — CEL is non-Turing-complete, linear-time, cost-bounded, with Go/Java/C++ runtimes, production-proven for third-party rules in-process (Kubernetes ValidatingAdmissionPolicy); JSON Schema lacks cross-field arithmetic; Schematron is XML/XPath-bound

cel-spec states CEL 'evaluates in linear time, is mutation free, and not Turing-complete'; official runtimes cel-go, cel-cpp, cel-java exist; Kubernetes ValidatingAdmissionPolicy evaluates user CEL in the API server (alpha v1.26, stable v1.30) with runtime cost budgets. Minor nuance: CEL has bounded comprehension macros (all/exists/map), so 'no loops' means no unbounded loops.

*Sources: https://github.com/google/cel-spec ; https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/*

### `CORRECTED` — CEN/TC 434 maintains official Schematron validation artefacts for EN 16931 (~955 rules), used design-time and run-time, republished with each Peppol BIS release

Verified by cloning the repo and counting assert/rule IDs: UBL preprocessed schematron 979 asserts/983 unique IDs, CII 806/808. Repo confirmed as official CEN/TC 434 artefacts covering UBL 2.1, CII D16B, and optional EDIFACT.

**Correction:** CEN/TC 434 does maintain official Schematron artefacts (ConnectingEurope/eInvoicing-EN16931, pre-compiled to XSLT), but the rule count is per-syntax and version-dependent: current v1.3.16 (Apr 2026) has ~983 unique rules for UBL and ~808 for CII (223 shared BR-xx semantic rules). '~955' matches no current artefact. Also, the artefacts have their own GitHub release cycle; OpenPeppol bundles/aligns them in its biannual BIS Billing releases rather than CEN republishing per BIS release.

*Sources: https://github.com/ConnectingEurope/eInvoicing-EN16931 (v1.3.16, counted directly)*

### `CONFIRMED` — Peppol SML/SMP dynamic discovery lets any participant exchange with any other without bilateral agreements or pairwise testing (four-corner model); computed-conformance pipeline generalizes this

Peppol's four-corner model with central SML and per-participant SMPs provides dynamic discovery of receiver capabilities and endpoints, eliminating bilateral setup — confirmed. The proposed conformance pipeline (clause-level verdicts, signed attestations) is the design's own contribution, not an existing fact; the Peppol precedent it leans on is real.

*Sources: https://en.wikipedia.org/wiki/PEPPOL (peppol.org blocked by bot-verification wall)*

### `PLAUSIBLE` — Change protocol: digest-versioned contracts with machine-computed breaking/non-breaking diff classification, overlap windows, capability-ACKs; precedent is Confluent schema-registry compatibility modes; EDIFACT/X12 MIG changes circulate as PDFs

The protocol itself is the proposal, not a checkable fact. Its anchors hold: Confluent Schema Registry computes BACKWARD/FORWARD/FULL(+transitive) compatibility on schema registration (confirmed); MIG/spec changes in EDIFACT/X12 practice circulating as PDFs/portals is accurate industry knowledge with no single citable source.

*Sources: https://docs.confluent.io/platform/current/schema-registry/fundamentals/schema-evolution.html*

### `CONFIRMED` — X12 997 and EDIFACT CONTRL acknowledge syntax only; business-level rejections travel out-of-band; EN 16931 BR-xx rule IDs show clause-level error codes are feasible

997 reports results of syntactical analysis of transaction sets and explicitly not semantic/business content (Microsoft BizTalk docs, 1EDISource); CONTRL is the EDIFACT syntax-level acknowledgment. BR-xx rule IDs confirmed: 223 distinct BR-* rules in the official EN 16931 schematron.

*Sources: https://learn.microsoft.com/en-us/biztalk/core/x12-997-acknowledgment ; github.com/ConnectingEurope/eInvoicing-EN16931*

### `CONFIRMED` — ebXML CPP/CPA (OASIS+UN/CEFACT, TC formed June 2001, v2.0 ratified December 2002) specified machine-readable partner profiles/agreements but never displaced EDI; LLM compilation removes the manual binding cost

OASIS formed the ebXML TCs (including CPPA) June 20-21, 2001, continuing the OASIS+UN/CEFACT ebXML initiative; CPPA v2.0 (spec dated 23 Sept 2002) was ratified as an OASIS Open Standard, announced December 2002. Non-displacement of EDI is historically accurate; the causal mechanism (manual ERP binding labor) and the LLM remedy are inference, as the claim itself flags.

*Sources: https://www.oasis-open.org/2001/06/20/oasis-forms-ebxml-technical-committees/ ; https://xml.coverpages.org/ni2002-12-03-b.html*

