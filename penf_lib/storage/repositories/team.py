"""Team repository with specialized operations."""

import uuid
from typing import List, Optional, Dict, Any
from datetime import datetime

from sqlalchemy import select, and_, or_, func, desc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from penf_lib.storage.models import Team
from .base import BaseRepository


class TeamRepository(BaseRepository[Team]):
    """Repository for Team entities."""

    def __init__(self, session: AsyncSession):
        """Initialize team repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, Team)

    async def create_team(
        self,
        tenant_id: uuid.UUID,
        name: str,
        team_type: Optional[str] = None,
        description: Optional[str] = None,
        member_emails: Optional[List[str]] = None,
        organizational_context: Optional[Dict[str, Any]] = None,
    ) -> Team:
        """Create a new team with validation.

        Args:
            tenant_id: Tenant ID
            name: Team name
            team_type: Type of team
            description: Team description
            member_emails: List of member email addresses
            organizational_context: Organizational metadata

        Returns:
            Created team instance
        """
        return await self.create(
            tenant_id=tenant_id,
            name=name,
            team_type=team_type,
            description=description,
            member_emails=member_emails or [],
            organizational_context=organizational_context,
        )

    async def find_by_name(
        self, tenant_id: uuid.UUID, name: str
    ) -> Optional[Team]:
        """Find team by name.

        Args:
            tenant_id: Tenant ID
            name: Team name

        Returns:
            Team instance or None if not found
        """
        return await self.find_one_by(
            tenant_id=tenant_id,
            name=name
        )

    async def find_by_type(
        self, tenant_id: uuid.UUID, team_type: str
    ) -> List[Team]:
        """Find teams by type.

        Args:
            tenant_id: Tenant ID
            team_type: Team type

        Returns:
            List of team instances
        """
        return await self.find_by(
            tenant_id=tenant_id,
            team_type=team_type
        )

    async def find_by_member_email(
        self, tenant_id: uuid.UUID, email: str
    ) -> List[Team]:
        """Find teams where person is a member.

        Args:
            tenant_id: Tenant ID
            email: Member email address

        Returns:
            List of team instances
        """
        email_lower = email.lower().strip()
        stmt = (
            self._build_query()
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    Team.member_emails.any(email_lower)
                )
            )
            .order_by(Team.name)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def search_teams(
        self, tenant_id: uuid.UUID, search_term: str, limit: int = 50
    ) -> List[Team]:
        """Search teams by name and description.

        Args:
            tenant_id: Tenant ID
            search_term: Search term
            limit: Maximum results to return

        Returns:
            List of matching team instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    or_(
                        Team.name.ilike(f'%{search_term}%'),
                        Team.description.ilike(f'%{search_term}%')
                    )
                )
            )
            .order_by(Team.name)
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def add_member(self, team_id: int, email: str) -> Optional[Team]:
        """Add member to team.

        Args:
            team_id: Team ID
            email: Member email address

        Returns:
            Updated team instance or None if not found
        """
        team = await self.get_by_id(team_id)
        if not team:
            return None

        email_lower = email.lower().strip()
        if email_lower not in team.member_emails:
            team.member_emails = team.member_emails + [email_lower]
            return await self.update_entity(team)

        return team

    async def remove_member(self, team_id: int, email: str) -> Optional[Team]:
        """Remove member from team.

        Args:
            team_id: Team ID
            email: Member email address

        Returns:
            Updated team instance or None if not found
        """
        team = await self.get_by_id(team_id)
        if not team:
            return None

        email_lower = email.lower().strip()
        if email_lower in team.member_emails:
            updated_members = [e for e in team.member_emails if e != email_lower]
            team.member_emails = updated_members
            return await self.update_entity(team)

        return team

    async def bulk_add_members(
        self, team_id: int, emails: List[str]
    ) -> Optional[Team]:
        """Add multiple members to team.

        Args:
            team_id: Team ID
            emails: List of member email addresses

        Returns:
            Updated team instance or None if not found
        """
        team = await self.get_by_id(team_id)
        if not team:
            return None

        normalized_emails = [e.lower().strip() for e in emails]
        existing_emails = set(team.member_emails)

        # Add only new emails
        new_emails = [e for e in normalized_emails if e not in existing_emails]
        if new_emails:
            team.member_emails = team.member_emails + new_emails
            return await self.update_entity(team)

        return team

    async def get_member_count(self, team_id: int) -> int:
        """Get team member count.

        Args:
            team_id: Team ID

        Returns:
            Number of team members
        """
        team = await self.get_by_id(team_id)
        return len(team.member_emails) if team else 0

    async def find_teams_with_member_count(
        self, tenant_id: uuid.UUID
    ) -> List[Dict[str, Any]]:
        """Find teams with member count information.

        Args:
            tenant_id: Tenant ID

        Returns:
            List of dictionaries with team info and member counts
        """
        stmt = (
            select(
                Team.id,
                Team.name,
                Team.team_type,
                Team.description,
                func.array_length(Team.member_emails, 1).label('member_count')
            )
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    Team.is_deleted.is_(False)
                )
            )
            .order_by(Team.name)
        )

        result = await self.session.execute(stmt)
        return [
            {
                'id': row.id,
                'name': row.name,
                'team_type': row.team_type,
                'description': row.description,
                'member_count': row.member_count or 0,
            }
            for row in result.all()
        ]

    async def find_by_organization(
        self, tenant_id: uuid.UUID, organization_key: str, organization_value: str
    ) -> List[Team]:
        """Find teams by organizational context.

        Args:
            tenant_id: Tenant ID
            organization_key: Key in organizational_context
            organization_value: Value to match

        Returns:
            List of team instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    Team.organizational_context[organization_key].astext == organization_value
                )
            )
            .order_by(Team.name)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_stats_by_type(self, tenant_id: uuid.UUID) -> Dict[str, int]:
        """Get team count statistics by type.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary mapping team types to counts
        """
        stmt = (
            select(
                func.coalesce(Team.team_type, 'unset').label('team_type'),
                func.count(Team.id)
            )
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    Team.is_deleted.is_(False)
                )
            )
            .group_by('team_type')
        )

        result = await self.session.execute(stmt)
        return dict(result.all())

    async def get_member_statistics(self, tenant_id: uuid.UUID) -> Dict[str, Any]:
        """Get team member statistics.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary with team member statistics
        """
        # Total teams
        total_stmt = (
            select(func.count(Team.id))
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    Team.is_deleted.is_(False)
                )
            )
        )

        # Average team size
        avg_size_stmt = (
            select(func.avg(func.array_length(Team.member_emails, 1)))
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    Team.is_deleted.is_(False)
                )
            )
        )

        # Largest team size
        max_size_stmt = (
            select(func.max(func.array_length(Team.member_emails, 1)))
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    Team.is_deleted.is_(False)
                )
            )
        )

        total_result = await self.session.execute(total_stmt)
        avg_result = await self.session.execute(avg_size_stmt)
        max_result = await self.session.execute(max_size_stmt)

        return {
            'total_teams': total_result.scalar() or 0,
            'average_team_size': float(avg_result.scalar() or 0),
            'largest_team_size': max_result.scalar() or 0,
        }

    async def find_empty_teams(self, tenant_id: uuid.UUID) -> List[Team]:
        """Find teams with no members.

        Args:
            tenant_id: Tenant ID

        Returns:
            List of empty team instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    or_(
                        Team.member_emails.is_(None),
                        func.array_length(Team.member_emails, 1).is_(None),
                        func.array_length(Team.member_emails, 1) == 0
                    )
                )
            )
            .order_by(Team.name)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_large_teams(
        self, tenant_id: uuid.UUID, min_size: int = 10
    ) -> List[Team]:
        """Find teams with many members.

        Args:
            tenant_id: Tenant ID
            min_size: Minimum team size threshold

        Returns:
            List of large team instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Team.tenant_id == tenant_id,
                    func.array_length(Team.member_emails, 1) >= min_size
                )
            )
            .order_by(func.array_length(Team.member_emails, 1).desc())
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())