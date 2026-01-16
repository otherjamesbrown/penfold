"""Repository layer for database operations."""

from .base import BaseRepository
from .source import SourceRepository
from .assertion import AssertionRepository
from .person import PersonRepository
from .project import ProjectRepository
from .team import TeamRepository
from .tenant import TenantRepository, TenantSessionRepository, CrossTenantPersonLinkRepository, UserTenantMembershipRepository
from .relationship import RelationshipRepository
from .review import ReviewRepository
from .search import SearchRepository
from .embedding import EmbeddingRepository
from .ingest import IngestJobRepository, IngestErrorRepository, EmailAttachmentRepository, ArchivedFileRepository

__all__ = [
    "BaseRepository",
    "SourceRepository",
    "AssertionRepository",
    "PersonRepository",
    "ProjectRepository",
    "TeamRepository",
    "TenantRepository",
    "TenantSessionRepository",
    "CrossTenantPersonLinkRepository",
    "UserTenantMembershipRepository",
    "RelationshipRepository",
    "ReviewRepository",
    "SearchRepository",
    "EmbeddingRepository",
    "IngestJobRepository",
    "IngestErrorRepository",
    "EmailAttachmentRepository",
    "ArchivedFileRepository",
]
