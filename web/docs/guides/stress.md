# Stress testing

`gtool stress` reuses the component pipeline — mocks and app — and replaces the test
phase with a stepped load ramp. It answers two questions about the service and gates
CI on the answer.

<div class="gt-pipeline">
  <span class="step">Mocks</span><span class="arr">→</span>
  <span class="step">App</span><span class="arr">→</span>
  <span class="step hot">Load ramp</span><span class="arr">→</span>
  <span class="step fail">Breaking point</span><span class="arr">→</span>
  <span class="step">Reports</span><span class="arr">→</span>
  <span class="step end">Cleanup</span>
</div>

- **Degradation point** — the first stage where p95 latency exceeds
  `degradation-latency-factor` × the baseline (first stage) p95, or the absolute
  `degradation-p95-ms` SLO, or the throughput drops below 90% of the requested rate.
- **Breaking point** — the first stage where the error rate (5xx and network
  errors: timeouts, refused connections) exceeds `break-error-rate`. The ramp stops
  there. 4xx responses are reported apart and never count as errors.
- **Max sustained RPS** — the highest requested RPS completed before the breaking
  point. The command fails when it is below `min-acceptable-rps`.

!!! info "Host-relative measurements"
    The load generator, the app and the mocks share the host. Use the numbers to
    compare commits; point `--target-url` at a dedicated environment for serious
    measurements.

## Commands

```sh
gtool stress plan                              # resolved targets, no load
gtool stress                                   # mocks → app → ramp → reports → cleanup
gtool stress --target-url http://staging:8080  # skip mocks and app, hit that URL
```

Without `--target-url` the load goes to `http://localhost:<host port>` (the
published `app-config.host-port`, or `port` when it is not set), after the
pipeline waits for `app-config.health-path`. In both modes the runner then waits up
to `orchestration.startup-timeout` for `/` on the target to answer HTTP. Ctrl-C
cancels the ramp; mocks and app are still torn down.

## Configuration

The block below lists every option with illustrative values; the runnable
[`configs/stress-example.yml`](https://github.com/oswaldom-code/gtool/blob/main/configs/stress-example.yml)
uses a gentler ramp (50 → 500 RPS in steps of 50 every 10 s, p95 SLO 200 ms).

```yaml
stress-config:
  source:
    openapi: api/openapi.yml          # Swagger 2.0 or OpenAPI 3.x
    base-path: /api/v1                # default: servers[0] path (v3) / basePath (v2)
    methods: [GET]                    # default: GET only
    include-tags: [products]
    exclude-tags: []
    include-paths: ["/products/*"]    # path.Match globs on the spec path
    exclude-paths: ["/health"]
    headers:
      Authorization: "Bearer ${STRESS_TOKEN}"
    param-values:
      productId: ["1", "2", "3"]      # rotated across requests
    operation-overrides:              # keyed by operationId
      createOrder: { body-file: test/stress/order.json, weight: 3 }
  targets:                            # explicit targets, path used as-is
    - name: health
      method: GET
      path: /health
      weight: 1
  ramp:
    start-rps: 10
    step-rps: 10
    step-duration: 20s
    max-rps: 500                      # the last stage is always max-rps
  thresholds:
    degradation-latency-factor: 2.0   # default 2.0
    degradation-p95-ms: 800           # 0 disables the absolute SLO
    break-error-rate: 0.05            # default 0.05
  min-acceptable-rps: 100             # 0 disables the gate
  reports-path: test/stress/reports   # default
```

`stress-config` is validated on its own: `gtool stress` does not need the
component-test settings such as `test-launcher`. Validation lists every problem
in the block:

- `source.openapi` or at least one entry in `targets` is required.
- `ramp.start-rps` and `ramp.step-duration` must be positive, `max-rps` must be at
  least `start-rps`, and `step-rps` must be positive when `max-rps` is above
  `start-rps`.
- `thresholds.degradation-latency-factor` must be greater than 1,
  `degradation-p95-ms` not negative and `break-error-rate` between 0 and 1.
- `min-acceptable-rps` must not be negative nor exceed `ramp.max-rps`.
- Targets need a supported method and a path starting with `/`; `body` and
  `body-file` are mutually exclusive, a `body-file` must exist and weights must
  not be negative. `source.base-path` must start with `/`.

## Target resolution

| Item | Precedence |
|---|---|
| Path, query and header params | `param-values` > spec `example` / `default` / `enum[0]` > value generated from the schema |
| Optional query and header params | sent only when `param-values` sets them |
| Body | `body-file` override > spec example > mock generated from the schema |
| Headers | `source.headers` + header params + the request body `Content-Type` |
| Weight | `weight` override, default 1; load is spread proportionally |
| Host | hosts in the spec are ignored; the pipeline app or `--target-url` is used |

Explicit `targets` replace a spec operation with the same method and full path and
are added otherwise. Generated values are seeded, so `plan` shows exactly what a run
sends.

`gtool stress plan` and the reports warn when mutating methods are enabled (writes
fill the mocks, which then become the bottleneck), when more than 20 targets share
the default weight, and when more than 10% of the baseline responses are 4xx — a
sign of badly resolved targets.

## Reports

Both files land in `reports-path`:

- **`stress-report.json`** — stable schema (`schema-version: 1`) for CI: the gtool
  build and the host that produced it, ramp, effective thresholds, targets,
  per-stage metrics (achieved RPS, throughput, latency mean/p50/p95/p99/max, error
  and 4xx rates, status codes), the two points (`null` when not reached),
  `max-sustained-rps`, `passed` and `warnings`.
- **`stress-report.html`** — a self-contained page: latency and error-rate charts by
  requested RPS with both points marked, stage and target tables, and the run
  details.

```sh
gtool stress || exit 1
jq '.["max-sustained-rps"], .passed' test/stress/reports/stress-report.json
```

## Tips

- Keep `methods: [GET]` unless writes are what you want to measure.
- At hundreds of RPS raise the open files limit: `ulimit -n 65535`.
- The repository ships a runnable example:
  [`configs/stress-example.yml`](https://github.com/oswaldom-code/gtool/blob/main/configs/stress-example.yml)
  stresses `examples/postgres-app`.
