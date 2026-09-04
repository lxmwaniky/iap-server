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