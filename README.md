# omni-client-go

[![CI](https://github.com/pod32g/omni-client-go/actions/workflows/test.yml/badge.svg)](https://github.com/pod32g/omni-client-go/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/pod32g/omni-client-go.svg)](https://pkg.go.dev/github.com/pod32g/omni-client-go)

A small, dependency-free Go client for the [omni-metrics](https://github.com/pod32g/omni-metrics)
HTTP API — a Prometheus-shaped metrics server. It wraps the `/api/v1` query and
metadata endpoints with typed results and errors.

> This client *reads from* a running server (runs queries, lists targets/labels)
> **and** can *write to* it via push (`Push`) — for a process that has no HTTP
> server to be scraped.

## Install

```sh
go get github.com/pod32g/omni-client-go
```

Requires Go 1.22+. The only dependency is the standard library.

## Usage

```go
package main

import (
	"context"
	"fmt"
	"time"

	omni "github.com/pod32g/omni-client-go"
)

func main() {
	c, err := omni.New("http://localhost:9090")
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	// Instant query (zero time => "now").
	res, err := c.Query(ctx, `rate(omni_http_requests_total[1m])`, time.Time{})
	if err != nil {
		panic(err)
	}
	for _, s := range res.Vector {
		fmt.Printf("%v => %g\n", s.Metric, s.Value)
	}

	// Range query.
	end := time.Now()
	m, _ := c.QueryRange(ctx, `up`, end.Add(-time.Hour), end, 15*time.Second)
	for _, series := range m.Matrix {
		fmt.Printf("%v has %d points\n", series.Metric, len(series.Points))
	}

	// Scrape target health.
	targets, _ := c.Targets(ctx)
	for _, t := range targets {
		fmt.Printf("%s/%s up=%v\n", t.Job, t.Instance, t.Up)
	}
}
```

## API

| Method | Endpoint | Returns |
| --- | --- | --- |
| `Query(ctx, expr, ts)` | `/api/v1/query` | `*QueryResult` (vector / scalar) |
| `QueryRange(ctx, expr, start, end, step)` | `/api/v1/query_range` | `*QueryResult` (matrix) |
| `Series(ctx, matches, start, end)` | `/api/v1/series` | `[]Labels` |
| `LabelNames(ctx)` | `/api/v1/labels` | `[]string` |
| `LabelValues(ctx, name)` | `/api/v1/label/{name}/values` | `[]string` |
| `Targets(ctx)` | `/api/v1/targets` | `[]Target` |
| `Push(ctx, *PushRequest)` | `/api/v1/push` | `*PushResult` |

`QueryResult.Type` is one of `ResultVector`, `ResultMatrix`, or `ResultScalar`;
the matching field (`Vector`, `Matrix`, or `Scalar`) is populated. Sample values
are `float64` (including `+Inf`/`-Inf`/`NaN`); timestamps are `time.Time`.

### Writing (push)

A process with no HTTP server to scrape can push samples instead. Each push
**appends** (building a time series, so `rate()` works on pushed counters). Build
the payload directly, or from a small in-process `Registry`:

```go
c, _ := omni.New("http://localhost:9090", omni.WithPushAuth("token")) // token optional

reg := omni.NewRegistry()
reg.Add("records_processed_total", nil, 1500)               // counter
reg.Set("queue_depth", omni.Labels{"queue": "high"}, 12)    // gauge

res, err := c.Push(ctx, &omni.PushRequest{
	Job:      "batch-importer",
	Instance: "worker-7", // optional; the server defaults it to your remote host
	Series:   reg.Series(),
})
```

Per series, set exactly one of `Value` (a sample at receive time) or `Samples`
(explicit `(time, value)` pairs). The server injects `job`/`instance`; a client
cannot override `__name__`/`job`/`instance`.

### Options

```go
c, _ := omni.New("http://localhost:9090",
	omni.WithTimeout(5*time.Second),
	omni.WithHTTPClient(myHTTPClient), // custom transport / TLS / proxy
	omni.WithPushAuth("token"),        // bearer token for Push writes
)
```

### Error handling

A server-side error (or non-2xx) is returned as a typed `*APIError`:

```go
res, err := c.Query(ctx, "bad{", time.Time{})
var apiErr *omni.APIError
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.Type, apiErr.Message) // e.g. "bad_data", "unexpected token ..."
}
```

## License

MIT — see [LICENSE](LICENSE).
