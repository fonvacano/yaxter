package fanout

import "github.com/prometheus/client_golang/prometheus"

// Metrics are the fan-out worker's domain metrics (ARCHITECTURE.md §5).
type Metrics struct {
	LagSeconds prometheus.Gauge
	Processed  prometheus.Counter
	Skipped    prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		LagSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fanout_lag_seconds",
			Help: "Age of the tweet event at fan-out consume time (commit→fanout).",
		}),
		Processed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fanout_events_processed_total",
			Help: "Tweet events fanned out (post-dedupe).",
		}),
		Skipped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fanout_events_skipped_celebrity_total",
			Help: "Tweet events skipped because the author is over the celebrity threshold.",
		}),
	}
	reg.MustRegister(m.LagSeconds, m.Processed, m.Skipped)
	return m
}
