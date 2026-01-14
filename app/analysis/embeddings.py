"""
Embedding Generation for Semantic Search

Generates vector embeddings for meeting transcripts, summaries, and insights
to enable semantic search capabilities using OpenAI embeddings.
"""
import openai
import numpy as np
from typing import Dict, Any, List, Optional, Tuple
import asyncio
from datetime import datetime
import json
import uuid
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update

from app.config import settings
from app.database import MeetingTranscript, MeetingSummary


class EmbeddingGenerator:
    """Generates embeddings for meeting content using OpenAI embeddings API"""

    def __init__(self):
        self.client_available = settings.OPENAI_API_KEY is not None
        if self.client_available:
            openai.api_key = settings.OPENAI_API_KEY

        # Embedding model configuration
        self.embedding_model = settings.EMBEDDING_MODEL
        self.embedding_dimension = settings.EMBEDDING_DIMENSION
        self.max_tokens_per_chunk = 8000  # OpenAI embedding limit

        # Content preparation settings
        self.chunk_overlap = 200  # words
        self.summary_weight = 0.6  # Weight summary higher than transcript
        self.metadata_weight = 0.3  # Weight for meeting metadata

    async def generate_meeting_embeddings(
        self,
        meeting_id: str,
        transcript_text: str,
        summary_data: Dict[str, Any],
        meeting_metadata: Dict[str, Any],
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Generate embeddings for all meeting content"""

        if not self.client_available:
            return self._create_error_result("OpenAI client not available")

        try:
            print(f"🔢 Generating embeddings for meeting {meeting_id}")

            # Prepare content for embedding
            content_chunks = await self._prepare_content_chunks(
                transcript_text, summary_data, meeting_metadata
            )

            # Generate embeddings for each chunk
            embedding_results = await self._generate_chunk_embeddings(content_chunks)

            # Create composite embeddings
            transcript_embedding = await self._create_transcript_embedding(
                transcript_text, embedding_results
            )

            summary_embedding = await self._create_summary_embedding(
                summary_data, embedding_results
            )

            # Update database with embeddings
            await self._save_embeddings_to_database(
                meeting_id, transcript_embedding, summary_embedding, db
            )

            # Generate searchable metadata
            search_metadata = self._generate_search_metadata(
                transcript_text, summary_data, meeting_metadata
            )

            result = {
                "meeting_id": meeting_id,
                "embeddings_generated": True,
                "transcript_embedding_dimension": len(transcript_embedding) if transcript_embedding else 0,
                "summary_embedding_dimension": len(summary_embedding) if summary_embedding else 0,
                "content_chunks": len(content_chunks),
                "search_metadata": search_metadata,
                "processing_metadata": {
                    "generated_at": datetime.utcnow().isoformat() + "Z",
                    "model_used": self.embedding_model,
                    "chunk_count": len(content_chunks),
                    "total_tokens_processed": sum(chunk["estimated_tokens"] for chunk in content_chunks)
                }
            }

            print(f"✅ Generated embeddings: {len(content_chunks)} chunks processed")
            return result

        except Exception as e:
            print(f"❌ Embedding generation failed: {str(e)}")
            return self._create_error_result(str(e))

    async def _prepare_content_chunks(
        self,
        transcript_text: str,
        summary_data: Dict[str, Any],
        meeting_metadata: Dict[str, Any]
    ) -> List[Dict[str, Any]]:
        """Prepare content chunks for embedding generation"""

        chunks = []

        # Chunk 1: Summary-focused content (highest weight)
        summary_content = self._build_summary_content(summary_data, meeting_metadata)
        chunks.append({
            "type": "summary",
            "content": summary_content,
            "weight": self.summary_weight,
            "estimated_tokens": len(summary_content.split()) * 1.3  # Rough token estimate
        })

        # Chunk 2: Metadata-focused content
        metadata_content = self._build_metadata_content(meeting_metadata, summary_data)
        chunks.append({
            "type": "metadata",
            "content": metadata_content,
            "weight": self.metadata_weight,
            "estimated_tokens": len(metadata_content.split()) * 1.3
        })

        # Chunk 3+: Transcript chunks (if needed)
        transcript_chunks = self._chunk_transcript(transcript_text)
        for i, chunk_text in enumerate(transcript_chunks):
            chunks.append({
                "type": "transcript",
                "chunk_index": i,
                "content": chunk_text,
                "weight": 1.0 - self.summary_weight - self.metadata_weight,
                "estimated_tokens": len(chunk_text.split()) * 1.3
            })

        return chunks

    def _build_summary_content(self, summary_data: Dict[str, Any], meeting_metadata: Dict[str, Any]) -> str:
        """Build summary-focused content for embedding"""

        parts = []

        # Add executive summary
        if summary_data.get("executive_summary"):
            parts.append(f"Executive Summary: {summary_data['executive_summary']}")

        # Add main summary
        if summary_data.get("summary_text"):
            parts.append(f"Meeting Summary: {summary_data['summary_text']}")

        # Add key points
        key_points = summary_data.get("key_points", [])
        if key_points:
            key_point_text = " ".join([point.get("point", "") for point in key_points])
            parts.append(f"Key Points: {key_point_text}")

        # Add decisions
        decisions = summary_data.get("decisions", [])
        if decisions:
            decision_text = " ".join([decision.get("decision", "") for decision in decisions])
            parts.append(f"Decisions Made: {decision_text}")

        # Add action items
        action_items = summary_data.get("action_items", [])
        if action_items:
            action_text = " ".join([item.get("item", "") for item in action_items])
            parts.append(f"Action Items: {action_text}")

        # Add meeting classification
        classification = summary_data.get("meeting_classification", {})
        if classification:
            meeting_type = classification.get("type", "")
            domain = classification.get("domain", "")
            if meeting_type:
                parts.append(f"Meeting Type: {meeting_type}")
            if domain:
                parts.append(f"Business Domain: {domain}")

        return " | ".join(parts)

    def _build_metadata_content(self, meeting_metadata: Dict[str, Any], summary_data: Dict[str, Any]) -> str:
        """Build metadata-focused content for embedding"""

        parts = []

        # Add filename context
        filename = meeting_metadata.get("filename", "")
        if filename:
            parts.append(f"File: {filename}")

        # Add participant information
        participant_count = meeting_metadata.get("participant_count", 0)
        if participant_count:
            parts.append(f"Participants: {participant_count} people")

        # Add duration context
        duration = meeting_metadata.get("audio_duration", 0)
        if duration:
            duration_minutes = int(duration / 60)
            parts.append(f"Duration: {duration_minutes} minutes")

        # Add privacy level context
        privacy_level = meeting_metadata.get("privacy_level", "")
        if privacy_level:
            parts.append(f"Privacy: {privacy_level}")

        # Add confidence indicators
        confidence_metrics = summary_data.get("confidence_metrics", {})
        if confidence_metrics:
            overall_confidence = confidence_metrics.get("overall_confidence", 0)
            if overall_confidence:
                quality = "high" if overall_confidence > 0.8 else "medium" if overall_confidence > 0.5 else "low"
                parts.append(f"Quality: {quality} confidence")

        return " | ".join(parts)

    def _chunk_transcript(self, transcript_text: str) -> List[str]:
        """Split transcript into manageable chunks for embedding"""

        words = transcript_text.split()
        chunk_size = 1500  # words per chunk
        chunks = []

        for i in range(0, len(words), chunk_size - self.chunk_overlap):
            chunk_words = words[i:i + chunk_size]
            chunk_text = " ".join(chunk_words)

            if len(chunk_text.strip()) > 50:  # Only include meaningful chunks
                chunks.append(chunk_text)

        return chunks

    async def _generate_chunk_embeddings(self, content_chunks: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Generate embeddings for all content chunks"""

        embedding_results = []

        for chunk in content_chunks:
            try:
                # Generate embedding for this chunk
                embedding = await self._generate_single_embedding(chunk["content"])

                embedding_results.append({
                    "type": chunk["type"],
                    "content": chunk["content"],
                    "weight": chunk["weight"],
                    "embedding": embedding,
                    "dimension": len(embedding) if embedding else 0,
                    "chunk_index": chunk.get("chunk_index", 0)
                })

                # Add small delay to respect API limits
                await asyncio.sleep(0.1)

            except Exception as e:
                print(f"⚠️ Failed to generate embedding for {chunk['type']} chunk: {str(e)}")
                embedding_results.append({
                    "type": chunk["type"],
                    "error": str(e),
                    "weight": chunk["weight"]
                })

        return embedding_results

    async def _generate_single_embedding(self, text: str) -> Optional[List[float]]:
        """Generate embedding for a single text chunk"""

        try:
            # Clean and prepare text
            cleaned_text = self._clean_text_for_embedding(text)

            if len(cleaned_text.strip()) < 10:  # Skip very short text
                return None

            # Generate embedding using OpenAI API
            response = await asyncio.to_thread(
                openai.Embedding.create,
                input=cleaned_text,
                model=self.embedding_model
            )

            embedding = response['data'][0]['embedding']
            return embedding

        except Exception as e:
            print(f"⚠️ OpenAI embedding generation failed: {str(e)}")
            return None

    def _clean_text_for_embedding(self, text: str) -> str:
        """Clean text for optimal embedding generation"""

        # Remove excessive whitespace
        text = " ".join(text.split())

        # Remove very short words and filler words
        words = text.split()
        cleaned_words = []

        filler_words = {"uh", "um", "ah", "er", "like", "you know", "so", "well"}

        for word in words:
            cleaned_word = word.strip(".,!?;:")
            if len(cleaned_word) > 2 and cleaned_word.lower() not in filler_words:
                cleaned_words.append(word)  # Keep original formatting

        cleaned_text = " ".join(cleaned_words)

        # Truncate if too long (safety check)
        if len(cleaned_text.split()) > 8000:
            cleaned_text = " ".join(cleaned_text.split()[:8000])

        return cleaned_text

    async def _create_transcript_embedding(
        self,
        transcript_text: str,
        embedding_results: List[Dict[str, Any]]
    ) -> Optional[List[float]]:
        """Create composite embedding for the full transcript"""

        # Get transcript chunk embeddings
        transcript_embeddings = [
            result["embedding"] for result in embedding_results
            if result["type"] == "transcript" and "embedding" in result and result["embedding"]
        ]

        if not transcript_embeddings:
            # Fallback: generate embedding for truncated transcript
            truncated_transcript = " ".join(transcript_text.split()[:1500])
            return await self._generate_single_embedding(truncated_transcript)

        # Average the transcript chunk embeddings
        return self._average_embeddings(transcript_embeddings)

    async def _create_summary_embedding(
        self,
        summary_data: Dict[str, Any],
        embedding_results: List[Dict[str, Any]]
    ) -> Optional[List[float]]:
        """Create embedding for the meeting summary"""

        # Get summary embedding
        summary_embeddings = [
            result["embedding"] for result in embedding_results
            if result["type"] == "summary" and "embedding" in result and result["embedding"]
        ]

        if summary_embeddings:
            return summary_embeddings[0]  # Use the summary embedding directly

        # Fallback: generate embedding for summary text
        summary_text = summary_data.get("summary_text", "")
        if summary_text:
            return await self._generate_single_embedding(summary_text)

        return None

    def _average_embeddings(self, embeddings: List[List[float]]) -> Optional[List[float]]:
        """Average multiple embeddings into a single embedding"""

        if not embeddings:
            return None

        # Convert to numpy for easier computation
        embedding_array = np.array(embeddings)

        # Calculate weighted average (all weights equal for now)
        average_embedding = np.mean(embedding_array, axis=0)

        # Normalize to unit vector
        norm = np.linalg.norm(average_embedding)
        if norm > 0:
            normalized_embedding = average_embedding / norm
        else:
            normalized_embedding = average_embedding

        return normalized_embedding.tolist()

    async def _save_embeddings_to_database(
        self,
        meeting_id: str,
        transcript_embedding: Optional[List[float]],
        summary_embedding: Optional[List[float]],
        db: AsyncSession
    ):
        """Save embeddings to database tables"""

        try:
            # Update transcript table with embedding
            if transcript_embedding:
                stmt = update(MeetingTranscript).where(
                    MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
                ).values(
                    embedding=transcript_embedding
                )
                await db.execute(stmt)

            # Update summary table with embedding
            if summary_embedding:
                stmt = update(MeetingSummary).where(
                    MeetingSummary.meeting_file_id == uuid.UUID(meeting_id)
                ).values(
                    summary_embedding=summary_embedding
                )
                await db.execute(stmt)

            await db.commit()
            print(f"💾 Saved embeddings to database for meeting {meeting_id}")

        except Exception as e:
            print(f"❌ Failed to save embeddings: {str(e)}")
            await db.rollback()
            raise

    def _generate_search_metadata(
        self,
        transcript_text: str,
        summary_data: Dict[str, Any],
        meeting_metadata: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Generate metadata for search optimization"""

        # Extract key terms for search
        key_terms = self._extract_search_terms(transcript_text, summary_data)

        # Generate search tags
        search_tags = self._generate_search_tags(summary_data, meeting_metadata)

        # Calculate content characteristics
        content_stats = {
            "word_count": len(transcript_text.split()),
            "key_point_count": len(summary_data.get("key_points", [])),
            "decision_count": len(summary_data.get("decisions", [])),
            "action_item_count": len(summary_data.get("action_items", [])),
            "participant_count": meeting_metadata.get("participant_count", 0)
        }

        return {
            "key_terms": key_terms,
            "search_tags": search_tags,
            "content_statistics": content_stats,
            "searchable_content": {
                "has_decisions": content_stats["decision_count"] > 0,
                "has_action_items": content_stats["action_item_count"] > 0,
                "substantial_content": content_stats["word_count"] > 500,
                "multi_participant": content_stats["participant_count"] > 1
            }
        }

    def _extract_search_terms(self, transcript_text: str, summary_data: Dict[str, Any]) -> List[str]:
        """Extract important terms for search indexing"""

        terms = set()

        # Extract from summary
        summary_text = summary_data.get("summary_text", "")
        if summary_text:
            # Simple term extraction (would use more sophisticated NLP in production)
            summary_words = [word.strip(".,!?;:") for word in summary_text.split()]
            important_words = [word for word in summary_words if len(word) > 4]
            terms.update(important_words)

        # Extract from key points
        for point in summary_data.get("key_points", []):
            point_text = point.get("point", "")
            point_words = [word.strip(".,!?;:") for word in point_text.split()]
            important_words = [word for word in point_words if len(word) > 4]
            terms.update(important_words)

        # Extract from decisions
        for decision in summary_data.get("decisions", []):
            decision_text = decision.get("decision", "")
            decision_words = [word.strip(".,!?;:") for word in decision_text.split()]
            important_words = [word for word in decision_words if len(word) > 4]
            terms.update(important_words)

        # Return top terms (by length as a simple heuristic)
        sorted_terms = sorted(terms, key=len, reverse=True)
        return sorted_terms[:20]  # Top 20 terms

    def _generate_search_tags(self, summary_data: Dict[str, Any], meeting_metadata: Dict[str, Any]) -> List[str]:
        """Generate search tags based on meeting content"""

        tags = []

        # Add classification-based tags
        classification = summary_data.get("meeting_classification", {})
        if classification:
            meeting_type = classification.get("type", "")
            domain = classification.get("domain", "")
            urgency = classification.get("urgency", "")

            if meeting_type:
                tags.append(f"type:{meeting_type}")
            if domain:
                tags.append(f"domain:{domain}")
            if urgency:
                tags.append(f"urgency:{urgency}")

        # Add content-based tags
        if summary_data.get("decisions"):
            tags.append("has:decisions")
        if summary_data.get("action_items"):
            tags.append("has:action_items")

        # Add metadata-based tags
        privacy_level = meeting_metadata.get("privacy_level", "")
        if privacy_level:
            tags.append(f"privacy:{privacy_level}")

        participant_count = meeting_metadata.get("participant_count", 0)
        if participant_count > 5:
            tags.append("large_meeting")
        elif participant_count <= 2:
            tags.append("small_meeting")

        return tags

    def _create_error_result(self, error_message: str) -> Dict[str, Any]:
        """Create error result for failed embedding generation"""

        return {
            "error": error_message,
            "embeddings_generated": False,
            "transcript_embedding_dimension": 0,
            "summary_embedding_dimension": 0
        }

    def is_available(self) -> bool:
        """Check if embedding generation is available"""
        return self.client_available

    async def test_embedding_generation(self) -> Dict[str, Any]:
        """Test embedding generation functionality"""

        if not self.client_available:
            return {"available": False, "error": "OpenAI API key not configured"}

        test_text = "This is a test meeting summary for embedding generation testing."

        try:
            embedding = await self._generate_single_embedding(test_text)

            return {
                "available": True,
                "test_successful": embedding is not None,
                "embedding_dimension": len(embedding) if embedding else 0,
                "model_used": self.embedding_model
            }

        except Exception as e:
            return {
                "available": False,
                "test_successful": False,
                "error": str(e)
            }

    async def search_similar_content(
        self,
        query_text: str,
        db: AsyncSession,
        limit: int = 10,
        similarity_threshold: float = 0.7
    ) -> List[Dict[str, Any]]:
        """Search for similar meeting content using embeddings"""

        if not self.client_available:
            return []

        try:
            # Generate embedding for query
            query_embedding = await self._generate_single_embedding(query_text)

            if not query_embedding:
                return []

            # This would be implemented with proper vector similarity search
            # For now, returning placeholder
            return [{
                "meeting_id": "placeholder",
                "similarity_score": 0.85,
                "content_type": "summary",
                "matched_content": "Placeholder search result"
            }]

        except Exception as e:
            print(f"❌ Similarity search failed: {str(e)}")
            return []


# Global embedding generator instance
embedding_generator = EmbeddingGenerator()