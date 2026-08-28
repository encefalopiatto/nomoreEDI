# Internal ground truth: Atlas answers and operational evidence review

*Two rounds of internal evidence. Round 1: direct Atlas answers (short prompts; longer ones timed out at the tool layer). Round 2: a fuller operational evidence review relayed from Atlas, which sharpened the epistemic status of each sizing claim and is the authority where the two rounds differ. All figures are directional estimates unless marked verified.*

## Round 1 — direct Atlas answers

### Q: What dominates onboarding effort and calendar time?

> Bilateral testing and business/master-data alignment usually dominate calendar time. They depend on partner response, correct GLNs/customer numbers, article/unit/tax rules, and resolving real test failures.
>
> Directional split, not measured Procuros time tracking:
>
> - Business/master-data alignment: **30–40%**
> - Bilateral partner testing: **30–40%**
> - JSONata mapping: **15–25%**
> - Syntax/normalizer configuration: **5–15%**
>
> For a standard EDI format with a good sample, configuration and mapping are often quick. Atypical formats, poor source data, or complex retailer rules can move mapping to **30%+**, but partner turnaround still tends to set the go-live date.

### Q: Top causes of post-go-live message failures / partner escalations?

> Practical ranking, **not a measured incident-rate report**:
>
> 1. **Master-data drift**: GLNs, ship-to/bill-to mappings, product IDs, VAT data, pack sizes, and recipient routing change after go-live.
> 2. **Silent partner requirement changes**: changed EDIFACT rules, mandatory fields, filename/SFTP conventions, or validation without notice.
> 3. **Untested business scenarios**: returns/credit notes, partial deliveries, substitutions, variable weight, Pfand (deposit), mixed VAT, and zero/negative values.
> 4. **Code-list and reference-data changes**: units of measure, tax/category codes, qualifiers, country codes, payment terms, or warehouse codes.
> 5. **Mapping assumptions that held only for test samples**: optional fields absent, repeated loops, multiple addresses, or unexpected line structures.
> 6. **Operational/configuration drift**: credentials, certificates, folders, relationship state, or endpoint changes.
> 7. **Volume/timing issues**: bursts, duplicate sends, delayed acknowledgements, or downstream ERP/SFTP limits.
>
> The common root cause is weak change control after go-live, not usually a platform defect.

## Round 2 — operational evidence review (relayed, Aug 2026)

Summary of the five findings, with epistemic labels as given:

1. **Where requirements live — partly structured, but not yet a complete contract** *(inferred, supported by system evidence)*. Structured implementation artifacts exist (transformations, relationship/process configuration, canonical fields and checks, extracted partner-guideline knowledge) — but partner intent remains materially distributed across PDFs/MIGs, prose tables, commercial-condition sheets, master-data prerequisites, and conventions embedded in mappings. Structured *implementation* exists after onboarding; structured, authoritative *requirement publication from the partner* generally does not. The "tribal" share is not defensibly quantifiable; the right baseline is a per-relationship × document-type classification of every requirement source (executable specification / partner guide / internal implementation / master-data-or-commercial attachment / undocumented operational knowledge — one requirement may correctly have several sources).

2. **Connectivity is diverse and real, but its cost share is not measured** *(verified for connector categories; not for comparative effort)*. A bounded production sample found **>50 ready integrations in each of AS2, SFTP, API, and WebEDI** — none is legacy edge traffic. X.400, SMTP, VAN, and PEPPOL were **not** verified as distinct first-class connector categories; some routes labelled "X.400" are technically configured as AS2, so counting names would overstate a separate transport category. The claim that connectivity costs less than testing and business alignment is plausible but unmeasured; connectivity failures cannot be ranked from current aggregated error reporting.

3. **Acknowledgement capability exists, but business-rejection automation is narrow** *(verified for a limited APERAK-style capability)*. **8 ready `ACKNOWLEDGEMENT` relationships** exist in the relationship model; one inspected live route uses a generic APERAK sender on an AS2/EDIFACT path with recent traffic. This does *not* show broad CONTRL/APERAK/custom usage, nor that wrong-price/unknown-GLN/master-data rejections consistently reach suppliers through an automated structured loop, nor that human back-and-forth is rare. Validation of the response/enrichment mechanism — and a warning that it creates value only if runtime-native.

4. **Onboarding duration is highly variable; partner sign-off waiting is not instrumented** *(verified for available monthly cohorts)*. **93 completed onboardings** across Aug 2025–Aug 2026; monthly reported medians range **0–949 days** with large swings across adjacent months — no single "typical duration" should be quoted before definition and data-quality review (zero-day and very long values in particular). Reporting does not isolate "test package sent → partner accepted/sign-off received", so *"partner turnaround determines go-live" remains an operational hypothesis, not a quantified finding*. Decisive instrumentation: request opened → requirements complete → technical configuration ready → first test sent → partner approval received → production go-live.

5. **Change volume is visibly high, but external requirement-change frequency is unknown** *(verified only as an implementation proxy)*. **55 transformation-version deployment rows in the most recent 7-day reporting window.** A deployment can be an internal fix, a rollback, multiple releases for one partner notice, a shared change affecting many partners, or unrelated implementation work — so no annualization may be presented as "partner changes per year". (Note: the review's own "~409 rows/year" annualization is also arithmetically inconsistent with 55/week ≈ 2,860/year — a further reason to quote no rate.) Needed: a change ledger recording origin (partner / regulation / internal improvement / defect), affected relationship and document flow, semantic vs technical impact, effective date, and a shared identifier linking one external notice to its several releases.

### The review's interpretation

- **Strongly supported:** requirements and operational knowledge are fragmented; transport is heterogeneous; acknowledgement mechanisms exist but are not universal; change activity is substantial; partner acceptance is not currently measurable or machine-enforceable.
- **Still unproven:** LLM-generated bindings at finance-grade reliability; executable contracts materially outperforming PDF-guided AI mapping on silent errors; retailers ever becoming contract publishers rather than leaving Procuros to author.
- **Practical first move:** do not start by inventing a new wire format. Build an internal, versioned contract representation for a small set of existing high-friction retail flows; generate validators, test vectors, readable guides, and legacy projections from it; then compare its silent-error and onboarding-cycle performance against the current mapping/PDF process. (Adopted as **Phase 0** in `ASSESSMENT.md` §7.)
