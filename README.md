# gin-ddb — SkiLifts API in Go / Gin

HTTP service managing ski-lift records (static lift properties, daily
snapshots, resort aggregates) on DynamoDB, runnable against local
DynamoDB. Behavior-compatible port of the TypeScript `fastify-ddb`
SkiLifts API: same routes, validation, error envelope, and DynamoDB
access patterns.

- **Spec:** `docs/specs/2026-10-04-gin-port-spec.md` (source of truth)
- **Status:** implemented, tested (84.9% coverage), smoke-verified against
  DynamoDB Local; CDK stack template-tested

## Quick Start

Prerequisites: Go 1.26+, Docker with Compose.

```bash
# 1. Start DynamoDB Local + API
docker compose up -d --build

# 2. Create the table (idempotent)
DYNAMODB_LOCAL_ENDPOINT=http://localhost:8000 go run ./scripts/createtable

# 3. Smoke test (§8 vectors)
curl -X POST localhost:3000/api/skilifts/static \
  -H 'Content-Type: application/json' \
  -d '{"Lift":"Summit Express","Metadata":"static","IsDetachable":false,"VerticalFeet":2500,"LiftTime":"8:00"}'
curl localhost:3000/api/skilifts/Summit%20Express/static
curl localhost:3000/health
```

Without Docker: point `DYNAMODB_LOCAL_ENDPOINT` at any reachable DynamoDB
Local and run `go run ./cmd/api`.

## Commands

| Command | Description |
|---------|-------------|
| `go build ./...` | Build all packages |
| `go test ./... --cover` | Full suite (gate: 80%+) |
| `gofmt -l . && go vet ./...` | Format check + vet |
| `go run ./cmd/api` | Run the service locally |
| `go run ./scripts/createtable` | Create table + GSI (idempotent) |
| `cd cdk && go test ./...` | CDK template tests |
| `docker compose up -d --build` | Local stack (app + DynamoDB Local) |

Key env vars (`internal/config`, full list in spec §5): `PORT` (3000),
`DYNAMODB_TABLE_NAME` (SkiLifts), `DYNAMODB_LOCAL_ENDPOINT`
(`http://localhost:8000`), `DYNAMODB_MODE` (`local`), `NODE_ENV=test`
disables rate limiting.

## API

Base: `http://localhost:3000`. Full contract in spec §§3–4, 6, 9–10;
interactive docs at `GET /docs` (Swagger UI).

| Method | Path | Notes |
|---|---|---|
| POST | `/api/skilifts/static\|dynamic\|resort` | Create only → 201, double-create → 409 |
| GET | `/api/skilifts` | Scan list, paginated |
| GET | `/api/skilifts/:lift/by-riders` | Rider-ordered (desc), `minRiders`/`maxRiders` |
| GET | `/api/skilifts/:lift` | Lift collection, paginated |
| GET | `/api/skilifts/:lift/:metadata` | Single item (`static`, `resort`, or `MM/DD/YY`) |
| PUT | `/api/skilifts/:lift/static` | Partial update (existing only) |
| PUT | `/api/skilifts/:lift/:metadata` | Partial update, dynamic or resort |
| DELETE | `/api/skilifts/:lift/:metadata` | → 204, empty body |
| GET | `/health`, `/ready`, `/` | Liveness, DynamoDB readiness, service info |

- Errors: `{error:{message,code,statusCode,details?}}`; `details`
  present on every 400. Date path params must be `%2F`-encoded.
- Pagination: `?limit=` (default 20, cap 100) + opaque `nextToken`;
  responses carry `{items,nextToken?,lastEvaluatedKey?,count}`.

## Architecture

```
internal/config      env parsing + fail-fast validation (§5)
internal/db          client factory, GSI constant, TableName()
internal/models      the three record kinds (§3)
internal/schemas     binding structs + closed-schema validation (§6)
internal/repository  DynamoDB I/O behind a narrow interface + in-memory fake (§7)
internal/service     domain rules only — no HTTP, no SDK types
internal/handlers    Gin handlers incl. health/ready/root (§4)
internal/apperrors   codes, DynamoDB→HTTP mapping, envelope writer (§9)
internal/pagination  opaque base64url cursor encode/decode (§10)
internal/server      engine, middleware (request-id, CORS, gzip, security
                     headers, rate limit, slog), route wiring (§11)
cmd/api              env load, listen, graceful shutdown
scripts/createtable  table + GSI creation CLI (PAY_PER_REQUEST, idempotent)
cdk/                 native Go CDK stack (table, ECS Fargate, ALB, alarms)
```

Decisions with rationale live in `docs/adr/` (billing, CSP scope,
container/CDK defaults, AWS mode).

## Testing

- `go test ./...`: unit (validation tables, error mapping, token
  round-trips, config), service-over-fake, HTTP integration over
  `httptest` with §8 fixture vectors on every §4 row.
- `cdk/`: CloudFormation template assertions (table/GSI, service env,
  ALB `/ready` check, autoscaling, alarms, outputs).
- CI (`.github/workflows/ci.yml`): fmt, vet, build, tests + 80% coverage
  gate for both modules.

## Contributing

Spec first: update `docs/specs/2026-10-04-gin-port-spec.md` (it is a
living document), then plan in `docs/tasks/`, implement with tests
(RED→GREEN), keep `go test ./...` green and coverage ≥ 80%. Record
architectural calls as new ADRs under `docs/adr/` — never rewrite old ones.
