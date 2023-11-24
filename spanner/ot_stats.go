package spanner

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var meter = otel.GetMeterProvider().Meter(statsPrefix)
var (
	TagKeyClientIDOT   = attribute.Key("client_id")
	TagKeyDatabaseOT   = attribute.Key("database")
	TagKeyInstanceOT   = attribute.Key("instance_id")
	TagKeyLibVersionOT = attribute.Key("library_version")
	TagKeyTypeOT       = attribute.Key("type")

	TagNumInUseSessionsOT = TagKeyTypeOT.String("num_in_use_sessions")
	TagNumSessionsOT      = TagKeyTypeOT.String("num_sessions")

	OpenSessionCountOT, _ = meter.Int64ObservableGauge(
		statsPrefix+"open_session_count_test_ot",
		metric.WithDescription("Number of sessions currently opened"),
		metric.WithUnit("1"),
	)

	MaxAllowedSessionsCountOT, _ = meter.Int64ObservableGauge(
		statsPrefix+"max_allowed_sessions_test_ot",
		metric.WithDescription("The maximum number of sessions allowed. Configurable by the user."),
		metric.WithUnit("1"),
	)

	SessionsCountOT, _ = meter.Int64ObservableGauge(
		statsPrefix+"num_sessions_in_pool_test_ot",
		metric.WithDescription("The number of sessions currently in use."),
		metric.WithUnit("1"),
	)

	MaxInUseSessionsCountOT, _ = meter.Int64ObservableGauge(
		statsPrefix+"max_in_use_sessions_test_ot",
		metric.WithDescription("The maximum number of sessions in use during the last 10 minute interval."),
		metric.WithUnit("1"),
	)

	GetSessionTimeoutsCountOT, _ = meter.Int64UpDownCounter(
		statsPrefix+"get_session_timeouts_test_ot",
		metric.WithDescription("The number of get sessions timeouts due to pool exhaustion."),
		metric.WithUnit("1"),
	)

	AcquiredSessionsCountOT, _ = meter.Int64UpDownCounter(
		statsPrefix+"num_acquired_sessions_test_ot",
		metric.WithDescription("The number of sessions acquired from the session pool."),
		metric.WithUnit("1"),
	)

	ReleasedSessionsCountOT, _ = meter.Int64UpDownCounter(
		statsPrefix+"num_released_sessions_test_ot",
		metric.WithDescription("The number of sessions released by the user and pool maintainer."),
		metric.WithUnit("1"),
	)
)

func getOTMetricsEnabled() bool {
	return true
}

func captureSessionPoolOTMetrics(pool *sessionPool) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	if !getOTMetricsEnabled() {
		return nil
	}

	attributes := pool.otTagMap
	attributesInUseSessions := append(attributes, TagNumInUseSessionsOT)
	attributesAvailableSessions := append(attributes, TagNumSessionsOT)

	reg, err := meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			pool.mu.Lock()
			defer pool.mu.Unlock()

			o.ObserveInt64(OpenSessionCountOT, int64(pool.numOpened), metric.WithAttributes(attributes...))
			o.ObserveInt64(MaxAllowedSessionsCountOT, int64(pool.SessionPoolConfig.MaxOpened), metric.WithAttributes(attributes...))
			o.ObserveInt64(SessionsCountOT, int64(pool.numInUse), metric.WithAttributes(attributesInUseSessions...))
			o.ObserveInt64(SessionsCountOT, int64(pool.numSessions), metric.WithAttributes(attributesAvailableSessions...))
			o.ObserveInt64(MaxInUseSessionsCountOT, int64(pool.maxNumInUse), metric.WithAttributes(attributes...))

			return nil
		},
		OpenSessionCountOT,
		MaxAllowedSessionsCountOT,
		SessionsCountOT,
		MaxInUseSessionsCountOT,
	)
	pool.otMetricRegister = reg
	return err
}
