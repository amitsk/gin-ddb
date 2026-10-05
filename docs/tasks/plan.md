# Implementation Plan: gin-ddb SkiLifts API

## Overview
Build the Go/Gin SkiLifts CRUD service per `docs/specs/2026-10-04-gin-port-spec.md`: DynamoDB-backed static/dynamic/resort records, 10 CRUD routes plus health/ready/root/docs, strict validation with closed schemas, unified error envelope, opaque pagination, and local-run/CDK/CI support. Greenfield — no Go code exists yet.

## Architecture Decisions
- Layered layout per spec §2: `config → db → models/schemas → repository → service → handlers/server → cmd/api`. Repository depends on a narrow DynamoDB interface (Put/Get/Query/Scan/Update/Delete) so tests inject fakes; service holds domain rules with no Gin or SDK types.
- PUT stays partial-update (documented §4.1 deviation, no PATCH) to match DynamoDB `UpdateItem` parity.
- `nextToken` is the only cursor; legacy `lastEvaluated*` query params are rejected with 400. Response still echoes deprecated `lastEvaluatedKey`.
- `by-riders` is per-`Lift` GSI query, descending, not a global leaderboard.
- AvalancheDanger locked to `Low|Moderate|Considerable|High|Extreme`; schemas closed (`additionalProperties:false`).
- Rate limiting: 100 req/min/IP with 429 `RATE_LIMITED`; disabled when `NODE_ENV=test`.
- Dependencies float to latest at build time: the implementer runs
  `go get <mod>@latest` for each module below, then pins the resolved
  version in `go.mod`/`cdk/go.mod`. Floor versions below were verified
  against proxy.golang.org on 2026-10-05 — never pin below them.

## Pinned dependencies (floors, verified 2026-10-05)
| Module | Floor | Notes |
|---|---|---|
| `github.com/gin-gonic/gin` | `v1.12.0` | HTTP framework |
| `github.com/go-playground/validator/v10` | `v10.30.5` | only where regexes don't rule |
| `github.com/aws/aws-sdk-go-v2/config` | `v1.33.6` | default config/region chain |
| `github.com/aws/aws-sdk-go-v2/credentials` | `v1.20.6` | static dummy creds (local) |
| `github.com/aws/aws-sdk-go-v2/service/dynamodb` | `v1.70.0` | table I/O |
| `github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue` | `v1.21.8` | integer-preserving marshal |
| `github.com/swaggo/swag` | `v1.16.6` | OpenAPI generator CLI |
| `github.com/swaggo/gin-swagger` | `v1.6.1` | `/docs` handler |
| `github.com/swaggo/files` | `v1.0.1` | Swagger UI assets |
| `github.com/google/uuid` | `v1.6.0` | `x-request-id` generation |
| `github.com/aws/aws-cdk-go/awscdk/v2` (`cdk/` module) | `v2.272.0` | stack (§12) |
| `github.com/aws/constructs-go/constructs/v10` (`cdk/` module) | `v10.8.1` | constructs |
| Go toolchain | `1.24+` per spec (local `go1.27.1`) | `go` directive ≥ 1.24 |

## Dependency Graph
```
config (§5) ──┐
apperrors (§9) + pagination (§10) ──┐ (independent, testable first)
models/schemas (§3/§6) ──→ repository (§7, narrow iface + fakes)
                                  ──→ service (domain rules, 404/409)
                                  ──→ handlers/server (§4/§11 + middleware)
                                  ──→ cmd/api (wiring, shutdown)
scripts/createtable + Dockerfile/compose (§12) ── parallel after repository
cdk/ (§12) ── parallel after table shape locked
HTTP integration (§8 vectors) ── after handlers
coverage/lint/smoke (§§13,16,17) ── final gate
```

## Task List
### Phase 1: Foundation
- [ ] Task 0: Fix spec refs and stage file move
- [ ] Task 1: Config + module scaffold (§5)
- [ ] Task 2: apperrors + pagination (§§9–10)

### Checkpoint: Foundation
- [ ] `go build ./...` succeeds, unit tests for config/errors/token pass

### Phase 2: Data path
- [ ] Task 3: Models + schemas validation (§§3,6)
- [ ] Task 4: Repository with fake (§7)
- [ ] Task 5: Service domain rules (§§7,9)

### Checkpoint: Data path
- [ ] Service tests with fake repo green (create/409, update-delete/404, by-riders branches)

### Phase 3: HTTP + ops
- [ ] Task 6: Handlers + server + middleware (§§4,11)
- [ ] Task 7: HTTP integration with §8 vectors (§§4,8–10)
- [ ] Task 8: cmd/api + table script + Docker/compose (§§5,12)

### Checkpoint: HTTP + ops
- [ ] All §4 rows exercised, `x-request-id` echoed, compose + create-table + smoke pass

### Phase 4: Release gates
- [ ] Task 9: CDK Go stack (§12)
- [ ] Task 10: Coverage 80%+ + lint + full acceptance (§§13,16,17)

### Checkpoint: Complete
- [ ] `go test ./...`, 80%+ cover gate, `cdk synth`, §§9–11 assertions hold

Tasks tracked in `docs/tasks/todo.md`.

## Risks and Mitigations
| Risk | Impact | Mitigation |
|------|--------|------------|
| Gin route capture (`:metadata` swallowing `static`/`by-riders`) | Med | Register static segments first per §4; integration test each route |
| GSI test fakes diverging from DynamoDB sort/filter semantics | High | Fake implements BETWEEN/>=/<= branches + desc order; vectors pin ordering |
| Token encode/decode mismatch (key sort, padding) | Med | Table test with sorted-keys vector + malformed cases; decoded-equivalence test |
| Number marshaling (`2500.0` vs `2500`) | Med | Use `attributevalue` paths only; assert raw JSON in integration tests |
| Rate-limit middleware breaking `NODE_ENV=test` runs | Low | Test-mode bypass covered by dedicated test |
| CDK jsii sidecar friction | Med | Isolate in `cdk/`; synth verified independently, never blocks API tasks |

## Open Questions
- Residual §15 ops only (CSP value, OpenAPI generator output, image tag/flags, billing mode, VPC/outputs, non-local `DYNAMODB_MODE`) — each lands as ADR under `docs/adr/` if not locked in spec.
- `GET /` shape locked in §4.1; confirm version string `1.0.0` stays hardcoded vs build-injected.
