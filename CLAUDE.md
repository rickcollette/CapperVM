# CLAUDE.md

You have full access to sibling checkouts `../CapperWeb` and `../CapsuleBuilder`.
When we make api CRUD (any change at all), lets make sure to update CapperWeb as well

CapDB lives in its own repo: <https://github.com/rickcollette/CapDB>

- To build/use CapDB, check it out (or update it) into `./CapDB` via
  `make capdb-fetch`. This is the only CapDB checkout the build should use
  (Makefile `CAPDB_DIR` defaults to ./CapDB, which is git-ignored).
- Do not use CapDB checkouts outside this repository (for example a CapDB tree
  under a sibling CapperVM workspace in another IDE). Never read from, write to,
  build against, or delete those trees.
