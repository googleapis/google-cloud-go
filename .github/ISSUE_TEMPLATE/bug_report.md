---
name: Bug report
about: Create a report to help us improve
title: 'packagename: short description of bug'
labels: triage me
assignees: ''

---

## Client

e.g. PubSub

## Environment

e.g. Alpine Docker on GKE
e.g. $ go version

## Code and Dependencies

```go
package main

func main() {
  // ...
}
```

<details>
  <summary>go.mod</summary>

```text
module modname

go 1.23.0

require (
   // ...
)
```

</details>

## Expected behavior

e.g. Messages arrive really fast.

## Actual behavior

e.g. Messages arrive really slowly.

## Screenshots

e.g. A chart showing how messages are slow. Delete if not necessary.

## Additional context

e.g. Started after upgrading to v0.50.0.
