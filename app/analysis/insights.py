"""
Meeting Insight Generation for Business Context

Extracts strategic business insights, identifies risks, opportunities,
and connects meeting content to broader organizational context.
"""
import openai
from typing import Dict, Any, List, Optional, Tuple
import json
import asyncio
from datetime import datetime, timedelta
import uuid
import re

from app.config import settings


class MeetingInsightGenerator:
    """Generates business insights from meeting content"""

    def __init__(self):
        self.client_available = settings.OPENAI_API_KEY is not None
        if self.client_available:
            openai.api_key = settings.OPENAI_API_KEY

        # Insight categories
        self.insight_types = [
            "strategic_decision",
            "risk_identification",
            "opportunity",
            "process_improvement",
            "resource_allocation",
            "timeline_impact",
            "stakeholder_concern",
            "technical_decision",
            "business_outcome",
            "escalation_needed"
        ]

        # Business context patterns
        self.business_patterns = {
            "financial": ["budget", "cost", "revenue", "profit", "funding", "investment", "ROI"],
            "timeline": ["deadline", "milestone", "schedule", "delay", "urgent", "priority"],
            "risk": ["risk", "concern", "issue", "problem", "blocker", "challenge"],
            "opportunity": ["opportunity", "growth", "expansion", "potential", "advantage"],
            "process": ["workflow", "process", "procedure", "efficiency", "optimization"],
            "stakeholder": ["client", "customer", "user", "stakeholder", "team", "management"]
        }

    async def generate_meeting_insights(
        self,
        transcript_text: str,
        summary_data: Dict[str, Any],
        participants: List[str],
        meeting_metadata: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Generate comprehensive business insights from meeting"""

        if not self.client_available:
            return self._create_error_result("OpenAI client not available")

        try:
            print(f"🔍 Generating business insights for meeting with {len(participants)} participants")

            # Extract strategic insights
            strategic_insights = await self._extract_strategic_insights(
                transcript_text, summary_data, meeting_metadata
            )

            # Identify risks and concerns
            risk_analysis = await self._analyze_risks_and_concerns(
                transcript_text, summary_data
            )

            # Find opportunities
            opportunities = await self._identify_opportunities(
                transcript_text, summary_data
            )

            # Analyze business context
            business_context = self._analyze_business_context(
                transcript_text, summary_data
            )

            # Generate follow-up recommendations
            followup_recommendations = await self._generate_followup_recommendations(
                strategic_insights, risk_analysis, opportunities, participants
            )

            # Calculate insight quality scores
            quality_metrics = self._calculate_insight_quality(
                strategic_insights, risk_analysis, opportunities, transcript_text
            )

            # Build comprehensive insight result
            result = {
                "strategic_insights": strategic_insights,
                "risk_analysis": risk_analysis,
                "opportunities": opportunities,
                "business_context": business_context,
                "followup_recommendations": followup_recommendations,
                "insight_summary": self._generate_insight_summary(
                    strategic_insights, risk_analysis, opportunities
                ),
                "quality_metrics": quality_metrics,
                "processing_metadata": {
                    "generated_at": datetime.utcnow().isoformat() + "Z",
                    "participant_count": len(participants),
                    "insight_categories": len(strategic_insights) + len(risk_analysis) + len(opportunities),
                    "business_domains": list(business_context.keys())
                }
            }

            print(f"✅ Generated {len(strategic_insights)} strategic insights, {len(risk_analysis)} risks, {len(opportunities)} opportunities")
            return result

        except Exception as e:
            print(f"❌ Insight generation failed: {str(e)}")
            return self._create_error_result(str(e))

    async def _extract_strategic_insights(
        self,
        transcript_text: str,
        summary_data: Dict[str, Any],
        meeting_metadata: Dict[str, Any]
    ) -> List[Dict[str, Any]]:
        """Extract strategic business insights"""

        key_points = summary_data.get("key_points", [])
        decisions = summary_data.get("decisions", [])

        prompt = f"""Analyze this meeting for strategic business insights.

MEETING SUMMARY:
{summary_data.get('summary_text', '')}

KEY DECISIONS:
{json.dumps(decisions, indent=2)}

FULL TRANSCRIPT: {transcript_text[:4000]}

Extract strategic insights in JSON format:
[
  {{
    "insight_type": "strategic_decision|business_outcome|timeline_impact|resource_allocation",
    "insight": "Clear, actionable business insight",
    "business_impact": "Description of potential business impact",
    "confidence": "high|medium|low",
    "urgency": "high|medium|low",
    "affected_areas": ["departments", "processes", "or", "teams", "affected"],
    "supporting_evidence": "Quote or reference from meeting",
    "recommended_actions": ["specific", "actions", "to", "take"],
    "timeline_implications": "Impact on business timelines (if any)"
  }}
]

Focus on:
- Strategic decisions and their implications
- Resource allocation changes
- Timeline impacts
- Cross-functional implications
- Business outcome predictors
- Process or organizational changes

Return only insights with clear business value."""

        try:
            response = await asyncio.to_thread(
                openai.ChatCompletion.create,
                model="gpt-4-0125-preview",
                messages=[
                    {"role": "system", "content": "You are a senior business strategist analyzing meeting insights. Return valid JSON only."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.3,
                max_tokens=1200
            )

            content = response.choices[0].message.content
            insights = json.loads(content)

            # Enhance insights with business context scoring
            for insight in insights:
                insight["business_context_score"] = self._score_business_context(insight)
                insight["insight_id"] = str(uuid.uuid4())

            return insights

        except Exception as e:
            print(f"⚠️ Strategic insight extraction failed: {str(e)}")
            return []

    async def _analyze_risks_and_concerns(
        self,
        transcript_text: str,
        summary_data: Dict[str, Any]
    ) -> List[Dict[str, Any]]:
        """Analyze risks and concerns mentioned in the meeting"""

        prompt = f"""Identify business risks, concerns, and potential issues from this meeting.

MEETING CONTENT:
{transcript_text[:4000]}

Extract risks in JSON format:
[
  {{
    "risk_type": "timeline|resource|technical|financial|stakeholder|compliance|operational",
    "risk_description": "Clear description of the risk or concern",
    "probability": "high|medium|low",
    "impact": "high|medium|low",
    "risk_level": "critical|significant|moderate|minor",
    "affected_stakeholders": ["who", "would", "be", "affected"],
    "mitigation_strategies": ["potential", "ways", "to", "address"],
    "indicators": "Signs or symptoms mentioned",
    "supporting_quote": "Direct quote from meeting (if available)",
    "requires_immediate_attention": true/false
  }}
]

Look for:
- Explicit concerns raised by participants
- Timeline pressures or delays
- Resource constraints
- Technical challenges
- Stakeholder dissatisfaction
- Process breakdowns
- Compliance issues

Only include legitimate business risks, not minor operational items."""

        try:
            response = await asyncio.to_thread(
                openai.ChatCompletion.create,
                model="gpt-4-0125-preview",
                messages=[
                    {"role": "system", "content": "You are a risk management specialist analyzing meeting content. Return valid JSON only."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.2,
                max_tokens=1000
            )

            content = response.choices[0].message.content
            risks = json.loads(content)

            # Add risk scoring and IDs
            for risk in risks:
                risk["risk_score"] = self._calculate_risk_score(risk)
                risk["risk_id"] = str(uuid.uuid4())

            return risks

        except Exception as e:
            print(f"⚠️ Risk analysis failed: {str(e)}")
            return []

    async def _identify_opportunities(
        self,
        transcript_text: str,
        summary_data: Dict[str, Any]
    ) -> List[Dict[str, Any]]:
        """Identify business opportunities discussed in the meeting"""

        prompt = f"""Identify business opportunities, potential improvements, and growth areas from this meeting.

MEETING CONTENT:
{transcript_text[:4000]}

Extract opportunities in JSON format:
[
  {{
    "opportunity_type": "growth|efficiency|cost_saving|revenue|process_improvement|innovation|partnership",
    "opportunity": "Clear description of the business opportunity",
    "potential_value": "Description of potential business value",
    "implementation_complexity": "high|medium|low",
    "timeframe": "immediate|short_term|medium_term|long_term",
    "required_resources": ["what", "resources", "would", "be", "needed"],
    "success_indicators": ["how", "to", "measure", "success"],
    "risks_if_not_pursued": "Potential downside of not acting",
    "competitive_advantage": "How this could provide competitive edge",
    "supporting_evidence": "Quote or context from meeting"
  }}
]

Focus on:
- Market opportunities mentioned
- Process improvement possibilities
- Cost reduction opportunities
- Revenue generation ideas
- Innovation prospects
- Efficiency gains
- Partnership possibilities
- Competitive advantages

Only include opportunities with clear business potential."""

        try:
            response = await asyncio.to_thread(
                openai.ChatCompletion.create,
                model="gpt-4-0125-preview",
                messages=[
                    {"role": "system", "content": "You are a business development strategist identifying opportunities. Return valid JSON only."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.4,
                max_tokens=1000
            )

            content = response.choices[0].message.content
            opportunities = json.loads(content)

            # Add opportunity scoring
            for opportunity in opportunities:
                opportunity["opportunity_score"] = self._score_opportunity(opportunity)
                opportunity["opportunity_id"] = str(uuid.uuid4())

            return opportunities

        except Exception as e:
            print(f"⚠️ Opportunity identification failed: {str(e)}")
            return []

    def _analyze_business_context(
        self,
        transcript_text: str,
        summary_data: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Analyze business context patterns in the meeting"""

        context_analysis = {}
        text_lower = transcript_text.lower()

        # Analyze each business domain
        for domain, keywords in self.business_patterns.items():
            matches = []
            domain_score = 0

            for keyword in keywords:
                count = text_lower.count(keyword)
                if count > 0:
                    matches.append({"keyword": keyword, "frequency": count})
                    domain_score += count

            if matches:
                context_analysis[domain] = {
                    "relevance_score": min(10, domain_score),  # Cap at 10
                    "keywords_found": matches,
                    "dominant": domain_score > 3  # Mark as dominant theme
                }

        # Identify primary business theme
        if context_analysis:
            primary_theme = max(context_analysis.keys(),
                              key=lambda k: context_analysis[k]["relevance_score"])
            context_analysis["primary_business_theme"] = primary_theme

        # Calculate overall business complexity
        complexity_score = len(context_analysis)
        context_analysis["meeting_complexity"] = {
            "score": complexity_score,
            "level": "high" if complexity_score >= 4 else "medium" if complexity_score >= 2 else "low",
            "cross_functional": complexity_score >= 3
        }

        return context_analysis

    async def _generate_followup_recommendations(
        self,
        strategic_insights: List[Dict[str, Any]],
        risks: List[Dict[str, Any]],
        opportunities: List[Dict[str, Any]],
        participants: List[str]
    ) -> List[Dict[str, Any]]:
        """Generate follow-up recommendations based on insights"""

        # Combine all insights for comprehensive analysis
        all_insights = strategic_insights + risks + opportunities

        if not all_insights:
            return []

        high_priority_items = [
            item for item in all_insights
            if item.get("urgency") == "high" or
               item.get("impact") == "high" or
               item.get("requires_immediate_attention", False)
        ]

        recommendations = []

        # Generate immediate action recommendations
        if high_priority_items:
            recommendations.append({
                "type": "immediate_action",
                "priority": "high",
                "recommendation": "Schedule follow-up meeting to address high-priority items identified",
                "rationale": f"Meeting identified {len(high_priority_items)} high-priority items requiring immediate attention",
                "suggested_participants": participants,
                "timeline": "within 48 hours",
                "related_insights": [item.get("insight_id") or item.get("risk_id") or item.get("opportunity_id") for item in high_priority_items]
            })

        # Risk mitigation recommendations
        high_risks = [r for r in risks if r.get("risk_level") in ["critical", "significant"]]
        if high_risks:
            recommendations.append({
                "type": "risk_mitigation",
                "priority": "high",
                "recommendation": "Develop risk mitigation plan for identified critical risks",
                "rationale": f"Meeting identified {len(high_risks)} critical/significant risks",
                "suggested_owner": "risk_management_team",
                "timeline": "within 1 week"
            })

        # Opportunity pursuit recommendations
        high_value_ops = [o for o in opportunities if o.get("implementation_complexity") == "low"]
        if high_value_ops:
            recommendations.append({
                "type": "opportunity_pursuit",
                "priority": "medium",
                "recommendation": "Evaluate and prioritize low-complexity, high-value opportunities",
                "rationale": f"Meeting identified {len(high_value_ops)} opportunities with low implementation complexity",
                "timeline": "within 2 weeks"
            })

        return recommendations

    def _score_business_context(self, insight: Dict[str, Any]) -> float:
        """Score business context relevance of an insight"""

        score = 0.0

        # Impact scoring
        impact_scores = {"high": 0.4, "medium": 0.2, "low": 0.1}
        score += impact_scores.get(insight.get("business_impact", "").lower(), 0.0)

        # Urgency scoring
        urgency_scores = {"high": 0.3, "medium": 0.2, "low": 0.1}
        score += urgency_scores.get(insight.get("urgency", "").lower(), 0.0)

        # Affected areas scoring (more areas = higher complexity/importance)
        affected_areas = len(insight.get("affected_areas", []))
        score += min(0.2, affected_areas * 0.05)

        # Evidence quality scoring
        if insight.get("supporting_evidence"):
            score += 0.1

        return round(min(1.0, score), 2)

    def _calculate_risk_score(self, risk: Dict[str, Any]) -> float:
        """Calculate risk score based on probability and impact"""

        prob_scores = {"high": 0.8, "medium": 0.5, "low": 0.2}
        impact_scores = {"high": 0.8, "medium": 0.5, "low": 0.2}

        probability = prob_scores.get(risk.get("probability", "low").lower(), 0.2)
        impact = impact_scores.get(risk.get("impact", "low").lower(), 0.2)

        # Risk score = probability × impact
        base_score = probability * impact

        # Boost for immediate attention items
        if risk.get("requires_immediate_attention", False):
            base_score *= 1.2

        return round(min(1.0, base_score), 2)

    def _score_opportunity(self, opportunity: Dict[str, Any]) -> float:
        """Score opportunity value"""

        score = 0.0

        # Implementation complexity (lower complexity = higher score)
        complexity_scores = {"low": 0.4, "medium": 0.3, "high": 0.1}
        score += complexity_scores.get(opportunity.get("implementation_complexity", "high").lower(), 0.1)

        # Timeframe (shorter timeframe = higher score for quick wins)
        timeframe_scores = {"immediate": 0.3, "short_term": 0.25, "medium_term": 0.15, "long_term": 0.1}
        score += timeframe_scores.get(opportunity.get("timeframe", "long_term").lower(), 0.1)

        # Competitive advantage
        if "competitive" in opportunity.get("competitive_advantage", "").lower():
            score += 0.2

        # Evidence quality
        if opportunity.get("supporting_evidence"):
            score += 0.1

        return round(min(1.0, score), 2)

    def _calculate_insight_quality(
        self,
        strategic_insights: List[Dict[str, Any]],
        risks: List[Dict[str, Any]],
        opportunities: List[Dict[str, Any]],
        transcript_text: str
    ) -> Dict[str, Any]:
        """Calculate overall insight quality metrics"""

        total_insights = len(strategic_insights) + len(risks) + len(opportunities)
        word_count = len(transcript_text.split())

        # Insight density (insights per 100 words)
        insight_density = (total_insights / max(word_count, 1)) * 100

        # High-value insight count
        high_value_insights = sum(1 for insight in strategic_insights + risks + opportunities
                                if insight.get("urgency") == "high" or
                                   insight.get("impact") == "high" or
                                   insight.get("priority") == "high")

        # Actionability score
        actionable_insights = sum(1 for insight in strategic_insights + risks + opportunities
                                if insight.get("recommended_actions") or
                                   insight.get("mitigation_strategies") or
                                   insight.get("required_resources"))

        actionability_score = actionable_insights / max(total_insights, 1)

        return {
            "total_insights": total_insights,
            "insight_density": round(insight_density, 2),
            "high_value_insights": high_value_insights,
            "actionability_score": round(actionability_score, 2),
            "business_coverage": {
                "strategic": len(strategic_insights),
                "risk": len(risks),
                "opportunity": len(opportunities)
            },
            "quality_assessment": {
                "comprehensive": total_insights >= 3,
                "actionable": actionability_score > 0.5,
                "strategic_value": high_value_insights > 0
            }
        }

    def _generate_insight_summary(
        self,
        strategic_insights: List[Dict[str, Any]],
        risks: List[Dict[str, Any]],
        opportunities: List[Dict[str, Any]]
    ) -> Dict[str, Any]:
        """Generate executive summary of all insights"""

        # Count high-priority items
        high_priority_strategic = sum(1 for i in strategic_insights if i.get("urgency") == "high")
        critical_risks = sum(1 for r in risks if r.get("risk_level") == "critical")
        immediate_opportunities = sum(1 for o in opportunities if o.get("timeframe") == "immediate")

        summary = {
            "executive_summary": f"Meeting analysis identified {len(strategic_insights)} strategic insights, {len(risks)} risks, and {len(opportunities)} opportunities.",
            "key_highlights": [],
            "immediate_actions_required": high_priority_strategic + critical_risks > 0,
            "strategic_implications": len(strategic_insights) > 2,
            "risk_level": "high" if critical_risks > 0 else "medium" if len(risks) > 2 else "low"
        }

        # Add key highlights
        if high_priority_strategic > 0:
            summary["key_highlights"].append(f"{high_priority_strategic} high-priority strategic insights requiring attention")

        if critical_risks > 0:
            summary["key_highlights"].append(f"{critical_risks} critical risks identified needing immediate mitigation")

        if immediate_opportunities > 0:
            summary["key_highlights"].append(f"{immediate_opportunities} immediate opportunities for quick wins")

        return summary

    def _create_error_result(self, error_message: str) -> Dict[str, Any]:
        """Create error result for failed insight generation"""

        return {
            "error": error_message,
            "strategic_insights": [],
            "risk_analysis": [],
            "opportunities": [],
            "quality_metrics": {
                "error": True,
                "total_insights": 0
            }
        }

    def is_available(self) -> bool:
        """Check if insight generation is available"""
        return self.client_available

    async def test_insight_generation(self) -> Dict[str, Any]:
        """Test insight generation functionality"""

        if not self.client_available:
            return {"available": False, "error": "OpenAI API key not configured"}

        test_transcript = """We discussed the Q2 budget and decided to increase marketing spend by 20%.
        There's a risk that our main competitor might launch before us, which could impact our market share.
        We identified an opportunity to partner with the supplier for cost reduction.
        The development timeline is tight and we might need additional resources."""

        test_summary = {
            "summary_text": "Budget discussion with decisions on marketing spend",
            "key_points": [{"point": "Increase marketing budget", "category": "decision"}],
            "decisions": [{"decision": "Increase marketing spend by 20%"}]
        }

        try:
            result = await self.generate_meeting_insights(
                test_transcript, test_summary, ["Test User"], {"filename": "test"}
            )

            return {
                "available": True,
                "test_successful": "error" not in result,
                "insights_generated": result.get("quality_metrics", {}).get("total_insights", 0),
                "categories_detected": len([
                    result.get("strategic_insights", []),
                    result.get("risk_analysis", []),
                    result.get("opportunities", [])
                ])
            }

        except Exception as e:
            return {
                "available": False,
                "test_successful": False,
                "error": str(e)
            }


# Global insight generator instance
meeting_insight_generator = MeetingInsightGenerator()