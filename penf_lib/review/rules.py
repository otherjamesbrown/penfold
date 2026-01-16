"""Learning rule management for the Daily Review workflow.

This module provides rule management for automated corrections including:
- Creating and managing learning rules
- Applying rules to review items
- Tracking rule effectiveness
- Managing rule suggestions

Based on User Story 4 (Phase 6) requirements from spec 006-daily-review.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Protocol
from uuid import UUID, uuid4

from penf_lib.review.exceptions import (
    ItemNotFoundError,
    InvalidSessionStateError,
)
from penf_lib.review.models import (
    AISuggestion,
    LearningRuleDTO,
    LearningRuleStatus,
    LearningRuleType,
    ReviewItemDTO,
)


# =============================================================================
# SUPPORTING DATA CLASSES
# =============================================================================


@dataclass
class RuleApplicationResult:
    """Result of applying rules to an item.

    Contains the original and modified suggestions along with
    details about which rules were applied.
    """

    applied_rules: list[LearningRuleDTO]
    original_suggestion: AISuggestion
    modified_suggestion: AISuggestion
    modifications: list[str]


@dataclass
class RuleSuggestion:
    """A suggested rule based on user feedback patterns.

    Represents a rule that has been automatically suggested by
    the learning system based on repeated user corrections.
    """

    id: int
    rule_type: LearningRuleType
    match_criteria: dict[str, Any]
    action: dict[str, Any]
    pattern_count: int
    confidence: Decimal
    created_at: datetime


# =============================================================================
# REPOSITORY PROTOCOL
# =============================================================================


class RuleRepositoryProtocol(Protocol):
    """Protocol for rule repository dependency injection.

    Defines the interface that any repository implementation must satisfy
    to work with the RuleManager.
    """

    async def get_rules_for_tenant(
        self,
        tenant_id: UUID,
        status: str | None,
    ) -> list[LearningRuleDTO]:
        """Get rules for a tenant with optional status filter.

        Args:
            tenant_id: Tenant isolation identifier
            status: Optional status filter (active, disabled, all)

        Returns:
            List of learning rule DTOs
        """
        ...

    async def get_rule_by_id(self, rule_id: int) -> LearningRuleDTO | None:
        """Get rule by ID.

        Args:
            rule_id: Rule identifier

        Returns:
            Learning rule DTO or None if not found
        """
        ...

    async def create_rule(self, rule_data: dict[str, Any]) -> LearningRuleDTO:
        """Create a new learning rule.

        Args:
            rule_data: Rule data dictionary

        Returns:
            Created learning rule DTO
        """
        ...

    async def update_rule(self, rule: LearningRuleDTO) -> LearningRuleDTO:
        """Update an existing learning rule.

        Args:
            rule: Learning rule DTO with updated values

        Returns:
            Updated learning rule DTO
        """
        ...

    async def get_pending_suggestions(
        self,
        tenant_id: UUID,
    ) -> list[LearningRuleDTO]:
        """Get suggested rules pending user approval.

        Args:
            tenant_id: Tenant isolation identifier

        Returns:
            List of pending learning rule suggestions
        """
        ...


# =============================================================================
# RULE MANAGER
# =============================================================================


class RuleManager:
    """Manages learning rules for automated corrections.

    The RuleManager is responsible for:
    - Creating and managing learning rules
    - Applying matching rules to review items
    - Tracking rule effectiveness based on user outcomes
    - Managing rule suggestions from the learning system

    Rule Types:
    - SENDER_MATCH: Match by sender email pattern
    - CONTENT_PATTERN: Match by content keywords/regex
    - CATEGORY_OVERRIDE: Override AI category suggestions
    - PARTICIPANT_MAPPING: Map participants to roles
    """

    # Default confidence threshold for rule application
    DEFAULT_CONFIDENCE_THRESHOLD = Decimal("0.800")

    # Default priority (lower = higher priority)
    DEFAULT_PRIORITY = 100

    def __init__(self, repository: RuleRepositoryProtocol) -> None:
        """Initialize the RuleManager.

        Args:
            repository: Repository implementing RuleRepositoryProtocol
        """
        self._repository = repository

    async def list_rules(
        self,
        tenant_id: UUID,
        status: str | None = None,
    ) -> list[LearningRuleDTO]:
        """List learning rules for tenant.

        Args:
            tenant_id: Tenant isolation identifier
            status: Filter by status (active, disabled, all)
                   If None or 'all', returns all rules

        Returns:
            List of learning rule DTOs matching the criteria
        """
        return await self._repository.get_rules_for_tenant(tenant_id, status)

    async def get_rule(
        self,
        rule_id: int,
    ) -> LearningRuleDTO | None:
        """Get rule by ID.

        Args:
            rule_id: Rule identifier

        Returns:
            Learning rule DTO or None if not found
        """
        return await self._repository.get_rule_by_id(rule_id)

    async def create_rule(
        self,
        tenant_id: UUID,
        rule_type: LearningRuleType,
        match_criteria: dict[str, Any],
        action: dict[str, Any],
        name: str | None = None,
        description: str | None = None,
        confidence_threshold: Decimal = Decimal("0.800"),
        priority: int = 100,
        created_by: str = "system",
    ) -> LearningRuleDTO:
        """Create a new learning rule.

        Args:
            tenant_id: Tenant isolation identifier
            rule_type: Type of rule (sender_match, content_pattern, etc.)
            match_criteria: Conditions for rule matching
            action: Action to apply when matched
            name: Human-readable rule name (auto-generated if not provided)
            description: Optional rule description
            confidence_threshold: Minimum confidence to apply rule
            priority: Priority (lower = higher priority, 1-1000)
            created_by: User or system that created the rule

        Returns:
            Created learning rule DTO

        Examples:
            # Sender match rule
            await manager.create_rule(
                tenant_id=tenant_id,
                rule_type=LearningRuleType.SENDER_MATCH,
                match_criteria={"sender": "newsletter@company.com"},
                action={"category": "newsletters/company"}
            )

            # Content pattern rule
            await manager.create_rule(
                tenant_id=tenant_id,
                rule_type=LearningRuleType.CONTENT_PATTERN,
                match_criteria={"keywords": ["budget", "quarterly"]},
                action={"category": "finance/reports"}
            )
        """
        now = datetime.now(timezone.utc)

        # Auto-generate name if not provided
        if name is None:
            name = self._generate_rule_name(rule_type, match_criteria)

        rule_data = {
            "rule_uuid": uuid4(),
            "tenant_id": tenant_id,
            "name": name,
            "description": description,
            "status": LearningRuleStatus.ACTIVE,
            "rule_type": rule_type,
            "match_criteria": match_criteria,
            "action": action,
            "confidence_threshold": confidence_threshold,
            "priority": max(1, min(1000, priority)),  # Clamp to valid range
            "derived_from_count": 0,
            "times_applied": 0,
            "times_overridden": 0,
            "effectiveness_score": None,
            "last_applied_at": None,
            "expires_at": None,
            "created_by": created_by,
            "created_at": now,
            "updated_at": now,
            "is_deleted": False,
            "deleted_at": None,
        }

        return await self._repository.create_rule(rule_data)

    async def enable_rule(self, rule_id: int) -> LearningRuleDTO:
        """Enable a disabled rule.

        Args:
            rule_id: Rule identifier

        Returns:
            Updated learning rule DTO with ACTIVE status

        Raises:
            ItemNotFoundError: If rule doesn't exist
            InvalidSessionStateError: If rule is not in DISABLED state
        """
        rule = await self._repository.get_rule_by_id(rule_id)
        if rule is None:
            raise ItemNotFoundError(item_id=rule_id)

        if rule.status != LearningRuleStatus.DISABLED:
            raise InvalidSessionStateError(
                session_id=rule_id,
                current_status=rule.status.value,
                operation="enable",
                allowed_statuses=["disabled"],
            )

        # Update rule status
        rule.status = LearningRuleStatus.ACTIVE
        rule.updated_at = datetime.now(timezone.utc)

        return await self._repository.update_rule(rule)

    async def disable_rule(self, rule_id: int) -> LearningRuleDTO:
        """Disable an active rule.

        Args:
            rule_id: Rule identifier

        Returns:
            Updated learning rule DTO with DISABLED status

        Raises:
            ItemNotFoundError: If rule doesn't exist
            InvalidSessionStateError: If rule is not in ACTIVE state
        """
        rule = await self._repository.get_rule_by_id(rule_id)
        if rule is None:
            raise ItemNotFoundError(item_id=rule_id)

        if rule.status != LearningRuleStatus.ACTIVE:
            raise InvalidSessionStateError(
                session_id=rule_id,
                current_status=rule.status.value,
                operation="disable",
                allowed_statuses=["active"],
            )

        # Update rule status
        rule.status = LearningRuleStatus.DISABLED
        rule.updated_at = datetime.now(timezone.utc)

        return await self._repository.update_rule(rule)

    async def get_pending_suggestions(
        self,
        tenant_id: UUID,
    ) -> list[LearningRuleDTO]:
        """Get suggested rules pending user approval.

        Suggestions are rules that have been automatically generated
        by the learning system based on user feedback patterns.

        Args:
            tenant_id: Tenant isolation identifier

        Returns:
            List of pending rule suggestions
        """
        return await self._repository.get_pending_suggestions(tenant_id)

    async def accept_suggestion(
        self,
        suggestion_id: int,
    ) -> LearningRuleDTO:
        """Accept a suggested rule, making it active.

        Args:
            suggestion_id: Suggestion (rule) identifier

        Returns:
            Updated learning rule DTO with ACTIVE status

        Raises:
            ItemNotFoundError: If suggestion doesn't exist
        """
        rule = await self._repository.get_rule_by_id(suggestion_id)
        if rule is None:
            raise ItemNotFoundError(item_id=suggestion_id)

        # Transition from pending suggestion to active rule
        rule.status = LearningRuleStatus.ACTIVE
        rule.updated_at = datetime.now(timezone.utc)

        return await self._repository.update_rule(rule)

    async def reject_suggestion(
        self,
        suggestion_id: int,
    ) -> bool:
        """Reject a suggested rule.

        Marks the suggestion as rejected so it won't be shown again.
        Uses soft delete to preserve audit trail.

        Args:
            suggestion_id: Suggestion (rule) identifier

        Returns:
            True if successfully rejected

        Raises:
            ItemNotFoundError: If suggestion doesn't exist
        """
        rule = await self._repository.get_rule_by_id(suggestion_id)
        if rule is None:
            raise ItemNotFoundError(item_id=suggestion_id)

        # Soft delete the rejected suggestion
        now = datetime.now(timezone.utc)
        rule.is_deleted = True
        rule.deleted_at = now
        rule.updated_at = now

        await self._repository.update_rule(rule)
        return True

    async def apply_rules(
        self,
        tenant_id: UUID,
        item: ReviewItemDTO,
    ) -> RuleApplicationResult:
        """Apply matching rules to an item.

        Retrieves active rules for the tenant, finds matching rules,
        and applies their actions in priority order.

        Args:
            tenant_id: Tenant isolation identifier
            item: Review item to apply rules to

        Returns:
            RuleApplicationResult with applied rules and modifications
        """
        # Get active rules sorted by priority
        rules = await self._repository.get_rules_for_tenant(
            tenant_id, status="active"
        )

        # Sort by priority (lower number = higher priority)
        rules = sorted(rules, key=lambda r: r.priority)

        # Track modifications
        applied_rules: list[LearningRuleDTO] = []
        modifications: list[str] = []

        # Start with original suggestion
        modified_suggestion = AISuggestion(
            category=item.ai_suggestion.category,
            participants=list(item.ai_suggestion.participants),
            tags=list(item.ai_suggestion.tags),
        )

        # Apply each matching rule
        for rule in rules:
            if self._rule_matches(rule, item):
                # Check confidence threshold
                if item.ai_confidence < rule.confidence_threshold:
                    # Apply the rule's action
                    modification = self._apply_rule_action(
                        rule, modified_suggestion
                    )
                    if modification:
                        applied_rules.append(rule)
                        modifications.append(modification)

        return RuleApplicationResult(
            applied_rules=applied_rules,
            original_suggestion=item.ai_suggestion,
            modified_suggestion=modified_suggestion,
            modifications=modifications,
        )

    async def record_rule_outcome(
        self,
        rule_id: int,
        was_overridden: bool,
    ) -> None:
        """Record whether user accepted or overrode rule application.

        Updates effectiveness tracking for the rule based on
        whether the user accepted or overrode the rule's suggestion.

        Args:
            rule_id: Rule identifier
            was_overridden: True if user overrode the rule's suggestion

        Raises:
            ItemNotFoundError: If rule doesn't exist
        """
        rule = await self._repository.get_rule_by_id(rule_id)
        if rule is None:
            raise ItemNotFoundError(item_id=rule_id)

        # Update statistics
        now = datetime.now(timezone.utc)
        rule.times_applied += 1
        if was_overridden:
            rule.times_overridden += 1
        rule.last_applied_at = now
        rule.updated_at = now

        # Recalculate effectiveness score
        rule.effectiveness_score = self.calculate_effectiveness(
            rule.times_applied,
            rule.times_overridden,
        )

        await self._repository.update_rule(rule)

    def calculate_effectiveness(
        self,
        times_applied: int,
        times_overridden: int,
    ) -> Decimal:
        """Calculate rule effectiveness score.

        Effectiveness = 1 - (times_overridden / times_applied)

        A score of 1.0 means the rule is never overridden.
        A score of 0.0 means the rule is always overridden.

        Args:
            times_applied: Total times the rule was applied
            times_overridden: Times the user overrode the rule

        Returns:
            Effectiveness score between 0.0 and 1.0
            Returns 1.0 if never applied (benefit of the doubt)
        """
        if times_applied == 0:
            return Decimal("1.000")

        override_rate = Decimal(times_overridden) / Decimal(times_applied)
        effectiveness = Decimal("1.000") - override_rate

        # Clamp to valid range [0.0, 1.0]
        return max(Decimal("0.000"), min(Decimal("1.000"), effectiveness))

    # =========================================================================
    # PRIVATE HELPER METHODS
    # =========================================================================

    def _generate_rule_name(
        self,
        rule_type: LearningRuleType,
        match_criteria: dict[str, Any],
    ) -> str:
        """Generate a human-readable rule name from criteria.

        Args:
            rule_type: Type of rule
            match_criteria: Rule matching criteria

        Returns:
            Generated rule name
        """
        if rule_type == LearningRuleType.SENDER_MATCH:
            sender = match_criteria.get("sender", "")
            domain = match_criteria.get("sender_domain", "")
            if sender:
                return f"Sender: {sender}"
            if domain:
                return f"Domain: {domain}"
            return "Sender match rule"

        if rule_type == LearningRuleType.CONTENT_PATTERN:
            keywords = match_criteria.get("keywords", [])
            subject = match_criteria.get("subject_contains", "")
            if subject:
                return f"Subject: {subject}"
            if keywords:
                return f"Keywords: {', '.join(keywords[:3])}"
            return "Content pattern rule"

        if rule_type == LearningRuleType.CATEGORY_OVERRIDE:
            from_cat = match_criteria.get("from_category", "")
            to_cat = match_criteria.get("to_category", "")
            return f"Override: {from_cat} -> {to_cat}"

        if rule_type == LearningRuleType.PARTICIPANT_MAPPING:
            email = match_criteria.get("email", "")
            role = match_criteria.get("role", "")
            return f"Participant: {email} as {role}"

        return f"{rule_type.value} rule"

    def _rule_matches(
        self,
        rule: LearningRuleDTO,
        item: ReviewItemDTO,
    ) -> bool:
        """Check if a rule matches a review item.

        Args:
            rule: Learning rule to check
            item: Review item to match against

        Returns:
            True if the rule matches the item
        """
        criteria = rule.match_criteria

        if rule.rule_type == LearningRuleType.SENDER_MATCH:
            return self._match_sender(criteria, item)

        if rule.rule_type == LearningRuleType.CONTENT_PATTERN:
            return self._match_content(criteria, item)

        if rule.rule_type == LearningRuleType.CATEGORY_OVERRIDE:
            return self._match_category(criteria, item)

        if rule.rule_type == LearningRuleType.PARTICIPANT_MAPPING:
            return self._match_participant(criteria, item)

        return False

    def _match_sender(
        self,
        criteria: dict[str, Any],
        item: ReviewItemDTO,
    ) -> bool:
        """Match sender criteria against item.

        Args:
            criteria: Match criteria with sender or sender_domain
            item: Review item

        Returns:
            True if sender matches
        """
        # Extract sender from content preview or AI suggestion participants
        # In a real implementation, we'd have access to full email metadata
        sender = criteria.get("sender", "")
        sender_domain = criteria.get("sender_domain", "")

        # Check participants for sender match
        for participant in item.ai_suggestion.participants:
            if sender and participant.lower() == sender.lower():
                return True
            if sender_domain and participant.lower().endswith(
                f"@{sender_domain.lower()}"
            ):
                return True

        return False

    def _match_content(
        self,
        criteria: dict[str, Any],
        item: ReviewItemDTO,
    ) -> bool:
        """Match content pattern criteria against item.

        Args:
            criteria: Match criteria with keywords or subject_contains
            item: Review item

        Returns:
            True if content matches
        """
        content = item.content_preview.lower()

        # Check keywords
        keywords = criteria.get("keywords", [])
        if keywords:
            for keyword in keywords:
                if keyword.lower() in content:
                    return True

        # Check subject contains
        subject_contains = criteria.get("subject_contains", "")
        if subject_contains and subject_contains.lower() in content:
            return True

        # Check regex pattern if provided
        pattern = criteria.get("pattern")
        if pattern:
            try:
                if re.search(pattern, item.content_preview, re.IGNORECASE):
                    return True
            except re.error:
                # Invalid regex, skip
                pass

        return False

    def _match_category(
        self,
        criteria: dict[str, Any],
        item: ReviewItemDTO,
    ) -> bool:
        """Match category override criteria against item.

        Args:
            criteria: Match criteria with from_category
            item: Review item

        Returns:
            True if category matches
        """
        from_category = criteria.get("from_category", "")
        if from_category:
            return item.ai_suggestion.category.lower() == from_category.lower()
        return False

    def _match_participant(
        self,
        criteria: dict[str, Any],
        item: ReviewItemDTO,
    ) -> bool:
        """Match participant mapping criteria against item.

        Args:
            criteria: Match criteria with email
            item: Review item

        Returns:
            True if participant is present
        """
        email = criteria.get("email", "")
        if email:
            for participant in item.ai_suggestion.participants:
                if participant.lower() == email.lower():
                    return True
        return False

    def _apply_rule_action(
        self,
        rule: LearningRuleDTO,
        suggestion: AISuggestion,
    ) -> str | None:
        """Apply a rule's action to a suggestion.

        Modifies the suggestion in place and returns a description
        of the modification.

        Args:
            rule: Rule with action to apply
            suggestion: Suggestion to modify (in place)

        Returns:
            Description of modification, or None if no change
        """
        action = rule.action
        modification = None

        # Apply category change
        if "category" in action:
            old_category = suggestion.category
            suggestion.category = action["category"]
            modification = f"Category: {old_category} -> {suggestion.category}"

        # Apply participant additions
        if "add_participants" in action:
            for participant in action["add_participants"]:
                if participant not in suggestion.participants:
                    suggestion.participants.append(participant)
            modification = f"Added participants: {', '.join(action['add_participants'])}"

        # Apply participant role mapping
        if "role" in action and "email" in rule.match_criteria:
            email = rule.match_criteria["email"]
            role = action["role"]
            # For participant mapping, we might add role annotation
            # This depends on how roles are represented in the system
            modification = f"Mapped {email} to role: {role}"

        # Apply tag changes
        if "add_tags" in action:
            for tag in action["add_tags"]:
                if tag not in suggestion.tags:
                    suggestion.tags.append(tag)
            modification = f"Added tags: {', '.join(action['add_tags'])}"

        if "remove_tags" in action:
            for tag in action["remove_tags"]:
                if tag in suggestion.tags:
                    suggestion.tags.remove(tag)
            modification = f"Removed tags: {', '.join(action['remove_tags'])}"

        return modification
