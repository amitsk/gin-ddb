# Ledger — plan: docs/tasks/plan.md

- Task 0: complete (28654b3, verification: grep clean + `git status` clean)
- Ruling: feature branch `feature/skilifts-api` created for implementation — main stays deployable; will merge at end.
- Ruling: module path `gin-ddb` (no org known) — cost if wrong: one `go mod edit -module` rename.
- Task 1: complete (06f495b, tests: `go test ./internal/config/...` → ok)
- Ruling: deps pin on first import, not all in Task 1 — `go mod tidy` drops
  unimported modules; aws stack + swag tool pinned at floors in Task 1,
  gin/validator/uuid/attributevalue/gin-swagger pin when imported
  (Tasks 2-6), final floor check in Task 10 — cost if wrong: version drift,
  caught by Task 10 gate.
- Ruling: pagination works on `map[string]string` raw key values, not SDK
  AttributeValues — keeps SDK types out of service/pagination layers;
  repository wraps to `{"S": …}` — cost if wrong: small translation code.
- Task 2: complete (eb0d417+07977ba, tests: apperrors + pagination → ok; gin v1.12.0, uuid v1.6.0 pinned at floors)
- Task 3: complete (94ce0b3, tests: `go test ./internal/schemas/...` → 24/24 pass)
- Ruling: validator stays unpinned/unused — every §6 rule is a regex,
  integer-range, enum, or structural check implemented explicitly, which
  spec §2 permits ("or explicit checks where regexes rule"); plan floor
  table updated to mark it unused — cost if wrong: none, zero imports.
- Task 4: complete (b9ecc20+483d03e, tests: `go test ./internal/repository/...` → 11/11 pass; attributevalue v1.21.8 pinned at floor)
- Task 5: complete (036f725, tests: `go test ./internal/service/...` → 8/8 pass)
- Task 6+7: complete (4dd2113, tests: server 6/6 + integration 18/18 pass, full suite green)
- Ruling: `UseRawPath=true` + `UnescapePathValues=true` on the engine —
  `%2F`-encoded dates in `:metadata` must not split into segments; found
  via failing integration tests (GET/PUT dynamic → 404), fixed, green —
  cost if wrong: date routes 404 for every client.
- Ruling: CSP `default-src 'self'` on API routes only, `/docs` exempt
  (Swagger UI needs inline scripts) until §15 locks the CSP value —
  cost if wrong: docs UI breakage, recorded for Task 10 ADR.
- Ruling: integration test is `package handlers_test` (external) — server
  imports handlers, so an in-package test would cycle — cost if wrong: none.
