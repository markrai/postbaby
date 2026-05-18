# Postbaby Backend

A minimal self-hostable backend for Postbaby.

It serves the existing Postbaby frontend and adds SQLite-backed snapshot sync behind a small username/password auth layer. The browser remains local-first; the backend stores whole Postbaby snapshots and secure server-side sessions.

This backend is for the current self-hosted sync mode. The free static/local-only mode does not require this server.

## Current Scope

This backend is intentionally small:

- Go stdlib-oriented HTTP server
- SQLite persistence
- initial owner setup flow
- username/password login
- secure cookie-backed sessions
- whole-document snapshot storage
- automatic background sync for the signed-in self-hosted app

Not included:

- OAuth or OIDC
- JWT auth
- realtime sync
- per-note merge
- Postgres
- hosted SaaS infrastructure
- custom billing forms
- multiple billing plans or plan tiers
- hosted account recovery flows

## Configuration

| Variable | Required | Default | Description |
|---|---:|---|---|
| `POSTBABY_DB_PATH` | no | `./data/postbaby.db` | SQLite database path |
| `POSTBABY_ADDR` | no | `:8080` | HTTP listen address |
| `POSTBABY_STATIC_DIR` | no | `../` | Static frontend directory |
| `POSTBABY_DEPLOYMENT_MODE` | no | `static_local` | `static_local`, `selfhosted_single_user`, or `cloud_multi_user` |
| `POSTBABY_COOKIE_SECURE` | no | `false` | Set cookie `Secure` when serving over HTTPS |
| `POSTBABY_SESSION_TTL` | no | `720h` | Session lifetime |
| `POSTBABY_BILLING_PROVIDER` | no | blank | Set to `stripe` to enable hosted billing routes in `cloud_multi_user` |
| `POSTBABY_STRIPE_SECRET_KEY` | when billing is enabled | blank | Stripe secret key placeholder for server-side checkout and portal calls |
| `POSTBABY_STRIPE_WEBHOOK_SECRET` | when billing is enabled | blank | Stripe webhook signing secret placeholder |
| `POSTBABY_STRIPE_PRICE_ID` | when billing is enabled | blank | Stripe price ID placeholder for the hosted sync subscription |
| `POSTBABY_PUBLIC_BASE_URL` | when billing is enabled | blank | Public app base URL used to build checkout and portal return URLs |

`cloud_multi_user` now supports the public app shell, hosted signup/login/logout, entitlement-gated account sync, and optional Stripe-backed billing. Real production values still belong in private deployment config.

## Auth Flow

1. In `selfhosted_single_user`, if no users exist, requests to `/` redirect to `/setup`.
2. The setup page creates the first user with `username`, `password`, and `confirm password`.
3. The first user becomes the admin/owner account.
4. After setup, the server creates a session cookie and redirects to `/`.
5. Future unauthenticated visits redirect to `/login`.
6. `POST /logout` invalidates the current session.

Setup is disabled once the first user exists.

## API Surface

Public routes:

- `GET /api/health`
- `GET /login` (`selfhosted_single_user` only)
- `POST /login` (`selfhosted_single_user` only)
- `GET /setup` (`selfhosted_single_user` only)
- `POST /setup` (`selfhosted_single_user` only)
- static assets

Authenticated routes:

- `POST /logout` (`selfhosted_single_user` only)
- `GET /api/document/meta` (`selfhosted_single_user` only)
- `GET /api/document` (`selfhosted_single_user` only)
- `PUT /api/document` (`selfhosted_single_user` only)

Authenticated API routes use the session cookie. They do not use `Authorization` headers anymore.

## Storage Model

The backend preserves the existing snapshot model:

- one `documents` row per `(owner_key, app_id)`
- `body_json` stores the current Postbaby snapshot as JSON
- `version` and `updated_at` support conservative sync checks

The current frontend uses:

- `appId = "postbaby-web"`

Sync metadata such as `postbabySyncVersion` and `postbabySyncEnabled` remains browser-local and is never uploaded. No manual sync token is required by the current backend.

## Database Schema

Tables:

- `documents`
  - `id`
  - `owner_key`
  - `app_id`
  - `body_json`
  - `version`
  - `updated_at`
- `users`
  - `id`
  - `username`
  - `password_hash`
  - `owner_key`
  - `is_admin`
  - `created_at`
- `sessions`
  - `id`
  - `user_id`
  - `token_hash`
  - `expires_at`
  - `created_at`
  - `last_seen_at`

## Migration Note

Document storage remains compatible with the earlier single-owner spike.

During first-user setup:

- if existing documents have one distinct `owner_key`, the first account adopts that owner key
- if there are no existing documents, a new owner key is generated
- if multiple distinct document owner keys already exist, setup stops and manual migration is required

That keeps old data readable without redesigning the document model.

## Security Notes

- Passwords are hashed with Argon2id.
- Session tokens are generated with cryptographic randomness.
- Only a SHA-256 hash of each session token is stored in SQLite.
- Session cookies are `HttpOnly` and `SameSite=Lax`.
- Cookie `Secure` is configurable for HTTP vs HTTPS deployments.
- Login, setup, and logout forms use a basic same-origin check using `Origin` or `Referer` when available.

## Run Locally

From the repo root:

```powershell
cd .\postbaby-backend
$env:POSTBABY_DB_PATH = ".\data\postbaby.db"
$env:POSTBABY_ADDR = ":8080"
$env:POSTBABY_STATIC_DIR = ".."
$env:POSTBABY_DEPLOYMENT_MODE = "selfhosted_single_user"
$env:POSTBABY_COOKIE_SECURE = "false"
$env:POSTBABY_SESSION_TTL = "720h"
go run ./cmd/postbaby-backend
```

Then open [http://localhost:8080/](http://localhost:8080/).

On first run, Postbaby redirects to `/setup`.

## Tests

```powershell
cd .\postbaby-backend
go test ./...
```

## Future Extension Points

This phase stays intentionally small, but the current shape leaves room for:

- multiple local users by adding user management around the existing `users` and `sessions` tables
- admin tooling for password rotation or session revocation
- optional OIDC later by swapping the login boundary while keeping documents, users, and session-backed app auth concepts intact
- other billing providers later by keeping entitlement checks local and provider-neutral

The goal is still boring self-hosted software, not a heavyweight auth platform.
