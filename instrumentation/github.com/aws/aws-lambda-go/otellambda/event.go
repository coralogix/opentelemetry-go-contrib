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
	"github.com/aws/aws-lambda-go/events"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	headerXForwardedProto = "X-Forwarded-Proto"
	headerHost            = "Host"
	headerUserAgent       = "User-Agent"

	httpRequestBodyAttributeKey  = "http.request.body"
	httpResponseBodyAttributeKey = "http.response.body"
)

func spanNameAndAttributesFromEvent(event []interface{}, lambdaAttr []attribute.KeyValue) ([]attribute.KeyValue, trace.SpanKind, string, []attribute.KeyValue) {
	var (
		eventSpanName   string
		eventAttributes []attribute.KeyValue
		eventSpanKind   trace.SpanKind
	)

	if event != nil && event[0] != nil {
		switch e := event[0].(type) {
		case events.APIGatewayProxyRequest:
			lambdaAttr = append(lambdaAttr, semconv.FaaSTriggerHTTP)
			eventSpanKind = trace.SpanKindServer
			eventSpanName, eventAttributes = apiGatewayProxyRequestSpanNameAndAttributes(e)
		case events.SQSEvent:
			lambdaAttr = append(lambdaAttr, semconv.FaaSTriggerPubsub)
			eventSpanKind = trace.SpanKindConsumer
			eventSpanName, eventAttributes = sqsEventSpanNameAndAttributes(e.Records)
		case events.S3Event:
			lambdaAttr = append(lambdaAttr, semconv.FaaSTriggerDatasource)
			eventSpanKind = trace.SpanKindConsumer
			eventSpanName, eventAttributes = s3EventSpanNameAndAttributes(e.Records)
		case events.DynamoDBEvent:
			lambdaAttr = append(lambdaAttr, semconv.FaaSTriggerDatasource)
			eventSpanKind = trace.SpanKindConsumer
			eventSpanName, eventAttributes = dynamoDBEventSpanNameAndAttributes(e.Records)
		case events.CognitoEvent:
			lambdaAttr = append(lambdaAttr, semconv.FaaSTriggerDatasource)
			eventSpanKind = trace.SpanKindConsumer
			eventSpanName, eventAttributes = cognitoEventSpanNameAndAttributes(e)
		case events.SNSEvent:
			lambdaAttr = append(lambdaAttr, semconv.FaaSTriggerPubsub)
			eventSpanKind = trace.SpanKindConsumer
			eventSpanName, eventAttributes = snsEventSpanNameAndAttributes(e.Records)
		case events.SimpleEmailEvent:
			lambdaAttr = append(lambdaAttr, semconv.FaaSTriggerPubsub)
			eventSpanKind = trace.SpanKindConsumer
			eventSpanName, eventAttributes = simpleEmailEventSpanNameAndAttributes(e.Records)
		default:
		}
	}

	return lambdaAttr, eventSpanKind, eventSpanName, eventAttributes
}

func apiGatewayProxyRequestSpanNameAndAttributes(e events.APIGatewayProxyRequest) (string, []attribute.KeyValue) {
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
	if e.Body != "" {
		attrs = append(attrs, attribute.String(httpRequestBodyAttributeKey, e.Body))
	}

	return e.Resource, attrs
}

func sqsEventSpanNameAndAttributes(m []events.SQSMessage) (string, []attribute.KeyValue) {
	eventAttributes := []attribute.KeyValue{
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

	return sqsSpanName, eventAttributes
}

func simpleEmailEventSpanNameAndAttributes(m []events.SimpleEmailRecord) (string, []attribute.KeyValue) {
	eventAttributes := []attribute.KeyValue{
		semconv.FaaSTriggerPubsub,
		semconv.MessagingSystemKey.String("AmazonSES"),
		semconv.MessagingOperationProcess,
	}

	var source, sesSpanName string
	for _, r := range m {
		if source != "" {
			if source != r.EventSource {
				sesSpanName = "multiple_sources process"
				break
			}

			continue
		}

		sesSpanName = r.EventSource + " process"
		source = r.EventSource
	}

	return sesSpanName, eventAttributes
}

func snsEventSpanNameAndAttributes(m []events.SNSEventRecord) (string, []attribute.KeyValue) {
	eventAttributes := []attribute.KeyValue{
		semconv.FaaSTriggerPubsub,
		semconv.MessagingSystemKey.String("AmazonSNS"),
		semconv.MessagingOperationProcess,
	}

	var source, snsSpanName string
	for _, r := range m {
		if source != "" {
			if source != r.EventSource {
				snsSpanName = "multiple_sources process"
				break
			}

			continue
		}

		snsSpanName = r.EventSource + " process"
		source = r.EventSource
	}

	return snsSpanName, eventAttributes
}

func s3EventSpanNameAndAttributes(m []events.S3EventRecord) (string, []attribute.KeyValue) {
	eventAttributes := []attribute.KeyValue{
		semconv.FaaSTriggerDatasource,
	}

	var source, s3SpanName string
	for _, r := range m {
		if source != "" {
			if source != r.EventSource {
				s3SpanName = "multiple_sources process"
				break
			}

			continue
		}

		s3SpanName = r.EventSource + " process"
		source = r.EventSource
	}

	return s3SpanName, eventAttributes
}

func dynamoDBEventSpanNameAndAttributes(m []events.DynamoDBEventRecord) (string, []attribute.KeyValue) {
	eventAttributes := []attribute.KeyValue{
		semconv.FaaSTriggerDatasource,
		semconv.DBSystemDynamoDB,
	}

	var source, dynamoDBSpanName, operation string
	for _, r := range m {
		if operation != "" {
			if operation != r.EventName {
				dynamoDBSpanName = "multiple_operations"
				break
			}

			continue
		}

		if source != "" {
			if source != r.EventSource {
				dynamoDBSpanName = "multiple_sources process"
				break
			}

			continue
		}

		dynamoDBSpanName = r.EventSource + " process"
		source = r.EventSource
		// For DynamoDB, the operation (INSERT, MODIFY or REMOVE) is the event name.
		operation = r.EventName
	}

	eventAttributes = append(eventAttributes, semconv.DBOperationKey.String(operation))

	return dynamoDBSpanName, eventAttributes
}

func cognitoEventSpanNameAndAttributes(m events.CognitoEvent) (string, []attribute.KeyValue) {
	eventAttributes := []attribute.KeyValue{
		semconv.FaaSTriggerDatasource,
	}

	return m.EventType + " process", eventAttributes
}
