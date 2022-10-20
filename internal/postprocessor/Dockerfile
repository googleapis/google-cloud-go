# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.19 as builder

# Copy local code to the container image.
RUN mkdir /postprocessor
WORKDIR /postprocessor
COPY . .

# Build the handle_task command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep"go .)
# go build with no path looks in current directory (/postprocessor) and finds package main
RUN CGO_ENABLED=0 GOOS=linux go build -v -o post_processor


# Use a Docker multi-stage build to create a lean production image.
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds

FROM golang:1.19

# Install dependencies

# Install tools used in build
RUN go install honnef.co/go/tools/cmd/staticcheck@latest && \
    go install github.com/jstemmer/go-junit-report@latest && \
    go install golang.org/x/lint/golint@latest && \
    go install golang.org/x/tools/cmd/goimports@latest

# Copy the binary to the production image from the builder stage.
# post_processor is like an app
COPY --from=builder /postprocessor/post_processor /post_processor

CMD [ "/post_processor"]
