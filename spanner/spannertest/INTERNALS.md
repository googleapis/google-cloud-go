# spannertest internals

This document describes how spannertest is implemented. It should be considered
a companion to the source code.

## Design philosophy

The purpose of spannertest is to support testing code that uses Cloud Spanner
as a client. It aims to be correct and comprehensive (see README.md for
status), but does not attempt to be performant. Clarity and simplicity of
implementation is a very important property; one should be able to read this
document and the code and have a good understanding of how an SQL database may
operate in principle.

## Overview

There are five sections to spannertest:

* RPC interface (`inmem.go`); this implements the same gRPC interface as Cloud
  Spanner. It handles transitions between the gRPC protobuf types and the types
  used by the rest of the package.
* Top-level DB interface (`db.go`); this has the primitive types such as
  `database`, `table` and `row`, implements schema management, all writes
  (insert/update/delete/etc. including DML), and basic reads (based on keys and
  key ranges).
* Query evaluator (`db_query.go`); this evaluates a `spansql.Query` on a
  database.
* Expression evaluator (`db_eval.go`); this evaluates a `spansql.Expr` in a
  specific context (e.g. on a table row).
* Expression functions (`funcs.go`).

## RPC interface (`inmem.go`)

TODO

## Top-level DB interface (`db.go`)

A `database` contains a set of named tables. It also contains a set of named
indexes, though `spannertest` does not do anything with them.

A `table` contains its own schema (a list of `colInfo` values). The primary key
columns appear first in that list for some implementation conveninence;
otherwise the order is as they appear in the `CREATE TABLE` DDL.

A `table` also has its data, stored as a list of `row` values, each of which is
a list of data values. The rows are stored in primary key order; this aids in
search operations.

A `row` is a list of raw data values, represented as `[]interface{}`. `db.go`
explains the mapping between Spanner types and Go types. These values are used
throughout the other parts of the `spannertest` implementation, particularly in
the expression evaluator.

## Query evaluator (`db_query.go`)

The query evaluator works by transforming a `spansql.Query` into a pipeline of
transformers (the `rowIter` interface). This pipeline implements the SQL
semantics. For instance, `SELECT * FROM X WHERE A > 2` would produce a pipeline
that reads rows from X (`tableIter`), filters them (`whereIter`), and outputs
the full set of columns (`selIter`). See `(*database).Query` and
`(*database.evalSelect)`.

## Expression evaluator (`db_eval.go`)

The expression evaluator walks a `spansql.Expr` in a particular "evaluation
context" to produce an output value. See `evalContext.evalExpr` for the main
entry point.

This also includes the value comparator (`compareVals`), which is used for
evaluating expressions, for ordering results (`ORDER BY`), some filtering
(`SELECT DISTINCT`)
