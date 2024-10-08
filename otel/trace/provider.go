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
	"go.opentelemetry.io/otel/trace"
)

type TracerProvider interface {
	Tracer(name string, options ...trace.TracerOption) trace.Tracer
}

type otelTracerProviderAdapter struct {
	otelTP trace.TracerProvider
}

func (a *otelTracerProviderAdapter) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return a.otelTP.Tracer(name, opts...)
}
