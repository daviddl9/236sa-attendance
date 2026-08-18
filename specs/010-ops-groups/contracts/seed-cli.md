# CLI Contract: seed-groups

## Usage

```sh
go run ./backend/cmd/seed-groups \
  -file "/path/to/callup.xlsx" \
  -password "yeoec" \
  -created-by "$ADMIN_USER_ID" \
  -db-url "$DATABASE_URL"
```

Flags:

| Flag | Required | Default | Notes |
|---|---|---|---|
| `-file` | yes | — | roster Excel path |
| `-password` | yes | — | Excel open password |
| `-created-by` | yes | — | user id that owns the seeded groups (must exist) |
| `-db-url` | no | `$DATABASE_URL` | postgres DSN |

## Exit / output

- Exit 0 on success; non-zero with a message on failure (unreadable file, wrong password, DB unreachable).
- Prints a summary on stdout:

```
Seeding 8 groups from roster (431 rows)…

RnS          created 1 · members 8   (0 unmatched)
FDC/BOC      created 1 · members 11  (0 unmatched)
BCS          created 1 · members 16  (0 unmatched)
CSS          reused 1 · members 76   (0 unmatched)
PSO          created 1 · members 40  (0 unmatched)
A Bty        reused 1 · members 90   (0 unmatched)
B Bty        created 1 · members 89  (1 unmatched: <name>)
HQ Bty       created 1 · members 139 (0 unmatched)

Done. 8 groups, 469 memberships. Re-runnable (additive, idempotent).
```

- Unmatched rows are listed under the group whose rule matched them but found no user in the DB.
