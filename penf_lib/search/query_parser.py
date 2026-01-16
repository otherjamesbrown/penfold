"""Query parsing and preprocessing for search operations.

This module handles:
- Natural language query normalization
- Query preprocessing (lowercase, stop words, abbreviations)
- Query embedding generation

Based on research.md specification from spec 007-search-interface.
"""

from __future__ import annotations

import re


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

    def parse(self, raw_query: str) -> tuple[str, dict]:
        """Parse raw query into normalized text and extracted filters.

        Args:
            raw_query: User's raw query input

        Returns:
            Tuple of (normalized query text, filters dict)
        """
        # First extract any structured filters
        query_text, filters = self.extract_filters(raw_query)

        # Then normalize the remaining query text
        normalized = self.normalize(query_text)

        return normalized, filters


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
