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

from penf_lib.search.models import (
    ContentTypeFilter,
    RelatedContentResponse,
    SearchQuery,
    SearchResponse,
)
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
# RELATED CONTENT OUTPUT HELPERS
# =============================================================================


def _print_related_pretty(response: RelatedContentResponse) -> None:
    """Print related content in rich formatted output."""
    console.print(Panel(
        f"Found [bold]{response.total_correlations}[/bold] related items "
        f"in {response.execution_time_ms}ms",
        title=f"Related to {response.entity_type}/{response.entity_id}",
    ))

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


@search_group.command(name="correlate")
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
        result = run_async(_correlate())
        sys.exit(result)
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        sys.exit(1)
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
        result = run_async(_find_related())
        sys.exit(result)
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        sys.exit(1)
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
        result = run_async(_find_related())
        sys.exit(result)
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        sys.exit(1)
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
        result = run_async(_find_related())
        sys.exit(result)
    except Exception as e:
        console.print(f"[red]Error: {e}[/red]")
        sys.exit(1)
    finally:
        run_async(cleanup_connections())
