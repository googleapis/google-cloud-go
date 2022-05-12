// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

// MaxSendRecvBytes is the maximum message size that can be sent and received
// over gRPC. Set to the same value as `pubsub`, allowing for at least one
// 10 MiB message to be sent and received in a batch.
const MaxSendRecvBytes = 20 * 1024 * 1024 // 20 MiB
