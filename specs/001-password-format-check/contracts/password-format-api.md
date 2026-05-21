# Contract: Password Format API Behavior

## Shared Format

Regular personnel password / NRIC Last 5 values must match:

```text
four digits followed by one alphabetic letter
```

Examples:
- Valid: `1234A`, `0001z`
- Invalid: `12345`, `123A4`, `1234@`, `1234AB`, `123A`

Invalid format message should state the expected shape and include an example such as `1234A`.

## POST /api/auth/sign-in

### Request

```json
{
  "identifier": "PTE Tan",
  "password": "1234A"
}
```

### Regular Personnel Behavior

- If `password` does not match the shared format, return a client error before creating a session or user account.
- If `password` matches the shared format, continue the existing credential verification and sign-in flow.
- Lowercase final letters are accepted for the format check.

### Administrator Behavior

- If `identifier` is the administrator account, the shared regular-personnel format does not apply.
- Existing administrator credential verification is preserved.

## POST /api/users/register

### Request

```json
{
  "fullName": "PTE Tan",
  "rank": "PTE",
  "battery": "Alpha",
  "nricLast5": "1234A",
  "dob": "010196"
}
```

### Behavior

- Reject requests when `nricLast5` does not match the shared format.
- Accept lowercase final letters for validation.
- Preserve existing required-field, rank, battery, duplicate-user, session, and redirect behavior after format validation passes.

## PUT /api/users/{id}

### Request

```json
{
  "nricLast5": "1234A"
}
```

### Behavior

- When a superadmin updates `nricLast5`, reject invalid values with a client error.
- When `nricLast5` is absent, do not change or revalidate the existing value.
- Preserve existing permission rules for who may update personnel credential fields.

## POST /api/admin/users/bulk-create

### Request

```json
{
  "users": [
    {
      "fullName": "PTE Tan",
      "rank": "PTE",
      "battery": "Alpha",
      "nricLast5": "1234A",
      "extras": {}
    }
  ]
}
```

### Behavior

- Validate each row independently.
- Invalid `nricLast5` values increment the failed count and add row-specific errors.
- Valid rows remain eligible for creation when other row validation passes.

## POST /api/admin/users/bulk-upload

### Behavior

- Uploaded file rows that map to NRIC Last 5 must use the shared format.
- Invalid rows are reported with row numbers and do not create personnel records.
- The response continues to include success, failed, skipped, errors, message, and users fields.
