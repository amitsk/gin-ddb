# ADR 002: Content-Security-Policy scope

Date: 2026-10-05 · Status: accepted (interim — spec §15 CSP value still open)

## Context
Spec §11 requires a configured CSP but §15 leaves the exact value TBD.
Swagger UI needs inline scripts, so a strict API policy breaks `/docs`.

## Decision
- API routes: `Content-Security-Policy: default-src 'self'`.
- `/docs/*`: no CSP header (exempt).
- `X-Content-Type-Options: nosniff` on all responses.

## Consequences
- `/docs` is less locked down than the API until the final CSP value lands.
- Locking the §15 value means revisiting this ADR, not the middleware shape.
