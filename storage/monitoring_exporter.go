// Copyright 2026 Google LLC
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

// Adapted from github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric.

package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"cloud.google.com/go/storage/internal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/api/distribution"
	"google.golang.org/genproto/googleapis/api/label"
	googlemetricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// sendBatchSize is the maximum number of TimeSeries per CreateTimeSeries /
	// CreateServiceTimeSeries request enforced by Cloud Monitoring.
	sendBatchSize = 200

	defaultMetricDescriptorNameFormat   = "workload.googleapis.com/%s"
	platformMappingMonitoredResourceKey = "gcp.resource_type"
)

var (
	errMonitoringExporterShutdown = errors.New("storage: monitoring exporter is shutdown")
	errBlankProjectID             = errors.New("storage: google cloud monitoring project ID is empty")

	nilTraceID trace.TraceID
	nilSpanID  trace.SpanID
)

type metricDescriptorKey struct {
	name        string
	libraryName string
}

type monitoringExporterOptions struct {
	context                       context.Context
	projectID                     string
	metricDescriptorTypeFormatter func(metricdata.Metrics) string
	resourceAttributeFilter       attribute.Filter
	createServiceTimeSeries       bool
	disableCreateMetricDescriptor bool
	monitoredResourceType         string
	monitoredResourceLabels       []string
	monitoringClientOptions       []option.ClientOption
}

// monitoringExporter exports OpenTelemetry metrics to Google Cloud Monitoring
// via cloud.google.com/go/monitoring/apiv3/v2.
type monitoringExporter struct {
	opts         monitoringExporterOptions
	mrLabelSet   map[string]struct{}
	client       *monitoring.MetricClient
	mdLock       sync.RWMutex
	mdCache      map[metricDescriptorKey]*googlemetricpb.MetricDescriptor
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

func defaultMonitoringResourceAttributeFilter(kv attribute.KeyValue) bool {
	switch string(kv.Key) {
	case "service.name", "service.namespace", "service.instance.id":
		return len(kv.Value.AsString()) > 0
	default:
		return false
	}
}

func newMonitoringExporter(opts *monitoringExporterOptions) (*monitoringExporter, error) {
	var o monitoringExporterOptions
	if opts != nil {
		o = *opts
	}
	if o.context == nil {
		o.context = context.Background()
	}
	if o.resourceAttributeFilter == nil {
		o.resourceAttributeFilter = defaultMonitoringResourceAttributeFilter
	}
	if o.createServiceTimeSeries {
		o.disableCreateMetricDescriptor = true
	}

	if strings.TrimSpace(o.projectID) == "" {
		creds, err := google.FindDefaultCredentials(o.context, monitoring.DefaultAuthScopes()...)
		if err != nil {
			return nil, fmt.Errorf("storage: finding Google Cloud credentials for monitoring: %w", err)
		}
		if strings.TrimSpace(creds.ProjectID) == "" {
			return nil, errBlankProjectID
		}
		o.projectID = creds.ProjectID
	}

	ua := fmt.Sprintf("opentelemetry-go %s; cloud-storage-go %s", otel.Version(), internal.Version)
	clientOpts := append([]option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUserAgent(ua)),
	}, o.monitoringClientOptions...)

	client, err := monitoring.NewMetricClient(o.context, clientOpts...)
	if err != nil {
		return nil, err
	}

	mrLabelSet := make(map[string]struct{}, len(o.monitoredResourceLabels))
	for _, l := range o.monitoredResourceLabels {
		mrLabelSet[l] = struct{}{}
	}

	return &monitoringExporter{
		opts:       o,
		mrLabelSet: mrLabelSet,
		client:     client,
		mdCache:    make(map[metricDescriptorKey]*googlemetricpb.MetricDescriptor),
		shutdown:   make(chan struct{}),
	}, nil
}

func (me *monitoringExporter) ForceFlush(ctx context.Context) error {
	return ctx.Err()
}

func (me *monitoringExporter) Shutdown(ctx context.Context) error {
	err := errMonitoringExporterShutdown
	me.shutdownOnce.Do(func() {
		close(me.shutdown)
		err = errors.Join(ctx.Err(), me.client.Close())
	})
	return err
}

func (me *monitoringExporter) Temporality(ik sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(ik)
}

func (me *monitoringExporter) Aggregation(ik sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(ik)
}

func (me *monitoringExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	select {
	case <-me.shutdown:
		return errMonitoringExporterShutdown
	default:
	}
	if rm == nil {
		return nil
	}
	return errors.Join(
		me.exportMetricDescriptors(ctx, rm),
		me.exportTimeSeries(ctx, rm),
	)
}

func (me *monitoringExporter) exportMetricDescriptors(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	if me.opts.disableCreateMetricDescriptor {
		return nil
	}

	me.mdLock.Lock()
	defer me.mdLock.Unlock()

	pending := make(map[metricDescriptorKey]*googlemetricpb.MetricDescriptor)
	extraLabels := me.extraLabelsFromResource(rm.Resource)
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			k := metricDescriptorKey{name: m.Name, libraryName: scope.Scope.Name}
			if _, cached := me.mdCache[k]; cached {
				continue
			}
			if _, exists := pending[k]; !exists {
				pending[k] = me.recordToMetricDescriptor(m, extraLabels)
			}
		}
	}

	var errs []error
	for k, md := range pending {
		if err := me.createMetricDescriptorIfNeeded(ctx, md); err != nil {
			errs = append(errs, err)
			continue
		}
		me.mdCache[k] = md
	}
	return errors.Join(errs...)
}

func (me *monitoringExporter) createMetricDescriptorIfNeeded(ctx context.Context, md *googlemetricpb.MetricDescriptor) error {
	getReq := &monitoringpb.GetMetricDescriptorRequest{
		Name: fmt.Sprintf("projects/%s/metricDescriptors/%s", me.opts.projectID, md.Type),
	}
	if _, err := me.client.GetMetricDescriptor(ctx, getReq); err == nil {
		return nil
	}
	createReq := &monitoringpb.CreateMetricDescriptorRequest{
		Name:             fmt.Sprintf("projects/%s", me.opts.projectID),
		MetricDescriptor: md,
	}
	_, err := me.client.CreateMetricDescriptor(ctx, createReq)
	return err
}

func (me *monitoringExporter) exportTimeSeries(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	tss, err := me.recordsToTimeSeries(rm)
	if len(tss) == 0 {
		return err
	}

	name := fmt.Sprintf("projects/%s", me.opts.projectID)
	errs := []error{err}
	for i := 0; i < len(tss); i += sendBatchSize {
		end := i + sendBatchSize
		if end > len(tss) {
			end = len(tss)
		}
		req := &monitoringpb.CreateTimeSeriesRequest{
			Name:       name,
			TimeSeries: tss[i:end],
		}
		if me.opts.createServiceTimeSeries {
			errs = append(errs, me.client.CreateServiceTimeSeries(ctx, req))
		} else {
			errs = append(errs, me.client.CreateTimeSeries(ctx, req))
		}
	}
	return errors.Join(errs...)
}

func (me *monitoringExporter) extraLabelsFromResource(res *resource.Resource) *attribute.Set {
	if res == nil {
		empty := attribute.NewSet()
		return &empty
	}
	set, _ := attribute.NewSetWithFiltered(res.Attributes(), me.opts.resourceAttributeFilter)
	return &set
}

func (me *monitoringExporter) metricType(m metricdata.Metrics) string {
	if me.opts.metricDescriptorTypeFormatter != nil {
		return me.opts.metricDescriptorTypeFormatter(m)
	}
	return fmt.Sprintf(defaultMetricDescriptorNameFormat, m.Name)
}

func metricTypeDisplayName(mType string) string {
	u, err := url.Parse(fmt.Sprintf("metrics://%s", mType))
	if err != nil || u.Path == "" {
		return mType
	}
	return strings.TrimLeft(u.Path, "/")
}

func (me *monitoringExporter) recordToMetricDescriptor(m metricdata.Metrics, extraLabels *attribute.Set) *googlemetricpb.MetricDescriptor {
	typ := me.metricType(m)
	kind, valueType := aggregationToKindAndValueType(m.Data)
	return &googlemetricpb.MetricDescriptor{
		Name:        m.Name,
		DisplayName: metricTypeDisplayName(typ),
		Type:        typ,
		MetricKind:  kind,
		ValueType:   valueType,
		Unit:        m.Unit,
		Description: m.Description,
		Labels:      extractLabelDescriptors(m, extraLabels),
	}
}

func extractLabelDescriptors(m metricdata.Metrics, extraLabels *attribute.Set) []*label.LabelDescriptor {
	var labels []*label.LabelDescriptor
	seen := make(map[string]struct{})
	addSet := func(s *attribute.Set) {
		if s == nil {
			return
		}
		iter := s.Iter()
		for iter.Next() {
			kv := iter.Attribute()
			key := normalizeMonitoringLabelKey(string(kv.Key))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			labels = append(labels, &label.LabelDescriptor{Key: key})
		}
	}
	addSet(extraLabels)
	switch a := m.Data.(type) {
	case metricdata.Gauge[int64]:
		for _, pt := range a.DataPoints {
			addSet(&pt.Attributes)
		}
	case metricdata.Gauge[float64]:
		for _, pt := range a.DataPoints {
			addSet(&pt.Attributes)
		}
	case metricdata.Sum[int64]:
		for _, pt := range a.DataPoints {
			addSet(&pt.Attributes)
		}
	case metricdata.Sum[float64]:
		for _, pt := range a.DataPoints {
			addSet(&pt.Attributes)
		}
	case metricdata.Histogram[int64]:
		for _, pt := range a.DataPoints {
			addSet(&pt.Attributes)
		}
	case metricdata.Histogram[float64]:
		for _, pt := range a.DataPoints {
			addSet(&pt.Attributes)
		}
	}
	return labels
}

func aggregationToKindAndValueType(a metricdata.Aggregation) (googlemetricpb.MetricDescriptor_MetricKind, googlemetricpb.MetricDescriptor_ValueType) {
	switch agg := a.(type) {
	case metricdata.Gauge[int64]:
		return googlemetricpb.MetricDescriptor_GAUGE, googlemetricpb.MetricDescriptor_INT64
	case metricdata.Gauge[float64]:
		return googlemetricpb.MetricDescriptor_GAUGE, googlemetricpb.MetricDescriptor_DOUBLE
	case metricdata.Sum[int64]:
		if agg.IsMonotonic {
			return googlemetricpb.MetricDescriptor_CUMULATIVE, googlemetricpb.MetricDescriptor_INT64
		}
		return googlemetricpb.MetricDescriptor_GAUGE, googlemetricpb.MetricDescriptor_INT64
	case metricdata.Sum[float64]:
		if agg.IsMonotonic {
			return googlemetricpb.MetricDescriptor_CUMULATIVE, googlemetricpb.MetricDescriptor_DOUBLE
		}
		return googlemetricpb.MetricDescriptor_GAUGE, googlemetricpb.MetricDescriptor_DOUBLE
	case metricdata.Histogram[int64], metricdata.Histogram[float64]:
		return googlemetricpb.MetricDescriptor_CUMULATIVE, googlemetricpb.MetricDescriptor_DISTRIBUTION
	default:
		return googlemetricpb.MetricDescriptor_METRIC_KIND_UNSPECIFIED, googlemetricpb.MetricDescriptor_VALUE_TYPE_UNSPECIFIED
	}
}

func (me *monitoringExporter) resourceToMonitoredResource(res *resource.Resource) *monitoredrespb.MonitoredResource {
	if res == nil {
		return &monitoredrespb.MonitoredResource{
			Type:   "generic_node",
			Labels: map[string]string{"location": "global", "namespace": "", "node_id": ""},
		}
	}
	rset := res.Set()
	if mrType, ok := rset.Value(platformMappingMonitoredResourceKey); ok && me.opts.monitoredResourceType != "" && mrType.AsString() == me.opts.monitoredResourceType {
		labels := make(map[string]string, len(me.mrLabelSet))
		for expected := range me.mrLabelSet {
			if v, found := rset.Value(attribute.Key(expected)); found {
				labels[expected] = sanitizeMonitoringUTF8(v.AsString())
			}
		}
		return &monitoredrespb.MonitoredResource{
			Type:   mrType.AsString(),
			Labels: labels,
		}
	}
	return mapDefaultMonitoredResource(rset)
}

// mapDefaultMonitoredResource maps standard OpenTelemetry resource attributes
// onto Google Cloud Monitoring's default monitored resource types (gce_instance,
// k8s_*, gae_instance, generic_task, or generic_node).
func mapDefaultMonitoredResource(rset *attribute.Set) *monitoredrespb.MonitoredResource {
	getStr := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := rset.Value(attribute.Key(k)); ok {
				s := v.AsString()
				if s != "" && !strings.HasPrefix(s, "unknown_service") {
					return sanitizeMonitoringUTF8(s)
				}
			}
		}
		for _, k := range keys {
			if k == "service.name" {
				if v, ok := rset.Value("service.name"); ok && v.AsString() != "" {
					return sanitizeMonitoringUTF8(v.AsString())
				}
			}
		}
		return ""
	}
	hasKey := func(k string) bool {
		_, ok := rset.Value(attribute.Key(k))
		return ok
	}
	locOrGlobal := func() string {
		if loc := getStr("cloud.availability_zone", "cloud.region"); loc != "" {
			return loc
		}
		return "global"
	}

	cloudPlatform := getStr("cloud.platform")
	switch cloudPlatform {
	case "gcp_app_engine":
		return &monitoredrespb.MonitoredResource{
			Type: "gae_instance",
			Labels: map[string]string{
				"location":    getStr("cloud.availability_zone", "cloud.region"),
				"module_id":   getStr("faas.name"),
				"version_id":  getStr("faas.version"),
				"instance_id": getStr("faas.instance"),
			},
		}
	case "gcp_compute_engine":
		return &monitoredrespb.MonitoredResource{
			Type: "gce_instance",
			Labels: map[string]string{
				"zone":        getStr("cloud.availability_zone"),
				"instance_id": getStr("host.id"),
			},
		}
	case "aws_ec2":
		return &monitoredrespb.MonitoredResource{
			Type: "aws_ec2_instance",
			Labels: map[string]string{
				"instance_id": getStr("host.id"),
				"region":      getStr("cloud.availability_zone", "cloud.region"),
				"aws_account": getStr("cloud.account.id"),
			},
		}
	case "gcp_bare_metal_solution":
		return &monitoredrespb.MonitoredResource{
			Type: "baremetalsolution.googleapis.com/Instance",
			Labels: map[string]string{
				"location":    getStr("cloud.region"),
				"instance_id": getStr("host.id"),
			},
		}
	}

	if hasKey("k8s.cluster.name") {
		if hasKey("k8s.container.name") {
			return &monitoredrespb.MonitoredResource{
				Type: "k8s_container",
				Labels: map[string]string{
					"location":       getStr("cloud.availability_zone", "cloud.region"),
					"cluster_name":   getStr("k8s.cluster.name"),
					"namespace_name": getStr("k8s.namespace.name"),
					"pod_name":       getStr("k8s.pod.name"),
					"container_name": getStr("k8s.container.name"),
				},
			}
		}
		if hasKey("k8s.pod.name") {
			return &monitoredrespb.MonitoredResource{
				Type: "k8s_pod",
				Labels: map[string]string{
					"location":       getStr("cloud.availability_zone", "cloud.region"),
					"cluster_name":   getStr("k8s.cluster.name"),
					"namespace_name": getStr("k8s.namespace.name"),
					"pod_name":       getStr("k8s.pod.name"),
				},
			}
		}
		if hasKey("k8s.node.name") {
			return &monitoredrespb.MonitoredResource{
				Type: "k8s_node",
				Labels: map[string]string{
					"location":     getStr("cloud.availability_zone", "cloud.region"),
					"cluster_name": getStr("k8s.cluster.name"),
					"node_name":    getStr("k8s.node.name"),
				},
			}
		}
		return &monitoredrespb.MonitoredResource{
			Type: "k8s_cluster",
			Labels: map[string]string{
				"location":     getStr("cloud.availability_zone", "cloud.region"),
				"cluster_name": getStr("k8s.cluster.name"),
			},
		}
	}

	if (hasKey("service.name") && hasKey("service.instance.id")) || (hasKey("faas.name") && hasKey("faas.instance")) {
		return &monitoredrespb.MonitoredResource{
			Type: "generic_task",
			Labels: map[string]string{
				"location":  locOrGlobal(),
				"namespace": getStr("service.namespace"),
				"job":       getStr("service.name", "faas.name"),
				"task_id":   getStr("service.instance.id", "faas.instance"),
			},
		}
	}

	return &monitoredrespb.MonitoredResource{
		Type: "generic_node",
		Labels: map[string]string{
			"location":  locOrGlobal(),
			"namespace": getStr("service.namespace"),
			"node_id":   getStr("host.id", "host.name"),
		},
	}
}

func (me *monitoringExporter) recordsToTimeSeries(rm *metricdata.ResourceMetrics) ([]*monitoringpb.TimeSeries, error) {
	mr := me.resourceToMonitoredResource(rm.Resource)
	extraLabels := me.extraLabelsFromResource(rm.Resource)

	var (
		tss  []*monitoringpb.TimeSeries
		errs []error
	)
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			ts, err := me.metricToTimeSeries(m, mr, scope.Scope, extraLabels)
			if err != nil {
				errs = append(errs, err)
			}
			tss = append(tss, ts...)
		}
	}
	return tss, errors.Join(errs...)
}

func (me *monitoringExporter) metricToProto(m metricdata.Metrics, ptAttrs attribute.Set, scope instrumentation.Scope, extraLabels *attribute.Set) *googlemetricpb.Metric {
	me.mdLock.RLock()
	md, ok := me.mdCache[metricDescriptorKey{name: m.Name, libraryName: scope.Name}]
	me.mdLock.RUnlock()

	typ := me.metricType(m)
	if ok {
		typ = md.Type
	}

	labels := make(map[string]string)
	addAttrs := func(s *attribute.Set) {
		if s == nil {
			return
		}
		iter := s.Iter()
		for iter.Next() {
			kv := iter.Attribute()
			labels[normalizeMonitoringLabelKey(string(kv.Key))] = sanitizeMonitoringUTF8(kv.Value.String())
		}
	}
	addAttrs(extraLabels)
	addAttrs(&ptAttrs)

	return &googlemetricpb.Metric{
		Type:   typ,
		Labels: labels,
	}
}

func (me *monitoringExporter) metricToTimeSeries(m metricdata.Metrics, mr *monitoredrespb.MonitoredResource, scope instrumentation.Scope, extraLabels *attribute.Set) ([]*monitoringpb.TimeSeries, error) {
	if m.Data == nil {
		return nil, nil
	}
	var (
		tss  []*monitoringpb.TimeSeries
		errs []error
	)
	switch a := m.Data.(type) {
	case metricdata.Gauge[int64]:
		for _, pt := range a.DataPoints {
			ts, err := gaugeDataPointToTimeSeries(pt, m, mr)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			ts.Metric = me.metricToProto(m, pt.Attributes, scope, extraLabels)
			tss = append(tss, ts)
		}
	case metricdata.Gauge[float64]:
		for _, pt := range a.DataPoints {
			ts, err := gaugeDataPointToTimeSeries(pt, m, mr)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			ts.Metric = me.metricToProto(m, pt.Attributes, scope, extraLabels)
			tss = append(tss, ts)
		}
	case metricdata.Sum[int64]:
		for _, pt := range a.DataPoints {
			var (
				ts  *monitoringpb.TimeSeries
				err error
			)
			if a.IsMonotonic {
				ts, err = sumDataPointToTimeSeries(pt, m, mr)
			} else {
				ts, err = gaugeDataPointToTimeSeries(pt, m, mr)
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}
			ts.Metric = me.metricToProto(m, pt.Attributes, scope, extraLabels)
			tss = append(tss, ts)
		}
	case metricdata.Sum[float64]:
		for _, pt := range a.DataPoints {
			var (
				ts  *monitoringpb.TimeSeries
				err error
			)
			if a.IsMonotonic {
				ts, err = sumDataPointToTimeSeries(pt, m, mr)
			} else {
				ts, err = gaugeDataPointToTimeSeries(pt, m, mr)
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}
			ts.Metric = me.metricToProto(m, pt.Attributes, scope, extraLabels)
			tss = append(tss, ts)
		}
	case metricdata.Histogram[int64]:
		for _, pt := range a.DataPoints {
			ts, err := histogramDataPointToTimeSeries(pt, m, mr, me.opts.projectID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			ts.Metric = me.metricToProto(m, pt.Attributes, scope, extraLabels)
			tss = append(tss, ts)
		}
	case metricdata.Histogram[float64]:
		for _, pt := range a.DataPoints {
			ts, err := histogramDataPointToTimeSeries(pt, m, mr, me.opts.projectID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			ts.Metric = me.metricToProto(m, pt.Attributes, scope, extraLabels)
			tss = append(tss, ts)
		}
	default:
		errs = append(errs, fmt.Errorf("storage: unsupported metric aggregation kind %v", reflect.TypeOf(m.Data)))
	}
	return tss, errors.Join(errs...)
}

func gaugeDataPointToTimeSeries[N int64 | float64](pt metricdata.DataPoint[N], m metricdata.Metrics, mr *monitoredrespb.MonitoredResource) (*monitoringpb.TimeSeries, error) {
	value, valueType := numberDataPointToTypedValue(pt)
	endTime := timestamppb.New(pt.Time)
	if err := endTime.CheckValid(); err != nil {
		return nil, err
	}
	return &monitoringpb.TimeSeries{
		Resource:    mr,
		Unit:        m.Unit,
		Description: m.Description,
		MetricKind:  googlemetricpb.MetricDescriptor_GAUGE,
		ValueType:   valueType,
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{EndTime: endTime},
			Value:    value,
		}},
	}, nil
}

func sumDataPointToTimeSeries[N int64 | float64](pt metricdata.DataPoint[N], m metricdata.Metrics, mr *monitoredrespb.MonitoredResource) (*monitoringpb.TimeSeries, error) {
	interval, err := toNonemptyTimeInterval(pt.StartTime, pt.Time)
	if err != nil {
		return nil, err
	}
	value, valueType := numberDataPointToTypedValue(pt)
	return &monitoringpb.TimeSeries{
		Resource:    mr,
		Unit:        m.Unit,
		Description: m.Description,
		MetricKind:  googlemetricpb.MetricDescriptor_CUMULATIVE,
		ValueType:   valueType,
		Points: []*monitoringpb.Point{{
			Interval: interval,
			Value:    value,
		}},
	}, nil
}

func histogramDataPointToTimeSeries[N int64 | float64](pt metricdata.HistogramDataPoint[N], m metricdata.Metrics, mr *monitoredrespb.MonitoredResource, projectID string) (*monitoringpb.TimeSeries, error) {
	interval, err := toNonemptyTimeInterval(pt.StartTime, pt.Time)
	if err != nil {
		return nil, err
	}
	counts := make([]int64, len(pt.BucketCounts))
	for i, c := range pt.BucketCounts {
		counts[i] = int64(c)
	}
	var mean float64
	if !math.IsNaN(float64(pt.Sum)) && pt.Count > 0 {
		mean = float64(pt.Sum) / float64(pt.Count)
	}
	dist := &distribution.Distribution{
		Count:        int64(pt.Count),
		Mean:         mean,
		BucketCounts: counts,
		BucketOptions: &distribution.Distribution_BucketOptions{
			Options: &distribution.Distribution_BucketOptions_ExplicitBuckets{
				ExplicitBuckets: &distribution.Distribution_BucketOptions_Explicit{
					Bounds: pt.Bounds,
				},
			},
		},
		Exemplars: toDistributionExemplars(pt.Exemplars, projectID),
	}
	return &monitoringpb.TimeSeries{
		Resource:    mr,
		Unit:        m.Unit,
		Description: m.Description,
		MetricKind:  googlemetricpb.MetricDescriptor_CUMULATIVE,
		ValueType:   googlemetricpb.MetricDescriptor_DISTRIBUTION,
		Points: []*monitoringpb.Point{{
			Interval: interval,
			Value: &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_DistributionValue{
					DistributionValue: dist,
				},
			},
		}},
	}, nil
}

func toDistributionExemplars[N int64 | float64](in []metricdata.Exemplar[N], projectID string) []*distribution.Distribution_Exemplar {
	if len(in) == 0 {
		return nil
	}
	out := make([]*distribution.Distribution_Exemplar, 0, len(in))
	for _, e := range in {
		var attachments []*anypb.Any
		if !bytes.Equal(e.TraceID[:], nilTraceID[:]) && !bytes.Equal(e.SpanID[:], nilSpanID[:]) {
			sctx, err := anypb.New(&monitoringpb.SpanContext{
				SpanName: fmt.Sprintf("projects/%s/traces/%s/spans/%s", projectID, hex.EncodeToString(e.TraceID[:]), hex.EncodeToString(e.SpanID[:])),
			})
			if err == nil {
				attachments = append(attachments, sctx)
			}
		}
		if len(e.FilteredAttributes) > 0 {
			labels := make(map[string]string, len(e.FilteredAttributes))
			for _, attr := range e.FilteredAttributes {
				labels[normalizeMonitoringLabelKey(string(attr.Key))] = sanitizeMonitoringUTF8(attr.Value.String())
			}
			if dropped, err := anypb.New(&monitoringpb.DroppedLabels{Label: labels}); err == nil {
				attachments = append(attachments, dropped)
			}
		}
		out = append(out, &distribution.Distribution_Exemplar{
			Value:       float64(e.Value),
			Timestamp:   timestamppb.New(e.Time),
			Attachments: attachments,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Value < out[j].Value
	})
	return out
}

func toNonemptyTimeInterval(start, end time.Time) (*monitoringpb.TimeInterval, error) {
	if end.Sub(start).Milliseconds() <= 1 {
		end = start.Add(time.Millisecond)
	}
	startPB := timestamppb.New(start)
	endPB := timestamppb.New(end)
	if err := errors.Join(startPB.CheckValid(), endPB.CheckValid()); err != nil {
		return nil, err
	}
	return &monitoringpb.TimeInterval{
		StartTime: startPB,
		EndTime:   endPB,
	}, nil
}

func numberDataPointToTypedValue[N int64 | float64](pt metricdata.DataPoint[N]) (*monitoringpb.TypedValue, googlemetricpb.MetricDescriptor_ValueType) {
	switch v := any(pt.Value).(type) {
	case int64:
		return &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{Int64Value: v}},
			googlemetricpb.MetricDescriptor_INT64
	case float64:
		return &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: v}},
			googlemetricpb.MetricDescriptor_DOUBLE
	default:
		return nil, googlemetricpb.MetricDescriptor_INT64
	}
}

func normalizeMonitoringLabelKey(s string) string {
	if s == "" {
		return s
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, s)
	if unicode.IsDigit(rune(s[0])) {
		return "key_" + s
	}
	return s
}

func sanitizeMonitoringUTF8(s string) string {
	return strings.ToValidUTF8(s, "\ufffd")
}
