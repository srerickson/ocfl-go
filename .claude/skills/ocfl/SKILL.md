---
name: ocfl
description: Guide for automating operations on existing OCFL (Oxford Common File Layout) v1.0 and v1.1 repositories — ingesting, updating, validating, and deleting objects. Use when reading or writing OCFL storage roots, object roots, inventories, version directories, or when implementing/reviewing code that manipulates OCFL structures.
---

# OCFL Automation Guide

OCFL v1.0 and v1.1 define an application-independent, filesystem-based layout for
versioned digital objects: each object is a directory of immutable version
directories plus a JSON inventory that maps content-addressed files (digests)
to logical paths. A repository must be rebuildable from the storage root alone.
Determine the governing version from the storage-root and object namaste files
before applying a requirement.

## Authoritative references

| Resource | Use it for |
|----------|-----------|
| `references/ocfl-spec-1.1.md` | Full normative v1.1 spec. Key sections: §3.3 version directories, §3.5 inventory, §3.6–3.7 inventory digests, §4 storage root |
| `references/ocfl-spec-1.0.html` | Full published normative v1.0 specification (unaltered HTML) |
| `references/validation-codes.md` | Lookup table of every `E###` (MUST) and `W###` (SHOULD) code with its requirement text — use it to name findings and to check what a code actually asserts |
| `references/implementation-notes.md` | Non-normative guidance: §1 digital preservation, §2 storage (incl. object-store/S3 concerns), §3 client behaviors |
| `references/extensions/` | The registered community extensions, one file per extension (see below) |
| <https://ocfl.github.io/extensions/> | Check for extensions registered after this bundle was captured |

When in doubt about a MUST/SHOULD, inspect the spec matching the object's
version. Both bundled specs carry validation codes inline as HTML spans
(for example, `<span id="E064" ...>`); `references/validation-codes.md`
resolves a bare code to its requirement without searching the spec.

### Bundled extensions

`references/extensions/` holds the registered specifications. Storage layouts
(`0002`, `0003`, `0004`, `0006`, `0007`, `0010`, `0011`, `0012`) define id →
path mapping; read the one named in `ocfl_layout.json` rather than inferring
the algorithm. The rest cover digest algorithms (`0001`, `0009`), mutable head
(`0005`), the schema registry (`0008`), and the extension template (`0000`).

## Anatomy

```
[storage_root]
├── 0=ocfl_1.1                  # namaste; content "ocfl_1.1\n"
├── ocfl_layout.json            # optional: {"extension": "...", "description": "..."}
├── extensions/                 # optional storage-root extensions
└── <hierarchy>/[object_root]   # layout-determined path per object
    ├── 0=ocfl_object_1.1       # namaste; content "ocfl_object_1.1\n"
    ├── inventory.json          # ALWAYS a byte-copy of the head version's inventory
    ├── inventory.json.sha512   # sidecar: "DIGEST inventory.json\n"
    ├── logs/                   # optional, freeform local audit records
    ├── extensions/             # optional object extensions
    └── v1..vN/
        ├── inventory.json + sidecar   # SHOULD be present per version
        └── content/                   # name overridable via contentDirectory
```

Inventory required keys: `id` (URI-ish, never changes), `type`
(`https://ocfl.io/1.1/spec/#inventory`), `digestAlgorithm` (`sha512` or
`sha256`; prefer `sha512`), `head`, `manifest` (digest → array of content
paths relative to object root), `versions` (name → `{created, state, message,
user}`). Optional: `contentDirectory` (fixed at v1, never changes), `fixity`
(algorithm → digest → content paths).

`state` maps digests → logical paths (the user-visible file tree at that
version); every state digest MUST exist as a manifest key. Deduplication:
unchanged files get no new content — their digest already resolves via the
manifest to an earlier version's content path.

## Invariants automations must never violate

1. **Version directories are immutable.** Never add, modify, or remove
   anything under an existing `vN/`. All change happens by adding `vN+1`.
2. **Root inventory = head version inventory** (E064). Copy the file; don't
   regenerate it separately.
3. **Sidecar is written last.** Digest the final inventory bytes, then write
   `inventory.json.<alg>`; it is the commit marker (E062).
4. **Prior `version` blocks are carried forward unchanged** — same logical
   state (E066); `created`/`message`/`user` should also be identical (W011).
5. **Version naming convention is fixed by v1** — padded vs non-padded, same
   width, no gaps in the sequence (E011–E013).
6. **Digest comparisons are case-insensitive**, but each digest may appear
   only once per block regardless of case (E096/E097).
7. **Path safety:** logical and content path elements must not be `.`, `..`,
   or empty; no leading/trailing `/`; no path may be a prefix of another
   (E051–E053, E095, E098–E101).
8. **No stray files:** nothing in an object root, version directory, or
   storage hierarchy beyond what the spec names; no empty directories under
   the storage root (E001, E015, E072, E073).

## Operations

Prefer an existing OCFL client (github.com/srerickson/ocfl-tools) over
hand-rolling these steps; use the steps to drive or verify the tool.

### Locate an object

Read `ocfl_layout.json` → apply the named layout extension (its spec is in
`references/extensions/`) to map id → path. If absent, check the storage
root's own `extensions/` for a layout config, else walk the hierarchy
for `0=ocfl_object_*` namaste files. After resolving, confirm
`inventory.json` `id` matches the requested id — never trust the path alone.

### Ingest (new object)

1. Compute the object path from the storage-root layout; the object root must
   not already exist and intermediate directories must contain no files.
2. Stage `v1/content/...` with the desired logical tree; digest every file.
3. Build the inventory (`head: "v1"`); write `v1/inventory.json` + sidecar,
   copy both to the object root, then write `0=ocfl_object_1.1` (or write
   namaste first and inventories last — end state is what's validated).
4. Build in a temp location and move into place atomically if the filesystem
   allows; a partially written object root is invalid.

### Update (new version)

1. Read and verify the root inventory (sidecar digest, `head`, id).
2. Next version name follows the object's existing convention.
3. New state = previous state ± changes. Only write content files whose
   digest is not already in the manifest; new files go under
   `vN+1/<contentDirectory>/`, and their manifest paths use the real version
   directory name.
4. Append the new version block: `created` in RFC 3339 with timezone,
   second-level granularity; include `message` and `user`
   (`name` + `address`, ideally mailto:/ORCID URI).
5. Update `head`, add manifest (and optional fixity) entries.
6. Write order: `vN+1` content → `vN+1/inventory.json` + sidecar → replace
   root `inventory.json` + sidecar last.
7. OCFL has no locking. Serialize writers externally and re-verify `head`
   hasn't moved immediately before step 6.

For anything that must survive interruption — resumable ingest, crash recovery,
rollback, concurrent writers — serialize the new inventory once and cache the
bytes before any storage write;
write directly to final paths (same code path for POSIX and S3); always write
`vN+1/inventory.json` + sidecar before touching the root, so an interrupted
commit is self-describing; treat the **root sidecar** as the sole commit point;
record phase durably in a log outside the storage root, ahead of the action it
authorizes.

An object created under OCFL 1.0 may stay 1.0 or be upgraded (new namaste
file + `type` value) — a version must conform to the same or later spec
version than its predecessor (E103), and never later than the storage root's.

### Validate

Use a real validator when available (e.g. `rocfl validate`,
`ocfl-py`'s `ocfl-validate.py`); report findings by their E/W codes, resolving
each against `references/validation-codes.md`. Core checks, roughly cheapest
first:

- namaste files present with exact expected content
- inventory parses, has all required keys and no unknown ones (E102)
- sidecar digest matches inventory bytes (every copy)
- version sequence `v1..head` continuous, consistent naming
- root inventory identical to head version's inventory
- every state digest resolves in the manifest; every manifest path exists on
  disk; every file under each `content/` appears in the manifest (E023)
- prior-version inventories agree with the current one on prior states
- full fixity: recompute file digests against manifest (and `fixity` block) —
  the expensive step; make it separately schedulable in automations

### Delete

Two distinct operations — always confirm which one is intended, and run
`inventory.json` id verification before either:

- **Logical deletion (the OCFL-native way):** create a new version whose
  `state` omits the paths. Content remains in prior versions and stays
  recoverable; the manifest keeps its entries. This is a normal update.
- **Abort (uncommitted):** if a create/update is still in flight, remove the
  target version directory and restore the root inventory. Distinct from both
  cases below.
- **Purge (destructive, outside the spec):** removing content from history
  or deleting a whole object means removing the object root (plus any
  now-empty parent directories, since empty dirs are forbidden) or rebuilding
  the object without the offending bitstreams. The spec provides no audit
  trail for this — record the action in an external log and/or the surviving
  object's `logs/` directory, and require explicit confirmation first.

## Pitfalls checklist

- [ ] Treating digests as case-sensitive strings when diffing inventories.
- [ ] Regenerating the root inventory instead of copying the head version's
      (whitespace/key-order drift breaks the "same file" requirement).
- [ ] Writing the sidecar before finishing inventory edits.
- [ ] Assuming `content` — honor `contentDirectory`.
- [ ] Assuming `v1`, `v2`… — honor zero-padded conventions on update.
- [ ] Emitting `created` timestamps without timezone or with sub-second-only
      granularity mismatch (RFC 3339 required).
- [ ] Adding files under a prior version directory "for efficiency".
- [ ] Deleting an object without removing now-empty hierarchy directories.
- [ ] Skipping the id-in-inventory check before destructive operations.
- [ ] Assuming a version directory contains an `inventory.json` — it is a
      SHOULD (W010), not a MUST, so head cannot be recovered by scanning for
      the highest version with a valid inventory.
- [ ] Writing an inventory or sidecar in place instead of temp-then-rename
      (POSIX) — a torn write to the root inventory loses the object.
- [ ] Leaving anything between the root `inventory.json` and its sidecar write:
      in that window the object fails validation outright (E060).
- [ ] Re-serializing the inventory on a resumed operation instead of replaying
      cached bytes (drift breaks the sidecar and E064).
- [ ] Trusting size+mtime alone that a staged source file hasn't changed since
      its digest was recorded.
