"""Base classes for workflow orchestration."""

import logging
import time
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)


@dataclass
class SourceCitation:
    """Citation for tracking information sources in briefings."""

    source_id: int
    source_type: str
    title: str
    timestamp: Optional[datetime] = None
    relevance_score: float = 0.0
    excerpt: Optional[str] = None


@dataclass
class WorkflowResult:
    """Base result class for workflow executions."""

    success: bool
    execution_time_ms: float
    sources_consulted: int = 0
    citations: List[SourceCitation] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)
    metadata: Dict[str, Any] = field(default_factory=dict)


class WorkflowBase(ABC):
    """Abstract base class for workflow orchestration."""

    def __init__(self, session, tenant_id: str, config: Optional[Dict[str, Any]] = None):
        """Initialize workflow.

        Args:
            session: Database session
            tenant_id: Current tenant ID
            config: Optional workflow configuration
        """
        self.session = session
        self.tenant_id = tenant_id
        self.config = config or {}
        self._citations: List[SourceCitation] = []
        self._start_time: Optional[float] = None

    @abstractmethod
    async def execute(self, *args, **kwargs) -> WorkflowResult:
        """Execute the workflow.

        Subclasses must implement this method.
        """
        pass

    def _start_timing(self) -> None:
        """Start timing for workflow execution."""
        self._start_time = time.perf_counter()

    def _get_elapsed_ms(self) -> float:
        """Get elapsed time in milliseconds."""
        if self._start_time is None:
            return 0.0
        return (time.perf_counter() - self._start_time) * 1000

    def _add_citation(self, citation: SourceCitation) -> None:
        """Add a source citation to the workflow."""
        self._citations.append(citation)

    def _get_citations(self) -> List[SourceCitation]:
        """Get all citations collected during workflow."""
        return self._citations.copy()
