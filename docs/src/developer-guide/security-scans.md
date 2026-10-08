---
title: "Security scans"
description: "Local SAST/SCA scanners and intentional gosec exclusions."
owner: "docs"
status: "stable"
reviewed: "2026-10-08"
outputs:
  - markdown
  - web
---

# Security scans

Run from the Capper repo root (Docker required for most tools):

| Tool | Purpose |
| --- | --- |
| gitleaks | Secrets |
| gosec | Go SAST |
| semgrep | Multi-language SAST |
| trivy fs | Dependency / secret / misconfig |
| govulncheck | Go vulnerable symbols |
| shellcheck | Shell script hygiene |

## gosec exclusions

`.gosec.json` excludes **G204** (subprocess) and **G304** (file path from variable). Capper must launch runtimes and open instance filesystem paths; those are not treated as vulnerabilities. Do not broaden the exclude list without review.
