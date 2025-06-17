package app

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	natsMessagesReceivedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "delivery_retrieval",
			Name:      "nats_messages_received_total",
			Help:      "Total number of NATS messages received.",
		},
		[]string{"subject_pattern"}, // e.g., "dlr.raw.*"
	)

	dlrEventsProcessedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "delivery_retrieval",
			Name:      "dlr_events_processed_total",
			Help:      "Total number of DLR events processed.",
		},
		[]string{"provider_name", "status"}, // status: "success", "error"
	)

	dlrEventProcessingDurationHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "delivery_retrieval",
			Name:      "dlr_event_processing_duration_seconds",
			Help:      "Duration of DLR event processing.",
			Buckets:   prometheus.DefBuckets, // Default buckets
		},
		[]string{"provider_name"},
	)
)
