#!/usr/bin/env python3
"""
CLI monitoring tool for Penfold observability framework.

This tool provides command-line access to agent health monitoring,
real-time status updates, and basic troubleshooting capabilities.
"""

import asyncio
import json
import sys
from datetime import datetime, timedelta
from typing import Optional, List

import click
import asyncpg
from rich.console import Console
from rich.table import Table
from rich.live import Live
from rich.panel import Panel
from rich.text import Text

# Import observability services
try:
    from observability_lib.config import settings
    from observability_lib.services.agent_health import AgentHealthService, HealthStatus
    from observability_lib.services.metrics_collector import MetricsCollector
except ImportError as e:
    click.echo(f"Error importing observability modules: {e}", err=True)
    sys.exit(1)

console = Console()

@click.group()
@click.option('--config', '-c', help='Configuration file path')
@click.option('--tenant-id', '-t', help='Tenant ID for multi-tenant setups')
@click.pass_context
def cli(ctx, config, tenant_id):
    """Penfold Observability Monitoring CLI."""
    ctx.ensure_object(dict)
    ctx.obj['config'] = config
    ctx.obj['tenant_id'] = tenant_id

@cli.command()
@click.option('--agent-id', '-a', help='Specific agent to monitor')
@click.option('--interval', '-i', default=5, help='Refresh interval in seconds')
@click.option('--format', '-f', type=click.Choice(['table', 'json']), default='table', help='Output format')
@click.pass_context
async def status(ctx, agent_id, interval, format):
    """Show real-time agent health status."""

    async def get_agent_status():
        """Get current agent status."""
        try:
            # Initialize health service
            health_service = AgentHealthService(settings)
            await health_service.start()

            if agent_id:
                # Get specific agent health
                health = await health_service.get_agent_health(agent_id, ctx.obj.get('tenant_id'))
                if health:
                    return [health]
                else:
                    console.print(f"[red]Agent '{agent_id}' not found[/red]")
                    return []
            else:
                # Get all agents health
                return await health_service.get_all_agents_health(ctx.obj.get('tenant_id'))

        except Exception as e:
            console.print(f"[red]Error getting agent status: {e}[/red]")
            return []

    def format_status_table(agents_health):
        """Format agent health as a table."""
        table = Table(title="Agent Health Status")
        table.add_column("Agent ID", style="cyan")
        table.add_column("Name", style="blue")
        table.add_column("Status", justify="center")
        table.add_column("Health Score", justify="right")
        table.add_column("Last Active", style="dim")
        table.add_column("Trend", justify="center")

        for health in agents_health:
            # Status color coding
            status_color = {
                HealthStatus.EXCELLENT: "green",
                HealthStatus.GOOD: "green",
                HealthStatus.FAIR: "yellow",
                HealthStatus.POOR: "red",
                HealthStatus.CRITICAL: "red bold"
            }.get(health.status, "white")

            # Trend indicators
            trend_indicator = {
                "improving": "↗️",
                "stable": "→",
                "declining": "↘️",
                "volatile": "↕️"
            }.get(health.trend_direction, "?")

            table.add_row(
                health.agent_id,
                health.agent_name or health.agent_id,
                f"[{status_color}]{health.status.value.upper()}[/{status_color}]",
                f"{health.health_score:.1f}",
                health.last_active.strftime("%H:%M:%S") if health.last_active else "Never",
                trend_indicator
            )

        return table

    def format_status_json(agents_health):
        """Format agent health as JSON."""
        return json.dumps([
            {
                "agent_id": h.agent_id,
                "agent_name": h.agent_name,
                "status": h.status.value,
                "health_score": h.health_score,
                "last_active": h.last_active.isoformat() if h.last_active else None,
                "trend_direction": h.trend_direction,
                "trend_slope": h.trend_slope
            }
            for h in agents_health
        ], indent=2)

    if interval > 0:
        # Real-time monitoring
        try:
            with Live(console=console, refresh_per_second=1/interval) as live:
                while True:
                    agents_health = await get_agent_status()

                    if format == 'json':
                        live.update(Text(format_status_json(agents_health)))
                    else:
                        if agents_health:
                            live.update(format_status_table(agents_health))
                        else:
                            live.update(Panel("[yellow]No agents found[/yellow]"))

                    await asyncio.sleep(interval)

        except KeyboardInterrupt:
            console.print("\n[yellow]Monitoring stopped[/yellow]")
    else:
        # One-time status
        agents_health = await get_agent_status()

        if format == 'json':
            console.print(format_status_json(agents_health))
        else:
            if agents_health:
                console.print(format_status_table(agents_health))
            else:
                console.print("[yellow]No agents found[/yellow]")

@cli.command()
@click.option('--agent-id', '-a', required=True, help='Agent to analyze')
@click.option('--hours', '-h', default=24, help='Hours of history to analyze')
@click.pass_context
async def health(ctx, agent_id, hours):
    """Get detailed health analysis for an agent."""

    try:
        health_service = AgentHealthService(settings)
        await health_service.start()

        # Get detailed health breakdown
        health_breakdown = await health_service.get_health_breakdown(
            agent_id, ctx.obj.get('tenant_id')
        )

        if not health_breakdown:
            console.print(f"[red]Agent '{agent_id}' not found[/red]")
            return

        # Create detailed health panel
        health_info = f"""
[bold cyan]Agent:[/bold cyan] {health_breakdown['agent_name']} ({agent_breakdown['agent_id']})
[bold cyan]Overall Health:[/bold cyan] {health_breakdown['health_score']:.1f}/100 ({health_breakdown['status'].upper()})
[bold cyan]Last Active:[/bold cyan] {health_breakdown['last_active'] or 'Never'}

[bold yellow]Health Components:[/bold yellow]
• Processing Completion: {health_breakdown['component_scores']['completion_rate']:.1f}/40
• Success Rate: {health_breakdown['component_scores']['success_rate']:.1f}/30
• Response Time: {health_breakdown['component_scores']['response_time']:.1f}/20
• Error Frequency: {health_breakdown['component_scores']['error_frequency']:.1f}/10

[bold yellow]Recommendations:[/bold yellow]
""" + "\n".join(f"• {rec}" for rec in health_breakdown['recommendations'])

        console.print(Panel(health_info, title=f"Health Analysis - {agent_id}", border_style="blue"))

        # Get health trends
        trends = await health_service.get_health_trends(agent_id, ctx.obj.get('tenant_id'), hours=hours)

        if trends.recent_scores:
            trend_info = f"""
[bold cyan]Trend Direction:[/bold cyan] {trends.direction.upper()}
[bold cyan]Trend Slope:[/bold cyan] {trends.slope:.2f} points/hour
[bold cyan]Volatility:[/bold cyan] {trends.volatility:.2f}
[bold cyan]Recent Scores:[/bold cyan] {[f"{s:.1f}" for s in trends.recent_scores[-10:]]}
"""
            console.print(Panel(trend_info, title="Health Trends", border_style="green"))

    except Exception as e:
        console.print(f"[red]Error analyzing agent health: {e}[/red]")

@cli.command()
@click.option('--agent-id', '-a', help='Filter by agent')
@click.option('--level', '-l', type=click.Choice(['DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL']),
              default='INFO', help='Minimum log level')
@click.option('--lines', '-n', default=20, help='Number of recent log lines')
@click.option('--follow', '-f', is_flag=True, help='Follow logs in real-time')
@click.pass_context
async def logs(ctx, agent_id, level, lines, follow):
    """View agent logs."""

    async def get_recent_logs():
        """Get recent log entries."""
        try:
            # Connect to database directly for log queries
            conn = await asyncpg.connect(settings.database_url)

            where_clause = "WHERE level >= $1"
            params = [level]

            if agent_id:
                where_clause += " AND agent_id = $2"
                params.append(agent_id)

            if ctx.obj.get('tenant_id'):
                where_clause += f" AND tenant_id = ${len(params) + 1}"
                params.append(ctx.obj['tenant_id'])

            query = f"""
                SELECT timestamp, agent_id, level, event, context
                FROM agent_logs
                {where_clause}
                ORDER BY timestamp DESC
                LIMIT {lines}
            """

            rows = await conn.fetch(query, *params)
            await conn.close()

            return rows

        except Exception as e:
            console.print(f"[red]Error fetching logs: {e}[/red]")
            return []

    def format_log_entry(row):
        """Format a single log entry."""
        level_colors = {
            'DEBUG': 'dim',
            'INFO': 'blue',
            'WARNING': 'yellow',
            'ERROR': 'red',
            'CRITICAL': 'red bold'
        }

        color = level_colors.get(row['level'], 'white')
        timestamp = row['timestamp'].strftime('%H:%M:%S')

        return f"[dim]{timestamp}[/dim] [{color}]{row['level']}[/{color}] [{row['agent_id']}] {row['event']}"

    if follow:
        # Real-time log following
        try:
            last_timestamp = datetime.utcnow()

            while True:
                logs_data = await get_recent_logs()

                # Show only new entries
                new_entries = [row for row in logs_data if row['timestamp'] > last_timestamp]

                for row in reversed(new_entries):
                    console.print(format_log_entry(row))

                if new_entries:
                    last_timestamp = max(row['timestamp'] for row in new_entries)

                await asyncio.sleep(2)

        except KeyboardInterrupt:
            console.print("\n[yellow]Log following stopped[/yellow]")
    else:
        # One-time log display
        logs_data = await get_recent_logs()

        if logs_data:
            for row in reversed(logs_data):
                console.print(format_log_entry(row))
        else:
            console.print("[yellow]No logs found[/yellow]")

@cli.command()
@click.option('--agent-id', '-a', help='Filter by agent')
@click.option('--hours', '-h', default=1, help='Hours of metrics to show')
@click.pass_context
async def metrics(ctx, agent_id, hours):
    """Show recent agent metrics."""

    try:
        # Connect to database for metrics queries
        conn = await asyncpg.connect(settings.database_url)

        where_clause = "WHERE timestamp >= NOW() - INTERVAL $1"
        params = [f"{hours} hours"]

        if agent_id:
            where_clause += " AND agent_id = $2"
            params.append(agent_id)

        if ctx.obj.get('tenant_id'):
            where_clause += f" AND tenant_id = ${len(params) + 1}"
            params.append(ctx.obj['tenant_id'])

        query = f"""
            SELECT agent_id, metric_name,
                   AVG(metric_value) as avg_value,
                   MAX(metric_value) as max_value,
                   MIN(metric_value) as min_value,
                   COUNT(*) as sample_count
            FROM agent_metrics
            {where_clause}
            GROUP BY agent_id, metric_name
            ORDER BY agent_id, metric_name
        """

        rows = await conn.fetch(query, *params)
        await conn.close()

        if rows:
            table = Table(title=f"Agent Metrics (Last {hours} hours)")
            table.add_column("Agent", style="cyan")
            table.add_column("Metric", style="blue")
            table.add_column("Avg", justify="right")
            table.add_column("Max", justify="right")
            table.add_column("Min", justify="right")
            table.add_column("Samples", justify="right", style="dim")

            for row in rows:
                table.add_row(
                    row['agent_id'],
                    row['metric_name'],
                    f"{row['avg_value']:.2f}",
                    f"{row['max_value']:.2f}",
                    f"{row['min_value']:.2f}",
                    str(row['sample_count'])
                )

            console.print(table)
        else:
            console.print(f"[yellow]No metrics found for last {hours} hours[/yellow]")

    except Exception as e:
        console.print(f"[red]Error fetching metrics: {e}[/red]")

def run_async_command(func):
    """Helper to run async click commands."""
    def wrapper(*args, **kwargs):
        return asyncio.run(func(*args, **kwargs))
    return wrapper

# Apply async wrapper to commands
status.callback = run_async_command(status.callback)
health.callback = run_async_command(health.callback)
logs.callback = run_async_command(logs.callback)
metrics.callback = run_async_command(metrics.callback)

if __name__ == '__main__':
    cli()