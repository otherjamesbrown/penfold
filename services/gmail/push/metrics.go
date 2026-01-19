// Package push provides Gmail push notification handling via Cloud Pub/Sub.
package push

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PushMetrics holds Prometheus metrics for push notification operations.
type PushMetrics struct {
	// Notification metrics.
	NotificationsReceived  prometheus.Counter
	NotificationsProcessed prometheus.Counter
	NotificationErrors     prometheus.Counter
	DuplicateNotifications prometheus.Counter

	// Subscription metrics.
	SubscriptionsCreated prometheus.Counter
	SubscriptionsRenewed prometheus.Counter
	SubscriptionsDeleted prometheus.Counter
	SubscriptionErrors   prometheus.Counter
	ActiveSubscriptions  prometheus.Gauge

	// HTTP server metrics.
	HTTPRequests *prometheus.CounterVec
	HTTPErrors   *prometheus.CounterVec

	// Processor metrics.
	TasksSubmitted    prometheus.Counter
	TasksProcessing   prometheus.Gauge
	TasksCompleted    prometheus.Counter
	TasksFailed       prometheus.Counter
	TasksRetried      prometheus.Counter
	TasksDropped      prometheus.Counter
	TaskLatency       prometheus.Histogram
	BatchesFlushed    prometheus.Counter
	NotificationsBatched prometheus.Counter
}

// NewPushMetrics creates and returns push notification metrics.
func NewPushMetrics(namespace, service string) *PushMetrics {
	return &PushMetrics{
		NotificationsReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_notifications_received_total",
			Help:        "Total number of push notifications received",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		NotificationsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_notifications_processed_total",
			Help:        "Total number of push notifications processed successfully",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		NotificationErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_notification_errors_total",
			Help:        "Total number of push notification errors",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		DuplicateNotifications: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_duplicate_notifications_total",
			Help:        "Total number of duplicate notifications received",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		SubscriptionsCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_subscriptions_created_total",
			Help:        "Total number of push subscriptions created",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		SubscriptionsRenewed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_subscriptions_renewed_total",
			Help:        "Total number of push subscriptions renewed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		SubscriptionsDeleted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_subscriptions_deleted_total",
			Help:        "Total number of push subscriptions deleted",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		SubscriptionErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_subscription_errors_total",
			Help:        "Total number of subscription errors",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		ActiveSubscriptions: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "gmail_push_active_subscriptions",
			Help:        "Current number of active push subscriptions",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		HTTPRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "gmail_push_http_requests_total",
				Help:        "Total number of HTTP requests to push endpoints",
				ConstLabels: prometheus.Labels{"service": service},
			},
			[]string{"endpoint", "method"},
		),
		HTTPErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "gmail_push_http_errors_total",
				Help:        "Total number of HTTP errors",
				ConstLabels: prometheus.Labels{"service": service},
			},
			[]string{"endpoint", "error_type"},
		),
		TasksSubmitted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_tasks_submitted_total",
			Help:        "Total number of processing tasks submitted",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksProcessing: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Name:        "gmail_push_tasks_processing",
			Help:        "Current number of tasks being processed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_tasks_completed_total",
			Help:        "Total number of tasks completed successfully",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_tasks_failed_total",
			Help:        "Total number of tasks that failed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksRetried: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_tasks_retried_total",
			Help:        "Total number of tasks retried",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TasksDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_tasks_dropped_total",
			Help:        "Total number of tasks dropped due to queue full",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		TaskLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   namespace,
			Name:        "gmail_push_task_latency_seconds",
			Help:        "Task processing latency in seconds",
			Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
			ConstLabels: prometheus.Labels{"service": service},
		}),
		BatchesFlushed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_batches_flushed_total",
			Help:        "Total number of notification batches flushed",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		NotificationsBatched: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "gmail_push_notifications_batched_total",
			Help:        "Total number of notifications included in batches",
			ConstLabels: prometheus.Labels{"service": service},
		}),
	}
}

// Register registers all metrics with the default Prometheus registry.
func (m *PushMetrics) Register() error {
	collectors := []prometheus.Collector{
		m.NotificationsReceived,
		m.NotificationsProcessed,
		m.NotificationErrors,
		m.DuplicateNotifications,
		m.SubscriptionsCreated,
		m.SubscriptionsRenewed,
		m.SubscriptionsDeleted,
		m.SubscriptionErrors,
		m.ActiveSubscriptions,
		m.HTTPRequests,
		m.HTTPErrors,
		m.TasksSubmitted,
		m.TasksProcessing,
		m.TasksCompleted,
		m.TasksFailed,
		m.TasksRetried,
		m.TasksDropped,
		m.TaskLatency,
		m.BatchesFlushed,
		m.NotificationsBatched,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// Unregister unregisters all metrics from the default Prometheus registry.
func (m *PushMetrics) Unregister() {
	prometheus.Unregister(m.NotificationsReceived)
	prometheus.Unregister(m.NotificationsProcessed)
	prometheus.Unregister(m.NotificationErrors)
	prometheus.Unregister(m.DuplicateNotifications)
	prometheus.Unregister(m.SubscriptionsCreated)
	prometheus.Unregister(m.SubscriptionsRenewed)
	prometheus.Unregister(m.SubscriptionsDeleted)
	prometheus.Unregister(m.SubscriptionErrors)
	prometheus.Unregister(m.ActiveSubscriptions)
	prometheus.Unregister(m.HTTPRequests)
	prometheus.Unregister(m.HTTPErrors)
	prometheus.Unregister(m.TasksSubmitted)
	prometheus.Unregister(m.TasksProcessing)
	prometheus.Unregister(m.TasksCompleted)
	prometheus.Unregister(m.TasksFailed)
	prometheus.Unregister(m.TasksRetried)
	prometheus.Unregister(m.TasksDropped)
	prometheus.Unregister(m.TaskLatency)
	prometheus.Unregister(m.BatchesFlushed)
	prometheus.Unregister(m.NotificationsBatched)
}
