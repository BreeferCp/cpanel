# Billing API

For WHMCS, Paymenter or any system that sells hosting. All endpoints live under `/api/v1/billing`, use JSON, and need an API token with the **`billing`** scope:

```
Authorization: Bearer bcp_xxxxxxxx
```

Create the token under *Profile & Security → API tokens*. Restrict it to the billing server's IP address and set an expiry.

A **root** token manages every customer; a **reseller** token only manages that reseller's customers and packages.

## Idempotency

The billing system's service id is sent as `external_ref`. Creating an account with an `external_ref` that already exists returns the existing account (`200`, no password) instead of failing or duplicating it, and suspend, unsuspend and terminate are all safe to repeat. Retry on network errors without fear.

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/billing/packages` | List packages the token may assign |
| `POST` | `/billing/accounts` | Create an account |
| `GET` | `/billing/accounts/{ref}` | Fetch an account (includes `disk_used_mb`, `bw_used_mb`, `status`) |
| `POST` | `/billing/accounts/{ref}/suspend` | Suspend (`{"reason": "..."}` optional) |
| `POST` | `/billing/accounts/{ref}/unsuspend` | Reactivate |
| `POST` | `/billing/accounts/{ref}/terminate` | Delete the account and all its data |
| `POST` | `/billing/accounts/{ref}/package` | Change plan (`{"package_id": "..."}`) |
| `POST` | `/billing/accounts/{ref}/password` | Set the panel password |
| `POST` | `/billing/accounts/{ref}/sso` | One-time login token for the customer |

`{ref}` is the `external_ref` used at creation.

### Create an account

```http
POST /api/v1/billing/accounts
{
  "username": "acme",
  "email": "ops@acme.test",
  "domain": "acme.test",
  "package_id": "dd3d54a1…",
  "external_ref": "1234",
  "password": "optional; a strong one is generated when omitted"
}
```

`201` returns `{"account": {…}, "password": "…"}`. The password is present **only** when it was generated, and only in this response.

### Single sign-on

`POST …/sso` returns `{"token": "…", "path": "/sso#token=…", "url": "https://<host>:2083/sso#token=…", "expires_in": 60}`. Redirect the customer's browser to `url`, which points at the cPanel port (customers cannot sign in on the WHM port 2087 the API is served from). The token is valid once for 60 seconds. When BreeferCP runs a single combined listener, `url` is empty and you use the API address + `path`.

### Errors

```json
{"error": {"code": "limit_reached", "message": "your plan does not allow more domains"}}
```

| Status | Meaning |
|---|---|
| 400 | Invalid input (`message` says what) |
| 401 | Missing, expired, revoked or IP-blocked token |
| 403 | Token lacks the scope |
| 404 | Unknown `ref` (or not owned by this token's owner) |
| 409 | Conflict, for example the username or domain is already taken |
| 429 | Rate limited |

## Integrations

- WHMCS server module: [`integrations/whmcs`](../integrations/whmcs)
- Paymenter extension: [`integrations/paymenter`](../integrations/paymenter)

Both are thin clients of this API. The Paymenter extension follows the v1 `Server` extension interface; check it against your Paymenter version before relying on it.
