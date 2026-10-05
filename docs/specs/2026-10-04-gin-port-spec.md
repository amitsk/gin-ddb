# gin-ddb — SkiLifts API specification

Date: 2026-10-04 (revised 2026-10-05) · Status: **spec only, not implemented**

This spec is self-contained. It defines the SkiLifts service from first
principles. No external implementation is normative.

## 1. Goal

A Go / Gin HTTP service managing ski-lift records, backed by DynamoDB,
runnable against local DynamoDB (`amazon/dynamodb-local`).

Scope: HTTP API + Dockerfile + compose + table creation script + tests +
native Go CDK stack.

## 2. Runtime and layout

- Go **1.24+**, Gin, `aws-sdk-go-v2` (`service/dynamodb`,
  `feature/dynamodb/attributevalue`, `credentials`),
  `go-playground/validator` (or explicit checks where regexes rule),
  `swaggo` for the OpenAPI document served at `/docs`.
- Testability rule: the repository depends on a **narrow DynamoDB interface**
  (Put/Get/Query/Scan/Update/Delete), not the concrete client, so tests inject
  fakes. The service accepts a repository override via constructor/wiring.

Go layout (normative for this repo):

| Package | Responsibility |
|---|---|
| `internal/config` | parse + validate env, §5 |
| `internal/db` | client factory, GSI name constant, `TableName()` |
| `internal/models` | structs for the three record kinds, §3 |
| `internal/schemas` | request binding structs + validation, §6 |
| `internal/repository` | DynamoDB table I/O only, §7 |
| `internal/service` | domain rules only (no HTTP, no DynamoDB SDK types) |
| `internal/handlers` | Gin handlers incl. health/ready/root, §4 |
| `internal/apperrors` | error codes, DynamoDB→HTTP mapping, envelope writer, §9 |
| `internal/pagination` | opaque token encode/decode, §10 |
| `internal/server` | engine assembly + middleware + wiring, §11 |
| `cmd/api/main.go` | env load, listen, graceful shutdown |
| `scripts/createtable` | table creation CLI (`go run ./scripts/createtable`) |
| `cdk/` | native Go CDK stack, §12 |

## 3. Domain model

Single DynamoDB table stores three record kinds sharing one key shape.

- Partition key `Lift` (String): ski-lift or resort name. Non-empty.
- Sort key `Metadata` (String): discriminates the kind:
  - literal `static` → static record (fixed lift properties),
  - literal `resort` → resort aggregate record,
  - date `MM/DD/YY` matching `^\d{2}\/\d{2}\/\d{2}$` → dynamic daily record.

### 3.1 Static record (`Metadata = "static"`)

Fixed properties of one lift. Full JSON schema (locked):

```json
{
  "Lift": "Summit Express",
  "Metadata": "static",
  "IsDetachable": false,
  "VerticalFeet": 2500,
  "LiftTime": "8:00"
}
```

- `Lift`: required string, `TrimSpace` then non-empty (see §6).
- `Metadata`: required literal `"static"`.
- `IsDetachable`: required boolean, strict (no coercion).
- `VerticalFeet`: required integer `> 0`.
- `LiftTime`: required string matching `^\d{1,2}:\d{2}$`.
- No additional fields permitted on create (`additionalProperties: false`);
  unknown fields → 400 `VALIDATION_ERROR`.

### 3.2 Dynamic record (`Metadata = MM/DD/YY`)

One day snapshot for one lift. Full JSON schema (locked):

```json
{
  "Lift": "Summit Express",
  "Metadata": "01/15/24",
  "TotalUniqueLiftRiders": 1250,
  "SnowCount": 48,
  "LiftStatus": "Open",
  "AvalancheDanger": "Low"
}
```

- `Lift`: required string, `TrimSpace` then non-empty.
- `Metadata`: required date matching `^\d{2}\/\d{2}\/\d{2}$`.
- `TotalUniqueLiftRiders`: required integer `>= 0`. Also the GSI sort key.
- `SnowCount`: required integer `>= 0`.
- `LiftStatus` ∈ `Open|Closed|Pending` (case-sensitive, required).
- `AvalancheDanger` ∈ `Low|Moderate|Considerable|High|Extreme`
  (case-sensitive, required — North-American danger scale, locked).
- No additional fields permitted on create; unknown fields → 400.

### 3.3 Resort record (`Metadata = "resort"`)

Aggregate for one resort area. Full JSON schema (locked):

```json
{
  "Lift": "Summit Resort",
  "Metadata": "resort",
  "TotalRiders": 8500,
  "TotalSnow": 42,
  "AvalancheDanger": "Moderate",
  "OpenLifts": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]
}
```

- `Lift`: required string, `TrimSpace` then non-empty (resort/area name).
- `Metadata`: required literal `"resort"`.
- `TotalRiders`: required integer `>= 0`.
- `TotalSnow`: required integer `>= 0`.
- `AvalancheDanger` ∈ same 5-value set as §3.2, required.
- `OpenLifts`: required list of JSON integers (each `>= 0`);
  DynamoDB List of Numbers (not Number Set). Empty list allowed.
- No additional fields permitted on create; unknown fields → 400.

### 3.4 Glossary

- **Lift**: PK value; identifies one lift.
- **Metadata**: SK value; `static`, `resort`, or a date.
- **Static / Dynamic / Resort**: the three record kinds above.
- **TotalUniqueLiftRiders**: GSI sort key; dynamic rider count, reused for
  rider-ordered queries.

`docs/GLOSSARY.md` is normative for term definitions.

## 4. Routes

| Method | Path | Success | Errors |
|---|---|---|---|
| POST | `/api/skilifts/static` | 201 static item | 400, 409 |
| POST | `/api/skilifts/dynamic` | 201 dynamic item | 400, 409 |
| POST | `/api/skilifts/resort` | 201 resort item | 400, 409 |
| GET | `/api/skilifts` | 200 paginated list (Scan) | 400 |
| GET | `/api/skilifts/:lift/by-riders` | 200 paginated, rider-ordered desc (GSI) | 400 |
| PUT | `/api/skilifts/:lift/static` | 200 static item (partial update) | 400, 404 |
| GET | `/api/skilifts/:lift` | 200 paginated lift collection (Query PK) | 400 |
| GET | `/api/skilifts/:lift/:metadata` | 200 single item | 400, 404 |
| PUT | `/api/skilifts/:lift/:metadata` | 200 dynamic or resort item (partial update) | 400, 404 |
| DELETE | `/api/skilifts/:lift/:metadata` | 204 empty body | 400, 404 |
| GET | `/health` | 200 `{status,timestamp,uptime}` | — |
| GET | `/ready` | 200 `{status,timestamp}` or 503 `NOT_READY` | 503 |
| GET | `/` | 200 service info per §4.1 | — |
| GET | `/docs/*any` | Swagger UI | — |

#### 4.1 Identity and update semantics (locked)

- POST body carries `Lift` and `Metadata`. Body `Metadata` must equal the
  route kind (`static` / date / `resort`); mismatch → 400.
- PUT path is authoritative. If the JSON body contains `Lift` or `Metadata`
  and it differs from `:lift` / `:metadata` (after trim), → 400
  `VALIDATION_ERROR`. Bodies may omit keys; the path supplies them.
- `PUT /:lift/static` forces `Metadata="static"`.
- `PUT /:lift/:metadata` accepts `:metadata` = date (`MM/DD/YY`) for dynamic
  updates or literal `resort` for resort updates. Literal `static` on this
  route → 400 (use the `/static` route).
- PUT is a partial update (despite the verb): only provided updatable fields
  are `SET`; absent fields are preserved. Empty update (no updatable fields
  after stripping keys) → 400. This matches DynamoDB `UpdateItem` parity
  and is documented here instead of introducing `PATCH`.
- `by-riders` is per-`Lift` only (GSI PK=`Lift`): it orders that lift's
  dynamic records by `TotalUniqueLiftRiders` descending. No cross-lift
  leaderboard. Query params: `minRiders`, `maxRiders` (see §6);
  `limit` caps `items` in the response page and is passed as DynamoDB `Limit`.
- `GET /` returns exactly
  `{"service":"gin-ddb","version":"1.0.0","status":"ok","table":TABLE,"region":REGION}`
  where `TABLE`=`DYNAMODB_TABLE_NAME`, `REGION`=`AWS_REGION` or
  `DYNAMODB_LOCAL_REGION` in local mode.

Rules:

- Register static segments (`static`, `by-riders`) before `/:lift/:metadata`
  so they are never captured as `:metadata`. Route params bind to
  `lift` / `metadata` names.
- POST = create only. PUT = update existing only. DELETE = remove existing.
- Collection GETs return `{items, nextToken?, lastEvaluatedKey?, count}`; see §10.
- Single-item GETs return the item JSON directly.
- Unknown routes/methods return the §9 `NOT_FOUND` envelope.

## 5. Environment

| Name | Default | Notes |
|---|---|---|
| `NODE_ENV` | — | name kept for env-file compatibility; `test` disables rate limiting |
| `PORT` | `3000` | listen port |
| `HOST` | `0.0.0.0` | bind address |
| `LOG_LEVEL` | `info` | slog level |
| `CORS_ORIGIN` | `*` | when `*`, credentials are forced off (§11) |
| `CORS_CREDENTIALS` | `false` | |
| `RATE_LIMIT_MAX` | `100` | requests per IP per window |
| `RATE_LIMIT_TIME_WINDOW` | `1 minute` | fixed per-minute window semantics |
| `DYNAMODB_MODE` | `local` | `local` \| AWS mode TBD |
| `DYNAMODB_TABLE_NAME` | `SkiLifts` | |
| `DYNAMODB_LOCAL_ENDPOINT` | `http://localhost:8000` | used in local mode |
| `DYNAMODB_LOCAL_REGION` | `us-east-1` | |
| `AWS_REGION` | `us-east-1` | |
| `AWS_ACCESS_KEY_ID` | — | optional; explicit creds only when **both** key and secret present |
| `AWS_SECRET_ACCESS_KEY` | — | local mode otherwise uses static dummy creds |

Invalid env → fail fast at startup with a joined `path: message` error
(one line per bad variable, process exits non-zero before listening).

## 6. Validation

Enforced at binding time. Any failure — including malformed JSON — returns
the §9 `VALIDATION_ERROR` envelope (400) with per-field `details`.

- `Lift`: required, `strings.TrimSpace` then non-empty; empty after trim → 400.
  Path `:lift` is trimmed the same way before Query/Get.
- `Metadata` (path or body): dynamic dates must match
  `^\d{2}\/\d{2}\/\d{2}$`; literals `static` / `resort` only where the route
  allows them per §4.1.
- Booleans: strict — no truthy/falsy coercion (`"true"`, `1`, etc. rejected).
- `VerticalFeet`: positive integer (`> 0`).
- `LiftTime`: `^\d{1,2}:\d{2}$`.
- Rider / snow counts (`TotalUniqueLiftRiders`, `SnowCount`, `TotalRiders`,
  `TotalSnow`): integers `>= 0`. Non-integers (floats, numeric strings)
  rejected.
- `LiftStatus` ∈ `Open|Closed|Pending` (case-sensitive).
- `AvalancheDanger` ∈ `Low|Moderate|Considerable|High|Extreme`
  (case-sensitive, locked).
- `OpenLifts`: list of JSON integers, each `>= 0`. Non-list, float
  elements, or negative elements → 400. Empty list allowed.
- `IsDetachable`: strict boolean.
- Unknown JSON fields on POST/PUT bodies → 400 (schemas are closed).
- `limit` (query): decimal-integer string, coerced to int; must be `> 0`;
  capped at `100`; defaults to `20` when absent. Hex (`0x…`), float, and
  non-numeric strings are invalid → 400. (No hex coercion by design.)
- `nextToken` (query): must decode per §10; malformed → 400.
- Legacy pagination inputs removed: `lastEvaluatedLift`,
  `lastEvaluatedMetadata`, `lastEvaluatedKey` as query params are NOT
  accepted; if supplied → 400 `VALIDATION_ERROR` with message
  `use nextToken`. Response still echoes `lastEvaluatedKey` per §10.
- `by-riders` filters: `minRiders`, `maxRiders` (locked names) — each, when
  present, a decimal-integer string coerced to int `>= 0` (same coercion
  rules as `limit`: hex/float/non-numeric → 400); `minRiders > maxRiders`
  → 400.
- Update bodies: empty update (no updatable fields) → 400.
- `details` shape (locked): `details` is an array of
  `{"field": string, "message": string}`. `field` is the JSON field or
  query-param name (`body.Lift`, `params.limit` style flattened to
  `Lift`, `limit`, `minRiders`, etc.). Message strings:
  `required`, `must be a non-empty string`, `must match pattern …`,
  `must be an integer >= 0`, `must be > 0`, `invalid value`,
  `unknown field`, `must match path`, `empty update`,
  `minRiders must be <= maxRiders`, `malformed token`, `use nextToken`,
  `invalid JSON`. Status code and `details` presence are normative;
  the strings above are normative.

## 7. Repository (table I/O only; domain rules stay in the service)

Table `SkiLifts`: PK `Lift` (S) + SK `Metadata` (S).
GSI `SkiLiftsByRiders`: PK `Lift` (S) + SK `TotalUniqueLiftRiders` (N),
projection `ALL`.

- Create (`PutItem`) with condition
  `attribute_not_exists(Lift) AND attribute_not_exists(Metadata)`.
  Condition failure on the create path → 409 `CONFLICT` via §9 mapper.
- Read (`GetItem`): absent → service returns 404.
- Lift collection: `Query` on PK.
- Update (`UpdateItem`): `SET #a=:a` with expression-attribute-name
  placeholders, row-must-exist condition, `ReturnValues=ALL_NEW`.
- Delete (`DeleteItem`): row-must-exist condition.
  Condition failure on update/delete → 404 via §9 mapper.
- List: `Scan`.
- Rider-ordered query: GSI `Query`, descending order, with three branches:
  `BETWEEN min AND max` when both bounds present, `>= min` / `<= max` when
  one bound present, unbounded otherwise.
- Readiness probe: `Scan Limit=1 ProjectionExpression=Lift`.
- Numbers: marshal via `attributevalue` so integral values encode as JSON
  integers (`2500`, never `2500.0`). `OpenLifts` is a DynamoDB List of
  numbers (not a Number Set).

## 8. Fixture vectors (acceptance examples)

These vectors must pass and double as the smoke test (field names locked
per §3):

- Static round trip: create + fetch
  `{"Lift":"Summit Express","Metadata":"static","IsDetachable":false,"VerticalFeet":2500,"LiftTime":"8:00"}`.
- Dynamic round trip:
  `{"Lift":"Summit Express","Metadata":"01/15/24","TotalUniqueLiftRiders":1250,"SnowCount":48,"LiftStatus":"Open","AvalancheDanger":"Low"}`.
- Resort round trip:
  `{"Lift":"Summit Resort","Metadata":"resort","TotalRiders":8500,"TotalSnow":42,"AvalancheDanger":"Moderate","OpenLifts":[1,2,3,4,5,6,7,8,9,10,11,12]}`.
- Negatives: invalid bodies → 400 with `details`; double-create → 409;
  update/delete absent → 404; empty update → 400; malformed `nextToken` → 400;
  `minRiders > maxRiders` → 400; delete → 204 with empty body;
  `limit` default 20 / cap 100. Update resort via `PUT /:lift/resort`;
  `PUT /:lift/static` on the generic route → 400.

## 9. Errors

Envelope: `{error:{message, code, statusCode, details?}}`.
`details` is present on every `VALIDATION_ERROR` (possibly empty array
never — at least one entry) and absent otherwise, except `REQUEST_ERROR`
may carry a single-entry `details` with `field` set to the offending
header/query/body location.

| Code | HTTP | When |
|---|---|---|
| `NOT_FOUND` | 404 | unknown route/method; missing item; failed exists-condition on update/delete; missing table |
| `VALIDATION_ERROR` | 400 | any §6 failure, incl. malformed JSON and bad query params; path/body identity mismatch; unknown body fields |
| `CONFLICT` | 409 | failed exists-condition on create path only |
| `THROTTLED` | 503 | DynamoDB throttling errors (`ThrottlingException`, `ProvisionedThroughputExceededException`, `RequestLimitExceeded`) |
| `RATE_LIMITED` | 429 | IP rate limit exceeded (100 req/min); `Retry-After: 60` header |
| `DYNAMODB_ERROR` | 500 | other DynamoDB failures |
| `INTERNAL_ERROR` | 500 | unknown errors; message is always exactly `Internal server error` |
| `REQUEST_ERROR` | 4xx | framework-level client errors preserving client message: bad `Content-Type`, body too large, header binding failure |
| `NOT_READY` | 503 | `GET /ready` when the DynamoDB probe fails |

DynamoDB mapping: `ConditionalCheckFailedException` → 404, except create
path → 409; `ResourceNotFoundException` → 404; throttling errors → 503
`THROTTLED`; all else → 500. Unknown errors never leak internals.

## 10. Pagination

- Request: `limit` (§6) + optional opaque `nextToken`. `nextToken` is the
  only accepted cursor; legacy `lastEvaluated*` query params → 400 per §6.
- `nextToken` = base64url (`RawURLEncoding`, no padding) of compact JSON of
  the DynamoDB `LastEvaluatedKey`, with **keys sorted alphabetically**, UTF-8.
  (Go maps marshal with sorted keys by default — keep it and test it.)
- Response keeps `lastEvaluatedKey: {Lift, Metadata}` (deprecated but returned)
  alongside `nextToken`, plus `count` (items in this page).
- Token equality is judged by **decoded equivalence**, not raw string match.
- Malformed token → 400 `VALIDATION_ERROR`.
- `limit` is the max `items` returned; DynamoDB `Limit` equals `limit`
  (filters such as `minRiders`/`maxRiders` narrow within that scanned page;
  clients paginate with `nextToken` until it is absent).

## 11. Server behavior

- `x-request-id`: honor incoming header, else generate UUID; echo on **all**
  responses including errors.
- CORS: when origin is `*`, credentials are forced off regardless of
  `CORS_CREDENTIALS`.
- Gzip responses ≥ 1024 bytes.
- Security headers: at minimum `x-content-type-options: nosniff` and a
  configured Content-Security-Policy (exact CSP value TBD).
- Rate limit: 100 req/min/IP; **disabled** when `NODE_ENV=test`.
- Structured logging via `slog` at `LOG_LEVEL`.
- OpenAPI UI at `/docs` (exact generator output TBD).
- Graceful shutdown on SIGINT/SIGTERM (drain in-flight, then exit).

## 12. Local run, Docker, CDK

- `docker-compose.yml`: `dynamodb-local` (`amazon/dynamodb-local`, local flags,
  healthcheck) + `app` (Go binary,
  `DYNAMODB_LOCAL_ENDPOINT=http://dynamodb-local:8000`, `/health` healthcheck).
  Exact image tag/flags TBD (see §15).
- Table script (`go run ./scripts/createtable`): creates `SkiLifts` with keys
  and GSI per §7; idempotent on `ResourceInUse` (treat as success).
  Throughput/billing mode values TBD.
- `Dockerfile`: multi-stage build → minimal runtime, non-root user, `EXPOSE`
  the service port, env names matching §5.
- `cdk/` (native Go CDK with `awscdk` + `constructs`, jsii sidecar at synth
  time): table + GSI (`ALL` projection), VPC lookup, ECS cluster + Fargate
  service, log group, task definition, task-role grant, container env (same
  names as §5), container healthcheck on `/health`, ALB with target check on
  `/ready`, CPU/memory autoscaling, CPU-85 and 5xx alarms + SNS, stack outputs
  (table name, service URL — exact output list TBD).

## 13. Tests

`go test ./...`:

- Unit: validation tables (§6), service with fake repository, error mapping
  (§9), token encode/decode incl. malformed (§10), config parsing (§5).
- HTTP integration via `httptest` with a fake DynamoDB behind the repository
  interface: every row of §4 exercised with §8 vectors. Assert status,
  envelope `code`, `details` presence on 400s, and `x-request-id` echo.
- Coverage 80%+ via `go tool cover` gate in CI.

## 14. Non-goals

No auth. No pagination redesign (tokens stay opaque per §10). No seed script.
No shared test-runner harness across languages.

## 15. Open questions (residual — implementation may proceed)

Schema, enum, identity, pagination, and error items from the 2026-10-04
revision are now locked in §§3–4, 6, 9–10. Remaining:

1. CSP value, OpenAPI generator output, docker image tag/flags, table
   throughput/billing mode, CDK VPC/output specifics.
2. `DYNAMODB_MODE` non-local behavior (default `local`; AWS mode resolves
   credentials/region/table via standard SDK chain — exact IAM + endpoint
   rules TBD, does not block local implementation).
3. Fixture field-name migration: §8 vectors now bind to locked names
   (`IsDetachable`, `SnowCount`, `TotalRiders`, `TotalSnow`); no further TBD.

## 16. Acceptance

- All suites green at 80%+ coverage.
- `compose up` + create-table + §8 smoke passes.
- `cdk synth` succeeds.
- Header / token / envelope assertions in §§9–11 hold.
- No §15 item left ambiguous in code: each is either locked in this spec or
  recorded as an ADR under `docs/adr/`.

## 17. Commands (normative)

- Build: `go build ./...`
- Test: `go test ./... --cover` (gate: 80%+ via `go tool cover`)
- Lint: `gofmt -l . && go vet ./...`
- Dev: `go run ./cmd/api` (requires DynamoDB local on `DYNAMODB_LOCAL_ENDPOINT`)
- Table: `go run ./scripts/createtable`
- Synth: `cd cdk && go run . -- synth` (exact entrypoint TBD per §15)

## 18. Code Style

Go 1.24+, `gofmt` clean, package layout per §2. Example envelope writer:

```go
// internal/apperrors/errors.go
func Write(c *gin.Context, code string, status int, message string, details any) {
  c.Header("x-request-id", RequestID(c))
  c.JSON(status, gin.H{"error": gin.H{
    "message": message, "code": code,
    "statusCode": status, "details": details,
  }})
}
```

Conventions: narrow DynamoDB interface in `internal/repository` for fakes;
domain rules in `internal/service` with no Gin or SDK types; validation at
binding time in `internal/schemas`; `slog` structured logging only.

## 19. Boundaries

- Always: run `go test ./...` before commits; validate inputs per §6;
  echo `x-request-id` on all responses; keep error envelope per §9.
- Ask first: DynamoDB schema/key changes; new dependencies; CI config;
  CSP / Docker tag / CDK VPC changes (§15); `DYNAMODB_MODE` non-local behavior.
- Never: commit secrets; edit vendor/jsii output by hand; delete or weaken
  failing tests without approval; leak DynamoDB internals in error messages.
