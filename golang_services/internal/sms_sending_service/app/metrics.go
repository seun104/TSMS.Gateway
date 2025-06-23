package app

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	natsSMSJobsReceivedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sms_sending",
			Name:      "nats_jobs_received_total",
			Help:      "Total NATS SMS jobs received.",
		},
		[]string{"subject"},
	)

	smsSendingProcessedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sms_sending",
			Name:      "jobs_processed_total",
			Help:      "Total SMS jobs processed.",
		},
		[]string{"provider_name", "status"}, // e.g., status: "success", "error_billing", "error_provider", ...
	)

	smsSendingProcessingDurationHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "sms_sending",
			Name:      "job_processing_duration_seconds",
			Help:      "Duration of SMS job processing.",
			Buckets:   prometheus.DefBuckets, // Default buckets
		},
		[]string{"provider_name"},
	)

	billingServiceRequestDurationHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "sms_sending",
			Name:      "billing_service_request_duration_seconds",
			Help:      "Duration of gRPC requests to billing service.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"grpc_method"}, // e.g., "DeductUserCreditForSMS"
	)

	// Note: Renaming to singular "provider" for consistency with other metrics
	smsProviderRequestDurationHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "sms_sending",
			Name:      "provider_request_duration_seconds",
			Help:      "Duration of HTTP requests to SMS providers.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"provider_name"},
	)

	smsSegmentsSentCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "sms_sending",
			Name:      "segments_sent_total",
			Help:      "Total number of SMS segments sent.",
		},
		[]string{"provider_name"},
	)
)
