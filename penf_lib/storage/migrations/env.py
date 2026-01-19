"""Alembic migration environment configuration."""

import os
from logging.config import fileConfig
from pathlib import Path

from sqlalchemy import pool
from sqlalchemy.engine import Connection

from alembic import context

# Add project root to path for imports
project_root = Path(__file__).parent.parent.parent.parent
import sys
sys.path.insert(0, str(project_root))

# Import models and configuration
from penf_lib.storage.models import Base
from penf_lib.storage.config import config as app_config

# this is the Alembic Config object, which provides
# access to the values within the .ini file in use.
config = context.config

# Interpret the config file for Python logging.
# This line sets up loggers basically.
if config.config_file_name is not None:
    fileConfig(config.config_file_name)

# Set the SQLAlchemy URL from our configuration
config.set_main_option("sqlalchemy.url", app_config.database.sync_database_url)

# add your model's MetaData object here
# for 'autogenerate' support
target_metadata = Base.metadata

# other values from the config, defined by the needs of env.py,
# can be acquired:
# my_important_option = config.get_main_option("my_important_option")
# ... etc.


def include_name(name, type_, parent_names):
    """
    Filter which database objects to include in migrations.

    This function is called by Alembic to determine if a database object
    should be included when generating migrations.
    """
    if type_ == "table":
        # Include all tables defined in our models
        return name in target_metadata.tables

    # Include all other objects (indexes, constraints, etc.)
    return True


def run_migrations_offline() -> None:
    """
    Run migrations in 'offline' mode.

    This configures the context with just a URL
    and not an Engine, though an Engine is acceptable
    here as well.  By skipping the Engine creation
    we don't even need a DBAPI to be available.

    Calls to context.execute() here emit the given string to the
    script output.
    """
    url = config.get_main_option("sqlalchemy.url")
    context.configure(
        url=url,
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
        include_name=include_name,
        compare_type=True,
        compare_server_default=True,
        render_as_batch=True,  # Enable batch mode for better compatibility
    )

    with context.begin_transaction():
        context.run_migrations()






def run_migrations_online() -> None:
    """
    Run migrations in 'online' mode.

    In this scenario we need to create an Engine
    and associate a connection with the context.
    """
    # Use synchronous engine for migrations
    from sqlalchemy import create_engine, pool

    connectable = create_engine(
        config.get_main_option("sqlalchemy.url"),
        poolclass=pool.NullPool,
    )

    with connectable.connect() as connection:
        context.configure(
            connection=connection,
            target_metadata=target_metadata,
            include_name=include_name,
            compare_type=True,
            compare_server_default=True,
            render_as_batch=True,  # Enable batch mode for better compatibility
        )

        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()