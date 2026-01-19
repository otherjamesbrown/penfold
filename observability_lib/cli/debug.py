#!/usr/bin/env python3
"""
Workflow debugging CLI tool for Penfold observability framework.

This tool provides command-line access to workflow tracing, analysis,
and debugging capabilities for troubleshooting agent processing pipelines.
"""

import asyncio
import json
import sys
from datetime import datetime, timedelta
from typing import Optional, List, Dict, Any

import click
import asyncpg
from rich.console import Console
from rich.table import Table
from rich.tree import Tree
from rich.panel import Panel
from rich.text import Text
from rich.progress import track

# Import observability services
try:
    from observability_lib.config import settings
    from observability_lib.services.workflow_tracker import WorkflowTracker
    from observability_lib.services.workflow_analyzer import WorkflowAnalyzer
    from observability_lib.models.agent_metrics import WorkflowEvent, EventType, EventStatus
except ImportError as e:
    click.echo(f"Error importing observability modules: {e}", err=True)
    sys.exit(1)

console = Console()

@click.group()
@click.option('--config', '-c', help='Configuration file path')
@click.option('--tenant-id', '-t', help='Tenant ID for multi-tenant setups')
@click.pass_context
def cli(ctx, config, tenant_id):
    """Penfold Workflow Debugging CLI."""
    ctx.ensure_object(dict)
    ctx.obj['config'] = config
    ctx.obj['tenant_id'] = tenant_id

@cli.command()
@click.option('--agent-id', '-a', help='Filter by agent')
@click.option('--status', '-s', type=click.Choice(['running', 'completed', 'failed', 'timeout']), help='Filter by status')
@click.option('--hours', '-h', default=24, help='Hours of history to show')
@click.option('--limit', '-l', default=20, help='Maximum number of workflows to show')
@click.option('--format', '-f', type=click.Choice(['table', 'json']), default='table', help='Output format')
@click.pass_context
async def list_workflows(ctx, agent_id, status, hours, limit, format):
    """List recent workflows with status and performance summary."""

    try:
        # Connect to database
        conn = await asyncpg.connect(settings.database_url)

        # Build query with filters
        where_conditions = ["timestamp >= NOW() - INTERVAL $1"]
        params = [f"{hours} hours"]
        param_count = 1

        if agent_id:
            param_count += 1
            where_conditions.append(f"agent_id = ${param_count}")
            params.append(agent_id)

        if ctx.obj.get('tenant_id'):
            param_count += 1
            where_conditions.append(f"tenant_id = ${param_count}")
            params.append(ctx.obj['tenant_id'])

        # Query for workflow summaries
        query = f"""
            WITH workflow_summary AS (
                SELECT
                    workflow_id,
                    MIN(timestamp) as start_time,
                    MAX(timestamp) as end_time,
                    array_agg(DISTINCT agent_id) as agents,
                    COUNT(*) as event_count,
                    COUNT(*) FILTER (WHERE event_status = 'success') as success_events,
                    COUNT(*) FILTER (WHERE event_status = 'failure') as failed_events,
                    MAX(CASE WHEN event_type = 'workflow_finished' THEN event_status END) as final_status
                FROM workflow_events
                WHERE {' AND '.join(where_conditions)}
                GROUP BY workflow_id
                ORDER BY start_time DESC
                LIMIT {limit}
            )
            SELECT
                workflow_id,
                start_time,
                end_time,
                agents,
                event_count,
                success_events,
                failed_events,
                final_status,
                EXTRACT(EPOCH FROM (end_time - start_time)) * 1000 as duration_ms
            FROM workflow_summary
        """

        workflows = await conn.fetch(query, *params)
        await conn.close()

        if format == 'json':
            json_data = []
            for wf in workflows:
                json_data.append({
                    'workflow_id': str(wf['workflow_id']),
                    'start_time': wf['start_time'].isoformat(),
                    'end_time': wf['end_time'].isoformat() if wf['end_time'] else None,
                    'agents': wf['agents'],
                    'event_count': wf['event_count'],
                    'success_events': wf['success_events'],
                    'failed_events': wf['failed_events'],
                    'final_status': wf['final_status'],
                    'duration_ms': wf['duration_ms']
                })
            console.print(json.dumps(json_data, indent=2))
        else:
            if workflows:
                table = Table(title=f"Recent Workflows (Last {hours}h)")
                table.add_column("Workflow ID", style="cyan", width=36)
                table.add_column("Agents", style="blue")
                table.add_column("Status", justify="center")
                table.add_column("Duration", justify="right")
                table.add_column("Events", justify="right", style="dim")
                table.add_column("Started", style="dim")

                for wf in workflows:
                    # Determine status and color
                    if wf['final_status'] == 'success':
                        status_text = "[green]COMPLETED[/green]"
                    elif wf['final_status'] == 'failure':
                        status_text = "[red]FAILED[/red]"
                    elif wf['failed_events'] > 0:
                        status_text = "[yellow]PARTIAL[/yellow]"
                    elif wf['end_time']:
                        status_text = "[blue]FINISHED[/blue]"
                    else:
                        status_text = "[yellow]RUNNING[/yellow]"

                    # Format duration
                    if wf['duration_ms']:
                        if wf['duration_ms'] < 1000:
                            duration = f"{wf['duration_ms']:.0f}ms"
                        elif wf['duration_ms'] < 60000:
                            duration = f"{wf['duration_ms']/1000:.1f}s"
                        else:
                            duration = f"{wf['duration_ms']/60000:.1f}m"
                    else:
                        duration = "ongoing"

                    # Format agents list
                    agents_text = ", ".join(wf['agents'][:3])
                    if len(wf['agents']) > 3:
                        agents_text += f" +{len(wf['agents'])-3}"

                    table.add_row(
                        str(wf['workflow_id'])[-8:],  # Show last 8 chars
                        agents_text,
                        status_text,
                        duration,
                        f"{wf['success_events']}/{wf['event_count']}",
                        wf['start_time'].strftime("%H:%M:%S")
                    )

                console.print(table)
            else:
                console.print(f"[yellow]No workflows found in last {hours} hours[/yellow]")

    except Exception as e:
        console.print(f"[red]Error fetching workflows: {e}[/red]")

@cli.command()
@click.argument('workflow_id')
@click.option('--include-context', '-c', is_flag=True, help='Include full event context')
@click.pass_context
async def trace(ctx, workflow_id, include_context):
    """Show detailed workflow trace with timeline and events."""

    try:
        # Connect to database
        conn = await asyncpg.connect(settings.database_url)

        # Query workflow events
        where_conditions = ["workflow_id = $1"]
        params = [workflow_id]

        if ctx.obj.get('tenant_id'):
            where_conditions.append("tenant_id = $2")
            params.append(ctx.obj['tenant_id'])

        query = f"""
            SELECT
                timestamp, agent_id, event_type, event_status,
                event_data, processing_time_ms
            FROM workflow_events
            WHERE {' AND '.join(where_conditions)}
            ORDER BY timestamp ASC
        """

        events = await conn.fetch(query, *params)

        if not events:
            console.print(f"[red]Workflow {workflow_id} not found[/red]")
            return

        # Create timeline visualization
        tree = Tree(f"[bold cyan]Workflow Trace: {workflow_id}[/bold cyan]")

        start_time = events[0]['timestamp']
        total_duration = (events[-1]['timestamp'] - start_time).total_seconds() * 1000

        for i, event in enumerate(events):
            elapsed = (event['timestamp'] - start_time).total_seconds() * 1000

            # Format event type and status
            event_color = {
                'workflow_started': 'green',
                'stage_completed': 'blue',
                'handoff': 'yellow',
                'workflow_finished': 'green',
                'error': 'red'
            }.get(event['event_type'], 'white')

            status_color = {
                'success': 'green',
                'failure': 'red',
                'timeout': 'orange',
                'retry': 'yellow'
            }.get(event['event_status'], 'white')

            # Create event description
            event_desc = f"[{event_color}]{event['event_type']}[/{event_color}] "
            event_desc += f"[{status_color}]({event['event_status']})[/{status_color}] "
            event_desc += f"[cyan]{event['agent_id']}[/cyan] "
            event_desc += f"[dim]+{elapsed:.0f}ms"

            if event['processing_time_ms']:
                event_desc += f" ({event['processing_time_ms']:.0f}ms)[/dim]"
            else:
                event_desc += "[/dim]"

            event_node = tree.add(event_desc)

            # Add context if requested
            if include_context and event['event_data']:
                context_text = json.dumps(event['event_data'], indent=2)
                if len(context_text) > 200:
                    context_text = context_text[:200] + "..."
                event_node.add(f"[dim]{context_text}[/dim]")

        console.print(tree)

        # Summary panel
        summary = f"""
[bold cyan]Workflow Summary:[/bold cyan]
• Total Duration: {total_duration:.0f}ms
• Events: {len(events)}
• Agents Involved: {len(set(e['agent_id'] for e in events))}
• Status: {events[-1]['event_status'].upper()}
"""

        console.print(Panel(summary, title="Summary", border_style="blue"))

        await conn.close()

    except Exception as e:
        console.print(f"[red]Error fetching workflow trace: {e}[/red]")

@cli.command()
@click.argument('workflow_id')
@click.pass_context
async def analyze(ctx, workflow_id):
    """Analyze workflow performance and identify bottlenecks."""

    try:
        # Initialize workflow analyzer
        analyzer = WorkflowAnalyzer(settings)

        # Get workflow analysis
        analysis = await analyzer.analyze_workflow_performance(workflow_id, ctx.obj.get('tenant_id'))

        if not analysis:
            console.print(f"[red]Workflow {workflow_id} not found[/red]")
            return

        # Performance analysis panel
        perf_info = f"""
[bold cyan]Performance Analysis:[/bold cyan]
• Total Duration: {analysis.total_duration_ms:.0f}ms
• Agent Count: {len(analysis.agents_involved)}
• Stage Count: {len(analysis.stage_timings)}
• Success Rate: {analysis.success_rate:.1%}

[bold yellow]Stage Performance:[/bold yellow]
"""

        for stage in analysis.stage_timings[:5]:  # Show top 5 slowest stages
            percentage = (stage['duration_ms'] / analysis.total_duration_ms) * 100
            perf_info += f"• {stage['stage_name']}: {stage['duration_ms']:.0f}ms ({percentage:.1f}%)\n"

        if analysis.bottlenecks:
            perf_info += f"\n[bold red]Bottlenecks Identified:[/bold red]\n"
            for bottleneck in analysis.bottlenecks[:3]:
                perf_info += f"• {bottleneck['description']}\n"

        if analysis.recommendations:
            perf_info += f"\n[bold green]Recommendations:[/bold green]\n"
            for rec in analysis.recommendations[:3]:
                perf_info += f"• {rec}\n"

        console.print(Panel(perf_info, title=f"Analysis - {workflow_id}", border_style="blue"))

    except Exception as e:
        console.print(f"[red]Error analyzing workflow: {e}[/red]")

@cli.command()
@click.option('--hours', '-h', default=24, help='Hours of history to analyze')
@click.option('--agent-id', '-a', help='Filter by agent')
@click.option('--limit', '-l', default=10, help='Number of bottlenecks to show')
@click.pass_context
async def bottlenecks(ctx, hours, agent_id, limit):
    """Identify system-wide bottlenecks and performance issues."""

    try:
        # Initialize workflow analyzer
        analyzer = WorkflowAnalyzer(settings)

        # Get bottlenecks analysis
        bottlenecks = await analyzer.identify_bottlenecks(
            hours_back=hours,
            agent_id=agent_id,
            tenant_id=ctx.obj.get('tenant_id')
        )

        if bottlenecks:
            table = Table(title=f"System Bottlenecks (Last {hours}h)")
            table.add_column("Bottleneck Type", style="cyan")
            table.add_column("Impact", style="red")
            table.add_column("Frequency", justify="right", style="yellow")
            table.add_column("Avg Duration", justify="right")
            table.add_column("Recommendation", style="green")

            for bottleneck in bottlenecks[:limit]:
                impact_color = {
                    'Critical': 'red bold',
                    'High': 'red',
                    'Moderate': 'yellow',
                    'Low': 'blue'
                }.get(bottleneck.severity, 'white')

                table.add_row(
                    bottleneck.bottleneck_type,
                    f"[{impact_color}]{bottleneck.severity}[/{impact_color}]",
                    str(bottleneck.frequency),
                    f"{bottleneck.avg_duration_ms:.0f}ms",
                    bottleneck.recommendation[:50] + "..." if len(bottleneck.recommendation) > 50 else bottleneck.recommendation
                )

            console.print(table)
        else:
            console.print("[green]No significant bottlenecks detected[/green]")

    except Exception as e:
        console.print(f"[red]Error analyzing bottlenecks: {e}[/red]")

@cli.command()
@click.option('--follow', '-f', is_flag=True, help='Follow workflows in real-time')
@click.option('--agent-id', '-a', help='Filter by agent')
@click.pass_context
async def stream(ctx, follow, agent_id):
    """Stream live workflow events."""

    if not follow:
        console.print("[yellow]Use --follow to enable real-time streaming[/yellow]")
        return

    try:
        console.print("[green]Streaming live workflow events... (Ctrl+C to stop)[/green]")

        # Initialize workflow tracker for streaming
        tracker = WorkflowTracker(settings)

        # Simple polling implementation (in production, use SSE or websockets)
        last_timestamp = datetime.utcnow()

        while True:
            # Connect to database
            conn = await asyncpg.connect(settings.database_url)

            where_conditions = ["timestamp > $1"]
            params = [last_timestamp]

            if agent_id:
                where_conditions.append("agent_id = $2")
                params.append(agent_id)

            if ctx.obj.get('tenant_id'):
                where_conditions.append(f"tenant_id = ${len(params) + 1}")
                params.append(ctx.obj['tenant_id'])

            query = f"""
                SELECT timestamp, workflow_id, agent_id, event_type, event_status, processing_time_ms
                FROM workflow_events
                WHERE {' AND '.join(where_conditions)}
                ORDER BY timestamp ASC
                LIMIT 10
            """

            events = await conn.fetch(query, *params)
            await conn.close()

            for event in events:
                event_color = {
                    'workflow_started': 'green',
                    'stage_completed': 'blue',
                    'handoff': 'yellow',
                    'workflow_finished': 'green',
                    'error': 'red'
                }.get(event['event_type'], 'white')

                status_color = {
                    'success': 'green',
                    'failure': 'red',
                    'timeout': 'orange',
                    'retry': 'yellow'
                }.get(event['event_status'], 'white')

                event_line = f"[dim]{event['timestamp'].strftime('%H:%M:%S')}[/dim] "
                event_line += f"[cyan]{str(event['workflow_id'])[-8:]}[/cyan] "
                event_line += f"[{event_color}]{event['event_type']}[/{event_color}] "
                event_line += f"[{status_color}]({event['event_status']})[/{status_color}] "
                event_line += f"[blue]{event['agent_id']}[/blue]"

                if event['processing_time_ms']:
                    event_line += f" [dim]{event['processing_time_ms']:.0f}ms[/dim]"

                console.print(event_line)

                last_timestamp = max(last_timestamp, event['timestamp'])

            await asyncio.sleep(2)  # Poll every 2 seconds

    except KeyboardInterrupt:
        console.print("\n[yellow]Streaming stopped[/yellow]")
    except Exception as e:
        console.print(f"[red]Error streaming workflows: {e}[/red]")

def run_async_command(func):
    """Helper to run async click commands."""
    def wrapper(*args, **kwargs):
        return asyncio.run(func(*args, **kwargs))
    return wrapper

# Apply async wrapper to commands
list_workflows.callback = run_async_command(list_workflows.callback)
trace.callback = run_async_command(trace.callback)
analyze.callback = run_async_command(analyze.callback)
bottlenecks.callback = run_async_command(bottlenecks.callback)
stream.callback = run_async_command(stream.callback)

if __name__ == '__main__':
    cli()