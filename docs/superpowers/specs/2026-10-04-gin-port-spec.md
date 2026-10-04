# gin-ddb (Go / Gin port) — specification

Date: 2026-10-04 · Status: **spec only, not implemented**
Canonical contract anchor: `fastify-ddb` (TypeScript/Fastify) repo,
`docs/superpowers/specs/2026-10-04-skilifts-polyglot-ports-design.md`.
This repo's spec is self-contained; where it and the TS code disagree, the TS code wins and this spec must be amended.

## 1. Goal

A Gin service that is behavior-compatible with the TypeScript `fastify-ddb` SkiLifts API:
same routes, same validation rules, same error envelope, same DynamoDB access patterns,
runnable against local DynamoDB (`amazon/dynamodb-local`), with full repo parity
(API + Dockerfile + compose + table script + tests + native Go CDK stack).

Parity model: **behavioral parity proven by duplicated native tests** (no shared runner).
The fixture vectors in §8 must produce the same outcomes as the TS suite.

## 2. Runtime versions

- Go **1.24+**, Gin, `aws-sdk-go-v2` (`service/dynamodb`, `feature/dynamodb/attributevalue`,
  `credentials`), `go-playground/validator` (or explicit checks where regexes rule),
  `swaggo` for the OpenAPI document served at `/docs`.
- Testability rule: the repository depends on a **narrow DynamoDB interface**
  (Put/Get/Query/Scan/Update/Delete), not the concrete client, so tests inject fakes;
  the service accepts a repository override (mirrors the TS constructor).

## 3. Module map (TS → Go)

| TS (`src/…`)              | Go (`internal/…`)                                   |
|---------------------------|-----------------------------------------------------|
| `config/env.ts`           | `internal/config` — parse + validate env, §5        |
| `db/dynamodb.client.ts`   | `internal/db` — client factory, `GSINAME`, `TableName()` |
| `types/skilift.types.ts`  | `internal/models` — structs + `SkiLiftRecord` union handling |
| `schemas/skilift.schemas.ts` | `internal/schemas` — binding structs + validation, §6 |
| `repositories/skilift.repository.ts` | `internal/repository` — table I/O only, §7 |
| `services/skilift.service.ts` | `internal/service` — domain rules only |
| `routes/skilift.routes.ts` + health/ready/root | `internal/handlers` (Gin handlers incl. health/ready/root) |
| `utils/error-handler.ts`  | `internal/apperrors` — codes, mapping, envelope writer, §9 |
| `utils/pagination.ts`     | `internal/pagination` — canonical tokens, §10       |
| `plugins/dynamodb.plugin.ts` | wiring in `internal/server` (client + service construction) |
| `server.ts`               | `internal/server` — engine assembly + middleware, §11 |
| `index.ts`                | `cmd/api/main.go` — env load, listen, graceful shutdown |

## 4. Route table (identical paths, methods, status codes)

| Method | Path | Success | Errors |
|---|---|---|---|
| POST | `/api/skilifts/static` | 201 static item | 400, 409 |
| POST | `/api/skilifts/dynamic` | 201 dynamic item | 400, 409 |
| POST | `/api/skilifts/resort` | 201 resort item | 400, 409 |
| GET | `/api/skilifts` | 200 paginated (Scan) | 400 |
| GET | `/api/skilifts/:lift/by-riders` | 200 paginated (GSI) | 400 |
| PUT | `/api/skilifts/:lift/static` | 200 static item | 400, 404 |
| GET | `/api/skilifts/:lift` | 200 paginated (Query PK) | 400 |
| GET | `/api/skilifts/:lift/:metadata` | 200 item | 404 |
| PUT | `/api/skilifts/:lift/:metadata` | 200 dynamic item | 400, 404 |
| DELETE | `/api/skilifts/:lift/:metadata` | 204 empty | 404 |
| GET | `/health` | 200 `{status,timestamp,uptime}` | — |
| GET | `/ready` | 200 `{status,timestamp}` or 503 `NOT_READY` | 503 |
| GET | `/` | 200 service info (same keys as TS) | — |
| GET | `/docs/*any` | Swagger UI | — |

Register static segments before `/:lift/:metadata` so `by-riders`/`static`
are never captured as `:metadata`. Route params bind to `lift`/`metadata` names.

## 5. Environment (names and defaults identical to TS)

`NODE_ENV` (name kept for env-file compatibility), `PORT=3000`, `HOST=0.0.0.0`,
`LOG_LEVEL=info`, `CORS_ORIGIN=*`, `CORS_CREDENTIALS=false`,
`RATE_LIMIT_MAX=100`, `RATE_LIMIT_TIME_WINDOW="1 minute"` (same per-minute semantics),
`DYNAMODB_MODE=local`, `DYNAMODB_TABLE_NAME=SkiLifts`,
`DYNAMODB_LOCAL_ENDPOINT=http://localhost:8000`, `DYNAMODB_LOCAL_REGION=us-east-1`,
`AWS_REGION=us-east-1`, optional `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`
(only set explicit creds when **both** present; local mode uses static dummy creds).
Invalid env → fail fast at startup with a joined `path: message` error.

## 6. Validation rules (mirror Zod semantics exactly)

Same rules as the Python spec §6, enforced at binding time with identical messages/codes:
non-empty `Lift`; `Metadata` `^\d{2}\/\d{2}\/\d{2}$`; strict booleans (no truthy coercion);
positive `VerticalFeet`; `LiftTime` `^\d{1,2}:\d{2}$`; non-negative rider/snow counts;
`LiftStatus` ∈ `Open|Closed|Pending`; `AvalancheDanger` ∈ 5-value set;
`OpenLifts` numeric list; `limit` decimal-int coerce → positive → max 100 → default 20
(JS-style `0x10` coercions NOT reproduced — documented divergence);
malformed `nextToken` → 400; `lastEvaluated*` both-or-neither; empty update → 400;
`minRiders > maxRiders` → 400.
Binding/validation failures (incl. malformed JSON) → 400 envelope with per-field details.

## 7. Repository (table I/O only; domain rules stay in the service)

Table `SkiLifts`: PK `Lift` (S) + SK `Metadata` (S).
GSI `SkiLiftsByRiders`: PK `Lift` (S) + SK `TotalUniqueLiftRiders` (N), projection `ALL`.

- `PutItem` with `attribute_not_exists(Lift) AND attribute_not_exists(Metadata)`;
  `ConditionalCheckFailedException` → **409** `CONFLICT` (create path only).
- `GetItem` → absent → service returns 404.
- `Query` on PK for lift collection; `UpdateItem` with `SET #a=:a` name placeholders,
  exists-condition, `ReturnValues=ALL_NEW`; `DeleteItem` with exists-condition;
  condition failures on update/delete → 404 via the error mapper.
- `Scan` for list; GSI `Query` with descending order, BETWEEN/`>=`/`<=` branches
  identical to TS; `ping` via `Scan Limit=1 ProjectionExpression=Lift`.
- **Numbers:** use `attributevalue` marshaling that preserves integral values as JSON
  integers (`2500`, never `2500.0`); `OpenLifts` as a JSON number list (DynamoDB List,
  matching code truth — not the stale `NS` in `datamodel/skilifts.json`).

## 8. Shared fixture vectors (must pass identically here and in TS)

Same vectors as the Python spec §8: static/dynamic/resort round trips
(`Summit Express`, `01/15/24`, riders `1250`, snow `48`, `Open`/`Low`;
static `false/2500/"8:00"`; resort `8500/42/Moderate/OpenLifts=[1..12]`) plus negatives:
invalid bodies → 400 with `details`; double-create → 409; update/delete absent → 404;
empty update → 400; malformed `nextToken` → 400; `minRiders>maxRiders` → 400;
delete → 204 empty body; `limit` default 20 / cap 100.

## 9. Error envelope (identical)

`{error:{message, code, statusCode, details?}}`, codes:
`NOT_FOUND` 404 · `VALIDATION_ERROR` 400 · `CONFLICT` 409 · `THROTTLED` 503 ·
`DYNAMODB_ERROR` 500 · `INTERNAL_ERROR` 500 (message always `Internal server error`) ·
`REQUEST_ERROR` (4xx framework errors, client message) · `NOT_READY` 503 (`/ready` only).
AWS mapping: `ConditionalCheckFailedException` → 404 except create path → 409;
`ResourceNotFoundException` → 404; throttling pair → 503 `THROTTLED`; else 500.
Unknown errors never leak internals. Unknown routes → 404 `NOT_FOUND` envelope
(via `NoRoute`/`NoMethod` handlers using the same writer).

## 10. Pagination tokens (canonical)

`nextToken` = base64url (**`RawURLEncoding`, no padding**) of compact JSON of the DynamoDB
`LastEvaluatedKey` with **keys sorted alphabetically**, UTF-8. (Go maps marshal with
sorted keys by default — keep it that way and test it explicitly.)
Responses keep the deprecated `lastEvaluatedKey: {Lift, Metadata}` alongside `nextToken`,
plus `count`. Conformance compares tokens by **decoded equivalence**, not raw string.

## 11. Server behavior

- `x-request-id`: honor incoming, else uuid; echo on **all** responses incl. errors.
- CORS: `*` origin forces credentials off (mirror the TS guard).
- Gzip ≥ 1024 bytes; security headers equivalent to the TS helmet set
  (conformance asserts at least `x-content-type-options: nosniff` and the configured CSP);
  rate limit 100/min/IP, **disabled** when `NODE_ENV=test`; `slog` structured logging
  at `LOG_LEVEL`; OpenAPI UI at `/docs`; graceful shutdown on SIGINT/SIGTERM.

## 12. Local run, Docker, CDK

- `docker-compose.yml`: `dynamodb-local` (same image/flags/healthcheck as TS) + `app`
  (Go binary, `DYNAMODB_LOCAL_ENDPOINT=http://dynamodb-local:8000`, `/health` healthcheck).
- `scripts/createtable` (`go run ./scripts/createtable`): same keys/GSI/throughput as TS;
  idempotent on `ResourceInUse`.
- `Dockerfile`: multi-stage (`golang:1.27-bookworm` build → minimal runtime), non-root,
  `EXPOSE`/env parity.
- `cdk/` (native Go CDK with `awscdk` + `constructs`, jsii sidecar at synth time):
  mirror `fargate-stack.ts` resource-for-resource — table+GSI (`ALL`), VPC lookup,
  cluster, log group, task def, task-role grant, container env (same names),
  `/health` container check, ALB (`/ready` target check), CPU/memory autoscaling,
  CPU-85 and 5xx alarms + SNS, same outputs.

## 13. Tests

`go test ./...`: unit (validation tables, service with fake repository, error mapping,
tokens incl. malformed, config parsing) + HTTP-level integration via `httptest`
(fake DynamoDB behind the repository interface: every row of §4/§8). Mirror the TS
assertions (status, envelope code, details presence, headers). Coverage 80%+ via
`go tool cover` gate in CI, same thresholds as TS.

## 14. Non-goals

No auth, no pagination redesign, no seed script (TS has none), no TS behavior changes
beyond token canonicalization (tracked TS-side prerequisite), no shared-runner harness.

## 15. Acceptance

All suites green at 80%+ coverage; `compose up` + create-table + §8 smoke passes;
`cdk synth` succeeds; header/token/envelope assertions in §§9–11 hold.
