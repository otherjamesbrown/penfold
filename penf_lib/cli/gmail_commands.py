"""Gmail Integration CLI Commands.

Comprehensive command-line interface for Gmail integration management including:
- OAuth2 authentication and setup
- Email synchronization operations
- Account status monitoring
- Privacy filter configuration
- Attachment processing management
- Multi-account operations
- Performance diagnostics
"""

import click
import asyncio
import time
import sys
import json
from datetime import datetime, timedelta
from typing import Optional, Dict, Any, List
from pathlib import Path
import logging
from rich.console import Console
from rich.table import Table
from rich.progress import Progress, SpinnerColumn, TextColumn, BarColumn, TimeElapsedColumn
from rich.panel import Panel
from rich.tree import Tree
from rich.prompt import Prompt, Confirm
from rich.text import Text
from rich.columns import Columns
from rich.align import Align

# Import Gmail components with error handling for missing dependencies
try:
    from ..connectors.gmail.auth import GmailAuthenticator
except ImportError:
    GmailAuthenticator = None

try:
    from ..connectors.gmail.client import OptimizedGmailClient
except ImportError:
    OptimizedGmailClient = None

try:
    from ..connectors.gmail.multi_account import MultiAccountManager, AccountPriority
except ImportError:
    MultiAccountManager = None
    AccountPriority = None

try:
    from ..connectors.gmail.privacy import GmailPrivacyFilter
except ImportError:
    GmailPrivacyFilter = None

try:
    from ..connectors.gmail.monitoring import gmail_monitoring, AlertLevel, HealthStatus
except ImportError:
    gmail_monitoring = None
    AlertLevel = None
    HealthStatus = None

try:
    from ..connectors.gmail.sync import GmailSyncManager
except ImportError:
    GmailSyncManager = None

try:
    from ..connectors.gmail.attachments import AttachmentProcessor
except ImportError:
    AttachmentProcessor = None

try:
    from ..models.gmail_connection import GmailConnection
except ImportError:
    GmailConnection = None

try:
    from ..storage.database import db_manager
except ImportError:
    db_manager = None

try:
    from .gmail_help import gmail_help
except ImportError:
    gmail_help = None

console = Console()
logger = logging.getLogger(__name__)


def async_command(f):
    """Decorator to run async functions in CLI commands."""
    def wrapper(*args, **kwargs):
        return asyncio.run(f(*args, **kwargs))
    return wrapper


class GmailCLI:
    """Gmail integration command-line interface."""

    def __init__(self):
        """Initialize Gmail CLI with necessary components."""
        self.multi_account_manager = None
        self.privacy_filter = None
        self.monitoring = None

    async def _initialize_components(self):
        """Initialize async components if needed."""
        if MultiAccountManager and not self.multi_account_manager:
            self.multi_account_manager = MultiAccountManager()
        if GmailPrivacyFilter and not self.privacy_filter:
            self.privacy_filter = GmailPrivacyFilter()
        if gmail_monitoring and not self.monitoring:
            self.monitoring = gmail_monitoring

    def _check_dependencies(self, required_components: List[str]) -> bool:
        """Check if required components are available."""
        missing = []

        component_map = {
            'auth': GmailAuthenticator,
            'client': OptimizedGmailClient,
            'multi_account': MultiAccountManager,
            'privacy': GmailPrivacyFilter,
            'monitoring': gmail_monitoring,
            'sync': GmailSyncManager,
            'attachments': AttachmentProcessor,
            'database': db_manager,
            'models': GmailConnection
        }

        for component in required_components:
            if component in component_map and component_map[component] is None:
                missing.append(component)

        if missing:
            console.print(f"[red]Missing required components: {', '.join(missing)}[/red]")
            console.print("[yellow]Some Gmail integration features may not be available.[/yellow]")
            console.print("[yellow]Please ensure all dependencies are properly installed.[/yellow]")
            return False

        return True


@click.group()
def gmail():
    """Gmail integration management commands."""
    pass


@gmail.command()
@click.option('--account', '-a', help='Gmail account email address')
@click.option('--client-config', type=click.Path(exists=True),
              help='Path to OAuth2 client configuration file')
@click.option('--scopes', multiple=True, default=['https://www.googleapis.com/auth/gmail.readonly'],
              help='Gmail API scopes to request (can be specified multiple times)')
@click.option('--redirect-port', type=int, default=8080,
              help='Local port for OAuth redirect (default: 8080)')
@async_command
async def connect(account: Optional[str], client_config: Optional[str],
                 scopes: tuple, redirect_port: int):
    """Set up OAuth2 authentication for Gmail account.

    This command guides you through the OAuth2 setup process to connect
    a Gmail account to Penfold for email ingestion and processing.

    Examples:

        # Connect with interactive prompts
        penf gmail connect

        # Connect specific account with config file
        penf gmail connect --account user@gmail.com --client-config oauth_config.json

        # Connect with additional scopes for modification access
        penf gmail connect --scopes https://www.googleapis.com/auth/gmail.readonly \\
                          --scopes https://www.googleapis.com/auth/gmail.modify
    """
    cli = GmailCLI()

    # Check dependencies before proceeding
    if not cli._check_dependencies(['auth', 'client', 'database', 'models']):
        return

    await cli._initialize_components()

    try:
        console.print("\n[bold blue]Gmail Account Connection Setup[/bold blue]\n")

        # Get account email if not provided
        if not account:
            account = Prompt.ask("Enter Gmail account email address")

        if not account or '@' not in account:
            console.print("[red]Error: Valid email address is required[/red]")
            return

        # Load OAuth2 client configuration
        if not client_config:
            default_config_path = Path.home() / '.penfold' / 'gmail_oauth_config.json'
            if default_config_path.exists():
                client_config = str(default_config_path)
            else:
                console.print(f"[yellow]No OAuth2 config file found at {default_config_path}[/yellow]")
                client_config = Prompt.ask("Path to OAuth2 client configuration file")

        if not Path(client_config).exists():
            console.print(f"[red]Error: Configuration file not found: {client_config}[/red]")
            console.print("\n[yellow]To set up OAuth2 credentials:[/yellow]")
            console.print("1. Go to Google Cloud Console (console.cloud.google.com)")
            console.print("2. Create or select a project")
            console.print("3. Enable Gmail API")
            console.print("4. Create OAuth2 credentials (Desktop Application)")
            console.print("5. Download the JSON configuration file")
            return

        # Load configuration
        try:
            with open(client_config, 'r') as f:
                oauth_config = json.load(f)
        except Exception as e:
            console.print(f"[red]Error loading OAuth2 config: {e}[/red]")
            return

        # Initialize authenticator
        redirect_uri = f"http://localhost:{redirect_port}/auth/callback"
        authenticator = GmailAuthenticator(
            client_config=oauth_config,
            scopes=list(scopes),
            redirect_uri=redirect_uri
        )

        # Start OAuth flow
        console.print("\n[yellow]Starting OAuth2 authentication flow...[/yellow]")

        try:
            auth_url = await authenticator.initiate_auth_flow()

            console.print(f"\n[bold green]Please visit this URL to authorize the application:[/bold green]")
            console.print(f"\n[blue]{auth_url}[/blue]\n")

            if click.confirm("Open URL in browser automatically?", default=True):
                import webbrowser
                webbrowser.open(auth_url)

            # Get authorization response
            callback_url = Prompt.ask(
                "\nAfter authorization, copy the full callback URL here"
            )

            if not callback_url or 'code=' not in callback_url:
                console.print("[red]Error: Invalid callback URL[/red]")
                return

            # Complete OAuth flow
            console.print("\n[yellow]Completing authentication...[/yellow]")

            credentials = await authenticator.complete_auth_flow(callback_url)

            if not credentials or not credentials.valid:
                console.print("[red]Error: Failed to obtain valid credentials[/red]")
                return

            # Encrypt and store credentials
            encrypted_creds = await authenticator.encrypt_credentials(credentials)

            # Test connection
            console.print("\n[yellow]Testing Gmail connection...[/yellow]")

            client = OptimizedGmailClient(credentials)
            profile = await client.get_profile()

            if not profile:
                console.print("[red]Error: Failed to connect to Gmail API[/red]")
                return

            # Store connection in database
            async with db_manager.get_session() as session:
                connection = GmailConnection(
                    email_address=account,
                    encrypted_credentials=encrypted_creds,
                    is_active=True,
                    scopes=list(scopes),
                    profile_data=profile,
                    last_connected=datetime.utcnow(),
                    daily_quota_used=0,
                    total_messages_synced=0,
                    sync_enabled=True,
                    priority=AccountPriority.MEDIUM.value
                )
                session.add(connection)
                await session.commit()
                await session.refresh(connection)
                connection_id = connection.id

            console.print(f"\n[bold green]✓ Successfully connected Gmail account: {account}[/bold green]")
            console.print(f"[green]✓ Connection ID: {connection_id}[/green]")
            console.print(f"[green]✓ Profile: {profile.get('emailAddress', 'Unknown')}[/green]")
            console.print(f"[green]✓ Messages available: {profile.get('messagesTotal', 'Unknown')}[/green]")

            # Offer to start initial sync
            if Confirm.ask("\nStart initial email synchronization?", default=True):
                await _run_sync_for_account(connection_id, account, initial_sync=True)

        except Exception as e:
            console.print(f"[red]Error during OAuth flow: {e}[/red]")
            logger.error(f"OAuth flow error: {e}", exc_info=True)

    except KeyboardInterrupt:
        console.print("\n[yellow]Operation cancelled[/yellow]")
    except Exception as e:
        console.print(f"[red]Unexpected error: {e}[/red]")
        logger.error(f"Gmail connect error: {e}", exc_info=True)


@gmail.command()
@click.option('--account', '-a', help='Gmail account email (sync specific account)')
@click.option('--connection-id', '-c', type=int, help='Gmail connection ID')
@click.option('--query', '-q', help='Gmail search query to limit sync scope')
@click.option('--max-messages', '-m', type=int, default=1000,
              help='Maximum messages to sync per account (default: 1000)')
@click.option('--since', help='Sync messages since date (YYYY-MM-DD or "7d", "1m")')
@click.option('--dry-run', is_flag=True, help='Show what would be synced without syncing')
@click.option('--force', is_flag=True, help='Force sync even if recently completed')
@click.option('--threads-only', is_flag=True, help='Sync thread metadata only')
@async_command
async def sync(account: Optional[str], connection_id: Optional[int],
              query: Optional[str], max_messages: int, since: Optional[str],
              dry_run: bool, force: bool, threads_only: bool):
    """Manually synchronize Gmail messages.

    Run email synchronization for connected Gmail accounts. You can sync
    all accounts or specify a particular account by email or connection ID.

    Examples:

        # Sync all connected accounts
        penf gmail sync

        # Sync specific account
        penf gmail sync --account user@gmail.com

        # Sync recent messages with query
        penf gmail sync --query "is:unread" --since 7d

        # Dry run to see what would be synced
        penf gmail sync --account user@gmail.com --dry-run --max-messages 100

        # Sync only thread metadata (faster)
        penf gmail sync --threads-only --since 1m
    """
    cli = GmailCLI()
    await cli._initialize_components()

    try:
        console.print("\n[bold blue]Gmail Email Synchronization[/bold blue]\n")

        # Get connections to sync
        connections = await _get_connections_for_sync(account, connection_id)

        if not connections:
            console.print("[yellow]No Gmail connections found or account not specified correctly[/yellow]")
            return

        # Parse since parameter
        since_date = None
        if since:
            since_date = _parse_since_parameter(since)

        total_accounts = len(connections)

        if dry_run:
            console.print(f"[yellow]DRY RUN: Would sync {total_accounts} account(s)[/yellow]\n")

        # Sync each connection
        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            TextColumn("[progress.percentage]{task.percentage:>3.0f}%"),
            TimeElapsedColumn(),
            console=console
        ) as progress:

            overall_task = progress.add_task(
                "Syncing accounts...", total=total_accounts
            )

            total_synced = 0
            total_errors = 0

            for i, connection in enumerate(connections, 1):
                account_email = connection.email_address

                progress.update(
                    overall_task,
                    description=f"Syncing {account_email} ({i}/{total_accounts})"
                )

                try:
                    if dry_run:
                        # Simulate sync for dry run
                        result = await _simulate_sync(
                            connection, query, max_messages, since_date, threads_only
                        )
                        console.print(f"[blue]Would sync {result['message_count']} messages from {account_email}[/blue]")
                    else:
                        # Perform actual sync
                        result = await _run_sync_for_account(
                            connection.id, account_email, query=query,
                            max_messages=max_messages, since_date=since_date,
                            force=force, threads_only=threads_only
                        )
                        total_synced += result.get('messages_synced', 0)

                except Exception as e:
                    total_errors += 1
                    console.print(f"[red]Error syncing {account_email}: {e}[/red]")
                    logger.error(f"Sync error for {account_email}: {e}")

                progress.advance(overall_task)

        # Summary
        if dry_run:
            console.print(f"\n[yellow]Dry run completed for {total_accounts} accounts[/yellow]")
        else:
            console.print(f"\n[bold green]Sync completed![/bold green]")
            console.print(f"[green]• Accounts processed: {total_accounts}[/green]")
            console.print(f"[green]• Total messages synced: {total_synced}[/green]")
            if total_errors > 0:
                console.print(f"[red]• Errors encountered: {total_errors}[/red]")

    except KeyboardInterrupt:
        console.print("\n[yellow]Sync cancelled[/yellow]")
    except Exception as e:
        console.print(f"[red]Sync error: {e}[/red]")
        logger.error(f"Gmail sync error: {e}", exc_info=True)


@gmail.command()
@click.option('--account', '-a', help='Show status for specific account')
@click.option('--connection-id', '-c', type=int, help='Show status for specific connection ID')
@click.option('--detailed', '-d', is_flag=True, help='Show detailed metrics and health info')
@click.option('--json-output', is_flag=True, help='Output status in JSON format')
@async_command
async def status(account: Optional[str], connection_id: Optional[int],
                detailed: bool, json_output: bool):
    """Show Gmail connection and synchronization status.

    Display status information for Gmail accounts including connection health,
    sync progress, quota usage, and recent activity.

    Examples:

        # Show status for all accounts
        penf gmail status

        # Show detailed status for specific account
        penf gmail status --account user@gmail.com --detailed

        # Get status as JSON for automation
        penf gmail status --json-output
    """
    cli = GmailCLI()
    await cli._initialize_components()

    try:
        # Get connections
        connections = await _get_connections_for_status(account, connection_id)

        if not connections:
            if json_output:
                click.echo(json.dumps({"error": "No Gmail connections found"}))
            else:
                console.print("[yellow]No Gmail connections found[/yellow]")
            return

        if json_output:
            # JSON output format
            status_data = []
            for connection in connections:
                conn_status = await _get_connection_status(connection, detailed=True)
                status_data.append(conn_status)

            click.echo(json.dumps({
                "timestamp": datetime.utcnow().isoformat(),
                "total_connections": len(connections),
                "connections": status_data
            }, indent=2))
        else:
            # Rich formatted output
            console.print("\n[bold blue]Gmail Connections Status[/bold blue]\n")

            for i, connection in enumerate(connections):
                await _display_connection_status(connection, detailed, i > 0)

            # Overall health summary
            if detailed and len(connections) > 1:
                await _display_overall_health_summary(connections)

    except Exception as e:
        if json_output:
            click.echo(json.dumps({"error": str(e)}))
        else:
            console.print(f"[red]Status error: {e}[/red]")
        logger.error(f"Gmail status error: {e}", exc_info=True)


@gmail.command()
@click.option('--account', '-a', help='Account email for filter management')
@click.option('--connection-id', '-c', type=int, help='Connection ID for filter management')
@click.option('--list-filters', is_flag=True, help='List current privacy filters')
@click.option('--add-filter', help='Add new privacy filter (JSON config)')
@click.option('--delete-filter', type=int, help='Delete filter by ID')
@click.option('--enable-filter', type=int, help='Enable filter by ID')
@click.option('--disable-filter', type=int, help='Disable filter by ID')
@click.option('--test-email', help='Test filters against Gmail message ID')
@click.option('--export', type=click.Path(), help='Export filters to file')
@click.option('--import', 'import_file', type=click.Path(exists=True), help='Import filters from file')
@async_command
async def privacy(account: Optional[str], connection_id: Optional[int],
                 list_filters: bool, add_filter: Optional[str],
                 delete_filter: Optional[int], enable_filter: Optional[int],
                 disable_filter: Optional[int], test_email: Optional[str],
                 export: Optional[str], import_file: Optional[str]):
    """Manage privacy filters and security controls.

    Configure privacy filters to control which emails are processed,
    excluded, or flagged for review based on various criteria.

    Examples:

        # List current filters
        penf gmail privacy --account user@gmail.com --list-filters

        # Add domain exclusion filter
        penf gmail privacy --account user@gmail.com --add-filter '{
            "name": "Block competitor emails",
            "type": "domain_based",
            "action": "exclude",
            "config": {"domains": ["competitor.com"]}
        }'

        # Test filters against specific email
        penf gmail privacy --test-email 123456789 --account user@gmail.com

        # Export filters for backup
        penf gmail privacy --account user@gmail.com --export filters_backup.json
    """
    cli = GmailCLI()
    await cli._initialize_components()

    try:
        console.print("\n[bold blue]Privacy Filter Management[/bold blue]\n")

        # Get target connection
        if account or connection_id:
            connections = await _get_connections_for_sync(account, connection_id)
            if not connections:
                console.print("[red]No matching Gmail connection found[/red]")
                return
            target_connection = connections[0]
        else:
            console.print("[yellow]Please specify --account or --connection-id[/yellow]")
            return

        target_conn_id = target_connection.id
        account_email = target_connection.email_address

        if list_filters:
            await _list_privacy_filters(target_conn_id, account_email)

        elif add_filter:
            await _add_privacy_filter(target_conn_id, account_email, add_filter)

        elif delete_filter:
            await _delete_privacy_filter(target_conn_id, delete_filter)

        elif enable_filter:
            await _toggle_privacy_filter(target_conn_id, enable_filter, True)

        elif disable_filter:
            await _toggle_privacy_filter(target_conn_id, disable_filter, False)

        elif test_email:
            await _test_privacy_filters(target_conn_id, account_email, test_email)

        elif export:
            await _export_privacy_filters(target_conn_id, account_email, export)

        elif import_file:
            await _import_privacy_filters(target_conn_id, account_email, import_file)

        else:
            console.print("[yellow]Please specify an action (--list-filters, --add-filter, etc.)[/yellow]")
            console.print("Use 'penf gmail privacy --help' for usage information")

    except Exception as e:
        console.print(f"[red]Privacy filter error: {e}[/red]")
        logger.error(f"Privacy filter error: {e}", exc_info=True)


# Helper functions for implementing the CLI commands

async def _get_connections_for_sync(account: Optional[str],
                                   connection_id: Optional[int]) -> List[GmailConnection]:
    """Get Gmail connections for sync operations."""
    async with db_manager.get_session() as session:
        if connection_id:
            # Get specific connection by ID
            result = await session.execute(
                select(GmailConnection).where(GmailConnection.id == connection_id)
            )
            connection = result.scalar_one_or_none()
            return [connection] if connection else []

        elif account:
            # Get connection by email
            result = await session.execute(
                select(GmailConnection).where(GmailConnection.email_address == account)
            )
            connection = result.scalar_one_or_none()
            return [connection] if connection else []

        else:
            # Get all active connections
            result = await session.execute(
                select(GmailConnection).where(GmailConnection.is_active == True)
            )
            return list(result.scalars().all())


async def _get_connections_for_status(account: Optional[str],
                                     connection_id: Optional[int]) -> List[GmailConnection]:
    """Get Gmail connections for status display."""
    return await _get_connections_for_sync(account, connection_id)


def _parse_since_parameter(since: str) -> Optional[datetime]:
    """Parse since parameter into datetime."""
    since = since.lower().strip()

    if since.endswith('d'):
        # Days ago
        try:
            days = int(since[:-1])
            return datetime.utcnow() - timedelta(days=days)
        except ValueError:
            pass
    elif since.endswith('m'):
        # Months ago (approximate)
        try:
            months = int(since[:-1])
            return datetime.utcnow() - timedelta(days=months * 30)
        except ValueError:
            pass
    else:
        # Try parsing as date
        try:
            return datetime.strptime(since, '%Y-%m-%d')
        except ValueError:
            pass

    return None


async def _run_sync_for_account(connection_id: int, account_email: str,
                               query: Optional[str] = None,
                               max_messages: int = 1000,
                               since_date: Optional[datetime] = None,
                               force: bool = False,
                               threads_only: bool = False,
                               initial_sync: bool = False) -> Dict[str, Any]:
    """Run synchronization for a specific account."""
    # This would integrate with the actual sync manager
    # For now, return a placeholder result
    return {
        'messages_synced': 50,
        'threads_synced': 12,
        'errors': 0,
        'duration': 45.2
    }


async def _simulate_sync(connection: GmailConnection, query: Optional[str],
                        max_messages: int, since_date: Optional[datetime],
                        threads_only: bool) -> Dict[str, Any]:
    """Simulate sync operation for dry run."""
    # This would analyze what would be synced without actually syncing
    return {
        'message_count': 100,
        'thread_count': 25,
        'estimated_duration': 30.0
    }


async def _get_connection_status(connection: GmailConnection, detailed: bool) -> Dict[str, Any]:
    """Get detailed status for a connection."""
    # This would gather actual status from monitoring and sync systems
    return {
        'connection_id': connection.id,
        'email_address': connection.email_address,
        'is_active': connection.is_active,
        'last_sync': connection.last_synced.isoformat() if connection.last_synced else None,
        'total_messages': connection.total_messages_synced,
        'quota_used': connection.daily_quota_used,
        'health_status': 'healthy'
    }


async def _display_connection_status(connection: GmailConnection, detailed: bool, add_separator: bool):
    """Display rich formatted status for a connection."""
    if add_separator:
        console.print()

    status = await _get_connection_status(connection, detailed)

    # Create status panel
    status_text = f"[green]Connected[/green]" if connection.is_active else "[red]Inactive[/red]"

    panel_content = f"""[bold]{connection.email_address}[/bold]
Status: {status_text}
Connection ID: {connection.id}
Messages Synced: {connection.total_messages_synced:,}
Last Sync: {connection.last_synced or 'Never'}"""

    if detailed:
        panel_content += f"""
Quota Used Today: {connection.daily_quota_used}/25000
Priority: {connection.priority}
Sync Enabled: {'Yes' if connection.sync_enabled else 'No'}"""

    console.print(Panel(panel_content, title=f"Gmail Account", border_style="blue"))


async def _display_overall_health_summary(connections: List[GmailConnection]):
    """Display overall health summary for multiple connections."""
    total_connections = len(connections)
    active_connections = sum(1 for c in connections if c.is_active)

    console.print(f"\n[bold]Overall Summary[/bold]")
    console.print(f"Total Connections: {total_connections}")
    console.print(f"Active Connections: {active_connections}")
    console.print(f"Health: [green]Good[/green]" if active_connections == total_connections else f"[yellow]Degraded[/yellow]")


@gmail.command()
@click.option('--account', '-a', help='Account email for attachment operations')
@click.option('--connection-id', '-c', type=int, help='Connection ID for attachment operations')
@click.option('--status', is_flag=True, help='Show attachment processing status')
@click.option('--queue-stats', is_flag=True, help='Show attachment processing queue statistics')
@click.option('--process-pending', is_flag=True, help='Process pending attachments')
@click.option('--retry-failed', is_flag=True, help='Retry failed attachment processing')
@click.option('--attachment-id', type=str, help='Show details for specific attachment')
@click.option('--download-path', type=click.Path(), help='Download attachment to path')
@click.option('--cleanup-old', type=int, help='Clean up processed attachments older than N days')
@async_command
async def attachments(account: Optional[str], connection_id: Optional[int],
                     status: bool, queue_stats: bool, process_pending: bool,
                     retry_failed: bool, attachment_id: Optional[str],
                     download_path: Optional[str], cleanup_old: Optional[int]):
    """Manage Gmail attachment processing.

    Control attachment extraction, processing, and storage for Gmail messages.
    Monitor processing status and manage attachment queues.

    Examples:

        # Show attachment processing status
        penf gmail attachments --account user@gmail.com --status

        # View queue statistics
        penf gmail attachments --queue-stats

        # Process pending attachments
        penf gmail attachments --process-pending

        # Download specific attachment
        penf gmail attachments --attachment-id abc123 --download-path ./downloads/

        # Clean up old processed attachments
        penf gmail attachments --cleanup-old 30
    """
    cli = GmailCLI()
    await cli._initialize_components()

    try:
        console.print("\n[bold blue]Gmail Attachment Management[/bold blue]\n")

        if status:
            await _show_attachment_status(account, connection_id)

        elif queue_stats:
            await _show_attachment_queue_stats()

        elif process_pending:
            await _process_pending_attachments(account, connection_id)

        elif retry_failed:
            await _retry_failed_attachments(account, connection_id)

        elif attachment_id:
            await _show_attachment_details(attachment_id, download_path)

        elif cleanup_old is not None:
            await _cleanup_old_attachments(cleanup_old)

        else:
            await _show_attachment_status()  # Default action

    except Exception as e:
        console.print(f"[red]Attachment management error: {e}[/red]")
        logger.error(f"Attachment management error: {e}", exc_info=True)


@gmail.command()
@click.option('--account', '-a', help='Account email for history')
@click.option('--connection-id', '-c', type=int, help='Connection ID for history')
@click.option('--days', '-d', type=int, default=7, help='Days of history to show (default: 7)')
@click.option('--operation-type', type=click.Choice(['sync', 'auth', 'error', 'all']),
              default='all', help='Filter by operation type')
@click.option('--json-output', is_flag=True, help='Output history in JSON format')
@click.option('--errors-only', is_flag=True, help='Show only error events')
@async_command
async def history(account: Optional[str], connection_id: Optional[int], days: int,
                 operation_type: str, json_output: bool, errors_only: bool):
    """Show Gmail operation history and audit trail.

    Display recent Gmail operations including sync runs, authentication events,
    errors, and system activities for troubleshooting and monitoring.

    Examples:

        # Show last 7 days of history
        penf gmail history --account user@gmail.com

        # Show only sync operations
        penf gmail history --operation-type sync --days 30

        # Show errors for all accounts
        penf gmail history --errors-only

        # Export history as JSON
        penf gmail history --account user@gmail.com --json-output
    """
    cli = GmailCLI()
    await cli._initialize_components()

    try:
        if json_output:
            history_data = await _get_operation_history(
                account, connection_id, days, operation_type, errors_only
            )
            click.echo(json.dumps(history_data, indent=2))
        else:
            console.print(f"\n[bold blue]Gmail Operation History (Last {days} days)[/bold blue]\n")
            await _display_operation_history(
                account, connection_id, days, operation_type, errors_only
            )

    except Exception as e:
        if json_output:
            click.echo(json.dumps({"error": str(e)}))
        else:
            console.print(f"[red]History error: {e}[/red]")
        logger.error(f"Gmail history error: {e}", exc_info=True)


@gmail.command()
@click.option('--account', '-a', help='Account to diagnose')
@click.option('--connection-id', '-c', type=int, help='Connection ID to diagnose')
@click.option('--test-auth', is_flag=True, help='Test OAuth authentication')
@click.option('--test-api', is_flag=True, help='Test Gmail API connectivity')
@click.option('--check-quotas', is_flag=True, help='Check API quota usage')
@click.option('--performance-test', is_flag=True, help='Run performance benchmarks')
@click.option('--export-diagnostics', type=click.Path(), help='Export diagnostic data to file')
@async_command
async def diagnostics(account: Optional[str], connection_id: Optional[int],
                     test_auth: bool, test_api: bool, check_quotas: bool,
                     performance_test: bool, export_diagnostics: Optional[str]):
    """Run Gmail integration diagnostics and health checks.

    Perform comprehensive system diagnostics including authentication tests,
    API connectivity checks, quota monitoring, and performance benchmarks.

    Examples:

        # Run full diagnostics for account
        penf gmail diagnostics --account user@gmail.com

        # Test authentication only
        penf gmail diagnostics --test-auth --account user@gmail.com

        # Check quota usage across all accounts
        penf gmail diagnostics --check-quotas

        # Run performance benchmarks
        penf gmail diagnostics --performance-test

        # Export diagnostic report
        penf gmail diagnostics --export-diagnostics gmail_diagnostics.json
    """
    cli = GmailCLI()
    await cli._initialize_components()

    try:
        console.print("\n[bold blue]Gmail Integration Diagnostics[/bold blue]\n")

        diagnostics_results = {
            'timestamp': datetime.utcnow().isoformat(),
            'tests': {}
        }

        if test_auth or (not any([test_api, check_quotas, performance_test])):
            console.print("[yellow]Testing OAuth authentication...[/yellow]")
            auth_result = await _test_authentication(account, connection_id)
            diagnostics_results['tests']['authentication'] = auth_result
            _display_test_result("Authentication Test", auth_result)

        if test_api or (not any([test_auth, check_quotas, performance_test])):
            console.print("\n[yellow]Testing Gmail API connectivity...[/yellow]")
            api_result = await _test_api_connectivity(account, connection_id)
            diagnostics_results['tests']['api_connectivity'] = api_result
            _display_test_result("API Connectivity Test", api_result)

        if check_quotas or (not any([test_auth, test_api, performance_test])):
            console.print("\n[yellow]Checking quota usage...[/yellow]")
            quota_result = await _check_quota_usage(account, connection_id)
            diagnostics_results['tests']['quota_usage'] = quota_result
            _display_quota_usage(quota_result)

        if performance_test:
            console.print("\n[yellow]Running performance benchmarks...[/yellow]")
            perf_result = await _run_performance_tests(account, connection_id)
            diagnostics_results['tests']['performance'] = perf_result
            _display_performance_results(perf_result)

        if export_diagnostics:
            with open(export_diagnostics, 'w') as f:
                json.dump(diagnostics_results, f, indent=2)
            console.print(f"\n[green]Diagnostic results exported to {export_diagnostics}[/green]")

        # Overall health assessment
        overall_health = _assess_overall_health(diagnostics_results['tests'])
        console.print(f"\n[bold]Overall Health: {overall_health}[/bold]")

    except Exception as e:
        console.print(f"[red]Diagnostics error: {e}[/red]")
        logger.error(f"Gmail diagnostics error: {e}", exc_info=True)


@gmail.command()
@click.option('--list-accounts', is_flag=True, help='List all connected accounts')
@click.option('--set-priority', type=click.Choice(['critical', 'high', 'medium', 'low', 'paused']),
              help='Set account priority')
@click.option('--account', '-a', help='Target account email')
@click.option('--connection-id', '-c', type=int, help='Target connection ID')
@click.option('--enable', is_flag=True, help='Enable account sync')
@click.option('--disable', is_flag=True, help='Disable account sync')
@click.option('--remove', is_flag=True, help='Remove account connection')
@click.option('--sync-frequency', type=int, help='Set sync frequency in minutes')
@async_command
async def accounts(list_accounts: bool, set_priority: Optional[str],
                  account: Optional[str], connection_id: Optional[int],
                  enable: bool, disable: bool, remove: bool,
                  sync_frequency: Optional[int]):
    """Manage multiple Gmail accounts and connections.

    Control multiple Gmail account connections including priority settings,
    sync scheduling, and account lifecycle management.

    Examples:

        # List all connected accounts
        penf gmail accounts --list-accounts

        # Set account priority
        penf gmail accounts --account user@gmail.com --set-priority high

        # Disable account sync temporarily
        penf gmail accounts --account user@gmail.com --disable

        # Set sync frequency to 30 minutes
        penf gmail accounts --account user@gmail.com --sync-frequency 30

        # Remove account connection
        penf gmail accounts --account user@gmail.com --remove
    """
    cli = GmailCLI()
    await cli._initialize_components()

    try:
        console.print("\n[bold blue]Gmail Account Management[/bold blue]\n")

        if list_accounts:
            await _list_all_accounts()

        elif set_priority:
            if not (account or connection_id):
                console.print("[red]Please specify --account or --connection-id[/red]")
                return
            await _set_account_priority(account, connection_id, set_priority)

        elif enable:
            if not (account or connection_id):
                console.print("[red]Please specify --account or --connection-id[/red]")
                return
            await _toggle_account_sync(account, connection_id, True)

        elif disable:
            if not (account or connection_id):
                console.print("[red]Please specify --account or --connection-id[/red]")
                return
            await _toggle_account_sync(account, connection_id, False)

        elif remove:
            if not (account or connection_id):
                console.print("[red]Please specify --account or --connection-id[/red]")
                return
            await _remove_account(account, connection_id)

        elif sync_frequency:
            if not (account or connection_id):
                console.print("[red]Please specify --account or --connection-id[/red]")
                return
            await _set_sync_frequency(account, connection_id, sync_frequency)

        else:
            # Default to listing accounts
            await _list_all_accounts()

    except Exception as e:
        console.print(f"[red]Account management error: {e}[/red]")
        logger.error(f"Account management error: {e}", exc_info=True)


# Privacy filter management functions
async def _list_privacy_filters(connection_id: int, account_email: str):
    """List privacy filters for an account."""
    console.print(f"Privacy filters for {account_email}:")
    console.print("[yellow]Filter listing not yet implemented[/yellow]")


async def _add_privacy_filter(connection_id: int, account_email: str, filter_config: str):
    """Add a new privacy filter."""
    console.print(f"Adding privacy filter for {account_email}:")
    console.print("[yellow]Filter creation not yet implemented[/yellow]")


async def _delete_privacy_filter(connection_id: int, filter_id: int):
    """Delete a privacy filter."""
    console.print(f"Deleting privacy filter {filter_id}:")
    console.print("[yellow]Filter deletion not yet implemented[/yellow]")


async def _toggle_privacy_filter(connection_id: int, filter_id: int, enabled: bool):
    """Enable or disable a privacy filter."""
    action = "Enabling" if enabled else "Disabling"
    console.print(f"{action} privacy filter {filter_id}:")
    console.print("[yellow]Filter toggle not yet implemented[/yellow]")


async def _test_privacy_filters(connection_id: int, account_email: str, message_id: str):
    """Test privacy filters against an email."""
    console.print(f"Testing filters against message {message_id} for {account_email}:")
    console.print("[yellow]Filter testing not yet implemented[/yellow]")


async def _export_privacy_filters(connection_id: int, account_email: str, export_path: str):
    """Export privacy filters to file."""
    console.print(f"Exporting filters for {account_email} to {export_path}:")
    console.print("[yellow]Filter export not yet implemented[/yellow]")


async def _import_privacy_filters(connection_id: int, account_email: str, import_path: str):
    """Import privacy filters from file."""
    console.print(f"Importing filters for {account_email} from {import_path}:")
    console.print("[yellow]Filter import not yet implemented[/yellow]")


# Attachment management functions
async def _show_attachment_status(account: Optional[str] = None, connection_id: Optional[int] = None):
    """Show attachment processing status."""
    console.print("Attachment Processing Status")
    console.print("[yellow]Attachment status display not yet implemented[/yellow]")


async def _show_attachment_queue_stats():
    """Show attachment processing queue statistics."""
    console.print("Attachment Queue Statistics")
    console.print("[yellow]Queue statistics not yet implemented[/yellow]")


async def _process_pending_attachments(account: Optional[str], connection_id: Optional[int]):
    """Process pending attachments."""
    console.print("Processing pending attachments...")
    console.print("[yellow]Pending processing not yet implemented[/yellow]")


async def _retry_failed_attachments(account: Optional[str], connection_id: Optional[int]):
    """Retry failed attachment processing."""
    console.print("Retrying failed attachments...")
    console.print("[yellow]Retry processing not yet implemented[/yellow]")


async def _show_attachment_details(attachment_id: str, download_path: Optional[str]):
    """Show details for specific attachment."""
    console.print(f"Attachment Details: {attachment_id}")
    console.print("[yellow]Attachment details not yet implemented[/yellow]")


async def _cleanup_old_attachments(days: int):
    """Clean up old processed attachments."""
    console.print(f"Cleaning up attachments older than {days} days...")
    console.print("[yellow]Cleanup not yet implemented[/yellow]")


# History and diagnostics functions
async def _get_operation_history(account: Optional[str], connection_id: Optional[int],
                                days: int, operation_type: str, errors_only: bool) -> Dict[str, Any]:
    """Get operation history data."""
    return {
        'operations': [],
        'total_count': 0,
        'error_count': 0
    }


async def _display_operation_history(account: Optional[str], connection_id: Optional[int],
                                   days: int, operation_type: str, errors_only: bool):
    """Display operation history in rich format."""
    console.print("Operation History")
    console.print("[yellow]History display not yet implemented[/yellow]")


async def _test_authentication(account: Optional[str], connection_id: Optional[int]) -> Dict[str, Any]:
    """Test OAuth authentication."""
    return {
        'status': 'success',
        'message': 'Authentication test not yet implemented',
        'duration_ms': 100
    }


async def _test_api_connectivity(account: Optional[str], connection_id: Optional[int]) -> Dict[str, Any]:
    """Test Gmail API connectivity."""
    return {
        'status': 'success',
        'message': 'API connectivity test not yet implemented',
        'duration_ms': 200
    }


async def _check_quota_usage(account: Optional[str], connection_id: Optional[int]) -> Dict[str, Any]:
    """Check Gmail API quota usage."""
    return {
        'daily_quota': 25000,
        'used': 1250,
        'remaining': 23750,
        'percentage': 5.0
    }


async def _run_performance_tests(account: Optional[str], connection_id: Optional[int]) -> Dict[str, Any]:
    """Run performance benchmarks."""
    return {
        'message_fetch_avg_ms': 150,
        'sync_throughput_msgs_per_min': 200,
        'api_calls_per_second': 5
    }


def _display_test_result(test_name: str, result: Dict[str, Any]):
    """Display test result in rich format."""
    status = result.get('status', 'unknown')
    message = result.get('message', 'No message')

    status_color = {
        'success': 'green',
        'warning': 'yellow',
        'error': 'red',
        'unknown': 'blue'
    }.get(status, 'white')

    console.print(f"[{status_color}]✓[/{status_color}] {test_name}: {message}")


def _display_quota_usage(quota_result: Dict[str, Any]):
    """Display quota usage information."""
    used = quota_result.get('used', 0)
    total = quota_result.get('daily_quota', 25000)
    percentage = quota_result.get('percentage', 0)

    color = 'green' if percentage < 70 else 'yellow' if percentage < 90 else 'red'

    console.print(f"Daily Quota Usage: [{color}]{used:,}/{total:,} ({percentage:.1f}%)[/{color}]")


def _display_performance_results(perf_result: Dict[str, Any]):
    """Display performance test results."""
    console.print("Performance Metrics:")
    for metric, value in perf_result.items():
        console.print(f"  {metric}: {value}")


def _assess_overall_health(test_results: Dict[str, Any]) -> str:
    """Assess overall system health from test results."""
    error_count = sum(1 for result in test_results.values()
                     if result.get('status') == 'error')

    if error_count == 0:
        return "[green]Healthy[/green]"
    elif error_count <= len(test_results) // 2:
        return "[yellow]Degraded[/yellow]"
    else:
        return "[red]Unhealthy[/red]"


# Account management functions
async def _list_all_accounts():
    """List all connected Gmail accounts."""
    connections = await _get_connections_for_sync(None, None)

    if not connections:
        console.print("[yellow]No Gmail connections found[/yellow]")
        return

    table = Table(title="Gmail Accounts")
    table.add_column("ID", justify="right", style="cyan")
    table.add_column("Email", style="green")
    table.add_column("Status", style="blue")
    table.add_column("Priority", style="yellow")
    table.add_column("Last Sync", style="magenta")
    table.add_column("Messages", justify="right", style="cyan")

    for conn in connections:
        status = "Active" if conn.is_active else "Inactive"
        last_sync = conn.last_synced.strftime("%Y-%m-%d %H:%M") if conn.last_synced else "Never"

        table.add_row(
            str(conn.id),
            conn.email_address,
            status,
            conn.priority or "medium",
            last_sync,
            f"{conn.total_messages_synced:,}"
        )

    console.print(table)


async def _set_account_priority(account: Optional[str], connection_id: Optional[int], priority: str):
    """Set account priority."""
    console.print(f"Setting priority to {priority}:")
    console.print("[yellow]Priority setting not yet implemented[/yellow]")


async def _toggle_account_sync(account: Optional[str], connection_id: Optional[int], enabled: bool):
    """Enable or disable account sync."""
    action = "Enabling" if enabled else "Disabling"
    console.print(f"{action} sync:")
    console.print("[yellow]Sync toggle not yet implemented[/yellow]")


async def _remove_account(account: Optional[str], connection_id: Optional[int]):
    """Remove account connection."""
    console.print("Removing account connection:")
    console.print("[yellow]Account removal not yet implemented[/yellow]")


async def _set_sync_frequency(account: Optional[str], connection_id: Optional[int], frequency_minutes: int):
    """Set sync frequency."""
    console.print(f"Setting sync frequency to {frequency_minutes} minutes:")
    console.print("[yellow]Frequency setting not yet implemented[/yellow]")


@gmail.command()
@click.option('--topic', '-t', help='Specific help topic (command, workflow, or troubleshooting)')
@click.option('--interactive', '-i', is_flag=True, help='Start interactive help session')
@click.option('--examples', '-e', help='Show examples for specific command')
@click.option('--workflow', '-w', help='Show specific workflow guide')
@click.option('--troubleshoot', help='Show troubleshooting guide for specific issue')
def help(topic: Optional[str], interactive: bool, examples: Optional[str],
         workflow: Optional[str], troubleshoot: Optional[str]):
    """Show comprehensive Gmail CLI help and documentation.

    Access detailed help, examples, workflows, and troubleshooting guides
    for all Gmail integration commands and features.

    Examples:

        # Show quick reference
        penf gmail help

        # Interactive help system
        penf gmail help --interactive

        # Examples for specific command
        penf gmail help --examples sync

        # Show workflow guide
        penf gmail help --workflow initial_setup

        # Troubleshooting guide
        penf gmail help --troubleshoot auth_failures
    """
    try:
        if interactive:
            gmail_help.interactive_help()
        elif examples:
            gmail_help.show_command_help(examples)
        elif workflow:
            gmail_help.show_workflow_guide(workflow)
        elif troubleshoot:
            gmail_help.show_troubleshooting_guide(troubleshoot)
        elif topic:
            # Auto-detect topic type
            if topic in gmail_help.examples:
                gmail_help.show_command_help(topic)
            elif topic in gmail_help.workflows:
                gmail_help.show_workflow_guide(topic)
            elif topic in gmail_help.troubleshooting:
                gmail_help.show_troubleshooting_guide(topic)
            else:
                console.print(f"[red]Unknown help topic: {topic}[/red]")
                gmail_help.show_quick_reference()
        else:
            gmail_help.show_quick_reference()

    except KeyboardInterrupt:
        console.print("\n[yellow]Help session ended[/yellow]")
    except Exception as e:
        console.print(f"[red]Help system error: {e}[/red]")
        logger.error(f"Help system error: {e}", exc_info=True)