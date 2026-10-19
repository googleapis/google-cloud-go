// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pscompat

// MessagingBackend specifies the messaging backend to use for Publisher and
// Subscriber clients.
type MessagingBackend int

const (
	// PubSubLite uses Google Cloud Pub/Sub Lite as the backend (default).
	// This is the traditional backend with zonal storage and predictable pr
	PubSubLite MessagingBackend = iota

	// ManagedKafka uses Google Cloud Managed Service for Apache Kafka as th
	// backend. Provides Kafka-compatible API with Google Cloud management.
	ManagedKafka
)
