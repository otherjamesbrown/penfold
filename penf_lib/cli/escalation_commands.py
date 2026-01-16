"""Click command group for escalation briefings.

This module implements the `penf escalation` command for generating
comprehensive escalation briefings about entities (people, projects, topics).

Commands:
    - penf escalation <entity>: Generate an escalation briefing
    - penf escalation --days 30 <entity>: Briefing with custom timeframe
    - penf escalation --output file.md <entity>: Save to file
"""

from __future__ import annotations

import asyncio
import sys
from pathlib import Path
from typing import Optional

import click
from rich.console import Console
from rich.markdown import Markdown
from rich.panel import Panel
from rich.progress import Progress, SpinnerColumn, TextColumn

console = Console()


@click.group(name="escalation")
@click.pass_context
def escalation_group(ctx: click.Context) -> None:
    """Generate escalation briefings for entities.

    Escalation briefings provide comprehensive context about a person,
    project, or topic by aggregating and correlating information from
    all available sources within a specified timeframe.

    Examples:

        penf escalation brief "John Smith"

        penf escalation brief --days 30 "Project Alpha"

        penf escalation brief --output briefing.md "jane@example.com"
    """
    pass


@escalation_group.command(name="brief")
@click.argument("entity", required=True)
@click.option(
    "--days", "-d",
    type=int,
    default=180,
    help="Number of days to look back (default: 180)"
)
@click.option(
    "--output", "-o",
    type=click.Path(),
    help="Output file for the briefing (markdown format)"
)
@click.option(
    "--no-correlations",
    is_flag=True,
    help="Skip correlation expansion (faster but less comprehensive)"
)
@click.option(
    "--max-sources", "-m",
    type=int,
    default=100,
    help="Maximum sources to consider (default: 100)"
)
@click.pass_context
def brief_command(
    ctx: click.Context,
    entity: str,
    days: int,
    output: Optional[str],
    no_correlations: bool,
    max_sources: int,
) -> None:
    """Generate an escalation briefing for an entity.

    ENTITY can be a person's name, email address, project name, or topic.

    The briefing includes:

    - Resolved entity information

    - Chronological timeline of relevant events

    - Key insights extracted from the content

    - Recommended actions based on the analysis

    Examples:

        penf escalation brief "John Smith"

        penf escalation brief --days 30 "Project Alpha"

        penf escalation brief --output briefing.md "jane@example.com"
    """
    async def generate_briefing():
        from penf_lib.storage.connections import get_session, cleanup_connections
        from penf_lib.storage.tenant_manager import tenant_manager
        from penf_lib.workflows import EscalationBriefing

        try:
            # Get tenant context
            tenant_info = await tenant_manager.get_current_tenant()
            if not tenant_info:
                console.print("[red]Error:[/red] No active tenant session")
                console.print("Use [bold]penf tenant switch <name>[/bold] to activate a tenant")
                return 1

            tenant_id = tenant_info["tenant_id"]

            with Progress(
                SpinnerColumn(),
                TextColumn("[progress.description]{task.description}"),
                console=console,
                transient=True,
            ) as progress:
                progress.add_task(f"Generating briefing for '{entity}'...", total=None)

                async with get_session() as session:
                    config = {
                        "max_results": max_sources,
                        "include_correlations": not no_correlations,
                    }

                    workflow = EscalationBriefing(
                        session=session,
                        tenant_id=str(tenant_id),
                        config=config,
                    )

                    result = await workflow.execute(
                        entity_query=entity,
                        timeframe_days=days,
                    )

            if not result.success:
                console.print(f"[red]Error:[/red] Briefing generation failed")
                for error in result.errors:
                    console.print(f"  - {error}")
                return 1

            # Display or save the briefing
            if output:
                output_path = Path(output)
                output_path.write_text(result.briefing_markdown)
                console.print(f"[green]✓[/green] Briefing saved to [bold]{output}[/bold]")
                console.print(f"  - {len(result.timeline)} timeline events")
                console.print(f"  - {len(result.key_insights)} insights")
                console.print(f"  - {result.sources_consulted} sources consulted")
                console.print(f"  - Execution time: {result.execution_time_ms:.0f}ms")
            else:
                # Render as rich markdown
                console.print()
                console.print(Panel(
                    Markdown(result.briefing_markdown),
                    title=f"Escalation Briefing: {entity}",
                    border_style="blue",
                ))

                # Print summary stats
                console.print()
                console.print(f"[dim]Sources consulted: {result.sources_consulted}[/dim]")
                console.print(f"[dim]Execution time: {result.execution_time_ms:.0f}ms[/dim]")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            if ctx.obj.get("verbose"):
                console.print_exception()
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(generate_briefing())
    sys.exit(result)


@escalation_group.command(name="resolve")
@click.argument("query", required=True)
@click.pass_context
def resolve_command(ctx: click.Context, query: str) -> None:
    """Resolve an entity query to canonical identifiers.

    Shows how the system resolves a query to known entities
    (people, projects, topics) without generating a full briefing.

    Examples:

        penf escalation resolve "John Smith"

        penf escalation resolve "john@example.com"
    """
    async def resolve_entity():
        from penf_lib.storage.connections import get_session, cleanup_connections
        from penf_lib.storage.tenant_manager import tenant_manager
        from penf_lib.workflows import EscalationBriefing

        try:
            # Get tenant context
            tenant_info = await tenant_manager.get_current_tenant()
            if not tenant_info:
                console.print("[red]Error:[/red] No active tenant session")
                return 1

            tenant_id = tenant_info["tenant_id"]

            async with get_session() as session:
                workflow = EscalationBriefing(
                    session=session,
                    tenant_id=str(tenant_id),
                )

                resolved = await workflow._resolve_entities(query)

            console.print(f"\nQuery: [bold]{query}[/bold]")
            console.print(f"Resolved to {len(resolved)} entity/entities:\n")

            for entity in resolved:
                if entity.lower() == query.lower():
                    console.print(f"  • [yellow]{entity}[/yellow] (as-is, no match found)")
                else:
                    console.print(f"  • [green]{entity}[/green]")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(resolve_entity())
    sys.exit(result)


# Convenience alias - allow "penf escalation <entity>" as shorthand
@click.command(name="escalation-brief")
@click.argument("entity", required=True)
@click.option("--days", "-d", type=int, default=180)
@click.option("--output", "-o", type=click.Path())
@click.pass_context
def escalation_shorthand(ctx: click.Context, entity: str, days: int, output: Optional[str]) -> None:
    """Shorthand for 'penf escalation brief <entity>'."""
    ctx.invoke(brief_command, entity=entity, days=days, output=output)
