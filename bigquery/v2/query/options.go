package query

import (
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
)

// WithClient allows to override the internal bigquery apiv2_client.Client
func WithClient(client *apiv2_client.Client) option.ClientOption {
	return &customClientOption{client: client}
}

type customClientOption struct {
	internaloption.EmbeddableAdapter
	client           *apiv2_client.Client
	billingProjectID string
}

// WithBillingProjectID sets the billing project ID for the client.
func WithBillingProjectID(projectID string) option.ClientOption {
	return &customClientOption{billingProjectID: projectID}
}

func (s *customClientOption) ApplyCustomClientOpt(c *QueryClient) {
	if s.client != nil {
		c.c = s.client
	}
	if s.billingProjectID != "" {
		c.billingProjectID = s.billingProjectID
	}
}
