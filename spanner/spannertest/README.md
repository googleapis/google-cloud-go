This directory contains `spannertest`, an in-memory fake Cloud Spanner. A sibling
directory, `spansql`, contains types and parser for the Cloud Spanner SQL dialect.

`spansql` is reusable for anything that interacts with Cloud Spanner on a
syntactic basis, such as tools for handling Spanner schema (DDL).

`spannertest` builds on `spansql` for testing code that uses Cloud Spanner client
libraries.

Neither of these packages aims to be performant nor exact replicas of the
production Cloud Spanner. They are reasonable for building tools, or writing
unit or integration tests. Full-scale performance testing or serious workloads
should use the production Cloud Spanner instead.

See [INTERNALS.md](INTERNALS.md) for an explanation of the implementation.

Here's a list of features that are missing or incomplete. It is roughly ordered
by ascending esotericism:

- expression functions
- NUMERIC
- more aggregation functions
- SELECT HAVING
- more literal types
- generated columns referencing other generated columns as well as proper handling of dropping columns generator expressions refer to
- expression type casting, coercion
- multiple joins
- subselects
- case insensitivity of table and column names and query aliases
- transaction simulation
- FOREIGN KEY and CHECK constraints
- INSERT DML statements
- set operations (UNION, INTERSECT, EXCEPT)
- STRUCT types
- partition support
- conditional expressions
- table sampling (implementation)
