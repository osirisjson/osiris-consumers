# OSIRIS JSON Maintainers

## Descriptions of section entries and preferred order
| Tag | Field | Description | Format / Allowed values | Notes / Examples |
|---|---|---|---|---|
| M | Mail | Patches should be sent to | `FullName <address@domain>` | Example: `Jane Doe <jdoe@domain.com>` |
| S | Status | Maintenance status | `Maintained` \| `Orphan` \| `Obsolete` | **Maintained:** actively maintained<br>**Orphan:** no current maintainer<br>**Obsolete:** replaced by a better system |
| F | Files | File/dir wildcard patterns | One pattern per line (multiple `F:` lines allowed) | Trailing `/` includes all files and subdirectories. See examples below. |

| Pattern example | Meaning |
|---|---|
| `osiris/` | All files in and below `osiris/` |
| `osiris/*` | All files in `osiris/`, but not below it |
| `*/osiris/*` | All files in `osiris/` under any top-level directory |


## Maintainers List

When reading this list, please look for the most precise areas first. When adding to this list, please keep the entries in alphabetical order.

**ORIGINAL CREATOR & LEAD MAINTAINER**
| Tag | Field |
|---|---|
|M|Tia Zanella @skhell <tiazanella@osirisjson.org>|
|S|Maintained|
|F|*|

**CONTRIBUTORS**

