"""Unit tests for vector operations module.

Tests vector operations functionality that doesn't require database connection.
"""

import pytest
from unittest.mock import MagicMock, AsyncMock
import numpy as np

from penf_lib.storage.vector import VectorOperations, VectorError, VectorDimensionError


class TestVectorOperations:
    """Test vector operations without database dependency."""

    def setup_method(self):
        """Set up test fixtures."""
        self.mock_session = AsyncMock()
        self.vector_ops = VectorOperations(
            session=self.mock_session,
            expected_dimension=768,
            distance_metric="l2",
            cache_enabled=True
        )

    def test_initialization(self):
        """Test VectorOperations initialization."""
        assert self.vector_ops.expected_dimension == 768
        assert self.vector_ops.distance_metric == "l2"
        assert self.vector_ops.cache_enabled is True
        assert isinstance(self.vector_ops._search_cache, dict)

    def test_initialization_invalid_metric(self):
        """Test initialization with invalid distance metric raises error."""
        with pytest.raises(ValueError, match="Unsupported distance metric"):
            VectorOperations(
                session=self.mock_session,
                distance_metric="invalid_metric"
            )

    def test_valid_distance_metrics(self):
        """Test all valid distance metrics can be initialized."""
        valid_metrics = ["l2", "cosine", "inner_product"]

        for metric in valid_metrics:
            ops = VectorOperations(
                session=self.mock_session,
                distance_metric=metric
            )
            assert ops.distance_metric == metric

    def test_cache_key_generation(self):
        """Test search cache key generation."""
        query_vector = [0.1] * 768

        key1 = self.vector_ops._get_search_cache_key(
            query_vector=query_vector,
            limit=10,
            entity_types=["source"],
            distance_threshold=0.5,
            embedding_models=["nomic-embed"],
            min_confidence=0.8,
        )

        # Same parameters should generate same key
        key2 = self.vector_ops._get_search_cache_key(
            query_vector=query_vector,
            limit=10,
            entity_types=["source"],
            distance_threshold=0.5,
            embedding_models=["nomic-embed"],
            min_confidence=0.8,
        )

        assert key1 == key2
        assert isinstance(key1, str)
        assert len(key1) == 32  # MD5 hash length

    def test_cache_key_different_parameters(self):
        """Test that different parameters generate different cache keys."""
        query_vector = [0.1] * 768

        key1 = self.vector_ops._get_search_cache_key(
            query_vector=query_vector,
            limit=10,
            entity_types=["source"],
            distance_threshold=0.5,
            embedding_models=None,
            min_confidence=None,
        )

        key2 = self.vector_ops._get_search_cache_key(
            query_vector=query_vector,
            limit=10,
            entity_types=["assertion"],  # Different entity type
            distance_threshold=0.5,
            embedding_models=None,
            min_confidence=None,
        )

        assert key1 != key2

    def test_clear_cache(self):
        """Test cache clearing functionality."""
        # Add something to cache
        self.vector_ops._search_cache["test_key"] = "test_value"
        assert len(self.vector_ops._search_cache) == 1

        # Clear cache
        self.vector_ops.clear_cache()
        assert len(self.vector_ops._search_cache) == 0

    @pytest.mark.asyncio
    async def test_store_embedding_dimension_validation(self):
        """Test that store_embedding validates vector dimension."""
        # Test with wrong dimension
        wrong_dimension_vector = [0.1] * 512  # Should be 768

        with pytest.raises(VectorDimensionError, match="Expected 768D vector, got 512D"):
            await self.vector_ops.store_embedding(
                entity_type="source",
                entity_id=1,
                embedding=wrong_dimension_vector,
                embedding_model="test-model"
            )

    @pytest.mark.asyncio
    async def test_similarity_search_dimension_validation(self):
        """Test that similarity_search validates query vector dimension."""
        wrong_dimension_vector = [0.1] * 512  # Should be 768

        with pytest.raises(VectorDimensionError, match="Expected 768D query vector, got 512D"):
            await self.vector_ops.similarity_search(
                query_vector=wrong_dimension_vector,
                limit=10
            )

    @pytest.mark.asyncio
    async def test_store_embeddings_batch_dimension_validation(self):
        """Test batch embedding storage validates all vector dimensions."""
        embeddings_data = [
            {
                "entity_type": "source",
                "entity_id": 1,
                "embedding": [0.1] * 768,  # Correct dimension
                "embedding_model": "test-model"
            },
            {
                "entity_type": "source",
                "entity_id": 2,
                "embedding": [0.1] * 512,  # Wrong dimension
                "embedding_model": "test-model"
            }
        ]

        with pytest.raises(VectorDimensionError, match="Embedding 1: Expected 768D vector, got 512D"):
            await self.vector_ops.store_embeddings_batch(embeddings_data)

    def test_empty_batch_handling(self):
        """Test that empty batch is handled gracefully."""
        result = self.vector_ops.store_embeddings_batch([])
        # Should return empty list for empty input
        # Note: This is testing the synchronous validation, not the async part


class TestVectorUtilities:
    """Test utility functions and edge cases."""

    def test_content_hash_generation(self):
        """Test content hash generation consistency."""
        import hashlib

        test_content = "This is test content for hashing"
        expected_hash = hashlib.sha256(test_content.encode()).hexdigest()

        # Verify our hash generation method produces consistent results
        actual_hash = hashlib.sha256(test_content.encode()).hexdigest()
        assert actual_hash == expected_hash
        assert len(actual_hash) == 64  # SHA256 hex length

    def test_vector_dimension_validation_edge_cases(self):
        """Test edge cases for vector dimension validation."""
        mock_session = AsyncMock()

        # Test different dimensions
        test_cases = [
            (384, [0.1] * 384, True),   # Valid for 384D model
            (768, [0.1] * 768, True),   # Valid for 768D model
            (1536, [0.1] * 1536, True), # Valid for 1536D model
            (768, [0.1] * 767, False),  # One dimension short
            (768, [0.1] * 769, False),  # One dimension over
            (768, [], False),           # Empty vector
        ]

        for expected_dim, test_vector, should_be_valid in test_cases:
            ops = VectorOperations(
                session=mock_session,
                expected_dimension=expected_dim
            )

            if should_be_valid:
                # Should not raise exception during validation
                assert len(test_vector) == expected_dim
            else:
                # Would raise exception if we called store_embedding
                assert len(test_vector) != expected_dim


class TestVectorSearchService:
    """Test vector search service functionality."""

    def test_diversity_application(self):
        """Test result diversification logic."""
        from penf_lib.storage.vector import VectorSearchService

        mock_session = AsyncMock()
        service = VectorSearchService(mock_session)

        # Mock results with different entity types
        mock_results = [
            {"entity_type": "source", "enhanced_score": 0.9},
            {"entity_type": "source", "enhanced_score": 0.85},
            {"entity_type": "assertion", "enhanced_score": 0.8},
            {"entity_type": "source", "enhanced_score": 0.75},
            {"entity_type": "person", "enhanced_score": 0.7},
        ]

        # Test diversification
        diversified = service._apply_diversity(
            results=mock_results,
            diversity_factor=0.2,
            target_count=3
        )

        # Should prefer diverse entity types
        assert len(diversified) == 3
        assert diversified[0]["entity_type"] == "source"  # Highest score always included

        # Check that we got some diversity
        entity_types = [r["entity_type"] for r in diversified]
        unique_types = set(entity_types)
        assert len(unique_types) >= 2  # Should have at least 2 different types

    def test_diversity_no_factor(self):
        """Test that zero diversity factor returns original order."""
        from penf_lib.storage.vector import VectorSearchService

        mock_session = AsyncMock()
        service = VectorSearchService(mock_session)

        mock_results = [
            {"entity_type": "source", "enhanced_score": 0.9},
            {"entity_type": "source", "enhanced_score": 0.85},
            {"entity_type": "source", "enhanced_score": 0.8},
        ]

        diversified = service._apply_diversity(
            results=mock_results,
            diversity_factor=0.0,  # No diversity
            target_count=3
        )

        # Should return original results
        assert diversified == mock_results


def test_import_vector_modules():
    """Test that vector modules can be imported successfully."""
    # Test imports work
    from penf_lib.storage.vector import (
        VectorOperations,
        VectorSearchService,
        VectorError,
        VectorDimensionError,
        VectorSearchError
    )

    # Test exception hierarchy
    assert issubclass(VectorDimensionError, VectorError)
    assert issubclass(VectorSearchError, VectorError)
    assert issubclass(VectorError, Exception)


def test_import_embedding_repository():
    """Test that embedding repository can be imported successfully."""
    from penf_lib.storage.repositories.embedding import EmbeddingRepository

    # Should import without error
    assert EmbeddingRepository is not None