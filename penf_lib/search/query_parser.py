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
    STOP_WORDS: set[str] = {
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
    ABBREVIATIONS: dict[str, str] = {
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

    def suggest_corrections(self, query: str) -> list[str]:
        """Suggest corrections for a potentially misspelled query.

        Uses edit distance-based fuzzy matching to suggest corrections
        for misspelled words in the query.

        Args:
            query: Query string to check for misspellings

        Returns:
            List of suggestion strings (e.g., "Did you mean: corrected query")
        """
        # Use SpellChecker for fuzzy matching suggestions
        # SpellChecker is defined later in this module
        checker = SpellChecker()
        corrected = checker.suggest_query_correction(query)

        suggestions: list[str] = []
        if corrected:
            suggestions.append(f"Did you mean: {corrected}")

        return suggestions


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


def _edit_distance(s1: str, s2: str) -> int:
    """Calculate Levenshtein edit distance between two strings.

    Uses dynamic programming for O(m*n) time complexity.

    Args:
        s1: First string
        s2: Second string

    Returns:
        Edit distance (minimum number of insertions, deletions, substitutions)
    """
    m, n = len(s1), len(s2)

    # Handle empty string cases
    if m == 0:
        return n
    if n == 0:
        return m

    # Create distance matrix
    dp = [[0] * (n + 1) for _ in range(m + 1)]

    # Initialize base cases
    for i in range(m + 1):
        dp[i][0] = i
    for j in range(n + 1):
        dp[0][j] = j

    # Fill in the matrix
    for i in range(1, m + 1):
        for j in range(1, n + 1):
            if s1[i - 1] == s2[j - 1]:
                dp[i][j] = dp[i - 1][j - 1]
            else:
                dp[i][j] = 1 + min(
                    dp[i - 1][j],      # deletion
                    dp[i][j - 1],      # insertion
                    dp[i - 1][j - 1],  # substitution
                )

    return dp[m][n]


class SpellChecker:
    """Simple spell-check suggestion system using edit distance.

    Uses a dictionary of common search terms and abbreviations to
    suggest corrections for misspelled query terms.

    Attributes:
        dictionary: Set of known correct words
        max_edit_distance: Maximum edit distance for suggestions
    """

    # Common search terms in the Penfold domain
    DEFAULT_DICTIONARY: set[str] = {
        # Communication types
        "email", "emails", "meeting", "meetings", "message", "messages",
        "document", "documents", "slack", "thread", "threads",
        # Actions
        "discussion", "update", "updates", "decision", "decisions",
        "report", "reports", "summary", "notes", "status",
        # Business terms
        "project", "projects", "deployment", "release", "production",
        "development", "environment", "customer", "customers", "client",
        "team", "manager", "department", "deadline", "schedule",
        # Technical terms
        "api", "database", "server", "configuration", "integration",
        "testing", "review", "approval", "request", "issue", "issues",
        "bug", "feature", "implementation", "specification",
        # Time-related
        "today", "yesterday", "week", "month", "recent", "since",
        "before", "after", "between", "around",
    }

    def __init__(
        self,
        dictionary: Optional[set[str]] = None,
        max_edit_distance: int = 2,
    ):
        """Initialize spell checker.

        Args:
            dictionary: Custom dictionary of known words (uses default if None)
            max_edit_distance: Maximum edit distance for suggestions (default 2)
        """
        self.dictionary = dictionary or self.DEFAULT_DICTIONARY
        self.max_edit_distance = max_edit_distance

    def suggest_correction(self, word: str) -> Optional[str]:
        """Suggest a correction for a potentially misspelled word.

        Args:
            word: Word to check

        Returns:
            Suggested correction or None if word is correct or no close match
        """
        word_lower = word.lower()

        # If word is in dictionary, no correction needed
        if word_lower in self.dictionary:
            return None

        # Skip very short words
        if len(word_lower) < 3:
            return None

        # Find closest match
        best_match = None
        best_distance = self.max_edit_distance + 1

        for dict_word in self.dictionary:
            # Skip if length difference is too large
            if abs(len(dict_word) - len(word_lower)) > self.max_edit_distance:
                continue

            distance = _edit_distance(word_lower, dict_word)
            if distance < best_distance:
                best_distance = distance
                best_match = dict_word

        if best_distance <= self.max_edit_distance:
            return best_match

        return None

    def suggest_corrections(self, query: str) -> list[tuple[str, str]]:
        """Suggest corrections for all misspelled words in a query.

        Args:
            query: Query string to check

        Returns:
            List of (original_word, suggested_word) tuples
        """
        corrections: list[tuple[str, str]] = []

        # Split query into words, preserving original case for reporting
        words = query.split()

        for word in words:
            # Remove common punctuation for checking
            clean_word = word.strip(".,!?;:\"'")
            if not clean_word:
                continue

            suggestion = self.suggest_correction(clean_word)
            if suggestion:
                corrections.append((clean_word, suggestion))

        return corrections

    def suggest_query_correction(self, query: str) -> Optional[str]:
        """Suggest a corrected version of the entire query.

        Args:
            query: Query string to check

        Returns:
            Corrected query string or None if no corrections needed
        """
        corrections = self.suggest_corrections(query)

        if not corrections:
            return None

        corrected = query
        for original, suggestion in corrections:
            # Case-insensitive replacement (re is already imported at module level)
            corrected = re.sub(
                rf"\b{re.escape(original)}\b",
                suggestion,
                corrected,
                flags=re.IGNORECASE,
            )

        # Only return if we actually made changes
        if corrected.lower() != query.lower():
            return corrected

        return None


class QueryCorrector:
    """Query correction system combining spell-check with query analysis.

    Provides comprehensive query correction and suggestion capabilities
    including spelling fixes and query refinement suggestions.
    """

    def __init__(
        self,
        spell_checker: Optional[SpellChecker] = None,
    ):
        """Initialize query corrector.

        Args:
            spell_checker: Spell checker instance (creates default if None)
        """
        self.spell_checker = spell_checker or SpellChecker()

    def suggest_corrections(self, query: str) -> list[str]:
        """Generate correction suggestions for a query.

        Args:
            query: Original query string

        Returns:
            List of suggestion strings (may include corrected query or tips)
        """
        suggestions: list[str] = []

        # Check for spelling corrections
        corrected = self.spell_checker.suggest_query_correction(query)
        if corrected:
            suggestions.append(f"Did you mean: {corrected}")

        # Check for very short queries
        words = [w for w in query.split() if len(w) > 2]
        if len(words) < 2:
            suggestions.append("Try adding more specific terms to narrow your search")

        return suggestions

    def get_no_results_suggestions(self, query: str) -> list[str]:
        """Generate suggestions when a search returns no results.

        Args:
            query: Original query that returned no results

        Returns:
            List of suggestions to help refine the search
        """
        suggestions: list[str] = []

        # Check for spelling errors first
        corrected = self.spell_checker.suggest_query_correction(query)
        if corrected:
            suggestions.append(f"Did you mean: {corrected}")

        # General tips for no results
        suggestions.append("Try using fewer or broader search terms")

        # If query has filters, suggest removing them
        if any(f in query.lower() for f in ["from:", "type:", "project:"]):
            suggestions.append("Try removing some filters to broaden your search")

        # If query is long, suggest shortening
        if len(query.split()) > 5:
            suggestions.append("Try using just the key terms from your query")

        # Check for specific content type terms and suggest alternatives
        if "email" in query.lower():
            suggestions.append("Try searching for 'message' instead of 'email'")
        if "document" in query.lower():
            suggestions.append("Try searching for 'doc' or 'file' instead")

        return suggestions

    def get_broad_query_suggestions(self, query: str, result_count: int) -> list[str]:
        """Generate suggestions when a search returns many results.

        Args:
            query: Original query that returned many results
            result_count: Number of results returned

        Returns:
            List of suggestions to narrow the search
        """
        suggestions: list[str] = []

        suggestions.append(
            f"Your search returned {result_count} results. "
            "Consider narrowing your search with filters."
        )

        # Suggest adding filters
        if "from:" not in query.lower():
            suggestions.append("Add 'from:email@example.com' to filter by sender")

        if "type:" not in query.lower():
            suggestions.append("Add 'type:email' or 'type:meeting' to filter by content type")

        if not any(t in query.lower() for t in ["last week", "this month", "since", "before"]):
            suggestions.append("Add a date filter like 'since December' or 'last week'")

        return suggestions
