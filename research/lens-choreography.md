# Response choreography: messages that declare how to answer them

*Research lens `choreography` — structured output of the research agent, followed by adversarial fact-check verdicts. Confidence labels are the agent's own; verdicts are from an independent verification agent with web access.*

## Summary

Prior art strongly supports per-message response declarations ("affordances") over global process choreography. The canonical success is the HTML form: the server ships a template with typed holes, hidden echo fields, a target, and a method; the client fills only business decisions. Machine-readable descendants — Siren actions, HAL-FORMS _templates, Hydra operations (still only a W3C Community Group draft), OpenAPI Links (runtime expressions like $response.body#/id mapping response values into a named next operation's parameters) — are technically adequate but saw marginal adoption because pre-LLM clients were hardcoded to known workflows, so runtime affordances cost effort without payoff. Current commentary argues agentic/LLM clients invert this: dynamic affordance discovery is exactly what agents need. Global choreography languages (ebXML BPSS, WS-CDL, BPMN choreography) failed or stayed on paper; WS-CDL died as an over-complex "simulation language." What survived: executable single-party orchestration (BPEL) and RosettaNet PIPs — paired request/response transactions with per-PIP Business Activity Performance Controls (time-to-acknowledge, time-to-perform, retries, non-repudiation), proving per-exchange response obligations with deadlines are implementable in production supply chains. In EDI today, response-construction rules live as prose in bilateral MIGs (ORDRSP echoing buyer line numbers/order reference, INVOIC carrying RFF+ON for 3-way match, DESADV CPS/SSCC pallet-carton hierarchy per GS1 EANCOM); verifying them is a core driver of 6–12-week onboarding test cycles. PEPPOL standardizes response semantics as separate BIS profiles (Ordering, Advanced Ordering, Invoice Response T111 with UNCL4343 status/action codes) validated by the same Schematron infrastructure as requests — but adoption is overwhelmingly Billing-only; mandates (e.g., Belgium 2026) cover invoices, and ordering/response profiles remain thin. Design split for the new language: compilable = correlation keys, echo bindings (source-pointer→target-field), allowed-next-document enumeration, deadlines, and response constraint sets checked by the same contract engine; irreducibly human/ERP = accept/reject/partial decisions, substitutions, actual quantities/dates — model these as typed holes with enumerated decision codes. Guard against choreography drift, request/response contract version skew (pin+hash), responder-capability mismatch (negotiate/downgrade), and deadlock from mandated responses nobody can send.

## Key points

### 1. HTML forms are the proven self-describing response mechanism: server sends typed fields, prefilled/hidden echo values, required flags, target URI and method; client fills only decision holes. This 'template with holes + echo fields' pattern maps directly onto declaring ORDRSP/DESADV/INVOIC construction.

- **Relevance:** Canonical design template for the response-declaration feature
- **Confidence:** `model-knowledge`
- **Source:** HTML spec / REST literature

### 2. Machine hypermedia affordances exist and work: Siren 'actions' (name, method, href, typed fields), HAL-FORMS '_templates', Hydra 'operations' (JSON-LD, W3C Community Group draft — never standardized). HAL (links-only) got most adoption; forms-capable formats stayed niche.

- **Relevance:** Direct prior art for embedding response specs in messages; shows spec adequacy was never the blocker
- **Evidence:** hydra-cg.com; Sookocheff hypermedia-format comparison; w3.org/community/hydra
- **Confidence:** `verified-web`
- **Source:** https://www.hydra-cg.com/ ; https://sookocheff.com/post/api/on-choosing-a-hypermedia-format/

### 3. Hypermedia M2M clients failed pre-LLM because clients were hardcoded to known workflows anyway — runtime affordances added cost without benefit, so providers omitted them. Current analyses argue LLM/agentic clients flip this: dynamic affordance discovery suits agents ('the API style that was waiting for AI').

- **Relevance:** Justifies why this feature can succeed now when HATEOAS failed; agentic mapping-builders are the new consumer
- **Evidence:** Nordic APIs essay; htmx HATEOAS essay; arXiv work on affordances/signifiers for autonomous agents
- **Confidence:** `verified-web`
- **Source:** https://nordicapis.com/hateoas-the-api-design-style-that-was-waiting-for-ai/ ; https://htmx.org/essays/hateoas/

### 4. OpenAPI Links objects are the closest mainstream precedent for compilable copy-rules: a response declares named follow-up operations with parameter bindings via runtime expressions (e.g. username: $response.body#/username). Design-time declarative, mechanically executable; tooling support remains weak.

- **Relevance:** Proven syntax pattern for source-pointer→target-field echo bindings
- **Evidence:** OAS 3.1 Link Object and runtime-expression grammar ($response.body#/json-pointer)
- **Confidence:** `verified-web`
- **Source:** https://spec.openapis.org/oas/v3.1.1.html ; https://learn.openapis.org/specification/links.html

### 5. Global choreography languages failed: WS-CDL stalled at W3C Candidate Recommendation (2005), criticized as an over-complex 'simulation language' with no mainstream implementations; ebXML BPSS (OASIS ebBP 2.0.4) defined binary collaborations with time-to-acknowledge/perform but adoption foundered on complexity. BPEL survived only as single-party orchestration.

- **Relevance:** Warns against a separate global process-definition layer; keep choreography local and per-message
- **Evidence:** W3C WS-CDL CR status; goland.org critique; OASIS ebBP 2.0.4 spec; CIO-wiki adoption obstacles
- **Confidence:** `verified-web`
- **Source:** https://www.w3.org/TR/ws-cdl-10/ ; https://www.goland.org/wscdl/ ; https://docs.oasis-open.org/ebxml-bp/2.0.4/

### 6. RosettaNet PIPs survived: each PIP (e.g. 3A4 Request Purchase Order) is a paired request/response with a Business Activity Performance Controls table — Time to Acknowledge, Time to Perform (typical 2h ack / 24h perform), retry counts, non-repudiation — enforced by B2B gateways in high-tech supply chains.

- **Relevance:** Production proof that per-exchange response obligations with deadlines belong in the message contract and are machine-enforceable
- **Evidence:** PIP spec Table 3-3 'Business Activity Performance Controls'; Oracle/IBM/Microsoft gateway docs implement these timers
- **Confidence:** `verified-web`
- **Source:** https://docs.oracle.com/cd/B10464_02/integrate.904/b12121/b2bstandards.htm ; https://learn.microsoft.com/en-us/biztalk/adapters-and-accelerators/accelerator-rosettanet/using-the-pip-specification-to-create-a-process-configuration

### 7. Today's echo rules live as prose in bilateral MIGs: ORDRSP must repeat buyer line numbers (LIN DE1082) and order reference (RFF+ON); INVOIC must carry RFF+ON for the buyer's 3-way match; DESADV must express pallet/carton hierarchy via CPS parent-child + SSCC per GS1 EANCOM. Humans verify these in test cycles.

- **Relevance:** The exact rules the new language should formalize; CPS/SSCC hierarchy and LIN structure verified in GS1 EANCOM DESADV docs, ORDRSP echo detail from domain knowledge
- **Evidence:** GS1 EANCOM 2002 DESADV guideline: first CPS = shipment, parent for next lower level; SSCC links child package to parent
- **Confidence:** `verified-web`
- **Source:** https://www.gs1.org/sites/default/files/docs/eancom/s4/desadv.pdf ; https://www.gs1.org/sites/default/files/docs/eancom/ean02s4/part2/desadv/0524.htm

### 8. EDI onboarding effort verified: industry sources cite 6–12 weeks per trading partner under legacy methods, dominated by manual mapping plus repeated bilateral test cycles surfacing mapping errors (rejected POs, failed invoices, inaccurate ASNs, chargebacks). No source quantifies echo-rules' exact share — support is directional.

- **Relevance:** Confirms the business case: response-construction correctness is a major tested failure surface
- **Evidence:** Orderful: 8–12 weeks average, 'week 5 first testing round finds 12 errors'; Cleo: 6–12 weeks
- **Confidence:** `verified-web`
- **Source:** https://www.orderful.com/blog/transform-edi-partner-onboarding ; https://www.cleo.com/how-to-onboard-edi-trading-partners-faster

### 9. PEPPOL standardizes response semantics as profiles: BIS Ordering 3 (order response: accept/reject/change), Advanced Ordering 3.0, Order Agreement, and Invoice Response 3.x (transaction T111, UBL ApplicationResponse) with UNCL4343 status codes (e.g. AP=approved) plus clarification/action codes — all validated by the same Schematron infrastructure as the request documents.

- **Relevance:** Proves 'response constraints validated by the same contract infrastructure' works at network scale; reusable code lists for responder decisions
- **Evidence:** docs.peppol.eu profile 63 Invoice Response, T111 syntax, UNCL4343-T111 codelist
- **Confidence:** `verified-web`
- **Source:** https://docs.peppol.eu/poacc/upgrade-3/profiles/63-invoiceresponse/ ; https://docs.peppol.eu/poacc/upgrade-3/syntax/InvoiceResponse/

### 10. PEPPOL adoption reality: Billing 3.0 dominates; mandates (Belgium B2B from Jan 2026, penalties from April 2026) cover invoices only. Ordering/Invoice Response profiles are voluntary and thin — Norway is still 'building on invoicing success' toward orders/catalogues. Response choreography exists on paper but the network runs mostly one-way invoices.

- **Relevance:** Cautionary: standardized response semantics without mandate or economic forcing see weak uptake; the new language must make responses cheaper, not just specified
- **Evidence:** Belgium 2026 mandate = Peppol BIS Billing; Norway country profile on post-award expansion
- **Confidence:** `verified-web`
- **Source:** https://peppolvalidator.com/peppol-belgium ; https://peppol.org/learn-more/country-profiles/norway/

### 11. Compilable vs human split: mechanical = correlation keys, echo bindings (JSONPath/pointer source→target), allowed-next-document enumeration, deadlines, arithmetic constraints (invoice qty ≤ despatched qty), response validation by the same engine. Irreducibly business/ERP = accept/reject/partial decision, substitutions, actual quantities/dates/SSCCs — model as typed holes with enumerated decision codes (UNCL4343-style).

- **Relevance:** Core design partition for the response-declaration feature; synthesis of HTML-forms, OpenAPI Links, and PEPPOL code-list patterns
- **Confidence:** `model-knowledge`
- **Source:** synthesis of verified prior art above

### 12. Failure modes to engineer around: choreography drift (declared vs actual behavior — mitigate by generating validators from the declaration and rejecting non-conforming responses), version skew (embed response-contract version/hash in the request; responses reference it), responder ERP unable to produce demanded structure (capability negotiation/graceful downgrade, as RosettaNet TPAs did), and deadlock from mandatory responses nobody sends (deadline + timeout-default semantics, escalation paths).

- **Relevance:** Directly answers lens (f); each mitigation has a named precedent
- **Confidence:** `model-knowledge`
- **Source:** synthesis: RosettaNet TPA parameters, PEPPOL optional-response experience, BPSS timeToPerform semantics

## Verification verdicts

### `CONFIRMED` — HTML forms are the proven self-describing response mechanism (typed fields, prefilled/hidden echo values, required flags, target URI and method; client fills only decision holes); this pattern maps onto declaring ORDRSP/DESADV/INVOIC construction.

The factual core is uncontroversial HTML: input types, hidden inputs carrying echo values, the required attribute, and form action/method are all standard HTML form features. The htmx HATEOAS essay independently confirms the framing — an HTML form response contains 'all the information necessary' to perform the follow-up action, unlike JSON links that need out-of-band knowledge. The mapping onto EDIFACT response construction is an analytical design opinion, not independently verifiable, but nothing found contradicts it.

*Sources: https://htmx.org/essays/hateoas/ ; WHATWG HTML form spec (common knowledge)*

### `CONFIRMED` — Machine hypermedia affordances exist: Siren 'actions' (name, method, href, typed fields), HAL-FORMS '_templates', Hydra 'operations' (JSON-LD, W3C Community Group draft, never standardized); HAL (links-only) got most adoption, forms-capable formats stayed niche.

All four format assertions check out. Hydra remains a W3C Community Group effort (hydra-cg.com explicitly invites joining 'the Hydra W3C Community Group'), never a formal W3C Recommendation. HAL-FORMS defines the '_templates' dictionary carrying HTTP method, content-type, and property/field arguments (media type application/prs.hal-forms+json). Siren defines Actions with method, href, fields — explicitly built to 'overcome the main drawback of HAL — support for actions.' Comparison literature (Zuplo, Sookocheff) confirms HAL as the most-adopted of these while Siren/Hydra stayed niche. Minor nuance: HAL itself was also never formally standardized (its IETF draft expired), and JSON:API rivals HAL in adoption — but neither contradicts the claim as scoped.

*Sources: https://www.hydra-cg.com/ ; https://zuplo.com/learning-center/a-deep-dive-into-alternative-data-formats-for-apis-hal-siren-and-json-ld ; https://sookocheff.com/post/api/on-choosing-a-hypermedia-format/ ; https://spring.io/blog/2018/01/12/building-richer-hypermedia-with-spring-hateoas/*

### `CONFIRMED` — Hypermedia M2M clients failed pre-LLM because clients were hardcoded to known workflows; current analyses argue LLM/agentic clients flip this — 'the API style that was waiting for AI'.

The Nordic APIs essay exists with exactly this thesis: HATEOAS overhead wasn't worth it for hardcoded clients, but LLM agents benefit from runtime affordance discovery, contextual next-step constraint, and state-based tool selection ('HATEOAS was just waiting for the right technology'). The htmx essay confirms industry rejection of JSON HATEOAS, though its causal diagnosis differs slightly — it blames JSON not being a natural hypermedia (vs. HTML) more than client hardcoding per se. arXiv/academic work on affordances and signifiers for autonomous agents exists as cited (arXiv:2302.06970 'Signifiers as a First-class Abstraction in Hypermedia Multi-Agent Systems'; arXiv:2510.24459 'Affordance Representation and Recognition for Autonomous Agents', which includes a Hypermedia Affordances Recognition Pattern for runtime discovery).

*Sources: https://nordicapis.com/hateoas-the-api-design-style-that-was-waiting-for-ai/ ; https://htmx.org/essays/hateoas/ ; https://arxiv.org/abs/2302.06970 ; https://arxiv.org/abs/2510.24459*

### `CONFIRMED` — OpenAPI Links objects: a response declares named follow-up operations with parameter bindings via runtime expressions (e.g. username: $response.body#/username); design-time declarative, mechanically executable; tooling support remains weak.

The OAS 3.x Link Object works exactly as described, and 'username: $response.body#/username' is literally the spec's own example (UserRepositories link). Runtime expressions can reference request/response bodies via JSON Pointer. Weak tooling is corroborated by the official OpenAPI learn docs, which concede 'the promise of dynamically consuming links returned from an API has rarely been born out in the practicalities of both publishing APIs and software development' and that consumers are under no obligation to follow links.

*Sources: https://spec.openapis.org/oas/v3.1.1.html ; https://learn.openapis.org/specification/links.html ; https://swagger.io/docs/specification/v3_0/links/*

### `CONFIRMED` — Global choreography languages failed: WS-CDL stalled at W3C Candidate Recommendation (2005), criticized as an over-complex 'simulation language' with no mainstream implementations; ebXML BPSS (OASIS ebBP 2.0.4) defined binary collaborations with time-to-acknowledge/perform but adoption foundered on complexity; BPEL survived only as single-party orchestration.

WS-CDL 1.0 reached Candidate Recommendation on 9 November 2005 and was never promoted to Recommendation. Yaron Goland's critique at goland.org calls it Turing-complete, over-abstracted, and says it operates as 'a Web Services Simulation Language' when a purely declarative approach was needed — matching the claim's characterization. ebBP 2.0.4 is an OASIS Standard (21 December 2006) defining Binary Collaborations with timeToAcknowledgeReceipt, timeToAcknowledgeAcceptance, and timeToPerform attributes. WS-BPEL, by contrast, is widely noted as the orchestration spec that saw real use. Two directional elements remain judgment calls: 'no mainstream implementations' (pi4soa existed as a reference implementation, but nothing mainstream — fair) and ebBP adoption foundering specifically 'on complexity' (adoption failure is well attested; the complexity attribution is a common but not quantified explanation).

*Sources: https://www.w3.org/TR/ws-cdl-10/ ; https://www.goland.org/wscdl/ ; https://docs.oasis-open.org/ebxml-bp/2.0.4/OS/spec/ebxmlbp-v2.0.4-Spec-os-en-html/ebxmlbp-v2.0.4-Spec-os-en.htm ; https://www.oasis-open.org/news/pr/members-approve-oasis-ebxml-business-process-ebbp-as-oasis-standard/*

### `CONFIRMED` — RosettaNet PIPs survived: each PIP (e.g. 3A4) is a paired request/response with a Business Activity Performance Controls table — Time to Acknowledge, Time to Perform (typical 2h ack / 24h perform), retry counts, non-repudiation — enforced by B2B gateways in high-tech supply chains.

Microsoft's BizTalk Accelerator for RosettaNet docs confirm the exact mechanism: process-configuration settings for Non-Repudiation, Time to Acknowledge, Time to Perform, and Retry Count all map to 'Table 3-3: Business Activity Performance Controls' in the downloaded PIP specification — i.e., the table number in the claim is exactly right. Gateway enforcement is real: Oracle docs state the B2B adapter resends when an acknowledgment misses the expiration time (and an Oracle support note exists for 'Time to Perform does not work for 3A4'); IBM Sterling/webMethods and TIBCO guides implement the same timers. The typical 2h/24h values for 3A4's Create Purchase Order activity are corroborated by secondary sources and Oracle WLI's 'standard PIP timeout value of 24 hours', though I could not fetch an original PIP 3A4 spec PDF to quote Table 3-3 directly; note also that vendor sample configs vary (Azure Logic Apps' 3A4 sample uses 60s ack / 600s perform), so 2h/24h should be framed as the PIP spec's values, not universal runtime settings. 'Survived in high-tech supply chains' is consistent with RosettaNet's continued maintenance under GS1 US and ongoing vendor support.

**Correction:** Minor caveat only: 2h/24h are the PIP-spec Table 3-3 values commonly cited for 3A4; deployed gateway configurations frequently differ (e.g. Azure Logic Apps defaults of 60s/600s).

*Sources: https://learn.microsoft.com/en-us/biztalk/adapters-and-accelerators/accelerator-rosettanet/using-the-pip-specification-to-create-a-process-configuration ; https://docs.oracle.com/cd/B10464_02/integrate.904/b12121/b2bstandards.htm ; https://support.oracle.com/knowledge/Middleware/1540960_1.html ; https://learn.microsoft.com/en-us/rest/api/logic/rosetta-net-process-configurations/get*

### `CONFIRMED` — Today's echo rules live as prose in bilateral MIGs: ORDRSP must repeat buyer line numbers (LIN DE1082) and order reference (RFF+ON); INVOIC must carry RFF+ON for 3-way match; DESADV must express pallet/carton hierarchy via CPS parent-child + SSCC per GS1 EANCOM; humans verify in test cycles.

The DESADV structure is verified against GS1 EANCOM guidance: the first CPS identifies the shipment as a whole and acts as parent for the next lower packaging level (pallet), subsequent CPS segments carry parent-child sequence references, and SSCC identifies each despatch unit in the hierarchy (GS1 Poland DESADV guideline; GS1 EANCOM 2002 DESADV; ecosio 'DESADV with SSCC'). EANCOM ORDRSP verification confirms LIN DE1082 line numbering and RFF+ON as the order-number reference tying the response to the original order (EdiFabric EANCOM ORDRSP docs, GS1 Germany ORDRSP profile, retailer MIGs like Metcash). RFF+ON in INVOIC as the PO reference is standard EANCOM practice; the '3-way match' purpose is the standard business rationale, though stated analytically. The strongest unverifiable part — that ORDRSP 'must repeat' the buyer's numbers as a prose rule in bilateral MIGs verified by humans in test cycles — is exactly what retailer MIGs do in practice and matches the published guideline structure, but is guideline-level practice rather than a single citable normative rule, as the claim itself concedes.

*Sources: https://gs1pl.org/app/uploads/2022/02/desadv_23_en.pdf ; https://ecosio.com/en/blog/what-is-a-desadv-with-sscc/ ; https://support.edifabric.com/hc/en-us/articles/360008207212-EANCOM-ORDRSP-Purchase-Order-Response ; https://www.publikationen.gs1-germany.de/Complete/eancom_v9.3/profiles/ae/gesamt/ordrsp/en/ordrsp-ae-gesamt-en.pdf*

### `CORRECTED` — EDI onboarding effort: industry sources cite 6–12 weeks per trading partner under legacy methods, dominated by manual mapping plus repeated bilateral test cycles (Orderful: 8–12 weeks, 'week 5 first testing round finds 12 errors'; Cleo: 6–12 weeks). No source quantifies echo-rules' exact share.

The numbers all check out: Cleo states 'Traditional EDI onboarding takes 6–12 weeks per partner due to manual processes and repeated testing cycles'; Orderful's transform-edi-partner-onboarding post states 'Traditional EDI partner onboarding takes 8 to 12 weeks because most of that time goes into manual mapping, one-off testing cycles, and email threads'. The 'week 5, first round of testing finds 12 errors' timeline is also real Orderful content, but it lives in a different post — '5 Signs Your Trading Partner Onboarding Process Is Broken' (trading-partner-onboarding-warning-signs), not the cited transform-edi-partner-onboarding URL. Failure examples (rejected POs, chargebacks) appear as consequences of skipped validation rather than as documented test-cycle outcomes. The claim's own caveat — support is directional, no source quantifies echo-rules' share — is accurate.

**Correction:** Attribute the 'week 5 / 12 errors' vignette to https://www.orderful.com/blog/trading-partner-onboarding-warning-signs (its Week 1 spec exchange → Week 3 mapping underway → Week 5 twelve testing errors → Week 8 frustration timeline), not to the transform-edi-partner-onboarding post, which carries only the 8–12 week figure.

*Sources: https://www.cleo.com/how-to-onboard-edi-trading-partners-faster ; https://www.orderful.com/blog/transform-edi-partner-onboarding ; https://www.orderful.com/blog/trading-partner-onboarding-warning-signs*

