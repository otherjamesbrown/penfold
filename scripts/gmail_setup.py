#!/usr/bin/env python3
"""Gmail OAuth2 setup script for initial account connection."""

import asyncio
import json
import os
import sys
from pathlib import Path
from typing import Dict, Any

import click
from sqlalchemy import select

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from penf_lib.connectors.gmail.auth import GmailAuthenticator
from penf_lib.storage.database import DatabaseManager, db_manager
from penf_lib.models.gmail_connection import GmailConnection


async def setup_gmail_account(
    client_config_path: str,
    account_email: str,
    account_name: str = None
) -> None:
    """Set up Gmail account with OAuth2 authentication.

    Args:
        client_config_path: Path to OAuth2 client configuration JSON
        account_email: Gmail account email address
        account_name: Optional display name for the account
    """
    # Load client configuration
    with open(client_config_path, 'r') as f:
        client_config = json.load(f)

    # Initialize authenticator
    authenticator = GmailAuthenticator(client_config)

    print(f"Setting up Gmail account: {account_email}")

    try:
        # Start OAuth2 flow
        auth_url = await authenticator.initiate_auth_flow()
        print(f"\nPlease visit this URL to authorize access:")
        print(f"\n{auth_url}\n")

        # Wait for user to complete authorization
        authorization_response = input(
            "After authorization, copy the full callback URL and paste it here: "
        ).strip()

        if not authorization_response:
            print("No authorization response provided. Exiting.")
            return

        # Complete OAuth2 flow
        print("Completing authorization...")
        credentials = await authenticator.complete_auth_flow(authorization_response)

        # Encrypt credentials for storage
        encrypted_credentials = await authenticator.encrypt_credentials(credentials)

        # Store connection in database
        async with db_manager.get_session() as session:
            # Check if connection already exists
            existing = await session.execute(
                select(GmailConnection).where(
                    GmailConnection.account_email == account_email
                )
            )
            connection = existing.scalar_one_or_none()

            if connection:
                print(f"Updating existing connection for {account_email}")
                connection.encrypted_credentials = encrypted_credentials
                connection.account_name = account_name or connection.account_name
                connection.is_active = True
            else:
                print(f"Creating new connection for {account_email}")
                connection = GmailConnection(
                    account_email=account_email,
                    account_name=account_name or account_email,
                    encrypted_credentials=encrypted_credentials,
                    is_active=True
                )
                session.add(connection)

            await session.commit()

        print(f"✅ Gmail account {account_email} successfully configured!")
        print(f"Connection ID: {connection.id}")

    except Exception as e:
        print(f"❌ Error setting up Gmail account: {e}")
        raise


async def list_gmail_connections() -> None:
    """List all configured Gmail connections."""
    async with db_manager.get_session() as session:
        result = await session.execute(select(GmailConnection))
        connections = result.scalars().all()

        if not connections:
            print("No Gmail connections configured.")
            return

        print("\nConfigured Gmail connections:")
        print("-" * 60)
        for conn in connections:
            status = "✅ Active" if conn.is_active else "❌ Inactive"
            last_sync = conn.last_sync.strftime("%Y-%m-%d %H:%M:%S") if conn.last_sync else "Never"
            print(f"ID: {conn.id}")
            print(f"Email: {conn.account_email}")
            print(f"Name: {conn.account_name}")
            print(f"Status: {status}")
            print(f"Last Sync: {last_sync}")
            print("-" * 60)


async def remove_gmail_connection(account_email: str) -> None:
    """Remove Gmail connection.

    Args:
        account_email: Email address of account to remove
    """
    async with db_manager.get_session() as session:
        result = await session.execute(
            select(GmailConnection).where(
                GmailConnection.account_email == account_email
            )
        )
        connection = result.scalar_one_or_none()

        if not connection:
            print(f"❌ No connection found for {account_email}")
            return

        await session.delete(connection)
        await session.commit()

        print(f"✅ Gmail connection for {account_email} removed successfully!")


@click.group()
def cli():
    """Gmail setup and management commands."""
    pass


@cli.command()
@click.option(
    '--config',
    '-c',
    required=True,
    help='Path to OAuth2 client configuration JSON file'
)
@click.option(
    '--email',
    '-e',
    required=True,
    help='Gmail account email address'
)
@click.option(
    '--name',
    '-n',
    help='Display name for the account'
)
def add(config: str, email: str, name: str):
    """Add new Gmail account connection."""
    asyncio.run(setup_gmail_account(config, email, name))


@cli.command()
def list():
    """List all Gmail connections."""
    asyncio.run(list_gmail_connections())


@cli.command()
@click.option(
    '--email',
    '-e',
    required=True,
    help='Gmail account email address to remove'
)
def remove(email: str):
    """Remove Gmail connection."""
    if click.confirm(f"Are you sure you want to remove connection for {email}?"):
        asyncio.run(remove_gmail_connection(email))


if __name__ == "__main__":
    cli()