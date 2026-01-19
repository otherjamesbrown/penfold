"""Data models for AI Coordination framework.

Defines the key entities for coordination decisions, quality validation,
model profiles, and performance tracking.
"""

from datetime import datetime, timezone
from typing import Dict, List, Optional, Any
from dataclasses import dataclass, field
from enum import Enum

from sqlalchemy import Column, String, Float, Integer, Boolean, DateTime, Text, JSON, ForeignKey, Index
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy.orm import relationship
import uuid

from ..storage.models import Base, TimestampMixin, TenantMixin


class ModelCapability(Enum):
    """Types of AI model capabilities."""
    TEXT_GENERATION = "text_generation"
    SUMMARIZATION = "summarization"
    ENTITY_EXTRACTION = "entity_extraction"
    SENTIMENT_ANALYSIS = "sentiment_analysis"
    CLASSIFICATION = "classification"
    TRANSLATION = "translation"
    CODE_GENERATION = "code_generation"
    QUESTION_ANSWERING = "question_answering"


class ModelType(Enum):
    """Types of AI models by deployment location."""
    LOCAL = "local"
    CLOUD = "cloud"
    HYBRID = "hybrid"


@dataclass
class ModelProfile:
    """Profile describing AI model capabilities and characteristics."""
    model_id: str
    name: str
    model_type: ModelType
    capabilities: List[ModelCapability]
    supported_content_types: List[str]
    max_input_tokens: int
    avg_response_time_ms: float
    cost_per_1k_tokens: float
    confidence_reliability: float  # How reliable the model's confidence scores are
    priority: int = 2  # Lower number = higher priority
    enabled: bool = True
    metadata: Dict[str, Any] = field(default_factory=dict)

    def supports_capability(self, capability: ModelCapability) -> bool:
        """Check if model supports a specific capability."""
        return capability in self.capabilities

    def supports_content_type(self, content_type: str) -> bool:
        """Check if model supports a content type."""
        return content_type in self.supported_content_types

    def estimate_cost(self, input_tokens: int) -> float:
        """Estimate processing cost for given input size."""
        return (input_tokens / 1000.0) * self.cost_per_1k_tokens


class CoordinationDecision(Base, TimestampMixin, TenantMixin):
    """Record of coordination decisions for analysis and learning.

    Tracks how models were selected, weighted, and combined for processing tasks,
    enabling learning and optimization of coordination strategies.
    """
    __tablename__ = "coordination_decisions"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    coordination_id = Column(String(255), nullable=False, index=True)
    content_id = Column(String(255), nullable=False)
    content_type = Column(String(100), nullable=False)

    # Decision details
    selected_models = Column(JSONB, nullable=False)  # List of model IDs
    selection_reasoning = Column(Text)
    ensemble_strategy = Column(String(50))  # weighted_average, voting, etc.
    estimated_cost = Column(Float)
    expected_quality = Column(Float)

    # Results
    actual_cost = Column(Float)
    actual_quality = Column(Float)
    processing_time_ms = Column(Integer)
    success = Column(Boolean, default=True)

    # Performance metrics
    model_contributions = Column(JSONB)  # Model ID -> contribution weight
    confidence_scores = Column(JSONB)   # Model ID -> confidence score
    ensemble_confidence = Column(Float)
    quality_improvement = Column(Float)  # vs best individual model

    # Learning data
    human_feedback = Column(JSONB)  # Structured feedback for learning
    effectiveness_score = Column(Float)  # Calculated effectiveness
    lessons_learned = Column(JSONB)  # Structured insights

    __table_args__ = (
        Index("idx_coordination_content_type", "tenant_id", "content_type", "created_at"),
        Index("idx_coordination_models", "tenant_id", "selected_models"),
        Index("idx_coordination_effectiveness", "effectiveness_score", "created_at"),
    )


class QualityValidation(Base, TimestampMixin, TenantMixin):
    """Human feedback on AI coordination decisions for learning.

    Captures human validation of coordination decisions to improve
    future model selection and ensemble strategies.
    """
    __tablename__ = "quality_validations"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    coordination_decision_id = Column(UUID(as_uuid=True), ForeignKey("coordination_decisions.id"), nullable=False)
    validator_id = Column(String(100), nullable=False)

    # Validation details
    overall_rating = Column(Integer)  # 1-5 scale
    model_selection_rating = Column(Integer)  # Was model selection appropriate?
    ensemble_quality_rating = Column(Integer)  # Was ensemble combination good?
    cost_effectiveness_rating = Column(Integer)  # Worth the cost?

    # Specific feedback
    preferred_models = Column(JSONB)  # Which models should have been used
    improvement_suggestions = Column(Text)
    result_quality_issues = Column(JSONB)  # Structured quality problems
    escalation_appropriateness = Column(Integer)  # Was escalation decision right?

    # Context
    validation_context = Column(JSONB)  # Why this was reviewed
    confidence_in_feedback = Column(Float)  # How confident is the validator

    # Relationship
    coordination_decision = relationship("CoordinationDecision", backref="quality_validations")

    __table_args__ = (
        Index("idx_quality_validation_decision", "coordination_decision_id"),
        Index("idx_quality_validation_validator", "tenant_id", "validator_id", "created_at"),
    )


class ModelPerformanceRecord(Base, TimestampMixin):
    """Individual performance record for model tracking.

    Records each processing session's performance for learning and optimization.
    """
    __tablename__ = "model_performance_records"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    model_id = Column(String(255), nullable=False, index=True)
    content_type = Column(String(100), nullable=False)

    # Performance data
    processing_time_ms = Column(Float)
    confidence_score = Column(Float)
    success = Column(Boolean, nullable=False)
    token_count = Column(Integer)
    actual_cost = Column(Float)

    # Context
    content_complexity_score = Column(Float)
    content_length = Column(Integer)
    escalation_reason = Column(String(100))  # If this was an escalation
    coordination_id = Column(String(255))  # Link to coordination

    # Additional metrics
    additional_metrics = Column(JSONB)  # Model-specific performance data
    timestamp = Column(DateTime(timezone=True), default=lambda: datetime.now(timezone.utc), nullable=False)

    __table_args__ = (
        Index("idx_performance_model_content", "model_id", "content_type", "timestamp"),
        Index("idx_performance_success", "model_id", "success", "timestamp"),
        Index("idx_performance_coordination", "coordination_id"),
    )


@dataclass
class EscalationMetrics:
    """Metrics for evaluating escalation effectiveness."""
    escalation_id: str
    trigger_reason: str
    local_confidence: float
    cloud_confidence: float
    quality_improvement: float
    cost_incurred: float
    cost_per_improvement: float
    processing_time_increase_ms: int
    escalation_worthwhile: bool
    timestamp: datetime = field(default_factory=lambda: datetime.now(timezone.utc))


@dataclass
class CoordinationSummary:
    """Summary of a coordination workflow for reporting."""
    coordination_id: str
    content_id: str
    content_type: str
    models_used: List[str]
    processing_strategy: str
    total_cost: float
    best_individual_confidence: float
    ensemble_confidence: float
    quality_improvement: float
    processing_time_ms: int
    success_rate: float
    escalation_triggered: bool
    human_feedback_received: bool
    effectiveness_score: Optional[float]
    created_at: datetime
    completed_at: Optional[datetime]


# Default model profiles for common models
DEFAULT_MODEL_PROFILES = {
    "llama-3.1-8b": ModelProfile(
        model_id="llama-3.1-8b",
        name="Llama 3.1 8B (Local)",
        model_type=ModelType.LOCAL,
        capabilities=[
            ModelCapability.TEXT_GENERATION,
            ModelCapability.SUMMARIZATION,
            ModelCapability.ENTITY_EXTRACTION,
            ModelCapability.CLASSIFICATION
        ],
        supported_content_types=["email", "document", "meeting", "text"],
        max_input_tokens=8192,
        avg_response_time_ms=2000,
        cost_per_1k_tokens=0.0,  # Local model
        confidence_reliability=0.7,
        priority=2
    ),

    "gpt-4": ModelProfile(
        model_id="gpt-4",
        name="GPT-4 (Cloud)",
        model_type=ModelType.CLOUD,
        capabilities=[
            ModelCapability.TEXT_GENERATION,
            ModelCapability.SUMMARIZATION,
            ModelCapability.ENTITY_EXTRACTION,
            ModelCapability.SENTIMENT_ANALYSIS,
            ModelCapability.CLASSIFICATION,
            ModelCapability.QUESTION_ANSWERING
        ],
        supported_content_types=["email", "document", "meeting", "text", "code"],
        max_input_tokens=8192,
        avg_response_time_ms=5000,
        cost_per_1k_tokens=0.03,
        confidence_reliability=0.9,
        priority=1
    ),

    "claude-3-sonnet": ModelProfile(
        model_id="claude-3-sonnet",
        name="Claude 3 Sonnet (Cloud)",
        model_type=ModelType.CLOUD,
        capabilities=[
            ModelCapability.TEXT_GENERATION,
            ModelCapability.SUMMARIZATION,
            ModelCapability.ENTITY_EXTRACTION,
            ModelCapability.SENTIMENT_ANALYSIS,
            ModelCapability.CLASSIFICATION,
            ModelCapability.CODE_GENERATION
        ],
        supported_content_types=["email", "document", "meeting", "text", "code"],
        max_input_tokens=200000,
        avg_response_time_ms=4000,
        cost_per_1k_tokens=0.003,
        confidence_reliability=0.85,
        priority=1
    ),

    "gemini-pro": ModelProfile(
        model_id="gemini-pro",
        name="Gemini Pro (Cloud)",
        model_type=ModelType.CLOUD,
        capabilities=[
            ModelCapability.TEXT_GENERATION,
            ModelCapability.SUMMARIZATION,
            ModelCapability.ENTITY_EXTRACTION,
            ModelCapability.SENTIMENT_ANALYSIS,
            ModelCapability.CLASSIFICATION
        ],
        supported_content_types=["email", "document", "meeting", "text"],
        max_input_tokens=30720,
        avg_response_time_ms=3000,
        cost_per_1k_tokens=0.0005,
        confidence_reliability=0.8,
        priority=1
    )
}


def get_default_model_profile(model_id: str) -> Optional[ModelProfile]:
    """Get default profile for a known model ID."""
    return DEFAULT_MODEL_PROFILES.get(model_id)


def create_custom_model_profile(
    model_id: str,
    name: str,
    model_type: ModelType,
    capabilities: List[str],
    content_types: List[str],
    **kwargs
) -> ModelProfile:
    """Create a custom model profile with validation."""
    # Convert string capabilities to enum
    capability_enums = []
    for cap in capabilities:
        try:
            capability_enums.append(ModelCapability(cap))
        except ValueError:
            logger.warning(f"Unknown capability: {cap}")

    return ModelProfile(
        model_id=model_id,
        name=name,
        model_type=model_type,
        capabilities=capability_enums,
        supported_content_types=content_types,
        max_input_tokens=kwargs.get("max_input_tokens", 4096),
        avg_response_time_ms=kwargs.get("avg_response_time_ms", 2000),
        cost_per_1k_tokens=kwargs.get("cost_per_1k_tokens", 0.0),
        confidence_reliability=kwargs.get("confidence_reliability", 0.7),
        priority=kwargs.get("priority", 2),
        enabled=kwargs.get("enabled", True),
        metadata=kwargs.get("metadata", {})
    )