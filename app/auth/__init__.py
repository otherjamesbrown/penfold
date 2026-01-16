"""
Authentication module for Penfold FastAPI application.

Provides API key authentication with Bearer token support.

Usage:
    from app.auth import get_current_user, validate_api_key, verify_api_key_configured

    # As a FastAPI dependency
    @router.get("/protected")
    async def protected_route(user: dict = Depends(get_current_user)):
        return {"message": "Authenticated", "user": user}

    # At application startup
    verify_api_key_configured()  # Raises in production if PENF_API_KEY not set
"""

from app.auth.api_key import (
    API_KEY_ENV_VAR,
    AuthenticationError,
    CurrentUser,
    get_api_key,
    get_current_user,
    validate_api_key,
    verify_api_key_configured,
)

__all__ = [
    "get_current_user",
    "validate_api_key",
    "verify_api_key_configured",
    "get_api_key",
    "AuthenticationError",
    "CurrentUser",
    "API_KEY_ENV_VAR",
]
