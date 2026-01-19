"""Celery configuration for Gmail attachment processing."""

import os
from celery import Celery

# Redis configuration for Celery broker
redis_url = os.getenv('REDIS_URL', 'redis://localhost:6379/0')

# Celery configuration
broker_url = redis_url
result_backend = redis_url

# Task routing
task_routes = {
    'penf_lib.connectors.gmail.tasks.process_attachment_background': {
        'queue': 'attachment_processing'
    },
    'penf_lib.connectors.gmail.tasks.cleanup_old_attachments': {
        'queue': 'maintenance'
    },
    'penf_lib.connectors.gmail.tasks.generate_storage_report': {
        'queue': 'maintenance'
    },
}

# Worker configuration
worker_prefetch_multiplier = 1
task_acks_late = True
worker_max_tasks_per_child = 1000

# Task configuration
task_serializer = 'json'
result_serializer = 'json'
accept_content = ['json']
timezone = 'UTC'
enable_utc = True

# Task timeout settings
task_soft_time_limit = 300  # 5 minutes
task_time_limit = 600  # 10 minutes
task_track_started = True

# Retry configuration
task_annotations = {
    'penf_lib.connectors.gmail.tasks.process_attachment_background': {
        'rate_limit': '10/m'  # Max 10 attachment processing tasks per minute
    }
}

# Result backend settings
result_expires = 3600  # 1 hour

# Queue configuration
task_default_queue = 'default'
task_queues = {
    'default': {
        'routing_key': 'default',
    },
    'attachment_processing': {
        'routing_key': 'attachment_processing',
    },
    'maintenance': {
        'routing_key': 'maintenance',
    },
}