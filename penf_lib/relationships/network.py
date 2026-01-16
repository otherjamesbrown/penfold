"""NetworkAnalyzer - Network Analysis for Relationships.

This module provides network analysis capabilities for relationship data,
enabling identification of communication patterns, hubs, bottlenecks,
and collaboration opportunities.

Features:
- Network graph building from relationships
- Centrality calculations (degree, betweenness, closeness, eigenvector)
- Communication hub identification
- Isolated participant detection
- Community/cluster detection
- Bottleneck and influence pattern analysis
- Export functionality (JSON, DOT, CSV)
"""

from __future__ import annotations

import csv
import io
import json
from collections import defaultdict
from dataclasses import dataclass, field
from enum import Enum
from typing import Any
from uuid import UUID

import networkx as nx
from pydantic import BaseModel, Field

from .models import (
    EntityReference,
    RelationshipNetworkEdge,
    RelationshipNetworkNode,
    RelationshipResponse,
    RelationshipType,
)


class ExportFormat(str, Enum):
    """Supported export formats for network data."""

    JSON = "json"
    DOT = "dot"
    CSV = "csv"


class CentralityMetrics(BaseModel):
    """Centrality metrics for a node in the network.

    Centrality measures indicate the importance or influence of a node
    within the network structure.
    """

    entity_id: UUID = Field(description="Entity ID for these metrics")
    degree_centrality: float = Field(
        ge=0.0,
        le=1.0,
        description="Normalized degree centrality (connection ratio)",
    )
    betweenness_centrality: float = Field(
        ge=0.0,
        le=1.0,
        description="Betweenness centrality (bridge importance)",
    )
    closeness_centrality: float = Field(
        ge=0.0,
        le=1.0,
        description="Closeness centrality (average distance to all nodes)",
    )
    eigenvector_centrality: float = Field(
        ge=0.0,
        le=1.0,
        default=0.0,
        description="Eigenvector centrality (influence measure)",
    )


class CommunityInfo(BaseModel):
    """Information about a detected community or cluster.

    Communities are groups of nodes that are more densely connected
    to each other than to the rest of the network.
    """

    community_id: str = Field(description="Unique identifier for the community")
    members: list[UUID] = Field(description="Entity IDs of community members")
    cohesion_score: float = Field(
        ge=0.0,
        le=1.0,
        description="Internal density of the community",
    )
    key_topics: list[str] = Field(
        default_factory=list,
        description="Common topics within this community",
    )

    @property
    def size(self) -> int:
        """Return the number of members in the community."""
        return len(self.members)


class BottleneckInfo(BaseModel):
    """Information about a communication bottleneck.

    Bottlenecks are nodes whose removal would significantly impact
    network connectivity.
    """

    entity_id: UUID = Field(description="Entity ID of the bottleneck")
    display_name: str = Field(description="Human-readable name")
    impact_score: float = Field(
        ge=0.0,
        le=1.0,
        description="Impact score if this node is removed",
    )
    connected_components_affected: int = Field(
        description="Number of components that would be disconnected",
    )
    reason: str = Field(
        default="",
        description="Explanation of why this is a bottleneck",
    )


class CollaborationOpportunity(BaseModel):
    """A potential collaboration opportunity between entities.

    Identifies pairs of entities that don't have a direct relationship
    but might benefit from one based on network structure.
    """

    source_entity_id: UUID = Field(description="First entity ID")
    source_display_name: str = Field(description="First entity name")
    target_entity_id: UUID = Field(description="Second entity ID")
    target_display_name: str = Field(description="Second entity name")
    common_neighbors: int = Field(
        description="Number of shared connections",
    )
    opportunity_score: float = Field(
        ge=0.0,
        le=1.0,
        description="Likelihood that collaboration would be beneficial",
    )
    reason: str = Field(
        default="",
        description="Explanation of why this is an opportunity",
    )


@dataclass
class NetworkGraph:
    """In-memory graph representation for network analysis.

    Stores nodes and edges with efficient lookup structures
    for graph algorithms.
    """

    nodes: dict[UUID, EntityReference] = field(default_factory=dict)
    edges: list[RelationshipNetworkEdge] = field(default_factory=list)
    _adjacency: dict[UUID, set[UUID]] = field(default_factory=lambda: defaultdict(set))
    _edge_weights: dict[tuple[UUID, UUID], float] = field(default_factory=dict)

    def add_node(self, entity: EntityReference) -> None:
        """Add a node to the graph.

        Args:
            entity: Entity reference to add as a node
        """
        self.nodes[entity.entity_id] = entity

    def add_edge(self, edge: RelationshipNetworkEdge) -> None:
        """Add an edge to the graph.

        Args:
            edge: Network edge to add
        """
        self.edges.append(edge)

        # Update adjacency list (treat as undirected for most analyses)
        self._adjacency[edge.source_entity_id].add(edge.target_entity_id)
        self._adjacency[edge.target_entity_id].add(edge.source_entity_id)

        # Store edge weight
        self._edge_weights[(edge.source_entity_id, edge.target_entity_id)] = (
            edge.combined_strength
        )
        self._edge_weights[(edge.target_entity_id, edge.source_entity_id)] = (
            edge.combined_strength
        )

    def get_neighbors(self, entity_id: UUID) -> set[UUID]:
        """Get all neighbors of a node.

        Args:
            entity_id: ID of the node to get neighbors for

        Returns:
            Set of neighbor entity IDs
        """
        return self._adjacency.get(entity_id, set())

    def get_edge_weight(self, source_id: UUID, target_id: UUID) -> float:
        """Get the weight of an edge between two nodes.

        Args:
            source_id: Source node ID
            target_id: Target node ID

        Returns:
            Edge weight, or 0.0 if no edge exists
        """
        return self._edge_weights.get((source_id, target_id), 0.0)


class NetworkInsights(BaseModel):
    """Comprehensive network analysis results.

    Contains all insights derived from network analysis including
    hubs, isolated participants, communities, and more.
    """

    total_nodes: int = Field(description="Total number of nodes in the network")
    total_edges: int = Field(description="Total number of edges in the network")
    density: float = Field(
        ge=0.0,
        le=1.0,
        default=0.0,
        description="Network density (actual edges / possible edges)",
    )
    hubs: list[RelationshipNetworkNode] = Field(
        default_factory=list,
        description="Communication hubs in the network",
    )
    isolated_participants: list[RelationshipNetworkNode] = Field(
        default_factory=list,
        description="Participants with few connections",
    )
    communities: list[CommunityInfo] = Field(
        default_factory=list,
        description="Detected communities/clusters",
    )
    bottlenecks: list[BottleneckInfo] = Field(
        default_factory=list,
        description="Communication bottlenecks",
    )
    collaboration_opportunities: list[CollaborationOpportunity] = Field(
        default_factory=list,
        description="Potential collaboration opportunities",
    )
    influence_scores: dict[str, float] = Field(
        default_factory=dict,
        description="Influence scores by entity ID (as string)",
    )


class NetworkAnalyzer:
    """Analyzer for relationship network structures.

    Provides methods for building network graphs from relationships
    and computing various network analysis metrics.
    """

    def __init__(self) -> None:
        """Initialize the network analyzer."""
        pass

    def build_network(
        self,
        relationships: list[RelationshipResponse],
    ) -> NetworkGraph:
        """Build a network graph from relationships.

        Args:
            relationships: List of relationships to build graph from

        Returns:
            NetworkGraph containing nodes and edges
        """
        graph = NetworkGraph()

        # Add all unique entities as nodes
        for rel in relationships:
            graph.add_node(rel.source_entity)
            graph.add_node(rel.target_entity)

            # Create edge
            edge = RelationshipNetworkEdge(
                source_entity_id=rel.source_entity.entity_id,
                target_entity_id=rel.target_entity.entity_id,
                relationship_types=[rel.relationship_type],
                combined_strength=rel.confidence_score,
                interaction_count=rel.evidence_count,
                first_interaction=rel.first_observed,
                last_interaction=rel.last_observed,
            )
            graph.add_edge(edge)

        return graph

    def _to_networkx(self, graph: NetworkGraph) -> nx.Graph:
        """Convert our NetworkGraph to a networkx Graph.

        Args:
            graph: Our internal network graph

        Returns:
            networkx Graph instance
        """
        G = nx.Graph()

        # Add nodes
        for entity_id, node in graph.nodes.items():
            G.add_node(entity_id, **node.model_dump())

        # Add edges
        for edge in graph.edges:
            G.add_edge(
                edge.source_entity_id,
                edge.target_entity_id,
                weight=float(edge.combined_strength),
            )

        return G

    def calculate_centrality(
        self,
        graph: NetworkGraph,
    ) -> dict[UUID, CentralityMetrics]:
        """Calculate centrality metrics for all nodes.

        Args:
            graph: Network graph to analyze

        Returns:
            Dictionary mapping entity IDs to their centrality metrics
        """
        n = len(graph.nodes)
        if n <= 1:
            # Return default metrics for trivial graphs
            return {
                entity_id: CentralityMetrics(
                    entity_id=entity_id,
                    degree_centrality=0.0,
                    betweenness_centrality=0.0,
                    closeness_centrality=0.0,
                    eigenvector_centrality=0.0,
                )
                for entity_id in graph.nodes
            }

        # Calculate degree centrality
        degree_centrality = self._calculate_degree_centrality(graph)

        # Calculate betweenness centrality
        betweenness_centrality = self._calculate_betweenness_centrality(graph)

        # Calculate closeness centrality
        closeness_centrality = self._calculate_closeness_centrality(graph)

        # Calculate eigenvector centrality (simplified)
        eigenvector_centrality = self._calculate_eigenvector_centrality(graph)

        # Combine all metrics
        metrics = {}
        for entity_id in graph.nodes:
            metrics[entity_id] = CentralityMetrics(
                entity_id=entity_id,
                degree_centrality=degree_centrality.get(entity_id, 0.0),
                betweenness_centrality=betweenness_centrality.get(entity_id, 0.0),
                closeness_centrality=closeness_centrality.get(entity_id, 0.0),
                eigenvector_centrality=eigenvector_centrality.get(entity_id, 0.0),
            )

        return metrics

    def _calculate_degree_centrality(
        self,
        graph: NetworkGraph,
    ) -> dict[UUID, float]:
        """Calculate degree centrality for each node using networkx.

        Degree centrality is the fraction of nodes that a node is connected to.

        Args:
            graph: Network graph

        Returns:
            Dictionary mapping entity IDs to degree centrality
        """
        if len(graph.nodes) <= 1:
            return dict.fromkeys(graph.nodes, 0.0)

        G = self._to_networkx(graph)
        return nx.degree_centrality(G)

    def _calculate_betweenness_centrality(
        self,
        graph: NetworkGraph,
    ) -> dict[UUID, float]:
        """Calculate betweenness centrality for each node using networkx.

        Betweenness centrality measures how often a node lies on shortest
        paths between other nodes.

        Args:
            graph: Network graph

        Returns:
            Dictionary mapping entity IDs to betweenness centrality
        """
        if len(graph.nodes) <= 2:
            return dict.fromkeys(graph.nodes, 0.0)

        G = self._to_networkx(graph)
        return nx.betweenness_centrality(G, normalized=True)

    def _calculate_closeness_centrality(
        self,
        graph: NetworkGraph,
    ) -> dict[UUID, float]:
        """Calculate closeness centrality for each node using networkx.

        Closeness centrality is based on the average shortest path length
        from a node to all other nodes.

        Args:
            graph: Network graph

        Returns:
            Dictionary mapping entity IDs to closeness centrality
        """
        if len(graph.nodes) <= 1:
            return dict.fromkeys(graph.nodes, 0.0)

        G = self._to_networkx(graph)
        # Use wf_improved=True for Wasserman-Faust normalization (handles disconnected graphs)
        return nx.closeness_centrality(G, wf_improved=True)

    def _calculate_eigenvector_centrality(
        self,
        graph: NetworkGraph,
        max_iterations: int = 100,
        tolerance: float = 1e-6,
    ) -> dict[UUID, float]:
        """Calculate eigenvector centrality using networkx.

        Eigenvector centrality measures the influence of a node based on
        its connections to other influential nodes.

        Args:
            graph: Network graph
            max_iterations: Maximum iterations for convergence
            tolerance: Convergence tolerance

        Returns:
            Dictionary mapping entity IDs to eigenvector centrality
        """
        if len(graph.nodes) == 0:
            return {}

        G = self._to_networkx(graph)
        try:
            return nx.eigenvector_centrality(
                G, max_iter=max_iterations, tol=tolerance
            )
        except nx.PowerIterationFailedConvergence:
            # Fall back to degree centrality if eigenvector doesn't converge
            return nx.degree_centrality(G)

    def identify_hubs(
        self,
        graph: NetworkGraph,
        threshold: float = 0.5,
    ) -> list[RelationshipNetworkNode]:
        """Identify communication hubs in the network.

        Hubs are nodes with high degree centrality that serve as
        connection points for many other nodes.

        Args:
            graph: Network graph
            threshold: Minimum centrality threshold to be considered a hub

        Returns:
            List of network nodes identified as hubs
        """
        if len(graph.nodes) == 0:
            return []

        centrality = self._calculate_degree_centrality(graph)
        hubs = []

        for entity_id, entity in graph.nodes.items():
            score = centrality.get(entity_id, 0.0)
            if score >= threshold:
                node = RelationshipNetworkNode(
                    entity=entity,
                    relationship_count=len(graph.get_neighbors(entity_id)),
                    centrality_score=score,
                    is_hub=True,
                )
                hubs.append(node)

        # Sort by centrality score descending
        hubs.sort(key=lambda h: h.centrality_score, reverse=True)
        return hubs

    def detect_isolated(
        self,
        graph: NetworkGraph,
        connection_threshold: int = 2,
    ) -> list[RelationshipNetworkNode]:
        """Detect isolated participants with few connections.

        Args:
            graph: Network graph
            connection_threshold: Maximum connections to be considered isolated

        Returns:
            List of isolated network nodes
        """
        isolated = []

        for entity_id, entity in graph.nodes.items():
            connection_count = len(graph.get_neighbors(entity_id))
            if connection_count < connection_threshold:
                node = RelationshipNetworkNode(
                    entity=entity,
                    relationship_count=connection_count,
                    centrality_score=0.0,
                    is_hub=False,
                )
                isolated.append(node)

        return isolated

    def detect_communities(
        self,
        graph: NetworkGraph,
    ) -> list[CommunityInfo]:
        """Detect communities/clusters in the network.

        Uses a simple label propagation algorithm for community detection.

        Args:
            graph: Network graph

        Returns:
            List of detected communities
        """
        if len(graph.nodes) == 0:
            return []

        nodes = list(graph.nodes.keys())

        # Initialize each node with its own community
        labels = {node: i for i, node in enumerate(nodes)}

        # Label propagation
        max_iterations = 50
        for _ in range(max_iterations):
            changed = False
            import random
            random.shuffle(nodes)

            for node in nodes:
                neighbors = graph.get_neighbors(node)
                if not neighbors:
                    continue

                # Count neighbor labels
                label_counts: dict[int, int] = defaultdict(int)
                for neighbor in neighbors:
                    label_counts[labels[neighbor]] += 1

                # Adopt most common neighbor label
                if label_counts:
                    max_label = max(label_counts, key=lambda x: label_counts[x])
                    if labels[node] != max_label:
                        labels[node] = max_label
                        changed = True

            if not changed:
                break

        # Group nodes by label
        communities_dict: dict[int, list[UUID]] = defaultdict(list)
        for node, label in labels.items():
            communities_dict[label].append(node)

        # Create community objects
        communities = []
        for i, (_label, members) in enumerate(communities_dict.items()):
            if len(members) >= 2:  # Only include communities with 2+ members
                # Calculate cohesion (internal density)
                internal_edges = 0
                for member in members:
                    for neighbor in graph.get_neighbors(member):
                        if neighbor in members:
                            internal_edges += 1
                internal_edges //= 2  # Each edge counted twice

                possible_edges = len(members) * (len(members) - 1) / 2
                cohesion = internal_edges / possible_edges if possible_edges > 0 else 0.0

                community = CommunityInfo(
                    community_id=f"community-{i}",
                    members=members,
                    cohesion_score=cohesion,
                    key_topics=[],
                )
                communities.append(community)

        return communities

    def identify_bottlenecks(
        self,
        graph: NetworkGraph,
    ) -> list[BottleneckInfo]:
        """Identify communication bottlenecks in the network.

        Bottlenecks are nodes whose removal would significantly impact
        network connectivity (articulation points).

        Args:
            graph: Network graph

        Returns:
            List of bottleneck information
        """
        if len(graph.nodes) <= 2:
            return []

        bottlenecks = []
        betweenness = self._calculate_betweenness_centrality(graph)

        # Nodes with high betweenness are potential bottlenecks
        threshold = 0.1  # Consider nodes with betweenness > 0.1

        for entity_id, score in betweenness.items():
            if score > threshold:
                entity = graph.nodes[entity_id]
                neighbors = graph.get_neighbors(entity_id)

                bottleneck = BottleneckInfo(
                    entity_id=entity_id,
                    display_name=entity.display_name,
                    impact_score=score,
                    connected_components_affected=len(neighbors),
                    reason=f"High betweenness centrality ({score:.2f}) indicates bridge role",
                )
                bottlenecks.append(bottleneck)

        # Sort by impact score
        bottlenecks.sort(key=lambda b: b.impact_score, reverse=True)
        return bottlenecks

    def find_collaboration_opportunities(
        self,
        graph: NetworkGraph,
        min_common_neighbors: int = 1,
    ) -> list[CollaborationOpportunity]:
        """Find potential collaboration opportunities.

        Identifies pairs of nodes that share common neighbors but
        don't have a direct connection.

        Args:
            graph: Network graph
            min_common_neighbors: Minimum common neighbors to suggest

        Returns:
            List of collaboration opportunities
        """
        opportunities = []
        nodes = list(graph.nodes.keys())

        # Check all pairs of non-connected nodes
        for i, node1 in enumerate(nodes):
            neighbors1 = graph.get_neighbors(node1)

            for node2 in nodes[i + 1:]:
                # Skip if already connected
                if node2 in neighbors1:
                    continue

                neighbors2 = graph.get_neighbors(node2)
                common = neighbors1 & neighbors2

                if len(common) >= min_common_neighbors:
                    entity1 = graph.nodes[node1]
                    entity2 = graph.nodes[node2]

                    # Calculate opportunity score based on common neighbors
                    max_possible = min(len(neighbors1), len(neighbors2))
                    score = len(common) / max_possible if max_possible > 0 else 0.0

                    opportunity = CollaborationOpportunity(
                        source_entity_id=node1,
                        source_display_name=entity1.display_name,
                        target_entity_id=node2,
                        target_display_name=entity2.display_name,
                        common_neighbors=len(common),
                        opportunity_score=min(score, 1.0),
                        reason=f"Share {len(common)} common connection(s)",
                    )
                    opportunities.append(opportunity)

        # Sort by opportunity score
        opportunities.sort(key=lambda o: o.opportunity_score, reverse=True)
        return opportunities

    def calculate_density(self, graph: NetworkGraph) -> float:
        """Calculate network density.

        Density is the ratio of actual edges to possible edges.

        Args:
            graph: Network graph

        Returns:
            Network density between 0.0 and 1.0
        """
        n = len(graph.nodes)
        if n <= 1:
            return 0.0

        # For directed graph, max edges = n * (n - 1)
        # For undirected graph, max edges = n * (n - 1) / 2
        # Since we treat as undirected and each relationship creates one edge
        max_edges = n * (n - 1) / 2
        actual_edges = len(graph.edges)

        return actual_edges / max_edges if max_edges > 0 else 0.0

    def calculate_influence(
        self,
        graph: NetworkGraph,
    ) -> dict[UUID, float]:
        """Calculate influence scores for all nodes.

        Influence is based on a combination of centrality metrics
        and relationship types (e.g., MANAGES relationships carry more weight).

        Args:
            graph: Network graph

        Returns:
            Dictionary mapping entity IDs to influence scores
        """
        if len(graph.nodes) == 0:
            return {}

        # Get base eigenvector centrality
        eigen = self._calculate_eigenvector_centrality(graph)

        # Boost influence based on management relationships
        influence = {}
        for entity_id in graph.nodes:
            base_score = eigen.get(entity_id, 0.0)

            # Check for outgoing management relationships
            management_boost = 0.0
            for edge in graph.edges:
                if edge.source_entity_id == entity_id:
                    if RelationshipType.MANAGES in edge.relationship_types:
                        management_boost += 0.1
                    if RelationshipType.LEADS in edge.relationship_types:
                        management_boost += 0.05

            influence[entity_id] = min(base_score + management_boost, 1.0)

        return influence

    def analyze(
        self,
        relationships: list[RelationshipResponse],
    ) -> NetworkInsights:
        """Perform full network analysis.

        Args:
            relationships: List of relationships to analyze

        Returns:
            Comprehensive network insights
        """
        if not relationships:
            return NetworkInsights(
                total_nodes=0,
                total_edges=0,
                density=0.0,
            )

        graph = self.build_network(relationships)
        density = self.calculate_density(graph)
        hubs = self.identify_hubs(graph, threshold=0.5)
        isolated = self.detect_isolated(graph, connection_threshold=2)
        communities = self.detect_communities(graph)
        bottlenecks = self.identify_bottlenecks(graph)
        opportunities = self.find_collaboration_opportunities(graph)
        influence = self.calculate_influence(graph)

        # Convert influence dict to string keys for JSON serialization
        influence_str = {str(k): v for k, v in influence.items()}

        return NetworkInsights(
            total_nodes=len(graph.nodes),
            total_edges=len(graph.edges),
            density=density,
            hubs=hubs,
            isolated_participants=isolated,
            communities=communities,
            bottlenecks=bottlenecks,
            collaboration_opportunities=opportunities,
            influence_scores=influence_str,
        )

    def export(
        self,
        graph: NetworkGraph,
        format: ExportFormat,
        include_metrics: bool = False,
    ) -> str:
        """Export network graph to various formats.

        Args:
            graph: Network graph to export
            format: Export format (JSON, DOT, or CSV)
            include_metrics: Whether to include centrality metrics

        Returns:
            String representation in the requested format
        """
        if format == ExportFormat.JSON:
            return self._export_json(graph, include_metrics)
        elif format == ExportFormat.DOT:
            return self._export_dot(graph)
        elif format == ExportFormat.CSV:
            return self._export_csv(graph)
        else:
            raise ValueError(f"Unsupported export format: {format}")

    def _export_json(
        self,
        graph: NetworkGraph,
        include_metrics: bool = False,
    ) -> str:
        """Export graph to JSON format.

        Args:
            graph: Network graph
            include_metrics: Whether to include centrality metrics

        Returns:
            JSON string
        """
        centrality = {}
        if include_metrics:
            centrality = self.calculate_centrality(graph)

        nodes = []
        for entity_id, entity in graph.nodes.items():
            node_data: dict[str, Any] = {
                "id": str(entity_id),
                "display_name": entity.display_name,
                "entity_type": entity.entity_type.value,
            }

            if include_metrics and entity_id in centrality:
                metrics = centrality[entity_id]
                node_data["degree_centrality"] = metrics.degree_centrality
                node_data["betweenness_centrality"] = metrics.betweenness_centrality
                node_data["closeness_centrality"] = metrics.closeness_centrality

            nodes.append(node_data)

        edges = []
        for edge in graph.edges:
            edge_data = {
                "source": str(edge.source_entity_id),
                "target": str(edge.target_entity_id),
                "relationship_types": [rt.value for rt in edge.relationship_types],
                "weight": edge.combined_strength,
                "interaction_count": edge.interaction_count,
            }
            edges.append(edge_data)

        data = {
            "nodes": nodes,
            "edges": edges,
        }

        return json.dumps(data, indent=2)

    def _export_dot(self, graph: NetworkGraph) -> str:
        """Export graph to DOT format for Graphviz.

        Args:
            graph: Network graph

        Returns:
            DOT format string
        """
        lines = ["digraph RelationshipNetwork {"]
        lines.append("  rankdir=LR;")
        lines.append('  node [shape=ellipse, style=filled, fillcolor="#e0e0e0"];')
        lines.append("")

        # Add nodes
        for entity_id, entity in graph.nodes.items():
            label = entity.display_name.replace('"', '\\"')
            node_id = str(entity_id).replace("-", "_")
            lines.append(f'  "{node_id}" [label="{label}"];')

        lines.append("")

        # Add edges
        for edge in graph.edges:
            source_id = str(edge.source_entity_id).replace("-", "_")
            target_id = str(edge.target_entity_id).replace("-", "_")
            rel_types = ", ".join(rt.value for rt in edge.relationship_types)
            weight = edge.combined_strength

            # Scale pen width by weight
            penwidth = max(1, weight * 3)

            lines.append(
                f'  "{source_id}" -> "{target_id}" '
                f'[label="{rel_types}", penwidth={penwidth:.1f}];'
            )

        lines.append("}")
        return "\n".join(lines)

    def _export_csv(self, graph: NetworkGraph) -> str:
        """Export graph edges to CSV format.

        Uses Python's csv module for proper escaping of special characters
        in names (commas, quotes, newlines).

        Args:
            graph: Network graph

        Returns:
            CSV format string
        """
        output = io.StringIO()
        writer = csv.writer(output, quoting=csv.QUOTE_MINIMAL)

        # Write header
        writer.writerow([
            "source_id",
            "source_name",
            "target_id",
            "target_name",
            "relationship_types",
            "weight",
            "interaction_count",
        ])

        # Write edges
        for edge in graph.edges:
            source = graph.nodes.get(edge.source_entity_id)
            target = graph.nodes.get(edge.target_entity_id)

            source_name = source.display_name if source else "Unknown"
            target_name = target.display_name if target else "Unknown"
            rel_types = "|".join(rt.value for rt in edge.relationship_types)

            writer.writerow([
                str(edge.source_entity_id),
                source_name,
                str(edge.target_entity_id),
                target_name,
                rel_types,
                f"{edge.combined_strength:.2f}",
                str(edge.interaction_count),
            ])

        return output.getvalue().rstrip("\r\n")


__all__ = [
    "ExportFormat",
    "CentralityMetrics",
    "CommunityInfo",
    "BottleneckInfo",
    "CollaborationOpportunity",
    "NetworkGraph",
    "NetworkInsights",
    "NetworkAnalyzer",
]
