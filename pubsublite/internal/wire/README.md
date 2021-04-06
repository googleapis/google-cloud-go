# Wire

This directory contains internal implementation details for Cloud Pub/Sub Lite.
Its exported interface can change at any time.

## Conventions

The following are general conventions used in this package:

* Capitalized methods and fields of a struct denotes its public interface. They
  are safe to call from outside the struct (e.g. accesses immutable fields or
  guarded by a mutex). All other methods are considered internal implementation
  details that should not be called from outside the struct.
* unsafeFoo() methods indicate that the caller is expected to have already
  acquired the struct's mutex. Since Go does not support re-entrant locks, they
  do not acquire the mutex. These are typically common util methods that need
  to be atomic with other operations.
