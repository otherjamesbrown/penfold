"""Gmail Integration Performance Dashboard.

This module provides a comprehensive performance dashboard and monitoring
interface for the Gmail integration system.
"""

import asyncio
import json
import logging
from datetime import datetime, timedelta
from typing import Dict, List, Any, Optional
from dataclasses import dataclass, asdict
from pathlib import Path
import jinja2
import aiofiles

from .monitoring import gmail_monitoring, Metric, MetricType, HealthStatus
from .optimization import gmail_cache, memory_optimizer, adaptive_rate_limiter
from ...storage.database import db_manager

logger = logging.getLogger(__name__)


@dataclass
class DashboardMetrics:
    """Container for dashboard metrics."""
    timestamp: datetime
    cache_stats: Dict[str, Any]
    database_stats: Dict[str, Any]
    memory_stats: Dict[str, Any]
    rate_limit_stats: Dict[str, Any]
    system_health: Dict[str, Any]
    performance_summary: Dict[str, Any]


class PerformanceDashboard:
    """Interactive performance dashboard for Gmail integration."""

    def __init__(self, output_dir: str = "./performance_reports"):
        """Initialize performance dashboard.

        Args:
            output_dir: Directory to save dashboard files
        """
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)

        self.metrics_history: List[DashboardMetrics] = []
        self.max_history_size = 1000

        # HTML template for dashboard
        self.template_env = jinja2.Environment(
            loader=jinja2.DictLoader({
                'dashboard.html': self._get_dashboard_template()
            })
        )

    async def collect_metrics(self) -> DashboardMetrics:
        """Collect current performance metrics from all components."""
        timestamp = datetime.utcnow()

        # Collect cache metrics
        cache_stats = gmail_cache.get_stats()
        cache_metrics = {
            'hit_rate': cache_stats.hit_rate,
            'entry_count': cache_stats.entry_count,
            'memory_usage_mb': cache_stats.memory_usage_bytes / 1024 / 1024,
            'hits': cache_stats.hits,
            'misses': cache_stats.misses,
            'evictions': cache_stats.evictions
        }

        # Collect database metrics
        try:
            db_stats = await db_manager.get_performance_stats()
            database_metrics = {
                'pool_size': db_stats.get('pool_status', {}).get('pool_size', 0),
                'active_connections': db_stats.get('pool_status', {}).get('checked_out', 0),
                'total_connections': db_stats.get('pool_status', {}).get('total_connections', 0),
                'avg_query_time': db_stats.get('query_stats', {}).get('avg_query_time', 0),
                'slow_queries': db_stats.get('query_stats', {}).get('slow_queries', 0),
                'total_queries': db_stats.get('query_stats', {}).get('total_queries', 0)
            }
        except Exception as e:
            logger.warning(f"Failed to collect database stats: {e}")
            database_metrics = {}

        # Collect memory metrics
        memory_snapshot = await memory_optimizer.monitor_memory_usage()
        memory_metrics = {
            'rss_mb': memory_snapshot.get('rss_mb', 0),
            'vms_mb': memory_snapshot.get('vms_mb', 0),
            'percent': memory_snapshot.get('percent', 0),
            'gc_objects': sum(memory_snapshot.get('gc_stats', {}).values()) if memory_snapshot.get('gc_stats') else 0
        }

        # Collect rate limiting metrics
        rate_limit_metrics = {
            'current_rate': adaptive_rate_limiter.calls_per_second,
            'adaptation_factor': adaptive_rate_limiter.adaptation_factor,
            'success_count': adaptive_rate_limiter.success_count,
            'error_count': adaptive_rate_limiter.error_count,
            'avg_response_time': (
                sum(adaptive_rate_limiter.response_times) / len(adaptive_rate_limiter.response_times)
                if adaptive_rate_limiter.response_times else 0
            )
        }

        # Collect system health metrics
        try:
            system_status = await gmail_monitoring.get_system_status()
            health_metrics = {
                'overall_status': system_status.get('overall_status', 'unknown'),
                'monitoring_active': system_status.get('monitoring_active', False),
                'recent_alerts': len(system_status.get('recent_alerts', [])),
                'health_checks': {
                    name: result.get('status', 'unknown')
                    for name, result in system_status.get('health_checks', {}).items()
                }
            }
        except Exception as e:
            logger.warning(f"Failed to collect health stats: {e}")
            health_metrics = {}

        # Calculate performance summary
        performance_summary = self._calculate_performance_summary(
            cache_metrics, database_metrics, memory_metrics, rate_limit_metrics
        )

        metrics = DashboardMetrics(
            timestamp=timestamp,
            cache_stats=cache_metrics,
            database_stats=database_metrics,
            memory_stats=memory_metrics,
            rate_limit_stats=rate_limit_metrics,
            system_health=health_metrics,
            performance_summary=performance_summary
        )

        # Add to history
        self.metrics_history.append(metrics)
        if len(self.metrics_history) > self.max_history_size:
            self.metrics_history.pop(0)

        return metrics

    def _calculate_performance_summary(
        self,
        cache_metrics: Dict[str, Any],
        database_metrics: Dict[str, Any],
        memory_metrics: Dict[str, Any],
        rate_limit_metrics: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Calculate overall performance summary."""
        # Performance score (0-100)
        score = 100.0

        # Cache performance impact
        cache_hit_rate = cache_metrics.get('hit_rate', 0)
        if cache_hit_rate < 0.8:
            score -= (0.8 - cache_hit_rate) * 30

        # Database performance impact
        avg_query_time = database_metrics.get('avg_query_time', 0)
        if avg_query_time > 0.1:  # 100ms
            score -= min(20, (avg_query_time - 0.1) * 200)

        slow_queries = database_metrics.get('slow_queries', 0)
        if slow_queries > 0:
            score -= min(15, slow_queries * 2)

        # Memory usage impact
        memory_percent = memory_metrics.get('percent', 0)
        if memory_percent > 80:
            score -= (memory_percent - 80) * 0.5

        # Rate limiting impact
        error_count = rate_limit_metrics.get('error_count', 0)
        success_count = rate_limit_metrics.get('success_count', 1)
        error_rate = error_count / (error_count + success_count)
        if error_rate > 0.05:  # 5% error rate
            score -= (error_rate - 0.05) * 100

        score = max(0, score)

        # Performance level
        if score >= 90:
            level = "Excellent"
        elif score >= 75:
            level = "Good"
        elif score >= 60:
            level = "Fair"
        elif score >= 40:
            level = "Poor"
        else:
            level = "Critical"

        return {
            'score': round(score, 1),
            'level': level,
            'cache_hit_rate': cache_hit_rate,
            'avg_query_time_ms': avg_query_time * 1000,
            'memory_usage_percent': memory_percent,
            'error_rate': error_rate
        }

    async def generate_dashboard(self, filename: str = None) -> str:
        """Generate HTML dashboard file.

        Args:
            filename: Output filename (default: performance_dashboard_<timestamp>.html)

        Returns:
            Path to generated dashboard file
        """
        if not filename:
            timestamp = datetime.utcnow().strftime("%Y%m%d_%H%M%S")
            filename = f"performance_dashboard_{timestamp}.html"

        # Collect current metrics
        current_metrics = await self.collect_metrics()

        # Prepare data for template
        template_data = {
            'current_metrics': asdict(current_metrics),
            'metrics_history': [asdict(m) for m in self.metrics_history[-100:]],  # Last 100 points
            'generated_at': datetime.utcnow().isoformat(),
            'total_metrics_collected': len(self.metrics_history)
        }

        # Render template
        template = self.template_env.get_template('dashboard.html')
        html_content = template.render(**template_data)

        # Write to file
        output_path = self.output_dir / filename
        async with aiofiles.open(output_path, 'w') as f:
            await f.write(html_content)

        logger.info(f"Performance dashboard generated: {output_path}")
        return str(output_path)

    async def export_metrics_json(self, filename: str = None) -> str:
        """Export metrics to JSON format.

        Args:
            filename: Output filename (default: metrics_<timestamp>.json)

        Returns:
            Path to exported JSON file
        """
        if not filename:
            timestamp = datetime.utcnow().strftime("%Y%m%d_%H%M%S")
            filename = f"metrics_{timestamp}.json"

        # Prepare data for export
        export_data = {
            'export_timestamp': datetime.utcnow().isoformat(),
            'metrics_count': len(self.metrics_history),
            'metrics': [
                {
                    **asdict(m),
                    'timestamp': m.timestamp.isoformat()
                }
                for m in self.metrics_history
            ]
        }

        # Write JSON file
        output_path = self.output_dir / filename
        async with aiofiles.open(output_path, 'w') as f:
            await f.write(json.dumps(export_data, indent=2))

        logger.info(f"Metrics exported to JSON: {output_path}")
        return str(output_path)

    def get_performance_trends(self, hours: int = 24) -> Dict[str, Any]:
        """Analyze performance trends over time.

        Args:
            hours: Number of hours to analyze

        Returns:
            Performance trend analysis
        """
        cutoff_time = datetime.utcnow() - timedelta(hours=hours)
        recent_metrics = [m for m in self.metrics_history if m.timestamp > cutoff_time]

        if len(recent_metrics) < 2:
            return {'error': 'Insufficient data for trend analysis'}

        # Extract trend data
        timestamps = [m.timestamp for m in recent_metrics]
        scores = [m.performance_summary['score'] for m in recent_metrics]
        cache_hit_rates = [m.cache_stats.get('hit_rate', 0) for m in recent_metrics]
        memory_usage = [m.memory_stats.get('percent', 0) for m in recent_metrics]
        query_times = [m.database_stats.get('avg_query_time', 0) for m in recent_metrics]

        def calculate_trend(values: List[float]) -> str:
            """Calculate trend direction."""
            if len(values) < 2:
                return 'stable'

            first_half = values[:len(values)//2]
            second_half = values[len(values)//2:]

            avg_first = sum(first_half) / len(first_half)
            avg_second = sum(second_half) / len(second_half)

            diff_percent = ((avg_second - avg_first) / avg_first * 100) if avg_first > 0 else 0

            if diff_percent > 5:
                return 'improving'
            elif diff_percent < -5:
                return 'degrading'
            else:
                return 'stable'

        return {
            'time_period_hours': hours,
            'data_points': len(recent_metrics),
            'trends': {
                'performance_score': {
                    'current': scores[-1] if scores else 0,
                    'average': sum(scores) / len(scores) if scores else 0,
                    'min': min(scores) if scores else 0,
                    'max': max(scores) if scores else 0,
                    'trend': calculate_trend(scores)
                },
                'cache_hit_rate': {
                    'current': cache_hit_rates[-1] if cache_hit_rates else 0,
                    'average': sum(cache_hit_rates) / len(cache_hit_rates) if cache_hit_rates else 0,
                    'trend': calculate_trend(cache_hit_rates)
                },
                'memory_usage': {
                    'current': memory_usage[-1] if memory_usage else 0,
                    'average': sum(memory_usage) / len(memory_usage) if memory_usage else 0,
                    'max': max(memory_usage) if memory_usage else 0,
                    'trend': calculate_trend(memory_usage)
                },
                'query_performance': {
                    'current_ms': query_times[-1] * 1000 if query_times else 0,
                    'average_ms': sum(query_times) / len(query_times) * 1000 if query_times else 0,
                    'max_ms': max(query_times) * 1000 if query_times else 0,
                    'trend': calculate_trend([q * -1 for q in query_times])  # Inverted - lower is better
                }
            }
        }

    def _get_dashboard_template(self) -> str:
        """Get HTML template for dashboard."""
        return """
<!DOCTYPE html>
<html>
<head>
    <title>Gmail Integration Performance Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .header {
            background: white;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .metric-card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .metric-value {
            font-size: 2em;
            font-weight: bold;
            margin: 10px 0;
        }
        .metric-label {
            color: #666;
            font-size: 0.9em;
        }
        .status-excellent { color: #28a745; }
        .status-good { color: #20c997; }
        .status-fair { color: #ffc107; }
        .status-poor { color: #fd7e14; }
        .status-critical { color: #dc3545; }
        .chart-container {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        .health-status {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.8em;
            font-weight: bold;
            text-transform: uppercase;
        }
        .health-healthy { background: #d4edda; color: #155724; }
        .health-degraded { background: #fff3cd; color: #856404; }
        .health-unhealthy { background: #f8d7da; color: #721c24; }
        .health-critical { background: #dc3545; color: white; }
        .timestamp {
            color: #6c757d;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Gmail Integration Performance Dashboard</h1>
            <p class="timestamp">Generated: {{ generated_at }} | Total Metrics: {{ total_metrics_collected }}</p>
        </div>

        <div class="metrics-grid">
            <div class="metric-card">
                <div class="metric-label">Performance Score</div>
                <div class="metric-value status-{{ current_metrics.performance_summary.level.lower() }}">
                    {{ current_metrics.performance_summary.score }}%
                </div>
                <div class="metric-label">{{ current_metrics.performance_summary.level }}</div>
            </div>

            <div class="metric-card">
                <div class="metric-label">Cache Hit Rate</div>
                <div class="metric-value">{{ "%.1f"|format(current_metrics.cache_stats.hit_rate * 100) }}%</div>
                <div class="metric-label">{{ current_metrics.cache_stats.hits }} hits, {{ current_metrics.cache_stats.misses }} misses</div>
            </div>

            <div class="metric-card">
                <div class="metric-label">Memory Usage</div>
                <div class="metric-value">{{ "%.1f"|format(current_metrics.memory_stats.rss_mb) }} MB</div>
                <div class="metric-label">{{ "%.1f"|format(current_metrics.memory_stats.percent) }}% of system memory</div>
            </div>

            <div class="metric-card">
                <div class="metric-label">Database Health</div>
                <div class="metric-value">
                    <span class="health-status health-{{ current_metrics.system_health.health_checks.database|default('unknown') }}">
                        {{ current_metrics.system_health.health_checks.database|default('Unknown') }}
                    </span>
                </div>
                <div class="metric-label">{{ current_metrics.database_stats.active_connections|default(0) }}/{{ current_metrics.database_stats.pool_size|default(0) }} connections</div>
            </div>

            <div class="metric-card">
                <div class="metric-label">API Rate Limit</div>
                <div class="metric-value">{{ current_metrics.rate_limit_stats.current_rate }} req/s</div>
                <div class="metric-label">Adaptation: {{ "%.2f"|format(current_metrics.rate_limit_stats.adaptation_factor) }}x</div>
            </div>

            <div class="metric-card">
                <div class="metric-label">Query Performance</div>
                <div class="metric-value">{{ "%.1f"|format(current_metrics.performance_summary.avg_query_time_ms) }} ms</div>
                <div class="metric-label">Average query time</div>
            </div>
        </div>

        {% if metrics_history|length > 1 %}
        <div class="chart-container">
            <h3>Performance Trends</h3>
            <canvas id="performanceChart" width="800" height="400"></canvas>
        </div>
        {% endif %}

        <div class="chart-container">
            <h3>System Status</h3>
            <p><strong>Overall Status:</strong>
                <span class="health-status health-{{ current_metrics.system_health.overall_status|default('unknown') }}">
                    {{ current_metrics.system_health.overall_status|default('Unknown') }}
                </span>
            </p>
            <p><strong>Monitoring Active:</strong> {{ current_metrics.system_health.monitoring_active }}</p>
            <p><strong>Recent Alerts:</strong> {{ current_metrics.system_health.recent_alerts }}</p>

            <h4>Health Checks</h4>
            <ul>
            {% for check_name, status in current_metrics.system_health.health_checks.items() %}
                <li>{{ check_name }}: <span class="health-status health-{{ status }}">{{ status }}</span></li>
            {% endfor %}
            </ul>
        </div>
    </div>

    {% if metrics_history|length > 1 %}
    <script>
        const ctx = document.getElementById('performanceChart').getContext('2d');
        const chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: [
                    {% for metric in metrics_history %}
                    '{{ metric.timestamp }}',
                    {% endfor %}
                ],
                datasets: [
                    {
                        label: 'Performance Score',
                        data: [
                            {% for metric in metrics_history %}
                            {{ metric.performance_summary.score }},
                            {% endfor %}
                        ],
                        borderColor: 'rgb(75, 192, 192)',
                        tension: 0.1,
                        yAxisID: 'y'
                    },
                    {
                        label: 'Cache Hit Rate (%)',
                        data: [
                            {% for metric in metrics_history %}
                            {{ metric.cache_stats.hit_rate * 100 }},
                            {% endfor %}
                        ],
                        borderColor: 'rgb(255, 99, 132)',
                        tension: 0.1,
                        yAxisID: 'y'
                    },
                    {
                        label: 'Memory Usage (MB)',
                        data: [
                            {% for metric in metrics_history %}
                            {{ metric.memory_stats.rss_mb }},
                            {% endfor %}
                        ],
                        borderColor: 'rgb(54, 162, 235)',
                        tension: 0.1,
                        yAxisID: 'y1'
                    }
                ]
            },
            options: {
                responsive: true,
                interaction: {
                    mode: 'index',
                    intersect: false,
                },
                scales: {
                    x: {
                        display: true,
                        title: {
                            display: true,
                            text: 'Time'
                        }
                    },
                    y: {
                        type: 'linear',
                        display: true,
                        position: 'left',
                        title: {
                            display: true,
                            text: 'Score / Percentage'
                        }
                    },
                    y1: {
                        type: 'linear',
                        display: true,
                        position: 'right',
                        title: {
                            display: true,
                            text: 'Memory (MB)'
                        },
                        grid: {
                            drawOnChartArea: false,
                        },
                    }
                }
            }
        });
    </script>
    {% endif %}
</body>
</html>
        """


# Global dashboard instance
performance_dashboard = PerformanceDashboard()


async def generate_performance_report() -> Dict[str, str]:
    """Generate complete performance report with dashboard and metrics export.

    Returns:
        Dictionary with paths to generated files
    """
    # Generate dashboard
    dashboard_path = await performance_dashboard.generate_dashboard()

    # Export metrics
    metrics_path = await performance_dashboard.export_metrics_json()

    # Get trends analysis
    trends = performance_dashboard.get_performance_trends(hours=24)

    logger.info(f"Performance report generated: dashboard={dashboard_path}, metrics={metrics_path}")

    return {
        'dashboard_path': dashboard_path,
        'metrics_path': metrics_path,
        'trends': trends
    }


async def start_dashboard_monitoring(interval_minutes: int = 5):
    """Start background dashboard monitoring.

    Args:
        interval_minutes: Collection interval in minutes
    """
    async def monitoring_loop():
        while True:
            try:
                await performance_dashboard.collect_metrics()
                logger.debug("Dashboard metrics collected")
                await asyncio.sleep(interval_minutes * 60)
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Dashboard monitoring error: {e}")
                await asyncio.sleep(60)  # Wait 1 minute on error

    task = asyncio.create_task(monitoring_loop())
    logger.info(f"Started dashboard monitoring with {interval_minutes} minute intervals")
    return task