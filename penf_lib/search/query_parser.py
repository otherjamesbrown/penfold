"""Query parsing and preprocessing for search operations.

This module handles:
- Natural language query normalization
- Query preprocessing (lowercase, stop words, abbreviations)
- Temporal expression parsing
- Query embedding generation

Based on research.md specification from spec 007-search-interface.
"""

from __future__ import annotations

import re
from datetime import datetime, timedelta, timezone
from typing import Optional

import dateparser

from penf_lib.search.models import TemporalConstraint


def _this_week() -> tuple[datetime, datetime]:
    """Get start and end of current week (Monday-Sunday)."""
    now = datetime.now(timezone.utc)
    start = now - timedelta(days=now.weekday())
    start = start.replace(hour=0, minute=0, second=0, microsecond=0)
    return start, now


def _this_month() -> tuple[datetime, datetime]:
    """Get start of current month to now."""
    now = datetime.now(timezone.utc)
    start = now.replace(day=1, hour=0, minute=0, second=0, microsecond=0)
    return start, now


class TemporalQueryParser:
    """Parse temporal expressions from natural language queries.

    Uses dateparser library for robust NLP date parsing with custom
    patterns for Penfold-specific temporal expressions.

    Supported expression types:
    - Absolute dates: "December 15, 2025", "2025-12-15"
    - Relative expressions: "last week", "yesterday", "3 days ago"
    - Named periods: "since Christmas", "before the new year"
    - Fuzzy periods: "around the deployment" (expanded +/-3 days)
    - Range expressions: "between January and March"
    """

    # Relative patterns with handlers
    RELATIVE_PATTERNS: dict[str, callable] = {
        r"\blast\s+week\b": lambda: (
            datetime.now(timezone.utc) - timedelta(weeks=1),
            datetime.now(timezone.utc),
        ),
        r"\blast\s+month\b": lambda: (
            datetime.now(timezone.utc) - timedelta(days=30),
            datetime.now(timezone.utc),
        ),
        r"\byesterday\b": lambda: (
            datetime.now(timezone.utc).replace(hour=0, minute=0, second=0, microsecond=0)
            - timedelta(days=1),
            datetime.now(timezone.utc).replace(hour=23, minute=59, second=59, microsecond=0)
            - timedelta(days=1),
        ),
        r"\btoday\b": lambda: (
            datetime.now(timezone.utc).replace(hour=0, minute=0, second=0, microsecond=0),
            datetime.now(timezone.utc),
        ),
        r"\bthis\s+week\b": _this_week,
        r"\bthis\s+month\b": _this_month,
    }

    # Range patterns - capture date expressions between keywords
    RANGE_PATTERNS = [
        r"\bbetween\s+(.+?)\s+and\s+(.+?)$",
        r"\bfrom\s+(.+?)\s+to\s+(.+?)$",
    ]

    # Since/before patterns - capture everything after the keyword to end of string
    SINCE_PATTERN = r"\bsince\s+(.+?)$"
    BEFORE_PATTERN = r"\bbefore\s+(.+?)$"
    AROUND_PATTERN = r"\baround\s+(.+?)$"

    def __init__(self, fuzzy_days: int = 3):
        """Initialize temporal parser.

        Args:
            fuzzy_days: Days to expand for "around" queries
        """
        self.fuzzy_days = fuzzy_days
        self.dateparser_settings: dict = {
            "PREFER_DATES_FROM": "past",
            "RELATIVE_BASE": datetime.now(timezone.utc),
            "RETURN_AS_TIMEZONE_AWARE": True,
        }

    def _normalize_whitespace(self, text: str) -> str:
        """Normalize whitespace in text."""
        return " ".join(text.split())

    def extract_temporal(
        self, query: str
    ) -> tuple[str, Optional[TemporalConstraint]]:
        """Extract temporal constraints from query text.

        Args:
            query: Query string potentially containing temporal expressions

        Returns:
            Tuple of (remaining query text, TemporalConstraint or None)
        """
        remaining = query

        # Try relative patterns first
        for pattern, handler in self.RELATIVE_PATTERNS.items():
            if re.search(pattern, query, re.IGNORECASE):
                start, end = handler()
                remaining = re.sub(pattern, "", remaining, flags=re.IGNORECASE)
                return self._normalize_whitespace(remaining), TemporalConstraint(
                    start_date=start, end_date=end
                )

        # Try range patterns
        for pattern in self.RANGE_PATTERNS:
            match = re.search(pattern, query, re.IGNORECASE)
            if match:
                start = dateparser.parse(
                    match.group(1), settings=self.dateparser_settings
                )
                end = dateparser.parse(
                    match.group(2), settings=self.dateparser_settings
                )
                if start and end:
                    remaining = re.sub(pattern, "", remaining, flags=re.IGNORECASE)
                    return self._normalize_whitespace(remaining), TemporalConstraint(
                        start_date=start, end_date=end
                    )

        # Try since pattern
        match = re.search(self.SINCE_PATTERN, query, re.IGNORECASE)
        if match:
            start = dateparser.parse(
                match.group(1), settings=self.dateparser_settings
            )
            if start:
                remaining = re.sub(
                    self.SINCE_PATTERN, "", remaining, flags=re.IGNORECASE
                )
                return self._normalize_whitespace(remaining), TemporalConstraint(
                    start_date=start,
                    relative_expression=f"since {match.group(1)}",
                )

        # Try before pattern
        match = re.search(self.BEFORE_PATTERN, query, re.IGNORECASE)
        if match:
            end = dateparser.parse(match.group(1), settings=self.dateparser_settings)
            if end:
                remaining = re.sub(
                    self.BEFORE_PATTERN, "", remaining, flags=re.IGNORECASE
                )
                return self._normalize_whitespace(remaining), TemporalConstraint(
                    end_date=end,
                    relative_expression=f"before {match.group(1)}",
                )

        # Try around pattern (fuzzy)
        match = re.search(self.AROUND_PATTERN, query, re.IGNORECASE)
        if match:
            center = dateparser.parse(
                match.group(1), settings=self.dateparser_settings
            )
            if center:
                start = center - timedelta(days=self.fuzzy_days)
                end = center + timedelta(days=self.fuzzy_days)
                remaining = re.sub(
                    self.AROUND_PATTERN, "", remaining, flags=re.IGNORECASE
                )
                return self._normalize_whitespace(remaining), TemporalConstraint(
                    start_date=start,
                    end_date=end,
                    relative_expression=f"around {match.group(1)}",
                )

        # No temporal expression found
        return query, None

    def parse_date(self, text: str) -> Optional[datetime]:
        """Parse a date string to datetime.

        Args:
            text: Date string to parse

        Returns:
            Parsed datetime or None
        """
        return dateparser.parse(text, settings=self.dateparser_settings)


class QueryParser:
    """Parse and preprocess natural language search queries.

    Normalizes queries for consistent search behavior and extracts
    structured information from natural language input.
    """

    # Common English stop words to optionally filter
    STOP_WORDS = {
        "a",
        "an",
        "the",
        "and",
        "or",
        "but",
        "in",
        "on",
        "at",
        "to",
        "for",
        "of",
        "with",
        "by",
        "from",
        "as",
        "is",
        "was",
        "are",
        "were",
        "been",
        "be",
        "have",
        "has",
        "had",
        "do",
        "does",
        "did",
        "will",
        "would",
        "could",
        "should",
        "may",
        "might",
        "must",
        "shall",
        "can",
        "need",
        "dare",
        "ought",
        "used",
        "it",
        "its",
        "this",
        "that",
        "these",
        "those",
        "i",
        "you",
        "he",
        "she",
        "we",
        "they",
        "what",
        "which",
        "who",
        "whom",
    }

    # Common abbreviations to expand
    ABBREVIATIONS = {
        "mtg": "meeting",
        "msg": "message",
        "msgs": "messages",
        "dept": "department",
        "mgr": "manager",
        "proj": "project",
        "dev": "development",
        "prod": "production",
        "env": "environment",
        "req": "requirement",
        "spec": "specification",
        "doc": "document",
        "docs": "documents",
        "info": "information",
        "appt": "appointment",
        "conf": "conference",
        "sync": "synchronization",
        "async": "asynchronous",
    }

    def __init__(self, remove_stop_words: bool = False):
        """Initialize query parser.

        Args:
            remove_stop_words: Whether to remove stop words (default False).
                              Often better to keep them for phrase matching.
        """
        self.remove_stop_words = remove_stop_words

    def normalize(self, query: str) -> str:
        """Normalize query text for consistent processing.

        Steps:
        1. Lowercase
        2. Normalize whitespace
        3. Expand abbreviations
        4. Optionally remove stop words
        5. Preserve quoted phrases

        Args:
            query: Raw query string

        Returns:
            Normalized query string
        """
        # Extract quoted phrases to preserve them
        quoted_phrases: list[str] = []

        def save_quoted(match: re.Match[str]) -> str:
            quoted_phrases.append(match.group(1))
            return f"__QUOTED_{len(quoted_phrases)-1}__"

        normalized = re.sub(r'"([^"]+)"', save_quoted, query)

        # Lowercase
        normalized = normalized.lower()

        # Normalize whitespace
        normalized = " ".join(normalized.split())

        # Expand abbreviations (whole word only)
        words = normalized.split()
        expanded: list[str] = []
        for word in words:
            # Remove punctuation for lookup
            clean_word = re.sub(r"[^\w]", "", word)
            if clean_word in self.ABBREVIATIONS:
                expanded.append(self.ABBREVIATIONS[clean_word])
            elif self.remove_stop_words and clean_word in self.STOP_WORDS:
                continue
            else:
                expanded.append(word)

        normalized = " ".join(expanded)

        # Restore quoted phrases
        for i, phrase in enumerate(quoted_phrases):
            normalized = normalized.replace(f"__quoted_{i}__", f'"{phrase}"')

        return normalized

    def extract_filters(self, query: str) -> tuple[str, dict]:
        """Extract structured filters from query text.

        Recognizes patterns like:
        - "from:alice@example.com" -> participant filter
        - "type:email" -> content type filter
        - "project:Atlas" -> project filter

        Args:
            query: Query string potentially containing filters

        Returns:
            Tuple of (remaining query, extracted filters dict)
        """
        filters: dict[str, list[str]] = {
            "participants": [],
            "content_types": [],
            "projects": [],
        }

        remaining = query

        # Extract from: patterns
        from_pattern = r"\bfrom:(\S+)"
        for match in re.finditer(from_pattern, query, re.IGNORECASE):
            filters["participants"].append(match.group(1))
            remaining = remaining.replace(match.group(0), "")

        # Extract type: patterns
        type_pattern = r"\btype:(\S+)"
        for match in re.finditer(type_pattern, query, re.IGNORECASE):
            filters["content_types"].append(match.group(1))
            remaining = remaining.replace(match.group(0), "")

        # Extract project: patterns
        project_pattern = r"\bproject:(\S+)"
        for match in re.finditer(project_pattern, query, re.IGNORECASE):
            filters["projects"].append(match.group(1))
            remaining = remaining.replace(match.group(0), "")

        # Clean up remaining query
        remaining = " ".join(remaining.split())

        # Remove empty filter lists
        filters = {k: v for k, v in filters.items() if v}

        return remaining, filters

    def parse(
        self, raw_query: str
    ) -> tuple[str, dict, Optional[TemporalConstraint]]:
        """Parse raw query into normalized text, filters, and temporal constraint.

        Args:
            raw_query: User's raw query input

        Returns:
            Tuple of (normalized query text, filters dict, TemporalConstraint or None)
        """
        # First extract temporal constraints
        temporal_parser = TemporalQueryParser()
        query_text, temporal = temporal_parser.extract_temporal(raw_query)

        # Then extract any structured filters
        query_text, filters = self.extract_filters(query_text)

        # Finally normalize the remaining query text
        normalized = self.normalize(query_text)

        return normalized, filters, temporal


class QueryEmbedder:
    """Generate embeddings for search queries.

    Uses the same embedding model as indexed content to ensure
    compatible vector representations for similarity search.
    """

    def __init__(self, model_name: str = "nomic-embed-text"):
        """Initialize query embedder.

        Args:
            model_name: Name of the embedding model (must match indexed content)
        """
        self.model_name = model_name

    async def embed(self, query: str) -> list[float]:
        """Generate embedding vector for query.

        Args:
            query: Normalized query text

        Returns:
            768-dimensional embedding vector
        """
        # TODO: Integrate with Ollama embedding API
        # For now, return placeholder to allow testing
        # Real implementation will use:
        # response = await ollama.embed(model=self.model_name, input=query)
        # return response.embeddings[0]

        # Placeholder: return zeros (will be replaced in integration)
        return [0.0] * 768

    async def embed_batch(self, queries: list[str]) -> list[list[float]]:
        """Generate embeddings for multiple queries.

        Args:
            queries: List of normalized query texts

        Returns:
            List of 768-dimensional embedding vectors
        """
        # TODO: Batch embedding for efficiency
        return [await self.embed(q) for q in queries]
