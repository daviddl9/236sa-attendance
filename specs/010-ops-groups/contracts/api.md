# API Contracts: Group Member Management

**Status**: Tier 3+ (RequireUnitCommander) on all endpoints, matching existing group routes.

## PUT /api/groups/{id}/members

Replace the group's full member list atomically.

Request:

```json
{ "memberIds": ["u-abc123", "u-def456"] }
```

Responses:

```http
200 OK
{ "id": "g-...", "name": "CSS", "createdBy": "user-1", "memberCount": 2, "createdAt": "...", "updatedAt": "..." }

404 Not Found   # unknown group id
400 Bad Request # malformed body
```

## DELETE /api/groups/{id}/members/{userId}

Remove a single member.

Responses:

```http
204 No Content        # removed, or user wasn't a member (idempotent no-op)
404 Not Found         # unknown group id
401 Unauthorized      # not authenticated
403 Forbidden         # below Tier 3
```

## GET /api/groups/{id} (extended)

Existing endpoint now returns member **details**:

```json
{
  "id": "g-...", "name": "CSS", "createdBy": "user-1",
  "memberCount": 76,
  "createdAt": "2026-08-17T00:00:00Z",
  "updatedAt": "2026-08-17T00:00:00Z",
  "members": [
    { "userId": "u-1", "fullName": "D DAVID LIVINGSTON", "rank": "CPT", "battery": "HQ" }
  ]
}
```

`members` is omitted/empty for groups with no members. Sorted by name asc.
