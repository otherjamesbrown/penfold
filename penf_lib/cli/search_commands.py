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
import sys
import uuid
from datetime import datetime

import click
from rich.console import Console
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from penf_lib.search.models import ContentTypeFilter, SearchQuery, SearchResponse
from penf_lib.search.search_engine import SearchEngine
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
    content_types: tuple[str, ...],
    since: datetime | None,
    until: datetime | None,
    limit: int,
    output_format: str,
):
    """Search across all content with natural language query.

    Example:
        penf search query "customer deployment issues"
        penf search query "meeting about Atlas" --type meeting --limit 10
        penf search query "budget discussions" --format json
    """
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

            # Build temporal constraint if provided
            temporal = None
            if since or until:
                from penf_lib.search.models import TemporalConstraint
                temporal = TemporalConstraint(
                    start_date=since,
                    end_date=until,
                )

            # Build query
            query = SearchQuery(
                query_text=query_text,
                content_types=types,
                temporal=temporal,
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
        result = run_async(_history())
        sys.exit(result)
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        sys.exit(1)
    finally:
        run_async(cleanup_connections())


# =============================================================================
# SEARCH SUGGEST COMMAND (PLACEHOLDER)
# =============================================================================


@search_group.command(name="suggest")
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
@click.pass_context
def search_suggest(
    ctx: click.Context,
    context: str | None,
    limit: int,
):
    """Get AI-powered query suggestions.

    Generate intelligent search suggestions based on your recent
    activity, content patterns, and optional context.

    Examples:

        penf search suggest

        penf search suggest -c "preparing for quarterly review"

        penf search suggest -l 10
    """
    async def _suggest():
        try:
            console.print("[dim]Not yet implemented (Phase 6)[/dim]")
            if context:
                console.print(f"[dim]Context: {context}[/dim]")
            console.print(f"[dim]Would return {limit} suggestions[/dim]")
            return 0
        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1

    result = run_async(_suggest())
    sys.exit(result)


# =============================================================================
# SEARCH CORRELATE COMMAND (PLACEHOLDER)
# =============================================================================


@search_group.command(name="correlate")
@click.argument("content_id")
@click.option(
    "--type", "-t",
    "content_types",
    multiple=True,
    type=click.Choice(["email", "document", "meeting", "message", "note"]),
    help="Content types to correlate with (can specify multiple)"
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
    type=click.Choice(["table", "json"]),
    default="table",
    show_default=True,
    help="Output format"
)
@click.pass_context
def search_correlate(
    ctx: click.Context,
    content_id: str,
    content_types: tuple[str, ...],
    limit: int,
    output_format: str,
):
    """Find related content across sources.

    Given a piece of content (email, document, etc.), find related
    content across all sources using semantic similarity and
    entity relationships.

    Examples:

        penf search correlate abc123

        penf search correlate abc123 -t email -t document

        penf search correlate abc123 -l 20 -f json
    """
    async def _correlate():
        try:
            console.print("[dim]Not yet implemented (Phase 5)[/dim]")
            console.print(f"[dim]Would find content related to: {content_id}[/dim]")
            if content_types:
                console.print(f"[dim]Looking in: {', '.join(content_types)}[/dim]")
            console.print(f"[dim]Limit: {limit}, Format: {output_format}[/dim]")
            return 0
        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1

    result = run_async(_correlate())
    sys.exit(result)
