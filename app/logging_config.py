"""
Logging Configuration for Meeting Pipeline

Provides structured logging with proper security audit trails
and configurable output formats for development and production.
"""
import logging
import logging.config
import json
import os
from datetime import datetime
from typing import Dict, Any

# Configure structured logging for production
LOGGING_CONFIG = {
    'version': 1,
    'disable_existing_loggers': False,
    'formatters': {
        'standard': {
            'format': '%(asctime)s [%(levelname)s] %(name)s: %(message)s'
        },
        'json': {
            'format': '%(asctime)s %(levelname)s %(name)s %(message)s',
            'class': 'app.logging_config.JSONFormatter'
        },
        'audit': {
            'format': '%(asctime)s [AUDIT] %(message)s',
            'class': 'app.logging_config.AuditFormatter'
        }
    },
    'handlers': {
        'default': {
            'level': 'INFO',
            'formatter': 'standard',
            'class': 'logging.StreamHandler',
            'stream': 'ext://sys.stdout'
        },
        'audit': {
            'level': 'INFO',
            'formatter': 'audit',
            'class': 'logging.FileHandler',
            'filename': '/var/log/penfold/audit.log',
            'mode': 'a',
        },
        'security': {
            'level': 'WARNING',
            'formatter': 'json',
            'class': 'logging.FileHandler',
            'filename': '/var/log/penfold/security.log',
            'mode': 'a',
        }
    },
    'loggers': {
        '': {  # root logger
            'handlers': ['default'],
            'level': 'INFO',
            'propagate': False
        },
        'app.audit': {
            'handlers': ['audit'],
            'level': 'INFO',
            'propagate': False
        },
        'app.security': {
            'handlers': ['security'],
            'level': 'WARNING',
            'propagate': False
        }
    }
}


class JSONFormatter(logging.Formatter):
    """JSON formatter for structured logging"""

    def format(self, record):
        log_entry = {
            'timestamp': datetime.utcnow().isoformat(),
            'level': record.levelname,
            'logger': record.name,
            'message': record.getMessage(),
            'module': record.module,
            'function': record.funcName,
            'line': record.lineno,
        }

        # Add extra fields if present
        if hasattr(record, 'user_id'):
            log_entry['user_id'] = record.user_id
        if hasattr(record, 'request_id'):
            log_entry['request_id'] = record.request_id
        if hasattr(record, 'action'):
            log_entry['action'] = record.action

        return json.dumps(log_entry)


class AuditFormatter(logging.Formatter):
    """Specialized formatter for audit logs"""

    def format(self, record):
        if hasattr(record, 'audit_data'):
            audit_data = record.audit_data
            return json.dumps({
                'timestamp': datetime.utcnow().isoformat(),
                'event_type': 'audit',
                **audit_data
            })
        return super().format(record)


def setup_logging():
    """Setup logging configuration"""

    # Create log directories if they don't exist
    log_dir = '/var/log/penfold'
    if not os.path.exists(log_dir):
        # Fallback to local directory in development
        log_dir = 'logs'
        os.makedirs(log_dir, exist_ok=True)

        # Update log file paths for development
        LOGGING_CONFIG['handlers']['audit']['filename'] = f'{log_dir}/audit.log'
        LOGGING_CONFIG['handlers']['security']['filename'] = f'{log_dir}/security.log'

    # Apply logging configuration
    logging.config.dictConfig(LOGGING_CONFIG)


def get_audit_logger() -> logging.Logger:
    """Get configured audit logger"""
    return logging.getLogger('app.audit')


def get_security_logger() -> logging.Logger:
    """Get configured security logger"""
    return logging.getLogger('app.security')


def log_audit_event(
    action: str,
    user_id: str,
    details: Dict[str, Any],
    request_id: str = None
):
    """Log an audit event with structured data"""

    audit_logger = get_audit_logger()

    audit_data = {
        'action': action,
        'user_id': user_id,
        'timestamp': datetime.utcnow().isoformat(),
        'details': details,
        'request_id': request_id
    }

    # Create log record with audit data
    audit_logger.info(
        f"Audit: {action} by user {user_id}",
        extra={'audit_data': audit_data}
    )


def log_security_event(
    event_type: str,
    severity: str,
    details: Dict[str, Any],
    user_id: str = None
):
    """Log a security event"""

    security_logger = get_security_logger()

    security_data = {
        'event_type': event_type,
        'severity': severity,
        'timestamp': datetime.utcnow().isoformat(),
        'details': details,
        'user_id': user_id
    }

    # Log at appropriate level based on severity
    if severity == 'critical':
        level = logging.CRITICAL
    elif severity == 'high':
        level = logging.ERROR
    elif severity == 'medium':
        level = logging.WARNING
    else:
        level = logging.INFO

    security_logger.log(
        level,
        f"Security: {event_type} - {severity} severity",
        extra={'security_data': security_data}
    )