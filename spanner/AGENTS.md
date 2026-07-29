# Spanner agent instructions

- gRPC built-in metric attributes pass through two allowlist chokepoints backed by
  `allowedMetricLabels`: SDK views in `metrics.go` and Cloud Monitoring conversion
  in `metrics_monitoring_exporter.go`. Update tests for both paths when changing
  labels.

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.
