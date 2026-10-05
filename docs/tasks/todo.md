## Task 0: Fix spec refs and stage file move
**Description:** Repair stale path and dangling section refs left by the `docs/superpowers/specs` → `docs/specs` move so the spec is internally consistent before implementation.
**Acceptance criteria:**
- [ ] `README.md` spec link points to `docs/specs/2026-10-04-gin-port-spec.md`
- [ ] No `§15.6` references remain (lines 414, 440 → `§15`)
- [ ] §3.4 GLOSSARY wording no longer says "once created"
- [ ] `git status` shows old path removed / new path staged (rename detected)
**Verification:**
- [ ] `grep -rn "superpowers/specs" . --exclude-dir=.git` empty
- [ ] `grep -n "§15\.6" docs/specs/2026-10-04-gin-port-spec.md` empty
- [ ] `git status --short` reviewed
**Dependencies:** None
**Files likely touched:**
- `README.md`
- `docs/specs/2026-10-04-gin-port-spec.md`
**Estimated scope:** Small: 1-2 files

## Task 1: Config + module scaffold (§5)
**Description:** `go.mod` plus `internal/config` env parsing with fail-fast joined `path: message` errors and `internal/db` client factory/GSI constant/`TableName()`. Resolve every dependency to latest first (`go get <mod>@latest`), then pin — floors in plan.md `Pinned dependencies`, never below.
**Acceptance criteria:**
- [ ] All 14 env vars from §5 table parse with defaults; invalid env exits non-zero before listen
- [ ] Explicit creds used only when both key and secret present; local mode uses dummy creds
- [ ] `NODE_ENV=test` flag exposed for rate-limit bypass
- [ ] `go.mod` pins every plan.md floor module at floor or newer; `go.sum` complete
**Verification:**
- [ ] `go get github.com/gin-gonic/gin@latest github.com/aws/aws-sdk-go-v2/config@latest github.com/aws/aws-sdk-go-v2/credentials@latest github.com/aws/aws-sdk-go-v2/service/dynamodb@latest github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue@latest github.com/swaggo/files@latest github.com/swaggo/gin-swagger@latest github.com/google/uuid@latest` then `go mod tidy` clean (validator intentionally unused — explicit checks per spec §2)
- [ ] `go list -m all` output recorded in the task/commit message; no module below its floor
- [ ] Tests pass: `go test ./internal/config/...`
- [ ] Build succeeds: `go build ./...`
**Dependencies:** Task 0
**Files likely touched:**
- `go.mod`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/db/db.go`
**Estimated scope:** Medium: 3-5 files

## Task 2: apperrors + pagination (§§9–10)
**Description:** Error envelope writer with all 9 codes (incl. `RATE_LIMITED` 429 + `Retry-After`), DynamoDB→HTTP mapper, and base64url sorted-keys token encode/decode.
**Acceptance criteria:**
- [ ] Envelope is exactly `{error:{message,code,statusCode,details?}}`; `INTERNAL_ERROR` message is `Internal server error`
- [ ] `ConditionalCheckFailedException` → 409 on create else 404; throttling → 503; `ResourceNotFound` → 404
- [ ] Token round-trips; decoded-equivalence holds; malformed → error for 400 mapping
**Verification:**
- [ ] Tests pass: `go test ./internal/apperrors/... ./internal/pagination/...`
- [ ] Build succeeds: `go build ./...`
**Dependencies:** Task 1
**Files likely touched:**
- `internal/apperrors/errors.go`
- `internal/apperrors/errors_test.go`
- `internal/pagination/token.go`
- `internal/pagination/token_test.go`
**Estimated scope:** Medium: 3-5 files

## Checkpoint: Foundation
- [ ] `go build ./...` succeeds
- [ ] Config, error-mapping, and token unit tests pass
- [ ] Review with human before proceeding

## Task 3: Models + schemas validation (§§3,6)
**Description:** Locked structs for static/dynamic/resort plus binding validation: closed schemas, strict booleans, `TrimSpace` Lift, `Metadata` patterns, integer `>=0`/`>0` rules, enums, `OpenLifts` elements, `limit`/`nextToken`/`minRiders`/`maxRiders` coercion, identity-mismatch and empty-update detection, normative `details` entries.
**Acceptance criteria:**
- [ ] §8 fixture bodies validate; unknown fields, float riders, `"true"` booleans, hex limits rejected
- [ ] `minRiders > maxRiders`, legacy `lastEvaluated*` params, path/body mismatch all → 400 entries
**Verification:**
- [ ] Tests pass: `go test ./internal/models/... ./internal/schemas/...`
- [ ] Build succeeds: `go build ./...`
**Dependencies:** Task 2
**Files likely touched:**
- `internal/models/models.go`
- `internal/schemas/schemas.go`
- `internal/schemas/schemas_test.go`
**Estimated scope:** Medium: 3-5 files

## Task 4: Repository with fake (§7)
**Description:** DynamoDB table I/O behind the narrow Put/Get/Query/Scan/Update/Delete interface: conditional creates, must-exist update/delete with `ALL_NEW`, PK Query, Scan list, GSI `by-riders` with BETWEEN/>=/<= branches descending, `Scan Limit=1` readiness probe, integer-preserving marshaling.
**Acceptance criteria:**
- [ ] Double-create surfaces condition failure (service maps to 409); update/delete absent surfaces condition failure (404)
- [ ] `by-riders` three branches return descending order; probe uses `ProjectionExpression=Lift`
**Verification:**
- [ ] Tests pass: `go test ./internal/repository/...` (fake + interface contract)
- [ ] Build succeeds: `go build ./...`
**Dependencies:** Task 3
**Files likely touched:**
- `internal/repository/repository.go`
- `internal/repository/fake.go`
- `internal/repository/repository_test.go`
**Estimated scope:** Medium: 3-5 files

## Task 5: Service domain rules (§§7,9)
**Description:** Pure domain layer (no Gin/SDK types): create-only vs update-only enforcement, 404/409 translation, partial-update `SET` building with empty-update guard, per-lift `by-riders` filter validation.
**Acceptance criteria:**
- [ ] Create existing → Conflict; update/delete missing → NotFound; empty update → ValidationError
- [ ] Filter `minRiders > maxRiders` rejected before repository call
**Verification:**
- [ ] Tests pass: `go test ./internal/service/...` (fake repository)
- [ ] Build succeeds: `go build ./...`
**Dependencies:** Task 4
**Files likely touched:**
- `internal/service/service.go`
- `internal/service/service_test.go`
**Estimated scope:** Small: 1-2 files

## Checkpoint: Data path
- [ ] All service tests with fake repo green
- [ ] `go vet` clean on touched packages
- [ ] Review with human before proceeding

## Task 6: Handlers + server + middleware (§§4,11)
**Description:** All 14 route rows with §4.1 identity rules (POST kind match, PUT path authoritative, `static`-on-generic → 400), collection envelope `{items,nextToken?,lastEvaluatedKey?,count}`, single-item passthrough, 204 empty delete, `x-request-id` echo, CORS `*`-forces-no-credentials, gzip ≥1024, security headers, 100/min IP limit bypassed in test, `slog`, `/docs`, graceful shutdown wiring.
**Acceptance criteria:**
- [ ] Static segments registered before `/:lift/:metadata`; unknown routes return `NOT_FOUND` envelope
- [ ] Every response carries `x-request-id`; rate-limit exceed → 429 `RATE_LIMITED` with `Retry-After: 60`
**Verification:**
- [ ] Tests pass: `go test ./internal/handlers/... ./internal/server/...`
- [ ] Build succeeds: `go build ./...`
- [ ] Manual check: `go run ./cmd/api` serves `/health`
**Dependencies:** Task 5
**Files likely touched:**
- `internal/handlers/handlers.go`
- `internal/server/server.go`
- `cmd/api/main.go`
**Estimated scope:** Medium: 3-5 files

## Task 7: HTTP integration with §8 vectors (§§4,8–10)
**Description:** `httptest` suite with fake DynamoDB behind the repository: every §4 row with locked §8 JSON, asserting status, envelope `code`, `details` on 400s, `x-request-id` echo, token/limit-cap/header assertions from §§9–11.
**Acceptance criteria:**
- [ ] Static/dynamic/resort round-trips pass verbatim from §8
- [ ] Negatives covered: 400s with `details`, 409 double-create, 404 update/delete absent, 400 empty update/malformed token/`minRiders>maxRiders`, 204 empty delete, limit default 20/cap 100
**Verification:**
- [ ] Tests pass: `go test ./internal/handlers/... -run Integration -v`
- [ ] Build succeeds: `go build ./...`
**Dependencies:** Task 6
**Files likely touched:**
- `internal/handlers/integration_test.go`
- `internal/handlers/testdata/` (fixtures)
**Estimated scope:** Small: 1-2 files

## Task 8: cmd/api + table script + Docker/compose (§§5,12)
**Description:** Env load/listen/graceful shutdown, idempotent `go run ./scripts/createtable` (keys + GSI per §7, `ResourceInUse` = success), multi-stage non-root `Dockerfile`, compose with `dynamodb-local` healthcheck and app `/health` check.
**Acceptance criteria:**
- [ ] `compose up` + create-table yields working table; app connects via `DYNAMODB_LOCAL_ENDPOINT=http://dynamodb-local:8000`
- [ ] Invalid env fails fast with joined `path: message` errors
**Verification:**
- [ ] `go run ./scripts/createtable` succeeds twice (idempotent)
- [ ] Manual check: `compose up` + `curl /health` + §8 smoke body
**Dependencies:** Task 7
**Files likely touched:**
- `cmd/api/main.go`
- `scripts/createtable/main.go`
- `Dockerfile`
- `docker-compose.yml`
**Estimated scope:** Medium: 3-5 files

## Checkpoint: HTTP + ops
- [ ] All §4 rows exercised green
- [ ] compose + create-table + §8 smoke passes
- [ ] Review with human before proceeding

## Task 9: CDK Go stack (§12)
**Description:** Native Go CDK: table + GSI (`ALL`), VPC lookup, ECS Fargate + log group + task-role grant, container env mirroring §5, `/health` container check, ALB `/ready` target check, CPU/memory autoscaling, CPU-85 + 5xx alarms + SNS, stack outputs. Separate `cdk/` module; resolve to latest first, floors `awscdk/v2 v2.272.0` + `constructs/v10 v10.8.1` (verified 2026-10-05).
**Acceptance criteria:**
- [ ] `cdk synth` succeeds with table/GSI/ALB/alarms/outputs present
- [ ] Container env names match §5 exactly
- [ ] `cdk/go.mod` pins `awscdk/v2` and `constructs/v10` at floor or newer (`go get …@latest` in `cdk/` first)
**Verification:**
- [ ] `cd cdk && go run . -- synth` exits 0 (per §17; entrypoint finalized here if it differs)
- [ ] Snapshot/template diff reviewed
**Dependencies:** Task 8
**Files likely touched:**
- `cdk/go.mod`
- `cdk/stack.go`
- `cdk/stack_test.go`
**Estimated scope:** Medium: 3-5 files

## Task 10: Coverage 80%+ + lint + full acceptance (§§13,16,17)
**Description:** Close coverage gaps to the 80%+ `go tool cover` gate, clean `gofmt`/`go vet`, and run the full §16 acceptance: suites green, smoke, synth, header/token/envelope assertions, §15 residuals recorded as ADRs.
**Acceptance criteria:**
- [ ] Coverage ≥ 80% via `go tool cover` CI gate
- [ ] `gofmt -l .` empty and `go vet ./...` clean
- [ ] No §15 item ambiguous in code (locked or `docs/adr/` entry)
**Verification:**
- [ ] Tests pass: `go test ./... --cover`
- [ ] Lint: `gofmt -l . && go vet ./...`
- [ ] Manual check: compose smoke + `cdk synth` both green
**Dependencies:** Task 9
**Files likely touched:**
- `docs/adr/*.md`
- gaps found by cover profile
**Estimated scope:** Medium: 3-5 files

## Checkpoint: Complete
- [ ] All acceptance criteria met
- [ ] Ready for review
