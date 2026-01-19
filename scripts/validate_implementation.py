#!/usr/bin/env python3.12

"""Implementation validation script for Penfold database schema.

This script validates the database implementation without requiring
a full database setup. It tests imports, model definitions, and
basic functionality.
"""

import sys
import os
import traceback
from typing import List, Dict, Any
import asyncio


class ValidationResult:
    """Stores validation results."""

    def __init__(self):
        self.passed = 0
        self.failed = 0
        self.errors = []

    def add_pass(self, test_name: str):
        """Record a passing test."""
        self.passed += 1
        print(f"✅ {test_name}")

    def add_fail(self, test_name: str, error: str):
        """Record a failing test."""
        self.failed += 1
        self.errors.append((test_name, error))
        print(f"❌ {test_name}: {error}")

    def print_summary(self):
        """Print validation summary."""
        total = self.passed + self.failed
        print(f"\n{'='*50}")
        print(f"VALIDATION SUMMARY")
        print(f"{'='*50}")
        print(f"Total Tests: {total}")
        print(f"Passed: {self.passed}")
        print(f"Failed: {self.failed}")

        if self.failed > 0:
            print(f"\n❌ FAILED TESTS:")
            for test_name, error in self.errors:
                print(f"  • {test_name}: {error}")

        success_rate = (self.passed / total * 100) if total > 0 else 0
        print(f"\nSuccess Rate: {success_rate:.1f}%")

        if self.failed == 0:
            print("\n🎉 ALL VALIDATIONS PASSED!")
        else:
            print(f"\n⚠️  {self.failed} validations failed")


def validate_imports(result: ValidationResult):
    """Validate that all modules can be imported."""
    print("\n🔍 Testing Module Imports...")

    imports_to_test = [
        # Core models
        ("penf_lib.storage.models", "Base, Source, Assertion, Person, Project, Team"),
        ("penf_lib.storage.models", "Embedding, ProcessingEvent, ProcessingJob, ProcessingResult, Subscription"),

        # Storage modules
        ("penf_lib.storage.config", "config"),
        ("penf_lib.storage.connections", "get_session, get_redis_client"),

        # New implementations
        ("penf_lib.storage.vector", "VectorOperations, VectorSearchService"),
        ("penf_lib.storage.events", "EventPublisher, EventSubscriber, SubscriptionManager"),
        ("penf_lib.storage.jobs", "JobManager, JobStatus"),

        # Repositories
        ("penf_lib.storage.repositories.embedding", "EmbeddingRepository"),
        ("penf_lib.storage.repositories.source", "SourceRepository"),
        ("penf_lib.storage.repositories.assertion", "AssertionRepository"),
    ]

    for module_name, items in imports_to_test:
        try:
            exec(f"from {module_name} import {items}")
            result.add_pass(f"Import {module_name}")
        except Exception as e:
            result.add_fail(f"Import {module_name}", str(e))


def validate_models(result: ValidationResult):
    """Validate model definitions and structure."""
    print("\n🏗️  Testing Model Definitions...")

    try:
        from penf_lib.storage.models import (
            Base, Source, Assertion, Person, Project, Team,
            Embedding, ProcessingEvent, ProcessingJob, ProcessingResult, Subscription
        )

        # Test that models inherit from Base
        models = [Source, Assertion, Person, Project, Team,
                 Embedding, ProcessingEvent, ProcessingJob, ProcessingResult, Subscription]

        for model in models:
            if issubclass(model, Base):
                result.add_pass(f"Model {model.__name__} inherits from Base")
            else:
                result.add_fail(f"Model {model.__name__}", "does not inherit from Base")

        # Test that models have required attributes
        required_attrs = ["__tablename__", "__table_args__"]

        for model in models:
            for attr in required_attrs:
                if hasattr(model, attr):
                    result.add_pass(f"Model {model.__name__} has {attr}")
                else:
                    result.add_fail(f"Model {model.__name__}", f"missing {attr}")

    except Exception as e:
        result.add_fail("Model validation", str(e))


def validate_vector_operations(result: ValidationResult):
    """Validate vector operations functionality."""
    print("\n🔢 Testing Vector Operations...")

    try:
        from penf_lib.storage.vector import VectorOperations, VectorDimensionError
        from unittest.mock import AsyncMock

        # Test initialization
        mock_session = AsyncMock()
        vector_ops = VectorOperations(mock_session, expected_dimension=768)

        if vector_ops.expected_dimension == 768:
            result.add_pass("VectorOperations initialization")
        else:
            result.add_fail("VectorOperations initialization", "dimension mismatch")

        # Test cache functionality
        vector_ops._search_cache["test"] = "value"
        vector_ops.clear_cache()

        if len(vector_ops._search_cache) == 0:
            result.add_pass("VectorOperations cache clearing")
        else:
            result.add_fail("VectorOperations cache clearing", "cache not cleared")

        # Test distance metrics
        valid_metrics = ["l2", "cosine", "inner_product"]
        for metric in valid_metrics:
            try:
                VectorOperations(mock_session, distance_metric=metric)
                result.add_pass(f"VectorOperations {metric} metric")
            except Exception as e:
                result.add_fail(f"VectorOperations {metric} metric", str(e))

    except Exception as e:
        result.add_fail("Vector operations validation", str(e))


def validate_event_processing(result: ValidationResult):
    """Validate event processing functionality."""
    print("\n📡 Testing Event Processing...")

    try:
        from penf_lib.storage.events import EventPublisher, EventSubscriber
        from penf_lib.storage.jobs import JobManager, JobStatus
        from unittest.mock import AsyncMock

        # Test JobStatus enum
        expected_statuses = ["pending", "running", "completed", "failed", "cancelled", "retrying"]
        for status in expected_statuses:
            if hasattr(JobStatus, status.upper()) and getattr(JobStatus, status.upper()) == status:
                result.add_pass(f"JobStatus.{status.upper()}")
            else:
                result.add_fail(f"JobStatus.{status.upper()}", "not defined correctly")

        # Test EventPublisher initialization
        mock_session = AsyncMock()
        mock_redis = AsyncMock()

        publisher = EventPublisher(mock_session, mock_redis)
        if publisher.session == mock_session and publisher.redis_client == mock_redis:
            result.add_pass("EventPublisher initialization")
        else:
            result.add_fail("EventPublisher initialization", "incorrect setup")

        # Test EventSubscriber initialization
        subscriber = EventSubscriber(mock_redis)
        if subscriber.redis_client == mock_redis and isinstance(subscriber._subscriptions, set):
            result.add_pass("EventSubscriber initialization")
        else:
            result.add_fail("EventSubscriber initialization", "incorrect setup")

        # Test JobManager initialization
        job_manager = JobManager(mock_session)
        if job_manager.session == mock_session:
            result.add_pass("JobManager initialization")
        else:
            result.add_fail("JobManager initialization", "incorrect setup")

    except Exception as e:
        result.add_fail("Event processing validation", str(e))


def validate_configuration(result: ValidationResult):
    """Validate configuration setup."""
    print("\n⚙️  Testing Configuration...")

    try:
        from penf_lib.storage.config import config

        # Test that config object exists and has required sections
        required_sections = ["database", "redis", "storage"]

        for section in required_sections:
            if hasattr(config, section):
                result.add_pass(f"Config section: {section}")
            else:
                result.add_fail(f"Config section: {section}", "not found")

        # Test database configuration
        if hasattr(config.database, 'host') and hasattr(config.database, 'port'):
            result.add_pass("Database configuration structure")
        else:
            result.add_fail("Database configuration structure", "missing required fields")

        # Test Redis configuration
        if hasattr(config.redis, 'host') and hasattr(config.redis, 'port'):
            result.add_pass("Redis configuration structure")
        else:
            result.add_fail("Redis configuration structure", "missing required fields")

    except Exception as e:
        result.add_fail("Configuration validation", str(e))


def validate_repository_structure(result: ValidationResult):
    """Validate repository structure and inheritance."""
    print("\n🏛️  Testing Repository Structure...")

    try:
        from penf_lib.storage.repositories.base import BaseRepository
        from penf_lib.storage.repositories.embedding import EmbeddingRepository
        from penf_lib.storage.repositories.source import SourceRepository

        # Test that repositories inherit from BaseRepository
        repos = [EmbeddingRepository, SourceRepository]

        for repo_class in repos:
            # We can't directly test inheritance without instantiation,
            # but we can check method existence
            expected_methods = ['create', 'get_by_id', 'update', 'delete']

            for method in expected_methods:
                if hasattr(repo_class, method):
                    result.add_pass(f"{repo_class.__name__} has {method} method")
                else:
                    result.add_fail(f"{repo_class.__name__}", f"missing {method} method")

    except Exception as e:
        result.add_fail("Repository structure validation", str(e))


def validate_performance_targets(result: ValidationResult):
    """Validate that performance-critical code is properly structured."""
    print("\n⚡ Testing Performance Structure...")

    try:
        from penf_lib.storage.vector import VectorOperations
        from penf_lib.storage.events import EventPublisher
        from penf_lib.storage.jobs import JobManager

        # Check that classes have async methods for performance
        perf_classes = [
            (VectorOperations, ['store_embedding', 'similarity_search', 'store_embeddings_batch']),
            (EventPublisher, ['publish']),
            (JobManager, ['create_job', 'get_job', 'start_job', 'complete_job']),
        ]

        for class_obj, methods in perf_classes:
            for method_name in methods:
                if hasattr(class_obj, method_name):
                    method = getattr(class_obj, method_name)
                    if asyncio.iscoroutinefunction(method):
                        result.add_pass(f"{class_obj.__name__}.{method_name} is async")
                    else:
                        result.add_fail(f"{class_obj.__name__}.{method_name}", "not async")
                else:
                    result.add_fail(f"{class_obj.__name__}", f"missing {method_name} method")

    except Exception as e:
        result.add_fail("Performance structure validation", str(e))


def main():
    """Run all validations."""
    print("🔍 Penfold Database Schema Implementation Validation")
    print("=" * 55)

    # Add project root to Python path
    project_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    sys.path.insert(0, project_root)

    result = ValidationResult()

    try:
        validate_imports(result)
        validate_models(result)
        validate_vector_operations(result)
        validate_event_processing(result)
        validate_configuration(result)
        validate_repository_structure(result)
        validate_performance_targets(result)

    except Exception as e:
        result.add_fail("Validation framework", f"Unexpected error: {e}")
        traceback.print_exc()

    result.print_summary()

    # Exit with appropriate code
    sys.exit(0 if result.failed == 0 else 1)


if __name__ == "__main__":
    main()