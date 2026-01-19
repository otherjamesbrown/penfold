"""Batch Processing for Manual Ingest.

Handles batch processing of .eml files with:
- Progress tracking using Rich
- Job checkpoint for resume capability (FR-009)
- Error isolation (FR-006)
- Event publishing for AI pipeline

Reference: specs/012-manual-ingest/research.md Section 3
"""

from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Optional
from uuid import UUID

from rich.console import Console
from rich.progress import (
    BarColumn,
    MofNCompleteColumn,
    Progress,
    TextColumn,
    TimeElapsedColumn,
)

from .models import (
    IngestError,
    IngestJobCreate,
    IngestJobStatus,
    IngestJobSummary,
    IngestResult,
    ParsedEmail,
)
from .parser import ParseError, parse_eml_file


@dataclass
class ProcessingResult:
    """Result of batch processing operation.

    Attributes:
        job_id: Unique job identifier
        imported: Count of successfully imported files
        skipped: Count of skipped duplicates
        failed: Count of failed files
        results: Per-file results
        errors: Error details for failed files
    """

    job_id: UUID
    imported: int = 0
    skipped: int = 0
    failed: int = 0
    results: list[IngestResult] = field(default_factory=list)
    errors: list[IngestError] = field(default_factory=list)
    labels_created: list[str] = field(default_factory=list)
    attachments_extracted: int = 0
    attachments_skipped: int = 0


class BatchProcessor:
    """Batch processor for .eml files with progress tracking.

    Processes files sequentially with:
    - Progress bar display
    - Per-file error isolation
    - Job checkpoint for resume
    - Event publishing

    Usage:
        processor = BatchProcessor(console)
        result = await processor.process_batch(files, source_tag, job_id)
    """

    def __init__(self, console: Optional[Console] = None):
        """Initialize batch processor.

        Args:
            console: Rich console for output (created if not provided)
        """
        self.console = console or Console()

    async def process_batch(
        self,
        job_config: IngestJobCreate,
        job_id: UUID,
        tenant_id: UUID,
        store_callback: Optional[Callable[[ParsedEmail, str, UUID], Any]] = None,
        duplicate_check: Optional[Callable[[str, str, UUID], bool]] = None,
        progress_callback: Optional[Callable[[UUID, str, int, int], None]] = None,
    ) -> ProcessingResult:
        """Process batch of .eml files.

        Args:
            job_config: Job configuration with file paths and options
            job_id: Unique job identifier
            tenant_id: Tenant ID for storage
            store_callback: Async function to store parsed email
            duplicate_check: Async function to check for duplicates
            progress_callback: Optional callback for progress updates

        Returns:
            ProcessingResult with counts and details
        """
        result = ProcessingResult(job_id=job_id)
        files = job_config.file_paths
        labels_seen: set[str] = set()

        with Progress(
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            MofNCompleteColumn(),
            TimeElapsedColumn(),
            console=self.console,
        ) as progress:
            task = progress.add_task("Processing emails", total=len(files))

            for file_path in files:
                file_result = await self._process_single_file(
                    file_path=file_path,
                    job_config=job_config,
                    tenant_id=tenant_id,
                    store_callback=store_callback,
                    duplicate_check=duplicate_check,
                    labels_seen=labels_seen,
                )

                result.results.append(file_result)

                if file_result.status == "imported":
                    result.imported += 1
                elif file_result.status == "skipped":
                    result.skipped += 1
                elif file_result.status == "failed":
                    result.failed += 1
                    if file_result.error:
                        result.errors.append(file_result.error)

                # Update progress
                progress.update(task, advance=1)

                # Call progress callback if provided
                if progress_callback:
                    await progress_callback(
                        job_id,
                        str(file_path),
                        result.imported + result.skipped + result.failed,
                        len(files),
                    )

        result.labels_created = sorted(labels_seen)
        return result

    async def _process_single_file(
        self,
        file_path: Path,
        job_config: IngestJobCreate,
        tenant_id: UUID,
        store_callback: Optional[Callable],
        duplicate_check: Optional[Callable],
        labels_seen: set[str],
    ) -> IngestResult:
        """Process a single .eml file.

        Args:
            file_path: Path to .eml file
            job_config: Job configuration
            tenant_id: Tenant ID
            store_callback: Storage function
            duplicate_check: Duplicate check function
            labels_seen: Set of labels for tracking

        Returns:
            IngestResult for this file
        """
        try:
            # Parse the file
            parsed = parse_eml_file(file_path)

            # Extract labels from folder structure if enabled
            if job_config.preserve_folders:
                labels = self._extract_labels(file_path, job_config.file_paths)
                labels_seen.update(labels)

            # Check for duplicates
            if duplicate_check:
                is_duplicate = await duplicate_check(
                    parsed.message_id, parsed.content_hash, tenant_id
                )
                if is_duplicate:
                    return IngestResult(
                        file_path=str(file_path),
                        status="skipped",
                        message_id=parsed.message_id,
                    )

            # Dry run - don't persist
            if job_config.dry_run:
                return IngestResult(
                    file_path=str(file_path),
                    status="imported",
                    message_id=parsed.message_id,
                )

            # Store the email
            if store_callback:
                source_id = await store_callback(
                    parsed, job_config.source_tag, tenant_id
                )
                return IngestResult(
                    file_path=str(file_path),
                    status="imported",
                    source_id=source_id,
                    message_id=parsed.message_id,
                )

            return IngestResult(
                file_path=str(file_path),
                status="imported",
                message_id=parsed.message_id,
            )

        except ParseError as e:
            return IngestResult(
                file_path=str(file_path),
                status="failed",
                error=IngestError(
                    file_path=str(file_path),
                    error_type="parse_error",
                    error_message=str(e),
                ),
            )
        except FileNotFoundError:
            return IngestResult(
                file_path=str(file_path),
                status="failed",
                error=IngestError(
                    file_path=str(file_path),
                    error_type="io_error",
                    error_message="File not found",
                ),
            )
        except Exception as e:
            return IngestResult(
                file_path=str(file_path),
                status="failed",
                error=IngestError(
                    file_path=str(file_path),
                    error_type="unknown_error",
                    error_message=str(e),
                ),
            )

    def _extract_labels(
        self, file_path: Path, all_paths: list[Path]
    ) -> list[str]:
        """Extract labels from folder structure.

        Creates labels from the relative folder path of the file.

        Args:
            file_path: Path to the file
            all_paths: All file paths in batch (to find common root)

        Returns:
            List of label strings
        """
        # Find common parent
        if len(all_paths) == 1:
            return []

        try:
            # Get parent directories for folder-based labels
            parents = file_path.parent.parts
            # Skip root and current directory markers
            labels = [p for p in parents if p not in (".", "/", "")]
            return labels[-3:] if len(labels) > 3 else labels  # Limit depth
        except Exception:
            return []
