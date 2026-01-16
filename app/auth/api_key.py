"""
API Key authentication for Penfold FastAPI application.

Provides simple API key authentication using Bearer token format.
In production, requires PENF_API_KEY environment variable to be set.
"""

import os
import secrets

from fastapi import Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

from penf_lib.storage.config import config as penf_config

# Security scheme for OpenAPI documentation
security = HTTPBearer(auto_error=False)

# Environment variable name for the API key
API_KEY_ENV_VAR = "PENF_API_KEY"


class AuthenticationError(Exception):
    """Raised when authentication fails."""
    pass


def get_api_key() -> str | None:
    """
    Get the configured API key from environment.

    Returns:
        The API key if set, None otherwise.
    """
    return os.getenv(API_KEY_ENV_VAR)


def validate_api_key(provided_key: str) -> bool:
    """
    Validate an API key using timing-safe comparison.

    Uses secrets.compare_digest() to prevent timing attacks.

    Args:
        provided_key: The API key to validate.

    Returns:
        True if the key is valid, False otherwise.
    """
    expected_key = get_api_key()

    if expected_key is None:
        # No key configured - reject all attempts
        return False

    if not provided_key:
        return False

    # Use timing-safe comparison to prevent timing attacks
    return secrets.compare_digest(provided_key, expected_key)


def verify_api_key_configured() -> None:
    """
    Verify that the API key is configured.

    Should be called at application startup.
    Raises an error in production if PENF_API_KEY is not set.

    Raises:
        RuntimeError: If running in production without API key configured.
    """
    api_key = get_api_key()

    if penf_config.is_production and api_key is None:
        raise RuntimeError(
            f"Production environment requires {API_KEY_ENV_VAR} to be set. "
            "Set PENFOLD_ENV to 'development' or 'testing' to bypass this check."
        )

    if api_key is None:
        import logging
        logging.getLogger(__name__).warning(
            f"{API_KEY_ENV_VAR} is not set. API authentication is disabled. "
            "This is acceptable for development but must be configured for production."
        )


async def get_current_user(
    credentials: HTTPAuthorizationCredentials | None = Depends(security),
) -> dict:
    """
    FastAPI dependency that validates the API key and returns user info.

    Extracts the API key from the Authorization header (Bearer token format)
    and validates it.

    Args:
        credentials: The HTTP authorization credentials from the request.

    Returns:
        A dict containing user information with admin role.

    Raises:
        HTTPException: 401 if authentication fails.
    """
    # Check if API key is configured
    expected_key = get_api_key()

    # In development without API key, allow all requests
    if expected_key is None and not penf_config.is_production:
        return {
            "authenticated": True,
            "role": "admin",
            "auth_method": "none",
            "message": "Development mode - authentication bypassed"
        }

    # Require credentials when API key is configured
    if credentials is None:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Missing authentication credentials",
            headers={"WWW-Authenticate": "Bearer"},
        )

    # Validate the provided API key
    if not validate_api_key(credentials.credentials):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid API key",
            headers={"WWW-Authenticate": "Bearer"},
        )

    # All authenticated users are admin for now
    return {
        "authenticated": True,
        "role": "admin",
        "auth_method": "api_key",
    }


# Type alias for use in route dependencies
CurrentUser = dict
