"""
Hybrid Search Algorithm

Combines semantic and full-text search with intelligent result ranking
and relevance scoring for optimal meeting discovery.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from typing import List, Dict, Any, Optional, Set, Tuple
import math
from datetime import datetime, timedelta
from collections import defaultdict

from app.search.semantic import semantic_search
from app.search.fulltext import fulltext_search


class HybridSearchService:
    """Service for hybrid search combining semantic and full-text approaches"""

    def __init__(self):
        # Scoring weights for different search components
        self.weights = {
            "semantic_score": 0.4,      # Semantic similarity weight
            "fulltext_score": 0.3,      # Full-text relevance weight
            "recency_score": 0.15,      # Time-based relevance weight
            "quality_score": 0.15       # Transcript/summary quality weight
        }

        # Search thresholds
        self.semantic_threshold = 0.6
        self.fulltext_threshold = 0.1
        self.hybrid_threshold = 0.3

        # Performance settings
        self.max_semantic_results = 50
        self.max_fulltext_results = 50
        self.max_final_results = 20

    async def search_meetings(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        search_mode: str = "auto",  # auto, semantic_only, fulltext_only, hybrid
        limit: int = 20,
        include_summaries: bool = True,
        date_range: Optional[Tuple[datetime, datetime]] = None,
        boost_recent: bool = True
    ) -> Dict[str, Any]:
        """
        Perform hybrid search across meeting content

        Args:
            query_text: Search query
            db: Database session
            privacy_levels: Privacy levels to include
            search_mode: Search strategy to use
            limit: Maximum results to return
            include_summaries: Whether to search summaries in addition to transcripts
            date_range: Optional date range filter (start_date, end_date)
            boost_recent: Whether to boost recent meetings in ranking

        Returns:
            Dictionary containing search results and metadata
        """

        if not query_text.strip():
            return self._empty_result("Empty query")

        # Determine optimal search strategy
        if search_mode == "auto":
            search_mode = self._determine_search_strategy(query_text)

        print(f"🔍 Hybrid search: '{query_text}' using {search_mode} mode")

        try:
            # Initialize result containers
            semantic_results = []
            fulltext_results = []
            combined_results = []

            # Perform semantic search if enabled
            if search_mode in ["semantic_only", "hybrid"]:
                semantic_results = await self._perform_semantic_search(
                    query_text, db, privacy_levels, include_summaries, date_range
                )

            # Perform full-text search if enabled
            if search_mode in ["fulltext_only", "hybrid"]:
                fulltext_results = await self._perform_fulltext_search(
                    query_text, db, privacy_levels, include_summaries, date_range
                )

            # Combine and rank results
            if search_mode == "hybrid":
                combined_results = self._merge_search_results(semantic_results, fulltext_results, query_text)
            elif search_mode == "semantic_only":
                combined_results = self._format_semantic_results(semantic_results)
            elif search_mode == "fulltext_only":
                combined_results = self._format_fulltext_results(fulltext_results)

            # Apply additional ranking factors
            if combined_results:
                combined_results = await self._apply_ranking_factors(
                    combined_results, db, boost_recent, date_range
                )

                # Sort by final score and limit results
                combined_results.sort(key=lambda x: x.get("final_score", 0), reverse=True)
                combined_results = combined_results[:limit]

            # Prepare response
            return {
                "results": combined_results,
                "search_metadata": {
                    "query": query_text,
                    "search_mode": search_mode,
                    "total_results": len(combined_results),
                    "semantic_results_count": len(semantic_results),
                    "fulltext_results_count": len(fulltext_results),
                    "privacy_levels": privacy_levels or ["public", "organization", "team_only"],
                    "search_weights": self.weights,
                    "processing_time_ms": None  # Will be set by caller
                }
            }

        except Exception as e:
            print(f"❌ Hybrid search failed: {str(e)}")
            return self._empty_result(f"Search error: {str(e)}")

    async def search_with_suggestions(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 20
    ) -> Dict[str, Any]:
        """Search with query suggestions for improved user experience"""

        # Get initial search results
        search_results = await self.search_meetings(
            query_text, db, privacy_levels, limit=limit
        )

        # Generate suggestions if results are limited
        suggestions = []
        if len(search_results["results"]) < limit // 2:
            suggestions = await self._generate_search_suggestions(
                query_text, db, privacy_levels
            )

        # Add suggestions to response
        search_results["suggestions"] = suggestions
        return search_results

    async def find_related_meetings(
        self,
        meeting_id: str,
        db: AsyncSession,
        limit: int = 10,
        similarity_threshold: float = 0.7,
        include_different_privacy: bool = False
    ) -> List[Dict[str, Any]]:
        """Find meetings related to a given meeting"""

        try:
            # Use semantic search to find similar meetings
            similar_meetings = await semantic_search.find_similar_meetings(
                meeting_id, db, limit * 2, similarity_threshold
            )

            if not similar_meetings:
                return []

            # Get additional context for ranking
            enhanced_results = await self._enhance_similarity_results(
                similar_meetings, db, include_different_privacy
            )

            # Sort by enhanced relevance score
            enhanced_results.sort(key=lambda x: x.get("relevance_score", 0), reverse=True)

            return enhanced_results[:limit]

        except Exception as e:
            print(f"❌ Related meetings search failed: {str(e)}")
            return []

    def _determine_search_strategy(self, query_text: str) -> str:
        """Determine the optimal search strategy based on query characteristics"""

        # Analyze query characteristics
        word_count = len(query_text.split())
        has_quotes = '"' in query_text
        has_special_chars = any(char in query_text for char in ['@', '#', ':', '-', '_'])
        is_short = len(query_text.strip()) < 10
        is_conceptual = any(word in query_text.lower() for word in [
            'about', 'regarding', 'discuss', 'meeting', 'decision', 'action', 'follow', 'next'
        ])

        # Decision logic
        if has_quotes or has_special_chars:
            return "fulltext_only"  # Exact phrase or specific term search
        elif is_short and not is_conceptual:
            return "fulltext_only"  # Short specific queries work better with full-text
        elif is_conceptual or word_count > 5:
            return "semantic_only"  # Conceptual or long queries work better with semantic
        else:
            return "hybrid"  # Balanced approach for medium complexity queries

    async def _perform_semantic_search(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str],
        include_summaries: bool,
        date_range: Optional[Tuple[datetime, datetime]]
    ) -> List[Dict[str, Any]]:
        """Perform semantic search with filtering"""

        # Search transcripts
        transcript_results = await semantic_search.search_meetings_by_text(
            query_text, db, privacy_levels, self.max_semantic_results, self.semantic_threshold
        )

        # Search summaries if requested
        summary_results = []
        if include_summaries:
            summary_results = await semantic_search.search_summaries_by_text(
                query_text, db, privacy_levels, self.max_semantic_results, self.semantic_threshold
            )

        # Combine and deduplicate by meeting_id
        all_results = transcript_results + summary_results
        return self._deduplicate_by_meeting_id(all_results)

    async def _perform_fulltext_search(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str],
        include_summaries: bool,
        date_range: Optional[Tuple[datetime, datetime]]
    ) -> List[Dict[str, Any]]:
        """Perform full-text search with filtering"""

        # Search transcripts
        transcript_results = await fulltext_search.search_transcript_content(
            query_text, db, privacy_levels, self.max_fulltext_results
        )

        # Search summaries if requested
        summary_results = []
        if include_summaries:
            summary_results = await fulltext_search.search_summary_content(
                query_text, db, privacy_levels, self.max_fulltext_results
            )

        # Search filenames
        filename_results = await fulltext_search.search_by_filename(
            query_text, db, privacy_levels, 10
        )

        # Combine and deduplicate
        all_results = transcript_results + summary_results + filename_results
        return self._deduplicate_by_meeting_id(all_results)

    def _merge_search_results(
        self,
        semantic_results: List[Dict[str, Any]],
        fulltext_results: List[Dict[str, Any]],
        query_text: str
    ) -> List[Dict[str, Any]]:
        """Merge semantic and full-text search results with score fusion"""

        # Group results by meeting_id
        meeting_results = defaultdict(list)

        # Add semantic results
        for result in semantic_results:
            meeting_id = result["meeting_id"]
            result["source"] = "semantic"
            meeting_results[meeting_id].append(result)

        # Add full-text results
        for result in fulltext_results:
            meeting_id = result["meeting_id"]
            result["source"] = "fulltext"
            meeting_results[meeting_id].append(result)

        # Merge results for each meeting
        merged_results = []
        for meeting_id, results in meeting_results.items():
            merged_result = self._merge_meeting_results(results, query_text)
            if merged_result:
                merged_results.append(merged_result)

        return merged_results

    def _merge_meeting_results(
        self,
        results: List[Dict[str, Any]],
        query_text: str
    ) -> Optional[Dict[str, Any]]:
        """Merge multiple results for the same meeting"""

        if not results:
            return None

        # Use the first result as base
        merged = results[0].copy()

        # Collect scores from different sources
        semantic_score = 0.0
        fulltext_score = 0.0
        sources = set()

        for result in results:
            source = result.get("source", "unknown")
            sources.add(source)

            if source == "semantic":
                semantic_score = max(semantic_score, result.get("similarity_score", 0) / 100)
            elif source == "fulltext":
                fulltext_score = max(fulltext_score, result.get("rank_score", 0))

        # Normalize full-text score (typically 0-1 range)
        if fulltext_score > 1.0:
            fulltext_score = min(fulltext_score / 10.0, 1.0)  # Rough normalization

        # Calculate hybrid score
        hybrid_score = (
            semantic_score * self.weights["semantic_score"] +
            fulltext_score * self.weights["fulltext_score"]
        )

        # Update merged result
        merged.update({
            "semantic_score": semantic_score,
            "fulltext_score": fulltext_score,
            "hybrid_score": hybrid_score,
            "sources": list(sources),
            "result_type": "hybrid"
        })

        return merged

    async def _apply_ranking_factors(
        self,
        results: List[Dict[str, Any]],
        db: AsyncSession,
        boost_recent: bool,
        date_range: Optional[Tuple[datetime, datetime]]
    ) -> List[Dict[str, Any]]:
        """Apply additional ranking factors like recency and quality"""

        if not results:
            return results

        # Calculate recency scores if boosting recent content
        if boost_recent:
            self._calculate_recency_scores(results)

        # Calculate quality scores
        self._calculate_quality_scores(results)

        # Calculate final scores
        for result in results:
            final_score = (
                result.get("hybrid_score", result.get("semantic_score", result.get("rank_score", 0))) *
                (self.weights["semantic_score"] + self.weights["fulltext_score"]) +
                result.get("recency_score", 0) * self.weights["recency_score"] +
                result.get("quality_score", 0) * self.weights["quality_score"]
            )

            result["final_score"] = final_score

        return results

    def _calculate_recency_scores(self, results: List[Dict[str, Any]]) -> None:
        """Calculate recency scores based on meeting age"""

        if not results:
            return

        now = datetime.utcnow()
        max_age_days = 365  # 1 year

        for result in results:
            try:
                created_at_str = result.get("created_at", "")
                if created_at_str:
                    # Parse ISO format datetime
                    created_at = datetime.fromisoformat(created_at_str.replace("Z", "+00:00"))
                    age_days = (now - created_at).days

                    # Calculate recency score (newer = higher score)
                    if age_days <= 7:
                        recency_score = 1.0  # Last week
                    elif age_days <= 30:
                        recency_score = 0.8  # Last month
                    elif age_days <= 90:
                        recency_score = 0.6  # Last quarter
                    elif age_days <= 180:
                        recency_score = 0.4  # Last 6 months
                    else:
                        recency_score = max(0.1, 1.0 - (age_days / max_age_days))

                    result["recency_score"] = recency_score
                else:
                    result["recency_score"] = 0.5  # Default for unknown date
            except Exception:
                result["recency_score"] = 0.5  # Default for parsing errors

    def _calculate_quality_scores(self, results: List[Dict[str, Any]]) -> None:
        """Calculate quality scores based on transcript/summary metrics"""

        for result in results:
            quality_score = 0.5  # Default

            # Factor in transcript confidence if available
            confidence_score = result.get("confidence_score", 0)
            if confidence_score > 0:
                quality_score += (confidence_score / 100) * 0.3

            # Factor in summary quality if available
            summary_quality = result.get("quality_score", 0)
            if summary_quality > 0:
                quality_score += (summary_quality / 100) * 0.3

            # Factor in structured content presence
            if result.get("key_points_count", 0) > 0:
                quality_score += 0.1
            if result.get("action_items_count", 0) > 0:
                quality_score += 0.1
            if result.get("decisions_count", 0) > 0:
                quality_score += 0.1

            # Factor in speaker diversity
            speaker_count = result.get("speaker_count", 0)
            if speaker_count >= 2:
                quality_score += 0.1

            result["quality_score"] = min(1.0, quality_score)

    def _deduplicate_by_meeting_id(self, results: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Remove duplicate results based on meeting_id, keeping the best score"""

        if not results:
            return []

        # Group by meeting_id and keep the best scoring result
        best_results = {}
        for result in results:
            meeting_id = result["meeting_id"]
            score = result.get("similarity_score", result.get("rank_score", 0))

            if meeting_id not in best_results or score > best_results[meeting_id].get("similarity_score", best_results[meeting_id].get("rank_score", 0)):
                best_results[meeting_id] = result

        return list(best_results.values())

    def _format_semantic_results(self, results: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Format semantic-only results for consistency"""

        for result in results:
            result.update({
                "semantic_score": result.get("similarity_score", 0) / 100,
                "fulltext_score": 0.0,
                "hybrid_score": result.get("similarity_score", 0) / 100,
                "sources": ["semantic"],
                "result_type": "semantic_only"
            })

        return results

    def _format_fulltext_results(self, results: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Format full-text-only results for consistency"""

        for result in results:
            # Normalize rank score
            rank_score = result.get("rank_score", 0)
            if rank_score > 1.0:
                rank_score = min(rank_score / 10.0, 1.0)

            result.update({
                "semantic_score": 0.0,
                "fulltext_score": rank_score,
                "hybrid_score": rank_score,
                "sources": ["fulltext"],
                "result_type": "fulltext_only"
            })

        return results

    async def _generate_search_suggestions(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str]
    ) -> List[str]:
        """Generate search suggestions when results are limited"""

        suggestions = []

        try:
            # Get suggestions from semantic search
            semantic_suggestions = await semantic_search.get_search_suggestions(
                query_text, db, privacy_levels, 3
            )
            suggestions.extend(semantic_suggestions)

            # Add query expansion suggestions
            expanded_queries = self._expand_query(query_text)
            suggestions.extend(expanded_queries[:2])

            # Remove duplicates while preserving order
            unique_suggestions = []
            seen = set()
            for suggestion in suggestions:
                if suggestion.lower() not in seen:
                    unique_suggestions.append(suggestion)
                    seen.add(suggestion.lower())

            return unique_suggestions[:5]

        except Exception as e:
            print(f"⚠️ Search suggestions failed: {str(e)}")
            return []

    def _expand_query(self, query_text: str) -> List[str]:
        """Generate query expansion suggestions"""

        expansions = []
        words = query_text.lower().split()

        # Add related business terms
        business_terms = {
            'meeting': ['discussion', 'call', 'session'],
            'decision': ['choice', 'resolution', 'conclusion'],
            'action': ['task', 'todo', 'assignment'],
            'project': ['initiative', 'program', 'work'],
            'team': ['group', 'people', 'members']
        }

        for word in words:
            if word in business_terms:
                for synonym in business_terms[word]:
                    expanded = query_text.replace(word, synonym)
                    if expanded != query_text:
                        expansions.append(expanded)

        return expansions

    async def _enhance_similarity_results(
        self,
        similar_meetings: List[Dict[str, Any]],
        db: AsyncSession,
        include_different_privacy: bool
    ) -> List[Dict[str, Any]]:
        """Enhance similarity results with additional ranking factors"""

        enhanced = []
        for meeting in similar_meetings:
            # Base relevance from similarity score
            relevance_score = meeting.get("similarity_score", 0) / 100

            # Add time-based relevance (more recent = slightly more relevant)
            try:
                created_at = datetime.fromisoformat(meeting["created_at"].replace("Z", "+00:00"))
                age_days = (datetime.utcnow() - created_at).days
                time_factor = max(0.8, 1.0 - (age_days / 365))  # Decay over 1 year
                relevance_score *= time_factor
            except Exception:
                pass  # Skip time adjustment on parse error

            meeting["relevance_score"] = relevance_score
            enhanced.append(meeting)

        return enhanced

    def _empty_result(self, reason: str) -> Dict[str, Any]:
        """Return empty search result with metadata"""

        return {
            "results": [],
            "search_metadata": {
                "query": "",
                "search_mode": "none",
                "total_results": 0,
                "semantic_results_count": 0,
                "fulltext_results_count": 0,
                "privacy_levels": [],
                "error": reason,
                "search_weights": self.weights,
                "processing_time_ms": 0
            }
        }

    def update_search_weights(self, new_weights: Dict[str, float]) -> None:
        """Update search component weights for tuning"""

        # Validate weights sum to approximately 1.0
        total_weight = sum(new_weights.values())
        if abs(total_weight - 1.0) > 0.1:
            print(f"⚠️ Search weights sum to {total_weight}, should be close to 1.0")

        self.weights.update(new_weights)
        print(f"✅ Updated search weights: {self.weights}")

    def get_search_configuration(self) -> Dict[str, Any]:
        """Get current search configuration"""

        return {
            "weights": self.weights.copy(),
            "thresholds": {
                "semantic": self.semantic_threshold,
                "fulltext": self.fulltext_threshold,
                "hybrid": self.hybrid_threshold
            },
            "limits": {
                "semantic_results": self.max_semantic_results,
                "fulltext_results": self.max_fulltext_results,
                "final_results": self.max_final_results
            }
        }


# Global hybrid search service
hybrid_search = HybridSearchService()