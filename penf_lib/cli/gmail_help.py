"""Gmail CLI Help Documentation and Examples.

Comprehensive help system for Gmail integration CLI commands including
detailed usage examples, common workflows, and troubleshooting guides.
"""

from typing import Dict, List
from rich.console import Console
from rich.panel import Panel
from rich.columns import Columns
from rich.syntax import Syntax
from rich.table import Table
from rich.text import Text
from rich.tree import Tree


console = Console()


class GmailHelpSystem:
    """Gmail CLI help system with interactive guides and examples."""

    def __init__(self):
        """Initialize the Gmail help system."""
        self.examples = self._load_examples()
        self.workflows = self._load_workflows()
        self.troubleshooting = self._load_troubleshooting()

    def _load_examples(self) -> Dict[str, Dict[str, str]]:
        """Load command examples organized by command."""
        return {
            'connect': {
                'basic': '''# Connect Gmail account with interactive setup
penf gmail connect

# Connect with specific OAuth config file
penf gmail connect --account user@gmail.com --client-config ~/.penfold/oauth_config.json

# Connect with additional scopes for email modification
penf gmail connect --scopes https://www.googleapis.com/auth/gmail.readonly \\
                   --scopes https://www.googleapis.com/auth/gmail.modify''',

                'advanced': '''# Connect with custom redirect port
penf gmail connect --account user@work.com --redirect-port 9000

# Connect multiple accounts sequentially
penf gmail connect --account personal@gmail.com
penf gmail connect --account work@company.com
penf gmail connect --account project@nonprofit.org''',

                'automation': '''# Non-interactive setup (requires pre-configured OAuth)
export GMAIL_ACCOUNT="user@gmail.com"
export OAUTH_CONFIG="/path/to/oauth.json"
penf gmail connect --account $GMAIL_ACCOUNT --client-config $OAUTH_CONFIG'''
            },

            'sync': {
                'basic': '''# Sync all connected accounts
penf gmail sync

# Sync specific account
penf gmail sync --account user@gmail.com

# Sync with message limit
penf gmail sync --max-messages 500''',

                'filtered': '''# Sync only unread emails from last week
penf gmail sync --query "is:unread" --since 7d

# Sync emails from specific sender
penf gmail sync --query "from:boss@company.com" --since 1m

# Sync thread metadata only (faster)
penf gmail sync --threads-only --since 30d''',

                'advanced': '''# Dry run to preview sync scope
penf gmail sync --account user@gmail.com --dry-run --max-messages 1000

# Force sync even if recently completed
penf gmail sync --force --query "has:attachment"

# Sync with specific date range
penf gmail sync --since 2023-12-01 --max-messages 2000'''
            },

            'status': {
                'basic': '''# Show status for all accounts
penf gmail status

# Show detailed status with metrics
penf gmail status --detailed

# Show status for specific account
penf gmail status --account user@gmail.com''',

                'monitoring': '''# Get status as JSON for automation
penf gmail status --json-output

# Monitor specific connection
penf gmail status --connection-id 1 --detailed

# Status check in scripts
penf gmail status --json-output | jq '.connections[].health_status' '''
            },

            'privacy': {
                'basic': '''# List current privacy filters
penf gmail privacy --account user@gmail.com --list-filters

# Add domain exclusion filter
penf gmail privacy --account user@gmail.com --add-filter '{
  "name": "Block competitor emails",
  "type": "domain_based",
  "action": "exclude",
  "config": {"domains": ["competitor.com"]}
}'

# Test filters against specific email
penf gmail privacy --account user@gmail.com --test-email 123456789''',

                'advanced': '''# Add content pattern filter
penf gmail privacy --account user@gmail.com --add-filter '{
  "name": "Sensitive content filter",
  "type": "content_pattern",
  "action": "quarantine",
  "config": {
    "patterns": ["confidential", "ssn:", "password:"],
    "case_sensitive": false
  }
}'

# Import filters from backup
penf gmail privacy --account user@gmail.com --import filters_backup.json

# Export current filters
penf gmail privacy --account user@gmail.com --export privacy_filters.json'''
            },

            'attachments': {
                'basic': '''# Show attachment processing status
penf gmail attachments --status

# View processing queue statistics
penf gmail attachments --queue-stats

# Process pending attachments
penf gmail attachments --process-pending''',

                'management': '''# Show details for specific attachment
penf gmail attachments --attachment-id abc123def456

# Download attachment to local directory
penf gmail attachments --attachment-id abc123def456 --download-path ./downloads/

# Retry failed attachment processing
penf gmail attachments --retry-failed

# Clean up old processed attachments
penf gmail attachments --cleanup-old 30'''
            },

            'history': {
                'basic': '''# Show last 7 days of operations
penf gmail history

# Show 30 days of sync operations only
penf gmail history --operation-type sync --days 30

# Show error events only
penf gmail history --errors-only''',

                'analysis': '''# Export history for analysis
penf gmail history --account user@gmail.com --json-output > history.json

# View authentication events
penf gmail history --operation-type auth --days 14

# Audit trail for specific account
penf gmail history --account user@gmail.com --days 90 --json-output'''
            },

            'diagnostics': {
                'basic': '''# Run full diagnostics
penf gmail diagnostics

# Test specific account authentication
penf gmail diagnostics --account user@gmail.com --test-auth

# Check quota usage across all accounts
penf gmail diagnostics --check-quotas''',

                'performance': '''# Run performance benchmarks
penf gmail diagnostics --performance-test

# Export diagnostic report
penf gmail diagnostics --export-diagnostics gmail_health_report.json

# Comprehensive health check
penf gmail diagnostics --account user@gmail.com --test-auth --test-api --check-quotas'''
            },

            'accounts': {
                'basic': '''# List all connected accounts
penf gmail accounts --list-accounts

# Set account priority
penf gmail accounts --account user@gmail.com --set-priority high

# Temporarily disable account sync
penf gmail accounts --account user@gmail.com --disable''',

                'management': '''# Set sync frequency to 30 minutes
penf gmail accounts --account user@gmail.com --sync-frequency 30

# Re-enable disabled account
penf gmail accounts --account user@gmail.com --enable

# Remove account connection (with confirmation)
penf gmail accounts --account old@company.com --remove'''
            }
        }

    def _load_workflows(self) -> Dict[str, Dict[str, str]]:
        """Load common workflow patterns."""
        return {
            'initial_setup': {
                'title': 'Initial Gmail Setup Workflow',
                'description': 'Complete setup process for new Gmail integration',
                'steps': '''1. Prepare OAuth2 credentials:
   - Go to Google Cloud Console
   - Create project and enable Gmail API
   - Create OAuth2 credentials (Desktop Application)
   - Download JSON configuration

2. Connect your Gmail account:
   penf gmail connect --account your@gmail.com --client-config oauth_config.json

3. Run initial synchronization:
   penf gmail sync --account your@gmail.com --max-messages 1000

4. Verify connection status:
   penf gmail status --account your@gmail.com --detailed

5. Set up privacy filters (optional):
   penf gmail privacy --account your@gmail.com --list-filters'''
            },

            'daily_operations': {
                'title': 'Daily Gmail Operations',
                'description': 'Regular maintenance and monitoring tasks',
                'steps': '''1. Check account health:
   penf gmail status --detailed

2. Run incremental sync:
   penf gmail sync --since 1d

3. Monitor quota usage:
   penf gmail diagnostics --check-quotas

4. Process any pending attachments:
   penf gmail attachments --process-pending

5. Review recent errors:
   penf gmail history --errors-only --days 1'''
            },

            'troubleshooting': {
                'title': 'Troubleshooting Common Issues',
                'description': 'Diagnose and resolve common Gmail integration problems',
                'steps': '''1. Run comprehensive diagnostics:
   penf gmail diagnostics --account your@gmail.com

2. Check authentication status:
   penf gmail diagnostics --test-auth --account your@gmail.com

3. Verify API connectivity:
   penf gmail diagnostics --test-api --account your@gmail.com

4. Review error history:
   penf gmail history --errors-only --days 7

5. Test with dry run sync:
   penf gmail sync --account your@gmail.com --dry-run --max-messages 10'''
            },

            'multi_account': {
                'title': 'Multi-Account Management',
                'description': 'Managing multiple Gmail accounts efficiently',
                'steps': '''1. List all connected accounts:
   penf gmail accounts --list-accounts

2. Set account priorities:
   penf gmail accounts --account primary@gmail.com --set-priority critical
   penf gmail accounts --account secondary@gmail.com --set-priority medium

3. Configure sync frequencies:
   penf gmail accounts --account primary@gmail.com --sync-frequency 15
   penf gmail accounts --account archive@gmail.com --sync-frequency 60

4. Sync high-priority accounts:
   penf gmail sync --account primary@gmail.com

5. Monitor all accounts:
   penf gmail status --detailed'''
            },

            'privacy_setup': {
                'title': 'Privacy Filter Configuration',
                'description': 'Set up comprehensive privacy controls',
                'steps': '''1. Review current filters:
   penf gmail privacy --account your@gmail.com --list-filters

2. Add domain exclusion filter:
   penf gmail privacy --account your@gmail.com --add-filter '{
     "name": "Block spam domains",
     "type": "domain_based",
     "action": "exclude",
     "config": {"domains": ["spam.com", "ads.net"]}
   }'

3. Add sensitive content filter:
   penf gmail privacy --account your@gmail.com --add-filter '{
     "name": "Detect PII",
     "type": "content_pattern",
     "action": "mark_sensitive",
     "config": {"patterns": ["ssn:", "password:", "credit card"]}
   }'

4. Test filters:
   penf gmail privacy --account your@gmail.com --test-email message_id

5. Export filter backup:
   penf gmail privacy --account your@gmail.com --export privacy_backup.json'''
            },

            'automation': {
                'title': 'Automation and Scripting',
                'description': 'Automate Gmail operations with scripts',
                'steps': '''1. Status monitoring script:
   #!/bin/bash
   STATUS=$(penf gmail status --json-output)
   if echo "$STATUS" | jq -e '.connections[].health_status | select(. != "healthy")'; then
     echo "Alert: Gmail connection unhealthy"
   fi

2. Automated sync with error handling:
   penf gmail sync --json-output > sync_results.json
   if jq -e '.error' sync_results.json; then
     echo "Sync failed, running diagnostics..."
     penf gmail diagnostics --export-diagnostics error_report.json
   fi

3. Quota monitoring:
   QUOTA=$(penf gmail diagnostics --check-quotas --json-output)
   if echo "$QUOTA" | jq '.percentage > 80'; then
     echo "Warning: Quota usage above 80%"
   fi'''
            }
        }

    def _load_troubleshooting(self) -> Dict[str, Dict[str, str]]:
        """Load troubleshooting guides."""
        return {
            'auth_failures': {
                'problem': 'OAuth2 Authentication Failures',
                'symptoms': 'Error messages like "Invalid credentials" or "Authentication failed"',
                'solutions': '''1. Check OAuth2 configuration:
   - Verify client_id and client_secret are correct
   - Ensure redirect_uri matches OAuth2 settings
   - Check that Gmail API is enabled in Google Cloud Console

2. Re-authenticate account:
   penf gmail connect --account your@gmail.com --client-config oauth_config.json

3. Test authentication:
   penf gmail diagnostics --test-auth --account your@gmail.com

4. Check token expiration:
   penf gmail status --account your@gmail.com --detailed'''
            },

            'quota_exceeded': {
                'problem': 'Gmail API Quota Exceeded',
                'symptoms': 'Sync failures with "Quota exceeded" or rate limiting errors',
                'solutions': '''1. Check current quota usage:
   penf gmail diagnostics --check-quotas

2. Reduce sync frequency:
   penf gmail accounts --account your@gmail.com --sync-frequency 60

3. Limit sync scope:
   penf gmail sync --account your@gmail.com --max-messages 100 --since 1d

4. Prioritize important accounts:
   penf gmail accounts --account important@gmail.com --set-priority high
   penf gmail accounts --account archive@gmail.com --set-priority low'''
            },

            'sync_issues': {
                'problem': 'Email Synchronization Problems',
                'symptoms': 'Incomplete syncs, missing messages, or sync timeouts',
                'solutions': '''1. Run diagnostic tests:
   penf gmail diagnostics --account your@gmail.com

2. Test with dry run:
   penf gmail sync --account your@gmail.com --dry-run --max-messages 10

3. Check for filtering issues:
   penf gmail privacy --account your@gmail.com --list-filters

4. Sync incrementally:
   penf gmail sync --account your@gmail.com --since 1d --max-messages 500

5. Review sync history:
   penf gmail history --operation-type sync --days 7'''
            },

            'performance_issues': {
                'problem': 'Slow Performance or Timeouts',
                'symptoms': 'Long sync times, timeouts, or high resource usage',
                'solutions': '''1. Run performance benchmarks:
   penf gmail diagnostics --performance-test

2. Use thread-only sync for speed:
   penf gmail sync --threads-only --since 7d

3. Reduce batch sizes:
   penf gmail sync --max-messages 100

4. Check system resources and network connectivity

5. Monitor attachment processing:
   penf gmail attachments --queue-stats'''
            },

            'attachment_problems': {
                'problem': 'Attachment Processing Issues',
                'symptoms': 'Failed downloads, processing errors, or missing attachments',
                'solutions': '''1. Check attachment status:
   penf gmail attachments --status

2. View processing queue:
   penf gmail attachments --queue-stats

3. Retry failed processing:
   penf gmail attachments --retry-failed

4. Clean up old attachments:
   penf gmail attachments --cleanup-old 30

5. Check disk space and file permissions'''
            }
        }

    def show_command_help(self, command: str) -> None:
        """Show detailed help for specific command."""
        if command not in self.examples:
            console.print(f"[red]No help available for command: {command}[/red]")
            return

        console.print(f"\n[bold blue]Gmail {command.title()} Command Help[/bold blue]\n")

        examples = self.examples[command]

        for category, example_code in examples.items():
            console.print(Panel(
                Syntax(example_code, "bash", theme="github-dark"),
                title=f"{category.title()} Examples",
                border_style="blue"
            ))
            console.print()

    def show_workflow_guide(self, workflow: str) -> None:
        """Show workflow guide."""
        if workflow not in self.workflows:
            console.print(f"[red]No workflow guide for: {workflow}[/red]")
            return

        guide = self.workflows[workflow]

        console.print(f"\n[bold green]{guide['title']}[/bold green]")
        console.print(f"[italic]{guide['description']}[/italic]\n")

        console.print(Panel(
            Text(guide['steps'], style="white"),
            title="Steps",
            border_style="green"
        ))

    def show_troubleshooting_guide(self, issue: str = None) -> None:
        """Show troubleshooting guide."""
        if issue and issue in self.troubleshooting:
            guide = self.troubleshooting[issue]

            console.print(f"\n[bold red]Troubleshooting: {guide['problem']}[/bold red]")
            console.print(f"[yellow]Symptoms:[/yellow] {guide['symptoms']}\n")

            console.print(Panel(
                Text(guide['solutions'], style="white"),
                title="Solutions",
                border_style="red"
            ))
        else:
            console.print("\n[bold red]Gmail Troubleshooting Guide[/bold red]\n")

            tree = Tree("Common Issues")
            for issue_key, guide in self.troubleshooting.items():
                tree.add(f"[red]{guide['problem']}[/red] - {issue_key}")

            console.print(tree)
            console.print("\n[yellow]Use: penf gmail help troubleshoot <issue_key>[/yellow]")

    def show_quick_reference(self) -> None:
        """Show quick reference card."""
        console.print("\n[bold blue]Gmail CLI Quick Reference[/bold blue]\n")

        # Command summary table
        table = Table(title="Commands", show_header=True, header_style="bold blue")
        table.add_column("Command", style="cyan", no_wrap=True)
        table.add_column("Description", style="white")
        table.add_column("Quick Example", style="yellow")

        commands_ref = [
            ("connect", "Set up OAuth2 authentication", "penf gmail connect"),
            ("sync", "Synchronize email messages", "penf gmail sync --since 7d"),
            ("status", "Show connection status", "penf gmail status --detailed"),
            ("privacy", "Manage privacy filters", "penf gmail privacy --list-filters"),
            ("attachments", "Manage attachments", "penf gmail attachments --status"),
            ("history", "View operation history", "penf gmail history --errors-only"),
            ("diagnostics", "Run health checks", "penf gmail diagnostics --test-auth"),
            ("accounts", "Manage multiple accounts", "penf gmail accounts --list-accounts")
        ]

        for cmd, desc, example in commands_ref:
            table.add_row(cmd, desc, example)

        console.print(table)

        # Common options
        console.print("\n[bold]Common Options:[/bold]")
        options_columns = [
            "[cyan]--account[/cyan] EMAIL\n[cyan]--connection-id[/cyan] ID\n[cyan]--json-output[/cyan]",
            "[cyan]--detailed[/cyan]\n[cyan]--dry-run[/cyan]\n[cyan]--help[/cyan]"
        ]
        console.print(Columns(options_columns, equal=True))

    def interactive_help(self) -> None:
        """Start interactive help system."""
        console.print("\n[bold blue]Gmail CLI Interactive Help System[/bold blue]\n")

        while True:
            console.print("[yellow]Available help topics:[/yellow]")
            console.print("1. Command help (connect, sync, status, etc.)")
            console.print("2. Workflow guides (setup, troubleshooting, etc.)")
            console.print("3. Troubleshooting specific issues")
            console.print("4. Quick reference")
            console.print("5. Exit help")

            choice = input("\nSelect option (1-5): ").strip()

            if choice == '1':
                cmd = input("Enter command name: ").strip().lower()
                self.show_command_help(cmd)
            elif choice == '2':
                console.print("\nAvailable workflows:")
                for workflow in self.workflows.keys():
                    console.print(f"  - {workflow}")
                workflow = input("Enter workflow name: ").strip()
                self.show_workflow_guide(workflow)
            elif choice == '3':
                self.show_troubleshooting_guide()
                issue = input("Enter issue key (or press Enter for all): ").strip()
                if issue:
                    self.show_troubleshooting_guide(issue)
            elif choice == '4':
                self.show_quick_reference()
            elif choice == '5':
                break
            else:
                console.print("[red]Invalid choice. Please select 1-5.[/red]")

            console.print("\n" + "="*50 + "\n")


# Global help instance
gmail_help = GmailHelpSystem()


# CLI command for accessing help
def show_help(topic: str = None, interactive: bool = False):
    """Show Gmail CLI help.

    Args:
        topic: Specific help topic (command, workflow, or troubleshooting)
        interactive: Start interactive help session
    """
    if interactive:
        gmail_help.interactive_help()
    elif topic:
        # Determine topic type and show appropriate help
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


if __name__ == "__main__":
    # Demo the help system
    gmail_help.show_quick_reference()
    gmail_help.show_command_help('sync')
    gmail_help.show_workflow_guide('initial_setup')