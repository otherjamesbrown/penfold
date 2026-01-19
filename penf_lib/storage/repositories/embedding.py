"""Repository for embedding operations with batch processing and optimization.

This module provides high-level embedding repository operations
with batch processing, caching, and performance optimization.
"""

import logging
from typing import Dict, Any, List, Optional, Tuple, Union
from datetime import datetime, timezone

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, and_, desc
from sqlalchemy.exc import SQLAlchemyError

from penf_lib.storage.models import Embedding
from penf_lib.storage.repositories.base import BaseRepository
from penf_lib.storage.vector import VectorOperations, VectorSearchService


logger = logging.getLogger(__name__)


class EmbeddingRepository(BaseRepository[Embedding]):
    """Repository for embedding operations with vector search capabilities.

    Provides high-level embedding operations including batch processing,
    similarity search, and performance optimization.
    """

    def __init__(self, session: AsyncSession):
        """Initialize embedding repository.

        Args:
            session: Database session
        """
        super().__init__(session, Embedding)
        self.vector_ops = VectorOperations(session)
        self.search_service = VectorSearchService(session)

    async def create_embedding(
        self,
        entity_type: str,
        entity_id: int,
        embedding: List[float],
        embedding_model: str,
        text_content: Optional[str] = None,
        model_version: Optional[str] = None,
        generation_confidence: Optional[float] = None,
        **entity_refs,
    ) -> Embedding:
        """Create and store a new embedding.

        Args:
            entity_type: Type of entity being embedded
            entity_id: ID of the entity being embedded
            embedding: Vector embedding
            embedding_model: Model used to generate the embedding
            text_content: Original text content
            model_version: Version of the embedding model
            generation_confidence: Confidence score for generation
            **entity_refs: Specific entity foreign key references

        Returns:
            Created embedding record

        Raises:
            ValueError: Invalid input data
            SQLAlchemyError: Database operation error
        """
        if not entity_type or not entity_id or not embedding or not embedding_model:
            raise ValueError("entity_type, entity_id, embedding, and embedding_model are required")

        try:
            embedding_id = await self.vector_ops.store_embedding(
                entity_type=entity_type,
                entity_id=entity_id,
                embedding=embedding,
                embedding_model=embedding_model,
                text_content=text_content,
                model_version=model_version,
                generation_confidence=generation_confidence,
                **entity_refs,
            )

            # Get the created embedding
            created_embedding = await self.get_by_id(embedding_id)
            if not created_embedding:
                raise SQLAlchemyError("Failed to retrieve created embedding")

            logger.info(f"Created embedding {embedding_id} for {entity_type} {entity_id}")
            return created_embedding

        except Exception as e:
            logger.error(f"Failed to create embedding: {e}")
            raise

    async def create_embeddings_batch(
        self,
        embeddings_data: List[Dict[str, Any]],
        batch_size: int = 100,
    ) -> List[Embedding]:
        """Create multiple embeddings in batches.

        Args:
            embeddings_data: List of embedding data dictionaries
            batch_size: Number of embeddings to process per batch

        Returns:
            List of created embedding records

        Raises:
            ValueError: Invalid input data
            SQLAlchemyError: Database operation error
        """
        if not embeddings_data:
            return []

        # Validate required fields
        for i, data in enumerate(embeddings_data):
            required_fields = ["entity_type", "entity_id", "embedding", "embedding_model"]
            missing_fields = [field for field in required_fields if field not in data]
            if missing_fields:
                raise ValueError(f"Embedding {i} missing required fields: {missing_fields}")

        try:
            # Store embeddings in batches
            embedding_ids = await self.vector_ops.store_embeddings_batch(
                embeddings_data, batch_size
            )

            # Retrieve created embeddings
            embeddings = []
            for embedding_id in embedding_ids:
                embedding = await self.get_by_id(embedding_id)
                if embedding:
                    embeddings.append(embedding)

            logger.info(f"Created {len(embeddings)} embeddings in batch")
            return embeddings

        except Exception as e:
            logger.error(f"Failed to create embedding batch: {e}")
            raise

    async def find_similar(
        self,
        query_vector: List[float],
        limit: int = 10,
        entity_types: Optional[List[str]] = None,
        distance_threshold: Optional[float] = None,
        embedding_models: Optional[List[str]] = None,
        min_confidence: Optional[float] = None,
        include_distances: bool = True,
    ) -> List[Tuple[Embedding, Optional[float]]]:
        """Find similar embeddings using vector similarity search.

        Args:
            query_vector: Query vector for similarity search
            limit: Maximum number of results
            entity_types: Optional filter by entity types
            distance_threshold: Optional maximum distance threshold
            embedding_models: Optional filter by embedding models
            min_confidence: Optional minimum confidence score
            include_distances: Whether to include distance scores

        Returns:
            List of tuples (embedding, distance) sorted by similarity

        Raises:
            ValueError: Invalid query vector
            VectorSearchError: Search operation error
        """
        try:
            return await self.vector_ops.similarity_search(
                query_vector=query_vector,
                limit=limit,
                entity_types=entity_types,
                distance_threshold=distance_threshold,
                embedding_models=embedding_models,
                min_confidence=min_confidence,
                include_distances=include_distances,
            )

        except Exception as e:
            logger.error(f"Similarity search failed: {e}")
            raise

    async def semantic_search(
        self,
        query_text: str,
        query_vector: List[float],
        limit: int = 10,
        entity_types: Optional[List[str]] = None,
        boost_recent: bool = True,
        diversity_factor: float = 0.0,
    ) -> List[Dict[str, Any]]:
        """Perform enhanced semantic search with ranking and diversity.

        Args:
            query_text: Original query text for context
            query_vector: Pre-computed query vector
            limit: Number of results to return
            entity_types: Optional entity type filter
            boost_recent: Whether to boost recently created content
            diversity_factor: Factor for diversifying results

        Returns:
            List of enhanced search results

        Raises:
            ValueError: Invalid input parameters
            VectorSearchError: Search operation error
        """
        try:
            return await self.search_service.semantic_search(
                query_text=query_text,
                query_vector=query_vector,
                limit=limit,
                entity_types=entity_types,
                boost_recent=boost_recent,
                diversity_factor=diversity_factor,
            )

        except Exception as e:
            logger.error(f"Semantic search failed: {e}")
            raise

    async def get_by_entity(
        self,
        entity_type: str,
        entity_id: int,
        embedding_model: Optional[str] = None,
    ) -> Optional[Embedding]:
        """Get embedding for a specific entity.

        Args:
            entity_type: Type of entity
            entity_id: ID of entity
            embedding_model: Optional specific embedding model

        Returns:
            Embedding record or None if not found
        """
        try:
            return await self.vector_ops.get_embedding_by_entity(
                entity_type=entity_type,
                entity_id=entity_id,
                embedding_model=embedding_model,
            )

        except Exception as e:
            logger.error(f"Failed to get embedding for {entity_type} {entity_id}: {e}")
            return None

    async def get_by_entity_batch(
        self,
        entity_requests: List[Tuple[str, int, Optional[str]]],
    ) -> List[Optional[Embedding]]:
        """Get embeddings for multiple entities in batch.

        Args:
            entity_requests: List of (entity_type, entity_id, embedding_model) tuples

        Returns:
            List of embedding records (None for not found) in same order as requests
        """
        if not entity_requests:
            return []

        try:
            results = []
            for entity_type, entity_id, embedding_model in entity_requests:
                embedding = await self.get_by_entity(entity_type, entity_id, embedding_model)
                results.append(embedding)

            return results

        except Exception as e:
            logger.error(f"Failed to get embeddings batch: {e}")
            return [None] * len(entity_requests)

    async def get_embeddings_by_model(
        self,
        embedding_model: str,
        model_version: Optional[str] = None,
        entity_types: Optional[List[str]] = None,
        limit: Optional[int] = None,
        offset: int = 0,
    ) -> List[Embedding]:
        """Get embeddings by model and version.

        Args:
            embedding_model: Embedding model name
            model_version: Optional specific model version
            entity_types: Optional filter by entity types
            limit: Optional limit on results
            offset: Offset for pagination

        Returns:
            List of matching embeddings
        """
        try:
            query = select(self.model).where(
                self.model.embedding_model == embedding_model
            )

            if model_version:
                query = query.where(self.model.model_version == model_version)

            if entity_types:
                query = query.where(self.model.entity_type.in_(entity_types))

            query = query.order_by(desc(self.model.created_at))

            if offset:
                query = query.offset(offset)

            if limit:
                query = query.limit(limit)

            result = await self.session.execute(query)
            return list(result.scalars())

        except SQLAlchemyError as e:
            logger.error(f"Failed to get embeddings by model {embedding_model}: {e}")
            return []

    async def delete_by_entity(
        self,
        entity_type: str,
        entity_id: int,
        embedding_model: Optional[str] = None,
    ) -> int:
        """Delete embeddings for a specific entity.

        Args:
            entity_type: Type of entity
            entity_id: ID of entity
            embedding_model: Optional specific embedding model

        Returns:
            Number of embeddings deleted
        """
        try:
            return await self.vector_ops.delete_embeddings(
                entity_type=entity_type,
                entity_id=entity_id,
                embedding_model=embedding_model,
            )

        except Exception as e:
            logger.error(f"Failed to delete embeddings for {entity_type} {entity_id}: {e}")
            return 0

    async def delete_by_model(
        self,
        embedding_model: str,
        model_version: Optional[str] = None,
        older_than: Optional[datetime] = None,
    ) -> int:
        """Delete embeddings by model criteria.

        Args:
            embedding_model: Embedding model to delete
            model_version: Optional specific model version
            older_than: Optional date filter for cleanup

        Returns:
            Number of embeddings deleted
        """
        try:
            conditions = [self.model.embedding_model == embedding_model]

            if model_version:
                conditions.append(self.model.model_version == model_version)

            if older_than:
                conditions.append(self.model.created_at < older_than)

            return await self._delete_by_conditions(conditions)

        except Exception as e:
            logger.error(f"Failed to delete embeddings by model {embedding_model}: {e}")
            return 0

    async def get_statistics(self) -> Dict[str, Any]:
        """Get comprehensive embedding statistics.

        Returns:
            Dictionary with detailed embedding statistics
        """
        try:
            # Get base statistics from vector operations
            stats = await self.vector_ops.get_embedding_statistics()

            # Add repository-specific statistics
            # Average confidence by model
            confidence_result = await self.session.execute(
                select(
                    self.model.embedding_model,
                    func.avg(self.model.generation_confidence).label("avg_confidence"),
                    func.count().label("count"),
                ).where(
                    self.model.generation_confidence.isnot(None)
                ).group_by(self.model.embedding_model)
            )

            confidence_stats = {}
            for row in confidence_result:
                confidence_stats[row.embedding_model] = {
                    "average_confidence": float(row.avg_confidence or 0),
                    "count_with_confidence": row.count,
                }

            stats["confidence_statistics"] = confidence_stats

            # Content statistics
            content_result = await self.session.execute(
                select(
                    func.count().filter(self.model.text_content.isnot(None)).label("with_content"),
                    func.avg(func.length(self.model.text_content)).label("avg_content_length"),
                    func.count().filter(self.model.content_hash.isnot(None)).label("with_hash"),
                )
            )

            content_stats = content_result.first()
            stats["content_statistics"] = {
                "embeddings_with_content": content_stats.with_content or 0,
                "average_content_length": float(content_stats.avg_content_length or 0),
                "embeddings_with_hash": content_stats.with_hash or 0,
            }

            return stats

        except SQLAlchemyError as e:
            logger.error(f"Failed to get embedding statistics: {e}")
            return {}

    async def cleanup_old_embeddings(
        self,
        older_than: datetime,
        embedding_model: Optional[str] = None,
        dry_run: bool = True,
    ) -> int:
        """Clean up old embeddings.

        Args:
            older_than: Delete embeddings older than this date
            embedding_model: Optional filter by embedding model
            dry_run: If True, only count what would be deleted

        Returns:
            Number of embeddings deleted (or would be deleted in dry run)
        """
        try:
            # Count embeddings to be deleted
            conditions = [self.model.created_at < older_than]

            if embedding_model:
                conditions.append(self.model.embedding_model == embedding_model)

            count_query = select(func.count()).where(and_(*conditions))
            count_result = await self.session.execute(count_query)
            count = count_result.scalar()

            if dry_run:
                logger.info(f"Would delete {count} old embeddings (dry run)")
                return count

            if count == 0:
                return 0

            # Perform deletion using vector operations
            deleted = await self.vector_ops.delete_embeddings(
                embedding_model=embedding_model,
                older_than=older_than,
            )

            logger.info(f"Cleaned up {deleted} old embeddings")
            return deleted

        except Exception as e:
            logger.error(f"Failed to cleanup old embeddings: {e}")
            return 0

    async def recompute_content_hashes(self, batch_size: int = 1000) -> int:
        """Recompute missing content hashes for embeddings with text content.

        Args:
            batch_size: Number of embeddings to process per batch

        Returns:
            Number of content hashes recomputed
        """
        try:
            import hashlib

            # Find embeddings with content but no hash
            query = select(self.model).where(
                and_(
                    self.model.text_content.isnot(None),
                    self.model.content_hash.is_(None),
                )
            ).limit(batch_size)

            result = await self.session.execute(query)
            embeddings = list(result.scalars())

            if not embeddings:
                return 0

            # Update content hashes
            updated_count = 0
            for embedding in embeddings:
                if embedding.text_content:
                    content_hash = hashlib.sha256(embedding.text_content.encode()).hexdigest()
                    embedding.content_hash = content_hash
                    updated_count += 1

            await self.session.flush()

            logger.info(f"Recomputed {updated_count} content hashes")
            return updated_count

        except SQLAlchemyError as e:
            logger.error(f"Failed to recompute content hashes: {e}")
            return 0

    def clear_search_cache(self) -> None:
        """Clear the vector search cache."""
        self.vector_ops.clear_cache()
        logger.debug("Cleared embedding search cache")

    async def _delete_by_conditions(self, conditions: List[Any]) -> int:
        """Helper method to delete embeddings by conditions.

        Args:
            conditions: SQLAlchemy conditions for deletion

        Returns:
            Number of embeddings deleted
        """
        if not conditions:
            return 0

        # Count first
        count_query = select(func.count()).where(and_(*conditions))
        count_result = await self.session.execute(count_query)
        count = count_result.scalar()

        if count == 0:
            return 0

        # Delete using base repository method
        deleted = await self.delete_by_conditions(conditions)

        logger.info(f"Deleted {deleted} embeddings matching conditions")
        return deleted