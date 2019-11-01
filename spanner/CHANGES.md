# Changes

## v1.1.0

- The String() method of NullString, NullTime and NullDate will now return
  an unquoted string instead of a quoted string. This is a BREAKING CHANGE.
  If you relied on the old behavior, please use fmt.Sprintf("%q", T).
- The Spanner client will now use the new BatchCreateSessions RPC to initialize
  the session pool. This will improve the startup time of clients that are
  initialized with a minimum number of sessions greater than zero
  (i.e. SessionPoolConfig.MinOpened>0).
- Spanner clients that are created with the NewClient method will now default
  to a minimum of 100 opened sessions in the pool
  (i.e. SessionPoolConfig.MinOpened=100). This will improve the performance
  of the first transaction/query that is executed by an application, as a
  session will normally not have to be created as part of the transaction.
  Spanner clients that are created with the NewClientWithConfig method are
  not affected by this change.
- The session pool maintenance worker has been improved so it keeps better
  track of the actual number of sessions needed. It will now less often delete
  and re-create sessions. This can improve the overall performance of
  applications with a low transaction rate.

## v1.0.0

This is the first tag to carve out spanner as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
