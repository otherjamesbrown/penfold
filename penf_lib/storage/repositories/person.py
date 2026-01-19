"""Person repository with specialized operations."""

import uuid
from typing import List, Optional, Dict, Any
from datetime import datetime

from sqlalchemy import select, and_, or_, func, desc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from penf_lib.storage.models import Person
from .base import BaseRepository


class PersonRepository(BaseRepository[Person]):
    """Repository for Person entities."""

    def __init__(self, session: AsyncSession):
        """Initialize person repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, Person)

    async def create_person(
        self,
        tenant_id: uuid.UUID,
        canonical_name: str,
        email_addresses: Optional[List[str]] = None,
        phone_numbers: Optional[List[str]] = None,
        aliases: Optional[List[str]] = None,
        organizational_context: Optional[Dict[str, Any]] = None,
    ) -> Person:
        """Create a new person with validation.

        Args:
            tenant_id: Tenant ID
            canonical_name: Primary canonical name
            email_addresses: List of email addresses
            phone_numbers: List of phone numbers
            aliases: List of name aliases
            organizational_context: Organizational metadata

        Returns:
            Created person instance
        """
        return await self.create(
            tenant_id=tenant_id,
            canonical_name=canonical_name,
            email_addresses=email_addresses or [],
            phone_numbers=phone_numbers or [],
            aliases=aliases or [],
            organizational_context=organizational_context,
        )

    async def find_by_email(
        self, tenant_id: uuid.UUID, email: str
    ) -> Optional[Person]:
        """Find person by email address.

        Args:
            tenant_id: Tenant ID
            email: Email address

        Returns:
            Person instance or None if not found
        """
        email_lower = email.lower().strip()
        stmt = (
            self._build_query()
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    Person.email_addresses.any(email_lower)
                )
            )
        )

        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def find_by_name(
        self, tenant_id: uuid.UUID, name: str
    ) -> Optional[Person]:
        """Find person by canonical name or alias.

        Args:
            tenant_id: Tenant ID
            name: Name to search for

        Returns:
            Person instance or None if not found
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    or_(
                        Person.canonical_name.ilike(f'%{name}%'),
                        Person.aliases.any(name)
                    )
                )
            )
        )

        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def find_by_phone(
        self, tenant_id: uuid.UUID, phone: str
    ) -> Optional[Person]:
        """Find person by phone number.

        Args:
            tenant_id: Tenant ID
            phone: Phone number

        Returns:
            Person instance or None if not found
        """
        # Clean phone number for search
        import re
        cleaned_phone = re.sub(r'[-()\\s]', '', phone.strip())

        stmt = (
            self._build_query()
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    Person.phone_numbers.any(cleaned_phone)
                )
            )
        )

        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def search_by_name(
        self, tenant_id: uuid.UUID, name_query: str, limit: int = 50
    ) -> List[Person]:
        """Search people by name (canonical name and aliases).

        Args:
            tenant_id: Tenant ID
            name_query: Name search query
            limit: Maximum results to return

        Returns:
            List of matching person instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    or_(
                        Person.canonical_name.ilike(f'%{name_query}%'),
                        Person.aliases.any(func.lower(func.any(Person.aliases)).like(f'%{name_query.lower()}%'))
                    )
                )
            )
            .order_by(Person.canonical_name)
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def add_email(self, person_id: int, email: str) -> Optional[Person]:
        """Add email address to person.

        Args:
            person_id: Person ID
            email: Email address to add

        Returns:
            Updated person instance or None if not found
        """
        person = await self.get_by_id(person_id)
        if not person:
            return None

        email_lower = email.lower().strip()
        if email_lower not in person.email_addresses:
            person.email_addresses = person.email_addresses + [email_lower]
            return await self.update_entity(person)

        return person

    async def add_phone(self, person_id: int, phone: str) -> Optional[Person]:
        """Add phone number to person.

        Args:
            person_id: Person ID
            phone: Phone number to add

        Returns:
            Updated person instance or None if not found
        """
        person = await self.get_by_id(person_id)
        if not person:
            return None

        # Clean phone number
        import re
        cleaned_phone = re.sub(r'[-()\\s]', '', phone.strip())

        if cleaned_phone not in person.phone_numbers:
            person.phone_numbers = person.phone_numbers + [cleaned_phone]
            return await self.update_entity(person)

        return person

    async def add_alias(self, person_id: int, alias: str) -> Optional[Person]:
        """Add name alias to person.

        Args:
            person_id: Person ID
            alias: Name alias to add

        Returns:
            Updated person instance or None if not found
        """
        person = await self.get_by_id(person_id)
        if not person:
            return None

        if alias not in person.aliases:
            person.aliases = person.aliases + [alias]
            return await self.update_entity(person)

        return person

    async def merge_persons(
        self, primary_person_id: int, secondary_person_id: int
    ) -> Optional[Person]:
        """Merge two person records.

        Args:
            primary_person_id: Primary person to keep
            secondary_person_id: Secondary person to merge into primary

        Returns:
            Updated primary person instance or None if either not found
        """
        primary = await self.get_by_id(primary_person_id)
        secondary = await self.get_by_id(secondary_person_id)

        if not primary or not secondary:
            return None

        # Merge email addresses
        merged_emails = list(set(primary.email_addresses + secondary.email_addresses))

        # Merge phone numbers
        merged_phones = list(set(primary.phone_numbers + secondary.phone_numbers))

        # Merge aliases (include secondary's canonical name as alias)
        merged_aliases = list(set(
            primary.aliases + secondary.aliases + [secondary.canonical_name]
        ))

        # Merge organizational context
        merged_context = primary.organizational_context or {}
        if secondary.organizational_context:
            merged_context.update(secondary.organizational_context)

        # Update primary person
        primary = await self.update_entity(
            primary,
            email_addresses=merged_emails,
            phone_numbers=merged_phones,
            aliases=merged_aliases,
            organizational_context=merged_context,
        )

        # Soft delete secondary person
        await self.delete_entity(secondary, hard_delete=False)

        return primary

    async def find_similar(
        self, tenant_id: uuid.UUID, canonical_name: str, email: Optional[str] = None
    ) -> List[Person]:
        """Find persons with similar names or matching email.

        Args:
            tenant_id: Tenant ID
            canonical_name: Name to compare
            email: Optional email to match

        Returns:
            List of similar person instances
        """
        conditions = [Person.tenant_id == tenant_id]

        # Name similarity (basic string matching)
        name_parts = canonical_name.lower().split()
        if name_parts:
            name_conditions = []
            for part in name_parts:
                name_conditions.extend([
                    Person.canonical_name.ilike(f'%{part}%'),
                    Person.aliases.any(func.lower(func.any(Person.aliases)).like(f'%{part}%'))
                ])
            conditions.append(or_(*name_conditions))

        # Email match
        if email:
            conditions.append(Person.email_addresses.any(email.lower()))

        stmt = (
            self._build_query()
            .where(and_(*conditions))
            .order_by(Person.canonical_name)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def find_by_organization(
        self, tenant_id: uuid.UUID, organization_key: str, organization_value: str
    ) -> List[Person]:
        """Find persons by organizational context.

        Args:
            tenant_id: Tenant ID
            organization_key: Key in organizational_context
            organization_value: Value to match

        Returns:
            List of person instances
        """
        stmt = (
            self._build_query()
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    Person.organizational_context[organization_key].astext == organization_value
                )
            )
            .order_by(Person.canonical_name)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_contact_stats(self, tenant_id: uuid.UUID) -> Dict[str, int]:
        """Get contact statistics.

        Args:
            tenant_id: Tenant ID

        Returns:
            Dictionary with contact statistics
        """
        total_stmt = (
            select(func.count(Person.id))
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    Person.is_deleted.is_(False)
                )
            )
        )

        with_email_stmt = (
            select(func.count(Person.id))
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    Person.is_deleted.is_(False),
                    func.array_length(Person.email_addresses, 1) > 0
                )
            )
        )

        with_phone_stmt = (
            select(func.count(Person.id))
            .where(
                and_(
                    Person.tenant_id == tenant_id,
                    Person.is_deleted.is_(False),
                    func.array_length(Person.phone_numbers, 1) > 0
                )
            )
        )

        total_result = await self.session.execute(total_stmt)
        email_result = await self.session.execute(with_email_stmt)
        phone_result = await self.session.execute(with_phone_stmt)

        return {
            'total_persons': total_result.scalar(),
            'with_email': email_result.scalar(),
            'with_phone': phone_result.scalar(),
        }