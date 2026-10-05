# ADR 003: Container and CDK defaults

Date: 2026-10-05 · Status: accepted (dev defaults — revisit for production)

## Context
Spec §15 leaves docker image tag/flags, CDK VPC/output specifics, and the
`GET /` version scheme open. Choices were needed to ship Task 8/9.

## Decision
- Build image `golang:1.27-bookworm` (go.mod requires go ≥ 1.26 after
  dependency resolution); runtime `alpine:3.21` non-root for a working
  `/health` healthcheck (busybox wget; distroless has no shell).
- DynamoDB Local runs in-memory (`-sharedDb`, no `-dbPath`): file-backed
  mode stalled on SQLite open errors in image 3.3.0. No persistence across
  `compose down`; tables recreated via the script.
- CDK: default-VPC lookup, `RemovalPolicy.DESTROY` (dev stack), container
  image placeholder `gin-ddb:latest` (wire ECR before production),
  outputs `TableName` + `ServiceURL`.
- `GET /` version is hardcoded `"1.0.0"` (no build injection yet).

## Consequences
- `DESTROY` + `:latest` + hardcoded version are unsafe for production;
  each needs a follow-up before any real deploy.
