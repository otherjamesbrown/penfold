"""Manual Ingest CLI Commands.

Command-line interface for the manual content ingest framework.

Commands:
- penf ingest email <path> --source <tag> : Upload .eml files
- penf ingest --resume <job-id>           : Resume interrupted job
- penf ingest jobs [--status <status>]    : List ingest jobs

Reference: specs/012-manual-ingest/contracts/cli.md
"""

import asyncio
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional

import click
from rich.console import Console
from rich.panel import Panel
from rich.progress import Progress, SpinnerColumn, TextColumn
from rich.table import Table

# Import with error handling for optional dependencies
try:
    from ..ingest import parse_eml_file, ParseError
except ImportError:
    parse_eml_file = None
    ParseError = Exception

try:
    from ..storage.connections import get_session
except ImportError:
    get_session = None

try:
    from ..storage.repositories import SourceRepository, IngestJobRepository, IngestErrorRepository
except ImportError:
    SourceRepository = None
    IngestJobRepository = None
    IngestErrorRepository = None

try:
    from ..events.schemas import ManualEmailIngestedEvent
except ImportError:
    ManualEmailIngestedEvent = None

try:
    from ..events.publishers import ManualIngestEventPublisher
except ImportError:
    ManualIngestEventPublisher = None

console = Console()


@click.group(name="ingest")
def ingest_group():
    """Manual content ingest commands.

    Upload archived content files (emails, documents) into Penfold
    for unified search and AI processing.

    Examples:
        penf ingest email invoice.eml --source "outlook"
        penf ingest email ./emails/ --source "archive"
        penf ingest jobs --status in_progress
    """
    pass


@ingest_group.command(name="email")
@click.argument("path", type=click.Path(exists=True))
@click.option(
    "--source",
    "-s",
    required=True,
    help="Source identifier for tracking (required)",
)
@click.option(
    "--projects",
    default=None,
    help="Comma-separated project tags",
)
@click.option(
    "--dry-run",
    is_flag=True,
    default=False,
    help="Preview without importing",
)
@click.option(
    "--no-preserve-folders",
    is_flag=True,
    default=False,
    help="Don't create labels from folder structure",
)
@click.option(
    "--skip-attachments",
    is_flag=True,
    default=False,
    help="Don't extract attachments",
)
@click.option(
    "--verbose",
    "-v",
    is_flag=True,
    default=False,
    help="Show detailed progress",
)
def ingest_email(
    path: str,
    source: str,
    projects: Optional[str],
    dry_run: bool,
    no_preserve_folders: bool,
    skip_attachments: bool,
    verbose: bool,
):
    """Upload .eml files for processing.

    PATH can be a single file, directory, or glob pattern.

    Examples:

        # Single file
        penf ingest email invoice.eml --source "outlook"

        # Directory
        penf ingest email ./emails/ --source "outlook-2024"

        # Glob pattern
        penf ingest email "./**/*.eml" --source "backup"

        # Dry run
        penf ingest email ./emails/ --source "test" --dry-run
    """
    # Check required dependencies
    if parse_eml_file is None:
        console.print("[red]Error: Ingest module not available. Check dependencies.[/red]")
        raise SystemExit(1)

    if get_session is None:
        console.print("[red]Error: Database connection not configured.[/red]")
        raise SystemExit(1)

    # Validate source tag format
    import re
    if not re.match(r"^[a-zA-Z0-9_-]+$", source):
        console.print(
            "[red]Error: source tag must contain only alphanumeric characters, "
            "hyphens, and underscores[/red]"
        )
        raise SystemExit(1)

    # Resolve path and collect .eml files
    path_obj = Path(path)
    eml_files: list[Path] = []

    if path_obj.is_file():
        if path_obj.suffix.lower() != ".eml":
            console.print(f"[red]Error: {path} is not an .eml file[/red]")
            raise SystemExit(1)
        eml_files.append(path_obj)
    elif path_obj.is_dir():
        eml_files.extend(path_obj.rglob("*.eml"))
    else:
        # Try as glob pattern
        eml_files.extend(Path.cwd().glob(path))
        eml_files = [f for f in eml_files if f.suffix.lower() == ".eml"]

    if not eml_files:
        console.print(f"[yellow]No .eml files found at: {path}[/yellow]")
        raise SystemExit(0)

    # Display file count
    file_count = len(eml_files)
    console.print(
        Panel(
            f"Found [bold]{file_count}[/bold] .eml file{'s' if file_count > 1 else ''}\n"
            f"Source tag: [cyan]{source}[/cyan]",
            title="Manual Email Ingest",
        )
    )

    if dry_run:
        console.print("\n[yellow]DRY RUN - No changes will be made[/yellow]")
        for f in eml_files[:10]:
            console.print(f"  [dim]Would process:[/dim] {f.name}")
        if file_count > 10:
            console.print(f"  [dim]... and {file_count - 10} more files[/dim]")
        return

    # Parse project tags
    project_tags = []
    if projects:
        project_tags = [p.strip() for p in projects.split(",") if p.strip()]

    # Run the async processing
    asyncio.run(
        _process_email_files(
            eml_files=eml_files,
            source_tag=source,
            project_tags=project_tags,
            preserve_folders=not no_preserve_folders,
            skip_attachments=skip_attachments,
            verbose=verbose,
        )
    )


async def _process_email_files(
    eml_files: list[Path],
    source_tag: str,
    project_tags: list[str],
    preserve_folders: bool,
    skip_attachments: bool,
    verbose: bool,
) -> None:
    """Process email files asynchronously.

    Args:
        eml_files: List of .eml file paths
        source_tag: User-provided source identifier
        project_tags: Optional project tags
        preserve_folders: Whether to create labels from folder structure
        skip_attachments: Whether to skip attachment extraction
        verbose: Show detailed progress
    """
    # Get tenant ID (use default for now)
    tenant_id = uuid.UUID("00000000-0000-0000-0000-000000000001")  # Default tenant

    # Track results
    imported_count = 0
    skipped_count = 0
    failed_count = 0
    errors: list[dict] = []
    labels_collected: set[str] = set()

    # Initialize event publisher (optional - may not have Redis)
    event_publisher = None
    if ManualIngestEventPublisher is not None:
        try:
            event_publisher = ManualIngestEventPublisher()
        except Exception:
            pass  # Event publishing is optional

    async with get_session() as session:
        # Initialize repositories
        source_repo = SourceRepository(session)
        job_repo = IngestJobRepository(session)
        error_repo = IngestErrorRepository(session)

        # Create ingest job
        job = await job_repo.create_job(
            tenant_id=tenant_id,
            source_tag=source_tag,
            file_manifest=[str(f) for f in eml_files],
            content_type="email",
            options={
                "preserve_folders": preserve_folders,
                "skip_attachments": skip_attachments,
                "projects": project_tags,
            },
        )

        # Start job
        await job_repo.start_job(job.id)
        await session.commit()

        # Process each file with progress display
        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            console=console,
        ) as progress:
            task = progress.add_task("Processing emails...", total=len(eml_files))

            for file_path in eml_files:
                try:
                    # Parse the email
                    parsed = parse_eml_file(file_path)

                    # Check for duplicates by Message-ID first, then content hash
                    existing = await source_repo.find_by_external_id(
                        tenant_id=tenant_id,
                        source_system="manual_eml",
                        external_id=parsed.message_id,
                    )

                    if existing:
                        skipped_count += 1
                        await job_repo.update_progress(job.id, str(file_path), "skipped")
                        if verbose:
                            console.print(f"  [dim]Skipped (duplicate):[/dim] {file_path.name}")
                        progress.advance(task)
                        continue

                    # Also check by content hash
                    existing_hash = await source_repo.find_by_content_hash(
                        tenant_id=tenant_id,
                        content_hash=parsed.content_hash,
                    )

                    if existing_hash:
                        skipped_count += 1
                        await job_repo.update_progress(job.id, str(file_path), "skipped")
                        if verbose:
                            console.print(f"  [dim]Skipped (content hash match):[/dim] {file_path.name}")
                        progress.advance(task)
                        continue

                    # Generate labels from folder structure
                    labels = []
                    if preserve_folders:
                        # Create labels from relative path
                        rel_path = file_path.parent
                        if rel_path.parts:
                            labels = ["/".join(rel_path.parts)]

                    # Build ingestion metadata
                    participants = [parsed.from_email] + parsed.to_emails + parsed.cc_emails
                    metadata = {
                        "source_tag": source_tag,
                        "original_file_path": str(file_path),
                        "message_id": parsed.message_id,
                        "message_id_synthetic": parsed.message_id_synthetic,
                        "from_email": parsed.from_email,
                        "from_name": parsed.from_name,
                        "to_emails": parsed.to_emails,
                        "cc_emails": parsed.cc_emails,
                        "subject": parsed.subject,
                        "in_reply_to": parsed.in_reply_to,
                        "references": parsed.references,
                        "has_attachments": parsed.has_attachments,
                        "attachment_count": parsed.attachment_count,
                        "date_fallback_used": parsed.date_fallback_used,
                        "labels": labels,
                        "projects": project_tags,
                        "participants": participants,
                    }

                    # Determine email date
                    email_date = parsed.email_date or datetime.now(timezone.utc)

                    # Store in Source table
                    source = await source_repo.create_source(
                        tenant_id=tenant_id,
                        source_system="manual_eml",
                        external_id=parsed.message_id,
                        raw_content=parsed.body_plain or parsed.body_html or "",
                        content_type="message/rfc822",
                        ingestion_metadata=metadata,
                        source_timestamp=email_date,
                    )

                    imported_count += 1
                    await job_repo.update_progress(job.id, str(file_path), "imported")

                    # Collect labels for summary
                    labels_collected.update(labels)

                    if verbose:
                        console.print(f"  [green]Imported:[/green] {file_path.name}")

                    # Publish event to trigger AI pipeline
                    if event_publisher is not None:
                        try:
                            await event_publisher.publish_manual_email_ingested(
                                source_id=source.id,
                                tenant_id=str(tenant_id),
                                message_id=parsed.message_id,
                                job_id=str(job.id),
                                from_email=parsed.from_email,
                                to_emails=parsed.to_emails,
                                email_date=email_date,
                                content_hash=parsed.content_hash,
                                source_tag=source_tag,
                                original_file_path=str(file_path),
                                from_name=parsed.from_name,
                                cc_emails=parsed.cc_emails,
                                subject=parsed.subject,
                                date_is_fallback=parsed.date_fallback_used,
                                in_reply_to=parsed.in_reply_to,
                                has_attachments=parsed.has_attachments,
                                attachment_count=parsed.attachment_count,
                                labels=labels,
                            )
                        except Exception:
                            pass  # Event publishing is best-effort

                except ParseError as e:
                    failed_count += 1
                    error_msg = str(e)
                    errors.append({
                        "file": str(file_path),
                        "error_type": "parse_error",
                        "message": error_msg,
                    })
                    await error_repo.record_error(
                        job_id=job.id,
                        file_path=str(file_path),
                        error_type="parse_error",
                        error_message=error_msg,
                    )
                    await job_repo.update_progress(job.id, str(file_path), "failed")
                    if verbose:
                        console.print(f"  [red]Failed:[/red] {file_path.name} - {error_msg}")

                except Exception as e:
                    failed_count += 1
                    error_msg = str(e)
                    errors.append({
                        "file": str(file_path),
                        "error_type": "unexpected_error",
                        "message": error_msg,
                    })
                    await error_repo.record_error(
                        job_id=job.id,
                        file_path=str(file_path),
                        error_type="unexpected_error",
                        error_message=error_msg,
                    )
                    await job_repo.update_progress(job.id, str(file_path), "failed")
                    if verbose:
                        console.print(f"  [red]Error:[/red] {file_path.name} - {error_msg}")

                progress.advance(task)

        # Complete job
        final_status = "completed" if failed_count == 0 else "completed_with_errors"
        await job_repo.complete_job(job.id, final_status)
        await session.commit()

        # Get job completion time for event
        completed_at = datetime.now(timezone.utc)
        duration = (completed_at - job.started_at).total_seconds() if job.started_at else 0

        # Publish job completion event
        if event_publisher is not None:
            try:
                await event_publisher.publish_ingest_job_completed(
                    job_id=str(job.id),
                    tenant_id=str(tenant_id),
                    source_tag=source_tag,
                    total_files=len(eml_files),
                    imported_count=imported_count,
                    skipped_count=skipped_count,
                    failed_count=failed_count,
                    started_at=job.started_at or completed_at,
                    completed_at=completed_at,
                    duration_seconds=duration,
                    success=failed_count == 0,
                    final_status=final_status,
                    labels_created=list(labels_collected),
                )
            except Exception:
                pass  # Best-effort
            finally:
                await event_publisher.close()

    # Display summary
    console.print()
    _display_summary(
        job_id=str(job.id),
        source_tag=source_tag,
        total_files=len(eml_files),
        imported_count=imported_count,
        skipped_count=skipped_count,
        failed_count=failed_count,
        errors=errors,
    )


def _display_summary(
    job_id: str,
    source_tag: str,
    total_files: int,
    imported_count: int,
    skipped_count: int,
    failed_count: int,
    errors: list[dict],
) -> None:
    """Display ingest job summary.

    Args:
        job_id: Job identifier
        source_tag: Source tag used
        total_files: Total files processed
        imported_count: Successfully imported
        skipped_count: Skipped (duplicates)
        failed_count: Failed files
        errors: List of error details
    """
    # Build summary table
    table = Table(title="Ingest Summary", show_header=False)
    table.add_column("Field", style="cyan")
    table.add_column("Value")

    table.add_row("Job ID", job_id)
    table.add_row("Source Tag", source_tag)
    table.add_row("Total Files", str(total_files))
    table.add_row("Imported", f"[green]{imported_count}[/green]")
    table.add_row("Skipped (duplicates)", f"[yellow]{skipped_count}[/yellow]")
    table.add_row("Failed", f"[red]{failed_count}[/red]" if failed_count > 0 else "0")

    console.print(table)

    # Show errors if any
    if errors:
        console.print("\n[red]Errors:[/red]")
        for err in errors[:5]:  # Show first 5 errors
            console.print(f"  • {err['file']}: {err['message']}")
        if len(errors) > 5:
            console.print(f"  ... and {len(errors) - 5} more errors")
        console.print(f"\n[dim]Use 'penf ingest errors {job_id}' to see all errors[/dim]")


@ingest_group.command(name="jobs")
@click.option(
    "--status",
    type=click.Choice(["pending", "in_progress", "completed", "failed"]),
    default=None,
    help="Filter by status",
)
@click.option(
    "--limit",
    type=int,
    default=10,
    help="Maximum jobs to show",
)
def list_jobs(status: Optional[str], limit: int):
    """List ingest jobs.

    Shows recent ingest jobs with their status and progress.

    Examples:

        penf ingest jobs
        penf ingest jobs --status in_progress
        penf ingest jobs --limit 20
    """
    if get_session is None:
        console.print("[red]Error: Database connection not configured.[/red]")
        raise SystemExit(1)

    asyncio.run(_list_jobs_async(status, limit))


async def _list_jobs_async(status: Optional[str], limit: int) -> None:
    """List ingest jobs asynchronously."""
    tenant_id = uuid.UUID("00000000-0000-0000-0000-000000000001")  # Default tenant

    async with get_session() as session:
        job_repo = IngestJobRepository(session)
        jobs = await job_repo.list_jobs(tenant_id, status=status, limit=limit)

        if not jobs:
            console.print("[yellow]No ingest jobs found[/yellow]")
            return

        table = Table(title="Ingest Jobs")
        table.add_column("ID", style="cyan", no_wrap=True)
        table.add_column("Source Tag")
        table.add_column("Status")
        table.add_column("Files")
        table.add_column("Progress")
        table.add_column("Created")

        for job in jobs:
            # Format status with color
            status_str = job.status
            if job.status == "completed":
                status_str = "[green]completed[/green]"
            elif job.status == "in_progress":
                status_str = "[blue]in_progress[/blue]"
            elif job.status == "failed":
                status_str = "[red]failed[/red]"

            # Calculate progress
            progress_pct = (
                f"{job.processed_count}/{job.total_files} "
                f"({job.processed_count * 100 // job.total_files}%)"
                if job.total_files > 0
                else "0/0"
            )

            # Format ID (short form)
            short_id = str(job.id)[:8]

            table.add_row(
                short_id,
                job.source_tag,
                status_str,
                str(job.total_files),
                progress_pct,
                job.created_at.strftime("%Y-%m-%d %H:%M") if job.created_at else "-",
            )

        console.print(table)


@ingest_group.command(name="resume")
@click.argument("job_id")
@click.option(
    "--verbose",
    "-v",
    is_flag=True,
    default=False,
    help="Show detailed progress",
)
def resume_job(job_id: str, verbose: bool):
    """Resume an interrupted ingest job.

    JOB_ID is the job identifier from a previous run.

    Examples:

        penf ingest resume abc123-def456
    """
    if get_session is None:
        console.print("[red]Error: Database connection not configured.[/red]")
        raise SystemExit(1)

    if parse_eml_file is None:
        console.print("[red]Error: Ingest module not available.[/red]")
        raise SystemExit(1)

    # Parse job ID
    try:
        job_uuid = uuid.UUID(job_id)
    except ValueError:
        # Try to find job by partial ID
        console.print(f"[red]Error: Invalid job ID format: {job_id}[/red]")
        raise SystemExit(1)

    asyncio.run(_resume_job_async(job_uuid, verbose))


async def _resume_job_async(job_id: uuid.UUID, verbose: bool) -> None:
    """Resume a job asynchronously."""
    tenant_id = uuid.UUID("00000000-0000-0000-0000-000000000001")  # Default tenant

    async with get_session() as session:
        job_repo = IngestJobRepository(session)
        error_repo = IngestErrorRepository(session)
        source_repo = SourceRepository(session)

        # Get the job
        job = await job_repo.get_job(job_id, tenant_id)
        if not job:
            console.print(f"[red]Error: Job {job_id} not found[/red]")
            return

        # Check job status
        if job.status == "completed":
            console.print(f"[yellow]Job {job_id} is already completed[/yellow]")
            return

        # Get unprocessed files
        unprocessed = await job_repo.get_unprocessed_files(job_id)
        if not unprocessed:
            console.print(f"[yellow]No unprocessed files remaining for job {job_id}[/yellow]")
            return

        console.print(
            Panel(
                f"Resuming job [cyan]{str(job_id)[:8]}[/cyan]\n"
                f"Source tag: [cyan]{job.source_tag}[/cyan]\n"
                f"Remaining files: [bold]{len(unprocessed)}[/bold]",
                title="Resume Ingest",
            )
        )

        # Get job options
        options = job.options or {}
        preserve_folders = options.get("preserve_folders", True)
        skip_attachments = options.get("skip_attachments", False)
        project_tags = options.get("projects", [])

        # Track results
        imported_count = 0
        skipped_count = 0
        failed_count = 0
        errors: list[dict] = []

        # Update job status
        await job_repo.start_job(job_id)
        await session.commit()

        # Process remaining files
        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            console=console,
        ) as progress:
            task = progress.add_task("Processing remaining emails...", total=len(unprocessed))

            for file_path_str in unprocessed:
                file_path = Path(file_path_str)

                if not file_path.exists():
                    failed_count += 1
                    await error_repo.record_error(
                        job_id=job_id,
                        file_path=file_path_str,
                        error_type="io_error",
                        error_message=f"File not found: {file_path_str}",
                    )
                    await job_repo.update_progress(job_id, file_path_str, "failed")
                    progress.advance(task)
                    continue

                try:
                    # Parse the email
                    parsed = parse_eml_file(file_path)

                    # Check for duplicates
                    existing = await source_repo.find_by_external_id(
                        tenant_id=tenant_id,
                        source_system="manual_eml",
                        external_id=parsed.message_id,
                    )

                    if existing:
                        skipped_count += 1
                        await job_repo.update_progress(job_id, file_path_str, "skipped")
                        progress.advance(task)
                        continue

                    existing_hash = await source_repo.find_by_content_hash(
                        tenant_id=tenant_id,
                        content_hash=parsed.content_hash,
                    )

                    if existing_hash:
                        skipped_count += 1
                        await job_repo.update_progress(job_id, file_path_str, "skipped")
                        progress.advance(task)
                        continue

                    # Generate labels
                    labels = []
                    if preserve_folders:
                        rel_path = file_path.parent
                        if rel_path.parts:
                            labels = ["/".join(rel_path.parts)]

                    # Build metadata
                    participants = [parsed.from_email] + parsed.to_emails + parsed.cc_emails
                    metadata = {
                        "source_tag": job.source_tag,
                        "original_file_path": file_path_str,
                        "message_id": parsed.message_id,
                        "message_id_synthetic": parsed.message_id_synthetic,
                        "from_email": parsed.from_email,
                        "from_name": parsed.from_name,
                        "to_emails": parsed.to_emails,
                        "cc_emails": parsed.cc_emails,
                        "subject": parsed.subject,
                        "in_reply_to": parsed.in_reply_to,
                        "references": parsed.references,
                        "has_attachments": parsed.has_attachments,
                        "attachment_count": parsed.attachment_count,
                        "date_fallback_used": parsed.date_fallback_used,
                        "labels": labels,
                        "projects": project_tags,
                        "participants": participants,
                    }

                    email_date = parsed.email_date or datetime.now(timezone.utc)

                    await source_repo.create_source(
                        tenant_id=tenant_id,
                        source_system="manual_eml",
                        external_id=parsed.message_id,
                        raw_content=parsed.body_plain or parsed.body_html or "",
                        content_type="message/rfc822",
                        ingestion_metadata=metadata,
                        source_timestamp=email_date,
                    )

                    imported_count += 1
                    await job_repo.update_progress(job_id, file_path_str, "imported")

                    if verbose:
                        console.print(f"  [green]Imported:[/green] {file_path.name}")

                except ParseError as e:
                    failed_count += 1
                    await error_repo.record_error(
                        job_id=job_id,
                        file_path=file_path_str,
                        error_type="parse_error",
                        error_message=str(e),
                    )
                    await job_repo.update_progress(job_id, file_path_str, "failed")
                    errors.append({
                        "file": file_path_str,
                        "error_type": "parse_error",
                        "message": str(e),
                    })

                except Exception as e:
                    failed_count += 1
                    await error_repo.record_error(
                        job_id=job_id,
                        file_path=file_path_str,
                        error_type="unexpected_error",
                        error_message=str(e),
                    )
                    await job_repo.update_progress(job_id, file_path_str, "failed")
                    errors.append({
                        "file": file_path_str,
                        "error_type": "unexpected_error",
                        "message": str(e),
                    })

                progress.advance(task)

        # Complete job
        final_status = "completed" if failed_count == 0 else "completed_with_errors"
        await job_repo.complete_job(job_id, final_status)
        await session.commit()

    # Display summary
    console.print()
    _display_summary(
        job_id=str(job_id),
        source_tag=job.source_tag,
        total_files=len(unprocessed),
        imported_count=imported_count,
        skipped_count=skipped_count,
        failed_count=failed_count,
        errors=errors,
    )


# Export for registration in main.py
__all__ = ["ingest_group"]
