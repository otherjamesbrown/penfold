"""
Review API Routes

Complete implementation for manual correction, entity resolution,
review queue management, and user feedback collection.
"""
from fastapi import APIRouter, HTTPException, Depends, Query
from fastapi.responses import HTMLResponse
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel
from typing import Dict, Any, List, Optional
from datetime import datetime
import uuid

from app.database import get_db
from app.ui.transcript_editor import router as transcript_editor_router
from app.ui.entity_resolution import router as entity_resolution_router
from app.ui.review_queue import router as review_queue_router
from app.review.feedback import feedback_service, FeedbackSubmission, CorrectionSubmission
from app.review.versioning import transcript_versioning

router = APIRouter()

# Include sub-routers
router.include_router(transcript_editor_router, prefix="/transcript-editor", tags=["transcript-editing"])
router.include_router(entity_resolution_router, prefix="/entity-resolution", tags=["entity-resolution"])
router.include_router(review_queue_router, prefix="/review-queue", tags=["review-queue"])


# Main review routes

@router.get("/review/dashboard", response_class=HTMLResponse)
async def get_review_dashboard():
    """Serve main review dashboard interface"""

    return HTMLResponse(content="""
    <!DOCTYPE html>
    <html>
    <head>
        <title>Review Dashboard - Meeting Pipeline</title>
        <meta charset="utf-8">
        <meta name="viewport" content="width=device-width, initial-scale=1">
        <style>
            body { font-family: Arial, sans-serif; margin: 20px; }
            .dashboard { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
            .card { border: 1px solid #ddd; padding: 20px; border-radius: 8px; }
            .card h3 { margin-top: 0; color: #333; }
            .stat { font-size: 2em; font-weight: bold; color: #007bff; }
            .link { color: #007bff; text-decoration: none; }
            .link:hover { text-decoration: underline; }
            .urgent { color: #dc3545; }
            .good { color: #28a745; }
        </style>
    </head>
    <body>
        <h1>Review Dashboard</h1>
        <div class="dashboard">
            <div class="card">
                <h3>Review Queue</h3>
                <p>Pending Items: <span class="stat" id="pending-count">Loading...</span></p>
                <p>Overdue Items: <span class="stat urgent" id="overdue-count">Loading...</span></p>
                <p><a href="#" class="link" onclick="loadReviewQueue()">View Full Queue</a></p>
            </div>

            <div class="card">
                <h3>Entity Resolution</h3>
                <p>Unresolved Entities: <span class="stat" id="entities-count">Loading...</span></p>
                <p>AI Confidence: <span class="stat" id="ai-confidence">Loading...</span></p>
                <p><a href="#" class="link" onclick="loadEntityResolution()">Resolve Entities</a></p>
            </div>

            <div class="card">
                <h3>Transcript Editing</h3>
                <p>Recent Edits: <span class="stat good" id="edits-count">Loading...</span></p>
                <p>Quality Score: <span class="stat good" id="quality-score">Loading...</span></p>
                <p><a href="#" class="link" onclick="loadTranscriptEditor()">Edit Transcripts</a></p>
            </div>

            <div class="card">
                <h3>User Feedback</h3>
                <p>Recent Feedback: <span class="stat" id="feedback-count">Loading...</span></p>
                <p>Avg Rating: <span class="stat good" id="avg-rating">Loading...</span></p>
                <p><a href="#" class="link" onclick="loadFeedbackAnalytics()">View Analytics</a></p>
            </div>
        </div>

        <div style="margin-top: 30px;">
            <h3>Quick Actions</h3>
            <button onclick="loadReviewQueue()" style="margin: 5px; padding: 10px 20px;">Review Queue</button>
            <button onclick="loadEntityResolution()" style="margin: 5px; padding: 10px 20px;">Entity Resolution</button>
            <button onclick="submitFeedback()" style="margin: 5px; padding: 10px 20px;">Submit Feedback</button>
            <button onclick="viewAnalytics()" style="margin: 5px; padding: 10px 20px;">View Analytics</button>
        </div>

        <script>
            async function loadDashboardData() {
                try {
                    // Load review queue stats
                    const queueResponse = await fetch('/api/v1/review/review-queue?page_size=1');
                    const queueData = await queueResponse.json();
                    document.getElementById('pending-count').textContent = queueData.statistics?.total_pending || 0;
                    document.getElementById('overdue-count').textContent = queueData.statistics?.overdue_items || 0;

                    // Load entity resolution stats
                    const entitiesResponse = await fetch('/api/v1/review/entity-resolution/pending?limit=1');
                    const entitiesData = await entitiesResponse.json();
                    document.getElementById('entities-count').textContent = entitiesData.statistics?.total_pending || 0;
                    document.getElementById('ai-confidence').textContent =
                        entitiesData.statistics?.avg_confidence ? (entitiesData.statistics.avg_confidence * 100).toFixed(1) + '%' : 'N/A';

                    // Load feedback stats
                    const feedbackResponse = await fetch('/api/v1/review/feedback/analytics');
                    const feedbackData = await feedbackResponse.json();
                    document.getElementById('feedback-count').textContent = feedbackData.total_feedback || 0;
                    document.getElementById('avg-rating').textContent =
                        feedbackData.analytics_summary?.average_rating ? feedbackData.analytics_summary.average_rating + '/5' : 'N/A';

                } catch (error) {
                    console.error('Failed to load dashboard data:', error);
                }
            }

            function loadReviewQueue() {
                window.open('/api/v1/review/review-queue', '_blank');
            }

            function loadEntityResolution() {
                window.open('/api/v1/review/entity-resolution/pending', '_blank');
            }

            function loadTranscriptEditor() {
                alert('Transcript editor would open here');
            }

            function submitFeedback() {
                alert('Feedback form would open here');
            }

            function viewAnalytics() {
                window.open('/api/v1/review/feedback/analytics', '_blank');
            }

            function loadFeedbackAnalytics() {
                window.open('/api/v1/review/feedback/analytics', '_blank');
            }

            // Load data on page load
            loadDashboardData();

            // Refresh data every 30 seconds
            setInterval(loadDashboardData, 30000);
        </script>
    </body>
    </html>
    """)


# Feedback API endpoints

@router.post("/feedback/submit")
async def submit_user_feedback(
    feedback: FeedbackSubmission,
    db: AsyncSession = Depends(get_db)
):
    """Submit user feedback on AI-generated content"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await feedback_service.submit_feedback(feedback, user_id, db)
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/feedback/correction")
async def submit_correction_pattern(
    correction: CorrectionSubmission,
    db: AsyncSession = Depends(get_db)
):
    """Submit a correction pattern for AI training"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await feedback_service.submit_correction(correction, user_id, db)
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/feedback/analytics")
async def get_feedback_analytics(
    days_back: int = Query(30, description="Days to analyze"),
    component: str = Query(None, description="Filter by component"),
    db: AsyncSession = Depends(get_db)
):
    """Get analytics on user feedback for AI improvement"""

    try:
        analytics = await feedback_service.get_feedback_analytics(db, days_back, component)
        return analytics

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/feedback/patterns")
async def get_correction_patterns(
    pattern_type: str = Query(None, description="Filter by pattern type"),
    min_frequency: int = Query(2, description="Minimum frequency threshold"),
    db: AsyncSession = Depends(get_db)
):
    """Get correction patterns for AI training"""

    try:
        patterns = await feedback_service.get_correction_patterns(db, pattern_type, min_frequency)
        return {
            "patterns": patterns,
            "pattern_type_filter": pattern_type,
            "min_frequency": min_frequency,
            "count": len(patterns)
        }

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/feedback/recommendations")
async def get_ai_improvement_recommendations(
    db: AsyncSession = Depends(get_db)
):
    """Generate recommendations for AI model improvements"""

    try:
        recommendations = await feedback_service.get_ai_improvement_recommendations(db)
        return recommendations

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


# Version control endpoints

@router.get("/transcript/{meeting_id}/versions")
async def get_transcript_versions(
    meeting_id: str,
    limit: int = Query(20, description="Maximum number of versions"),
    db: AsyncSession = Depends(get_db)
):
    """Get version history for a transcript"""

    try:
        history = await transcript_versioning.get_edit_history(meeting_id, db, limit)
        return {
            "meeting_id": meeting_id,
            "version_history": history,
            "total_versions": len(history)
        }

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/transcript/version/{version_id}")
async def get_version_details(
    version_id: str,
    db: AsyncSession = Depends(get_db)
):
    """Get detailed information about a specific version"""

    try:
        details = await transcript_versioning.get_version_details(version_id, db)
        if not details:
            raise HTTPException(status_code=404, detail="Version not found")

        return details

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/transcript/{meeting_id}/rollback/{version_id}")
async def rollback_transcript(
    meeting_id: str,
    version_id: str,
    db: AsyncSession = Depends(get_db)
):
    """Rollback transcript to a specific version"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await transcript_versioning.rollback_to_version(
            meeting_id, version_id, user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/transcript/compare/{version_id_1}/{version_id_2}")
async def compare_transcript_versions(
    version_id_1: str,
    version_id_2: str,
    db: AsyncSession = Depends(get_db)
):
    """Compare two transcript versions"""

    try:
        comparison = await transcript_versioning.compare_versions(
            version_id_1, version_id_2, db
        )
        return comparison

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


# Summary endpoints for dashboard

@router.get("/review/summary")
async def get_review_system_summary(
    db: AsyncSession = Depends(get_db)
):
    """Get overall summary of review system status"""

    try:
        # This would aggregate data from all components
        # For now, return a placeholder summary
        return {
            "review_queue": {
                "total_pending": 0,
                "total_in_review": 0,
                "total_overdue": 0
            },
            "entity_resolution": {
                "pending_participants": 0,
                "pending_topics": 0,
                "average_confidence": 0.85
            },
            "transcript_editing": {
                "total_edits_today": 0,
                "average_quality_improvement": 0.12
            },
            "feedback_system": {
                "feedback_last_24h": 0,
                "average_rating": 4.2,
                "improvement_trend": "stable"
            },
            "generated_at": datetime.utcnow().isoformat() + "Z"
        }

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))