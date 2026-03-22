package metrics

import (
	"context"
	"net/http"

	prometheusexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// agent stats — set on each heartbeat
	agentAllowed  metric.Int64Gauge
	agentBlocked  metric.Int64Gauge
	agentReqCount metric.Int64Gauge
	agentAvgMs    metric.Int64Gauge
	agentMinMs    metric.Int64Gauge
	agentMaxMs    metric.Int64Gauge

	// agent health — 1=healthy, 0=stale/killed
	agentHealth metric.Int64Gauge

	// log entries — incremented per ingested log
	logEntriesTotal metric.Int64Counter
)

// Init sets up the OTEL Prometheus exporter and initialises all instruments.
// Must be called once at startup before serving requests.
func Init() error {
	exporter, err := prometheusexporter.New()
	if err != nil {
		return err
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter("control-plane")

	agentAllowed, err = meter.Int64Gauge("agent_requests_allowed",
		metric.WithDescription("Cumulative allowed requests per agent (from heartbeat)"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	agentBlocked, err = meter.Int64Gauge("agent_requests_blocked",
		metric.WithDescription("Cumulative blocked requests per agent (from heartbeat)"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	agentReqCount, err = meter.Int64Gauge("agent_requests_total",
		metric.WithDescription("Cumulative total requests per agent (from heartbeat)"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	agentAvgMs, err = meter.Int64Gauge("agent_latency_avg_ms",
		metric.WithDescription("Average request latency per agent in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return err
	}

	agentMinMs, err = meter.Int64Gauge("agent_latency_min_ms",
		metric.WithDescription("Minimum request latency per agent in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return err
	}

	agentMaxMs, err = meter.Int64Gauge("agent_latency_max_ms",
		metric.WithDescription("Maximum request latency per agent in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return err
	}

	agentHealth, err = meter.Int64Gauge("agent_health",
		metric.WithDescription("Agent health status: 1=healthy, 0=stale or killed"),
	)
	if err != nil {
		return err
	}

	logEntriesTotal, err = meter.Int64Counter("agent_log_entries_total",
		metric.WithDescription("Total request log entries ingested per registration and action"),
		metric.WithUnit("{entry}"),
	)
	if err != nil {
		return err
	}

	return nil
}

// Handler returns the Prometheus /prom/metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// RecordHeartbeat updates per-agent gauges from a heartbeat payload.
func RecordHeartbeat(agentID, registration string, allowed, blocked, reqCount, avgMs, minMs, maxMs int64) {
	if agentAllowed == nil {
		return
	}
	attrs := agentAttrs(agentID, registration)
	ctx := context.Background()
	agentAllowed.Record(ctx, allowed, attrs)
	agentBlocked.Record(ctx, blocked, attrs)
	agentReqCount.Record(ctx, reqCount, attrs)
	agentAvgMs.Record(ctx, avgMs, attrs)
	agentMinMs.Record(ctx, minMs, attrs)
	agentMaxMs.Record(ctx, maxMs, attrs)
}

// RecordHealth sets the health gauge for an agent: 1=healthy, 0=stale/killed.
func RecordHealth(agentID, registration, status string) {
	if agentHealth == nil {
		return
	}
	var v int64
	if status == "healthy" {
		v = 1
	}
	agentHealth.Record(context.Background(), v, agentAttrs(agentID, registration))
}

// RecordLogBatch increments the log entries counter for each ingested entry.
func RecordLogBatch(registration, action string, count int64) {
	if logEntriesTotal == nil {
		return
	}
	logEntriesTotal.Add(context.Background(), count,
		metric.WithAttributes(
			attrRegistration(registration),
			attrAction(action),
		),
	)
}
