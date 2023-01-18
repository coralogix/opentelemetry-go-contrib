// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otellambda // import "go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"

	headerXForwardedProto = "X-Forwarded-Proto"
	headerHost            = "Host"
	headerUserAgent       = "User-Agent"
)

var errorLogger = log.New(log.Writer(), "OTel Lambda Error: ", 0)

type instrumentor struct {
	configuration config
	resAttrs      []attribute.KeyValue
	tracer        trace.Tracer
}

func newInstrumentor(opts ...Option) instrumentor {
	cfg := config{
		TracerProvider: otel.GetTracerProvider(),
		Flusher:        &noopFlusher{},
		EventToCarrier: emptyEventToCarrier,
		Propagator:     otel.GetTextMapPropagator(),
	}
	for _, opt := range opts {
		opt.apply(&cfg)
	}

	return instrumentor{configuration: cfg,
		tracer:   cfg.TracerProvider.Tracer(tracerName, trace.WithInstrumentationVersion(SemVersion())),
		resAttrs: []attribute.KeyValue{}}
}

// Logic to start OTel Tracing.
func (i *instrumentor) tracingBegin(ctx context.Context, eventJSON []byte, event ...interface{}) (context.Context, trace.Span, trace.Span) {
	// Add trace id to context
	errorLogger.Println(string(eventJSON))
	mc := i.configuration.EventToCarrier(eventJSON)
	ctx = i.configuration.Propagator.Extract(ctx, mc)

	var (
		attributes, sqsAttributes []attribute.KeyValue

		span, sqsSpan trace.Span
		sqsSpanName   string
		spanName      = os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	)

	lc, ok := lambdacontext.FromContext(ctx)
	if !ok {
		errorLogger.Println("failed to load lambda context from context, ensure tracing enabled in Lambda")
	}
	if lc != nil {
		errorLogger.Println(lc)

		ctxRequestID := lc.AwsRequestID
		attributes = append(attributes, semconv.FaaSExecutionKey.String(ctxRequestID))

		// Some resource attrs added as span attrs because lambda
		// resource detectors are created before a lambda
		// invocation and therefore lack lambdacontext.
		// Create these attrs upon first invocation
		if len(i.resAttrs) == 0 {
			ctxFunctionArn := lc.InvokedFunctionArn
			attributes = append(attributes, semconv.FaaSIDKey.String(ctxFunctionArn))
			arnParts := strings.Split(ctxFunctionArn, ":")
			if len(arnParts) >= 5 {
				attributes = append(attributes, semconv.CloudAccountIDKey.String(arnParts[4]))
			}
		}
		attributes = append(attributes, i.resAttrs...)
	}

	// Check if the event has any of the known types and if so,
	// instrument according to spec.
	if event != nil && event[0] != nil {
		switch e := event[0].(type) {
		case events.APIGatewayProxyRequest:
			attributes = append(attributes, semconv.FaaSTriggerHTTP)
			attributes = append(attributes, attributesFromAPIGatewayProxyRequest(e)...)
		case events.SQSEvent:
			attributes = append(attributes, semconv.FaaSTriggerPubsub)
			sqsSpanName, sqsAttributes = getSQSSpanNameAndAttributes(ctx, e.Records)
		default:
		}
	}

	ctx, span = i.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attributes...))

	if sqsSpanName != "" {
		ctx, sqsSpan = i.tracer.Start(ctx, sqsSpanName, trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(sqsAttributes...))
	}

	return ctx, span, sqsSpan
}

// Logic to wrap up OTel Tracing.
func (i *instrumentor) tracingEnd(ctx context.Context, span trace.Span, sqsSpan trace.Span) {
	if sqsSpan != nil {
		sqsSpan.End()
	}

	span.End()

	// force flush any tracing data since lambda may freeze
	err := i.configuration.Flusher.ForceFlush(ctx)
	if err != nil {
		errorLogger.Println("failed to force a flush, lambda may freeze before instrumentation exported: ", err)
	}
}

func attributesFromAPIGatewayProxyRequest(e events.APIGatewayProxyRequest) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}

	if h, ok := e.Headers[headerXForwardedProto]; ok {
		attrs = append(attrs, semconv.HTTPSchemeKey.String(h))
	}
	if h, ok := e.Headers[headerUserAgent]; ok {
		attrs = append(attrs, semconv.HTTPUserAgentKey.String(h))
	}
	if h, ok := e.Headers[headerHost]; ok {
		attrs = append(attrs, semconv.HTTPHostKey.String(h))
	}
	if e.HTTPMethod != "" {
		attrs = append(attrs, semconv.HTTPMethodKey.String(e.HTTPMethod))
	}
	if e.Resource != "" {
		attrs = append(attrs, semconv.HTTPRouteKey.String(e.Resource))
	}
	if e.Path != "" {
		attrs = append(attrs, semconv.HTTPTargetKey.String(e.Path))
	}

	return attrs
}

func getSQSSpanNameAndAttributes(ctx context.Context, m []events.SQSMessage) (string, []attribute.KeyValue) {
	sqsAttributes := []attribute.KeyValue{
		semconv.FaaSTriggerPubsub,
		semconv.MessagingSystemKey.String("AmazonSQS"),
		semconv.MessagingOperationProcess,
	}

	var source, sqsSpanName string
	for _, r := range m {
		if source != "" {
			if source != r.EventSource {
				sqsSpanName = "multiple_sources process"
				break
			}

			continue
		}

		sqsSpanName = r.EventSource + " process"
		source = r.EventSource
	}

	return sqsSpanName, sqsAttributes
}
