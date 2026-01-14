"""
Meeting Summary Generation using AI Coordination Framework

Generates structured meeting summaries with key points, decisions, and action items
using OpenAI GPT models with comprehensive prompt engineering.
"""
import openai
from typing import Dict, Any, List, Optional, Tuple
import json
import asyncio
from datetime import datetime
import uuid
from pathlib import Path

from app.config import settings


class MeetingSummarizer:
    """Generates structured meeting summaries using AI"""

    def __init__(self):
        # Initialize OpenAI client
        if settings.OPENAI_API_KEY:
            openai.api_key = settings.OPENAI_API_KEY
            self.client_available = True
        else:
            self.client_available = False
            print("⚠️ OpenAI API key not configured - summary generation disabled")

        # Model configurations
        self.models = {
            "summary": "gpt-4-0125-preview",  # Best for structured analysis
            "extraction": "gpt-3.5-turbo-0125",  # Good for entity extraction
            "classification": "gpt-3.5-turbo-0125"  # Good for categorization
        }

        # Quality thresholds
        self.min_transcript_length = 50  # words
        self.max_chunk_tokens = 8000  # For chunking long transcripts

    async def generate_meeting_summary(
        self,
        transcript_text: str,
        speaker_segments: List[Dict[str, Any]],
        meeting_metadata: Dict[str, Any],
        participants: List[str]
    ) -> Dict[str, Any]:
        """Generate comprehensive meeting summary"""

        if not self.client_available:
            return self._create_error_result("OpenAI client not available")

        try:
            print(f"📋 Generating summary for meeting with {len(transcript_text.split())} words")

            # Prepare context
            context = self._prepare_meeting_context(
                transcript_text, speaker_segments, meeting_metadata, participants
            )

            # Check if transcript is long enough for meaningful analysis
            word_count = len(transcript_text.split())
            if word_count < self.min_transcript_length:
                return self._create_minimal_summary(transcript_text, context)

            # Generate structured summary
            summary_result = await self._generate_structured_summary(context)

            # Extract key points
            key_points = await self._extract_key_points(context, summary_result)

            # Extract decisions
            decisions = await self._extract_decisions(context)

            # Extract action items
            action_items = await self._extract_action_items(context, participants)

            # Calculate confidence scores
            confidence_metrics = self._calculate_confidence_scores(
                summary_result, key_points, decisions, action_items, word_count
            )

            # Build comprehensive result
            result = {
                "summary_text": summary_result.get("summary", ""),
                "executive_summary": summary_result.get("executive_summary", ""),
                "key_points": key_points,
                "decisions": decisions,
                "action_items": action_items,
                "meeting_classification": summary_result.get("classification", {}),
                "confidence_metrics": confidence_metrics,
                "word_count": word_count,
                "participant_count": len(participants),
                "processing_metadata": {
                    "generated_at": datetime.utcnow().isoformat() + "Z",
                    "model_used": self.models["summary"],
                    "processing_time": None,  # Will be set by caller
                    "chunk_count": 1 if word_count <= 2000 else (word_count // 2000) + 1
                }
            }

            print(f"✅ Summary generated: {len(key_points)} key points, {len(decisions)} decisions, {len(action_items)} action items")
            return result

        except Exception as e:
            print(f"❌ Summary generation failed: {str(e)}")
            return self._create_error_result(str(e))

    def _prepare_meeting_context(
        self,
        transcript_text: str,
        speaker_segments: List[Dict[str, Any]],
        meeting_metadata: Dict[str, Any],
        participants: List[str]
    ) -> Dict[str, Any]:
        """Prepare meeting context for AI analysis"""

        # Format speaker segments for better AI understanding
        formatted_segments = []
        for segment in speaker_segments:
            speaker = segment.get("speaker", "Unknown")
            text = segment.get("text", "").strip()
            timestamp = segment.get("start_time", 0)

            if text:
                formatted_segments.append({
                    "speaker": speaker,
                    "timestamp": f"{int(timestamp // 60):02d}:{int(timestamp % 60):02d}",
                    "text": text
                })

        return {
            "transcript_text": transcript_text,
            "speaker_segments": formatted_segments,
            "participants": participants,
            "meeting_metadata": {
                "filename": meeting_metadata.get("filename", "Unknown"),
                "duration": meeting_metadata.get("audio_duration", 0),
                "date": meeting_metadata.get("transcribed_at", ""),
                "privacy_level": meeting_metadata.get("privacy_level", "unknown")
            },
            "total_words": len(transcript_text.split()),
            "speaker_count": len(participants)
        }

    async def _generate_structured_summary(self, context: Dict[str, Any]) -> Dict[str, Any]:
        """Generate main structured summary"""

        # Build comprehensive prompt
        prompt = self._build_summary_prompt(context)

        try:
            response = await asyncio.to_thread(
                openai.ChatCompletion.create,
                model=self.models["summary"],
                messages=[
                    {
                        "role": "system",
                        "content": self._get_summary_system_prompt()
                    },
                    {
                        "role": "user",
                        "content": prompt
                    }
                ],
                temperature=0.3,  # Lower temperature for more consistent output
                max_tokens=1500
            )

            content = response.choices[0].message.content

            # Parse structured response
            try:
                parsed_response = json.loads(content)
                return parsed_response
            except json.JSONDecodeError:
                # Fallback to text parsing
                return {"summary": content, "classification": {"type": "general"}}

        except Exception as e:
            raise RuntimeError(f"Summary generation API call failed: {str(e)}")

    def _get_summary_system_prompt(self) -> str:
        """Get system prompt for summary generation"""

        return """You are an expert meeting analyst specializing in business communications.
        Your task is to create structured, accurate summaries of meeting transcripts.

        Generate summaries in JSON format with these fields:
        {
          "executive_summary": "2-3 sentence high-level overview",
          "summary": "Detailed summary covering all important points (200-400 words)",
          "classification": {
            "type": "strategy/tactical/status_update/decision_making/brainstorming/training/other",
            "formality": "formal/informal/mixed",
            "domain": "business area or topic",
            "urgency": "high/medium/low"
          },
          "tone_analysis": {
            "overall_sentiment": "positive/negative/neutral/mixed",
            "engagement_level": "high/medium/low",
            "conflict_indicators": "none/minor/moderate/significant"
          }
        }

        Focus on:
        - Business outcomes and implications
        - Clear, professional language
        - Factual accuracy without speculation
        - Proper context for decisions
        - Identification of follow-up needs"""

    def _build_summary_prompt(self, context: Dict[str, Any]) -> str:
        """Build user prompt for summary generation"""

        participants_str = ", ".join(context["participants"])
        duration_min = int(context["meeting_metadata"]["duration"] / 60)

        # Format speaker segments for prompt
        conversation_text = ""
        for segment in context["speaker_segments"][:20]:  # Limit to prevent token overflow
            conversation_text += f"[{segment['timestamp']}] {segment['speaker']}: {segment['text']}\n"

        prompt = f"""Please analyze this meeting transcript and generate a structured summary.

MEETING DETAILS:
- Participants: {participants_str}
- Duration: {duration_min} minutes
- Word Count: {context['total_words']} words
- Speakers: {context['speaker_count']}

TRANSCRIPT EXCERPT:
{conversation_text[:4000]}

{"..." if len(conversation_text) > 4000 else ""}

FULL TRANSCRIPT:
{context['transcript_text'][:6000]}

{"[TRANSCRIPT TRUNCATED]" if len(context['transcript_text']) > 6000 else ""}

Please provide a comprehensive JSON-formatted analysis focusing on business value and actionable insights."""

        return prompt

    async def _extract_key_points(
        self,
        context: Dict[str, Any],
        summary_result: Dict[str, Any]
    ) -> List[Dict[str, Any]]:
        """Extract structured key points from the meeting"""

        prompt = f"""Based on this meeting transcript, extract the most important key points.

TRANSCRIPT: {context['transcript_text'][:5000]}

Return a JSON array of key points with this structure:
[
  {{
    "point": "Clear, specific description of the key point",
    "category": "decision/announcement/discussion/concern/opportunity/update",
    "speakers": ["list", "of", "speakers", "involved"],
    "timestamp_reference": "approximate time or section",
    "importance": "high/medium/low",
    "business_impact": "Brief description of business relevance"
  }}
]

Focus on:
- Actionable information
- Strategic discussions
- Important announcements
- Significant concerns or risks
- New opportunities identified
- Status updates on critical items

Limit to 5-8 most important points."""

        try:
            response = await asyncio.to_thread(
                openai.ChatCompletion.create,
                model=self.models["extraction"],
                messages=[
                    {"role": "system", "content": "You are a business analyst extracting key insights from meetings. Return valid JSON only."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.2,
                max_tokens=1000
            )

            content = response.choices[0].message.content
            key_points = json.loads(content)

            # Add confidence scores
            for point in key_points:
                point["confidence"] = self._estimate_extraction_confidence(point, context)

            return key_points

        except Exception as e:
            print(f"⚠️ Key point extraction failed: {str(e)}")
            return []

    async def _extract_decisions(self, context: Dict[str, Any]) -> List[Dict[str, Any]]:
        """Extract decisions made during the meeting"""

        prompt = f"""Identify explicit decisions made in this meeting transcript.

TRANSCRIPT: {context['transcript_text'][:5000]}

Return a JSON array of decisions:
[
  {{
    "decision": "Clear statement of what was decided",
    "rationale": "Why this decision was made (if discussed)",
    "decision_maker": "Person who made or announced the decision",
    "affected_parties": ["people", "teams", "projects", "affected"],
    "implementation_timeline": "when to implement (if mentioned)",
    "confidence": "high/medium/low",
    "requires_followup": true/false
  }}
]

Only include explicit decisions, not general discussions or possibilities."""

        try:
            response = await asyncio.to_thread(
                openai.ChatCompletion.create,
                model=self.models["extraction"],
                messages=[
                    {"role": "system", "content": "Extract only explicit decisions from meeting transcripts. Return valid JSON."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.1,  # Very low for factual extraction
                max_tokens=800
            )

            content = response.choices[0].message.content
            decisions = json.loads(content)

            return decisions

        except Exception as e:
            print(f"⚠️ Decision extraction failed: {str(e)}")
            return []

    async def _extract_action_items(
        self,
        context: Dict[str, Any],
        participants: List[str]
    ) -> List[Dict[str, Any]]:
        """Extract action items and assignments"""

        participants_str = ", ".join(participants)

        prompt = f"""Extract action items and assignments from this meeting.

PARTICIPANTS: {participants_str}

TRANSCRIPT: {context['transcript_text'][:5000]}

Return JSON array of action items:
[
  {{
    "item": "Specific description of what needs to be done",
    "assignee": "Person assigned (must be from participant list or 'unassigned')",
    "deadline": "mentioned deadline or 'not specified'",
    "priority": "high/medium/low/not specified",
    "context": "Why this action is needed",
    "dependencies": ["other", "tasks", "or", "people", "this", "depends", "on"],
    "status": "new",
    "confidence": "high/medium/low"
  }}
]

Only extract explicit action items or clear assignments. If unsure about assignee, mark as 'unassigned'."""

        try:
            response = await asyncio.to_thread(
                openai.ChatCompletion.create,
                model=self.models["extraction"],
                messages=[
                    {"role": "system", "content": "Extract action items and assignments from meetings. Return valid JSON only."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.1,
                max_tokens=1000
            )

            content = response.choices[0].message.content
            action_items = json.loads(content)

            # Validate assignees against participant list
            for item in action_items:
                assignee = item.get("assignee", "").lower()
                if assignee not in [p.lower() for p in participants] and assignee != "unassigned":
                    item["assignee"] = "unassigned"
                    item["confidence"] = "low"

            return action_items

        except Exception as e:
            print(f"⚠️ Action item extraction failed: {str(e)}")
            return []

    def _calculate_confidence_scores(
        self,
        summary_result: Dict[str, Any],
        key_points: List[Dict[str, Any]],
        decisions: List[Dict[str, Any]],
        action_items: List[Dict[str, Any]],
        word_count: int
    ) -> Dict[str, Any]:
        """Calculate confidence scores for AI-generated content"""

        # Base confidence on transcript length and structure
        length_confidence = min(1.0, word_count / 500)  # Full confidence at 500+ words

        # Structure confidence based on extracted elements
        structure_score = 0.0
        if key_points:
            structure_score += 0.4
        if decisions:
            structure_score += 0.3
        if action_items:
            structure_score += 0.3

        # Overall confidence
        overall_confidence = (length_confidence * 0.6) + (structure_score * 0.4)

        return {
            "overall_confidence": round(overall_confidence, 2),
            "summary_confidence": round(length_confidence, 2),
            "structure_confidence": round(structure_score, 2),
            "key_points_count": len(key_points),
            "decisions_count": len(decisions),
            "action_items_count": len(action_items),
            "quality_indicators": {
                "sufficient_length": word_count >= self.min_transcript_length,
                "structured_content": structure_score > 0.5,
                "actionable_outcomes": len(action_items) > 0 or len(decisions) > 0
            }
        }

    def _estimate_extraction_confidence(self, item: Dict[str, Any], context: Dict[str, Any]) -> str:
        """Estimate confidence for extracted items"""

        # Simple heuristic based on specificity and context
        text_length = len(item.get("point", item.get("item", "")))
        has_specifics = any(keyword in str(item).lower() for keyword in
                          ["timeline", "deadline", "specific", "will", "must", "decided"])

        if text_length > 50 and has_specifics:
            return "high"
        elif text_length > 25:
            return "medium"
        else:
            return "low"

    def _create_error_result(self, error_message: str) -> Dict[str, Any]:
        """Create error result for failed summary generation"""

        return {
            "error": error_message,
            "summary_text": "",
            "key_points": [],
            "decisions": [],
            "action_items": [],
            "confidence_metrics": {
                "overall_confidence": 0.0,
                "error": True
            }
        }

    def _create_minimal_summary(self, transcript_text: str, context: Dict[str, Any]) -> Dict[str, Any]:
        """Create minimal summary for very short transcripts"""

        word_count = len(transcript_text.split())

        return {
            "summary_text": f"Brief meeting transcript with {word_count} words. " +
                           "Transcript too short for detailed AI analysis.",
            "executive_summary": "Short meeting - manual review recommended",
            "key_points": [],
            "decisions": [],
            "action_items": [],
            "meeting_classification": {
                "type": "brief",
                "formality": "unknown",
                "domain": "unknown",
                "urgency": "low"
            },
            "confidence_metrics": {
                "overall_confidence": 0.1,
                "summary_confidence": 0.1,
                "insufficient_content": True,
                "word_count": word_count,
                "minimum_required": self.min_transcript_length
            }
        }

    def is_available(self) -> bool:
        """Check if summarization service is available"""
        return self.client_available

    async def test_summarization(self, test_transcript: str = None) -> Dict[str, Any]:
        """Test summarization functionality"""

        if not self.client_available:
            return {"available": False, "error": "OpenAI API key not configured"}

        test_text = test_transcript or "This is a test meeting. We discussed the project status and decided to proceed with phase two. John will handle the implementation by Friday."

        try:
            context = self._prepare_meeting_context(
                test_text,
                [{"speaker": "Test", "text": test_text, "start_time": 0}],
                {"filename": "test", "duration": 60},
                ["Test User"]
            )

            result = await self._generate_structured_summary(context)

            return {
                "available": True,
                "test_successful": True,
                "model_used": self.models["summary"],
                "summary_length": len(result.get("summary", "")),
                "response_structure": list(result.keys())
            }

        except Exception as e:
            return {
                "available": False,
                "test_successful": False,
                "error": str(e)
            }


# Global summarizer instance
meeting_summarizer = MeetingSummarizer()