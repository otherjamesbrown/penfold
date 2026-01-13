"""Vector storage and similarity search operations.

This module provides high-performance vector operations for semantic search
using pgvector with HNSW indexing for 768-dimensional embeddings.
"""

import logging
from typing import Dict, Any, List, Optional, Tuple, Union
from datetime import datetime, timezone
import asyncio
from concurrent.futures import ThreadPoolExecutor

import numpy as np
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, text, func, and_, or_, desc
from sqlalchemy.exc import SQLAlchemyError

from penf_lib.storage.models import Embedding
from penf_lib.storage.config import config


logger = logging.getLogger(__name__)


class VectorError(Exception):
    """Base exception for vector operations."""
    pass


class VectorDimensionError(VectorError):
    """Exception raised when vector dimensions don't match expected size."""
    pass


class VectorSearchError(VectorError):
    """Exception raised during vector search operations."""
    pass


class VectorOperations:
    """High-performance vector storage and similarity search operations.

    Provides comprehensive vector operations including storage, similarity search,
    batch processing, and performance optimization with caching.
    """

    def __init__(
        self,
        session: AsyncSession,
        expected_dimension: int = 768,
        distance_metric: str = "l2",
        cache_enabled: bool = True,
    ):
        """Initialize vector operations.

        Args:
            session: Database session for vector operations
            expected_dimension: Expected vector dimension (default 768 for nomic-embed-text)
            distance_metric: Distance metric for similarity search (l2, cosine, inner_product)
            cache_enabled: Whether to enable result caching
        """
        self.session = session
        self.expected_dimension = expected_dimension
        self.distance_metric = distance_metric.lower()
        self.cache_enabled = cache_enabled
        self._search_cache: Dict[str, Any] = {}

        # Validate distance metric
        if self.distance_metric not in ["l2", "cosine", "inner_product"]:
            raise ValueError(f"Unsupported distance metric: {distance_metric}")

    async def store_embedding(
        self,
        entity_type: str,
        entity_id: int,
        embedding: List[float],
        embedding_model: str,
        text_content: Optional[str] = None,
        model_version: Optional[str] = None,
        generation_confidence: Optional[float] = None,
        **entity_refs,
    ) -> int:
        """Store a single embedding in the database.

        Args:
            entity_type: Type of entity being embedded (source, assertion, etc.)
            entity_id: ID of the entity being embedded
            embedding: 768-dimensional vector embedding
            embedding_model: Model used to generate the embedding
            text_content: Original text content that was embedded
            model_version: Version of the embedding model
            generation_confidence: Confidence score for the embedding generation
            **entity_refs: Specific entity foreign key references (source_id, assertion_id, etc.)

        Returns:
            ID of the stored embedding

        Raises:
            VectorDimensionError: If embedding dimension doesn't match expected
            SQLAlchemyError: Database operation error
        """
        # Validate embedding dimension
        if len(embedding) != self.expected_dimension:
            raise VectorDimensionError(
                f"Expected {self.expected_dimension}D vector, got {len(embedding)}D"
            )

        # Generate content hash if text provided
        content_hash = None
        if text_content:
            import hashlib
            content_hash = hashlib.sha256(text_content.encode()).hexdigest()

        try:
            embedding_record = Embedding(
                entity_type=entity_type,
                entity_id=entity_id,
                embedding=embedding,
                embedding_model=embedding_model,
                model_version=model_version,
                text_content=text_content,
                content_hash=content_hash,
                generation_confidence=generation_confidence,
                **entity_refs,
            )

            self.session.add(embedding_record)
            await self.session.flush()  # Get ID without committing

            logger.debug(
                f"Stored embedding {embedding_record.id} for {entity_type} {entity_id}"
            )
            return embedding_record.id

        except SQLAlchemyError as e:
            logger.error(f"Failed to store embedding for {entity_type} {entity_id}: {e}")
            raise

    async def store_embeddings_batch(
        self,
        embeddings_data: List[Dict[str, Any]],
        batch_size: int = 100,
    ) -> List[int]:
        """Store multiple embeddings in batches for better performance.

        Args:
            embeddings_data: List of embedding data dictionaries
            batch_size: Number of embeddings to process per batch

        Returns:
            List of embedding IDs in the same order as input

        Raises:
            VectorDimensionError: If any embedding dimension doesn't match expected
            SQLAlchemyError: Database operation error
        """
        if not embeddings_data:
            return []

        # Validate all embeddings first
        for i, data in enumerate(embeddings_data):
            embedding = data.get("embedding", [])
            if len(embedding) != self.expected_dimension:
                raise VectorDimensionError(
                    f"Embedding {i}: Expected {self.expected_dimension}D vector, got {len(embedding)}D"
                )

        stored_ids = []

        try:
            # Process in batches
            for i in range(0, len(embeddings_data), batch_size):
                batch = embeddings_data[i:i + batch_size]
                batch_ids = []

                for data in batch:
                    # Generate content hash if text provided
                    content_hash = None
                    if data.get("text_content"):
                        import hashlib
                        content_hash = hashlib.sha256(
                            data["text_content"].encode()
                        ).hexdigest()

                    # Extract entity refs
                    entity_refs = {
                        k: v for k, v in data.items()
                        if k.endswith("_id") and k != "entity_id"
                    }

                    embedding_record = Embedding(
                        entity_type=data["entity_type"],
                        entity_id=data["entity_id"],
                        embedding=data["embedding"],
                        embedding_model=data["embedding_model"],
                        model_version=data.get("model_version"),
                        text_content=data.get("text_content"),
                        content_hash=content_hash,
                        generation_confidence=data.get("generation_confidence"),
                        **entity_refs,
                    )

                    self.session.add(embedding_record)

                # Flush batch to get IDs
                await self.session.flush()

                # Get IDs from this batch
                batch_records = []
                for j, record in enumerate(self.session.identity_map.values()):
                    if isinstance(record, Embedding) and record.id is not None:
                        if record not in batch_records:
                            batch_records.append(record)

                batch_ids.extend([record.id for record in batch_records[-len(batch):]])
                stored_ids.extend(batch_ids)

                logger.debug(f"Stored batch of {len(batch)} embeddings")

            logger.info(f"Stored {len(stored_ids)} embeddings in total")
            return stored_ids

        except SQLAlchemyError as e:
            logger.error(f"Failed to store embedding batch: {e}")
            raise

    async def similarity_search(
        self,
        query_vector: List[float],
        limit: int = 10,
        entity_types: Optional[List[str]] = None,
        distance_threshold: Optional[float] = None,
        embedding_models: Optional[List[str]] = None,
        min_confidence: Optional[float] = None,
        include_distances: bool = True,
    ) -> List[Tuple[Embedding, Optional[float]]]:
        """Perform similarity search using vector distance.

        Args:
            query_vector: Query vector for similarity search
            limit: Maximum number of results to return
            entity_types: Optional filter by entity types
            distance_threshold: Optional maximum distance threshold
            embedding_models: Optional filter by embedding models
            min_confidence: Optional minimum confidence score
            include_distances: Whether to include distance scores in results

        Returns:
            List of tuples (embedding, distance) sorted by similarity

        Raises:
            VectorDimensionError: If query vector dimension doesn't match expected
            VectorSearchError: Search operation error
        """
        # Validate query vector dimension
        if len(query_vector) != self.expected_dimension:
            raise VectorDimensionError(
                f"Expected {self.expected_dimension}D query vector, got {len(query_vector)}D"
            )

        # Check cache
        cache_key = self._get_search_cache_key(
            query_vector, limit, entity_types, distance_threshold,
            embedding_models, min_confidence
        )

        if self.cache_enabled and cache_key in self._search_cache:
            logger.debug("Returning cached search results")
            return self._search_cache[cache_key]

        try:
            # Choose distance operator based on metric
            if self.distance_metric == "l2":
                distance_expr = Embedding.embedding.l2_distance(query_vector)
            elif self.distance_metric == "cosine":
                distance_expr = Embedding.embedding.cosine_distance(query_vector)
            elif self.distance_metric == "inner_product":
                distance_expr = Embedding.embedding.max_inner_product(query_vector)
            else:
                raise VectorSearchError(f"Unsupported distance metric: {self.distance_metric}")

            # Build query
            if include_distances:
                query = select(Embedding, distance_expr.label("distance"))
            else:
                query = select(Embedding)

            # Add filters
            conditions = []

            if entity_types:
                conditions.append(Embedding.entity_type.in_(entity_types))

            if distance_threshold is not None:
                conditions.append(distance_expr <= distance_threshold)

            if embedding_models:
                conditions.append(Embedding.embedding_model.in_(embedding_models))

            if min_confidence is not None:
                conditions.append(Embedding.generation_confidence >= min_confidence)

            if conditions:
                query = query.where(and_(*conditions))

            # Order by similarity and limit
            query = query.order_by(distance_expr).limit(limit)

            # Execute query
            result = await self.session.execute(query)

            if include_distances:
                results = [(row.Embedding, row.distance) for row in result]
            else:
                results = [(row, None) for row in result.scalars()]

            # Update search statistics
            await self._update_search_stats([embedding for embedding, _ in results])

            # Cache results
            if self.cache_enabled:
                self._search_cache[cache_key] = results

            logger.debug(f"Found {len(results)} similar embeddings")
            return results

        except SQLAlchemyError as e:
            logger.error(f"Vector similarity search failed: {e}")
            raise VectorSearchError(f"Search failed: {e}")

    async def get_embedding_by_entity(
        self,
        entity_type: str,
        entity_id: int,
        embedding_model: Optional[str] = None,
    ) -> Optional[Embedding]:
        """Get embedding for a specific entity.

        Args:
            entity_type: Type of entity
            entity_id: ID of entity
            embedding_model: Optional specific embedding model filter

        Returns:
            Embedding record or None if not found
        """
        try:
            query = select(Embedding).where(
                and_(
                    Embedding.entity_type == entity_type,
                    Embedding.entity_id == entity_id,
                )
            )

            if embedding_model:
                query = query.where(Embedding.embedding_model == embedding_model)

            # Get most recent if multiple exist
            query = query.order_by(desc(Embedding.created_at))

            result = await self.session.execute(query)
            return result.scalar_one_or_none()

        except SQLAlchemyError as e:
            logger.error(f"Failed to get embedding for {entity_type} {entity_id}: {e}")
            return None

    async def delete_embeddings(
        self,
        entity_type: Optional[str] = None,
        entity_id: Optional[int] = None,
        embedding_model: Optional[str] = None,
        older_than: Optional[datetime] = None,
    ) -> int:
        """Delete embeddings matching the specified criteria.

        Args:
            entity_type: Optional entity type filter
            entity_id: Optional entity ID filter
            embedding_model: Optional embedding model filter
            older_than: Optional date filter for cleanup

        Returns:
            Number of embeddings deleted
        """
        try:
            conditions = []

            if entity_type:
                conditions.append(Embedding.entity_type == entity_type)

            if entity_id:
                conditions.append(Embedding.entity_id == entity_id)

            if embedding_model:
                conditions.append(Embedding.embedding_model == embedding_model)

            if older_than:
                conditions.append(Embedding.created_at < older_than)

            if not conditions:
                logger.warning("delete_embeddings called without any conditions - skipping")
                return 0

            # Count before delete for reporting
            count_query = select(func.count()).select_from(Embedding).where(and_(*conditions))
            result = await self.session.execute(count_query)
            count = result.scalar()

            if count == 0:
                return 0

            # Perform delete
            delete_query = text("""
                DELETE FROM embeddings
                WHERE """ + " AND ".join([
                f"{condition.left.name} = :{i}" for i, condition in enumerate(conditions)
            ]))

            # Build parameter dictionary
            params = {str(i): condition.right.value for i, condition in enumerate(conditions)}

            await self.session.execute(delete_query, params)

            logger.info(f"Deleted {count} embeddings")
            return count

        except SQLAlchemyError as e:
            logger.error(f"Failed to delete embeddings: {e}")
            raise

    async def get_embedding_statistics(self) -> Dict[str, Any]:
        """Get statistics about stored embeddings.

        Returns:
            Dictionary with embedding statistics
        """
        try:
            # Total count
            total_result = await self.session.execute(
                select(func.count()).select_from(Embedding)
            )
            total_count = total_result.scalar()

            # Count by entity type
            entity_type_result = await self.session.execute(
                select(
                    Embedding.entity_type,
                    func.count().label("count")
                ).group_by(Embedding.entity_type)
            )
            entity_type_counts = {row.entity_type: row.count for row in entity_type_result}

            # Count by embedding model
            model_result = await self.session.execute(
                select(
                    Embedding.embedding_model,
                    func.count().label("count")
                ).group_by(Embedding.embedding_model)
            )
            model_counts = {row.embedding_model: row.count for row in model_result}

            # Search statistics
            search_stats_result = await self.session.execute(
                select(
                    func.avg(Embedding.search_count).label("avg_searches"),
                    func.max(Embedding.search_count).label("max_searches"),
                    func.count().filter(Embedding.search_count > 0).label("searched_embeddings"),
                )
            )
            search_stats = search_stats_result.first()

            return {
                "total_embeddings": total_count,
                "entity_type_counts": entity_type_counts,
                "model_counts": model_counts,
                "search_statistics": {
                    "average_searches_per_embedding": float(search_stats.avg_searches or 0),
                    "max_searches": search_stats.max_searches or 0,
                    "embeddings_with_searches": search_stats.searched_embeddings or 0,
                },
                "cache_size": len(self._search_cache) if self.cache_enabled else 0,
            }

        except SQLAlchemyError as e:
            logger.error(f"Failed to get embedding statistics: {e}")
            return {}

    def clear_cache(self) -> None:
        """Clear the search result cache."""
        if self.cache_enabled:
            self._search_cache.clear()
            logger.debug("Cleared vector search cache")

    def _get_search_cache_key(
        self,
        query_vector: List[float],
        limit: int,
        entity_types: Optional[List[str]],
        distance_threshold: Optional[float],
        embedding_models: Optional[List[str]],
        min_confidence: Optional[float],
    ) -> str:
        """Generate cache key for search parameters."""
        import hashlib
        import json

        # Create deterministic string representation
        cache_data = {
            "vector_hash": hashlib.md5(str(query_vector).encode()).hexdigest()[:16],
            "limit": limit,
            "entity_types": sorted(entity_types) if entity_types else None,
            "distance_threshold": distance_threshold,
            "embedding_models": sorted(embedding_models) if embedding_models else None,
            "min_confidence": min_confidence,
            "distance_metric": self.distance_metric,
        }

        cache_string = json.dumps(cache_data, sort_keys=True)
        return hashlib.md5(cache_string.encode()).hexdigest()

    async def _update_search_stats(self, embeddings: List[Embedding]) -> None:
        """Update search statistics for embeddings."""
        if not embeddings:
            return

        try:
            # Update search count and last searched timestamp
            embedding_ids = [emb.id for emb in embeddings]
            current_time = datetime.now(timezone.utc)

            update_query = text("""
                UPDATE embeddings
                SET search_count = search_count + 1,
                    last_searched_at = :timestamp
                WHERE id = ANY(:embedding_ids)
            """)

            await self.session.execute(
                update_query,
                {"timestamp": current_time, "embedding_ids": embedding_ids}
            )

            logger.debug(f"Updated search stats for {len(embeddings)} embeddings")

        except SQLAlchemyError as e:
            logger.warning(f"Failed to update search statistics: {e}")


class VectorSearchService:
    """High-level vector search service with advanced features.

    Provides search functionality with result ranking, filtering,
    and performance optimization.
    """

    def __init__(self, session: AsyncSession):
        """Initialize vector search service.

        Args:
            session: Database session
        """
        self.session = session
        self.vector_ops = VectorOperations(session)

    async def semantic_search(
        self,
        query_text: str,
        query_vector: List[float],
        limit: int = 10,
        entity_types: Optional[List[str]] = None,
        boost_recent: bool = True,
        diversity_factor: float = 0.0,
    ) -> List[Dict[str, Any]]:
        """Perform semantic search with enhanced ranking.

        Args:
            query_text: Original query text for context
            query_vector: Pre-computed query vector
            limit: Number of results to return
            entity_types: Optional entity type filter
            boost_recent: Whether to boost recently created content
            diversity_factor: Factor for diversifying results (0.0 = no diversity)

        Returns:
            List of search results with enhanced metadata
        """
        # Perform base similarity search
        raw_results = await self.vector_ops.similarity_search(
            query_vector=query_vector,
            limit=limit * 2 if diversity_factor > 0 else limit,  # Get more for diversity
            entity_types=entity_types,
            include_distances=True,
        )

        # Enhance results with additional metadata and scoring
        enhanced_results = []

        for embedding, distance in raw_results:
            result = {
                "embedding_id": embedding.id,
                "entity_type": embedding.entity_type,
                "entity_id": embedding.entity_id,
                "text_content": embedding.text_content,
                "similarity_score": 1.0 - distance if distance is not None else 1.0,
                "distance": distance,
                "embedding_model": embedding.embedding_model,
                "model_version": embedding.model_version,
                "confidence": embedding.generation_confidence,
                "created_at": embedding.created_at,
                "search_popularity": embedding.search_count,
            }

            # Calculate enhanced score
            base_score = result["similarity_score"]
            enhanced_score = base_score

            # Recent content boost
            if boost_recent and embedding.created_at:
                days_old = (datetime.now(timezone.utc) - embedding.created_at).days
                recency_boost = max(0, 1 - (days_old / 30))  # Linear decay over 30 days
                enhanced_score += recency_boost * 0.1

            # Popularity boost (based on search count)
            if embedding.search_count > 0:
                popularity_boost = min(0.1, embedding.search_count / 100.0)
                enhanced_score += popularity_boost

            result["enhanced_score"] = enhanced_score
            enhanced_results.append(result)

        # Sort by enhanced score
        enhanced_results.sort(key=lambda x: x["enhanced_score"], reverse=True)

        # Apply diversity if requested
        if diversity_factor > 0:
            enhanced_results = self._apply_diversity(enhanced_results, diversity_factor, limit)

        return enhanced_results[:limit]

    def _apply_diversity(
        self,
        results: List[Dict[str, Any]],
        diversity_factor: float,
        target_count: int,
    ) -> List[Dict[str, Any]]:
        """Apply diversity to search results.

        Args:
            results: Search results to diversify
            diversity_factor: Diversity factor (0.0 to 1.0)
            target_count: Target number of diverse results

        Returns:
            Diversified results
        """
        if not results or diversity_factor <= 0:
            return results

        diversified = [results[0]]  # Always include top result
        remaining = results[1:]

        while len(diversified) < target_count and remaining:
            best_candidate = None
            best_score = -1

            for candidate in remaining:
                # Calculate diversity score based on entity type distribution
                type_count = sum(1 for r in diversified if r["entity_type"] == candidate["entity_type"])
                diversity_penalty = type_count * diversity_factor

                adjusted_score = candidate["enhanced_score"] - diversity_penalty

                if adjusted_score > best_score:
                    best_score = adjusted_score
                    best_candidate = candidate

            if best_candidate:
                diversified.append(best_candidate)
                remaining.remove(best_candidate)
            else:
                break

        return diversified