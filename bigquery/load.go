// Copyright 2016 Google Inc. All Rights Reserved.
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

package bigquery

import (
	"io"

	"golang.org/x/net/context"
	bq "google.golang.org/api/bigquery/v2"
)

// LoadConfig holds the configuration for a load job.
type LoadConfig struct {
	// Src is the source from which data will be loaded.
	Src LoadSource

	// Dst is the table into which the data will be loaded.
	Dst *Table

	// CreateDisposition specifies the circumstances under which the destination table will be created.
	// The default is CreateIfNeeded.
	CreateDisposition TableCreateDisposition

	// WriteDisposition specifies how existing data in the destination table is treated.
	// The default is WriteAppend.
	WriteDisposition TableWriteDisposition
}

// A Loader loads data from Google Cloud Storage into a BigQuery table.
type Loader struct {
	JobIDConfig
	LoadConfig
	c *Client
}

// A LoadSource represents a source of data that can be loaded into
// a BigQuery table.
//
// This package defines two LoadSources: GCSReference, for Google Cloud Storage
// objects, and ReaderSource, for data read from an io.Reader.
type LoadSource interface {
	// populates job, returns media
	populateJobForLoad(job *bq.Job) io.Reader
}

// LoaderFrom returns a Loader which can be used to load data into a BigQuery table.
// The returned Loader may optionally be further configured before its Run method is called.
// See GCSReference and ReaderSource for additional configuration options that
// affect loading.
func (t *Table) LoaderFrom(src LoadSource) *Loader {
	return &Loader{
		c: t.c,
		LoadConfig: LoadConfig{
			Src: src,
			Dst: t,
		},
	}
}

// Run initiates a load job.
func (l *Loader) Run(ctx context.Context) (*Job, error) {
	job, media := l.newJob()
	return l.c.insertJob(ctx, job, media)
}

func (l *Loader) newJob() (*bq.Job, io.Reader) {
	job := &bq.Job{
		JobReference: l.JobIDConfig.createJobRef(l.c.projectID),
		Configuration: &bq.JobConfiguration{
			Load: &bq.JobConfigurationLoad{
				CreateDisposition: string(l.CreateDisposition),
				WriteDisposition:  string(l.WriteDisposition),
			},
		},
	}
	media := l.Src.populateJobForLoad(job)
	job.Configuration.Load.DestinationTable = l.Dst.tableRefProto()
	return job, media
}
