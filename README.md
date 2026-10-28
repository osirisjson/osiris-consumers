# The OSIRIS JSON Consumers

[![GitHub License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://github.com/osirisjson/osiris-consumers/blob/main/LICENSE)
[![Go Reference](https://pkg.go.dev/badge/go.osirisjson.org/consumers.svg)](https://pkg.go.dev/go.osirisjson.org/consumers)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=osirisjson_osiris-consumers&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=osirisjson_osiris-consumers)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=osirisjson_osiris-consumers&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=osirisjson_osiris-consumers)

[OSIRIS JSON](https://github.com/osirisjson/osiris) is an Open
Specification for Infrastructure Resource Interchange Schema that
defines a vendor-neutral JSON format for describing infrastructure
resources, their properties and their topological relationships across
heterogeneous IT and OT environments.

An [OSIRIS JSON Producer](https://github.com/osirisjson/osiris-producers)
connects to a vendor applicance or platform and emits a vendor-neutral
portable OSIRIS JSON document.
OSIRIS JSON Consumers (this repository) read that document and generate:

1. a Markdown/PDF brief and inventory that represents every resource,
   connection, group, provider, and unknown type in the document without
   semantic loss with a companion XLSX file if reports generate tables
   over 50 rows.
2. a draw.io topology that represents the complete graph with generic
   fallback nodes and literal edges.

### Architecture workflow

```mermaid
flowchart TD 
    subgraph VP ["Vendor / Platform"]
        A["Hyperscalers, Cloud & Hosting Providers"]
        B["On-Premises<br/>IT and OT"]
    end
    
    subgraph SP ["The OSIRIS JSON Specification"]
        S["The authoritative<br/>source of truth for the<br/>specification and core schema"]
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
        AD["Architecture documentation reports"]
        AU["Audit reports"]
        DF["Snapshot diff view"]
        TP["Automatic topology<br/>generation and updates"]
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

    J -->|"Validated by"| V

    J -->|"Consumed by"| C
    C -->|"Generates"| AD
    C -->|"Generates"| AU
    C -->|"Generates"| DF
    C -->|"Generates"| TP

    J --> U
    U --> M
    U --> AI
    U --> X
    
    style VP fill:transparent,stroke:#555,stroke-width:1px,color:inherit
    style SP fill:transparent,stroke:#555,stroke-width:1px,color:inherit
    style PR fill:transparent,stroke:#555,stroke-width:2px,color:inherit
    style VA fill:transparent,stroke:#555,stroke-width:1px,color:inherit
    style OC fill:transparent,stroke:#4F7FE7,stroke-width:1px,color:inherit
    style UC fill:transparent,stroke:#555,stroke-width:1px,color:inherit
```

### Installation

See the official guidelines available on [docs.osirisjson.org/osiris-consumers/installation](https://docs.osirisjson.org/osiris-consumers/installation/)

### Documentation and contribution

See `CONTRIBUTING.md`. Contributions target the `dev` branch as small,
reviewable increments. `main` carries tagged releases only.

- [Community & contribution guidelines](https://docs.osirisjson.org/community/contributing/)