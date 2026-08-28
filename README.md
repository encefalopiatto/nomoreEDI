# nomoreEDI

Working notes toward a self-describing B2B exchange language intended to replace classic EDI — its guidelines, format guides, and bilateral testing.

## Contents

- **[ASSESSMENT.md](ASSESSMENT.md)** — the design assessment: verdict, thirty years of adversarially fact-checked prior art, why the strong form fails, internal ground truth, the surviving architecture ("Trade Contracts" / self-describing by reference), adoption strategy, Phase 0, and the measurement agenda.
- **[research/](research/)** — the full research corpus behind the assessment, one file per lens with the research agent's structured findings and the independent fact-checker's verdicts:
  - [lens-prior-art.md](research/lens-prior-art.md) — XML, ebXML CPP/CPA, X12 SEF, Stedi Guides, RosettaNet, Semantic Web, ISO 20022, UBL/EN 16931, OpenAPI, Avro
  - [lens-peppol-regulation.md](research/lens-peppol-regulation.md) — how PEPPOL killed bilateral testing; EU e-invoicing mandate dates (DE/FR/BE/IT, ViDA)
  - [lens-adversarial.md](research/lens-adversarial.md) — the strongest case against; semantic-variance taxonomy; security/legal/incentive objections; disposal of the embeddings/blockchain/CDC ideas
  - [lens-architecture.md](research/lens-architecture.md) — the steelman: content-addressed Trade Contracts, CEL rules, computed conformance, change protocol, legacy bridge, JSONata-corpus mining
  - [lens-adoption.md](research/lens-adoption.md) — Walmart AS2, Amazon, Tradacoms inertia, SDI/CFDI mandates, onboarding and deduction economics, wedge strategy
  - [lens-choreography.md](research/lens-choreography.md) — response declarations: HTML forms, Siren/HAL-FORMS/Hydra, OpenAPI Links, RosettaNet BAPC, PEPPOL response profiles
  - [completeness-critique.md](research/completeness-critique.md) — missing angles (FHIR, APERAK/824, GDSN, the LLM counterfactual, fraud vector), contradictions between lenses, claims to soften
  - [atlas-evidence.md](research/atlas-evidence.md) — internal ground truth: Atlas answers and the operational evidence review
  - [raw/](research/raw/) — the same lens outputs as raw JSON
- **[page/contracts-not-maps.html](page/contracts-not-maps.html)** — source of the shareable one-page version ("Contracts, Not Maps").
