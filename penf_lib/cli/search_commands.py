"""Search CLI commands for Penfold.

This module implements the `penf search` command group for the search interface:
- query: Natural language search across all content sources
- history: Show recent search queries
- suggest: Get query suggestions based on context
- correlate: Find related content across sources

Commands:
    - penf search query: Execute a natural language search
    - penf search history: View recent search queries
    - penf search suggest: Get AI-powered query suggestions
    - penf search correlate: Find related content across sources
"""

from __future__ import annotations

import asyncio
import json
import uuid
from datetime import datetime
from typing import Optional, Tuple

import click
from rich.console import Console
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from penf_lib.search.analytics import AnalyticsCollector
from penf_lib.search.models import (
    ContentTypeFilter,
    RelatedContentResponse,
    SearchQuery,
    SearchResponse,
    TemporalConstraint,
)
from penf_lib.search.search_engine import SearchEngine
from penf_lib.search.suggestions import SuggestionEngine
from penf_lib.storage.connections import cleanup_connections, get_session
from penf_lib.storage.repositories.search import SearchRepository

console = Console()


def run_async(coro):
    """Run an async coroutine synchronously."""
    return asyncio.run(coro)


# =============================================================================
# OUTPUT FORMATTING HELPERS
# =============================================================================


def _print_pretty_results(response: SearchResponse) -> None:
    """Print results in rich formatted output."""
    console.print(Panel(
        f"Found [bold]{response.metadata.total_results}[/bold] results "
        f"in {response.metadata.execution_time_ms}ms "
        f"({'cached' if response.metadata.cache_hit else 'fresh'})",
        title="Search Results",
    ))

    for i, result in enumerate(response.results, 1):
        _print_result_card(i, result)

    if response.has_more:
        console.print("\n[dim]More results available. Use --limit to see more.[/dim]")


def _print_result_card(index: int, result) -> None:
    """Print single result as a card."""
    # Content type badge
    type_colors = {
        "email": "blue",
        "meeting": "green",
        "document": "yellow",
        "slack": "magenta",
    }
    type_color = type_colors.get(result.content_type.value, "white")

    # Build header
    header = Text()
    header.append(f"[{index}] ", style="bold")
    header.append(f"[{result.content_type.value}] ", style=type_color)
    if result.preview.title:
        header.append(result.preview.title, style="bold")

    console.print(header)
    console.print(f"    {result.preview.snippet}", style="dim")
    console.print(
        f"    [dim italic]{result.timestamp.strftime('%Y-%m-%d %H:%M')} "
        f"| Score: {result.relevance_score:.2f}[/dim italic]"
    )
    if result.participants:
        participants_display = ", ".join(result.participants[:3])
        if len(result.participants) > 3:
            participants_display += f" (+{len(result.participants) - 3} more)"
        console.print(f"    [dim]{participants_display}[/dim]")
    console.print()


def _print_compact_results(response: SearchResponse) -> None:
    """Print results in compact table format."""
    table = Table(show_header=True, header_style="bold")
    table.add_column("#", width=3)
    table.add_column("Type", width=8)
    table.add_column("Title", width=40)
    table.add_column("Date", width=12)
    table.add_column("Score", width=6)

    for i, result in enumerate(response.results, 1):
        title = result.preview.title or result.preview.snippet
        table.add_row(
            str(i),
            result.content_type.value,
            title[:40] if title else "",
            result.timestamp.strftime("%Y-%m-%d"),
            f"{result.relevance_score:.2f}",
        )

    console.print(table)
    console.print(
        f"\n{response.metadata.total_results} results, "
        f"{response.metadata.execution_time_ms}ms"
    )


# =============================================================================
# SEARCH COMMAND GROUP
# =============================================================================


@click.group(name="search")
@click.pass_context
def search_group(ctx: click.Context):
    """Search across all content sources.

    Use natural language to search emails, documents, meetings,
    and other content across all connected sources.
    """
    pass


# =============================================================================
# SEARCH QUERY COMMAND
# =============================================================================


@search_group.command(name="query")
@click.argument("query_text")
@click.option(
    "--type", "-t",
    "content_types",
    multiple=True,
    type=click.Choice(["email", "meeting", "document", "slack", "all"]),
    help="Content types to search (can specify multiple)"
)
@click.option(
    "--exclude", "-x",
    "exclude_types",
    multiple=True,
    type=click.Choice(["email", "meeting", "document", "slack"]),
    help="Content types to exclude from search (can specify multiple)"
)
@click.option(
    "--participant", "-p",
    "participants",
    multiple=True,
    help="Filter by participant email/name (can specify multiple, matches ANY)"
)
@click.option(
    "--project",
    "projects",
    multiple=True,
    help="Filter by project reference (can specify multiple, matches ANY)"
)
@click.option(
    "--min-confidence",
    type=float,
    help="Minimum AI confidence score (0.0-1.0)"
)
@click.option(
    "--since",
    type=click.DateTime(),
    help="Filter results from this date/time onwards"
)
@click.option(
    "--until",
    type=click.DateTime(),
    help="Filter results until this date/time"
)
@click.option(
    "--limit", "-l",
    type=int,
    default=25,
    show_default=True,
    help="Maximum number of results to return"
)
@click.option(
    "--format", "-f",
    "output_format",
    type=click.Choice(["pretty", "json", "compact"]),
    default="pretty",
    show_default=True,
    help="Output format"
)
@click.pass_context
def search_query(
    ctx: click.Context,
    query_text: str,
    content_types: Tuple[str, ...],
    exclude_types: Tuple[str, ...],
    participants: Tuple[str, ...],
    projects: Tuple[str, ...],
    min_confidence: Optional[float],
    since: Optional[datetime],
    until: Optional[datetime],
    limit: int,
    output_format: str,
):
    """Search across all content with natural language query.

    Example:
        penf search query "customer deployment issues"
        penf search query "meeting about Atlas" --type meeting --limit 10
        penf search query "budget discussions" --format json
        penf search query "project updates" --participant alice@example.com
        penf search query "deployment" --project Atlas --min-confidence 0.8
        penf search query "all meetings" --exclude slack --exclude document
    """
    # Validate min_confidence if provided
    if min_confidence is not None and not 0.0 <= min_confidence <= 1.0:
        console.print("[red]Error: --min-confidence must be between 0.0 and 1.0[/red]")
        raise click.Abort()

    async def run_search():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            engine = SearchEngine(session, tenant_id)

            # Build content type filters
            if content_types:
                types = [ContentTypeFilter(t) for t in content_types]
            else:
                types = [ContentTypeFilter.ALL]

            # Handle exclusions by removing from the type list
            if exclude_types:
                exclude_set = set(exclude_types)
                if ContentTypeFilter.ALL in types:
                    # If ALL was requested, replace with all types except excluded
                    all_types = [
                        ContentTypeFilter.EMAIL,
                        ContentTypeFilter.MEETING,
                        ContentTypeFilter.DOCUMENT,
                        ContentTypeFilter.SLACK,
                    ]
                    types = [t for t in all_types if t.value not in exclude_set]
                else:
                    types = [t for t in types if t.value not in exclude_set]

            # Build temporal constraint if provided
            temporal = None
            if since or until:
                temporal = TemporalConstraint(
                    start_date=since,
                    end_date=until,
                )

            # Build query with all filter options
            query = SearchQuery(
                query_text=query_text,
                content_types=types,
                temporal=temporal,
                participants=list(participants) if participants else None,
                projects=list(projects) if projects else None,
                min_confidence=min_confidence,
                limit=limit,
            )

            # Execute search
            response = await engine.search(query)

            # Output results
            if output_format == "json":
                console.print_json(response.model_dump_json())
            elif output_format == "compact":
                _print_compact_results(response)
            else:
                _print_pretty_results(response)

    try:
        run_async(run_search())
    except Exception as e:
        console.print(f"[red]Search failed: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


# =============================================================================
# SEARCH HISTORY COMMAND
# =============================================================================


@search_group.command(name="history")
@click.option(
    "--limit", "-l",
    type=int,
    default=10,
    show_default=True,
    help="Number of recent queries to show"
)
@click.option(
    "--format", "-f",
    "output_format",
    type=click.Choice(["table", "json"]),
    default="table",
    show_default=True,
    help="Output format"
)
@click.pass_context
def search_history(
    ctx: click.Context,
    limit: int,
    output_format: str,
):
    """Show recent search queries.

    Display your search history with timestamps and result counts.
    Useful for re-running previous searches or tracking what you've
    looked for.

    Examples:

        penf search history

        penf search history -l 20 -f json
    """
    async def _history():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            repo = SearchRepository(session)

            # Convert tenant_id string to UUID
            try:
                tenant_uuid = uuid.UUID(tenant_id)
            except ValueError:
                # Use a deterministic UUID from the tenant name
                tenant_uuid = uuid.uuid5(uuid.NAMESPACE_DNS, tenant_id)

            queries = await repo.get_recent_queries(
                tenant_id=tenant_uuid,
                limit=limit,
            )

            if not queries:
                console.print("[dim]No search history found.[/dim]")
                return 0

            if output_format == "json":
                # Convert to JSON-serializable format
                json_queries = []
                for q in queries:
                    json_queries.append({
                        "id": str(q["id"]),
                        "query_text": q["query_text"],
                        "result_count": q["result_count"],
                        "execution_time_ms": q["execution_time_ms"],
                        "cache_hit": q["cache_hit"],
                        "search_strategy": q["search_strategy"],
                        "created_at": q["created_at"].isoformat() if q["created_at"] else None,
                    })
                console.print_json(json.dumps(json_queries, indent=2))
            else:
                # Table format
                table = Table(
                    title="Recent Search Queries",
                    show_header=True,
                    header_style="bold",
                )
                table.add_column("#", width=3)
                table.add_column("Query", width=40)
                table.add_column("Results", width=8, justify="right")
                table.add_column("Time (ms)", width=10, justify="right")
                table.add_column("Cached", width=6, justify="center")
                table.add_column("Date", width=16)

                for i, q in enumerate(queries, 1):
                    query_display = q["query_text"]
                    if len(query_display) > 40:
                        query_display = query_display[:37] + "..."

                    created_at = q["created_at"]
                    date_display = created_at.strftime("%Y-%m-%d %H:%M") if created_at else "-"

                    table.add_row(
                        str(i),
                        query_display,
                        str(q["result_count"] or 0),
                        str(q["execution_time_ms"] or 0),
                        "Y" if q["cache_hit"] else "N",
                        date_display,
                    )

                console.print(table)

            return 0

    try:
        run_async(_history())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


# =============================================================================
# SEARCH SUGGEST COMMAND (PLACEHOLDER)
# =============================================================================


@search_group.command(name="suggest")
@click.option(
    "--prefix", "-p",
    help="Prefix to filter suggestions (for autocomplete)"
)
@click.option(
    "--context", "-c",
    help="Optional context to base suggestions on"
)
@click.option(
    "--limit", "-l",
    type=int,
    default=5,
    show_default=True,
    help="Number of suggestions to return"
)
@click.option(
    "--format", "-f",
    "output_format",
    type=click.Choice(["table", "json"]),
    default="table",
    show_default=True,
    help="Output format"
)
@click.pass_context
def search_suggest(
    ctx: click.Context,
    prefix: Optional[str],
    context: Optional[str],
    limit: int,
    output_format: str,
):
    """Get query suggestions for autocomplete.

    Generate search suggestions based on popular queries, your recent
    searches, and optional context or prefix filtering.

    Examples:

        penf search suggest

        penf search suggest -p "meeting"

        penf search suggest -c "preparing for quarterly review"

        penf search suggest -l 10 -f json
    """
    async def _suggest():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            engine = SuggestionEngine(session, tenant_id)

            if context:
                suggestions = await engine.get_contextual_suggestions(
                    context=context, limit=limit
                )
            else:
                suggestions = await engine.get_suggestions(
                    prefix=prefix, limit=limit
                )

            if not suggestions:
                console.print("[dim]No suggestions found.[/dim]")
                return 0

            if output_format == "json":
                json_data = [
                    {
                        "text": s.text,
                        "type": s.suggestion_type,
                        "frequency": s.frequency,
                        "success_rate": s.success_rate,
                    }
                    for s in suggestions
                ]
                console.print_json(json.dumps(json_data, indent=2))
            else:
                table = Table(
                    title="Query Suggestions",
                    show_header=True,
                    header_style="bold",
                )
                table.add_column("#", width=3)
                table.add_column("Suggestion", width=40)
                table.add_column("Type", width=12)
                table.add_column("Frequency", width=10, justify="right")
                table.add_column("Success", width=10, justify="right")

                for i, s in enumerate(suggestions, 1):
                    success_display = f"{s.success_rate * 100:.0f}%" if s.success_rate else "-"
                    table.add_row(
                        str(i),
                        s.text,
                        s.suggestion_type,
                        str(s.frequency),
                        success_display,
                    )

                console.print(table)

            return 0

    try:
        run_async(_suggest())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


# =============================================================================
# SEARCH POPULAR COMMAND
# =============================================================================


@search_group.command(name="popular")
@click.option(
    "--limit",
    "-l",
    type=int,
    default=10,
    show_default=True,
    help="Number of popular queries to show",
)
@click.option(
    "--format",
    "-f",
    "output_format",
    type=click.Choice(["table", "json"]),
    default="table",
    show_default=True,
    help="Output format",
)
@click.pass_context
def search_popular(
    ctx: click.Context,
    limit: int,
    output_format: str,
):
    """Show most popular search queries.

    Display the most frequently used search queries, ranked by
    frequency and success rate.

    Examples:

        penf search popular

        penf search popular -l 20

        penf search popular -f json
    """
    async def _popular():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            engine = SuggestionEngine(session, tenant_id)

            suggestions = await engine.get_popular_queries(limit=limit)

            if not suggestions:
                console.print("[dim]No popular queries found.[/dim]")
                return 0

            if output_format == "json":
                json_data = [
                    {
                        "query": s.text,
                        "frequency": s.frequency,
                        "success_rate": s.success_rate,
                    }
                    for s in suggestions
                ]
                console.print_json(json.dumps(json_data, indent=2))
            else:
                table = Table(
                    title="Popular Search Queries",
                    show_header=True,
                    header_style="bold",
                )
                table.add_column("Rank", width=5, justify="center")
                table.add_column("Query", width=45)
                table.add_column("Searches", width=10, justify="right")
                table.add_column("Success Rate", width=12, justify="right")

                for i, s in enumerate(suggestions, 1):
                    success_display = (
                        f"{s.success_rate * 100:.0f}%" if s.success_rate else "-"
                    )
                    table.add_row(
                        str(i),
                        s.text,
                        str(s.frequency),
                        success_display,
                    )

                console.print(table)

            return 0

    try:
        run_async(_popular())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


# =============================================================================
# SEARCH STATS COMMAND
# =============================================================================


@search_group.command(name="stats")
@click.option(
    "--days",
    "-d",
    type=int,
    default=7,
    show_default=True,
    help="Number of days to analyze",
)
@click.option(
    "--format",
    "-f",
    "output_format",
    type=click.Choice(["pretty", "json"]),
    default="pretty",
    show_default=True,
    help="Output format",
)
@click.pass_context
def search_stats(
    ctx: click.Context,
    days: int,
    output_format: str,
):
    """Show search analytics and statistics.

    Display aggregated search metrics including query counts,
    performance percentiles, cache effectiveness, and success rates.

    Examples:

        penf search stats

        penf search stats -d 30

        penf search stats -f json
    """
    async def _stats():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            collector = AnalyticsCollector(session, tenant_id)

            metrics = await collector.get_metrics(days=days)
            trending = await collector.get_trending_queries(limit=5, days=days)

            if output_format == "json":
                json_data = {
                    "period_days": days,
                    "metrics": {
                        "total_queries": metrics.total_queries,
                        "unique_users": metrics.unique_users,
                        "avg_execution_time_ms": metrics.avg_execution_time_ms,
                        "cache_hit_rate": metrics.cache_hit_rate,
                        "zero_result_rate": metrics.zero_result_rate,
                        "successful_search_rate": metrics.successful_search_rate,
                        "p50_execution_time_ms": metrics.p50_execution_time_ms,
                        "p95_execution_time_ms": metrics.p95_execution_time_ms,
                        "p99_execution_time_ms": metrics.p99_execution_time_ms,
                    },
                    "strategy_distribution": metrics.content_type_distribution,
                    "trending_queries": trending,
                }
                console.print_json(json.dumps(json_data, indent=2))
            else:
                # Header panel
                console.print(
                    Panel(
                        f"Search Analytics - Last {days} Days",
                        style="bold blue",
                    )
                )

                # Metrics table
                metrics_table = Table(
                    title="Performance Metrics",
                    show_header=True,
                    header_style="bold",
                )
                metrics_table.add_column("Metric", width=25)
                metrics_table.add_column("Value", width=20, justify="right")

                metrics_table.add_row("Total Queries", str(metrics.total_queries))
                metrics_table.add_row(
                    "Average Response Time",
                    f"{metrics.avg_execution_time_ms:.0f}ms",
                )
                metrics_table.add_row(
                    "Cache Hit Rate", f"{metrics.cache_hit_rate * 100:.1f}%"
                )
                metrics_table.add_row(
                    "Success Rate", f"{metrics.successful_search_rate * 100:.1f}%"
                )
                metrics_table.add_row(
                    "Zero Result Rate", f"{metrics.zero_result_rate * 100:.1f}%"
                )

                console.print(metrics_table)

                # Percentiles table
                perf_table = Table(
                    title="Response Time Percentiles",
                    show_header=True,
                    header_style="bold",
                )
                perf_table.add_column("Percentile", width=15)
                perf_table.add_column("Time", width=15, justify="right")

                perf_table.add_row("p50 (median)", f"{metrics.p50_execution_time_ms}ms")
                perf_table.add_row("p95", f"{metrics.p95_execution_time_ms}ms")
                perf_table.add_row("p99", f"{metrics.p99_execution_time_ms}ms")

                console.print(perf_table)

                # Trending queries
                if trending:
                    trend_table = Table(
                        title="Trending Queries",
                        show_header=True,
                        header_style="bold",
                    )
                    trend_table.add_column("#", width=3)
                    trend_table.add_column("Query", width=35)
                    trend_table.add_column("Searches", width=10, justify="right")
                    trend_table.add_column("Success", width=10, justify="right")

                    for i, t in enumerate(trending, 1):
                        trend_table.add_row(
                            str(i),
                            t["query"],
                            str(t["frequency"]),
                            t["success_rate"],
                        )

                    console.print(trend_table)

            return 0

    try:
        run_async(_stats())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


# =============================================================================
# RELATED CONTENT OUTPUT HELPERS
# =============================================================================


def _print_related_pretty(response: RelatedContentResponse) -> None:
    """Print related content in rich formatted output."""
    console.print(
        Panel(
            f"Found [bold]{response.total_correlations}[/bold] related items "
            f"in {response.execution_time_ms}ms",
            title=f"Related to {response.entity_type}/{response.entity_id}",
        )
    )

    for i, item in enumerate(response.related_content, 1):
        _print_related_card(i, item)

    if not response.related_content:
        console.print("[dim]No related content found.[/dim]")


def _print_related_card(index: int, item) -> None:
    """Print single related item as a card."""
    # Content type badge
    type_colors = {
        "email": "blue",
        "meeting": "green",
        "document": "yellow",
        "slack": "magenta",
    }
    type_color = type_colors.get(item.content_type.value, "white")

    # Correlation type badge colors
    corr_colors = {
        "participant": "cyan",
        "project": "yellow",
        "temporal": "green",
        "semantic": "magenta",
        "thread": "blue",
    }
    corr_color = corr_colors.get(item.correlation_type.value, "white")

    # Build header
    header = Text()
    header.append(f"[{index}] ", style="bold")
    header.append(f"[{item.content_type.value}] ", style=type_color)
    if item.title:
        header.append(item.title, style="bold")
    else:
        header.append(f"Entity #{item.entity_id}", style="dim")

    console.print(header)

    # Show correlation info
    console.print(
        f"    [dim]{item.timestamp.strftime('%Y-%m-%d %H:%M')} | "
        f"Correlation: [{corr_color}]{item.correlation_type.value}[/{corr_color}] "
        f"({item.correlation_score:.2f})[/dim]"
    )

    # Show score breakdown if present
    if item.score_breakdown:
        breakdown_parts = [
            f"{sig}: {score:.2f}"
            for sig, score in sorted(item.score_breakdown.items(), key=lambda x: -x[1])
            if score > 0
        ]
        if breakdown_parts:
            console.print(f"    [dim italic]Signals: {', '.join(breakdown_parts)}[/dim italic]")

    console.print()


def _print_related_table(response: RelatedContentResponse) -> None:
    """Print related content in table format."""
    table = Table(show_header=True, header_style="bold")
    table.add_column("#", width=3)
    table.add_column("Type", width=8)
    table.add_column("Title", width=35)
    table.add_column("Correlation", width=12)
    table.add_column("Score", width=6)
    table.add_column("Date", width=12)

    for i, item in enumerate(response.related_content, 1):
        title = item.title or f"Entity #{item.entity_id}"
        table.add_row(
            str(i),
            item.content_type.value,
            title[:35] if title else "",
            item.correlation_type.value,
            f"{item.correlation_score:.2f}",
            item.timestamp.strftime("%Y-%m-%d"),
        )

    console.print(table)
    console.print(
        f"\n{response.total_correlations} related items found, "
        f"{response.execution_time_ms}ms"
    )


# =============================================================================
# SEARCH CORRELATE COMMAND
# =============================================================================


@search_group.command(name="correlate", deprecated=True)
@click.argument("content_id", type=int)
@click.option(
    "--entity-type", "-e",
    type=click.Choice(["source", "assertion"]),
    default="source",
    show_default=True,
    help="Type of content entity"
)
@click.option(
    "--limit", "-l",
    type=int,
    default=10,
    show_default=True,
    help="Maximum number of related items to return"
)
@click.option(
    "--format", "-f",
    "output_format",
    type=click.Choice(["pretty", "table", "json"]),
    default="pretty",
    show_default=True,
    help="Output format"
)
@click.pass_context
def search_correlate(
    ctx: click.Context,
    content_id: int,
    entity_type: str,
    limit: int,
    output_format: str,
):
    """Find content related to a specific item.

    DEPRECATED: Use 'penf search related source <id>' instead.
    This command will be removed in a future release.

    Given a content ID, discover related content across all sources
    using multiple correlation signals:

    \b
    - Shared participants (people involved in both)
    - Project references (same project mentioned)
    - Temporal proximity (content within time window)
    - Semantic similarity (embedding distance)
    - Thread chains (conversation relationships)

    Examples:

        penf search correlate 12345

        penf search correlate 12345 --entity-type assertion

        penf search correlate 12345 -l 20 -f json
    """
    async def _correlate():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            engine = SearchEngine(session, tenant_id)

            response = await engine.find_related(
                content_id=content_id,
                content_type=entity_type,
                limit=limit,
            )

            if output_format == "json":
                console.print_json(response.model_dump_json())
            elif output_format == "table":
                _print_related_table(response)
            else:
                _print_related_pretty(response)

            return 0

    try:
        run_async(_correlate())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


# =============================================================================
# RELATED SUBCOMMAND GROUP
# =============================================================================


@search_group.group(name="related")
@click.pass_context
def related_group(ctx: click.Context):
    """Find related content by entity type.

    Discover content related to specific people, projects,
    or other entities across all content sources.
    """
    pass


@related_group.command(name="person")
@click.argument("identifier")
@click.option(
    "--limit", "-l",
    type=int,
    default=20,
    show_default=True,
    help="Maximum number of items to return"
)
@click.option(
    "--format", "-f",
    "output_format",
    type=click.Choice(["pretty", "table", "json"]),
    default="pretty",
    show_default=True,
    help="Output format"
)
@click.pass_context
def related_person(
    ctx: click.Context,
    identifier: str,
    limit: int,
    output_format: str,
):
    """Find content related to a person.

    Search for content involving a specific person by email or name.

    Examples:

        penf search related person "john@example.com"

        penf search related person "Jane Smith" -l 30

        penf search related person "alice" -f json
    """
    async def _find_related():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            engine = SearchEngine(session, tenant_id)

            response = await engine.find_related_by_person(
                person_identifier=identifier,
                limit=limit,
            )

            if output_format == "json":
                console.print_json(response.model_dump_json())
            elif output_format == "table":
                _print_related_table(response)
            else:
                console.print(Panel(
                    f"Found [bold]{response.total_correlations}[/bold] items "
                    f"involving '{identifier}' in {response.execution_time_ms}ms",
                    title="Related by Person",
                ))
                for i, item in enumerate(response.related_content, 1):
                    _print_related_card(i, item)

            return 0

    try:
        run_async(_find_related())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


@related_group.command(name="project")
@click.argument("project_name")
@click.option(
    "--limit", "-l",
    type=int,
    default=20,
    show_default=True,
    help="Maximum number of items to return"
)
@click.option(
    "--format", "-f",
    "output_format",
    type=click.Choice(["pretty", "table", "json"]),
    default="pretty",
    show_default=True,
    help="Output format"
)
@click.pass_context
def related_project(
    ctx: click.Context,
    project_name: str,
    limit: int,
    output_format: str,
):
    """Find content related to a project.

    Search for content mentioning a specific project.

    Examples:

        penf search related project "Atlas"

        penf search related project "Q1 Planning" -l 30

        penf search related project "deployment" -f json
    """
    async def _find_related():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            engine = SearchEngine(session, tenant_id)

            response = await engine.find_related_by_project(
                project_name=project_name,
                limit=limit,
            )

            if output_format == "json":
                console.print_json(response.model_dump_json())
            elif output_format == "table":
                _print_related_table(response)
            else:
                console.print(Panel(
                    f"Found [bold]{response.total_correlations}[/bold] items "
                    f"for project '{project_name}' in {response.execution_time_ms}ms",
                    title="Related by Project",
                ))
                for i, item in enumerate(response.related_content, 1):
                    _print_related_card(i, item)

            return 0

    try:
        run_async(_find_related())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())


@related_group.command(name="source")
@click.argument("source_id", type=int)
@click.option(
    "--limit", "-l",
    type=int,
    default=10,
    show_default=True,
    help="Maximum number of related items to return"
)
@click.option(
    "--format", "-f",
    "output_format",
    type=click.Choice(["pretty", "table", "json"]),
    default="pretty",
    show_default=True,
    help="Output format"
)
@click.pass_context
def related_source(
    ctx: click.Context,
    source_id: int,
    limit: int,
    output_format: str,
):
    """Find content related to a source.

    Discover content related to a specific source ID through
    participant overlap, project references, temporal proximity,
    semantic similarity, and thread chains.

    Examples:

        penf search related source 12345

        penf search related source 12345 -l 20 -f json
    """
    async def _find_related():
        tenant_id = ctx.obj.get("tenant") if ctx.obj else None
        tenant_id = tenant_id or "default"

        async with get_session() as session:
            engine = SearchEngine(session, tenant_id)

            response = await engine.find_related(
                content_id=source_id,
                content_type="source",
                limit=limit,
            )

            if output_format == "json":
                console.print_json(response.model_dump_json())
            elif output_format == "table":
                _print_related_table(response)
            else:
                _print_related_pretty(response)

            return 0

    try:
        run_async(_find_related())
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        raise click.Abort() from e
    finally:
        run_async(cleanup_connections())
