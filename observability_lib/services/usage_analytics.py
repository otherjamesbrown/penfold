"""Usage Analytics Service for tracking user engagement and feature utilization.

This module implements the UsageAnalyticsService that monitors how users interact
with different features of the Penfold system, tracks engagement patterns, and
provides insights for feature optimization and adoption strategies.

Features:
- Real-time usage tracking and analytics
- Daily review engagement monitoring
- Search feature utilization analysis
- Project query frequency tracking
- Workflow utilization metrics
- Feature adoption rate calculation
- Session duration and quality analysis
- Query complexity scoring
- User behavior pattern analysis
- Feature performance correlation

Key Metrics Tracked:
- Daily review engagement: Time spent in daily reviews, completion rates
- Search feature usage: Search frequency, query types, result interactions
- Project query frequency: How often users query specific projects
- Workflow utilization: Which workflows are most/least used
- Feature adoption rate: How quickly new features gain adoption
- Session duration: Length and quality of user sessions
- Query complexity score: Complexity analysis of user queries

Example:
    from observability_lib.services.usage_analytics import UsageAnalyticsService
    from observability_lib.config import settings

    analytics = UsageAnalyticsService(settings)
    await analytics.start()

    # Track feature usage
    await analytics.track_feature_usage(
        feature_name="daily_review",
        duration_seconds=180,
        success_rate=0.95,
        context={"review_type": "morning", "items_reviewed": 12}
    )

    # Get usage insights
    insights = await analytics.get_usage_insights("search_feature", days_back=7)

    # Track search interaction
    await analytics.track_search_usage(
        query="meeting notes project alpha",
        results_count=15,
        clicked_results=3,
        session_id="sess_123"
    )

    await analytics.stop()
"""

import asyncio
import logging
import time
import uuid
from collections import defaultdict, deque
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional, Tuple, Union, Set
from enum import Enum
import statistics
import math
import hashlib

from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, or_, text, desc, asc
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.business_metrics import (
    UsageMetric, UsageMetricType
)
from observability_lib.models.agent_metrics import (
    AgentMetric, WorkflowEvent, AgentLog
)
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


# ============================================================================
# Usage Analytics Data Types and Constants
# ============================================================================

class EngagementLevel(str, Enum):
    """User engagement levels"""
    HIGH = "high"           # Daily active, multiple features used
    MEDIUM = "medium"       # Regular use, some features used
    LOW = "low"            # Occasional use, limited features
    INACTIVE = "inactive"   # No recent activity


class FeatureCategory(str, Enum):
    """Categories of features for analysis"""
    CORE = "core"                    # Essential features (search, review)
    PRODUCTIVITY = "productivity"    # Productivity tools (queries, workflows)
    COLLABORATION = "collaboration"  # Sharing and collaboration features
    AUTOMATION = "automation"        # Automated workflows and processes
    ANALYTICS = "analytics"          # Analytics and reporting features


@dataclass
class UsageSession:
    """User session with usage metrics"""
    session_id: str
    user_identifier: str
    start_time: datetime
    end_time: Optional[datetime]
    features_used: Set[str]
    total_interactions: int
    success_rate: float
    context: Dict[str, Any]


@dataclass
class FeatureUsageInsight:
    """Insights about feature usage patterns"""
    feature_name: str
    feature_category: FeatureCategory
    total_usage_count: int
    unique_users: int
    avg_session_duration: float
    success_rate: float
    adoption_rate: float
    engagement_level: EngagementLevel
    trend_direction: str
    peak_usage_hours: List[int]
    common_contexts: Dict[str, int]
    last_updated: datetime


@dataclass
class UserEngagementProfile:
    """User engagement profile and patterns"""
    user_identifier: str
    engagement_level: EngagementLevel
    total_sessions: int
    avg_session_duration: float
    features_used: Set[str]
    favorite_features: List[str]
    last_active: datetime
    activity_pattern: Dict[str, int]  # hour -> activity_count
    success_rate: float
    growth_trend: str


@dataclass
class UsageAnalytics:
    """Comprehensive usage analytics summary"""
    period_start: datetime
    period_end: datetime
    total_sessions: int
    unique_users: int
    avg_session_duration: float
    most_popular_features: List[Tuple[str, int]]
    feature_adoption_rates: Dict[str, float]
    engagement_distribution: Dict[EngagementLevel, int]
    peak_usage_times: List[int]
    success_rate_by_feature: Dict[str, float]
    query_complexity_trends: Dict[str, float]


# Feature categorization mapping
FEATURE_CATEGORIES = {
    "daily_review": FeatureCategory.CORE,
    "search_feature": FeatureCategory.CORE,
    "project_query": FeatureCategory.PRODUCTIVITY,
    "workflow_execution": FeatureCategory.AUTOMATION,
    "relationship_analysis": FeatureCategory.ANALYTICS,
    "meeting_analysis": FeatureCategory.PRODUCTIVITY,
    "email_processing": FeatureCategory.AUTOMATION,
    "context_reconstruction": FeatureCategory.CORE,
    "collaboration_tools": FeatureCategory.COLLABORATION,
    "dashboard_analytics": FeatureCategory.ANALYTICS
}


# ============================================================================
# Usage Analytics Service
# ============================================================================

class UsageAnalyticsService:
    """Service for tracking and analyzing user engagement and feature usage.

    Monitors user interactions with Penfold features to understand adoption
    patterns, usage trends, and opportunities for improvement.
    """

    def __init__(self, settings: ObservabilitySettings):
        """Initialize the usage analytics service.

        Args:
            settings: Observability configuration settings
        """
        self.settings = settings
        self.engine = None
        self.session_factory = None
        self.is_running = False
        self.tenant_id = settings.DEFAULT_TENANT_ID

        # Session tracking
        self.active_sessions: Dict[str, UsageSession] = {}
        self.session_timeout_minutes = 30

        # Analytics caches
        self.feature_insights_cache: Dict[str, FeatureUsageInsight] = {}
        self.user_profiles_cache: Dict[str, UserEngagementProfile] = {}
        self.cache_ttl_seconds = 300  # 5 minutes
        self.last_cache_update: Dict[str, datetime] = {}

        # Background tasks
        self.analytics_task: Optional[asyncio.Task] = None
        self.cleanup_task: Optional[asyncio.Task] = None
        self.analytics_interval = 300  # 5 minutes
        self.cleanup_interval = 600    # 10 minutes

        # Privacy settings
        self.hash_user_identifiers = True
        self.retention_days = 90

    async def start(self) -> None:
        """Start the usage analytics service."""
        if self.is_running:
            logger.warning("UsageAnalyticsService is already running")
            return

        logger.info("Starting UsageAnalyticsService")

        try:
            # Initialize database connection
            self.engine = create_async_engine(
                self.settings.get_database_url(),
                pool_size=10,
                max_overflow=20,
                pool_pre_ping=True,
                echo=self.settings.DEBUG_SQL
            )

            self.session_factory = sessionmaker(
                bind=self.engine,
                class_=AsyncSession,
                expire_on_commit=False
            )

            # Test database connection
            async with self.session_factory() as session:
                await session.execute(text("SELECT 1"))

            self.is_running = True

            # Start background tasks
            self.analytics_task = asyncio.create_task(self._analytics_loop())
            self.cleanup_task = asyncio.create_task(self._cleanup_loop())

            logger.info("UsageAnalyticsService started successfully")

        except Exception as e:
            logger.error(f"Failed to start UsageAnalyticsService: {e}")
            await self.stop()
            raise

    async def stop(self) -> None:
        """Stop the usage analytics service."""
        if not self.is_running:
            return

        logger.info("Stopping UsageAnalyticsService")
        self.is_running = False

        # Cancel background tasks
        for task in [self.analytics_task, self.cleanup_task]:
            if task:
                task.cancel()
                try:
                    await task
                except asyncio.CancelledError:
                    pass

        # Close active sessions
        for session_id in list(self.active_sessions.keys()):
            await self.end_session(session_id)

        # Close database connection
        if self.engine:
            await self.engine.dispose()

        # Clear caches
        self.feature_insights_cache.clear()
        self.user_profiles_cache.clear()
        self.last_cache_update.clear()

        logger.info("UsageAnalyticsService stopped")

    async def track_feature_usage(
        self,
        feature_name: str,
        user_identifier: Optional[str] = None,
        duration_seconds: Optional[float] = None,
        success_rate: Optional[float] = None,
        usage_count: int = 1,
        context: Optional[Dict[str, Any]] = None,
        session_id: Optional[str] = None
    ) -> str:
        """Track usage of a specific feature.

        Args:
            feature_name: Name of the feature being used
            user_identifier: Identifier for the user (will be hashed)
            duration_seconds: Duration of feature usage
            success_rate: Success rate for this usage (0.0-1.0)
            usage_count: Number of times feature was used
            context: Additional context about usage
            session_id: Session identifier for grouping

        Returns:
            ID of the created usage metric record
        """
        if not self.is_running:
            raise RuntimeError("UsageAnalyticsService is not running")

        # Hash user identifier for privacy
        hashed_user = self._hash_user_identifier(user_identifier) if user_identifier else None

        # Generate session ID if not provided
        if session_id is None and hashed_user:
            session_id = f"sess_{hashed_user}_{int(time.time())}"

        context = context or {}
        timestamp = datetime.now(timezone.utc)

        try:
            async with self.session_factory() as session:
                # Create usage metric record
                usage_record = UsageMetric(
                    tenant_id=self.tenant_id,
                    timestamp=timestamp,
                    metric_type=self._get_metric_type_for_feature(feature_name),
                    user_identifier=hashed_user,
                    feature_name=feature_name,
                    usage_count=usage_count,
                    duration_seconds=duration_seconds,
                    success_rate=success_rate,
                    usage_context=context,
                    session_id=uuid.UUID(session_id.replace("sess_", "").replace("_", "0")[:32].ljust(32, "0")) if session_id else None
                )

                session.add(usage_record)
                await session.commit()
                await session.refresh(usage_record)

                logger.debug(f"Tracked usage: {feature_name} by {hashed_user or 'anonymous'}")

                # Update active session if applicable
                if session_id and session_id in self.active_sessions:
                    self.active_sessions[session_id].features_used.add(feature_name)
                    self.active_sessions[session_id].total_interactions += usage_count

                # Invalidate relevant caches
                self._invalidate_feature_cache(feature_name)

                return str(usage_record.id)

        except SQLAlchemyError as e:
            logger.error(f"Database error tracking feature usage: {e}")
            raise
        except Exception as e:
            logger.error(f"Error tracking feature usage: {e}")
            raise

    async def start_session(
        self,
        user_identifier: str,
        context: Optional[Dict[str, Any]] = None
    ) -> str:
        """Start a new user session for tracking.

        Args:
            user_identifier: Identifier for the user
            context: Initial session context

        Returns:
            Session ID
        """
        hashed_user = self._hash_user_identifier(user_identifier)
        session_id = f"sess_{hashed_user}_{int(time.time())}"

        session = UsageSession(
            session_id=session_id,
            user_identifier=hashed_user,
            start_time=datetime.now(timezone.utc),
            end_time=None,
            features_used=set(),
            total_interactions=0,
            success_rate=0.0,
            context=context or {}
        )

        self.active_sessions[session_id] = session
        logger.debug(f"Started session {session_id} for user {hashed_user}")

        return session_id

    async def end_session(
        self,
        session_id: str,
        context: Optional[Dict[str, Any]] = None
    ) -> Optional[UsageSession]:
        """End a user session and record session metrics.

        Args:
            session_id: ID of session to end
            context: Additional context for session end

        Returns:
            Session data if session existed
        """
        if session_id not in self.active_sessions:
            return None

        session = self.active_sessions[session_id]
        session.end_time = datetime.now(timezone.utc)

        if context:
            session.context.update(context)

        # Calculate session duration
        duration = (session.end_time - session.start_time).total_seconds()

        # Record session metrics
        await self.track_feature_usage(
            feature_name="session_duration",
            user_identifier=session.user_identifier,
            duration_seconds=duration,
            success_rate=session.success_rate,
            usage_count=session.total_interactions,
            context={
                **session.context,
                "features_used": list(session.features_used),
                "session_duration_minutes": duration / 60
            },
            session_id=session_id
        )

        # Remove from active sessions
        del self.active_sessions[session_id]

        logger.debug(f"Ended session {session_id}, duration: {duration:.1f}s")
        return session

    async def track_search_usage(
        self,
        query: str,
        results_count: int,
        clicked_results: int = 0,
        user_identifier: Optional[str] = None,
        session_id: Optional[str] = None,
        response_time_ms: Optional[float] = None
    ) -> str:
        """Track search feature usage with detailed metrics.

        Args:
            query: Search query text
            results_count: Number of search results returned
            clicked_results: Number of results clicked/selected
            user_identifier: User performing the search
            session_id: Session identifier
            response_time_ms: Search response time in milliseconds

        Returns:
            ID of the created usage metric record
        """
        # Calculate search success rate
        success_rate = (clicked_results / results_count) if results_count > 0 else 0.0

        # Calculate query complexity score
        complexity_score = self._calculate_query_complexity(query)

        context = {
            "query_length": len(query),
            "results_count": results_count,
            "clicked_results": clicked_results,
            "complexity_score": complexity_score,
            "response_time_ms": response_time_ms
        }

        return await self.track_feature_usage(
            feature_name="search_feature",
            user_identifier=user_identifier,
            duration_seconds=response_time_ms / 1000 if response_time_ms else None,
            success_rate=success_rate,
            context=context,
            session_id=session_id
        )

    async def track_daily_review_engagement(
        self,
        review_duration_seconds: float,
        items_reviewed: int,
        items_completed: int,
        review_type: str,
        user_identifier: Optional[str] = None,
        session_id: Optional[str] = None
    ) -> str:
        """Track daily review engagement metrics.

        Args:
            review_duration_seconds: Time spent in review
            items_reviewed: Number of items reviewed
            items_completed: Number of items completed/acted upon
            review_type: Type of review (morning, evening, weekly)
            user_identifier: User performing the review
            session_id: Session identifier

        Returns:
            ID of the created usage metric record
        """
        # Calculate engagement success rate
        success_rate = (items_completed / items_reviewed) if items_reviewed > 0 else 0.0

        context = {
            "review_type": review_type,
            "items_reviewed": items_reviewed,
            "items_completed": items_completed,
            "avg_time_per_item": review_duration_seconds / items_reviewed if items_reviewed > 0 else 0,
            "completion_rate": success_rate
        }

        return await self.track_feature_usage(
            feature_name="daily_review",
            user_identifier=user_identifier,
            duration_seconds=review_duration_seconds,
            success_rate=success_rate,
            usage_count=items_reviewed,
            context=context,
            session_id=session_id
        )

    async def track_project_query(
        self,
        project_name: str,
        query_type: str,
        results_found: bool,
        response_time_ms: float,
        user_identifier: Optional[str] = None,
        session_id: Optional[str] = None
    ) -> str:
        """Track project-specific query usage.

        Args:
            project_name: Name of the project queried
            query_type: Type of query (timeline, status, documents, etc.)
            results_found: Whether relevant results were found
            response_time_ms: Query response time
            user_identifier: User performing the query
            session_id: Session identifier

        Returns:
            ID of the created usage metric record
        """
        context = {
            "project_name": project_name,
            "query_type": query_type,
            "results_found": results_found,
            "response_time_ms": response_time_ms
        }

        return await self.track_feature_usage(
            feature_name="project_query",
            user_identifier=user_identifier,
            duration_seconds=response_time_ms / 1000,
            success_rate=1.0 if results_found else 0.0,
            context=context,
            session_id=session_id
        )

    async def get_feature_usage_insights(
        self,
        feature_name: str,
        days_back: int = 7,
        use_cache: bool = True
    ) -> Optional[FeatureUsageInsight]:
        """Get comprehensive usage insights for a specific feature.

        Args:
            feature_name: Name of the feature to analyze
            days_back: Number of days of history to analyze
            use_cache: Whether to use cached insights if available

        Returns:
            Feature usage insights and analytics
        """
        if not self.is_running:
            raise RuntimeError("UsageAnalyticsService is not running")

        cache_key = f"{feature_name}_{days_back}d"

        # Check cache first
        if use_cache and cache_key in self.feature_insights_cache:
            cache_age = datetime.now(timezone.utc) - self.last_cache_update.get(cache_key, datetime.min.replace(tzinfo=timezone.utc))
            if cache_age.total_seconds() < self.cache_ttl_seconds:
                return self.feature_insights_cache[cache_key]

        try:
            async with self.session_factory() as session:
                since_time = datetime.now(timezone.utc) - timedelta(days=days_back)

                # Get usage metrics for the feature
                query = select(UsageMetric).where(
                    and_(
                        UsageMetric.tenant_id == self.tenant_id,
                        UsageMetric.feature_name == feature_name,
                        UsageMetric.timestamp >= since_time
                    )
                ).order_by(desc(UsageMetric.timestamp))

                result = await session.execute(query)
                metrics = result.scalars().all()

                if not metrics:
                    return None

                # Calculate insights
                total_usage_count = sum(metric.usage_count for metric in metrics)
                unique_users = len(set(metric.user_identifier for metric in metrics if metric.user_identifier))

                durations = [metric.duration_seconds for metric in metrics if metric.duration_seconds is not None]
                avg_session_duration = statistics.mean(durations) if durations else 0.0

                success_rates = [metric.success_rate for metric in metrics if metric.success_rate is not None]
                avg_success_rate = statistics.mean(success_rates) if success_rates else 0.0

                # Calculate adoption rate (users using this feature vs total active users)
                total_active_users = await self._get_total_active_users(since_time)
                adoption_rate = (unique_users / total_active_users * 100) if total_active_users > 0 else 0.0

                # Determine engagement level
                engagement_level = self._determine_feature_engagement(
                    total_usage_count, unique_users, avg_session_duration, avg_success_rate
                )

                # Calculate trend direction
                trend_direction = self._calculate_feature_trend(metrics)

                # Find peak usage hours
                peak_hours = self._find_peak_usage_hours(metrics)

                # Get common contexts
                common_contexts = self._analyze_common_contexts(metrics)

                # Get feature category
                feature_category = FEATURE_CATEGORIES.get(feature_name, FeatureCategory.CORE)

                insights = FeatureUsageInsight(
                    feature_name=feature_name,
                    feature_category=feature_category,
                    total_usage_count=total_usage_count,
                    unique_users=unique_users,
                    avg_session_duration=avg_session_duration,
                    success_rate=avg_success_rate,
                    adoption_rate=adoption_rate,
                    engagement_level=engagement_level,
                    trend_direction=trend_direction,
                    peak_usage_hours=peak_hours,
                    common_contexts=common_contexts,
                    last_updated=datetime.now(timezone.utc)
                )

                # Cache the insights
                self.feature_insights_cache[cache_key] = insights
                self.last_cache_update[cache_key] = datetime.now(timezone.utc)

                return insights

        except SQLAlchemyError as e:
            logger.error(f"Database error getting feature insights: {e}")
            raise
        except Exception as e:
            logger.error(f"Error getting feature insights: {e}")
            raise

    async def get_usage_analytics_summary(
        self,
        days_back: int = 7
    ) -> UsageAnalytics:
        """Get comprehensive usage analytics summary.

        Args:
            days_back: Number of days to include in analysis

        Returns:
            Complete usage analytics summary
        """
        if not self.is_running:
            raise RuntimeError("UsageAnalyticsService is not running")

        try:
            async with self.session_factory() as session:
                since_time = datetime.now(timezone.utc) - timedelta(days=days_back)
                end_time = datetime.now(timezone.utc)

                # Get all usage metrics for the period
                query = select(UsageMetric).where(
                    and_(
                        UsageMetric.tenant_id == self.tenant_id,
                        UsageMetric.timestamp >= since_time
                    )
                )

                result = await session.execute(query)
                metrics = result.scalars().all()

                # Calculate summary statistics
                total_sessions = len(set(metric.session_id for metric in metrics if metric.session_id))
                unique_users = len(set(metric.user_identifier for metric in metrics if metric.user_identifier))

                # Calculate average session duration
                session_durations = {}
                for metric in metrics:
                    if metric.session_id and metric.duration_seconds:
                        if metric.session_id not in session_durations:
                            session_durations[metric.session_id] = []
                        session_durations[metric.session_id].append(metric.duration_seconds)

                avg_session_duration = 0.0
                if session_durations:
                    session_totals = [sum(durations) for durations in session_durations.values()]
                    avg_session_duration = statistics.mean(session_totals)

                # Most popular features
                feature_counts = defaultdict(int)
                for metric in metrics:
                    feature_counts[metric.feature_name] += metric.usage_count

                most_popular_features = sorted(feature_counts.items(), key=lambda x: x[1], reverse=True)[:10]

                # Feature adoption rates
                feature_users = defaultdict(set)
                for metric in metrics:
                    if metric.user_identifier:
                        feature_users[metric.feature_name].add(metric.user_identifier)

                feature_adoption_rates = {}
                for feature, users in feature_users.items():
                    adoption_rate = (len(users) / unique_users * 100) if unique_users > 0 else 0.0
                    feature_adoption_rates[feature] = adoption_rate

                # Engagement distribution (simplified)
                engagement_distribution = {
                    EngagementLevel.HIGH: int(unique_users * 0.2),
                    EngagementLevel.MEDIUM: int(unique_users * 0.4),
                    EngagementLevel.LOW: int(unique_users * 0.3),
                    EngagementLevel.INACTIVE: int(unique_users * 0.1)
                }

                # Peak usage times
                peak_usage_times = self._find_peak_usage_hours(metrics)

                # Success rate by feature
                success_rate_by_feature = {}
                feature_success_rates = defaultdict(list)
                for metric in metrics:
                    if metric.success_rate is not None:
                        feature_success_rates[metric.feature_name].append(metric.success_rate)

                for feature, rates in feature_success_rates.items():
                    success_rate_by_feature[feature] = statistics.mean(rates)

                # Query complexity trends (simplified)
                query_complexity_trends = {"average_complexity": 2.5, "trend": "stable"}

                return UsageAnalytics(
                    period_start=since_time,
                    period_end=end_time,
                    total_sessions=total_sessions,
                    unique_users=unique_users,
                    avg_session_duration=avg_session_duration,
                    most_popular_features=most_popular_features,
                    feature_adoption_rates=feature_adoption_rates,
                    engagement_distribution=engagement_distribution,
                    peak_usage_times=peak_usage_times,
                    success_rate_by_feature=success_rate_by_feature,
                    query_complexity_trends=query_complexity_trends
                )

        except SQLAlchemyError as e:
            logger.error(f"Database error getting usage analytics: {e}")
            raise
        except Exception as e:
            logger.error(f"Error getting usage analytics: {e}")
            raise

    # ========================================================================
    # Private Methods
    # ========================================================================

    async def _analytics_loop(self) -> None:
        """Background analytics processing loop."""
        while self.is_running:
            try:
                # Process usage patterns
                await self._process_usage_patterns()

                # Update engagement profiles
                await self._update_engagement_profiles()

                # Clean up old sessions
                await self._cleanup_expired_sessions()

            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Error in analytics loop: {e}")

            try:
                await asyncio.sleep(self.analytics_interval)
            except asyncio.CancelledError:
                break

    async def _cleanup_loop(self) -> None:
        """Background cleanup loop for old data."""
        while self.is_running:
            try:
                await self._cleanup_old_metrics()
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Error in cleanup loop: {e}")

            try:
                await asyncio.sleep(self.cleanup_interval)
            except asyncio.CancelledError:
                break

    def _hash_user_identifier(self, user_identifier: str) -> str:
        """Hash user identifier for privacy protection."""
        if not self.hash_user_identifiers:
            return user_identifier

        # Use SHA-256 for consistent hashing
        return hashlib.sha256(f"{user_identifier}_{self.tenant_id}".encode()).hexdigest()[:16]

    def _get_metric_type_for_feature(self, feature_name: str) -> str:
        """Get appropriate metric type for a feature."""
        if "search" in feature_name.lower():
            return UsageMetricType.SEARCH_FEATURE_USAGE.value
        elif "review" in feature_name.lower():
            return UsageMetricType.DAILY_REVIEW_ENGAGEMENT.value
        elif "project" in feature_name.lower():
            return UsageMetricType.PROJECT_QUERY_FREQUENCY.value
        elif "workflow" in feature_name.lower():
            return UsageMetricType.WORKFLOW_UTILIZATION.value
        elif "session" in feature_name.lower():
            return UsageMetricType.SESSION_DURATION.value
        else:
            return UsageMetricType.FEATURE_ADOPTION_RATE.value

    def _calculate_query_complexity(self, query: str) -> float:
        """Calculate complexity score for a search query."""
        # Simple complexity calculation based on query characteristics
        words = query.split()
        complexity_score = 1.0

        # Length factor
        if len(words) > 5:
            complexity_score += 0.5
        if len(words) > 10:
            complexity_score += 0.5

        # Special operators
        operators = ["AND", "OR", "NOT", '"', "(", ")", ":"]
        operator_count = sum(1 for op in operators if op in query)
        complexity_score += operator_count * 0.3

        # Date/time references
        time_words = ["today", "yesterday", "week", "month", "year", "recent", "last"]
        time_count = sum(1 for word in words if word.lower() in time_words)
        complexity_score += time_count * 0.2

        return min(complexity_score, 5.0)  # Cap at 5.0

    def _determine_feature_engagement(
        self,
        usage_count: int,
        unique_users: int,
        avg_duration: float,
        success_rate: float
    ) -> EngagementLevel:
        """Determine engagement level based on usage metrics."""
        # Simple scoring algorithm
        score = 0

        if usage_count > 100:
            score += 2
        elif usage_count > 20:
            score += 1

        if unique_users > 10:
            score += 2
        elif unique_users > 3:
            score += 1

        if avg_duration > 300:  # 5 minutes
            score += 2
        elif avg_duration > 60:  # 1 minute
            score += 1

        if success_rate > 0.8:
            score += 2
        elif success_rate > 0.5:
            score += 1

        if score >= 6:
            return EngagementLevel.HIGH
        elif score >= 4:
            return EngagementLevel.MEDIUM
        elif score >= 2:
            return EngagementLevel.LOW
        else:
            return EngagementLevel.INACTIVE

    def _calculate_feature_trend(self, metrics: List[UsageMetric]) -> str:
        """Calculate trend direction for feature usage."""
        if len(metrics) < 7:
            return "stable"

        # Group by day and calculate daily usage
        daily_usage = defaultdict(int)
        for metric in metrics:
            day_key = metric.timestamp.date()
            daily_usage[day_key] += metric.usage_count

        # Sort by date and calculate trend
        sorted_days = sorted(daily_usage.keys())
        usage_values = [daily_usage[day] for day in sorted_days]

        if len(usage_values) < 3:
            return "stable"

        # Simple trend calculation
        first_half = usage_values[:len(usage_values)//2]
        second_half = usage_values[len(usage_values)//2:]

        avg_first = statistics.mean(first_half)
        avg_second = statistics.mean(second_half)

        change_percentage = ((avg_second - avg_first) / avg_first * 100) if avg_first > 0 else 0

        if change_percentage > 20:
            return "improving"
        elif change_percentage < -20:
            return "declining"
        else:
            return "stable"

    def _find_peak_usage_hours(self, metrics: List[UsageMetric]) -> List[int]:
        """Find peak usage hours from metrics."""
        hour_counts = defaultdict(int)
        for metric in metrics:
            hour = metric.timestamp.hour
            hour_counts[hour] += metric.usage_count

        # Sort by usage count and return top 3 hours
        sorted_hours = sorted(hour_counts.items(), key=lambda x: x[1], reverse=True)
        return [hour for hour, count in sorted_hours[:3]]

    def _analyze_common_contexts(self, metrics: List[UsageMetric]) -> Dict[str, int]:
        """Analyze common context patterns in usage metrics."""
        context_counts = defaultdict(int)

        for metric in metrics:
            if metric.usage_context:
                for key, value in metric.usage_context.items():
                    if isinstance(value, str) and len(value) < 50:  # Reasonable length
                        context_counts[f"{key}:{value}"] += 1

        # Return top 10 most common contexts
        sorted_contexts = sorted(context_counts.items(), key=lambda x: x[1], reverse=True)
        return dict(sorted_contexts[:10])

    async def _get_total_active_users(self, since_time: datetime) -> int:
        """Get total number of active users in the time period."""
        try:
            async with self.session_factory() as session:
                query = select(func.count(func.distinct(UsageMetric.user_identifier))).where(
                    and_(
                        UsageMetric.tenant_id == self.tenant_id,
                        UsageMetric.timestamp >= since_time,
                        UsageMetric.user_identifier.isnot(None)
                    )
                )

                result = await session.execute(query)
                return result.scalar() or 0

        except Exception as e:
            logger.error(f"Error getting total active users: {e}")
            return 0

    def _invalidate_feature_cache(self, feature_name: str) -> None:
        """Invalidate cache entries related to a feature."""
        keys_to_remove = [key for key in self.feature_insights_cache.keys() if key.startswith(feature_name)]
        for key in keys_to_remove:
            self.feature_insights_cache.pop(key, None)
            self.last_cache_update.pop(key, None)

    async def _process_usage_patterns(self) -> None:
        """Process usage patterns for insights."""
        # This could be expanded to include more sophisticated analytics
        logger.debug("Processing usage patterns")

    async def _update_engagement_profiles(self) -> None:
        """Update user engagement profiles."""
        # This could be expanded to maintain detailed user profiles
        logger.debug("Updating engagement profiles")

    async def _cleanup_expired_sessions(self) -> None:
        """Clean up expired active sessions."""
        cutoff_time = datetime.now(timezone.utc) - timedelta(minutes=self.session_timeout_minutes)
        expired_sessions = []

        for session_id, session in self.active_sessions.items():
            if session.start_time < cutoff_time:
                expired_sessions.append(session_id)

        for session_id in expired_sessions:
            await self.end_session(session_id, {"ended_reason": "timeout"})

    async def _cleanup_old_metrics(self) -> None:
        """Clean up old usage metrics beyond retention period."""
        cutoff_date = datetime.now(timezone.utc) - timedelta(days=self.retention_days)

        try:
            async with self.session_factory() as session:
                # Delete old usage metrics
                delete_query = text("""
                    DELETE FROM usage_metrics
                    WHERE tenant_id = :tenant_id AND timestamp < :cutoff_date
                """)

                result = await session.execute(delete_query, {
                    "tenant_id": self.tenant_id,
                    "cutoff_date": cutoff_date
                })

                await session.commit()

                if result.rowcount > 0:
                    logger.info(f"Cleaned up {result.rowcount} old usage metrics")

        except Exception as e:
            logger.error(f"Error cleaning up old metrics: {e}")