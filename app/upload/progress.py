"""
Upload Progress Tracking with Server-Sent Events

Manages real-time progress updates for file uploads and processing using
Server-Sent Events (SSE) for live updates to client applications.
"""
from fastapi import Request
from fastapi.responses import StreamingResponse
from typing import Dict, Any, Optional, AsyncGenerator
import asyncio
import json
import time
from datetime import datetime
import uuid
from collections import defaultdict

from app.config import settings


class ProgressEvent:
    """Represents a progress update event"""

    def __init__(
        self,
        session_id: str,
        progress_percentage: int,
        stage: str,
        message: Optional[str] = None,
        data: Optional[Dict[str, Any]] = None
    ):
        self.session_id = session_id
        self.progress_percentage = progress_percentage
        self.stage = stage
        self.message = message
        self.data = data or {}
        self.timestamp = datetime.utcnow()
        self.event_id = str(uuid.uuid4())

    def to_sse_format(self) -> str:
        """Convert to Server-Sent Events format"""
        event_data = {
            "session_id": self.session_id,
            "progress": self.progress_percentage,
            "stage": self.stage,
            "message": self.message,
            "timestamp": self.timestamp.isoformat() + "Z",
            "data": self.data
        }

        return f"id: {self.event_id}\ndata: {json.dumps(event_data)}\n\n"


class ProgressTracker:
    """Tracks and broadcasts upload/processing progress"""

    def __init__(self):
        # Store active sessions and their progress
        self.sessions: Dict[str, Dict[str, Any]] = {}

        # Store event queues for each client connection
        self.client_queues: Dict[str, asyncio.Queue] = {}

        # Track which sessions each client is interested in
        self.client_subscriptions: Dict[str, set] = defaultdict(set)

        # Lock for thread-safe operations
        self._lock = asyncio.Lock()

    async def update_progress(
        self,
        session_id: str,
        progress_percentage: int,
        stage: str,
        message: Optional[str] = None,
        data: Optional[Dict[str, Any]] = None
    ):
        """Update progress for a session and notify clients"""

        async with self._lock:
            # Update session progress
            if session_id not in self.sessions:
                self.sessions[session_id] = {
                    "created_at": datetime.utcnow(),
                    "last_update": datetime.utcnow(),
                    "progress": 0,
                    "stage": "initializing",
                    "history": []
                }

            session = self.sessions[session_id]
            session["progress"] = progress_percentage
            session["stage"] = stage
            session["last_update"] = datetime.utcnow()

            # Add to history
            history_entry = {
                "progress": progress_percentage,
                "stage": stage,
                "message": message,
                "timestamp": session["last_update"].isoformat(),
                "data": data
            }
            session["history"].append(history_entry)

            # Limit history size
            if len(session["history"]) > 100:
                session["history"] = session["history"][-50:]

            # Create progress event
            event = ProgressEvent(session_id, progress_percentage, stage, message, data)

            # Broadcast to all clients subscribed to this session
            await self._broadcast_event(session_id, event)

    async def _broadcast_event(self, session_id: str, event: ProgressEvent):
        """Broadcast event to all subscribed clients"""

        # Find all clients subscribed to this session
        for client_id, subscriptions in self.client_subscriptions.items():
            if session_id in subscriptions:
                if client_id in self.client_queues:
                    try:
                        await self.client_queues[client_id].put(event)
                    except Exception as e:
                        print(f"Error broadcasting to client {client_id}: {e}")

    async def subscribe_to_session(self, client_id: str, session_id: str):
        """Subscribe a client to progress updates for a session"""

        async with self._lock:
            if client_id not in self.client_queues:
                self.client_queues[client_id] = asyncio.Queue(maxsize=1000)

            self.client_subscriptions[client_id].add(session_id)

            # Send current progress if session exists
            if session_id in self.sessions:
                session = self.sessions[session_id]
                current_event = ProgressEvent(
                    session_id=session_id,
                    progress_percentage=session["progress"],
                    stage=session["stage"],
                    message="Current status",
                    data={"type": "current_status"}
                )
                await self.client_queues[client_id].put(current_event)

    async def unsubscribe_client(self, client_id: str):
        """Unsubscribe a client from all sessions"""

        async with self._lock:
            if client_id in self.client_queues:
                del self.client_queues[client_id]

            if client_id in self.client_subscriptions:
                del self.client_subscriptions[client_id]

    async def stream_progress(self, session_id: str, request: Request) -> StreamingResponse:
        """Create SSE stream for progress updates"""

        client_id = str(uuid.uuid4())

        async def event_generator() -> AsyncGenerator[str, None]:
            try:
                # Subscribe to session
                await self.subscribe_to_session(client_id, session_id)

                # Send initial connection message
                initial_event = ProgressEvent(
                    session_id=session_id,
                    progress_percentage=0,
                    stage="connected",
                    message="Connected to progress stream",
                    data={"type": "connection_established"}
                )
                yield initial_event.to_sse_format()

                # Stream events
                while True:
                    # Check if client disconnected
                    if await request.is_disconnected():
                        break

                    try:
                        # Wait for next event with timeout
                        event = await asyncio.wait_for(
                            self.client_queues[client_id].get(),
                            timeout=30.0  # Send keepalive every 30 seconds
                        )
                        yield event.to_sse_format()

                    except asyncio.TimeoutError:
                        # Send keepalive
                        keepalive_event = ProgressEvent(
                            session_id=session_id,
                            progress_percentage=-1,
                            stage="keepalive",
                            message="Connection active",
                            data={"type": "keepalive"}
                        )
                        yield keepalive_event.to_sse_format()

            except Exception as e:
                # Send error event
                error_event = ProgressEvent(
                    session_id=session_id,
                    progress_percentage=-1,
                    stage="error",
                    message=f"Stream error: {str(e)}",
                    data={"type": "error"}
                )
                yield error_event.to_sse_format()

            finally:
                # Cleanup
                await self.unsubscribe_client(client_id)

        return StreamingResponse(
            event_generator(),
            media_type="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Accel-Buffering": "no",  # Disable Nginx buffering
            }
        )

    async def get_session_status(self, session_id: str) -> Optional[Dict[str, Any]]:
        """Get current status for a session"""

        async with self._lock:
            if session_id not in self.sessions:
                return None

            session = self.sessions[session_id]
            return {
                "session_id": session_id,
                "progress": session["progress"],
                "stage": session["stage"],
                "created_at": session["created_at"].isoformat() + "Z",
                "last_update": session["last_update"].isoformat() + "Z",
                "history_count": len(session["history"])
            }

    async def get_session_history(
        self,
        session_id: str,
        limit: int = 50
    ) -> Optional[Dict[str, Any]]:
        """Get progress history for a session"""

        async with self._lock:
            if session_id not in self.sessions:
                return None

            session = self.sessions[session_id]
            history = session["history"][-limit:] if limit > 0 else session["history"]

            return {
                "session_id": session_id,
                "total_events": len(session["history"]),
                "returned_events": len(history),
                "history": history
            }

    async def cleanup_old_sessions(self, max_age_hours: int = 24):
        """Clean up old session data"""

        cutoff_time = datetime.utcnow().timestamp() - (max_age_hours * 3600)
        sessions_to_remove = []

        async with self._lock:
            for session_id, session in self.sessions.items():
                if session["last_update"].timestamp() < cutoff_time:
                    sessions_to_remove.append(session_id)

            for session_id in sessions_to_remove:
                del self.sessions[session_id]

        return len(sessions_to_remove)

    async def get_active_sessions(self) -> Dict[str, Any]:
        """Get information about active sessions"""

        async with self._lock:
            return {
                "total_sessions": len(self.sessions),
                "total_clients": len(self.client_queues),
                "active_subscriptions": sum(len(subs) for subs in self.client_subscriptions.values()),
                "sessions": {
                    session_id: {
                        "progress": session["progress"],
                        "stage": session["stage"],
                        "last_update": session["last_update"].isoformat() + "Z"
                    }
                    for session_id, session in self.sessions.items()
                }
            }

    async def send_custom_event(
        self,
        session_id: str,
        event_type: str,
        message: str,
        data: Optional[Dict[str, Any]] = None
    ):
        """Send a custom event to session subscribers"""

        event_data = {"type": event_type, **(data or {})}
        event = ProgressEvent(
            session_id=session_id,
            progress_percentage=-1,  # Special value for custom events
            stage=event_type,
            message=message,
            data=event_data
        )

        await self._broadcast_event(session_id, event)

    async def mark_session_complete(self, session_id: str, success: bool = True):
        """Mark a session as completed"""

        stage = "completed" if success else "failed"
        progress = 100 if success else -1
        message = "Processing completed successfully" if success else "Processing failed"

        await self.update_progress(
            session_id=session_id,
            progress_percentage=progress,
            stage=stage,
            message=message,
            data={"type": "final_status", "success": success}
        )


# Global progress tracker instance
progress_tracker = ProgressTracker()


async def periodic_cleanup():
    """Periodic cleanup of old progress data"""
    while True:
        try:
            await progress_tracker.cleanup_old_sessions()
            await asyncio.sleep(3600)  # Run every hour
        except Exception as e:
            print(f"Error in progress cleanup: {e}")
            await asyncio.sleep(300)  # Retry in 5 minutes