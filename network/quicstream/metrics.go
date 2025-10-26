package quicstream

import (
	"context"

	"github.com/ProtoconNet/mitum2/util"
)

// MetricsCollectorContextKey is used to propagate the optional metrics collector
// through contexts that are shared by the networking stack.
var MetricsCollectorContextKey = util.ContextKey("network-metrics-collector")

// MetricsCollector represents the subset of metrics hooks used by the network
// stack. Implementations are expected to be concurrency-safe.
type MetricsCollector interface {
	RecordQuicBytesSent(uint64)
	RecordQuicBytesReceived(uint64)
	RecordQuicStreamOpened()
	RecordQuicStreamClosed()
	RecordQuicConnectionOpened()
	RecordQuicConnectionClosed()
	RecordMemberlistBroadcast()
	RecordMemberlistMessageReceived()
	SetMemberlistMembers(int)
}

// WithMetricsCollector returns a derived context containing the provided
// collector.
func WithMetricsCollector(ctx context.Context, collector MetricsCollector) context.Context {
	if ctx == nil || collector == nil {
		return ctx
	}

	return context.WithValue(ctx, MetricsCollectorContextKey, collector)
}

// GetMetricsCollector extracts the MetricsCollector stored in the context, if
// any.
func GetMetricsCollector(ctx context.Context) MetricsCollector {
	if ctx == nil {
		return nil
	}

	if collector, ok := ctx.Value(MetricsCollectorContextKey).(MetricsCollector); ok {
		return collector
	}

	return nil
}
