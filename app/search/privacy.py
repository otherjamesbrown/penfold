"""
Privacy-Aware Search Filtering

Provides privacy controls and access filtering for search operations
based on user permissions and content classification levels.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, and_, or_
from typing import List, Dict, Any, Optional, Set
from enum import Enum
from datetime import datetime
import uuid

from app.database import MeetingFile, MeetingTranscript, MeetingSummary


class PrivacyLevel(Enum):
    """Privacy levels for meeting content"""
    PUBLIC = "public"                    # Publicly accessible
    ORGANIZATION = "organization"        # Organization-wide access
    TEAM_ONLY = "team_only"             # Team members only
    CONFIDENTIAL = "confidential"       # Restricted access


class UserRole(Enum):
    """User roles for access control"""
    GUEST = "guest"
    MEMBER = "member"
    TEAM_LEAD = "team_lead"
    ADMIN = "admin"
    SUPER_ADMIN = "super_admin"


class PrivacyFilterService:
    """Service for privacy-aware search filtering and access control"""

    def __init__(self):
        # Define access matrix: role -> allowed privacy levels
        self.access_matrix = {
            UserRole.GUEST: [PrivacyLevel.PUBLIC],
            UserRole.MEMBER: [PrivacyLevel.PUBLIC, PrivacyLevel.ORGANIZATION],
            UserRole.TEAM_LEAD: [PrivacyLevel.PUBLIC, PrivacyLevel.ORGANIZATION, PrivacyLevel.TEAM_ONLY],
            UserRole.ADMIN: [PrivacyLevel.PUBLIC, PrivacyLevel.ORGANIZATION, PrivacyLevel.TEAM_ONLY],
            UserRole.SUPER_ADMIN: [PrivacyLevel.PUBLIC, PrivacyLevel.ORGANIZATION, PrivacyLevel.TEAM_ONLY, PrivacyLevel.CONFIDENTIAL]
        }

        # Team-based access cache (in production, this would be backed by a proper auth system)
        self.team_memberships = {}  # user_id -> set of team_ids

    def get_allowed_privacy_levels(
        self,
        user_id: str,
        user_role: UserRole,
        team_ids: Optional[Set[str]] = None,
        additional_permissions: Optional[Set[str]] = None
    ) -> List[str]:
        """Get privacy levels accessible to a user"""

        # Base access from role
        allowed_levels = set()
        for level in self.access_matrix.get(user_role, []):
            allowed_levels.add(level.value)

        # Enhanced access for team leads within their teams
        if user_role == UserRole.TEAM_LEAD and team_ids:
            # Team leads can access team_only content for their teams
            allowed_levels.add(PrivacyLevel.TEAM_ONLY.value)

        # Additional permissions (for special cases)
        if additional_permissions:
            for permission in additional_permissions:
                if permission in [level.value for level in PrivacyLevel]:
                    allowed_levels.add(permission)

        return list(allowed_levels)

    async def filter_search_results(
        self,
        search_results: List[Dict[str, Any]],
        user_id: str,
        user_role: UserRole,
        team_ids: Optional[Set[str]] = None,
        db: AsyncSession = None
    ) -> List[Dict[str, Any]]:
        """Filter search results based on user access rights"""

        if not search_results:
            return []

        # Get allowed privacy levels for the user
        allowed_levels = self.get_allowed_privacy_levels(user_id, user_role, team_ids)

        # Filter results
        filtered_results = []
        for result in search_results:
            privacy_level = result.get("privacy_level", "confidential")

            # Check basic privacy access
            if privacy_level in allowed_levels:
                # Additional filtering for team_only content
                if privacy_level == PrivacyLevel.TEAM_ONLY.value:
                    if await self._check_team_access(result, user_id, team_ids, db):
                        filtered_results.append(result)
                else:
                    filtered_results.append(result)

        return filtered_results

    async def apply_privacy_constraints(
        self,
        base_query,
        user_id: str,
        user_role: UserRole,
        team_ids: Optional[Set[str]] = None
    ):
        """Apply privacy constraints to a SQLAlchemy query"""

        # Get allowed privacy levels
        allowed_levels = self.get_allowed_privacy_levels(user_id, user_role, team_ids)

        # Add privacy level filter
        privacy_filter = MeetingFile.privacy_level.in_(allowed_levels)

        # For team_only content, add team membership check if needed
        if PrivacyLevel.TEAM_ONLY.value in allowed_levels and team_ids:
            # This would need to be expanded with proper team-meeting relationships
            # For now, we'll allow team_only if user has team memberships
            team_filter = or_(
                MeetingFile.privacy_level != PrivacyLevel.TEAM_ONLY.value,
                # Add team-specific filtering here when team-meeting relationships are implemented
            )
            privacy_filter = and_(privacy_filter, team_filter)

        return base_query.where(privacy_filter)

    async def _check_team_access(
        self,
        result: Dict[str, Any],
        user_id: str,
        team_ids: Optional[Set[str]],
        db: AsyncSession
    ) -> bool:
        """Check if user has access to team_only content"""

        if not team_ids:
            return False

        # In a full implementation, this would check if the meeting
        # is associated with any of the user's teams
        # For now, we'll allow access if user is in any team
        return len(team_ids) > 0

    def sanitize_search_results(
        self,
        search_results: List[Dict[str, Any]],
        user_role: UserRole,
        include_sensitive_data: bool = False
    ) -> List[Dict[str, Any]]:
        """Sanitize search results by removing sensitive information"""

        sanitized_results = []

        for result in search_results.copy():
            sanitized_result = result.copy()

            privacy_level = result.get("privacy_level", "confidential")

            # Remove sensitive fields based on privacy level and user role
            if privacy_level == PrivacyLevel.CONFIDENTIAL.value and user_role not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
                # Very restricted view for confidential content
                sanitized_result = self._create_restricted_view(result)

            elif privacy_level == PrivacyLevel.TEAM_ONLY.value and user_role == UserRole.MEMBER:
                # Limited view for organization members viewing team content
                sanitized_result = self._create_limited_view(result)

            # Remove internal metadata for non-admin users
            if user_role not in [UserRole.ADMIN, UserRole.SUPER_ADMIN]:
                sanitized_result.pop("metadata", None)
                sanitized_result.pop("quality_metrics", None)
                sanitized_result.pop("processing_metadata", None)

            # Limit text previews for sensitive content
            if privacy_level in [PrivacyLevel.CONFIDENTIAL.value, PrivacyLevel.TEAM_ONLY.value]:
                self._limit_text_previews(sanitized_result)

            sanitized_results.append(sanitized_result)

        return sanitized_results

    def _create_restricted_view(self, result: Dict[str, Any]) -> Dict[str, Any]:
        """Create a very restricted view of confidential content"""

        return {
            "meeting_id": result.get("meeting_id"),
            "filename": self._sanitize_filename(result.get("filename", "")),
            "privacy_level": result.get("privacy_level"),
            "created_at": result.get("created_at"),
            "result_type": result.get("result_type", "restricted"),
            "access_restricted": True,
            "message": "Access restricted - contact administrator for details"
        }

    def _create_limited_view(self, result: Dict[str, Any]) -> Dict[str, Any]:
        """Create a limited view for team_only content viewed by organization members"""

        limited_result = result.copy()

        # Remove detailed content
        limited_result.pop("transcript_text", None)
        limited_result.pop("summary_text", None)
        limited_result.pop("text_preview", None)
        limited_result.pop("highlighted_text", None)
        limited_result.pop("speaker_segments", None)

        # Keep basic metadata
        limited_result["access_level"] = "limited"
        limited_result["message"] = "Limited access - join relevant team for full details"

        return limited_result

    def _limit_text_previews(self, result: Dict[str, Any]) -> None:
        """Limit text preview length for sensitive content"""

        max_preview_length = 100

        # Limit various text fields
        text_fields = ["text_preview", "transcript_preview", "summary_preview", "highlighted_text"]

        for field in text_fields:
            if field in result and result[field]:
                if len(result[field]) > max_preview_length:
                    result[field] = result[field][:max_preview_length] + "... [content truncated]"

    def _sanitize_filename(self, filename: str) -> str:
        """Sanitize filename to remove potentially sensitive information"""

        if not filename:
            return "Meeting Recording"

        # Remove common sensitive patterns
        sanitized = filename

        # Remove email addresses
        import re
        sanitized = re.sub(r'\S+@\S+\.\S+', '[email]', sanitized)

        # Remove what looks like personal names (simple heuristic)
        # This would need more sophisticated NER in production
        words = sanitized.split()
        if len(words) > 1:
            # Keep first word and generic descriptors
            generic_words = {'meeting', 'call', 'session', 'discussion', 'standup', 'sync', 'review'}
            filtered_words = [words[0]]  # Keep first word
            for word in words[1:]:
                if word.lower() in generic_words or word.isdigit() or len(word) <= 3:
                    filtered_words.append(word)
                else:
                    filtered_words.append('[name]')

            sanitized = ' '.join(filtered_words)

        return sanitized

    async def audit_search_access(
        self,
        user_id: str,
        query: str,
        results_count: int,
        privacy_levels_accessed: List[str],
        db: AsyncSession,
        request_id: str = None,
        ip_address: str = None,
        user_agent: str = None
    ) -> None:
        """Audit search access for security monitoring"""

        from app.logging_config import log_audit_event

        audit_details = {
            "query": query,
            "results_count": results_count,
            "privacy_levels": privacy_levels_accessed,
            "ip_address": ip_address,
            "user_agent": user_agent
        }

        # Log structured audit event
        log_audit_event(
            action="search_access",
            user_id=user_id,
            details=audit_details,
            request_id=request_id
        )

    def check_content_access(
        self,
        user_id: str,
        user_role: UserRole,
        privacy_level: str,
        content_metadata: Dict[str, Any] = None,
        team_ids: Optional[Set[str]] = None
    ) -> bool:
        """Check if user can access specific content"""

        allowed_levels = self.get_allowed_privacy_levels(user_id, user_role, team_ids)

        # Basic privacy level check
        if privacy_level not in allowed_levels:
            return False

        # Additional checks for team_only content
        if privacy_level == PrivacyLevel.TEAM_ONLY.value:
            # Check team membership (simplified for now)
            return team_ids is not None and len(team_ids) > 0

        # Additional checks for confidential content
        if privacy_level == PrivacyLevel.CONFIDENTIAL.value:
            # Only admin and super_admin roles can access confidential content
            return user_role in [UserRole.ADMIN, UserRole.SUPER_ADMIN]

        return True

    def get_privacy_level_description(self, privacy_level: str) -> str:
        """Get human-readable description of privacy level"""

        descriptions = {
            PrivacyLevel.PUBLIC.value: "Public - Accessible to anyone",
            PrivacyLevel.ORGANIZATION.value: "Organization - Accessible to all organization members",
            PrivacyLevel.TEAM_ONLY.value: "Team Only - Accessible to team members only",
            PrivacyLevel.CONFIDENTIAL.value: "Confidential - Restricted access only"
        }

        return descriptions.get(privacy_level, "Unknown privacy level")

    def recommend_privacy_level(
        self,
        content_metadata: Dict[str, Any],
        filename: str = "",
        participant_count: int = 0
    ) -> str:
        """Recommend appropriate privacy level based on content analysis"""

        # Default to organization level
        recommended_level = PrivacyLevel.ORGANIZATION.value

        # Check filename for sensitivity indicators
        filename_lower = filename.lower()
        confidential_keywords = ["confidential", "private", "restricted", "internal", "classified"]
        team_keywords = ["team", "standup", "sync", "1on1", "one-on-one"]

        if any(keyword in filename_lower for keyword in confidential_keywords):
            recommended_level = PrivacyLevel.CONFIDENTIAL.value
        elif any(keyword in filename_lower for keyword in team_keywords):
            recommended_level = PrivacyLevel.TEAM_ONLY.value

        # Consider participant count
        if participant_count <= 2:
            # Small meetings might be more sensitive
            if recommended_level == PrivacyLevel.ORGANIZATION.value:
                recommended_level = PrivacyLevel.TEAM_ONLY.value

        elif participant_count >= 10:
            # Large meetings might be more general
            if recommended_level == PrivacyLevel.TEAM_ONLY.value:
                recommended_level = PrivacyLevel.ORGANIZATION.value

        return recommended_level

    async def get_user_search_statistics(
        self,
        user_id: str,
        db: AsyncSession,
        days_back: int = 30
    ) -> Dict[str, Any]:
        """Get search statistics for a user (for admin monitoring)"""

        # In production, this would query audit logs
        # For now, return placeholder statistics
        return {
            "user_id": user_id,
            "period_days": days_back,
            "total_searches": 0,
            "privacy_levels_accessed": [],
            "most_common_queries": [],
            "last_search_date": None,
            "note": "Statistics not yet implemented - would require audit log system"
        }


# Global privacy filter service
privacy_filter = PrivacyFilterService()