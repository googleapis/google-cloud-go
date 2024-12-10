// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trace

import (
	ottrace "go.opentelemetry.io/otel/trace"
	ottraceembedded "go.opentelemetry.io/otel/trace/embedded"
)

type TracerProvider interface {
	Tracer(name string, options ...ottrace.TracerOption) ottrace.Tracer
}

// TracerProviderFromOtelTracerProvider converts any [go.opentelemetry.io/otel/trace.TracerProvider]
// into a [cloud.google.com/go/otel/trace.TracerProvider].
func TracerProviderFromOtelTracerProvider(tp ottrace.TracerProvider) TracerProvider {
	return &tracerProviderAdapter{otelTP: tp}
}

type tracerProviderAdapter struct {
	otelTP ottrace.TracerProvider
}

func (a tracerProviderAdapter) Tracer(name string, opts ...ottrace.TracerOption) ottrace.Tracer {
	return a.otelTP.Tracer(name, opts...)
}

// OtelTracerProviderFromTracerProvider converts any [cloud.google.com/go/otel/trace.TracerProvider]
// into a [go.opentelemetry.io/otel/trace.TracerProvider].
func OtelTracerProviderFromTracerProvider(tp TracerProvider) ottrace.TracerProvider {
	return &otelTracerProviderAdapter{tp: tp}
}

type otelTracerProviderAdapter struct {
	tp TracerProvider
	ottraceembedded.TracerProvider
}

func (a otelTracerProviderAdapter) Tracer(name string, opts ...ottrace.TracerOption) ottrace.Tracer {
	return a.tp.Tracer(name, opts...)
}
