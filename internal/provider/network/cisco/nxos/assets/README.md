# OSIRIS JSON Cisco NX-OS custom shape library for draw.io

This directory holds the hand-drawn front and rear chassis illustrations
`panel.go`'s `confirmedPanels` map resolves a Cisco Nexus switch's own
chassis product ID to. They back the confirmed-panel draw.io pages
(`FrontPhysical`, `FrontVLANs`, `Rear`) for the one Nexus model reviewed
and confirmed so far; a document whose chassis has no entry here falls
back to the generic topology page instead of a guessed or improper
generated illustration.

## Policy

- **Confirmed only:** An illustration is added here only after it has
  been drawn and visually reviewed against the real hardware chassis it
  represents. It MUST never be generated or approximated from a vendor
  datasheet or marketing illustration.
- **One model, one illustration pair:** Each confirmed model gets its
  own front and rear SVG, along with eventual components, under its
  own dedicated directory. Nothing in this directory is shared or
  parameterized across different models.
- **No dynamic fallback:** No dynamic fallback logic lives in this
  directory. The per-port dynamic assembly algorithm for unconfirmed
  chassis (which places generic connector icons at each of a switch's
  ports when no confirmed illustration exists) is handled as a separate,
  later increment (see `panel.go`'s own doc comment).


## Layout and naming convention

When designing a new model, files MUST be saved within this hierarchy,
respecting the strict naming convention below:


```text
assets/drawio/<family>/<model>/osiris-json-cisco-<model>-front.svg
assets/drawio/<family>/<model>/osiris-json-cisco-<model>-rear.svg
```

- `<family>` is the product family directory name (e.g. `nexus-9k`);
- `<model>` is the chassis product ID exactly as `panel.go`'s and is
  always written in capital letters.
- `confirmedPanels` map keys it (e.g. `N9K-C93180YC-FX`).

## How to contribute designing and adding a new confirmed model

We highly recommend using [Inkscape](https://inkscape.org) to design the
switches or their components.

1. **Setup the Canvas:** Configure your artwork page in landscape mode
   with a fixed width of `2964px`. The height will vary depending on the
   rack unit (U) height of the chassis. Make sure to crop the artwork to
   the bounding box once the design is complete to eliminate any blank
   space around the chassis.
2. **Draw & Review:** Draw and review the model's front and rear SVGs
   following the naming convention described above. Make sure to
   correctly use layers in the document to design ports and other
   details correctly naming them as appear on the real switch.
3. **Simplify Low-Level Details:** You can omit low-level components
   such as screws or passive airflow intake patterns if they do not
   impact topology visualization. Focus on what matters for the network
   layout. A good starting point is the flat `*-switch-body.svg`
   template located in `internal/provider/network/assets/drawio/shared`.
4. **Add the Files:** Place both files under a new `<family>/<model>/`
   directory inside this path.
5. **Update Go Source:** Add the corresponding `//go:embed` pair and a
   new `confirmedPanels` entry inside the `panel.go` file.
6. **Map Port Anchors:** Derive the model's port anchor fractions
   directly from the SVG's group transform matrices (never eyeball or
   guess them). Pin them using a dedicated test, mirroring the existing
   `panelPortAnchor` / `rearPortAnchor` derivation and
   `TestPanelPortAnchor` in `panel_render.go`.


## Metadata

Before saving your artwork make sure to add the following metadata:

| Field | Expected Format / Description |
| --- | --- |
| **Title** | `OSIRIS JSON Cisco Nexus <MODEL_IN_CAPITAL_LETTERS> <front\|rear> chassis` |
| **Date** | ISO format: `YYYY-MM-DD` |
| **Creator** | `Name Surname` (optionally followed by your website) |
| **Rights** | `https://osirisjson.org/en` |
| **Publisher** | `https://osirisjson.org/en` |
| **Identifier** | `https://osirisjson.org/en` |
| **Source** | `https://osirisjson.org/en` |
| **Description** | A brief technical description of the artwork |
| **Contributors** | A list of contributors formatted as: `Name Surname` (optionally followed by website) |

## Licensing

Every illustration here is original artwork created for the OSIRIS
JSON project, not a vendor-supplied or scraped image, and is covered by
this repository's own root `LICENSE` (Apache-2.0) like any other source
file, no separate license file or notice is needed for this directory.
Each SVG's own embedded metadata (`<metadata>` / `dc:creator` /
`dc:date`) records its own authorship and creation date for provenance,
consistent with that license; it does not modify it.

The draw.io Cisco19 stencil style used for the generic neighbour icons
on these pages (`shape=mxgraph.cisco19.rect`) is resolved by name from
draw.io's own bundled shape library at render time and is never
embedded or redistributed from this repository.
