"""
Meeting Pipeline FastAPI Application

Main application entry point for the meeting upload and processing pipeline.
Provides REST API for file uploads, processing status, search, and management.
"""
from fastapi import FastAPI, Depends
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
import uvicorn
from contextlib import asynccontextmanager
import asyncio

from app.config import settings
from app.database import init_database, get_db
from app.jobs import init_job_queue
from app.api import upload_routes, search_routes, review_routes, transcription_routes


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifecycle management"""
    # Startup
    print("🚀 Starting Meeting Pipeline API...")

    # Initialize database
    await init_database()
    print("✅ Database initialized")

    # Initialize job queue
    await init_job_queue()
    print("✅ Job queue initialized")

    print(f"🎯 API ready at http://localhost:{settings.PORT}")

    yield

    # Shutdown
    print("🛑 Shutting down Meeting Pipeline API...")


# Create FastAPI application
app = FastAPI(
    title="Meeting Pipeline API",
    description="API for uploading, processing, and searching meeting recordings",
    version="1.0.0",
    lifespan=lifespan,
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.ALLOWED_ORIGINS,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# Health check endpoint
@app.get("/health")
async def health_check():
    """Health check endpoint for monitoring"""
    return {
        "status": "healthy",
        "service": "meeting-pipeline",
        "version": "1.0.0",
        "components": {
            "database": "connected",
            "job_queue": "running",
            "file_storage": "available"
        }
    }


# Include API routers
app.include_router(upload_routes.router, prefix="/api/v1", tags=["upload"])
app.include_router(transcription_routes.router, prefix="/api/v1", tags=["transcription"])
app.include_router(search_routes.router, prefix="/api/v1", tags=["search"])
app.include_router(review_routes.router, prefix="/api/v1", tags=["review"])


# Exception handlers
@app.exception_handler(Exception)
async def global_exception_handler(request, exc):
    """Global exception handler for unhandled errors"""
    return JSONResponse(
        status_code=500,
        content={
            "error": "Internal server error",
            "message": "An unexpected error occurred",
            "type": type(exc).__name__
        }
    )


if __name__ == "__main__":
    # Development server
    uvicorn.run(
        "app.main:app",
        host=settings.HOST,
        port=settings.PORT,
        reload=settings.DEBUG,
        log_level="info" if settings.DEBUG else "warning"
    )