"""Penfold CLI main entry point."""

import asyncio
import logging
import sys
from typing import Optional

import click
from rich.console import Console
from rich.logging import RichHandler

from penf_lib.storage.connections import health_check, cleanup_connections
from .relationships import relationships_group
from .tenant import tenant_group
from .review_commands import review_group
from .automation_commands import automation_group
from .search_commands import search_group

# Initialize rich console for beautiful output
console = Console()

# Setup logging with rich handler
logging.basicConfig(
    level=logging.INFO,
    format="%(message)s",
    datefmt="[%X]",
    handlers=[RichHandler(console=console, rich_tracebacks=True)]
)

logger = logging.getLogger("penf")


@click.group()
@click.option(
    "--verbose", "-v",
    is_flag=True,
    help="Enable verbose logging"
)
@click.option(
    "--tenant",
    envvar="PENFOLD_TENANT",
    help="Tenant context (work, personal, family) - can also use PENFOLD_TENANT env var"
)
@click.pass_context
def cli(ctx: click.Context, verbose: bool, tenant: Optional[str]):
    """
    Penfold - Personal AI-powered contextual information system.

    A multi-tenant command-line interface for managing your contextual time machine
    across work, personal, and family contexts.
    """
    # Configure logging level
    if verbose:
        logging.getLogger().setLevel(logging.DEBUG)
        logger.debug("Verbose mode enabled")

    # Store context for subcommands
    ctx.ensure_object(dict)
    ctx.obj["verbose"] = verbose
    ctx.obj["tenant"] = tenant

    if verbose:
        console.print(f"[dim]Penfold CLI starting...[/dim]")
        if tenant:
            console.print(f"[dim]Tenant context: {tenant}[/dim]")


# Add command groups
cli.add_command(tenant_group)
cli.add_command(relationships_group)

# Add review workflow commands
cli.add_command(review_group)

# Add automation commands
cli.add_command(automation_group)

# Add search commands
cli.add_command(search_group)


@cli.command()
@click.pass_context
def health(ctx: click.Context):
    """Check health status of all Penfold services."""
    async def check_health():
        try:
            status = await health_check()

            if status["overall"] == "healthy":
                console.print("[green]✓[/green] All services are healthy")
                console.print(f"  Database: [green]{status['database']}[/green]")
                console.print(f"  Redis: [green]{status['redis']}[/green]")
                return 0
            else:
                console.print("[red]✗[/red] Some services are unhealthy")
                console.print(f"  Database: [{'green' if status['database'] == 'healthy' else 'red'}]{status['database']}[/]")
                console.print(f"  Redis: [{'green' if status['redis'] == 'healthy' else 'red'}]{status['redis']}[/]")
                return 1
        except Exception as e:
            console.print(f"[red]✗[/red] Health check failed: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(check_health())
    sys.exit(result)


@cli.command()
@click.pass_context
def version(ctx: click.Context):
    """Show Penfold version information."""
    try:
        import penf_lib
        version = getattr(penf_lib, "__version__", "0.1.0-dev")
    except ImportError:
        version = "0.1.0-dev"

    console.print(f"Penfold CLI version: [bold]{version}[/bold]")
    console.print("Personal AI-powered contextual information system")
    console.print("[dim]Multi-tenant contextual time machine[/dim]")


@cli.command()
@click.pass_context
def status(ctx: click.Context):
    """Show current session status and tenant information."""
    async def show_status():
        try:
            from penf_lib.storage.tenant_manager import tenant_manager

            # Get current session info
            session_info = await tenant_manager.get_current_tenant()

            if session_info:
                console.print("[green]✓[/green] Active session found")
                console.print(f"  Tenant: [bold]{session_info['tenant_display_name']}[/bold] ({session_info['tenant_name']})")
                console.print(f"  User: [blue]{session_info['user_email']}[/blue]")
                console.print(f"  Session ID: [dim]{session_info['session_id'][:8]}...[/dim]")
                console.print(f"  Last Activity: [dim]{session_info['last_activity']}[/dim]")
                console.print(f"  Expires: [dim]{session_info['expires_at']}[/dim]")
            else:
                console.print("[yellow]⚠[/yellow] No active tenant session")
                console.print("  Use [bold]penf tenant switch <name>[/bold] to activate a tenant context")

            return 0
        except Exception as e:
            console.print(f"[red]✗[/red] Status check failed: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(show_status())
    sys.exit(result)


def main():
    """Main CLI entry point."""
    try:
        cli()
    except KeyboardInterrupt:
        console.print("\n[yellow]Operation cancelled by user[/yellow]")
        sys.exit(130)
    except Exception as e:
        console.print(f"[red]Error:[/red] {e}")
        if logging.getLogger().level <= logging.DEBUG:
            console.print_exception()
        sys.exit(1)


if __name__ == "__main__":
    main()