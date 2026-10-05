# ADR 001: PAY_PER_REQUEST billing for the SkiLifts table

Date: 2026-10-05 · Status: accepted

## Context
Spec §15 leaves table throughput/billing mode TBD. `scripts/createtable`
must choose something for local/dev.

## Decision
`PAY_PER_REQUEST` (on-demand) in the table script and the CDK stack.

## Consequences
- No capacity planning for local/dev; works with DynamoDB Local.
- Production cost at scale is less predictable than provisioned + autoscaling.
- If traffic justifies provisioned capacity, change both the script default
  and the CDK stack together and record a superseding ADR.
