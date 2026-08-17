# Quickstart: Ops-Group Seeding

**Feature**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md)

## Seed the ops groups

1. Ensure the roster users are imported into the DB (the existing roster import flow creates them with `nric_last5` set).
2. Pick an admin user id to own the groups — the seeded groups appear under that creator (the Groups page lists all groups, not per-creator, but the idempotence reuse-key is creator+name).

```sh
# dev
go run ./backend/cmd/seed-groups \
  -file ~/Downloads/"236SA 2026 ICT 6 callup (test) 2 (1).xlsx" \
  -password yeoec \
  -created-by "$(psql $DATABASE_URL -tAc "SELECT id FROM \"user\" WHERE is_superadmin LIMIT 1;")"

# prod (on the server)
go build -o /tmp/seed-groups ./backend/cmd/seed-groups
scp /tmp/seed-groups ubuntu@redcon.236sa.one:/tmp/
ssh ubuntu@redcon.236sa.one \
  '/tmp/seed-groups -file <roster>.xlsx -password yeoec -created-by <id> -db-url <DATABASE_URL>'
```

3. Confirm counts: RnS 8 · FDC/BOC 11 · BCS 16 · CSS 76 · PSO 40 · A Bty 90 · B Bty 89 · HQ Bty 139.

Re-run any time after roster/import updates — it only adds.

## Manual group management (UI)

- Groups → click a group → members list; remove members.
- "Add people" → search the roster or create a new person inline → they're added to the group.

## Verification

- `go test ./...` (backend), `npm run build` + `npm run lint` (frontend).
- Run the seeder twice; assert no duplicates and a manually-added member survives.
