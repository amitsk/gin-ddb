# Glossary — gin-ddb

## Lift

Partition-key value. Names one ski lift. Non-empty string.

## Metadata

Sort-key value. Discriminates the record kind: literal `static`, literal
`resort`, or a date `MM/DD/YY` for a daily record.

## Static record

Fixed properties of one lift (`Metadata = "static"`).

## Dynamic record

One day snapshot for one lift (`Metadata` is a date).

## Resort record

Aggregate record (`Metadata = "resort"`).

## TotalUniqueLiftRiders

Dynamic rider count. Sort key of the `SkiLiftsByRiders` GSI.
