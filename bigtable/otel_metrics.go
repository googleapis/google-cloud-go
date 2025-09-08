package bigtable

import (
	"context"
	"fmt"
	"strings"
	"time"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/experimental/stats"
	"google.golang.org/grpc/stats/opentelemetry"
)

const (
	bigtableClientMonitoredResourceName = "bigtable_client"
	bigtableClientMetricPrefix          = "bigtable.googleapis.com/internal/client/"
)

// bigtable_client monitored resource labels
type bigtableClientMonitoredResource struct {
	project       string // project
	instance      string // instance
	appProfile    string // app_profile
	clientProject string // client_project
	cloudPlatform string // cloud_platform
	region        string // client_region
	hostID        string // host_id
	hostName      string // host_name
	clientName    string // client_name
	clientUID     string // uuid
	resource      *resource.Resource
}

func (bmr *bigtableClientMonitoredResource) exporter() (metric.Exporter, error) {
	exporter, err := mexporter.New(
		mexporter.WithProjectID(bmr.clientProject),
		mexporter.WithMetricDescriptorTypeFormatter(metricFormatter),
		mexporter.WithCreateServiceTimeSeries(),
		mexporter.WithMonitoredResourceDescription(bigtableClientMonitoredResourceName, []string{"project_id", "instance", "app_profile", "client_project", "cloud_platform", "host_id", "host_name", "client_name", "uuid", "region"}),
	)
	if err != nil {
		return nil, fmt.Errorf("bigtable: creating metrics exporter: %w", err)
	}
	return exporter, nil
}

func metricFormatter(m metricdata.Metrics) string {
	// converts grpc.lb.rls.target_picks to `bigtable.googleapis.com/internal/client/grpc/lb/rls/target_picks`
	return bigtableClientMetricPrefix + strings.ReplaceAll(string(m.Name), ".", "/")
}

func newBigtableClientMonitoredResource(ctx context.Context, project, clientProject, appProfile, instance, clientName, clientUID string, opts ...resource.Option) (*bigtableClientMonitoredResource, error) {
	detectedAttrs, err := resource.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	smr := &bigtableClientMonitoredResource{
		project:       project,
		instance:      instance,
		appProfile:    appProfile,
		clientName:    clientName,
		clientUID:     clientUID,
		clientProject: clientProject,
	}
	s := detectedAttrs.Set()
	// Attempt to use resource detector project id if project id wasn't
	// identified using ADC as a last resort. Otherwise metrics cannot be started.
	if p, present := s.Value("cloud.account.id"); present {
		smr.project = p.AsString()
	}
	if v, ok := s.Value("cloud.platform"); ok {
		smr.cloudPlatform = v.AsString()
	} else {
		smr.cloudPlatform = "unknown"
	}
	if v, ok := s.Value("host.id"); ok {
		smr.hostID = v.AsString()
		// cloud run / cloud functions have faas.id instead of host.id
	} else if v, ok := s.Value("faas.id"); ok {
		smr.hostID = v.AsString()
	} else {
		smr.hostID = "unknown"
	}

	if v, ok := s.Value("cloud.region"); ok {
		smr.region = v.AsString()
	} else {
		smr.region = "global"
	}
	if v, ok := s.Value("host.name"); ok {
		smr.hostName = v.AsString()
	} else {
		smr.hostName = "unknown"
	}
	smr.resource, err = resource.New(ctx, resource.WithAttributes([]attribute.KeyValue{
		{Key: "gcp.resource_type", Value: attribute.StringValue(bigtableClientMonitoredResourceName)},
		{Key: "project_id", Value: attribute.StringValue(project)},
		{Key: "app_profile", Value: attribute.StringValue(smr.appProfile)},
		{Key: "region", Value: attribute.StringValue(smr.region)},
		{Key: "instance", Value: attribute.StringValue(smr.instance)},
		{Key: "cloud_platform", Value: attribute.StringValue(smr.cloudPlatform)},
		{Key: "host_id", Value: attribute.StringValue(smr.hostID)},
		{Key: "host_name", Value: attribute.StringValue(smr.hostName)},
		{Key: "client_name", Value: attribute.StringValue(smr.clientName)},
		{Key: "uuid", Value: attribute.StringValue(smr.clientUID)},
		{Key: "client_project", Value: attribute.StringValue(smr.clientProject)},
	}...))
	if err != nil {
		return nil, err
	}
	return smr, nil
}

type metricsContext struct {
	// client options passed to gRPC channels
	clientOpts []option.ClientOption
	// instance of metric reader used by gRPC client-side metrics
	provider *metric.MeterProvider
	// clean func to call when closing gRPC client
	close func()
}

type metricsConfig struct {
	project         string // project_id
	instance        string // instance
	appProfile      string // app_profile
	clientName      string // client_name
	clientUID       string // uuid
	clientProject   string // client_project``
	interval        time.Duration
	customExporter  *metric.Exporter
	manualReader    *metric.ManualReader // used by tests
	disableExporter bool                 // used by tests disables exports
	resourceOpts    []resource.Option    // used by tests
}

func newOtelMetricsContext(ctx context.Context, cfg metricsConfig) (*metricsContext, error) {
	var exporter metric.Exporter
	meterOpts := []metric.Option{}
	if cfg.customExporter == nil {
		var ropts []resource.Option
		if cfg.resourceOpts != nil {
			ropts = cfg.resourceOpts
		} else {
			ropts = []resource.Option{resource.WithDetectors(gcp.NewDetector())}
		}
		smr, err := newBigtableClientMonitoredResource(ctx, cfg.project, cfg.clientProject, cfg.appProfile, cfg.instance, cfg.clientName, cfg.clientUID, ropts...)
		if err != nil {
			return nil, err
		}
		exporter, err = smr.exporter()
		if err != nil {
			return nil, err
		}
		meterOpts = append(meterOpts, metric.WithResource(smr.resource))
	} else {
		exporter = *cfg.customExporter
	}
	interval := time.Minute
	if cfg.interval > 0 {
		interval = cfg.interval
	}
	meterOpts = append(meterOpts,
		// customer histogram boundaries
		metric.WithView(
			createHistogramView("grpc.client.attempt.duration", latencyHistogramBoundaries()),
		))
	if cfg.manualReader != nil {
		meterOpts = append(meterOpts, metric.WithReader(cfg.manualReader))
	}
	if !cfg.disableExporter {
		meterOpts = append(meterOpts, metric.WithReader(
			metric.NewPeriodicReader(&exporterLogSuppressor{Exporter: exporter}, metric.WithInterval(interval))))
	}
	provider := metric.NewMeterProvider(meterOpts...)
	mo := opentelemetry.MetricsOptions{
		MeterProvider: provider,
		Metrics: stats.NewMetrics(
			"grpc.client.attempt.duration",
			"grpc.lb.rls.default_target_picks",
			"grpc.lb.rls.target_picks",
			"grpc.lb.rls.failed_picks",
			"grpc.xds_client.server_failure",
			"grpc.xds_client.resource_updates_invalid",
		),
		OptionalLabels: []string{"grpc.lb.locality"},
	}
	opts := []option.ClientOption{
		option.WithGRPCDialOption(
			opentelemetry.DialOption(opentelemetry.Options{MetricsOptions: mo})),
		option.WithGRPCDialOption(
			grpc.WithDefaultCallOptions(grpc.StaticMethodCallOption{})),
	}
	return &metricsContext{
		clientOpts: opts,
		provider:   provider,
		close: func() {
			provider.Shutdown(ctx)
		},
	}, nil
}

// Silences permission errors after initial error is emitted to prevent
// chatty logs.
type exporterLogSuppressor struct {
	metric.Exporter
	emittedFailure bool
}

// Implements OTel SDK metric.Exporter interface to prevent noisy logs from
// lack of credentials after initial failure.
func (e *exporterLogSuppressor) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	if err := e.Exporter.Export(ctx, rm); err != nil && !e.emittedFailure {
		if strings.Contains(err.Error(), "PermissionDenied") {
			e.emittedFailure = true
			return fmt.Errorf("gRPC metrics failed due permission issue: %w", err)
		}
		return err
	}
	return nil
}

func latencyHistogramBoundaries() []float64 {
	// numbers in seconds
	boundaries := []float64{0.0, 0.001, 0.002, 0.003, 0.004, 0.005, 0.006, 0.008, 0.01, 0.013, 0.016, 0.02, 0.025, 0.03, 0.04, 0.05, 0.065, 0.08, 0.1, 0.13, 0.16, 0.2, 0.25, 0.3, 0.4, 0.5, 0.65, 0.8, 1.0, 2.0, 5.0, 10.0, 20.0, 50.0, 100.0, 200.0, 400.0, 800.0, 1600.0, 3200.0} // max is 53.3 minutes
	return boundaries
}

func createHistogramView(name string, boundaries []float64) metric.View {
	return metric.NewView(metric.Instrument{
		Name: name,
		Kind: metric.InstrumentKindHistogram,
	}, metric.Stream{
		Name:        name,
		Aggregation: metric.AggregationExplicitBucketHistogram{Boundaries: boundaries},
	})
}
