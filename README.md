# IAP Server

Google Play Billing backend for mobile apps.

![Diagram of a typical Google Play Billing integration](https://developer.android.com/static/images/google/play/billing/overview-arch.svg)

## What It Does

- Verifies one-time products and subscriptions with the Google Play Developer API.
- Stores purchases, entitlements, and Google Play notification events in PostgreSQL 17.
- Supports lightweight anonymous app users by default, with optional JWT auth for apps that add accounts later.
- Acknowledges purchases only after the verified purchase and entitlement state are durably saved.
- Accepts Real-time Developer Notifications through a Pub/Sub push endpoint for entitlement sync.
- Keeps Google credentials, package names, database settings, and auth secrets in env config.

## Structure

```text
cmd/api/main.go                              # composition root and graceful shutdown
config/config.go                            # env loading and validation
internal/auth/                              # anonymous and JWT user authenticators
internal/domain/                            # plain purchase, entitlement, and repository contracts
internal/usecase/iap_usecase.go             # purchase verification and entitlement orchestration
internal/infrastructure/playbilling/         # Google Play Developer API adapter
internal/infrastructure/repository/          # Postgres 17 repository and embedded schema
internal/delivery/http/                     # handlers, middleware, router
```

## Configuration

Copy `.env.example` to `.env` for local development.

| Variable | Required | Description |
| --- | --- | --- |
| `APP_ENV` | No | `development` or `production`. Production uses JSON logs and stricter RTDN config. |
| `PORT` | No | HTTP port. Defaults to `8080`. |
| `DATABASE_URL` | No | Full Postgres URL. If absent, built from `DB_*` values. |
| `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_HOST`, `DB_PORT` | No | Local database parts used when `DATABASE_URL` is absent. |
| `GOOGLE_PLAY_PACKAGE_NAME` | Yes | Android package name for this deployment's app. |
| `AUTH_MODE` | No | `anonymous` or `jwt`. Defaults to `anonymous`. |
| `ANONYMOUS_TOKEN_SECRET` | Anonymous only | Secret used to sign server-issued anonymous user tokens. |
| `ANONYMOUS_TOKEN_TTL_HOURS` | No | Anonymous token lifetime. Defaults to `8760`. |
| `AUTH_JWT_SECRET` | JWT only | HS256 JWT secret used when `AUTH_MODE=jwt`. |
| `REQUIRE_API_KEY` | No | Set `true` to also require `X-API-Key`. |
| `API_KEY` | Conditional | Required when `REQUIRE_API_KEY=true`. |
| `RTDN_VERIFICATION_TOKEN` | Production | Shared random token on the Pub/Sub push URL. |
| `ALLOWED_ORIGINS` | No | `*`, a single origin, or comma-separated allowed origins. |
| `TRUSTED_PROXY_CIDRS` | No | Comma-separated proxy CIDRs allowed to supply `X-Forwarded-For`. |

## Local Run

```bash
docker compose up -d
go run ./cmd/api
```

The repository applies its embedded schema on startup.

## API

### Create Anonymous Session

`POST /api/v1/iap/anonymous-sessions`

For apps without signups, call this once on first launch and store the returned token locally.

Response:

```json
{
  "user_id": "9f455c68-566f-4b8b-b92c-5ff2269d3724",
  "token": "anon.v1...",
  "expires_at": "2027-09-01T12:00:00Z"
}
```

### Verify Purchase

`POST /api/v1/iap/verify`

Headers for the default anonymous mode:

```text
X-IAP-User-Token: <server-issued-anonymous-token>
Content-Type: application/json
```

For account-based apps, set `AUTH_MODE=jwt` and use `Authorization: Bearer <jwt>` instead.

Body:

```json
{
  "product_id": "pro_monthly",
  "purchase_token": "purchase-token-from-client",
  "type": "subscription"
}
```

Response:

```json
{
  "entitled": true,
  "status": "ACTIVE",
  "product_id": "pro_monthly",
  "expires_at": "2026-10-01T12:00:00Z"
}
```

Use `type: "one_time"` for non-consumable products.

### Get Entitlement

`GET /api/v1/iap/entitlements/{productID}`

Requires the same user identity header/token as verification. Returns that user's entitlement for the product.

### Google Play RTDN

`POST /api/v1/iap/rtdn`

Headers:

```text
Authorization: Bearer <RTDN_VERIFICATION_TOKEN>
```

Configure Google Pub/Sub push delivery to this endpoint. The handler accepts the Pub/Sub push envelope, decodes the Google Play Developer Notification, records it idempotently, then re-checks Google for known purchase tokens.

### Health

`GET /healthz`

Checks database connectivity.

## Cloud Run Notes

- Deploy one Cloud Run service per mobile app.
- Set `GOOGLE_PLAY_PACKAGE_NAME`, `DATABASE_URL`, `AUTH_MODE`, and `RTDN_VERIFICATION_TOKEN` per service.
- If `AUTH_MODE=anonymous`, also set `ANONYMOUS_TOKEN_SECRET`.
- If `AUTH_MODE=jwt`, also set `AUTH_JWT_SECRET`.
- Use Application Default Credentials through the Cloud Run service identity. Do not mount or configure service account JSON keys.
- Connect Cloud Run to Cloud SQL Postgres 17 or another managed Postgres 17 instance.
- Configure a Pub/Sub topic for Google Play Real-time Developer Notifications and push it to `/api/v1/iap/rtdn`.

## Mobile Client Notes

For apps without signups, call `POST /api/v1/iap/anonymous-sessions` on first launch, store the returned token locally, and send it as `X-IAP-User-Token`. The backend customer ID is inside that signed token.

The app should complete the Google Play purchase, then send `product_id`, `purchase_token`, and `type` to this backend. Do not send `user_id` in the JSON payload.

When launching the billing flow, set an obfuscated account ID derived from the anonymous UUID or account user ID. The backend stores Google’s returned obfuscated ID so we can enforce matching later if you want that stricter check.
