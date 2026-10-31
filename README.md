# The OSIRIS JSON Consumers

[![GitHub License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://github.com/osirisjson/osiris-consumers/blob/main/LICENSE)
[![Go Reference](https://pkg.go.dev/badge/go.osirisjson.org/consumers.svg)](https://pkg.go.dev/go.osirisjson.org/consumers)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=osirisjson_osiris-consumers&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=osirisjson_osiris-consumers)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=osirisjson_osiris-consumers&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=osirisjson_osiris-consumers)


Turn [OSIRIS JSON](https://github.com/osirisjson/osiris) portable
infrastructure snapshots into documentation, inventory reports, and
editable [draw.io](https://www.drawio.com/) diagrams without AI or any
license or vendor-specific applications.

[OSIRIS JSON](https://github.com/osirisjson/osiris) is an Open
Specification for Infrastructure Resource Interchange Schema.
It defines a vendor-neutral JSON Schema format for describing
infrastructure resources, their properties, provider provenance, groups
and topology relationships across heterogeneous IT and OT environments.

[OSIRIS JSON Producers](https://github.com/osirisjson/osiris-producers)
discover infrastructure from vendor platforms or devices and generate
portable, point-in-time OSIRIS JSON documents. **OSIRIS JSON Consumers**
(this repository) read those documents and turn them into useful,
human-readable and editable outputs.

## What OSIRIS JSON Consumers generate

1. **Infrastructure documentation and inventory:** Markdown and PDF
   reports covering resources, connections, groups, and provider
   attribution. Unknown resource and relationship types are handled
   generically rather than silently discarded. When report tables exceed
   50 rows, a companion XLSX workbook is automatically generated.
2. **Editable infrastructure topology:** draw.io diagrams built from the
   document's explicit resources and connections, with generic fallback
   nodes for unrecognized types. The source document remains the
   authoritative record; the diagram is a visualization of its contents.

These outputs are based on the OSIRIS JSON snapshot document.
OSIRIS JSON Consumers once installed works completely offline and never
require anything else to generate them.
In accordance with the OSIRIS JSON specification, Consumers MUST perform
structural validation (or equivalent checks) before processing documents
and handle unrecognized types without silently dropping resources.

### Examples

#### A Cisco Nexus 9000  draw.io topology

![OSIRIS JSON for Cisco NX-OS](.github/media/osirisjson-cisco-nxos-drawio.png)

*Example of a [draw.io](https://www.drawio.com/) topology auto-generated from an OSIRIS JSON snapshot of a Cisco Nexus 9000 device.*

#### A Microsoft Azure draw.io topology

![OSIRIS JSON for Microsoft Azure](.github/media/osirisjson-microsoft-azure.png)

*Example of a [draw.io](https://www.drawio.com/) topology auto-generated from an OSIRIS JSON snapshot of a Microsoft Azure Subscription.*

### Architecture workflow

```mermaid
flowchart TD
    subgraph VP ["Vendor / Platform"]
        A["Hyperscalers, Cloud & Hosting Providers"]
        B["On-Premises<br/>IT and OT"]
    end

    subgraph SP ["The OSIRIS JSON Specification"]
        S["Authoritative source of truth<br/>for the specification and core schema"]
    end

    subgraph PR ["The OSIRIS JSON Producers"]
        P["Discover, Normalize, Redact, Emit"]
        J["OSIRIS JSON Document<br/>Portable point-in-time infrastructure snapshot"]
    end

    subgraph VA ["The OSIRIS JSON Toolbox"]
        V["Validation toolbox<br/>Three-level validation"]
    end

    subgraph OC ["The OSIRIS JSON Consumers (This Repository)"]
        C["Read OSIRIS JSON documents"]
        AD["Architecture and inventory reports"]
        AU["Audit-ready documentation"]
        DF["Snapshot diff view<br/>(roadmap)"]
        TP["Editable draw.io<br/>topology generation"]
    end

    subgraph UC ["Under Your Full Control"]
        U["One OSIRIS JSON file<br/>Multiple use cases"]
        M["CMDB / IPAM / DCIM workflows"]
        AI["AI platforms, agents<br/>and MCP servers"]
        X["Custom analytics<br/>and third-party tools"]
    end

    A -->|"Discovered by"| P
    B -->|"Discovered by"| P

    S -. "Defines the format" .-> J
    S -. "Guides implementation" .-> P

    P -->|"Generates"| J
    J -. "Can be validated by" .-> V
    J -->|"Consumed by"| C

    C -->|"Generates"| AD
    C -->|"Generates"| AU
    C -. "Planned" .-> DF
    C -->|"Generates"| TP

    J --> U
    U --> M
    U --> AI
    U --> X

    style VP fill:transparent,stroke:#555,stroke-width:1px,color:inherit
    style SP fill:transparent,stroke:#555,stroke-width:1px,color:inherit
    style PR fill:transparent,stroke:#555,stroke-width:2px,color:inherit
    style VA fill:transparent,stroke:#555,stroke-width:1px,color:inherit
    style OC fill:transparent,stroke:#4F7FE7,stroke-width:2px,color:inherit
    style UC fill:transparent,stroke:#555,stroke-width:1px,color:inherit
```

## Installation and getting started

See the [OSIRIS JSON Consumers getting-started guide](https://docs.osirisjson.org/osiris-consumers/getting-started/)
for the project overview, installation guide and other informations.

## Documentation and contributing

Contributions are welcome. Please read [`CONTRIBUTING.md`](CONTRIBUTING.md)
and the [community contribution guidelines](https://docs.osirisjson.org/community/contributing/)
before opening a pull request.

Contributions target the `dev` branch as small, reviewable increments.
The `main` branch will contains only tagged releases.

## License

Licensed under the [Apache License 2.0](LICENSE).