"""CLI module for Penfold."""

# Lazy import to avoid heavy dependencies in unit tests
# Import main only when needed: from penf_lib.cli.main import main

__all__ = ["main"]


def main():
    """Entry point for CLI - imports actual main on demand."""
    from .main import main as _main
    return _main()