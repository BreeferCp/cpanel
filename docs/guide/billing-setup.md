# Connecting a billing system

Reference for the API itself: [billing-api.md](../billing-api.md).

## 1. Create an API token

Sign in as the administrator (or reseller), open **Profile & Security → API tokens → Create token**:

- Scope: **Billing & provisioning**
- Allowed IP addresses: the IP of your billing server (strongly recommended)
- Expiry: for example 365 days

Copy the token immediately; it is shown once. Also create at least one **package** and note its ID.

## 2. WHMCS

1. Copy `integrations/whmcs/modules/servers/breefercp` to `<whmcs>/modules/servers/breefercp`.
2. **Configuration → System Settings → Servers → Add New Server**: type **BreeferCP**, hostname = panel hostname, port `2087`, tick **Secure**, **Access Hash** = the token. Use **Test Connection**.
3. Create a product, **Module Settings → Module Name: BreeferCP**, **Package ID** = the BreeferCP package id, and choose automatic setup on payment.

Create, suspend, unsuspend, terminate, change package, change password, usage update and single sign-on are supported. WHMCS service ids are sent as `external_ref`, so retried jobs never duplicate an account. The WHMCS username must be 3-16 lowercase letters or digits.

## 3. Paymenter

1. Copy `integrations/paymenter/BreeferCP` to `<paymenter>/extensions/Servers/BreeferCP`.
2. **Admin → Extensions → BreeferCP**: panel URL (with port) and the token.
3. In the product, choose the BreeferCP server extension and set **BreeferCP package ID**. Ask for a `domain` at checkout.

The extension follows the Paymenter v1 server-extension interface; verify it against your installed version.

## 4. Anything else

Call the REST API directly; see [billing-api.md](../billing-api.md) for endpoints, errors and an idempotency guide.
