# OSIRIS JSON Consumers - test fixtures

Small, reviewed, synthetic OSIRIS JSON documents and the deterministic
artifacts they are expected to produce. Every fixture is hand-authored
with neutral placeholder data; none of them represent a capture from a
real infrastructure environment.
Data rules (documentation and reserved ranges only) are in
`CONTRIBUTING.md`.

Placing a document under `valid/`, `producer/`, or `rendering/` means
only that the canonical validation engine accepts it with zero errors.
It is not a statement that the string content is safe, meaningful, or
exemplary: several fixtures deliberately carry hostile or unusual
content that a correct consumer must still handle without failing.

## Buckets

| Directory | Contract |
|---|---|
| `valid/` | Canonically valid documents, each isolating one format feature a consumer must handle. Accepted and processed end to end. |
| `adversarial/` | Documents that must be refused, or that expose a structural anomaly. Each pins one specific failure. |
| `producer/` | Canonically valid documents shaped like real producer output. Prove the generic path handles realistic input. |
| `rendering/` | Canonically valid documents whose string content is hostile by design. Input to renderer-escaping and golden tests. |
| `golden/` | Byte-exact expected Markdown and draw.io artifacts, keyed to the inputs above. Added with the renderers. |

## valid/

| File | Description |
|---|---|
| `no-provider.json` | no recognized provider; generic-first output must still be complete. |
| `mixed-provider.json` | several providers plus an unknown resource and connection type in one document; every provider, type, node, and edge is represented. |
| `unknown-fields.json` | forward compatibility. Declares version `1.4.0` and adds unknown fields at every level. The `1.4.0` version and every `future*` field name are synthetic stand-ins for whatever a later 1.x OSIRIS JSON specification may add; they are not real schema fields or a real specification reference. Unknown data must be preserved, not dropped. |

## adversarial/

Each file is loaded by a test that asserts the exact outcome:

- Malformed or trailing JSON is refused before validation runs.
- A canonically invalid document is refused with the engine's own
  `V-*` diagnostic code surfaced verbatim.
- A structurally inconsistent but valid document is reported by the
  graph indexer without dropping anything.

## producer/

Hand-authored, neutral snapshots (13 to 15 resources each) modeled
on the shape of osiris-producer output: native identifier styles,
vendor type namespaces, provider provenance fields, `contains` and
`network*` connections, all three edge directions, and nested groups. 
The set is widened to one snapshot per active producer, plus a large
synthetic document for the scale budget, when the interoperability
suite is built.

## rendering/

| File | Description |
|---|---|
| `injection-strings.json` | resource names and descriptions, a connection name, and a group name carrying HTML such as `<script>`, Markdown headings and table pipes, draw.io cell-break sequences, C0 control characters, and non-ASCII text. The renderers must escape or normalize all of it: no active content in a Markdown viewer, and well-formed draw.io XML. The document is valid input; the point is the output, so it lives here rather than in `valid/`. |

## Validation

Fixtures are checked against the pinned canonical engine
(`@osirisjson/cli`), not against a schema copy in this repository.
Consumers vendors no schema and runs no validation of its own.
