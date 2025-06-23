package app

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	natsInboundSMSReceivedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "inbound_processor",
			Name:      "nats_messages_received_total",
			Help:      "Total number of NATS messages received for inbound SMS.",
		},
		[]string{"subject_pattern"}, // e.g., "sms.incoming.raw.*"
	)

	inboundSMSProcessedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "inbound_processor",
			Name:      "sms_processed_total",
			Help:      "Total number of inbound SMS messages processed.",
		},
		[]string{"provider_name", "status"}, // status: "success", "error_parsing", "error_db_save", "error_priv_num_assoc", "error_unknown"
	)

	inboundSMSProcessingDurationHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "inbound_processor",
			Name:      "sms_processing_duration_seconds",
			Help:      "Duration of inbound SMS message processing.",
			Buckets:   prometheus.DefBuckets, // Default buckets
		},
		[]string{"provider_name"},
	)
)
