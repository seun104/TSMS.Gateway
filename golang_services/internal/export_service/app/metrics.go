package app

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	natsExportRequestsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "export",
			Name:      "nats_requests_received_total",
			Help:      "Total number of NATS export requests received.",
		},
		[]string{"subject"},
	)

	exportJobsProcessedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "export",
			Name:      "jobs_processed_total",
			Help:      "Total number of export jobs processed.",
		},
		[]string{"export_type", "status"}, // e.g., export_type="outbox_csv", status="success"
	)

	exportJobProcessingDurationHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "export",
			Name:      "job_processing_duration_seconds",
			Help:      "Duration of export job processing.",
			Buckets:   prometheus.DefBuckets, // Default buckets
		},
		[]string{"export_type"},
	)

	exportedRowsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "export",
			Name:      "rows_exported_total",
			Help:      "Total number of rows exported.",
		},
		[]string{"export_type"},
	)
)
