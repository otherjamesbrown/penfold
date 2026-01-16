"""
Search API Routes

Provides REST API endpoints for meeting search with privacy controls,
hybrid search algorithms, and performance optimization.
"""
from fastapi import APIRouter, HTTPException, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel
from typing import Dict, Any, Optional, List
from datetime import datetime, timedelta
import time
import uuid

from app.database import get_db
from app.auth import get_current_user
from app.search.hybrid import hybrid_search
from app.search.semantic import semantic_search
from app.search.fulltext import fulltext_search
from app.search.privacy import privacy_filter, UserRole, PrivacyLevel
from app.search.discovery import discovery_service
from app.search.optimization import search_optimizer

router = APIRouter()


class SearchRequest(BaseModel):
    """Request model for meeting search"""
    query: str
    search_mode: str = "auto"  # auto, semantic_only, fulltext_only, hybrid
    privacy_levels: Optional[List[str]] = None
    limit: int = 20
    include_summaries: bool = True
    boost_recent: bool = True


class SearchResponse(BaseModel):
    """Response model for search results"""
    results: List[Dict[str, Any]]
    search_metadata: Dict[str, Any]
    privacy_info: Dict[str, Any]


class SimilarMeetingsRequest(BaseModel):
    """Request model for finding similar meetings"""
    meeting_id: str
    limit: int = 10
    similarity_threshold: float = 0.7
    include_different_privacy: bool = False


def _get_user_role(current_user: Dict[str, Any]) -> UserRole:
    """Convert string role from auth module to UserRole enum."""
    role_str = current_user.get("role", "guest")
    role_mapping = {
        "admin": UserRole.ADMIN,
        "super_admin": UserRole.SUPER_ADMIN,
        "member": UserRole.MEMBER,
        "guest": UserRole.GUEST,
    }
    return role_mapping.get(role_str, UserRole.GUEST)


def _get_user_id(current_user: Dict[str, Any]) -> str:
    """Get user ID from auth dict, with fallback."""
    return current_user.get("user_id", "authenticated_user")


@router.get("/search/meetings")
async def search_meetings(
    query: str = Query(..., description="Search query text"),
    search_mode: str = Query("auto", description="Search mode: auto, semantic_only, fulltext_only, hybrid"),
    privacy_levels: Optional[List[str]] = Query(None, description="Privacy levels to include"),
    limit: int = Query(20, description="Maximum number of results"),
    include_summaries: bool = Query(True, description="Include summary search"),
    boost_recent: bool = Query(True, description="Boost recent meetings in ranking"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Search meetings with hybrid algorithm and privacy filtering"""

    start_time = time.time()

    try:
        # Validate search mode
        valid_modes = ["auto", "semantic_only", "fulltext_only", "hybrid"]
        if search_mode not in valid_modes:
            raise HTTPException(status_code=400, detail=f"Invalid search mode. Use: {valid_modes}")

        # Get user's allowed privacy levels
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        # Filter requested privacy levels against user permissions
        if privacy_levels:
            allowed_levels = [level for level in privacy_levels if level in user_privacy_levels]
            if not allowed_levels:
                return {
                    "results": [],
                    "search_metadata": {
                        "query": query,
                        "total_results": 0,
                        "error": "No accessible privacy levels requested"
                    },
                    "privacy_info": {
                        "user_privacy_levels": user_privacy_levels,
                        "requested_levels": privacy_levels,
                        "accessible_levels": []
                    }
                }
        else:
            allowed_levels = user_privacy_levels

        # Perform search
        search_results = await hybrid_search.search_meetings(
            query_text=query,
            db=db,
            privacy_levels=allowed_levels,
            search_mode=search_mode,
            limit=limit,
            include_summaries=include_summaries,
            boost_recent=boost_recent
        )

        # Apply privacy filtering to results
        filtered_results = await privacy_filter.filter_search_results(
            search_results["results"],
            user_id,
            user_role,
            current_user.get("team_ids"),
            db
        )

        # Sanitize results based on user role
        sanitized_results = privacy_filter.sanitize_search_results(
            filtered_results,
            user_role
        )

        # Calculate processing time
        processing_time = (time.time() - start_time) * 1000
        search_results["search_metadata"]["processing_time_ms"] = round(processing_time, 2)

        # Audit the search
        await privacy_filter.audit_search_access(
            user_id,
            query,
            len(sanitized_results),
            allowed_levels,
            db
        )

        return {
            "results": sanitized_results,
            "search_metadata": search_results["search_metadata"],
            "privacy_info": {
                "user_privacy_levels": user_privacy_levels,
                "search_privacy_levels": allowed_levels,
                "results_filtered": len(filtered_results) != len(search_results["results"]),
                "user_role": current_user.get("role", "guest")
            }
        }

    except Exception as e:
        print(f"❌ Search API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/search/suggest")
async def get_search_suggestions(
    partial_query: str = Query(..., description="Partial search query"),
    limit: int = Query(5, description="Maximum number of suggestions"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Get search suggestions based on partial query"""

    try:
        # Get user's allowed privacy levels
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        # Get suggestions from semantic search
        suggestions = await semantic_search.get_search_suggestions(
            partial_query, db, user_privacy_levels, limit
        )

        return {
            "suggestions": suggestions,
            "partial_query": partial_query,
            "privacy_levels_used": user_privacy_levels
        }

    except Exception as e:
        print(f"❌ Search suggestions API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/meetings/{meeting_id}/related")
async def find_related_meetings(
    meeting_id: str,
    limit: int = Query(10, description="Maximum number of related meetings"),
    similarity_threshold: float = Query(0.7, description="Minimum similarity threshold"),
    include_different_privacy: bool = Query(False, description="Include different privacy levels"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Find meetings related to a specific meeting"""

    try:
        # Validate meeting_id format
        try:
            uuid.UUID(meeting_id)
        except ValueError:
            raise HTTPException(status_code=400, detail="Invalid meeting ID format")

        # Check if user can access the source meeting
        # This would need to be implemented with a proper meeting access check

        # Find related meetings
        related_meetings = await hybrid_search.find_related_meetings(
            meeting_id=meeting_id,
            db=db,
            limit=limit,
            similarity_threshold=similarity_threshold,
            include_different_privacy=include_different_privacy
        )

        # Apply privacy filtering
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        # Filter results by privacy levels
        accessible_meetings = []
        for meeting in related_meetings:
            privacy_level = meeting.get("privacy_level", "confidential")
            if privacy_level in user_privacy_levels:
                accessible_meetings.append(meeting)

        # Sanitize results
        sanitized_meetings = privacy_filter.sanitize_search_results(
            accessible_meetings,
            user_role
        )

        return {
            "meeting_id": meeting_id,
            "related_meetings": sanitized_meetings,
            "metadata": {
                "total_found": len(related_meetings),
                "accessible_count": len(sanitized_meetings),
                "similarity_threshold": similarity_threshold,
                "user_privacy_levels": user_privacy_levels
            }
        }

    except HTTPException:
        raise
    except Exception as e:
        print(f"❌ Related meetings API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/search/statistics")
async def get_search_statistics(
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Get search system statistics"""

    try:
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)

        # Check if user has admin access for detailed statistics
        if user_role not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
            # Return limited statistics for non-admin users
            return {
                "searchable_content": "Available",
                "search_types": ["semantic", "fulltext", "hybrid"],
                "privacy_levels": privacy_filter.get_allowed_privacy_levels(
                    user_id,
                    user_role,
                    current_user.get("team_ids")
                ),
                "user_role": current_user.get("role", "guest")
            }

        # Get detailed statistics for admin users
        semantic_stats = await semantic_search.get_search_statistics(db)
        search_config = hybrid_search.get_search_configuration()

        return {
            "semantic_search": semantic_stats,
            "search_configuration": search_config,
            "privacy_levels": {
                "available": [level.value for level in PrivacyLevel],
                "descriptions": {
                    level.value: privacy_filter.get_privacy_level_description(level.value)
                    for level in PrivacyLevel
                }
            },
            "user_access": {
                "role": current_user.get("role", "guest"),
                "accessible_privacy_levels": privacy_filter.get_allowed_privacy_levels(
                    user_id,
                    user_role,
                    current_user.get("team_ids")
                ),
                "team_count": len(current_user.get("team_ids", []))
            }
        }

    except Exception as e:
        print(f"❌ Search statistics API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/search/advanced")
async def advanced_search(
    request: SearchRequest,
    start_date: Optional[datetime] = Query(None, description="Start date for date range filter"),
    end_date: Optional[datetime] = Query(None, description="End date for date range filter"),
    participant_name: Optional[str] = Query(None, description="Filter by participant name"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Advanced search with multiple filters"""

    start_time = time.time()

    try:
        # Get user's allowed privacy levels
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        # Filter requested privacy levels
        if request.privacy_levels:
            allowed_levels = [level for level in request.privacy_levels if level in user_privacy_levels]
        else:
            allowed_levels = user_privacy_levels

        results = []

        # Perform date range search if specified
        if start_date or end_date:
            # Default to last 30 days if only one date specified
            if not start_date:
                start_date = (end_date or datetime.utcnow()) - timedelta(days=30)
            if not end_date:
                end_date = datetime.utcnow()

            date_results = await fulltext_search.search_by_date_range(
                start_date, end_date, db, allowed_levels, request.query, request.limit
            )
            results.extend(date_results)

        # Perform participant search if specified
        if participant_name:
            participant_results = await fulltext_search.search_by_participant(
                participant_name, db, allowed_levels, request.limit
            )
            results.extend(participant_results)

        # Perform main content search
        if request.query.strip():
            main_results = await hybrid_search.search_meetings(
                query_text=request.query,
                db=db,
                privacy_levels=allowed_levels,
                search_mode=request.search_mode,
                limit=request.limit,
                include_summaries=request.include_summaries,
                boost_recent=request.boost_recent
            )
            results.extend(main_results["results"])

        # Deduplicate by meeting_id
        seen_meetings = set()
        unique_results = []
        for result in results:
            meeting_id = result["meeting_id"]
            if meeting_id not in seen_meetings:
                seen_meetings.add(meeting_id)
                unique_results.append(result)

        # Apply privacy filtering and sanitization
        filtered_results = await privacy_filter.filter_search_results(
            unique_results, user_id, user_role,
            current_user.get("team_ids"), db
        )

        sanitized_results = privacy_filter.sanitize_search_results(
            filtered_results, user_role
        )

        # Sort by relevance (if available) and limit
        sanitized_results.sort(key=lambda x: x.get("final_score", x.get("rank_score", 0)), reverse=True)
        sanitized_results = sanitized_results[:request.limit]

        processing_time = (time.time() - start_time) * 1000

        return {
            "results": sanitized_results,
            "search_metadata": {
                "query": request.query,
                "search_mode": request.search_mode,
                "total_results": len(sanitized_results),
                "processing_time_ms": round(processing_time, 2),
                "filters_applied": {
                    "date_range": start_date and end_date,
                    "participant_filter": bool(participant_name),
                    "privacy_filtering": True
                }
            },
            "privacy_info": {
                "user_privacy_levels": user_privacy_levels,
                "search_privacy_levels": allowed_levels,
                "user_role": current_user.get("role", "guest")
            }
        }

    except Exception as e:
        print(f"❌ Advanced search API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/search/initialize")
async def initialize_search_indexes(
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Initialize or update search indexes (admin only)"""

    try:
        # Check admin permissions
        user_role = _get_user_role(current_user)
        if user_role not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
            raise HTTPException(status_code=403, detail="Admin access required")

        # Initialize semantic search indexes
        await semantic_search.initialize_indexes(db)

        return {
            "message": "Search indexes initialized successfully",
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "initialized_by": _get_user_id(current_user)
        }

    except HTTPException:
        raise
    except Exception as e:
        print(f"❌ Index initialization error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.put("/search/configuration")
async def update_search_configuration(
    weights: Dict[str, float],
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Update search algorithm weights (admin only)"""

    try:
        # Check admin permissions
        user_role = _get_user_role(current_user)
        if user_role not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
            raise HTTPException(status_code=403, detail="Admin access required")

        # Validate weights
        required_weights = ["semantic_score", "fulltext_score", "recency_score", "quality_score"]
        if not all(weight in weights for weight in required_weights):
            raise HTTPException(
                status_code=400,
                detail=f"Missing required weights. Need: {required_weights}"
            )

        # Update search weights
        hybrid_search.update_search_weights(weights)

        return {
            "message": "Search configuration updated successfully",
            "new_weights": weights,
            "updated_by": _get_user_id(current_user),
            "timestamp": datetime.utcnow().isoformat() + "Z"
        }

    except HTTPException:
        raise
    except Exception as e:
        print(f"❌ Configuration update error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/search/health")
async def search_health_check(
    db: AsyncSession = Depends(get_db)
):
    """Health check for search services"""

    try:
        # Test semantic search
        semantic_available = semantic_search.semantic_search is not None

        # Test database connection
        db_available = True
        try:
            await db.execute("SELECT 1")
        except Exception:
            db_available = False

        # Test hybrid search
        hybrid_available = hybrid_search is not None

        # Overall health
        overall_healthy = semantic_available and db_available and hybrid_available

        return {
            "status": "healthy" if overall_healthy else "degraded",
            "services": {
                "semantic_search": "available" if semantic_available else "unavailable",
                "database": "available" if db_available else "unavailable",
                "hybrid_search": "available" if hybrid_available else "unavailable"
            },
            "timestamp": datetime.utcnow().isoformat() + "Z"
        }

    except Exception as e:
        return {
            "status": "unhealthy",
            "error": str(e),
            "timestamp": datetime.utcnow().isoformat() + "Z"
        }


@router.get("/discover/trending")
async def get_trending_topics(
    days_back: int = Query(7, description="Days to look back for trending analysis"),
    limit: int = Query(10, description="Maximum number of trending topics"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Get trending topics from recent meetings"""

    try:
        # Get user's allowed privacy levels
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        trending_topics = await discovery_service.get_trending_topics(
            db, user_privacy_levels, days_back, limit
        )

        return {
            "trending_topics": trending_topics,
            "analysis_period_days": days_back,
            "user_privacy_levels": user_privacy_levels
        }

    except Exception as e:
        print(f"❌ Trending topics API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/discover/recommendations")
async def get_meeting_recommendations(
    limit: int = Query(20, description="Maximum number of recommendations"),
    recommendation_types: Optional[List[str]] = Query(None, description="Types of recommendations to include"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Get personalized meeting recommendations"""

    try:
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        recommendations = await discovery_service.get_meeting_recommendations(
            user_id,
            db,
            user_privacy_levels,
            limit,
            recommendation_types
        )

        return {
            "recommendations": recommendations,
            "user_id": user_id,
            "recommendation_types": recommendation_types or ["similar_content", "trending_topics", "recent_activity"],
            "privacy_levels": user_privacy_levels
        }

    except Exception as e:
        print(f"❌ Recommendations API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/discover/topic/{topic_name}")
async def discover_meetings_by_topic(
    topic_name: str,
    limit: int = Query(20, description="Maximum number of meetings"),
    min_relevance: float = Query(0.6, description="Minimum relevance score"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Discover meetings related to a specific topic"""

    try:
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        related_meetings = await discovery_service.discover_related_meetings_by_topic(
            topic_name, db, user_privacy_levels, limit, min_relevance
        )

        # Apply privacy filtering
        sanitized_meetings = privacy_filter.sanitize_search_results(
            related_meetings,
            user_role
        )

        return {
            "topic": topic_name,
            "related_meetings": sanitized_meetings,
            "min_relevance": min_relevance,
            "total_found": len(sanitized_meetings)
        }

    except Exception as e:
        print(f"❌ Topic discovery API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/discover/participants")
async def discover_meetings_by_participants(
    participants: List[str] = Query(..., description="Participant names to search for"),
    require_all: bool = Query(False, description="Require all participants to be present"),
    limit: int = Query(20, description="Maximum number of meetings"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Find meetings that include specific participants"""

    try:
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        meetings = await discovery_service.find_meetings_by_participants(
            participants, db, user_privacy_levels, require_all, limit
        )

        # Apply privacy filtering
        sanitized_meetings = privacy_filter.sanitize_search_results(
            meetings,
            user_role
        )

        return {
            "search_participants": participants,
            "require_all_participants": require_all,
            "meetings": sanitized_meetings,
            "total_found": len(sanitized_meetings)
        }

    except Exception as e:
        print(f"❌ Participant discovery API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/discover/insights")
async def get_meeting_insights_summary(
    days_back: int = Query(7, description="Days to analyze for insights"),
    insight_types: Optional[List[str]] = Query(None, description="Types of insights to include"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Get aggregated insights from recent meetings"""

    try:
        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        insights_summary = await discovery_service.get_meeting_insights_summary(
            db, user_privacy_levels, days_back, insight_types
        )

        return insights_summary

    except Exception as e:
        print(f"❌ Insights summary API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/discover/timeline")
async def get_meeting_activity_timeline(
    days_back: int = Query(30, description="Days to include in timeline"),
    granularity: str = Query("daily", description="Timeline granularity: daily or weekly"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Get timeline of meeting activity for visualization"""

    try:
        if granularity not in ["daily", "weekly"]:
            raise HTTPException(status_code=400, detail="Granularity must be 'daily' or 'weekly'")

        user_role = _get_user_role(current_user)
        user_id = _get_user_id(current_user)
        user_privacy_levels = privacy_filter.get_allowed_privacy_levels(
            user_id,
            user_role,
            current_user.get("team_ids")
        )

        timeline = await discovery_service.get_meeting_activity_timeline(
            db, user_privacy_levels, days_back, granularity
        )

        return timeline

    except HTTPException:
        raise
    except Exception as e:
        print(f"❌ Timeline API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/search/performance")
async def get_search_performance_metrics(
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Get search performance metrics (admin only)"""

    try:
        # Check admin permissions for detailed metrics
        user_role = _get_user_role(current_user)
        if user_role not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
            return {
                "message": "Limited performance data available",
                "cache_enabled": True,
                "optimization_enabled": True
            }

        # Get detailed performance report
        performance_report = search_optimizer.get_performance_report()

        return performance_report

    except Exception as e:
        print(f"❌ Performance metrics API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/search/optimize")
async def optimize_search_performance(
    warm_cache: bool = Query(False, description="Warm cache with common queries"),
    analyze_database: bool = Query(False, description="Analyze database performance"),
    db: AsyncSession = Depends(get_db),
    current_user: Dict[str, Any] = Depends(get_current_user)
):
    """Run search performance optimizations (admin only)"""

    try:
        # Check admin permissions
        user_role = _get_user_role(current_user)
        if user_role not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
            raise HTTPException(status_code=403, detail="Admin access required")

        optimization_results = {
            "optimizations_run": [],
            "recommendations": []
        }

        # Warm cache if requested
        if warm_cache:
            user_privacy_levels = ["public", "organization", "team_only"]  # Default for cache warming
            cache_result = await search_optimizer.warm_cache_with_common_queries(
                db, user_privacy_levels
            )
            optimization_results["cache_warming"] = cache_result
            optimization_results["optimizations_run"].append("cache_warming")

        # Analyze database if requested
        if analyze_database:
            db_analysis = await search_optimizer.optimize_database_performance(db)
            optimization_results["database_analysis"] = db_analysis
            optimization_results["optimizations_run"].append("database_analysis")

        # Always provide general recommendations
        optimization_results["recommendations"] = [
            "Monitor cache hit rates and adjust cache size if needed",
            "Use specific search terms for better performance",
            "Consider using semantic search for conceptual queries",
            "Use full-text search for exact phrase matching"
        ]

        return optimization_results

    except HTTPException:
        raise
    except Exception as e:
        print(f"❌ Search optimization API error: {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))