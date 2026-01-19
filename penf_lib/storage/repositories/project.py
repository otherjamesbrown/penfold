"""Project repository with specialized operations."""

import uuid
from typing import List, Optional, Dict, Any
from datetime import datetime

from sqlalchemy import select, and_, or_, func, desc, text
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from penf_lib.storage.models import Project
from .base import BaseRepository


class ProjectRepository(BaseRepository[Project]):
    """Repository for Project entities."""

    def __init__(self, session: AsyncSession):
        """Initialize project repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, Project)

    async def create_project(
        self,
        tenant_id: uuid.UUID,
        name: str,
        description: Optional[str] = None,
        status: str = 'planning',
        priority: Optional[str] = None,
        parent_id: Optional[int] = None,
        start_date: Optional[datetime] = None,
        end_date: Optional[datetime] = None,
        artifacts: Optional[Dict[str, Any]] = None,
    ) -> Project:
        """Create a new project with validation.

        Args:
            tenant_id: Tenant ID
            name: Project name
            description: Project description
            status: Project status
            priority: Project priority
            parent_id: Parent project ID for hierarchy
            start_date: Project start date
            end_date: Project end date
            artifacts: Project artifacts and metadata

        Returns:
            Created project instance
        """
        return await self.create(
            tenant_id=tenant_id,
            name=name,
            description=description,
            status=status,
            priority=priority,
            parent_id=parent_id,
            start_date=start_date,
            end_date=end_date,
            artifacts=artifacts,
        )

    async def find_by_name(
        self, tenant_id: uuid.UUID, name: str
    ) -> Optional[Project]:
        """Find project by name.

        Args:
            tenant_id: Tenant ID
            name: Project name

        Returns:
            Project instance or None if not found
        """
        return await self.find_one_by(
            tenant_id=tenant_id,
            name=name
        )

    async def find_by_status(
        self, tenant_id: uuid.UUID, status: str
    ) -> List[Project]:
        """Find projects by status.

        Args:
            tenant_id: Tenant ID
            status: Project status

        Returns:
            List of project instances
        """
        return await self.find_by(
            tenant_id=tenant_id,
            status=status
        )

    async def find_active_projects(self, tenant_id: uuid.UUID) -> List[Project]:
        """Find active projects (not completed, cancelled, or archived).

        Args:
            tenant_id: Tenant ID

        Returns:
            List of active project instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Project.tenant_id == tenant_id,
                    Project.status.in_(['planning', 'active', 'on_hold'])
                )
            )
            .order_by(
                func.case(
                    (Project.priority == 'critical', 0),
                    (Project.priority == 'high', 1),
                    (Project.priority == 'medium', 2),
                    else_=3
                ),
                Project.name
            )
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_priority(
        self, tenant_id: uuid.UUID, priority: str
    ) -> List[Project]:
        """Find projects by priority.

        Args:
            tenant_id: Tenant ID
            priority: Project priority

        Returns:
            List of project instances
        """
        return await self.find_by(
            tenant_id=tenant_id,
            priority=priority
        )

    async def find_children(self, parent_id: int) -> List[Project]:
        """Find child projects.

        Args:
            parent_id: Parent project ID

        Returns:
            List of child project instances
        """
        return await self.find_by(parent_id=parent_id)

    async def find_with_hierarchy(self, project_id: int) -> Optional[Project]:
        """Find project with parent and children loaded.

        Args:
            project_id: Project ID

        Returns:
            Project instance with hierarchy loaded, or None
        """
        stmt = (
            select(Project)
            .options(
                selectinload(Project.parent),
                selectinload(Project.children)
            )
            .where(Project.id == project_id)
        )

        if hasattr(Project, 'is_deleted'):
            stmt = stmt.where(Project.is_deleted.is_(False))

        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def find_root_projects(self, tenant_id: uuid.UUID) -> List[Project]:
        """Find root projects (no parent).

        Args:
            tenant_id: Tenant ID

        Returns:
            List of root project instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Project.tenant_id == tenant_id,
                    Project.parent_id.is_(None)
                )
            )
            .order_by(Project.name)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_date_range(
        self,
        tenant_id: uuid.UUID,
        start_date: Optional[datetime] = None,
        end_date: Optional[datetime] = None
    ) -> List[Project]:
        """Find projects by date range.

        Args:
            tenant_id: Tenant ID
            start_date: Filter by start_date >= this value
            end_date: Filter by end_date <= this value

        Returns:
            List of project instances
        """
        stmt = self._build_query().where(Project.tenant_id == tenant_id)

        if start_date:
            stmt = stmt.where(
                or_(
                    Project.start_date >= start_date,
                    Project.start_date.is_(None)
                )
            )
        if end_date:
            stmt = stmt.where(
                or_(
                    Project.end_date <= end_date,
                    Project.end_date.is_(None)
                )
            )

        stmt = stmt.order_by(
            Project.start_date.desc().nulls_last(),
            Project.name
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def search_projects(
        self, tenant_id: uuid.UUID, search_term: str, limit: int = 50
    ) -> List[Project]:
        """Search projects by name and description.

        Args:
            tenant_id: Tenant ID
            search_term: Search term
            limit: Maximum results to return

        Returns:
            List of matching project instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Project.tenant_id == tenant_id,
                    or_(
                        Project.name.ilike(f'%{search_term}%'),
                        Project.description.ilike(f'%{search_term}%')
                    )
                )
            )
            .order_by(Project.name)
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def update_status(
        self, project_id: int, status: str, status_reason: Optional[str] = None
    ) -> Optional[Project]:
        """Update project status.

        Args:
            project_id: Project ID
            status: New status
            status_reason: Optional reason for status change

        Returns:
            Updated project instance or None if not found
        """
        update_data = {'status': status}

        # Add status change to artifacts
        project = await self.get_by_id(project_id)
        if project:
            artifacts = project.artifacts or {}
            status_history = artifacts.get('status_history', [])
            status_history.append({
                'status': status,
                'reason': status_reason,
                'timestamp': datetime.utcnow().isoformat(),
            })
            artifacts['status_history'] = status_history
            update_data['artifacts'] = artifacts

        return await self.update(project_id, **update_data)

    async def add_artifact(
        self, project_id: int, artifact_key: str, artifact_data: Any
    ) -> Optional[Project]:
        """Add or update project artifact.

        Args:
            project_id: Project ID
            artifact_key: Artifact key
            artifact_data: Artifact data

        Returns:
            Updated project instance or None if not found
        """
        project = await self.get_by_id(project_id)
        if not project:
            return None

        artifacts = project.artifacts or {}
        artifacts[artifact_key] = artifact_data

        return await self.update_entity(project, artifacts=artifacts)

    async def get_project_hierarchy(self, project_id: int) -> List[Project]:
        """Get full project hierarchy (ancestors and descendants).

        Args:
            project_id: Root project ID

        Returns:
            List of all related projects in hierarchy
        """
        # This would ideally use recursive CTEs in PostgreSQL
        # For now, implement simpler version
        project = await self.get_by_id(project_id)
        if not project:
            return []

        hierarchy = [project]

        # Get children recursively
        async def get_all_children(parent_id: int, collected: List[Project]):
            children = await self.find_children(parent_id)
            for child in children:
                collected.append(child)
                await get_all_children(child.id, collected)

        await get_all_children(project.id, hierarchy)

        # Get ancestors
        current = project
        while current.parent_id:
            parent = await self.get_by_id(current.parent_id)
            if parent:
                hierarchy.insert(0, parent)
                current = parent
            else:
                break

        return hierarchy

    async def get_stats_by_status(self, tenant_id: uuid.UUID) -> Dict[str, int]:
        """Get project count statistics by status.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary mapping status to counts
        """
        stmt = (
            select(Project.status, func.count(Project.id))
            .where(
                and_(
                    Project.tenant_id == tenant_id,
                    Project.is_deleted.is_(False)
                )
            )
            .group_by(Project.status)
        )

        result = await self.session.execute(stmt)
        return dict(result.all())

    async def get_stats_by_priority(self, tenant_id: uuid.UUID) -> Dict[str, int]:
        """Get project count statistics by priority.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary mapping priority to counts
        """
        stmt = (
            select(
                func.coalesce(Project.priority, 'unset').label('priority'),
                func.count(Project.id)
            )
            .where(
                and_(
                    Project.tenant_id == tenant_id,
                    Project.is_deleted.is_(False)
                )
            )
            .group_by('priority')
        )

        result = await self.session.execute(stmt)
        return dict(result.all())

    async def find_overdue_projects(self, tenant_id: uuid.UUID) -> List[Project]:
        """Find projects that are overdue (end_date < now and still active).

        Args:
            tenant_id: Tenant ID

        Returns:
            List of overdue project instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Project.tenant_id == tenant_id,
                    Project.end_date < datetime.utcnow(),
                    Project.status.in_(['planning', 'active', 'on_hold'])
                )
            )
            .order_by(Project.end_date.desc())
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())