This directory contains `spannertest`, an in-memory fake Spanner. A sibling
directory, `spansql`, contains types and parser for the Spanner SQL dialect.

`spansql` is reusable for anything that interacts with Spanner on a
syntactic basis, such as tools for handling Spanner schema (DDL).

`spannertest` builds on `spansql` for testing code that uses Spanner client
libraries.

Neither of these packages aims to be performant nor exact replicas of the
production Spanner. They are reasonable for building tools, or writing
unit or integration tests. Full-scale performance testing, end-to-end testing or serious workloads
should use the production Spanner instance or the [Spanner Emulator](https://cloud.google.com/spanner/docs/emulator).


## Maintenance Status

This package is currently in limited maintenance mode. While it is still available for use, it is not actively maintained by the core team. We welcome external contributions and will assist with code reviews for pull requests that improve the package.

## Contributing

We welcome contributions from the community! If you'd like to contribute:
1. Fork the repository
2. Create your feature branch
3. Submit a pull request

Our team will review your contributions and provide feedback to help improve the package.

See [INTERNALS.md](INTERNALS.md) for an explanation of the implementation.

Here's a list of features that are missing or incomplete. It is roughly ordered
by ascending esotericism:

- expression functions
- NUMERIC
- JSON
- more aggregation functions
- SELECT HAVING
- more literal types
- DEFAULT
- expressions that return null for generated columns
- generated columns referencing other generated columns
- checking dependencies on a generated column before deleting a column
- expression type casting, coercion
- multiple joins
- subselects
- case insensitivity of table and column names and query aliases
- transaction simulation
- FOREIGN KEY and CHECK constraints
- set operations (UNION, INTERSECT, EXCEPT)
- STRUCT types
- partition support
- conditional expressions
- table sampling (implementation)
