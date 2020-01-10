# Changes

## v1.1.0

- Limit default grpc connections to 4.
- Fix issues with OpenCensus metric for pull count not including synchronous pull messages.
- Fix issue with publish bundle size calculations.
- Add ClearMessages method to pstest server.

## v1.0.1

Small fix to a package name.

## v1.0.0

This is the first tag to carve out pubsub as its own module. See:
https://github.com/golang/go/wiki/Modules#is-it-possible-to-add-a-module-to-a-multi-module-repository.
