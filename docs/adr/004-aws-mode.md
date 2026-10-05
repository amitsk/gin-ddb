# ADR 004: DYNAMODB_MODE=aws behavior

Date: 2026-10-05 · Status: accepted (interim — IAM specifics still open)

## Context
Spec §5/§15: `DYNAMODB_MODE` is `local` by default; AWS-mode behavior TBD.

## Decision
- `aws` mode uses the standard SDK chain: region from `AWS_REGION`,
  credentials from the environment/shared config unless both
  `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` are set (static creds).
- No local endpoint override in `aws` mode.
- IAM policy contents, endpoint/VPC specifics: still TBD, not blocking local.

## Consequences
- Local and AWS paths share one client factory; behavior differs only by
  the documented inputs above.
