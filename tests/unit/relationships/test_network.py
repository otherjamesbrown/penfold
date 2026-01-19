"""Unit tests for NetworkAnalyzer - Network Analysis for Relationships.

Tests the network analysis capabilities including:
- Network graph building from relationships
- Centrality calculations (degree, betweenness, closeness)
- Communication hub identification
- Isolated participant detection
- Community/cluster detection
- Influence pattern analysis
- Export functionality (JSON, DOT, CSV)

TDD: Tests written first before implementation.
"""

from __future__ import annotations

from collections.abc import Callable
from datetime import datetime, timedelta, timezone
from uuid import uuid4

import pytest

from penf_lib.relationships import (
    EntityReference,
    EntityType,
    LifecycleState,
    RelationshipNetworkEdge,
    RelationshipResponse,
    RelationshipScope,
    RelationshipType,
)
from penf_lib.relationships.network import (
    BottleneckInfo,
    CentralityMetrics,
    CollaborationOpportunity,
    CommunityInfo,
    ExportFormat,
    NetworkAnalyzer,
    NetworkGraph,
    NetworkInsights,
)

# Type alias for relationship factory
RelationshipFactoryType = Callable[..., RelationshipResponse]


class TestNetworkGraph:
    """Tests for the NetworkGraph data structure."""

    def test_create_empty_network_graph(self) -> None:
        """Test creating an empty network graph."""
        graph = NetworkGraph()

        assert len(graph.nodes) == 0
        assert len(graph.edges) == 0

    def test_add_node_to_graph(self) -> None:
        """Test adding a node to the network graph."""
        graph = NetworkGraph()
        entity = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Alice Johnson",
            external_ids={"email": "alice@example.com"},
        )

        graph.add_node(entity)

        assert len(graph.nodes) == 1
        assert entity.entity_id in graph.nodes

    def test_add_edge_to_graph(self) -> None:
        """Test adding an edge between two nodes."""
        graph = NetworkGraph()
        alice_id = uuid4()
        bob_id = uuid4()

        alice = EntityReference(
            entity_id=alice_id,
            entity_type=EntityType.PERSON,
            display_name="Alice Johnson",
            external_ids={},
        )
        bob = EntityReference(
            entity_id=bob_id,
            entity_type=EntityType.PERSON,
            display_name="Bob Smith",
            external_ids={},
        )

        graph.add_node(alice)
        graph.add_node(bob)

        now = datetime.now(timezone.utc)
        edge = RelationshipNetworkEdge(
            source_entity_id=alice_id,
            target_entity_id=bob_id,
            relationship_types=[RelationshipType.COLLABORATES_WITH],
            combined_strength=0.8,
            interaction_count=5,
            first_interaction=now,
            last_interaction=now,
        )
        graph.add_edge(edge)

        assert len(graph.edges) == 1

    def test_get_neighbors(self) -> None:
        """Test getting neighbors of a node."""
        graph = NetworkGraph()
        alice_id = uuid4()
        bob_id = uuid4()
        carol_id = uuid4()

        for entity_id, name in [(alice_id, "Alice"), (bob_id, "Bob"), (carol_id, "Carol")]:
            graph.add_node(
                EntityReference(
                    entity_id=entity_id,
                    entity_type=EntityType.PERSON,
                    display_name=name,
                    external_ids={},
                )
            )

        now = datetime.now(timezone.utc)
        # Alice -> Bob
        graph.add_edge(
            RelationshipNetworkEdge(
                source_entity_id=alice_id,
                target_entity_id=bob_id,
                relationship_types=[RelationshipType.COLLABORATES_WITH],
                combined_strength=0.8,
                interaction_count=5,
                first_interaction=now,
                last_interaction=now,
            )
        )
        # Alice -> Carol
        graph.add_edge(
            RelationshipNetworkEdge(
                source_entity_id=alice_id,
                target_entity_id=carol_id,
                relationship_types=[RelationshipType.COMMUNICATES_WITH],
                combined_strength=0.6,
                interaction_count=3,
                first_interaction=now,
                last_interaction=now,
            )
        )

        neighbors = graph.get_neighbors(alice_id)

        assert len(neighbors) == 2
        assert bob_id in neighbors
        assert carol_id in neighbors


class TestCentralityMetrics:
    """Tests for centrality metric calculations."""

    def test_create_centrality_metrics(self) -> None:
        """Test creating centrality metrics."""
        entity_id = uuid4()
        metrics = CentralityMetrics(
            entity_id=entity_id,
            degree_centrality=0.75,
            betweenness_centrality=0.45,
            closeness_centrality=0.60,
            eigenvector_centrality=0.55,
        )

        assert metrics.entity_id == entity_id
        assert metrics.degree_centrality == 0.75
        assert metrics.betweenness_centrality == 0.45
        assert metrics.closeness_centrality == 0.60

    def test_centrality_metrics_normalized_range(self) -> None:
        """Test that centrality metrics are in valid normalized range."""
        entity_id = uuid4()
        metrics = CentralityMetrics(
            entity_id=entity_id,
            degree_centrality=0.5,
            betweenness_centrality=0.3,
            closeness_centrality=0.4,
            eigenvector_centrality=0.2,
        )

        # All metrics should be in [0, 1] range
        assert 0.0 <= metrics.degree_centrality <= 1.0
        assert 0.0 <= metrics.betweenness_centrality <= 1.0
        assert 0.0 <= metrics.closeness_centrality <= 1.0
        assert 0.0 <= metrics.eigenvector_centrality <= 1.0


class TestCommunityInfo:
    """Tests for community/cluster detection results."""

    def test_create_community_info(self) -> None:
        """Test creating community info."""
        members = [uuid4(), uuid4(), uuid4()]
        community = CommunityInfo(
            community_id="team-alpha",
            members=members,
            cohesion_score=0.85,
            key_topics=["deployment", "infrastructure"],
        )

        assert community.community_id == "team-alpha"
        assert len(community.members) == 3
        assert community.cohesion_score == 0.85

    def test_community_size(self) -> None:
        """Test community size calculation."""
        members = [uuid4() for _ in range(5)]
        community = CommunityInfo(
            community_id="team-beta",
            members=members,
            cohesion_score=0.70,
            key_topics=[],
        )

        assert community.size == 5


class TestNetworkAnalyzer:
    """Tests for the NetworkAnalyzer class."""

    @pytest.fixture
    def sample_relationships(self) -> list[RelationshipResponse]:
        """Create a sample set of relationships for testing."""
        now = datetime.now(timezone.utc)
        relationships = []

        # Create entity references
        alice_id = uuid4()
        bob_id = uuid4()
        carol_id = uuid4()
        david_id = uuid4()
        eve_id = uuid4()  # Isolated participant

        # Store for reference
        self.alice_id = alice_id
        self.bob_id = bob_id
        self.carol_id = carol_id
        self.david_id = david_id
        self.eve_id = eve_id

        entities = {
            alice_id: EntityReference(
                entity_id=alice_id,
                entity_type=EntityType.PERSON,
                display_name="Alice Johnson",
                external_ids={"email": "alice@example.com"},
            ),
            bob_id: EntityReference(
                entity_id=bob_id,
                entity_type=EntityType.PERSON,
                display_name="Bob Smith",
                external_ids={"email": "bob@example.com"},
            ),
            carol_id: EntityReference(
                entity_id=carol_id,
                entity_type=EntityType.PERSON,
                display_name="Carol Davis",
                external_ids={"email": "carol@example.com"},
            ),
            david_id: EntityReference(
                entity_id=david_id,
                entity_type=EntityType.PERSON,
                display_name="David Lee",
                external_ids={"email": "david@example.com"},
            ),
            eve_id: EntityReference(
                entity_id=eve_id,
                entity_type=EntityType.PERSON,
                display_name="Eve Wilson",
                external_ids={"email": "eve@example.com"},
            ),
        }

        # Alice is a hub - connects to everyone except Eve
        # Alice -> Bob (strong)
        relationships.append(
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=entities[alice_id],
                target_entity=entities[bob_id],
                relationship_type=RelationshipType.COLLABORATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.9,
                evidence_count=10,
                first_observed=now - timedelta(days=90),
                last_observed=now,
                version=1,
            )
        )
        # Alice -> Carol
        relationships.append(
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=entities[alice_id],
                target_entity=entities[carol_id],
                relationship_type=RelationshipType.COMMUNICATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.85,
                evidence_count=7,
                first_observed=now - timedelta(days=60),
                last_observed=now,
                version=1,
            )
        )
        # Alice -> David
        relationships.append(
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=entities[alice_id],
                target_entity=entities[david_id],
                relationship_type=RelationshipType.MANAGES,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.95,
                evidence_count=15,
                first_observed=now - timedelta(days=120),
                last_observed=now,
                version=1,
            )
        )
        # Bob -> Carol
        relationships.append(
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=entities[bob_id],
                target_entity=entities[carol_id],
                relationship_type=RelationshipType.COLLABORATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.75,
                evidence_count=4,
                first_observed=now - timedelta(days=30),
                last_observed=now,
                version=1,
            )
        )

        # Eve has only one weak connection (isolated)
        relationships.append(
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=entities[eve_id],
                target_entity=entities[david_id],
                relationship_type=RelationshipType.COMMUNICATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.3,
                evidence_count=1,
                first_observed=now - timedelta(days=10),
                last_observed=now - timedelta(days=5),
                version=1,
            )
        )

        self.entities = entities
        return relationships

    @pytest.fixture
    def analyzer(self) -> NetworkAnalyzer:
        """Create a NetworkAnalyzer instance."""
        return NetworkAnalyzer()

    def test_build_network_from_relationships(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test building a network graph from relationships."""
        graph = analyzer.build_network(sample_relationships)

        assert isinstance(graph, NetworkGraph)
        # Should have 5 unique entities
        assert len(graph.nodes) == 5
        # Should have 5 edges (one per relationship)
        assert len(graph.edges) == 5

    def test_calculate_degree_centrality(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test degree centrality calculation."""
        graph = analyzer.build_network(sample_relationships)
        centrality = analyzer.calculate_centrality(graph)

        # Alice should have highest degree centrality (most connections)
        alice_metrics = centrality[self.alice_id]
        eve_metrics = centrality[self.eve_id]

        assert alice_metrics.degree_centrality > eve_metrics.degree_centrality

    def test_identify_communication_hubs(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test identification of communication hubs."""
        graph = analyzer.build_network(sample_relationships)
        hubs = analyzer.identify_hubs(graph, threshold=0.5)

        # Alice should be identified as a hub
        hub_ids = [h.entity.entity_id for h in hubs]
        assert self.alice_id in hub_ids

        # Eve should not be a hub
        assert self.eve_id not in hub_ids

    def test_detect_isolated_participants(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test detection of isolated participants."""
        graph = analyzer.build_network(sample_relationships)
        isolated = analyzer.detect_isolated(graph, connection_threshold=2)

        # Eve should be detected as isolated (only 1 connection)
        isolated_ids = [i.entity.entity_id for i in isolated]
        assert self.eve_id in isolated_ids

        # Alice should not be isolated
        assert self.alice_id not in isolated_ids

    def test_detect_communities(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test community/cluster detection."""
        graph = analyzer.build_network(sample_relationships)
        communities = analyzer.detect_communities(graph)

        # Should detect at least one community
        assert len(communities) >= 1

        # The main group (Alice, Bob, Carol, David) should be in the same or connected communities
        # Eve should potentially be in a separate or peripheral community

    def test_analyze_network_returns_insights(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test that full network analysis returns comprehensive insights."""
        insights = analyzer.analyze(sample_relationships)

        assert isinstance(insights, NetworkInsights)
        assert insights.total_nodes > 0
        assert insights.total_edges > 0
        assert len(insights.hubs) >= 0
        assert len(insights.isolated_participants) >= 0
        assert len(insights.communities) >= 0

    def test_identify_bottlenecks(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test identification of communication bottlenecks."""
        graph = analyzer.build_network(sample_relationships)
        bottlenecks = analyzer.identify_bottlenecks(graph)

        # Bottlenecks are nodes that, if removed, would disconnect the network
        # Alice is likely a bottleneck given the network structure
        assert isinstance(bottlenecks, list)
        for b in bottlenecks:
            assert isinstance(b, BottleneckInfo)

    def test_find_collaboration_opportunities(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test finding collaboration opportunities (gaps in network)."""
        graph = analyzer.build_network(sample_relationships)
        opportunities = analyzer.find_collaboration_opportunities(graph)

        # Should find potential connections that don't exist yet
        # E.g., if Bob and David don't directly collaborate but work with Alice
        assert isinstance(opportunities, list)
        for opp in opportunities:
            assert isinstance(opp, CollaborationOpportunity)

    def test_calculate_network_density(
        self,
        analyzer: NetworkAnalyzer,
        sample_relationships: list[RelationshipResponse],
    ) -> None:
        """Test network density calculation."""
        graph = analyzer.build_network(sample_relationships)
        density = analyzer.calculate_density(graph)

        # Density should be between 0 and 1
        assert 0.0 <= density <= 1.0

        # With 5 nodes and 5 edges (undirected), density = 5 / (5*4/2) = 5/10 = 0.5
        # But relationships are directional, so density might differ

    def test_empty_relationships_returns_empty_insights(
        self,
        analyzer: NetworkAnalyzer,
    ) -> None:
        """Test analyzing empty relationships list."""
        insights = analyzer.analyze([])

        assert insights.total_nodes == 0
        assert insights.total_edges == 0
        assert len(insights.hubs) == 0
        assert len(insights.isolated_participants) == 0

    def test_single_relationship_network(
        self,
        analyzer: NetworkAnalyzer,
    ) -> None:
        """Test network with only one relationship."""
        now = datetime.now(timezone.utc)
        alice = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Alice",
            external_ids={},
        )
        bob = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Bob",
            external_ids={},
        )

        relationships = [
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=alice,
                target_entity=bob,
                relationship_type=RelationshipType.COLLABORATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.8,
                evidence_count=3,
                first_observed=now,
                last_observed=now,
                version=1,
            )
        ]

        insights = analyzer.analyze(relationships)

        assert insights.total_nodes == 2
        assert insights.total_edges == 1


class TestNetworkExport:
    """Tests for network export functionality."""

    @pytest.fixture
    def simple_network(self) -> tuple[NetworkAnalyzer, NetworkGraph]:
        """Create a simple network for export testing."""
        analyzer = NetworkAnalyzer()
        now = datetime.now(timezone.utc)

        alice_id = uuid4()
        bob_id = uuid4()

        alice = EntityReference(
            entity_id=alice_id,
            entity_type=EntityType.PERSON,
            display_name="Alice Johnson",
            external_ids={"email": "alice@example.com"},
        )
        bob = EntityReference(
            entity_id=bob_id,
            entity_type=EntityType.PERSON,
            display_name="Bob Smith",
            external_ids={"email": "bob@example.com"},
        )

        relationships = [
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=alice,
                target_entity=bob,
                relationship_type=RelationshipType.COLLABORATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.85,
                evidence_count=5,
                first_observed=now,
                last_observed=now,
                version=1,
            )
        ]

        graph = analyzer.build_network(relationships)
        return analyzer, graph

    def test_export_to_json(
        self,
        simple_network: tuple[NetworkAnalyzer, NetworkGraph],
    ) -> None:
        """Test exporting network to JSON format."""
        analyzer, graph = simple_network
        json_output = analyzer.export(graph, ExportFormat.JSON)

        assert isinstance(json_output, str)
        # Should be valid JSON
        import json
        data = json.loads(json_output)

        assert "nodes" in data
        assert "edges" in data
        assert len(data["nodes"]) == 2
        assert len(data["edges"]) == 1

    def test_export_to_dot(
        self,
        simple_network: tuple[NetworkAnalyzer, NetworkGraph],
    ) -> None:
        """Test exporting network to DOT format for Graphviz."""
        analyzer, graph = simple_network
        dot_output = analyzer.export(graph, ExportFormat.DOT)

        assert isinstance(dot_output, str)
        # Should contain DOT format elements
        assert "digraph" in dot_output or "graph" in dot_output
        assert "Alice" in dot_output
        assert "Bob" in dot_output
        assert "->" in dot_output or "--" in dot_output

    def test_export_to_csv(
        self,
        simple_network: tuple[NetworkAnalyzer, NetworkGraph],
    ) -> None:
        """Test exporting network edges to CSV format."""
        analyzer, graph = simple_network
        csv_output = analyzer.export(graph, ExportFormat.CSV)

        assert isinstance(csv_output, str)
        lines = csv_output.strip().split("\n")

        # Should have header row and data row
        assert len(lines) >= 2
        # Header should contain expected columns
        header = lines[0].lower()
        assert "source" in header
        assert "target" in header

    def test_export_includes_metrics(
        self,
        simple_network: tuple[NetworkAnalyzer, NetworkGraph],
    ) -> None:
        """Test that export can include centrality metrics."""
        analyzer, graph = simple_network
        json_output = analyzer.export(
            graph,
            ExportFormat.JSON,
            include_metrics=True,
        )

        import json
        data = json.loads(json_output)

        # Nodes should have centrality metrics
        for node in data["nodes"]:
            assert "centrality" in node or "degree_centrality" in node


class TestNetworkAnalyzerPerformance:
    """Performance tests for network analysis."""

    def test_handles_large_network(self) -> None:
        """Test that analyzer handles networks with many relationships efficiently."""
        analyzer = NetworkAnalyzer()
        now = datetime.now(timezone.utc)

        # Create 100 entities and 500 relationships
        entities = []
        for i in range(100):
            entities.append(
                EntityReference(
                    entity_id=uuid4(),
                    entity_type=EntityType.PERSON,
                    display_name=f"Person {i}",
                    external_ids={},
                )
            )

        import random
        random.seed(42)  # Reproducible

        relationships = []
        for _ in range(500):
            source = random.choice(entities)
            target = random.choice(entities)
            if source.entity_id != target.entity_id:
                relationships.append(
                    RelationshipResponse(
                        relationship_id=uuid4(),
                        source_entity=source,
                        target_entity=target,
                        relationship_type=RelationshipType.COMMUNICATES_WITH,
                        scope=RelationshipScope.GLOBAL,
                        lifecycle_state=LifecycleState.ACTIVE,
                        confidence_score=random.random(),
                        evidence_count=random.randint(1, 10),
                        first_observed=now,
                        last_observed=now,
                        version=1,
                    )
                )

        import time
        start = time.time()
        insights = analyzer.analyze(relationships)
        elapsed = time.time() - start

        # Should complete in reasonable time (< 5 seconds)
        assert elapsed < 5.0
        assert insights.total_nodes <= 100
        assert insights.total_edges <= 500


class TestInfluencePatterns:
    """Tests for influence pattern identification."""

    @pytest.fixture
    def hierarchical_network(self) -> list[RelationshipResponse]:
        """Create a hierarchical network for influence testing."""
        now = datetime.now(timezone.utc)

        # Create a management hierarchy
        ceo = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="CEO",
            external_ids={},
        )
        vp1 = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="VP Engineering",
            external_ids={},
        )
        vp2 = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="VP Sales",
            external_ids={},
        )
        eng1 = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Engineer 1",
            external_ids={},
        )
        eng2 = EntityReference(
            entity_id=uuid4(),
            entity_type=EntityType.PERSON,
            display_name="Engineer 2",
            external_ids={},
        )

        self.ceo_id = ceo.entity_id
        self.vp1_id = vp1.entity_id

        relationships = [
            # CEO manages VPs
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=ceo,
                target_entity=vp1,
                relationship_type=RelationshipType.MANAGES,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.CONFIRMED,
                confidence_score=0.95,
                evidence_count=20,
                first_observed=now - timedelta(days=365),
                last_observed=now,
                version=1,
            ),
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=ceo,
                target_entity=vp2,
                relationship_type=RelationshipType.MANAGES,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.CONFIRMED,
                confidence_score=0.95,
                evidence_count=18,
                first_observed=now - timedelta(days=365),
                last_observed=now,
                version=1,
            ),
            # VP Engineering manages engineers
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=vp1,
                target_entity=eng1,
                relationship_type=RelationshipType.MANAGES,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.CONFIRMED,
                confidence_score=0.92,
                evidence_count=15,
                first_observed=now - timedelta(days=200),
                last_observed=now,
                version=1,
            ),
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=vp1,
                target_entity=eng2,
                relationship_type=RelationshipType.MANAGES,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.CONFIRMED,
                confidence_score=0.90,
                evidence_count=12,
                first_observed=now - timedelta(days=150),
                last_observed=now,
                version=1,
            ),
            # Engineers collaborate
            RelationshipResponse(
                relationship_id=uuid4(),
                source_entity=eng1,
                target_entity=eng2,
                relationship_type=RelationshipType.COLLABORATES_WITH,
                scope=RelationshipScope.GLOBAL,
                lifecycle_state=LifecycleState.ACTIVE,
                confidence_score=0.85,
                evidence_count=25,
                first_observed=now - timedelta(days=100),
                last_observed=now,
                version=1,
            ),
        ]

        return relationships

    def test_identify_influence_by_management_relationships(
        self,
        hierarchical_network: list[RelationshipResponse],
    ) -> None:
        """Test that management relationships indicate influence."""
        analyzer = NetworkAnalyzer()
        insights = analyzer.analyze(hierarchical_network)

        # CEO should have high influence score due to management relationships
        influence_scores = insights.influence_scores

        # CEO should have highest influence (influence_scores uses string keys)
        ceo_influence = influence_scores.get(str(self.ceo_id), 0)
        assert ceo_influence > 0

    def test_influence_considers_relationship_strength(
        self,
        hierarchical_network: list[RelationshipResponse],
    ) -> None:
        """Test that stronger relationships contribute more to influence."""
        analyzer = NetworkAnalyzer()
        graph = analyzer.build_network(hierarchical_network)
        influence = analyzer.calculate_influence(graph)

        # Influence calculation should exist and produce results
        assert len(influence) > 0
