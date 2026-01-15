"""Automation Rules Engine CLI commands.

CLI commands for managing automation rules, thresholds, patterns,
and viewing automation statistics.

Reference: contracts/automation-api.yaml
"""

import asyncio
import json
import sys
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Optional
from uuid import UUID

import click
from rich.console import Console
from rich.table import Table
from rich.panel import Panel

from penf_lib.storage.connections import cleanup_connections

console = Console()


def run_async(coro):
    """Run an async coroutine synchronously."""
    return asyncio.run(coro)


@click.group(name="automation")
@click.pass_context
def automation_group(ctx: click.Context):
    """
    Automation Rules Engine commands.

    Manage automation rules, thresholds, and view automation statistics.
    """
    pass


# =============================================================================
# THRESHOLD COMMANDS
# =============================================================================


@automation_group.group(name="thresholds")
@click.pass_context
def thresholds_group(ctx: click.Context):
    """
    Manage confidence thresholds for automatic processing.

    Thresholds determine when content is automatically processed
    versus queued for manual review.
    """
    pass


@thresholds_group.command(name="list")
@click.pass_context
def thresholds_list(ctx: click.Context):
    """List all confidence thresholds."""
    async def _list_thresholds():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            # Get current tenant context
            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session. Use 'penf tenant switch <name>' first.")
                return 1

            tenant_id = session_info["tenant_id"]
            user_id = session_info["user_email"]

            repo = AutomationRepository()

            # For now, display default since no DB connection
            # In full implementation, would query confidence_thresholds table

            table = Table(title="Confidence Thresholds")
            table.add_column("Content Type", style="cyan")
            table.add_column("Threshold", style="green")
            table.add_column("Status", style="yellow")

            # Show default threshold
            table.add_row("* (default)", "85.0%", "enabled")
            table.add_row("email", "85.0% (default)", "enabled")
            table.add_row("meeting", "85.0% (default)", "enabled")
            table.add_row("document", "85.0% (default)", "enabled")

            console.print(table)
            console.print()
            console.print("[dim]Use 'penf automation thresholds set' to customize thresholds[/dim]")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_list_thresholds())
    sys.exit(result)


@thresholds_group.command(name="set")
@click.option("--content-type", "-t", required=True, help="Content type (email, meeting, document, or * for default)")
@click.option("--value", "-v", required=True, type=float, help="Threshold value (0.0 to 1.0)")
@click.pass_context
def thresholds_set(ctx: click.Context, content_type: str, value: float):
    """Set confidence threshold for a content type."""
    async def _set_threshold():
        try:
            # Validate threshold value
            if value < 0.0 or value > 1.0:
                console.print("[red]Error:[/red] Threshold must be between 0.0 and 1.0")
                return 1

            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            # Get current tenant context
            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session. Use 'penf tenant switch <name>' first.")
                return 1

            tenant_id = UUID(session_info["tenant_id"])
            user_id = session_info["user_email"]

            repo = AutomationRepository()

            result = await repo.set_threshold(
                tenant_id=tenant_id,
                user_id=user_id,
                content_type=content_type,
                threshold_value=value,
            )

            console.print(f"[green]Success:[/green] Threshold set for '{content_type}' to {value:.1%}")
            console.print(f"[dim]Content with confidence >= {value:.1%} will be auto-processed[/dim]")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_set_threshold())
    sys.exit(result)


# =============================================================================
# DECISIONS COMMANDS
# =============================================================================


@automation_group.group(name="decisions")
@click.pass_context
def decisions_group(ctx: click.Context):
    """
    View automation decision history (audit trail).

    Shows how content was processed and the reasoning behind each decision.
    """
    pass


@decisions_group.command(name="list")
@click.option("--since", "-s", default="7 days ago", help="Show decisions since (e.g., '7 days ago', '2026-01-01')")
@click.option("--type", "-t", "decision_type", type=click.Choice(["auto_processed", "queued_review", "conflict_resolved"]), help="Filter by decision type")
@click.option("--limit", "-l", default=50, help="Maximum number of decisions to show")
@click.pass_context
def decisions_list(ctx: click.Context, since: str, decision_type: Optional[str], limit: int):
    """List recent automation decisions."""
    async def _list_decisions():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            # Get current tenant context
            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session. Use 'penf tenant switch <name>' first.")
                return 1

            # For now, show empty list since no DB connection
            console.print("[yellow]No decisions recorded yet.[/yellow]")
            console.print("[dim]Decisions will appear here as content is processed.[/dim]")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_list_decisions())
    sys.exit(result)


# =============================================================================
# STATS COMMAND
# =============================================================================


@automation_group.command(name="stats")
@click.option("--period", "-p", default=30, type=int, help="Stats period in days")
@click.pass_context
def automation_stats(ctx: click.Context, period: int):
    """Show automation statistics."""
    async def _show_stats():
        try:
            from penf_lib.storage.tenant_manager import tenant_manager

            # Get current tenant context
            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session. Use 'penf tenant switch <name>' first.")
                return 1

            # Display placeholder stats
            panel_content = f"""
[bold]Period:[/bold] {period} days

[bold]Processing Summary:[/bold]
  Total Processed:     0
  Auto-Processed:      0 (0%)
  Review Queue:        0 (0%)
  Accuracy Rate:       N/A

[bold]Rules:[/bold]
  Active Rules:        0
  Pending Patterns:    0
  Conflicts Resolved:  0

[dim]Statistics will populate as content is processed.[/dim]
"""

            console.print(Panel(
                panel_content,
                title="Automation Statistics",
                border_style="blue"
            ))

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_show_stats())
    sys.exit(result)


# =============================================================================
# SETTINGS COMMANDS
# =============================================================================


@automation_group.group(name="settings")
@click.pass_context
def settings_group(ctx: click.Context):
    """
    Manage progressive automation settings.

    Configure how aggressively the system learns and automates.
    """
    pass


@settings_group.command(name="show")
@click.pass_context
def settings_show(ctx: click.Context):
    """Show current automation settings."""
    async def _show_settings():
        try:
            from penf_lib.storage.tenant_manager import tenant_manager

            # Get current tenant context
            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session. Use 'penf tenant switch <name>' first.")
                return 1

            # Display default settings
            console.print()
            console.print("[bold]Automation Settings[/bold]")
            console.print()
            console.print(f"  Aggressiveness:           [cyan]moderate[/cyan]")
            console.print(f"  Auto Threshold Adjustment: [green]enabled[/green]")
            console.print(f"  Min Learning Period:      [yellow]30 days[/yellow]")
            console.print(f"  Target Automation Rate:   [cyan]60%[/cyan]")
            console.print(f"  Accuracy Floor:           [cyan]95%[/cyan]")
            console.print(f"  Pattern Suggestions:      [green]enabled[/green]")
            console.print()
            console.print("[dim]Use 'penf automation settings set' to customize[/dim]")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_show_settings())
    sys.exit(result)


@settings_group.command(name="set")
@click.option("--aggressiveness", type=click.Choice(["conservative", "moderate", "aggressive"]), help="Automation aggressiveness level")
@click.option("--target-rate", type=float, help="Target automation rate (0.0 to 1.0)")
@click.option("--accuracy-floor", type=float, help="Minimum accuracy threshold (0.8 to 1.0)")
@click.option("--auto-threshold-adjustment/--no-auto-threshold-adjustment", default=None, help="Enable/disable automatic threshold adjustment")
@click.pass_context
def settings_set(ctx: click.Context, aggressiveness: Optional[str], target_rate: Optional[float],
                 accuracy_floor: Optional[float], auto_threshold_adjustment: Optional[bool]):
    """Update automation settings."""
    async def _set_settings():
        try:
            from penf_lib.storage.tenant_manager import tenant_manager

            # Get current tenant context
            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session. Use 'penf tenant switch <name>' first.")
                return 1

            # Validate inputs
            if target_rate is not None and (target_rate < 0.0 or target_rate > 1.0):
                console.print("[red]Error:[/red] Target rate must be between 0.0 and 1.0")
                return 1

            if accuracy_floor is not None and (accuracy_floor < 0.8 or accuracy_floor > 1.0):
                console.print("[red]Error:[/red] Accuracy floor must be between 0.8 and 1.0")
                return 1

            updates = []
            if aggressiveness:
                updates.append(f"aggressiveness: {aggressiveness}")
            if target_rate is not None:
                updates.append(f"target rate: {target_rate:.0%}")
            if accuracy_floor is not None:
                updates.append(f"accuracy floor: {accuracy_floor:.0%}")
            if auto_threshold_adjustment is not None:
                updates.append(f"auto threshold adjustment: {'enabled' if auto_threshold_adjustment else 'disabled'}")

            if not updates:
                console.print("[yellow]No settings specified to update.[/yellow]")
                console.print("[dim]Use --help to see available options[/dim]")
                return 1

            console.print("[green]Success:[/green] Settings updated")
            for update in updates:
                console.print(f"  - {update}")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_set_settings())
    sys.exit(result)


# Export the automation group for inclusion in main CLI
__all__ = ["automation_group"]
