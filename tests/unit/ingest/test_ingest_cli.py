"""Unit tests for ingest CLI commands.

Tests CLI argument parsing and validation without database operations.
"""

import re
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from penf_lib.cli.ingest_commands import ingest_group


class TestIngestEmailCommand:
    """Tests for the `penf ingest email` command."""

    @pytest.fixture
    def runner(self):
        """Create CLI test runner."""
        return CliRunner()

    def test_command_requires_path(self, runner):
        """Path argument is required."""
        result = runner.invoke(ingest_group, ["email", "--source", "test"])
        assert result.exit_code != 0
        assert "Missing argument" in result.output or "Error" in result.output

    def test_command_requires_source(self, runner, tmp_path):
        """Source option is required."""
        # Create a test file
        test_file = tmp_path / "test.eml"
        test_file.write_text("From: test@example.com\nTo: other@example.com\n\nBody")

        result = runner.invoke(ingest_group, ["email", str(test_file)])
        assert result.exit_code != 0
        assert "Missing option" in result.output or "required" in result.output.lower()

    def test_invalid_source_tag_rejected(self, runner, tmp_path):
        """Source tag with invalid characters is rejected."""
        test_file = tmp_path / "test.eml"
        test_file.write_text("From: test@example.com\nTo: other@example.com\n\nBody")

        result = runner.invoke(
            ingest_group,
            ["email", str(test_file), "--source", "invalid source!@#"],
        )
        assert result.exit_code == 1
        assert "alphanumeric" in result.output.lower()

    def test_valid_source_tag_accepted(self, runner, tmp_path):
        """Valid source tags are accepted."""
        test_file = tmp_path / "test.eml"
        test_file.write_text("From: test@example.com\nTo: other@example.com\n\nBody")

        # Valid source tag should not trigger the "alphanumeric" error
        # The command will fail at DB connection, but we're testing tag validation
        result = runner.invoke(
            ingest_group,
            ["email", str(test_file), "--source", "valid-tag_123"],
        )
        # Should NOT contain the source tag validation error
        assert "alphanumeric" not in result.output.lower()

    def test_non_eml_file_rejected(self, runner, tmp_path):
        """Non-.eml files are rejected."""
        test_file = tmp_path / "test.txt"
        test_file.write_text("Some text")

        result = runner.invoke(
            ingest_group,
            ["email", str(test_file), "--source", "test"],
        )
        assert result.exit_code == 1
        assert ".eml" in result.output

    def test_nonexistent_path_rejected(self, runner):
        """Nonexistent paths are rejected."""
        result = runner.invoke(
            ingest_group,
            ["email", "/nonexistent/path/file.eml", "--source", "test"],
        )
        assert result.exit_code != 0

    def test_dry_run_shows_preview(self, runner, tmp_path):
        """Dry run shows file preview without processing."""
        # Create test files
        (tmp_path / "email1.eml").write_text(
            "From: test@example.com\nTo: other@example.com\n\nBody1"
        )
        (tmp_path / "email2.eml").write_text(
            "From: test@example.com\nTo: other@example.com\n\nBody2"
        )

        result = runner.invoke(
            ingest_group,
            ["email", str(tmp_path), "--source", "test", "--dry-run"],
        )
        assert "DRY RUN" in result.output
        assert "2" in result.output  # Should show 2 files

    def test_directory_scan_finds_eml_files(self, runner, tmp_path):
        """Directory scanning finds .eml files recursively."""
        # Create nested structure
        subdir = tmp_path / "inbox"
        subdir.mkdir()
        (tmp_path / "root.eml").write_text(
            "From: test@example.com\nTo: other@example.com\n\nRoot"
        )
        (subdir / "nested.eml").write_text(
            "From: test@example.com\nTo: other@example.com\n\nNested"
        )

        result = runner.invoke(
            ingest_group,
            ["email", str(tmp_path), "--source", "test", "--dry-run"],
        )
        assert "2" in result.output  # Should find both files

    def test_empty_directory_reports_no_files(self, runner, tmp_path):
        """Empty directory reports no files found."""
        result = runner.invoke(
            ingest_group,
            ["email", str(tmp_path), "--source", "test"],
        )
        assert result.exit_code == 0  # Clean exit
        assert "No .eml files" in result.output


class TestIngestJobsCommand:
    """Tests for the `penf ingest jobs` command."""

    @pytest.fixture
    def runner(self):
        """Create CLI test runner."""
        return CliRunner()

    def test_jobs_command_exists(self, runner):
        """Jobs command is registered."""
        result = runner.invoke(ingest_group, ["jobs", "--help"])
        assert result.exit_code == 0
        assert "List ingest jobs" in result.output

    def test_jobs_status_filter(self, runner):
        """Status filter option is available."""
        result = runner.invoke(ingest_group, ["jobs", "--help"])
        assert "--status" in result.output
        assert "pending" in result.output or "in_progress" in result.output

    def test_jobs_limit_option(self, runner):
        """Limit option is available."""
        result = runner.invoke(ingest_group, ["jobs", "--help"])
        assert "--limit" in result.output


class TestIngestResumeCommand:
    """Tests for the `penf ingest resume` command."""

    @pytest.fixture
    def runner(self):
        """Create CLI test runner."""
        return CliRunner()

    def test_resume_command_exists(self, runner):
        """Resume command is registered."""
        result = runner.invoke(ingest_group, ["resume", "--help"])
        assert result.exit_code == 0
        assert "Resume" in result.output

    def test_resume_requires_job_id(self, runner):
        """Resume command requires job ID argument."""
        result = runner.invoke(ingest_group, ["resume"])
        assert result.exit_code != 0
        assert "Missing argument" in result.output

    def test_invalid_job_id_format(self, runner):
        """Invalid job ID format is rejected."""
        # Mock get_session to avoid DB connection
        with patch("penf_lib.cli.ingest_commands.get_session", MagicMock()):
            result = runner.invoke(ingest_group, ["resume", "not-a-uuid"])
            assert result.exit_code == 1
            assert "Invalid job ID" in result.output


class TestSourceTagValidation:
    """Tests for source tag validation."""

    def test_valid_patterns(self):
        """Valid source tag patterns."""
        pattern = r"^[a-zA-Z0-9_-]+$"
        valid_tags = [
            "outlook",
            "outlook-2024",
            "archive_backup",
            "test123",
            "UPPERCASE",
            "mixed-Case_123",
        ]
        for tag in valid_tags:
            assert re.match(pattern, tag), f"Should accept: {tag}"

    def test_invalid_patterns(self):
        """Invalid source tag patterns."""
        pattern = r"^[a-zA-Z0-9_-]+$"
        invalid_tags = [
            "has space",
            "has@special",
            "has!exclaim",
            "",  # Empty
            "has/slash",
            "has.dot",
        ]
        for tag in invalid_tags:
            assert not re.match(pattern, tag), f"Should reject: {tag}"
