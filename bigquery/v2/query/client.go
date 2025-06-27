package query

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"google.golang.org/api/option"
)

// QueryClient is a client for running queries in BigQuery.
type QueryClient struct {
	c                *apiv2_client.Client
	projectID        string
	billingProjectID string
}

// NewClient creates a new query client.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*QueryClient, error) {
	qc := &QueryClient{projectID: projectID, billingProjectID: projectID}
	for _, opt := range opts {
		if cOpt, ok := opt.(*customClientOption); ok {
			cOpt.ApplyCustomClientOpt(qc)
		}
	}
	if qc.c == nil {
		c, err := apiv2_client.NewClient(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to setup bigquery client: %w", err)
		}
		qc.c = c
	}
	return qc, nil
}

func (c *QueryClient) Close() error {
	return c.c.Close()
}

// NewQueryRunner creates a new QueryRunner.
func (c *QueryClient) NewQueryRunner() *QueryRunner {
	return &QueryRunner{
		c: c,
	}
}

// NewQueryReader creates a new QueryReader.
func (c *QueryClient) NewQueryReader() *QueryReader {
	return &QueryReader{
		c: c,
	}
}
