"""Integration tests for Gmail CLI commands.

Tests the command-line interface for Gmail integration including:
- Command parsing and argument validation
- Error handling and user feedback
- JSON output formats
- Interactive prompts and confirmations
- Integration with underlying Gmail components
"""

import pytest
import asyncio
import json
import tempfile
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch, mock_open
from click.testing import CliRunner
from datetime import datetime, timedelta

from penf_lib.cli.main import cli
from penf_lib.models.gmail_connection import GmailConnection


@pytest.fixture
def runner():
    """Create a Click CLI test runner."""
    return CliRunner()


@pytest.fixture
def mock_gmail_connection():
    """Create a mock Gmail connection."""
    connection = MagicMock(spec=GmailConnection)
    connection.id = 1
    connection.email_address = "test@gmail.com"
    connection.is_active = True
    connection.priority = "medium"
    connection.total_messages_synced = 1250
    connection.daily_quota_used = 500
    connection.sync_enabled = True
    connection.last_synced = datetime.utcnow() - timedelta(hours=2)
    return connection


@pytest.fixture
def mock_oauth_config():
    """Create mock OAuth2 configuration."""
    return {
        "installed": {
            "client_id": "test-client-id",
            "client_secret": "test-client-secret",
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token",
            "redirect_uris": ["http://localhost:8080"]
        }
    }


class TestGmailStatusCommand:
    """Test suite for 'penf gmail status' command."""

    @patch('penf_lib.cli.gmail_commands._get_connections_for_status')
    def test_status_no_connections(self, mock_get_connections, runner):
        """Test status command when no Gmail connections exist."""
        mock_get_connections.return_value = asyncio.create_task(
            asyncio.coroutine(lambda: [])()
        )

        result = runner.invoke(cli, ['gmail', 'status'])

        assert result.exit_code == 0
        assert "No Gmail connections found" in result.output

    @patch('penf_lib.cli.gmail_commands._get_connections_for_status')
    @patch('penf_lib.cli.gmail_commands._display_connection_status')
    def test_status_with_connections(self, mock_display_status, mock_get_connections,
                                   runner, mock_gmail_connection):
        """Test status command with active connections."""
        mock_get_connections.return_value = asyncio.create_task(
            asyncio.coroutine(lambda: [mock_gmail_connection])()
        )
        mock_display_status.return_value = asyncio.create_task(
            asyncio.coroutine(lambda *args: None)()
        )

        result = runner.invoke(cli, ['gmail', 'status'])

        assert result.exit_code == 0
        assert "Gmail Connections Status" in result.output

    @patch('penf_lib.cli.gmail_commands._get_connections_for_status')
    def test_status_json_output(self, mock_get_connections, runner, mock_gmail_connection):
        """Test status command with JSON output format."""
        mock_get_connections.return_value = asyncio.create_task(
            asyncio.coroutine(lambda: [mock_gmail_connection])()
        )

        with patch('penf_lib.cli.gmail_commands._get_connection_status') as mock_status:
            mock_status.return_value = asyncio.create_task(
                asyncio.coroutine(lambda *args: {
                    'connection_id': 1,
                    'email_address': 'test@gmail.com',
                    'is_active': True,
                    'health_status': 'healthy'
                })()
            )

            result = runner.invoke(cli, ['gmail', 'status', '--json-output'])

            assert result.exit_code == 0

            # Parse JSON output
            output_data = json.loads(result.output)
            assert output_data['total_connections'] == 1
            assert len(output_data['connections']) == 1
            assert output_data['connections'][0]['email_address'] == 'test@gmail.com'

    @patch('penf_lib.cli.gmail_commands._get_connections_for_status')
    def test_status_specific_account(self, mock_get_connections, runner, mock_gmail_connection):
        """Test status command for specific account."""
        mock_get_connections.return_value = asyncio.create_task(
            asyncio.coroutine(lambda: [mock_gmail_connection])()
        )

        result = runner.invoke(cli, ['gmail', 'status', '--account', 'test@gmail.com'])

        assert result.exit_code == 0
        mock_get_connections.assert_called_once()


class TestGmailConnectCommand:
    """Test suite for 'penf gmail connect' command."""

    @patch('builtins.open', mock_open(read_data='{"installed": {"client_id": "test"}}'))
    @patch('pathlib.Path.exists')
    @patch('penf_lib.cli.gmail_commands.GmailAuthenticator')
    def test_connect_with_config_file(self, mock_authenticator, mock_exists, runner):
        """Test connect command with OAuth config file."""
        mock_exists.return_value = True
        mock_auth_instance = AsyncMock()
        mock_authenticator.return_value = mock_auth_instance
        mock_auth_instance.initiate_auth_flow.return_value = "https://auth.example.com"

        with patch('click.confirm', return_value=False):
            result = runner.invoke(cli, [
                'gmail', 'connect',
                '--account', 'test@gmail.com',
                '--client-config', 'oauth_config.json'
            ])

        # Should not exit with error before OAuth flow
        assert "Starting OAuth2 authentication flow" in result.output

    def test_connect_missing_account(self, runner):
        """Test connect command with missing account parameter."""
        # Test interactive prompt for account (will fail in test environment)
        result = runner.invoke(cli, ['gmail', 'connect'], input='\n')
        assert result.exit_code == 0  # Command handles missing input gracefully

    @patch('pathlib.Path.exists')
    def test_connect_missing_oauth_config(self, mock_exists, runner):
        """Test connect command with missing OAuth config."""
        mock_exists.return_value = False

        result = runner.invoke(cli, [
            'gmail', 'connect',
            '--account', 'test@gmail.com',
            '--client-config', 'nonexistent.json'
        ])

        assert result.exit_code == 0
        assert "Configuration file not found" in result.output
        assert "Go to Google Cloud Console" in result.output


class TestGmailSyncCommand:
    """Test suite for 'penf gmail sync' command."""

    @patch('penf_lib.cli.gmail_commands._get_connections_for_sync')
    def test_sync_no_connections(self, mock_get_connections, runner):
        """Test sync command when no connections are available."""
        mock_get_connections.return_value = []

        result = runner.invoke(cli, ['gmail', 'sync'])

        assert result.exit_code == 0
        assert "No Gmail connections found" in result.output

    @patch('penf_lib.cli.gmail_commands._get_connections_for_sync')
    @patch('penf_lib.cli.gmail_commands._run_sync_for_account')
    def test_sync_single_account(self, mock_run_sync, mock_get_connections,
                                runner, mock_gmail_connection):
        """Test sync command for single account."""
        mock_get_connections.return_value = [mock_gmail_connection]
        mock_run_sync.return_value = {
            'messages_synced': 50,
            'threads_synced': 12,
            'errors': 0,
            'duration': 45.2
        }

        result = runner.invoke(cli, ['gmail', 'sync'])

        assert result.exit_code == 0
        assert "Gmail Email Synchronization" in result.output
        assert "Sync completed" in result.output

    @patch('penf_lib.cli.gmail_commands._get_connections_for_sync')
    @patch('penf_lib.cli.gmail_commands._simulate_sync')
    def test_sync_dry_run(self, mock_simulate_sync, mock_get_connections,
                         runner, mock_gmail_connection):
        """Test sync command with dry run option."""
        mock_get_connections.return_value = [mock_gmail_connection]
        mock_simulate_sync.return_value = {
            'message_count': 100,
            'thread_count': 25,
            'estimated_duration': 30.0
        }

        result = runner.invoke(cli, ['gmail', 'sync', '--dry-run'])

        assert result.exit_code == 0
        assert "DRY RUN" in result.output
        assert "Would sync" in result.output

    def test_sync_with_query_params(self, runner):
        """Test sync command with query parameters."""
        result = runner.invoke(cli, [
            'gmail', 'sync',
            '--query', 'is:unread',
            '--max-messages', '500',
            '--since', '7d',
            '--threads-only'
        ])

        # Should execute without error (will show no connections)
        assert result.exit_code == 0

    @patch('penf_lib.cli.gmail_commands._parse_since_parameter')
    def test_parse_since_parameter(self, mock_parse):
        """Test since parameter parsing."""
        from penf_lib.cli.gmail_commands import _parse_since_parameter

        # Test days format
        result = _parse_since_parameter('7d')
        assert isinstance(result, datetime)

        # Test months format
        result = _parse_since_parameter('1m')
        assert isinstance(result, datetime)

        # Test date format
        result = _parse_since_parameter('2023-01-15')
        assert isinstance(result, datetime)

        # Test invalid format
        result = _parse_since_parameter('invalid')
        assert result is None


class TestGmailPrivacyCommand:
    """Test suite for 'penf gmail privacy' command."""

    def test_privacy_no_account_specified(self, runner):
        """Test privacy command without account specification."""
        result = runner.invoke(cli, ['gmail', 'privacy', '--list-filters'])

        assert result.exit_code == 0
        assert "Please specify --account or --connection-id" in result.output

    @patch('penf_lib.cli.gmail_commands._get_connections_for_sync')
    @patch('penf_lib.cli.gmail_commands._list_privacy_filters')
    def test_privacy_list_filters(self, mock_list_filters, mock_get_connections,
                                 runner, mock_gmail_connection):
        """Test privacy command for listing filters."""
        mock_get_connections.return_value = [mock_gmail_connection]
        mock_list_filters.return_value = None

        result = runner.invoke(cli, [
            'gmail', 'privacy',
            '--account', 'test@gmail.com',
            '--list-filters'
        ])

        assert result.exit_code == 0
        mock_list_filters.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._get_connections_for_sync')
    @patch('penf_lib.cli.gmail_commands._add_privacy_filter')
    def test_privacy_add_filter(self, mock_add_filter, mock_get_connections,
                               runner, mock_gmail_connection):
        """Test privacy command for adding filters."""
        mock_get_connections.return_value = [mock_gmail_connection]
        mock_add_filter.return_value = None

        filter_config = '{"name": "test", "type": "domain_based", "config": {}}'

        result = runner.invoke(cli, [
            'gmail', 'privacy',
            '--account', 'test@gmail.com',
            '--add-filter', filter_config
        ])

        assert result.exit_code == 0
        mock_add_filter.assert_called_once()

    def test_privacy_no_action_specified(self, runner):
        """Test privacy command without action specified."""
        result = runner.invoke(cli, [
            'gmail', 'privacy',
            '--account', 'test@gmail.com'
        ])

        assert result.exit_code == 0
        assert "Please specify an action" in result.output


class TestGmailAttachmentsCommand:
    """Test suite for 'penf gmail attachments' command."""

    @patch('penf_lib.cli.gmail_commands._show_attachment_status')
    def test_attachments_default_status(self, mock_show_status, runner):
        """Test attachments command default behavior (show status)."""
        mock_show_status.return_value = None

        result = runner.invoke(cli, ['gmail', 'attachments'])

        assert result.exit_code == 0
        assert "Gmail Attachment Management" in result.output
        mock_show_status.assert_called()

    @patch('penf_lib.cli.gmail_commands._show_attachment_queue_stats')
    def test_attachments_queue_stats(self, mock_queue_stats, runner):
        """Test attachments command queue statistics."""
        mock_queue_stats.return_value = None

        result = runner.invoke(cli, ['gmail', 'attachments', '--queue-stats'])

        assert result.exit_code == 0
        mock_queue_stats.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._process_pending_attachments')
    def test_attachments_process_pending(self, mock_process_pending, runner):
        """Test attachments command process pending."""
        mock_process_pending.return_value = None

        result = runner.invoke(cli, ['gmail', 'attachments', '--process-pending'])

        assert result.exit_code == 0
        mock_process_pending.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._show_attachment_details')
    def test_attachments_show_details(self, mock_show_details, runner):
        """Test attachments command show specific attachment details."""
        mock_show_details.return_value = None

        result = runner.invoke(cli, [
            'gmail', 'attachments',
            '--attachment-id', 'abc123',
            '--download-path', './downloads/'
        ])

        assert result.exit_code == 0
        mock_show_details.assert_called_once_with('abc123', './downloads/')


class TestGmailHistoryCommand:
    """Test suite for 'penf gmail history' command."""

    @patch('penf_lib.cli.gmail_commands._display_operation_history')
    def test_history_display_format(self, mock_display_history, runner):
        """Test history command with display format."""
        mock_display_history.return_value = None

        result = runner.invoke(cli, [
            'gmail', 'history',
            '--account', 'test@gmail.com',
            '--days', '14'
        ])

        assert result.exit_code == 0
        assert "Gmail Operation History" in result.output
        mock_display_history.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._get_operation_history')
    def test_history_json_output(self, mock_get_history, runner):
        """Test history command with JSON output."""
        mock_get_history.return_value = {
            'operations': [],
            'total_count': 0,
            'error_count': 0
        }

        result = runner.invoke(cli, [
            'gmail', 'history',
            '--json-output',
            '--errors-only'
        ])

        assert result.exit_code == 0

        # Verify JSON output
        output_data = json.loads(result.output)
        assert 'operations' in output_data
        assert 'total_count' in output_data

    def test_history_operation_type_filter(self, runner):
        """Test history command with operation type filtering."""
        result = runner.invoke(cli, [
            'gmail', 'history',
            '--operation-type', 'sync',
            '--days', '7'
        ])

        assert result.exit_code == 0


class TestGmailDiagnosticsCommand:
    """Test suite for 'penf gmail diagnostics' command."""

    @patch('penf_lib.cli.gmail_commands._test_authentication')
    @patch('penf_lib.cli.gmail_commands._test_api_connectivity')
    @patch('penf_lib.cli.gmail_commands._check_quota_usage')
    def test_diagnostics_full_run(self, mock_quota, mock_api, mock_auth, runner):
        """Test diagnostics command running all tests."""
        mock_auth.return_value = {'status': 'success', 'message': 'Auth OK'}
        mock_api.return_value = {'status': 'success', 'message': 'API OK'}
        mock_quota.return_value = {'daily_quota': 25000, 'used': 1000, 'percentage': 4.0}

        result = runner.invoke(cli, [
            'gmail', 'diagnostics',
            '--account', 'test@gmail.com'
        ])

        assert result.exit_code == 0
        assert "Gmail Integration Diagnostics" in result.output
        assert "Overall Health" in result.output

        mock_auth.assert_called_once()
        mock_api.assert_called_once()
        mock_quota.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._test_authentication')
    def test_diagnostics_auth_only(self, mock_auth, runner):
        """Test diagnostics command with auth test only."""
        mock_auth.return_value = {'status': 'success', 'message': 'Auth OK'}

        result = runner.invoke(cli, [
            'gmail', 'diagnostics',
            '--test-auth',
            '--account', 'test@gmail.com'
        ])

        assert result.exit_code == 0
        mock_auth.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._run_performance_tests')
    def test_diagnostics_performance_test(self, mock_perf_test, runner):
        """Test diagnostics command with performance testing."""
        mock_perf_test.return_value = {
            'message_fetch_avg_ms': 150,
            'sync_throughput_msgs_per_min': 200
        }

        result = runner.invoke(cli, [
            'gmail', 'diagnostics',
            '--performance-test'
        ])

        assert result.exit_code == 0
        mock_perf_test.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._test_authentication')
    @patch('builtins.open', mock_open())
    @patch('json.dump')
    def test_diagnostics_export_results(self, mock_json_dump, mock_auth, runner):
        """Test diagnostics command with results export."""
        mock_auth.return_value = {'status': 'success', 'message': 'Auth OK'}

        result = runner.invoke(cli, [
            'gmail', 'diagnostics',
            '--test-auth',
            '--export-diagnostics', 'diagnostics.json'
        ])

        assert result.exit_code == 0
        assert "exported to diagnostics.json" in result.output
        mock_json_dump.assert_called_once()


class TestGmailAccountsCommand:
    """Test suite for 'penf gmail accounts' command."""

    @patch('penf_lib.cli.gmail_commands._list_all_accounts')
    def test_accounts_list_default(self, mock_list_accounts, runner):
        """Test accounts command default behavior (list accounts)."""
        mock_list_accounts.return_value = None

        result = runner.invoke(cli, ['gmail', 'accounts'])

        assert result.exit_code == 0
        mock_list_accounts.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._list_all_accounts')
    def test_accounts_list_explicit(self, mock_list_accounts, runner):
        """Test accounts command with explicit list flag."""
        mock_list_accounts.return_value = None

        result = runner.invoke(cli, ['gmail', 'accounts', '--list-accounts'])

        assert result.exit_code == 0
        mock_list_accounts.assert_called_once()

    @patch('penf_lib.cli.gmail_commands._set_account_priority')
    def test_accounts_set_priority(self, mock_set_priority, runner):
        """Test accounts command for setting priority."""
        mock_set_priority.return_value = None

        result = runner.invoke(cli, [
            'gmail', 'accounts',
            '--account', 'test@gmail.com',
            '--set-priority', 'high'
        ])

        assert result.exit_code == 0
        mock_set_priority.assert_called_once_with('test@gmail.com', None, 'high')

    def test_accounts_priority_no_account(self, runner):
        """Test accounts command set priority without account specified."""
        result = runner.invoke(cli, [
            'gmail', 'accounts',
            '--set-priority', 'high'
        ])

        assert result.exit_code == 0
        assert "Please specify --account or --connection-id" in result.output

    @patch('penf_lib.cli.gmail_commands._toggle_account_sync')
    def test_accounts_enable_disable(self, mock_toggle, runner):
        """Test accounts command enable/disable sync."""
        mock_toggle.return_value = None

        # Test enable
        result = runner.invoke(cli, [
            'gmail', 'accounts',
            '--account', 'test@gmail.com',
            '--enable'
        ])

        assert result.exit_code == 0
        mock_toggle.assert_called_with('test@gmail.com', None, True)

        # Test disable
        result = runner.invoke(cli, [
            'gmail', 'accounts',
            '--account', 'test@gmail.com',
            '--disable'
        ])

        assert result.exit_code == 0


class TestCLIErrorHandling:
    """Test suite for CLI error handling and edge cases."""

    def test_invalid_command(self, runner):
        """Test handling of invalid Gmail subcommands."""
        result = runner.invoke(cli, ['gmail', 'invalid-command'])

        assert result.exit_code != 0
        assert "No such command" in result.output

    def test_missing_required_options(self, runner):
        """Test handling of missing required options."""
        # Test privacy command without account
        result = runner.invoke(cli, ['gmail', 'privacy', '--add-filter', '{}'])

        assert result.exit_code == 0  # Graceful handling
        assert "Please specify --account" in result.output

    @patch('penf_lib.cli.gmail_commands._get_connections_for_sync')
    def test_database_connection_error(self, mock_get_connections, runner):
        """Test handling of database connection errors."""
        mock_get_connections.side_effect = Exception("Database connection failed")

        result = runner.invoke(cli, ['gmail', 'status'])

        assert result.exit_code == 0  # CLI should handle gracefully
        assert "error" in result.output.lower()

    def test_malformed_json_in_filter(self, runner):
        """Test handling of malformed JSON in filter configuration."""
        result = runner.invoke(cli, [
            'gmail', 'privacy',
            '--account', 'test@gmail.com',
            '--add-filter', 'invalid-json{'
        ])

        # Should handle gracefully without crashing
        assert result.exit_code == 0


class TestCLIHelp:
    """Test suite for CLI help and documentation."""

    def test_main_help(self, runner):
        """Test main Gmail command help."""
        result = runner.invoke(cli, ['gmail', '--help'])

        assert result.exit_code == 0
        assert "Gmail integration management commands" in result.output

    def test_subcommand_help(self, runner):
        """Test subcommand help messages."""
        commands = ['status', 'connect', 'sync', 'privacy', 'attachments', 'history', 'diagnostics', 'accounts']

        for cmd in commands:
            result = runner.invoke(cli, ['gmail', cmd, '--help'])

            assert result.exit_code == 0
            assert "Examples:" in result.output or "help" in result.output.lower()

    def test_command_examples_in_help(self, runner):
        """Test that command examples are included in help."""
        result = runner.invoke(cli, ['gmail', 'sync', '--help'])

        assert result.exit_code == 0
        assert "Examples:" in result.output
        assert "penf gmail sync" in result.output

    def test_option_help_text(self, runner):
        """Test that option help text is informative."""
        result = runner.invoke(cli, ['gmail', 'status', '--help'])

        assert result.exit_code == 0
        assert "--account" in result.output
        assert "--detailed" in result.output
        assert "--json-output" in result.output


# Utility tests for helper functions
class TestUtilityFunctions:
    """Test suite for utility functions used in CLI commands."""

    def test_parse_since_parameter_days(self):
        """Test parsing of 'days ago' since parameter."""
        from penf_lib.cli.gmail_commands import _parse_since_parameter

        result = _parse_since_parameter('7d')
        assert isinstance(result, datetime)
        assert result < datetime.utcnow()

        # Test that it's approximately 7 days ago
        days_diff = (datetime.utcnow() - result).days
        assert 6 <= days_diff <= 8  # Allow for some variance

    def test_parse_since_parameter_months(self):
        """Test parsing of 'months ago' since parameter."""
        from penf_lib.cli.gmail_commands import _parse_since_parameter

        result = _parse_since_parameter('2m')
        assert isinstance(result, datetime)

        # Approximately 60 days ago (2 months * 30 days)
        days_diff = (datetime.utcnow() - result).days
        assert 55 <= days_diff <= 65

    def test_parse_since_parameter_date(self):
        """Test parsing of specific date since parameter."""
        from penf_lib.cli.gmail_commands import _parse_since_parameter

        result = _parse_since_parameter('2023-12-01')
        assert isinstance(result, datetime)
        assert result.year == 2023
        assert result.month == 12
        assert result.day == 1

    def test_parse_since_parameter_invalid(self):
        """Test parsing of invalid since parameter."""
        from penf_lib.cli.gmail_commands import _parse_since_parameter

        assert _parse_since_parameter('invalid') is None
        assert _parse_since_parameter('1z') is None
        assert _parse_since_parameter('2023-13-01') is None  # Invalid date


if __name__ == '__main__':
    pytest.main([__file__])