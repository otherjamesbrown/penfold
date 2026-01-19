"""Unit tests for repository layer."""

import pytest
import uuid
from datetime import datetime, timedelta
from decimal import Decimal
from typing import AsyncGenerator

from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.pool import StaticPool
from sqlalchemy.orm import sessionmaker

from penf_lib.storage.models import Base, Source, Assertion, Person, Project, Team
from penf_lib.storage.repositories import (
    SourceRepository,
    AssertionRepository,
    PersonRepository,
    ProjectRepository,
    TeamRepository,
)


@pytest.fixture
async def engine():
    """Create test database engine."""
    engine = create_async_engine(
        "sqlite+aiosqlite:///:memory:",
        poolclass=StaticPool,
        connect_args={"check_same_thread": False},
    )

    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    yield engine
    await engine.dispose()


@pytest.fixture
async def session(engine) -> AsyncGenerator[AsyncSession, None]:
    """Create test database session."""
    async_session = sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)

    async with async_session() as session:
        yield session


@pytest.fixture
def tenant_id() -> uuid.UUID:
    """Test tenant ID."""
    return uuid.uuid4()


class TestSourceRepository:
    """Test SourceRepository operations."""

    @pytest.fixture
    async def repository(self, session):
        """Create source repository."""
        return SourceRepository(session)

    async def test_create_source(self, repository, session, tenant_id):
        """Test creating a source."""
        source = await repository.create_source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="msg_123",
            raw_content="Test email content",
            content_type="text/plain",
            source_timestamp=datetime.utcnow(),
        )

        assert source.id is not None
        assert source.tenant_id == tenant_id
        assert source.source_system == "gmail"
        assert source.external_id == "msg_123"
        assert source.raw_content == "Test email content"
        assert source.content_hash != ""
        assert source.content_size == len("Test email content")

    async def test_find_by_external_id(self, repository, session, tenant_id):
        """Test finding source by external ID."""
        # Create source
        created = await repository.create_source(
            tenant_id=tenant_id,
            source_system="slack",
            external_id="channel_456",
            raw_content="Slack message",
        )
        await session.commit()

        # Find by external ID
        found = await repository.find_by_external_id(
            tenant_id=tenant_id,
            source_system="slack",
            external_id="channel_456"
        )

        assert found is not None
        assert found.id == created.id
        assert found.external_id == "channel_456"

    async def test_find_by_content_hash(self, repository, session, tenant_id):
        """Test finding source by content hash."""
        content = "Unique content for hash test"
        source = await repository.create_source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="msg_hash",
            raw_content=content,
        )
        await session.commit()

        found = await repository.find_by_content_hash(
            tenant_id=tenant_id,
            content_hash=source.content_hash
        )

        assert found is not None
        assert found.id == source.id

    async def test_update_processing_status(self, repository, session, tenant_id):
        """Test updating processing status."""
        source = await repository.create_source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="msg_status",
            raw_content="Test content",
        )
        await session.commit()

        # Update status
        updated = await repository.update_processing_status(
            source.id,
            "completed",
            {"processed_at": datetime.utcnow().isoformat()}
        )

        assert updated is not None
        assert updated.processing_status == "completed"
        assert "processed_at" in updated.ingestion_metadata

    async def test_soft_delete(self, repository, session, tenant_id):
        """Test soft delete functionality."""
        source = await repository.create_source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="msg_delete",
            raw_content="To be deleted",
        )
        await session.commit()

        # Soft delete
        deleted = await repository.delete(source.id, hard_delete=False)
        assert deleted is True

        # Should not be found in normal queries
        found = await repository.get_by_id(source.id)
        assert found is None

        # Should be found when including deleted
        all_sources = await repository.list_all(include_deleted=True)
        deleted_source = next((s for s in all_sources if s.id == source.id), None)
        assert deleted_source is not None
        assert deleted_source.is_deleted is True


class TestAssertionRepository:
    """Test AssertionRepository operations."""

    @pytest.fixture
    async def repository(self, session):
        """Create assertion repository."""
        return AssertionRepository(session)

    @pytest.fixture
    async def source_repo(self, session):
        """Create source repository for test dependencies."""
        return SourceRepository(session)

    async def test_create_assertion(self, repository, source_repo, session, tenant_id):
        """Test creating an assertion."""
        # Create source first
        source = await source_repo.create_source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="msg_assertion",
            raw_content="Email about project decision",
        )
        await session.commit()

        # Create assertion
        assertion = await repository.create_assertion(
            tenant_id=tenant_id,
            source_id=source.id,
            assertion_type="decision",
            content="We decided to use PostgreSQL for the database",
            confidence_score=Decimal('0.85'),
            extracted_entities={"technology": "PostgreSQL"},
        )

        assert assertion.id is not None
        assert assertion.tenant_id == tenant_id
        assert assertion.source_id == source.id
        assert assertion.assertion_type == "decision"
        assert assertion.confidence_score == Decimal('0.85')

    async def test_find_by_type(self, repository, source_repo, session, tenant_id):
        """Test finding assertions by type."""
        # Create source and assertions
        source = await source_repo.create_source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="msg_types",
            raw_content="Meeting notes",
        )

        decision1 = await repository.create_assertion(
            tenant_id=tenant_id,
            source_id=source.id,
            assertion_type="decision",
            content="Decision 1",
        )

        risk1 = await repository.create_assertion(
            tenant_id=tenant_id,
            source_id=source.id,
            assertion_type="risk",
            content="Risk 1",
        )

        await session.commit()

        # Find by type
        decisions = await repository.find_by_type(tenant_id, "decision")
        risks = await repository.find_by_type(tenant_id, "risk")

        assert len(decisions) == 1
        assert decisions[0].assertion_type == "decision"
        assert len(risks) == 1
        assert risks[0].assertion_type == "risk"

    async def test_find_high_confidence(self, repository, source_repo, session, tenant_id):
        """Test finding high-confidence assertions."""
        source = await source_repo.create_source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="msg_confidence",
            raw_content="Confidence test",
        )

        # Create assertions with different confidence scores
        low_conf = await repository.create_assertion(
            tenant_id=tenant_id,
            source_id=source.id,
            assertion_type="decision",
            content="Low confidence",
            confidence_score=Decimal('0.3'),
        )

        high_conf = await repository.create_assertion(
            tenant_id=tenant_id,
            source_id=source.id,
            assertion_type="decision",
            content="High confidence",
            confidence_score=Decimal('0.9'),
        )

        await session.commit()

        # Find high confidence (>= 0.8)
        high_confidence_assertions = await repository.find_high_confidence(
            tenant_id, Decimal('0.8')
        )

        assert len(high_confidence_assertions) == 1
        assert high_confidence_assertions[0].id == high_conf.id


class TestPersonRepository:
    """Test PersonRepository operations."""

    @pytest.fixture
    async def repository(self, session):
        """Create person repository."""
        return PersonRepository(session)

    async def test_create_person(self, repository, session, tenant_id):
        """Test creating a person."""
        person = await repository.create_person(
            tenant_id=tenant_id,
            canonical_name="John Doe",
            email_addresses=["john.doe@example.com", "j.doe@company.com"],
            phone_numbers=["+1234567890"],
            aliases=["Johnny", "John D"],
        )

        assert person.id is not None
        assert person.canonical_name == "John Doe"
        assert len(person.email_addresses) == 2
        assert "john.doe@example.com" in person.email_addresses

    async def test_find_by_email(self, repository, session, tenant_id):
        """Test finding person by email."""
        person = await repository.create_person(
            tenant_id=tenant_id,
            canonical_name="Jane Smith",
            email_addresses=["jane.smith@example.com"],
        )
        await session.commit()

        found = await repository.find_by_email(
            tenant_id, "jane.smith@example.com"
        )

        assert found is not None
        assert found.id == person.id
        assert found.canonical_name == "Jane Smith"

    async def test_add_email(self, repository, session, tenant_id):
        """Test adding email to person."""
        person = await repository.create_person(
            tenant_id=tenant_id,
            canonical_name="Bob Wilson",
            email_addresses=["bob@example.com"],
        )
        await session.commit()

        # Add new email
        updated = await repository.add_email(person.id, "bob.wilson@company.com")

        assert updated is not None
        assert len(updated.email_addresses) == 2
        assert "bob.wilson@company.com" in updated.email_addresses

    async def test_merge_persons(self, repository, session, tenant_id):
        """Test merging two person records."""
        person1 = await repository.create_person(
            tenant_id=tenant_id,
            canonical_name="Alice Johnson",
            email_addresses=["alice@example.com"],
        )

        person2 = await repository.create_person(
            tenant_id=tenant_id,
            canonical_name="A. Johnson",
            email_addresses=["a.johnson@company.com"],
        )
        await session.commit()

        # Merge person2 into person1
        merged = await repository.merge_persons(person1.id, person2.id)

        assert merged is not None
        assert merged.id == person1.id
        assert len(merged.email_addresses) == 2
        assert "A. Johnson" in merged.aliases

        # Person2 should be soft-deleted
        person2_check = await repository.get_by_id(person2.id)
        assert person2_check is None


class TestProjectRepository:
    """Test ProjectRepository operations."""

    @pytest.fixture
    async def repository(self, session):
        """Create project repository."""
        return ProjectRepository(session)

    async def test_create_project(self, repository, session, tenant_id):
        """Test creating a project."""
        project = await repository.create_project(
            tenant_id=tenant_id,
            name="Website Redesign",
            description="Redesign company website",
            status="planning",
            priority="high",
            start_date=datetime.utcnow(),
            end_date=datetime.utcnow() + timedelta(days=90),
        )

        assert project.id is not None
        assert project.name == "Website Redesign"
        assert project.status == "planning"
        assert project.priority == "high"

    async def test_find_active_projects(self, repository, session, tenant_id):
        """Test finding active projects."""
        # Create projects with different statuses
        active1 = await repository.create_project(
            tenant_id=tenant_id,
            name="Active Project 1",
            status="active",
            priority="critical",
        )

        completed = await repository.create_project(
            tenant_id=tenant_id,
            name="Completed Project",
            status="completed",
        )

        active2 = await repository.create_project(
            tenant_id=tenant_id,
            name="Active Project 2",
            status="planning",
            priority="medium",
        )

        await session.commit()

        # Find active projects
        active_projects = await repository.find_active_projects(tenant_id)

        assert len(active_projects) == 2
        project_names = [p.name for p in active_projects]
        assert "Active Project 1" in project_names
        assert "Active Project 2" in project_names
        assert "Completed Project" not in project_names

        # Should be ordered by priority (critical first)
        assert active_projects[0].priority == "critical"

    async def test_project_hierarchy(self, repository, session, tenant_id):
        """Test project hierarchy operations."""
        # Create parent project
        parent = await repository.create_project(
            tenant_id=tenant_id,
            name="Parent Project",
            status="active",
        )

        # Create child projects
        child1 = await repository.create_project(
            tenant_id=tenant_id,
            name="Child Project 1",
            parent_id=parent.id,
            status="planning",
        )

        child2 = await repository.create_project(
            tenant_id=tenant_id,
            name="Child Project 2",
            parent_id=parent.id,
            status="active",
        )

        await session.commit()

        # Find children
        children = await repository.find_children(parent.id)
        assert len(children) == 2

        # Find root projects
        root_projects = await repository.find_root_projects(tenant_id)
        assert len(root_projects) == 1
        assert root_projects[0].id == parent.id

    async def test_update_status(self, repository, session, tenant_id):
        """Test updating project status with history."""
        project = await repository.create_project(
            tenant_id=tenant_id,
            name="Status Test Project",
            status="planning",
        )
        await session.commit()

        # Update status
        updated = await repository.update_status(
            project.id, "active", "All planning completed"
        )

        assert updated is not None
        assert updated.status == "active"
        assert updated.artifacts is not None
        assert "status_history" in updated.artifacts
        assert len(updated.artifacts["status_history"]) == 1


class TestTeamRepository:
    """Test TeamRepository operations."""

    @pytest.fixture
    async def repository(self, session):
        """Create team repository."""
        return TeamRepository(session)

    async def test_create_team(self, repository, session, tenant_id):
        """Test creating a team."""
        team = await repository.create_team(
            tenant_id=tenant_id,
            name="Engineering Team",
            team_type="department",
            description="Software engineering department",
            member_emails=["alice@example.com", "bob@example.com"],
        )

        assert team.id is not None
        assert team.name == "Engineering Team"
        assert team.team_type == "department"
        assert len(team.member_emails) == 2

    async def test_add_remove_members(self, repository, session, tenant_id):
        """Test adding and removing team members."""
        team = await repository.create_team(
            tenant_id=tenant_id,
            name="Project Team",
            member_emails=["alice@example.com"],
        )
        await session.commit()

        # Add member
        updated = await repository.add_member(team.id, "bob@example.com")
        assert len(updated.member_emails) == 2

        # Remove member
        updated = await repository.remove_member(team.id, "alice@example.com")
        assert len(updated.member_emails) == 1
        assert "bob@example.com" in updated.member_emails

    async def test_find_by_member_email(self, repository, session, tenant_id):
        """Test finding teams by member email."""
        team1 = await repository.create_team(
            tenant_id=tenant_id,
            name="Team Alpha",
            member_emails=["alice@example.com", "bob@example.com"],
        )

        team2 = await repository.create_team(
            tenant_id=tenant_id,
            name="Team Beta",
            member_emails=["alice@example.com", "charlie@example.com"],
        )

        await session.commit()

        # Find teams with Alice as member
        alice_teams = await repository.find_by_member_email(
            tenant_id, "alice@example.com"
        )

        assert len(alice_teams) == 2
        team_names = [t.name for t in alice_teams]
        assert "Team Alpha" in team_names
        assert "Team Beta" in team_names

        # Find teams with Charlie as member
        charlie_teams = await repository.find_by_member_email(
            tenant_id, "charlie@example.com"
        )

        assert len(charlie_teams) == 1
        assert charlie_teams[0].name == "Team Beta"

    async def test_bulk_add_members(self, repository, session, tenant_id):
        """Test bulk adding team members."""
        team = await repository.create_team(
            tenant_id=tenant_id,
            name="Bulk Team",
            member_emails=[],
        )
        await session.commit()

        # Bulk add members
        new_members = ["user1@example.com", "user2@example.com", "user3@example.com"]
        updated = await repository.bulk_add_members(team.id, new_members)

        assert len(updated.member_emails) == 3
        for email in new_members:
            assert email in updated.member_emails

    async def test_find_empty_teams(self, repository, session, tenant_id):
        """Test finding teams with no members."""
        empty_team = await repository.create_team(
            tenant_id=tenant_id,
            name="Empty Team",
            member_emails=[],
        )

        full_team = await repository.create_team(
            tenant_id=tenant_id,
            name="Full Team",
            member_emails=["user@example.com"],
        )

        await session.commit()

        empty_teams = await repository.find_empty_teams(tenant_id)

        assert len(empty_teams) == 1
        assert empty_teams[0].id == empty_team.id