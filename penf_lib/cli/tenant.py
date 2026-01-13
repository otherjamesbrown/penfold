"""Tenant management CLI commands."""

import asyncio
import sys
from typing import Optional

import click
from rich.console import Console
from rich.table import Table
from rich.prompt import Prompt, Confirm

from penf_lib.storage.connections import cleanup_connections
from penf_lib.storage.tenant_manager import tenant_manager

console = Console()


@click.group(name="tenant")
def tenant_group():
    """Manage tenant contexts (work, personal, family)."""
    pass


@tenant_group.command("list")
@click.option(
    "--user-email",
    envvar="PENFOLD_USER_EMAIL",
    help="User email (can also use PENFOLD_USER_EMAIL env var)"
)
def list_tenants(user_email: Optional[str]):
    """List available tenant contexts."""
    async def list_tenants_async():
        try:
            if not user_email:
                user_email_input = Prompt.ask("Enter your email address")
            else:
                user_email_input = user_email

            tenants = await tenant_manager.list_tenants(user_email_input)

            if not tenants:
                console.print("[yellow]⚠[/yellow] No tenants found for this user")
                console.print("Create a tenant with [bold]penf tenant create[/bold]")
                return 0

            # Display tenants in a table
            table = Table(title="Available Tenant Contexts")
            table.add_column("Name", style="bold")
            table.add_column("Display Name", style="cyan")
            table.add_column("Description", style="dim")
            table.add_column("Status", justify="center")

            for tenant_info in tenants:
                status = "[green]Active[/green]" if tenant_info["is_active"] else "[red]Inactive[/red]"
                table.add_row(
                    tenant_info["name"],
                    tenant_info["display_name"],
                    tenant_info["description"] or "[dim]No description[/dim]",
                    status
                )

            console.print(table)
            return 0

        except Exception as e:
            console.print(f"[red]✗[/red] Failed to list tenants: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(list_tenants_async())
    sys.exit(result)


@tenant_group.command("switch")
@click.argument("tenant_name", type=click.Choice(["work", "personal", "family"]))
@click.option(
    "--user-email",
    envvar="PENFOLD_USER_EMAIL",
    help="User email (can also use PENFOLD_USER_EMAIL env var)"
)
def switch_tenant(tenant_name: str, user_email: Optional[str]):
    """Switch to a specific tenant context."""
    async def switch_tenant_async():
        try:
            if not user_email:
                user_email_input = Prompt.ask("Enter your email address")
            else:
                user_email_input = user_email

            # Switch to tenant
            session_info = await tenant_manager.switch_to_tenant(tenant_name, user_email_input)

            console.print(f"[green]✓[/green] Switched to tenant: [bold]{session_info['tenant_display_name']}[/bold]")
            console.print(f"  Context: [blue]{tenant_name}[/blue]")
            console.print(f"  User: [dim]{user_email_input}[/dim]")
            console.print(f"  Session: [dim]{session_info.get('session_id', 'N/A')[:8]}...[/dim]")

            # Show guidance
            console.print("\n[dim]Your CLI operations will now be scoped to this tenant context.[/dim]")
            console.print("[dim]Use [bold]penf status[/bold] to check current session info.[/dim]")

            return 0

        except ValueError as e:
            console.print(f"[red]✗[/red] {e}")
            return 1
        except Exception as e:
            console.print(f"[red]✗[/red] Failed to switch tenant: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(switch_tenant_async())
    sys.exit(result)


@tenant_group.command("create")
@click.argument("tenant_name", type=click.Choice(["work", "personal", "family"]))
@click.option("--display-name", help="Human-readable display name")
@click.option("--description", help="Tenant description")
@click.option(
    "--user-email",
    envvar="PENFOLD_USER_EMAIL",
    help="Owner email (can also use PENFOLD_USER_EMAIL env var)"
)
def create_tenant(
    tenant_name: str,
    display_name: Optional[str],
    description: Optional[str],
    user_email: Optional[str]
):
    """Create a new tenant context."""
    async def create_tenant_async():
        try:
            if not user_email:
                user_email_input = Prompt.ask("Enter your email address")
            else:
                user_email_input = user_email

            if not display_name:
                default_display_names = {
                    "work": "Work",
                    "personal": "Personal",
                    "family": "Family"
                }
                display_name_input = Prompt.ask(
                    f"Display name for tenant",
                    default=default_display_names.get(tenant_name, tenant_name.title())
                )
            else:
                display_name_input = display_name

            if not description:
                default_descriptions = {
                    "work": "Professional work context - company emails, meetings, projects",
                    "personal": "Personal life context - personal emails, projects, goals",
                    "family": "Family context - shared communications, projects, planning"
                }
                description_input = Prompt.ask(
                    "Description (optional)",
                    default=default_descriptions.get(tenant_name, "")
                )
            else:
                description_input = description

            # Create tenant
            tenant_info = await tenant_manager.create_tenant(
                name=tenant_name,
                display_name=display_name_input,
                owner_email=user_email_input,
                description=description_input or None
            )

            console.print(f"[green]✓[/green] Created tenant: [bold]{display_name_input}[/bold]")
            console.print(f"  Name: [blue]{tenant_name}[/blue]")
            console.print(f"  Owner: [dim]{user_email_input}[/dim]")
            console.print(f"  Description: [dim]{description_input or 'No description'}[/dim]")
            console.print(f"  ID: [dim]{tenant_info['id']}[/dim]")

            # Ask if they want to switch to it
            if Confirm.ask(f"\nSwitch to '{tenant_name}' tenant now?"):
                session_info = await tenant_manager.switch_to_tenant(tenant_name, user_email_input)
                console.print(f"[green]✓[/green] Switched to new tenant context")

            return 0

        except ValueError as e:
            console.print(f"[red]✗[/red] {e}")
            return 1
        except Exception as e:
            console.print(f"[red]✗[/red] Failed to create tenant: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(create_tenant_async())
    sys.exit(result)


@tenant_group.command("show")
@click.argument("tenant_name", type=click.Choice(["work", "personal", "family"]), required=False)
def show_tenant(tenant_name: Optional[str]):
    """Show details about a tenant context or current tenant."""
    async def show_tenant_async():
        try:
            if tenant_name:
                # Show specific tenant info (placeholder - would need tenant lookup)
                console.print(f"[blue]Tenant:[/blue] {tenant_name}")
                console.print("[dim]Detailed tenant info not yet implemented[/dim]")
            else:
                # Show current tenant
                session_info = await tenant_manager.get_current_tenant()

                if session_info:
                    console.print("[green]✓[/green] Current tenant context:")
                    console.print(f"  Tenant: [bold]{session_info['tenant_display_name']}[/bold] ({session_info['tenant_name']})")
                    console.print(f"  User: [blue]{session_info['user_email']}[/blue]")
                    console.print(f"  Session ID: [dim]{session_info['session_id'][:8]}...[/dim]")
                    console.print(f"  Last Activity: [dim]{session_info['last_activity']}[/dim]")
                    console.print(f"  Expires: [dim]{session_info['expires_at']}[/dim]")
                else:
                    console.print("[yellow]⚠[/yellow] No active tenant session")
                    console.print("Use [bold]penf tenant switch <name>[/bold] to activate a context")

            return 0

        except Exception as e:
            console.print(f"[red]✗[/red] Failed to show tenant: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(show_tenant_async())
    sys.exit(result)


@tenant_group.command("clear")
def clear_context():
    """Clear current tenant context (switch to superuser mode)."""
    async def clear_context_async():
        try:
            await tenant_manager.clear_tenant_context()
            console.print("[green]✓[/green] Cleared tenant context")
            console.print("[dim]CLI operations will now have superuser access to all data[/dim]")
            console.print("[dim]Use [bold]penf tenant switch <name>[/bold] to activate a tenant context[/dim]")
            return 0

        except Exception as e:
            console.print(f"[red]✗[/red] Failed to clear context: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(clear_context_async())
    sys.exit(result)


@tenant_group.command("extend")
@click.option("--hours", type=int, default=24, help="Hours to extend session (default: 24)")
def extend_session(hours: int):
    """Extend current session expiration."""
    async def extend_session_async():
        try:
            success = await tenant_manager.extend_session(hours)

            if success:
                console.print(f"[green]✓[/green] Extended session by {hours} hours")
            else:
                console.print("[yellow]⚠[/yellow] No active session to extend")
                console.print("Use [bold]penf tenant switch <name>[/bold] to start a session")

            return 0 if success else 1

        except Exception as e:
            console.print(f"[red]✗[/red] Failed to extend session: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(extend_session_async())
    sys.exit(result)


@tenant_group.command("cleanup")
def cleanup_sessions():
    """Clean up expired tenant sessions."""
    async def cleanup_sessions_async():
        try:
            count = await tenant_manager.cleanup_sessions()

            if count > 0:
                console.print(f"[green]✓[/green] Cleaned up {count} expired sessions")
            else:
                console.print("[blue]ℹ[/blue] No expired sessions found")

            return 0

        except Exception as e:
            console.print(f"[red]✗[/red] Failed to cleanup sessions: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(cleanup_sessions_async())
    sys.exit(result)


@tenant_group.group("person")
def person_group():
    """Manage cross-tenant person linking."""
    pass


@person_group.command("link")
@click.argument("canonical_person_id")
@click.option("--tenant-mappings", multiple=True, help="Tenant:person_id mappings (e.g., work:123)")
@click.option("--method", default="manual", help="Link method (manual, email_match, ai_suggested)")
@click.option("--validated-by", help="Email of person validating the link")
def link_person(
    canonical_person_id: str,
    tenant_mappings: tuple,
    method: str,
    validated_by: Optional[str]
):
    """Link a person across multiple tenant contexts."""
    async def link_person_async():
        try:
            if not tenant_mappings:
                console.print("[red]✗[/red] At least one tenant mapping is required")
                console.print("Example: --tenant-mappings work:123 --tenant-mappings personal:456")
                return 1

            # Parse tenant mappings
            parsed_mappings = []
            for mapping in tenant_mappings:
                try:
                    tenant_name, person_id_str = mapping.split(":", 1)
                    person_id = int(person_id_str)
                    parsed_mappings.append((tenant_name, person_id))
                except ValueError:
                    console.print(f"[red]✗[/red] Invalid mapping format: {mapping}")
                    console.print("Expected format: tenant_name:person_id")
                    return 1

            # Create links
            links = await tenant_manager.link_person_across_tenants(
                canonical_person_id=canonical_person_id,
                tenant_person_mappings=parsed_mappings,
                link_method=method,
                validated_by=validated_by
            )

            console.print(f"[green]✓[/green] Created {len(links)} person links for: [bold]{canonical_person_id}[/bold]")

            for link in links:
                console.print(f"  {link['tenant_name']}: person {link['person_id']} (confidence: {link['link_confidence']:.3f})")

            return 0

        except ValueError as e:
            console.print(f"[red]✗[/red] {e}")
            return 1
        except Exception as e:
            console.print(f"[red]✗[/red] Failed to link person: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(link_person_async())
    sys.exit(result)


@person_group.command("show")
@click.argument("canonical_person_id")
def show_person_links(canonical_person_id: str):
    """Show all tenant contexts where a person appears."""
    async def show_person_links_async():
        try:
            persons = await tenant_manager.get_linked_persons(canonical_person_id)

            if not persons:
                console.print(f"[yellow]⚠[/yellow] No person links found for: [bold]{canonical_person_id}[/bold]")
                return 0

            console.print(f"[blue]Person links for:[/blue] [bold]{canonical_person_id}[/bold]\n")

            table = Table(title=f"Cross-Tenant Person: {canonical_person_id}")
            table.add_column("Tenant", style="blue")
            table.add_column("Person ID", justify="right")
            table.add_column("Name", style="bold")
            table.add_column("Emails", style="dim")
            table.add_column("Confidence", justify="center")
            table.add_column("Method", style="dim")

            for person in persons:
                emails_str = ", ".join(person["email_addresses"][:2])  # Show first 2 emails
                if len(person["email_addresses"]) > 2:
                    emails_str += "..."

                table.add_row(
                    person["tenant_name"],
                    str(person["person_id"]),
                    person["canonical_name"],
                    emails_str,
                    f"{person['link_confidence']:.3f}",
                    person["link_method"]
                )

            console.print(table)
            return 0

        except Exception as e:
            console.print(f"[red]✗[/red] Failed to show person links: {e}")
            return 1
        finally:
            await cleanup_connections()

    result = asyncio.run(show_person_links_async())
    sys.exit(result)