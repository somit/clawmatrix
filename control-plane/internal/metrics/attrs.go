package metrics

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func agentAttrs(agentID, registration string) metric.MeasurementOption {
	return metric.WithAttributes(
		attribute.String("agent", agentID),
		attribute.String("registration", registration),
	)
}

func attrRegistration(v string) attribute.KeyValue {
	return attribute.String("registration", v)
}

func attrAction(v string) attribute.KeyValue {
	return attribute.String("action", v)
}
