"""Ensemble result combination system.

Implements User Story 2: Ensemble Result Combination
- Combines results from multiple AI models using various strategies
- Handles confidence weighting and conflict resolution
- Normalizes different model output formats
- Provides ensemble quality metrics and selection reasoning
"""

import logging
import statistics
from datetime import datetime, timezone
from typing import Dict, List, Optional, Any, Union
from dataclasses import dataclass, asdict
from enum import Enum

logger = logging.getLogger(__name__)


class CombinationStrategy(Enum):
    """Strategies for combining multiple model results."""
    WEIGHTED_AVERAGE = "weighted_average"
    CONFIDENCE_VOTING = "confidence_voting"
    BEST_CONFIDENCE = "best_confidence"
    MAJORITY_VOTE = "majority_vote"
    CONSENSUS_THRESHOLD = "consensus_threshold"


@dataclass
class EnsembleResult:
    """Combined result from multiple AI models with aggregated confidence.

    Represents the output of ensemble combination with detailed reasoning
    and quality metrics for transparency and improvement.
    """
    ensemble_id: str
    primary_result: Dict[str, Any]
    supporting_results: List[Dict[str, Any]]
    confidence_score: float
    combination_strategy: str
    consensus_strength: float
    conflict_indicators: Dict[str, Any]
    selection_reasoning: str
    model_contributions: Dict[str, float]
    quality_metrics: Dict[str, Any]
    created_at: datetime
    processing_time_ms: Optional[int] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for serialization."""
        result = asdict(self)
        result["created_at"] = self.created_at.isoformat()
        return result


class ResultNormalizer:
    """Normalizes different model output formats for comparison."""

    @staticmethod
    def normalize_result(result: Dict[str, Any], model_name: str) -> Dict[str, Any]:
        """Normalize model result to standard format.

        Args:
            result: Raw model result
            model_name: Name of the model that produced the result

        Returns:
            Normalized result with consistent structure
        """
        normalized = {
            "model_name": model_name,
            "content": {},
            "entities": [],
            "categories": [],
            "sentiment": None,
            "confidence": result.get("confidence", 0.5),
            "metadata": {}
        }

        # Extract common fields from different model formats
        if "text" in result:
            normalized["content"]["text"] = result["text"]
        if "summary" in result:
            normalized["content"]["summary"] = result["summary"]
        if "entities" in result:
            normalized["entities"] = ResultNormalizer._normalize_entities(result["entities"])
        if "categories" in result or "classification" in result:
            normalized["categories"] = result.get("categories", result.get("classification", []))
        if "sentiment" in result:
            normalized["sentiment"] = ResultNormalizer._normalize_sentiment(result["sentiment"])

        # Preserve model-specific data in metadata
        normalized["metadata"] = {
            k: v for k, v in result.items()
            if k not in ["text", "summary", "entities", "categories", "classification", "sentiment", "confidence"]
        }

        return normalized

    @staticmethod
    def _normalize_entities(entities: Any) -> List[Dict[str, Any]]:
        """Normalize entity extraction results."""
        if isinstance(entities, list):
            return [
                {"text": e.get("text", str(e)), "type": e.get("type", "UNKNOWN"), "confidence": e.get("confidence", 1.0)}
                if isinstance(e, dict) else {"text": str(e), "type": "UNKNOWN", "confidence": 1.0}
                for e in entities
            ]
        return []

    @staticmethod
    def _normalize_sentiment(sentiment: Any) -> Optional[Dict[str, Any]]:
        """Normalize sentiment analysis results."""
        if isinstance(sentiment, dict):
            return {
                "label": sentiment.get("label", sentiment.get("sentiment", "neutral")),
                "score": sentiment.get("score", sentiment.get("confidence", 0.5))
            }
        elif isinstance(sentiment, str):
            return {"label": sentiment, "score": 0.5}
        return None


class EnsembleCombiner:
    """Combines results from multiple AI models using various strategies."""

    def __init__(self, default_strategy: CombinationStrategy = CombinationStrategy.CONFIDENCE_VOTING):
        self.default_strategy = default_strategy
        self.normalizer = ResultNormalizer()

    async def combine_results(
        self,
        individual_results: List[Dict[str, Any]],
        content_type: str,
        content_data: Optional[Dict[str, Any]] = None,
        strategy: Optional[CombinationStrategy] = None
    ) -> EnsembleResult:
        """Combine multiple model results into ensemble output.

        Args:
            individual_results: List of results from different models
            content_type: Type of content being processed
            content_data: Original content data for context
            strategy: Combination strategy to use

        Returns:
            EnsembleResult with combined output and quality metrics
        """
        if not individual_results:
            raise ValueError("No results to combine")

        if len(individual_results) == 1:
            # Single result - no combination needed
            return self._create_single_result_ensemble(individual_results[0])

        start_time = datetime.now()
        strategy = strategy or self.default_strategy

        # Normalize all results to consistent format
        normalized_results = []
        for result in individual_results:
            normalized = self.normalizer.normalize_result(
                result["result_data"],
                result["processor_name"]
            )
            normalized["confidence"] = result["confidence_score"]
            normalized["processing_time"] = result.get("processing_time_ms")
            normalized_results.append(normalized)

        # Apply combination strategy
        if strategy == CombinationStrategy.WEIGHTED_AVERAGE:
            ensemble = await self._weighted_average_combination(normalized_results)
        elif strategy == CombinationStrategy.CONFIDENCE_VOTING:
            ensemble = await self._confidence_voting_combination(normalized_results)
        elif strategy == CombinationStrategy.BEST_CONFIDENCE:
            ensemble = await self._best_confidence_combination(normalized_results)
        elif strategy == CombinationStrategy.MAJORITY_VOTE:
            ensemble = await self._majority_vote_combination(normalized_results)
        elif strategy == CombinationStrategy.CONSENSUS_THRESHOLD:
            ensemble = await self._consensus_threshold_combination(normalized_results)
        else:
            raise ValueError(f"Unknown combination strategy: {strategy}")

        # Calculate processing time
        processing_time = int((datetime.now() - start_time).total_seconds() * 1000)
        ensemble.processing_time_ms = processing_time

        return ensemble

    async def _weighted_average_combination(self, results: List[Dict[str, Any]]) -> EnsembleResult:
        """Combine results using confidence-weighted averaging."""
        total_weight = sum(r["confidence"] for r in results)

        if total_weight == 0:
            # Fall back to equal weighting
            total_weight = len(results)
            weights = [1.0 for _ in results]
        else:
            weights = [r["confidence"] / total_weight for r in results]

        # Weighted average confidence
        ensemble_confidence = sum(r["confidence"] * w for r, w in zip(results, weights))

        # Select best result as primary, but weight contributions
        best_result = max(results, key=lambda r: r["confidence"])

        # Calculate consensus strength
        consensus_strength = self._calculate_consensus_strength(results)

        # Combine text content (use highest confidence)
        primary_content = best_result["content"]

        # Aggregate entities with confidence weighting
        all_entities = []
        for r, weight in zip(results, weights):
            for entity in r["entities"]:
                entity["weighted_confidence"] = entity["confidence"] * weight
                all_entities.append(entity)

        # Merge similar entities and calculate weighted confidence
        merged_entities = self._merge_similar_entities(all_entities)

        return EnsembleResult(
            ensemble_id=f"ensemble_{datetime.now().timestamp()}",
            primary_result={
                "content": primary_content,
                "entities": merged_entities,
                "categories": self._combine_categories(results, weights),
                "sentiment": self._combine_sentiment(results, weights)
            },
            supporting_results=results,
            confidence_score=ensemble_confidence,
            combination_strategy="weighted_average",
            consensus_strength=consensus_strength,
            conflict_indicators=self._identify_conflicts(results),
            selection_reasoning=f"Weighted average combination with {len(results)} models, primary result from {best_result['model_name']} (confidence: {best_result['confidence']:.2f})",
            model_contributions={r["model_name"]: w for r, w in zip(results, weights)},
            quality_metrics=self._calculate_quality_metrics(results, ensemble_confidence),
            created_at=datetime.now(timezone.utc)
        )

    async def _confidence_voting_combination(self, results: List[Dict[str, Any]]) -> EnsembleResult:
        """Combine results using confidence-based voting."""
        # Sort results by confidence (highest first)
        sorted_results = sorted(results, key=lambda r: r["confidence"], reverse=True)

        # Primary result is highest confidence
        primary = sorted_results[0]

        # Ensemble confidence is weighted by model agreement
        confidence_scores = [r["confidence"] for r in results]
        ensemble_confidence = statistics.mean(confidence_scores)

        # Boost confidence if models agree
        consensus_strength = self._calculate_consensus_strength(results)
        ensemble_confidence = min(1.0, ensemble_confidence * (1 + consensus_strength * 0.2))

        return EnsembleResult(
            ensemble_id=f"ensemble_{datetime.now().timestamp()}",
            primary_result=primary["content"],
            supporting_results=results,
            confidence_score=ensemble_confidence,
            combination_strategy="confidence_voting",
            consensus_strength=consensus_strength,
            conflict_indicators=self._identify_conflicts(results),
            selection_reasoning=f"Confidence voting: selected {primary['model_name']} with {primary['confidence']:.2f} confidence from {len(results)} models",
            model_contributions={r["model_name"]: r["confidence"] for r in results},
            quality_metrics=self._calculate_quality_metrics(results, ensemble_confidence),
            created_at=datetime.now(timezone.utc)
        )

    async def _best_confidence_combination(self, results: List[Dict[str, Any]]) -> EnsembleResult:
        """Simply select the result with highest confidence."""
        best_result = max(results, key=lambda r: r["confidence"])

        return EnsembleResult(
            ensemble_id=f"ensemble_{datetime.now().timestamp()}",
            primary_result=best_result["content"],
            supporting_results=[r for r in results if r != best_result],
            confidence_score=best_result["confidence"],
            combination_strategy="best_confidence",
            consensus_strength=self._calculate_consensus_strength(results),
            conflict_indicators=self._identify_conflicts(results),
            selection_reasoning=f"Best confidence: selected {best_result['model_name']} with {best_result['confidence']:.2f} confidence",
            model_contributions={best_result["model_name"]: 1.0},
            quality_metrics=self._calculate_quality_metrics(results, best_result["confidence"]),
            created_at=datetime.now(timezone.utc)
        )

    async def _majority_vote_combination(self, results: List[Dict[str, Any]]) -> EnsembleResult:
        """Combine using majority voting for categorical decisions."""
        # For categories and sentiment, use voting
        category_votes = {}
        sentiment_votes = {}

        for result in results:
            # Vote on categories
            for category in result.get("categories", []):
                if category not in category_votes:
                    category_votes[category] = 0
                category_votes[category] += 1

            # Vote on sentiment
            if result.get("sentiment"):
                sentiment_label = result["sentiment"].get("label", "neutral")
                if sentiment_label not in sentiment_votes:
                    sentiment_votes[sentiment_label] = 0
                sentiment_votes[sentiment_label] += 1

        # Select majority categories and sentiment
        majority_categories = [
            cat for cat, votes in category_votes.items()
            if votes > len(results) / 2
        ]

        majority_sentiment = max(sentiment_votes.items(), key=lambda x: x[1])[0] if sentiment_votes else "neutral"

        # Use highest confidence result as base
        best_result = max(results, key=lambda r: r["confidence"])

        ensemble_confidence = statistics.mean([r["confidence"] for r in results])

        return EnsembleResult(
            ensemble_id=f"ensemble_{datetime.now().timestamp()}",
            primary_result={
                "content": best_result["content"],
                "entities": best_result["entities"],  # Use best model's entities
                "categories": majority_categories,
                "sentiment": {"label": majority_sentiment, "score": ensemble_confidence}
            },
            supporting_results=results,
            confidence_score=ensemble_confidence,
            combination_strategy="majority_vote",
            consensus_strength=self._calculate_consensus_strength(results),
            conflict_indicators=self._identify_conflicts(results),
            selection_reasoning=f"Majority vote: {len(majority_categories)} categories, {majority_sentiment} sentiment from {len(results)} models",
            model_contributions={r["model_name"]: 1.0/len(results) for r in results},
            quality_metrics=self._calculate_quality_metrics(results, ensemble_confidence),
            created_at=datetime.now(timezone.utc)
        )

    async def _consensus_threshold_combination(self, results: List[Dict[str, Any]]) -> EnsembleResult:
        """Require minimum consensus before accepting ensemble result."""
        consensus_threshold = 0.7  # 70% agreement required
        consensus_strength = self._calculate_consensus_strength(results)

        if consensus_strength < consensus_threshold:
            # Not enough consensus - flag for human review
            best_result = max(results, key=lambda r: r["confidence"])
            return EnsembleResult(
                ensemble_id=f"ensemble_{datetime.now().timestamp()}",
                primary_result=best_result["content"],
                supporting_results=results,
                confidence_score=best_result["confidence"] * 0.5,  # Reduce confidence due to disagreement
                combination_strategy="consensus_threshold",
                consensus_strength=consensus_strength,
                conflict_indicators=self._identify_conflicts(results),
                selection_reasoning=f"Low consensus ({consensus_strength:.2f} < {consensus_threshold}): flagged for review, using {best_result['model_name']}",
                model_contributions={best_result["model_name"]: 1.0},
                quality_metrics=self._calculate_quality_metrics(results, best_result["confidence"]),
                created_at=datetime.now(timezone.utc)
            )

        # High consensus - use weighted average
        return await self._weighted_average_combination(results)

    def _create_single_result_ensemble(self, result: Dict[str, Any]) -> EnsembleResult:
        """Create ensemble result for single model output."""
        normalized = self.normalizer.normalize_result(
            result["result_data"],
            result["processor_name"]
        )

        return EnsembleResult(
            ensemble_id=f"ensemble_{datetime.now().timestamp()}",
            primary_result=normalized["content"],
            supporting_results=[],
            confidence_score=result["confidence_score"],
            combination_strategy="single_model",
            consensus_strength=1.0,
            conflict_indicators={},
            selection_reasoning=f"Single model result from {result['processor_name']}",
            model_contributions={result["processor_name"]: 1.0},
            quality_metrics={"model_count": 1, "confidence_variance": 0.0},
            created_at=datetime.now(timezone.utc),
            processing_time_ms=result.get("processing_time_ms")
        )

    def _calculate_consensus_strength(self, results: List[Dict[str, Any]]) -> float:
        """Calculate how much the models agree with each other."""
        if len(results) < 2:
            return 1.0

        # Calculate confidence variance (lower variance = higher consensus)
        confidences = [r["confidence"] for r in results]
        confidence_variance = statistics.variance(confidences) if len(confidences) > 1 else 0.0

        # Calculate entity overlap
        entity_similarity = self._calculate_entity_similarity(results)

        # Calculate sentiment agreement
        sentiment_agreement = self._calculate_sentiment_agreement(results)

        # Combine metrics (lower variance and higher similarity = higher consensus)
        consensus = (
            (1 - min(1.0, confidence_variance)) * 0.4 +
            entity_similarity * 0.4 +
            sentiment_agreement * 0.2
        )

        return max(0.0, min(1.0, consensus))

    def _calculate_entity_similarity(self, results: List[Dict[str, Any]]) -> float:
        """Calculate similarity of extracted entities across models."""
        all_entities = set()
        model_entities = []

        for result in results:
            entities = set(entity["text"].lower() for entity in result.get("entities", []))
            model_entities.append(entities)
            all_entities.update(entities)

        if not all_entities:
            return 1.0  # No entities to compare

        # Calculate Jaccard similarity between all pairs
        similarities = []
        for i in range(len(model_entities)):
            for j in range(i + 1, len(model_entities)):
                intersection = len(model_entities[i] & model_entities[j])
                union = len(model_entities[i] | model_entities[j])
                similarity = intersection / union if union > 0 else 1.0
                similarities.append(similarity)

        return statistics.mean(similarities) if similarities else 1.0

    def _calculate_sentiment_agreement(self, results: List[Dict[str, Any]]) -> float:
        """Calculate sentiment agreement across models."""
        sentiments = [
            r.get("sentiment", {}).get("label", "neutral")
            for r in results if r.get("sentiment")
        ]

        if not sentiments:
            return 1.0

        # Calculate most common sentiment
        sentiment_counts = {}
        for sentiment in sentiments:
            sentiment_counts[sentiment] = sentiment_counts.get(sentiment, 0) + 1

        max_count = max(sentiment_counts.values())
        agreement = max_count / len(sentiments)

        return agreement

    def _identify_conflicts(self, results: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Identify conflicts and disagreements between models."""
        conflicts = {
            "confidence_variance": statistics.variance([r["confidence"] for r in results]) if len(results) > 1 else 0.0,
            "sentiment_conflicts": [],
            "category_conflicts": [],
            "entity_conflicts": []
        }

        # Check for sentiment conflicts
        sentiments = [r.get("sentiment", {}).get("label") for r in results if r.get("sentiment")]
        unique_sentiments = set(filter(None, sentiments))
        if len(unique_sentiments) > 1:
            conflicts["sentiment_conflicts"] = list(unique_sentiments)

        # Check for category conflicts
        all_categories = set()
        for result in results:
            all_categories.update(result.get("categories", []))

        category_support = {}
        for category in all_categories:
            support_count = sum(1 for r in results if category in r.get("categories", []))
            category_support[category] = support_count / len(results)

        # Flag categories with low support as conflicts
        conflicts["category_conflicts"] = [
            cat for cat, support in category_support.items() if 0 < support < 0.5
        ]

        return conflicts

    def _merge_similar_entities(self, entities: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Merge similar entities and combine their confidence scores."""
        entity_groups = {}

        for entity in entities:
            key = entity["text"].lower().strip()
            if key not in entity_groups:
                entity_groups[key] = []
            entity_groups[key].append(entity)

        merged_entities = []
        for key, group in entity_groups.items():
            if not group:
                continue

            # Use most common text format
            most_common_text = max(group, key=lambda e: len(group))["text"]

            # Average confidence scores
            avg_confidence = statistics.mean([e.get("weighted_confidence", e["confidence"]) for e in group])

            # Use most common type
            types = [e["type"] for e in group]
            most_common_type = max(set(types), key=types.count) if types else "UNKNOWN"

            merged_entities.append({
                "text": most_common_text,
                "type": most_common_type,
                "confidence": avg_confidence,
                "source_count": len(group)
            })

        return sorted(merged_entities, key=lambda e: e["confidence"], reverse=True)

    def _combine_categories(self, results: List[Dict[str, Any]], weights: List[float]) -> List[str]:
        """Combine categories using weighted voting."""
        category_scores = {}

        for result, weight in zip(results, weights):
            for category in result.get("categories", []):
                if category not in category_scores:
                    category_scores[category] = 0
                category_scores[category] += weight

        # Return categories with score above threshold
        threshold = sum(weights) * 0.3  # 30% of total weight
        return [cat for cat, score in category_scores.items() if score >= threshold]

    def _combine_sentiment(self, results: List[Dict[str, Any]], weights: List[float]) -> Optional[Dict[str, Any]]:
        """Combine sentiment using weighted averaging."""
        sentiment_scores = {}

        for result, weight in zip(results, weights):
            sentiment = result.get("sentiment")
            if sentiment and sentiment.get("label"):
                label = sentiment["label"]
                score = sentiment.get("score", 0.5)
                if label not in sentiment_scores:
                    sentiment_scores[label] = 0
                sentiment_scores[label] += score * weight

        if not sentiment_scores:
            return None

        # Return highest weighted sentiment
        best_sentiment = max(sentiment_scores.items(), key=lambda x: x[1])
        return {
            "label": best_sentiment[0],
            "score": min(1.0, best_sentiment[1] / sum(weights))
        }

    def _calculate_quality_metrics(self, results: List[Dict[str, Any]], ensemble_confidence: float) -> Dict[str, Any]:
        """Calculate ensemble quality metrics."""
        confidences = [r["confidence"] for r in results]

        return {
            "model_count": len(results),
            "confidence_mean": statistics.mean(confidences),
            "confidence_variance": statistics.variance(confidences) if len(confidences) > 1 else 0.0,
            "confidence_range": max(confidences) - min(confidences) if confidences else 0.0,
            "ensemble_improvement": ensemble_confidence - max(confidences) if confidences else 0.0,
            "consensus_strength": self._calculate_consensus_strength(results)
        }