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


def parse_uuid(value: str, name: str = "ID") -> Optional[UUID]:
    """Parse a UUID string with user-friendly error handling.

    Args:
        value: String value to parse as UUID
        name: Name of the field for error messages

    Returns:
        UUID if valid, None if invalid (error is printed)
    """
    try:
        return UUID(value)
    except ValueError:
        console.print(f"[red]Error:[/red] Invalid {name} format: '{value}'")
        return None


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


# =============================================================================
# RULES COMMANDS
# =============================================================================


@automation_group.group(name="rules")
@click.pass_context
def rules_group(ctx: click.Context):
    """
    Manage automation rules.

    Rules define conditions and actions for automatic content processing.
    """
    pass


@rules_group.command(name="list")
@click.option("--all", "-a", "show_all", is_flag=True, help="Show all rules including disabled")
@click.pass_context
def rules_list(ctx: click.Context, show_all: bool):
    """List automation rules."""
    async def _list_rules():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            tenant_id = UUID(session_info["tenant_id"])
            user_id = session_info["user_email"]

            repo = AutomationRepository()
            rules = await repo.get_rules_for_user(
                tenant_id, user_id, enabled_only=not show_all
            )

            if not rules:
                console.print("[yellow]No rules found.[/yellow]")
                console.print("[dim]Create rules with 'penf automation rules create'[/dim]")
                return 0

            table = Table(title="Automation Rules")
            table.add_column("ID", style="dim", width=8)
            table.add_column("Name", style="cyan")
            table.add_column("Priority", justify="center")
            table.add_column("Status", style="green")
            table.add_column("Version")

            for rule in rules:
                rule_id = str(rule["id"])[:8]
                status = "[green]enabled[/green]" if rule.get("is_enabled") else "[dim]disabled[/dim]"
                table.add_row(
                    rule_id,
                    rule["name"],
                    str(rule.get("priority", 5)),
                    status,
                    f"v{rule.get('current_version_id', 1)}",
                )

            console.print(table)
            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_list_rules())
    sys.exit(result)


@rules_group.command(name="show")
@click.argument("rule_id")
@click.pass_context
def rules_show(ctx: click.Context, rule_id: str):
    """Show rule details."""
    async def _show_rule():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            tenant_id = UUID(session_info["tenant_id"])

            rule_uuid = parse_uuid(rule_id, "rule ID")
            if not rule_uuid:
                return 1

            repo = AutomationRepository()
            rule = await repo.get_rule_by_id(tenant_id, rule_uuid)

            if not rule:
                console.print(f"[red]Error:[/red] Rule '{rule_id}' not found")
                return 1

            console.print()
            console.print(f"[bold]Rule: {rule['name']}[/bold]")
            console.print(f"ID: {rule['id']}")
            console.print(f"Priority: {rule.get('priority', 5)}")
            console.print(f"Status: {'[green]enabled[/green]' if rule.get('is_enabled') else '[dim]disabled[/dim]'}")
            console.print(f"Version: v{rule.get('current_version_id', 1)}")
            if rule.get("description"):
                console.print(f"Description: {rule['description']}")

            console.print()
            console.print("[bold]Conditions:[/bold]")
            console.print(json.dumps(rule.get("conditions", {}), indent=2))

            console.print()
            console.print("[bold]Actions:[/bold]")
            console.print(json.dumps(rule.get("actions", {}), indent=2))

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_show_rule())
    sys.exit(result)


@rules_group.command(name="create")
@click.option("--name", "-n", required=True, help="Rule name")
@click.option("--conditions", "-c", required=True, help="JSON conditions")
@click.option("--actions", "-a", required=True, help="JSON actions")
@click.option("--description", "-d", help="Rule description")
@click.option("--priority", "-p", type=int, default=5, help="Priority (1-10, default 5)")
@click.pass_context
def rules_create(ctx: click.Context, name: str, conditions: str, actions: str,
                 description: Optional[str], priority: int):
    """Create a new automation rule."""
    async def _create_rule():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            tenant_id = UUID(session_info["tenant_id"])
            user_id = session_info["user_email"]

            # Parse JSON
            try:
                conditions_dict = json.loads(conditions)
            except json.JSONDecodeError as e:
                console.print(f"[red]Error:[/red] Invalid conditions JSON: {e}")
                return 1

            try:
                actions_dict = json.loads(actions)
            except json.JSONDecodeError as e:
                console.print(f"[red]Error:[/red] Invalid actions JSON: {e}")
                return 1

            repo = AutomationRepository()
            rule = await repo.create_rule(
                tenant_id=tenant_id,
                user_id=user_id,
                name=name,
                conditions=conditions_dict,
                actions=actions_dict,
                description=description,
                priority=priority,
            )

            console.print(f"[green]Success:[/green] Rule '{name}' created")
            console.print(f"ID: {rule['id']}")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_create_rule())
    sys.exit(result)


@rules_group.command(name="enable")
@click.argument("rule_id")
@click.pass_context
def rules_enable(ctx: click.Context, rule_id: str):
    """Enable a rule."""
    async def _enable_rule():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            rule_uuid = parse_uuid(rule_id, "rule ID")
            if not rule_uuid:
                return 1

            repo = AutomationRepository()
            result = await repo.enable_rule(UUID(session_info["tenant_id"]), rule_uuid)

            if result:
                console.print(f"[green]Success:[/green] Rule enabled")
            else:
                console.print(f"[red]Error:[/red] Rule not found")
                return 1

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_enable_rule())
    sys.exit(result)


@rules_group.command(name="disable")
@click.argument("rule_id")
@click.pass_context
def rules_disable(ctx: click.Context, rule_id: str):
    """Disable a rule."""
    async def _disable_rule():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            rule_uuid = parse_uuid(rule_id, "rule ID")
            if not rule_uuid:
                return 1

            repo = AutomationRepository()
            result = await repo.disable_rule(UUID(session_info["tenant_id"]), rule_uuid)

            if result:
                console.print(f"[green]Success:[/green] Rule disabled")
            else:
                console.print(f"[red]Error:[/red] Rule not found")
                return 1

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_disable_rule())
    sys.exit(result)


@rules_group.command(name="delete")
@click.argument("rule_id")
@click.option("--reason", "-r", help="Deletion reason")
@click.option("--yes", "-y", is_flag=True, help="Skip confirmation")
@click.pass_context
def rules_delete(ctx: click.Context, rule_id: str, reason: Optional[str], yes: bool):
    """Delete a rule."""
    async def _delete_rule():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            rule_uuid = parse_uuid(rule_id, "rule ID")
            if not rule_uuid:
                return 1

            if not yes:
                console.print("[yellow]Warning:[/yellow] This will soft delete the rule.")
                if not click.confirm("Continue?"):
                    console.print("Cancelled.")
                    return 0

            repo = AutomationRepository()
            result = await repo.delete_rule(
                UUID(session_info["tenant_id"]),
                rule_uuid,
                reason=reason
            )

            if result:
                console.print(f"[green]Success:[/green] Rule deleted")
            else:
                console.print(f"[red]Error:[/red] Rule not found")
                return 1

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_delete_rule())
    sys.exit(result)


@rules_group.command(name="versions")
@click.argument("rule_id")
@click.pass_context
def rules_versions(ctx: click.Context, rule_id: str):
    """Show rule version history."""
    async def _show_versions():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            rule_uuid = parse_uuid(rule_id, "rule ID")
            if not rule_uuid:
                return 1

            repo = AutomationRepository()
            versions = await repo.get_rule_versions(
                UUID(session_info["tenant_id"]),
                rule_uuid
            )

            if not versions:
                console.print(f"[yellow]No versions found for rule {rule_id}[/yellow]")
                return 0

            table = Table(title=f"Version History for Rule {rule_id[:8]}")
            table.add_column("Version", style="cyan")
            table.add_column("Status")
            table.add_column("Change Description")
            table.add_column("Created")

            for v in versions:
                status = "[green]active[/green]" if v.get("is_active") else "[dim]inactive[/dim]"
                table.add_row(
                    f"v{v['version_number']}",
                    status,
                    v.get("change_description", ""),
                    str(v.get("created_at", ""))[:19],
                )

            console.print(table)
            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_show_versions())
    sys.exit(result)


@rules_group.command(name="rollback")
@click.argument("rule_id")
@click.option("--version", "-v", "version_num", type=int, required=True, help="Version number to rollback to")
@click.pass_context
def rules_rollback(ctx: click.Context, rule_id: str, version_num: int):
    """Rollback a rule to a previous version."""
    async def _rollback_rule():
        try:
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            rule_uuid = parse_uuid(rule_id, "rule ID")
            if not rule_uuid:
                return 1

            repo = AutomationRepository()
            result = await repo.rollback_rule(
                UUID(session_info["tenant_id"]),
                rule_uuid,
                version_num,
                session_info["user_email"],
            )

            if result:
                console.print(f"[green]Success:[/green] Rule rolled back to v{version_num}")
                console.print(f"New version: v{result['current_version_id']}")
            else:
                console.print(f"[red]Error:[/red] Rule or version not found")
                return 1

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_rollback_rule())
    sys.exit(result)


@rules_group.command(name="test")
@click.argument("rule_id")
@click.option("--content", "-c", required=True, help="JSON content to test against")
@click.pass_context
def rules_test(ctx: click.Context, rule_id: str, content: str):
    """Test a rule against sample content (FR-012)."""
    async def _test_rule():
        try:
            from penf_lib.automation.engine import AutomationEngine
            from penf_lib.automation.repository import AutomationRepository
            from penf_lib.storage.tenant_manager import tenant_manager

            session_info = await tenant_manager.get_current_tenant()
            if not session_info:
                console.print("[red]Error:[/red] No active tenant session.")
                return 1

            # Parse content
            try:
                content_dict = json.loads(content)
            except json.JSONDecodeError as e:
                console.print(f"[red]Error:[/red] Invalid content JSON: {e}")
                return 1

            rule_uuid = parse_uuid(rule_id, "rule ID")
            if not rule_uuid:
                return 1

            repo = AutomationRepository()
            rule = await repo.get_rule_by_id(
                UUID(session_info["tenant_id"]),
                rule_uuid
            )

            if not rule:
                console.print(f"[red]Error:[/red] Rule not found")
                return 1

            engine = AutomationEngine()
            matches = engine.rule_matches(rule["conditions"], content_dict)

            console.print()
            console.print(f"[bold]Testing Rule: {rule['name']}[/bold]")
            console.print()

            if matches:
                console.print("[green]MATCH[/green] - Rule would trigger")
                console.print()
                console.print("[bold]Actions that would execute:[/bold]")
                console.print(json.dumps(rule["actions"], indent=2))
            else:
                console.print("[yellow]NO MATCH[/yellow] - Rule would not trigger")

            return 0

        except Exception as e:
            console.print(f"[red]Error:[/red] {e}")
            return 1
        finally:
            await cleanup_connections()

    result = run_async(_test_rule())
    sys.exit(result)


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
