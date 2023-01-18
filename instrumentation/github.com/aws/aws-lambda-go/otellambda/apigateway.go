package otellambda

import "go.opentelemetry.io/otel/propagation"

func apiGatewayEventToCarrier([]byte) propagation.TextMapCarrier {
	return propagation.HeaderCarrier{"X-Amzn-Trace-Id": []string{"xrayTraceID"}}
}
