package relationships

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository provides access to relationship data.
// It aggregates data from people, projects, teams, and products tables
// to build a relationship graph view.
type Repository struct {
	db     *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new relationship repository.
func NewRepository(db *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

// ListRelationships returns relationships matching the filter.
// Relationships are derived from team_members, project_members, and content_mentions.
func (r *Repository) ListRelationships(ctx context.Context, filter ListRelationshipsFilter) ([]Relationship, int64, error) {
	relationships := make([]Relationship, 0)
	var totalCount int64

	// Get colleague relationships from team_members (people on same team)
	colleagueRels, colleagueCount, err := r.getColleagueRelationships(ctx, filter)
	if err != nil {
		r.logger.Error("Error getting colleague relationships", logging.Err(err))
	} else {
		relationships = append(relationships, colleagueRels...)
		totalCount += colleagueCount
	}

	// Get member_of relationships (people belonging to teams/projects)
	memberRels, memberCount, err := r.getMemberOfRelationships(ctx, filter)
	if err != nil {
		r.logger.Error("Error getting member_of relationships", logging.Err(err))
	} else {
		relationships = append(relationships, memberRels...)
		totalCount += memberCount
	}

	// Get works_on relationships (people working on projects)
	worksOnRels, worksOnCount, err := r.getWorksOnRelationships(ctx, filter)
	if err != nil {
		r.logger.Error("Error getting works_on relationships", logging.Err(err))
	} else {
		relationships = append(relationships, worksOnRels...)
		totalCount += worksOnCount
	}

	// Apply limit and offset to combined results
	if filter.Offset > 0 && filter.Offset < len(relationships) {
		relationships = relationships[filter.Offset:]
	} else if filter.Offset >= len(relationships) {
		relationships = []Relationship{}
	}

	if filter.Limit > 0 && filter.Limit < len(relationships) {
		relationships = relationships[:filter.Limit]
	}

	return relationships, totalCount, nil
}

// getColleagueRelationships finds colleague relationships via shared team membership.
func (r *Repository) getColleagueRelationships(ctx context.Context, filter ListRelationshipsFilter) ([]Relationship, int64, error) {
	if filter.RelationshipType != "" && filter.RelationshipType != RelationshipTypeColleague {
		return nil, 0, nil
	}

	query := `
		SELECT
			p1.id as source_id, p1.canonical_name as source_name,
			p2.id as target_id, p2.canonical_name as target_name,
			t.name as team_name,
			LEAST(p1.created_at, p2.created_at) as first_seen,
			NOW() as last_seen
		FROM team_members tm1
		JOIN team_members tm2 ON tm1.team_id = tm2.team_id AND tm1.person_id < tm2.person_id
		JOIN people p1 ON tm1.person_id = p1.id
		JOIN people p2 ON tm2.person_id = p2.id
		JOIN teams t ON tm1.team_id = t.id
		WHERE t.tenant_id = $1
	`
	args := []interface{}{filter.TenantID}

	if filter.EntityID != "" {
		query += ` AND (p1.id::text = $2 OR p2.id::text = $2)`
		args = append(args, filter.EntityID)
	}

	query += ` ORDER BY p1.canonical_name, p2.canonical_name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query colleague relationships: %w", err)
	}
	defer rows.Close()

	var relationships []Relationship
	for rows.Next() {
		var sourceID, sourceName, targetID, targetName, teamName string
		var firstSeen, lastSeen time.Time

		if err := rows.Scan(&sourceID, &sourceName, &targetID, &targetName, &teamName, &firstSeen, &lastSeen); err != nil {
			return nil, 0, fmt.Errorf("scan colleague relationship: %w", err)
		}

		relationships = append(relationships, Relationship{
			ID:         fmt.Sprintf("rel-colleague-%s-%s", sourceID, targetID),
			SourceID:   fmt.Sprintf("ent-person-%s", sourceID),
			SourceName: sourceName,
			SourceType: EntityTypePerson,
			TargetID:   fmt.Sprintf("ent-person-%s", targetID),
			TargetName: targetName,
			TargetType: EntityTypePerson,
			Type:       RelationshipTypeColleague,
			Status:     RelationshipStatusConfirmed,
			Confidence: 0.95,
			Weight:     0.8,
			Evidence: []Evidence{
				{
					SourceType: "team_membership",
					Excerpt:    fmt.Sprintf("Both are members of team '%s'", teamName),
				},
			},
			FirstSeen:   firstSeen,
			LastSeen:    lastSeen,
			SourceCount: 1,
			TenantID:    filter.TenantID,
		})
	}

	return relationships, int64(len(relationships)), rows.Err()
}

// getMemberOfRelationships finds member_of relationships (people in teams).
func (r *Repository) getMemberOfRelationships(ctx context.Context, filter ListRelationshipsFilter) ([]Relationship, int64, error) {
	if filter.RelationshipType != "" && filter.RelationshipType != RelationshipTypeMemberOf {
		return nil, 0, nil
	}

	query := `
		SELECT
			p.id as person_id, p.canonical_name as person_name,
			t.id as team_id, t.name as team_name,
			tm.role,
			tm.joined_at as first_seen,
			NOW() as last_seen
		FROM team_members tm
		JOIN people p ON tm.person_id = p.id
		JOIN teams t ON tm.team_id = t.id
		WHERE t.tenant_id = $1
	`
	args := []interface{}{filter.TenantID}

	if filter.EntityID != "" {
		query += ` AND (p.id::text = $2 OR t.id::text = $2)`
		args = append(args, filter.EntityID)
	}

	query += ` ORDER BY p.canonical_name, t.name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query member_of relationships: %w", err)
	}
	defer rows.Close()

	var relationships []Relationship
	for rows.Next() {
		var personID, personName, teamID, teamName string
		var role *string
		var firstSeen, lastSeen time.Time

		if err := rows.Scan(&personID, &personName, &teamID, &teamName, &role, &firstSeen, &lastSeen); err != nil {
			return nil, 0, fmt.Errorf("scan member_of relationship: %w", err)
		}

		roleStr := "member"
		if role != nil {
			roleStr = *role
		}

		relationships = append(relationships, Relationship{
			ID:         fmt.Sprintf("rel-member-%s-%s", personID, teamID),
			SourceID:   fmt.Sprintf("ent-person-%s", personID),
			SourceName: personName,
			SourceType: EntityTypePerson,
			TargetID:   fmt.Sprintf("ent-org-%s", teamID),
			TargetName: teamName,
			TargetType: EntityTypeOrganization,
			Type:       RelationshipTypeMemberOf,
			Status:     RelationshipStatusConfirmed,
			Confidence: 0.99,
			Weight:     1.0,
			Evidence: []Evidence{
				{
					SourceType: "team_membership",
					Excerpt:    fmt.Sprintf("Member with role '%s'", roleStr),
				},
			},
			FirstSeen:   firstSeen,
			LastSeen:    lastSeen,
			SourceCount: 1,
			TenantID:    filter.TenantID,
		})
	}

	return relationships, int64(len(relationships)), rows.Err()
}

// getWorksOnRelationships finds works_on relationships (people on projects).
func (r *Repository) getWorksOnRelationships(ctx context.Context, filter ListRelationshipsFilter) ([]Relationship, int64, error) {
	if filter.RelationshipType != "" && filter.RelationshipType != RelationshipTypeWorksOn {
		return nil, 0, nil
	}

	query := `
		SELECT
			p.id as person_id, p.canonical_name as person_name,
			pr.id as project_id, pr.name as project_name,
			pm.role,
			pm.added_at as first_seen,
			NOW() as last_seen
		FROM project_members pm
		JOIN people p ON pm.person_id = p.id
		JOIN projects pr ON pm.project_id = pr.id
		WHERE pr.tenant_id = $1 AND pm.person_id IS NOT NULL
	`
	args := []interface{}{filter.TenantID}

	if filter.EntityID != "" {
		query += ` AND (p.id::text = $2 OR pr.id::text = $2)`
		args = append(args, filter.EntityID)
	}

	query += ` ORDER BY p.canonical_name, pr.name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query works_on relationships: %w", err)
	}
	defer rows.Close()

	var relationships []Relationship
	for rows.Next() {
		var personID, personName, projectID, projectName string
		var role *string
		var firstSeen, lastSeen time.Time

		if err := rows.Scan(&personID, &personName, &projectID, &projectName, &role, &firstSeen, &lastSeen); err != nil {
			return nil, 0, fmt.Errorf("scan works_on relationship: %w", err)
		}

		roleStr := "contributor"
		if role != nil {
			roleStr = *role
		}

		relationships = append(relationships, Relationship{
			ID:         fmt.Sprintf("rel-works-%s-%s", personID, projectID),
			SourceID:   fmt.Sprintf("ent-person-%s", personID),
			SourceName: personName,
			SourceType: EntityTypePerson,
			TargetID:   fmt.Sprintf("ent-project-%s", projectID),
			TargetName: projectName,
			TargetType: EntityTypeProject,
			Type:       RelationshipTypeWorksOn,
			Status:     RelationshipStatusConfirmed,
			Confidence: 0.95,
			Weight:     0.7,
			Evidence: []Evidence{
				{
					SourceType: "project_membership",
					Excerpt:    fmt.Sprintf("Project member with role '%s'", roleStr),
				},
			},
			FirstSeen:   firstSeen,
			LastSeen:    lastSeen,
			SourceCount: 1,
			TenantID:    filter.TenantID,
		})
	}

	return relationships, int64(len(relationships)), rows.Err()
}

// GetRelationship retrieves a single relationship by ID.
func (r *Repository) GetRelationship(ctx context.Context, tenantID, relationshipID string) (*Relationship, error) {
	// Parse the relationship ID to determine type and entity IDs
	// Format: rel-{type}-{sourceID}-{targetID}
	parts := strings.Split(relationshipID, "-")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid relationship ID format: %s", relationshipID)
	}

	relType := parts[1]
	sourceID := parts[2]
	targetID := parts[3]

	// Fetch based on relationship type
	filter := ListRelationshipsFilter{
		TenantID: tenantID,
		EntityID: sourceID,
		Limit:    100,
	}

	relationships, _, err := r.ListRelationships(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Find the matching relationship
	for _, rel := range relationships {
		if rel.ID == relationshipID {
			return &rel, nil
		}
		// Also check if IDs match (in case of format differences)
		if strings.Contains(rel.SourceID, sourceID) && strings.Contains(rel.TargetID, targetID) {
			if (relType == "colleague" && rel.Type == RelationshipTypeColleague) ||
				(relType == "member" && rel.Type == RelationshipTypeMemberOf) ||
				(relType == "works" && rel.Type == RelationshipTypeWorksOn) {
				return &rel, nil
			}
		}
	}

	return nil, nil
}

// SearchRelationships searches relationships by entity name.
func (r *Repository) SearchRelationships(ctx context.Context, query SearchRelationshipsQuery) ([]Relationship, error) {
	searchTerm := strings.ToLower(query.Query)

	// Get all relationships and filter by name match
	filter := ListRelationshipsFilter{
		TenantID:      query.TenantID,
		MinConfidence: query.MinConfidence,
		Limit:         500, // Get more to filter
	}

	relationships, _, err := r.ListRelationships(ctx, filter)
	if err != nil {
		return nil, err
	}

	var results []Relationship
	for _, rel := range relationships {
		if strings.Contains(strings.ToLower(rel.SourceName), searchTerm) ||
			strings.Contains(strings.ToLower(rel.TargetName), searchTerm) {
			results = append(results, rel)
		}
	}

	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	return results, nil
}

// ListEntities returns entities matching the filter.
func (r *Repository) ListEntities(ctx context.Context, filter ListEntitiesFilter) ([]Entity, int64, error) {
	entities := make([]Entity, 0)
	var totalCount int64

	// Get people entities
	if filter.EntityType == "" || filter.EntityType == EntityTypePerson {
		people, count, err := r.getPeopleEntities(ctx, filter)
		if err != nil {
			return nil, 0, fmt.Errorf("get people entities: %w", err)
		}
		entities = append(entities, people...)
		totalCount += count
	}

	// Get organization entities (teams)
	if filter.EntityType == "" || filter.EntityType == EntityTypeOrganization {
		orgs, count, err := r.getOrganizationEntities(ctx, filter)
		if err != nil {
			return nil, 0, fmt.Errorf("get organization entities: %w", err)
		}
		entities = append(entities, orgs...)
		totalCount += count
	}

	// Get project entities
	if filter.EntityType == "" || filter.EntityType == EntityTypeProject {
		projects, count, err := r.getProjectEntities(ctx, filter)
		if err != nil {
			return nil, 0, fmt.Errorf("get project entities: %w", err)
		}
		entities = append(entities, projects...)
		totalCount += count
	}

	// Get product entities
	if filter.EntityType == "" || filter.EntityType == EntityTypeProduct {
		products, count, err := r.getProductEntities(ctx, filter)
		if err != nil {
			return nil, 0, fmt.Errorf("get product entities: %w", err)
		}
		entities = append(entities, products...)
		totalCount += count
	}

	// Apply limit and offset
	if filter.Offset > 0 && filter.Offset < len(entities) {
		entities = entities[filter.Offset:]
	} else if filter.Offset >= len(entities) {
		entities = []Entity{}
	}

	if filter.Limit > 0 && filter.Limit < len(entities) {
		entities = entities[:filter.Limit]
	}

	return entities, totalCount, nil
}

func (r *Repository) getPeopleEntities(ctx context.Context, filter ListEntitiesFilter) ([]Entity, int64, error) {
	query := `
		SELECT
			p.id, p.canonical_name, p.primary_email, p.job_title, p.department,
			p.confidence_score, p.created_at, p.updated_at,
			COUNT(DISTINCT pa.id) as alias_count,
			COUNT(DISTINCT tm.team_id) + COUNT(DISTINCT pm.project_id) as relation_count,
			p.sent_count, p.received_count, p.entity_resolution_metadata
		FROM people p
		LEFT JOIN person_aliases pa ON p.id = pa.person_id
		LEFT JOIN team_members tm ON p.id = tm.person_id
		LEFT JOIN project_members pm ON p.id = pm.person_id
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	// Only filter by tenant_id if a specific, non-default value is provided.
	// People may have been created with per-source tenant_ids, so filtering
	// by the CLI's configured tenant_id would miss most records.
	if filter.TenantID != "" && filter.TenantID != "00000000-0000-0000-0000-000000000000" {
		query += fmt.Sprintf(` AND p.tenant_id = $%d`, argNum)
		args = append(args, filter.TenantID)
		argNum++
	}

	if filter.Search != "" {
		query += fmt.Sprintf(` AND (LOWER(p.canonical_name) LIKE $%d OR LOWER(p.primary_email) LIKE $%d)`, argNum, argNum)
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		argNum++
	}

	if filter.MinConfidence > 0 {
		query += fmt.Sprintf(` AND p.confidence_score >= $%d`, argNum)
		args = append(args, filter.MinConfidence)
		argNum++
	}

	query += ` GROUP BY p.id ORDER BY relation_count DESC, p.canonical_name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query people entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var id int64
		var canonicalName string
		var primaryEmail *string
		var jobTitle, department *string
		var confidence *float64
		var createdAt, updatedAt time.Time
		var aliasCount, relationCount int
		var sentCount, receivedCount int
		var resolutionMetadata *[]byte

		if err := rows.Scan(&id, &canonicalName, &primaryEmail, &jobTitle, &department,
			&confidence, &createdAt, &updatedAt, &aliasCount, &relationCount, &sentCount, &receivedCount, &resolutionMetadata); err != nil {
			return nil, 0, fmt.Errorf("scan people entity: %w", err)
		}

		metadata := map[string]string{}

		// First, parse entity_resolution_metadata JSONB if present
		if resolutionMetadata != nil && len(*resolutionMetadata) > 0 {
			var customMetadata map[string]string
			if err := json.Unmarshal(*resolutionMetadata, &customMetadata); err == nil {
				for k, v := range customMetadata {
					metadata[k] = v
				}
			}
		}

		// Then, overlay hardcoded column-level metadata (takes precedence)
		if primaryEmail != nil && *primaryEmail != "" {
			metadata["email"] = *primaryEmail
		}
		if jobTitle != nil && *jobTitle != "" {
			metadata["job_title"] = *jobTitle
		}
		if department != nil && *department != "" {
			metadata["department"] = *department
		}

		conf := 0.0
		if confidence != nil {
			conf = *confidence
		}
		entities = append(entities, Entity{
			ID:            fmt.Sprintf("ent-person-%d", id),
			Name:          canonicalName,
			Type:          EntityTypePerson,
			CanonicalName: canonicalName,
			Confidence:    conf,
			SourceCount:   aliasCount,
			FirstSeen:     createdAt,
			LastSeen:      updatedAt,
			Metadata:      metadata,
			RelationCount: relationCount,
			SentCount:     sentCount,
			ReceivedCount: receivedCount,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
		})
	}

	return entities, int64(len(entities)), rows.Err()
}

func (r *Repository) getOrganizationEntities(ctx context.Context, filter ListEntitiesFilter) ([]Entity, int64, error) {
	query := `
		SELECT
			t.id, t.name, t.description, t.created_at, t.updated_at,
			COUNT(DISTINCT tm.person_id) as member_count
		FROM teams t
		LEFT JOIN team_members tm ON t.id = tm.team_id
		WHERE t.tenant_id = $1
	`
	args := []interface{}{filter.TenantID}
	argNum := 2

	if filter.Search != "" {
		query += fmt.Sprintf(` AND LOWER(t.name) LIKE $%d`, argNum)
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		argNum++
	}

	query += ` GROUP BY t.id ORDER BY member_count DESC, t.name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query organization entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var id int64
		var name string
		var description *string
		var createdAt, updatedAt time.Time
		var memberCount int

		if err := rows.Scan(&id, &name, &description, &createdAt, &updatedAt, &memberCount); err != nil {
			return nil, 0, fmt.Errorf("scan organization entity: %w", err)
		}

		metadata := map[string]string{
			"member_count": fmt.Sprintf("%d", memberCount),
		}
		if description != nil && *description != "" {
			metadata["description"] = *description
		}

		entities = append(entities, Entity{
			ID:            fmt.Sprintf("ent-org-%d", id),
			Name:          name,
			Type:          EntityTypeOrganization,
			Confidence:    1.0,
			SourceCount:   1,
			FirstSeen:     createdAt,
			LastSeen:      updatedAt,
			Metadata:      metadata,
			RelationCount: memberCount,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
		})
	}

	return entities, int64(len(entities)), rows.Err()
}

func (r *Repository) getProjectEntities(ctx context.Context, filter ListEntitiesFilter) ([]Entity, int64, error) {
	query := `
		SELECT
			pr.id, pr.name, pr.description, pr.created_at, pr.updated_at,
			COUNT(DISTINCT pm.person_id) as member_count
		FROM projects pr
		LEFT JOIN project_members pm ON pr.id = pm.project_id AND pm.person_id IS NOT NULL
		WHERE pr.tenant_id = $1
	`
	args := []interface{}{filter.TenantID}
	argNum := 2

	if filter.Search != "" {
		query += fmt.Sprintf(` AND LOWER(pr.name) LIKE $%d`, argNum)
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		argNum++
	}

	query += ` GROUP BY pr.id ORDER BY member_count DESC, pr.name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query project entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var id int64
		var name string
		var description *string
		var createdAt, updatedAt time.Time
		var memberCount int

		if err := rows.Scan(&id, &name, &description, &createdAt, &updatedAt, &memberCount); err != nil {
			return nil, 0, fmt.Errorf("scan project entity: %w", err)
		}

		metadata := map[string]string{
			"member_count": fmt.Sprintf("%d", memberCount),
		}
		if description != nil && *description != "" {
			metadata["description"] = *description
		}

		entities = append(entities, Entity{
			ID:            fmt.Sprintf("ent-project-%d", id),
			Name:          name,
			Type:          EntityTypeProject,
			Confidence:    1.0,
			SourceCount:   1,
			FirstSeen:     createdAt,
			LastSeen:      updatedAt,
			Metadata:      metadata,
			RelationCount: memberCount,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
		})
	}

	return entities, int64(len(entities)), rows.Err()
}

func (r *Repository) getProductEntities(ctx context.Context, filter ListEntitiesFilter) ([]Entity, int64, error) {
	query := `
		SELECT
			pr.id, pr.name, pr.description, pr.product_type, pr.created_at, pr.updated_at
		FROM products pr
		WHERE pr.tenant_id = $1
	`
	args := []interface{}{filter.TenantID}
	argNum := 2

	if filter.Search != "" {
		query += fmt.Sprintf(` AND LOWER(pr.name) LIKE $%d`, argNum)
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		argNum++
	}

	query += ` ORDER BY pr.name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query product entities: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var id int64
		var name string
		var description, productType *string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &name, &description, &productType, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan product entity: %w", err)
		}

		metadata := map[string]string{}
		if description != nil && *description != "" {
			metadata["description"] = *description
		}
		if productType != nil && *productType != "" {
			metadata["product_type"] = *productType
		}

		entities = append(entities, Entity{
			ID:            fmt.Sprintf("ent-product-%d", id),
			Name:          name,
			Type:          EntityTypeProduct,
			Confidence:    1.0,
			SourceCount:   1,
			FirstSeen:     createdAt,
			LastSeen:      updatedAt,
			Metadata:      metadata,
			RelationCount: 0,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
		})
	}

	return entities, int64(len(entities)), rows.Err()
}

// GetEntity retrieves a single entity by ID.
func (r *Repository) GetEntity(ctx context.Context, tenantID, entityID string) (*Entity, error) {
	// Parse entity ID to determine type
	// Format: ent-{type}-{id}
	parts := strings.Split(entityID, "-")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid entity ID format: %s", entityID)
	}

	entType := parts[1]

	filter := ListEntitiesFilter{
		TenantID: tenantID,
		Limit:    500,
	}

	switch entType {
	case "person":
		filter.EntityType = EntityTypePerson
	case "org":
		filter.EntityType = EntityTypeOrganization
	case "project":
		filter.EntityType = EntityTypeProject
	case "product":
		filter.EntityType = EntityTypeProduct
	}

	entities, _, err := r.ListEntities(ctx, filter)
	if err != nil {
		return nil, err
	}

	for _, ent := range entities {
		if ent.ID == entityID {
			return &ent, nil
		}
	}

	return nil, nil
}

// GetNetworkStats returns statistics about the relationship network.
func (r *Repository) GetNetworkStats(ctx context.Context, tenantID string) (*NetworkStats, error) {
	stats := &NetworkStats{
		EntityTypeCounts: make(map[EntityType]int),
		RelTypeCounts:    make(map[RelationshipType]int),
	}

	// Count entities by type
	entities, _, err := r.ListEntities(ctx, ListEntitiesFilter{TenantID: tenantID, Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("counting entities: %w", err)
	}

	for _, ent := range entities {
		stats.TotalNodes++
		stats.EntityTypeCounts[ent.Type]++
	}

	// Count relationships by type
	relationships, _, err := r.ListRelationships(ctx, ListRelationshipsFilter{TenantID: tenantID, Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("counting relationships: %w", err)
	}

	for _, rel := range relationships {
		stats.TotalEdges++
		stats.RelTypeCounts[rel.Type]++
	}

	// Calculate density and average connections
	if stats.TotalNodes > 1 {
		maxEdges := float64(stats.TotalNodes * (stats.TotalNodes - 1) / 2)
		stats.Density = float64(stats.TotalEdges) / maxEdges
		stats.AvgConnections = float64(stats.TotalEdges*2) / float64(stats.TotalNodes)
	}

	// Estimate clusters (simplified - just count distinct teams)
	var clusterCount int
	err = r.db.QueryRow(ctx, `SELECT COUNT(*) FROM teams WHERE tenant_id = $1`, tenantID).Scan(&clusterCount)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("counting clusters: %w", err)
	}
	stats.ClusterCount = clusterCount

	return stats, nil
}

// GetCentralEntities returns the most connected entities.
func (r *Repository) GetCentralEntities(ctx context.Context, tenantID string, limit int) ([]Entity, error) {
	if limit <= 0 {
		limit = 10
	}

	entities, _, err := r.ListEntities(ctx, ListEntitiesFilter{
		TenantID: tenantID,
		Limit:    limit * 3, // Get extra for filtering
	})
	if err != nil {
		return nil, err
	}

	// Sort by relation count (already sorted by query)
	if len(entities) > limit {
		entities = entities[:limit]
	}

	return entities, nil
}

// GetClusters returns clusters in the relationship network.
func (r *Repository) GetClusters(ctx context.Context, tenantID string) ([]NetworkCluster, error) {
	// Get teams as clusters
	query := `
		SELECT
			t.id, t.name,
			COUNT(DISTINCT tm.person_id) as member_count
		FROM teams t
		LEFT JOIN team_members tm ON t.id = tm.team_id
		WHERE t.tenant_id = $1
		GROUP BY t.id
		ORDER BY member_count DESC
	`

	rows, err := r.db.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	var clusters []NetworkCluster
	for rows.Next() {
		var id int64
		var name string
		var memberCount int

		if err := rows.Scan(&id, &name, &memberCount); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}

		// Get top entities in this cluster
		topEntities, _, err := r.getPeopleInTeam(ctx, tenantID, id, 3)
		if err != nil {
			r.logger.Warn("Error getting top entities for cluster",
				logging.F("cluster_id", id),
				logging.Err(err))
		}

		// Calculate density (connections within team / possible connections)
		var density float64
		if memberCount > 1 {
			possibleConnections := memberCount * (memberCount - 1) / 2
			// Assume everyone in a team is connected
			density = 1.0
			if possibleConnections > 0 {
				density = float64(possibleConnections) / float64(possibleConnections)
			}
		}

		clusters = append(clusters, NetworkCluster{
			ID:          fmt.Sprintf("cluster-%d", id),
			Name:        name,
			EntityCount: memberCount,
			TopEntities: topEntities,
			Density:     density,
		})
	}

	return clusters, rows.Err()
}

func (r *Repository) getPeopleInTeam(ctx context.Context, tenantID string, teamID int64, limit int) ([]Entity, int64, error) {
	query := `
		SELECT
			p.id, p.canonical_name, p.confidence, p.created_at, p.updated_at
		FROM team_members tm
		JOIN people p ON tm.person_id = p.id
		WHERE p.tenant_id = $1 AND tm.team_id = $2
		ORDER BY p.canonical_name
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, tenantID, teamID, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("query team members: %w", err)
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var id int64
		var canonicalName string
		var confidence float64
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &canonicalName, &confidence, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan team member: %w", err)
		}

		entities = append(entities, Entity{
			ID:         fmt.Sprintf("ent-person-%d", id),
			Name:       canonicalName,
			Type:       EntityTypePerson,
			Confidence: confidence,
			FirstSeen:  createdAt,
			LastSeen:   updatedAt,
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		})
	}

	return entities, int64(len(entities)), rows.Err()
}

// ListConflicts returns relationship conflicts matching the filter.
// Note: This is a placeholder as there's no dedicated conflicts table yet.
// Conflicts would be detected by analyzing duplicate entities, contradictory relationships, etc.
func (r *Repository) ListConflicts(ctx context.Context, filter ListConflictsFilter) ([]RelationshipConflict, int64, error) {
	// Detect potential duplicate people
	conflicts := make([]RelationshipConflict, 0)

	if filter.Status == "" || filter.Status == ConflictStatusPending {
		// Find people with potential_duplicates set
		query := `
			SELECT id, canonical_name, potential_duplicates
			FROM people
			WHERE tenant_id = $1 AND array_length(potential_duplicates, 1) > 0
			LIMIT $2
		`
		limit := filter.Limit
		if limit <= 0 {
			limit = 50
		}

		rows, err := r.db.Query(ctx, query, filter.TenantID, limit)
		if err != nil {
			return nil, 0, fmt.Errorf("query conflicts: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var id int64
			var name string
			var duplicates []int64

			if err := rows.Scan(&id, &name, &duplicates); err != nil {
				continue
			}

			conflicts = append(conflicts, RelationshipConflict{
				ID:              fmt.Sprintf("conf-dup-%d", id),
				TenantID:        filter.TenantID,
				Type:            "duplicate_entity",
				Description:     fmt.Sprintf("Possible duplicate: '%s' has %d potential duplicates", name, len(duplicates)),
				SuggestedAction: "Review and merge entities using 'penf relationship entity merge'",
				Status:          ConflictStatusPending,
				CreatedAt:       time.Now(),
			})
		}
	}

	return conflicts, int64(len(conflicts)), nil
}

// GetConflict retrieves a single conflict by ID.
func (r *Repository) GetConflict(ctx context.Context, tenantID, conflictID string) (*RelationshipConflict, error) {
	conflicts, _, err := r.ListConflicts(ctx, ListConflictsFilter{
		TenantID: tenantID,
		Limit:    100,
	})
	if err != nil {
		return nil, err
	}

	for _, c := range conflicts {
		if c.ID == conflictID {
			return &c, nil
		}
	}

	return nil, nil
}

// ResolveConflict resolves a relationship conflict.
func (r *Repository) ResolveConflict(ctx context.Context, req ResolveConflictRequest) (*ResolveConflictResult, error) {
	// Parse conflict ID to get the entity ID
	// Format: conf-dup-{id}
	parts := strings.Split(req.ConflictID, "-")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid conflict ID format: %s", req.ConflictID)
	}

	// Clear the potential_duplicates array based on strategy
	if req.Strategy == ConflictStrategyManual {
		// Just mark for manual review - don't auto-resolve
		return &ResolveConflictResult{
			Conflict: &RelationshipConflict{
				ID:       req.ConflictID,
				TenantID: req.TenantID,
				Status:   ConflictStatusPending,
			},
			RelationshipsUpdated: 0,
		}, nil
	}

	// For other strategies, clear the duplicates (simplified resolution)
	entityID := parts[2]
	_, err := r.db.Exec(ctx, `
		UPDATE people SET potential_duplicates = '{}' WHERE id = $1 AND tenant_id = $2
	`, entityID, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("resolve conflict: %w", err)
	}

	now := time.Now()
	return &ResolveConflictResult{
		Conflict: &RelationshipConflict{
			ID:         req.ConflictID,
			TenantID:   req.TenantID,
			Status:     ConflictStatusResolved,
			Resolution: string(req.Strategy),
			ResolvedBy: req.ResolvedBy,
			ResolvedAt: &now,
		},
		RelationshipsUpdated: 1,
	}, nil
}

// InsertRelationship manually creates a new relationship in the database.
func (r *Repository) InsertRelationship(ctx context.Context, req InsertRelationshipRequest) (*Relationship, error) {
	query := `
		INSERT INTO relationships (
			tenant_id,
			source_entity_type,
			source_entity_id,
			target_entity_type,
			target_entity_id,
			relationship_type,
			relationship_subtype,
			confidence_score,
			user_confirmed,
			lifecycle_state,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	var id int64
	var createdAt, updatedAt time.Time

	err := r.db.QueryRow(ctx, query,
		req.TenantID,
		string(req.FromEntityType),
		req.FromEntityID,
		string(req.ToEntityType),
		req.ToEntityID,
		string(req.Type),
		req.Subtype,
		req.Confidence,
		req.UserConfirmed,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		// Check for duplicate key constraint violation
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, fmt.Errorf("relationship already exists")
		}
		return nil, fmt.Errorf("insert relationship: %w", err)
	}

	// Return the created relationship
	return &Relationship{
		ID:         fmt.Sprintf("rel-%s-%d", req.Type, id),
		SourceID:   req.FromEntityID,
		SourceType: req.FromEntityType,
		TargetID:   req.ToEntityID,
		TargetType: req.ToEntityType,
		Type:       req.Type,
		Status:     RelationshipStatusConfirmed,
		Confidence: req.Confidence,
		TenantID:   req.TenantID,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

// MergeEntities merges two entities into one.
func (r *Repository) MergeEntities(ctx context.Context, req MergeEntitiesRequest) (*MergeEntitiesResult, error) {
	// Parse entity IDs
	// Format: ent-person-{id}
	primaryParts := strings.Split(req.PrimaryEntityID, "-")
	mergedParts := strings.Split(req.MergedEntityID, "-")

	if len(primaryParts) < 3 || len(mergedParts) < 3 {
		return nil, fmt.Errorf("invalid entity ID format")
	}

	if primaryParts[1] != "person" || mergedParts[1] != "person" {
		return nil, fmt.Errorf("only person entities can be merged currently")
	}

	primaryID := primaryParts[2]
	mergedID := mergedParts[2]

	// Start transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Transfer team memberships
	result, err := tx.Exec(ctx, `
		UPDATE team_members SET person_id = $1 WHERE person_id = $2
		AND team_id NOT IN (SELECT team_id FROM team_members WHERE person_id = $1)
	`, primaryID, mergedID)
	if err != nil {
		return nil, fmt.Errorf("transfer team memberships: %w", err)
	}
	transferred := result.RowsAffected()

	// Transfer project memberships
	result, err = tx.Exec(ctx, `
		UPDATE project_members SET person_id = $1 WHERE person_id = $2
		AND project_id NOT IN (SELECT project_id FROM project_members WHERE person_id = $1)
	`, primaryID, mergedID)
	if err != nil {
		return nil, fmt.Errorf("transfer project memberships: %w", err)
	}
	transferred += result.RowsAffected()

	// Transfer aliases
	result, err = tx.Exec(ctx, `
		UPDATE person_aliases SET person_id = $1 WHERE person_id = $2
	`, primaryID, mergedID)
	if err != nil {
		return nil, fmt.Errorf("transfer aliases: %w", err)
	}

	// Delete merged entity's remaining memberships (duplicates)
	_, err = tx.Exec(ctx, `DELETE FROM team_members WHERE person_id = $1`, mergedID)
	if err != nil {
		return nil, fmt.Errorf("cleanup team memberships: %w", err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM project_members WHERE person_id = $1`, mergedID)
	if err != nil {
		return nil, fmt.Errorf("cleanup project memberships: %w", err)
	}

	// Mark merged entity as archived (or delete)
	_, err = tx.Exec(ctx, `
		UPDATE people SET
			canonical_name = canonical_name || ' (merged into ' || $1 || ')',
			needs_review = false
		WHERE id = $2 AND tenant_id = $3
	`, primaryID, mergedID, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("mark merged entity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Get updated primary entity
	primary, err := r.GetEntity(ctx, req.TenantID, req.PrimaryEntityID)
	if err != nil {
		return nil, fmt.Errorf("get updated entity: %w", err)
	}

	return &MergeEntitiesResult{
		PrimaryEntity:           primary,
		RelationshipsTransferred: int(transferred),
	}, nil
}

// FindDuplicateEntities finds pairs of entities that are likely duplicates.
// It blocks by name prefix and email domain for efficiency, then scores candidates
// using EntitySimilarity. Returns pairs above minSimilarity threshold with signals.
// Default limit is 100 pairs.
func (r *Repository) FindDuplicateEntities(ctx context.Context, tenantID string, minSimilarity float64) ([]DuplicatePair, error) {
	// Import entities package for similarity calculation
	// (Already imported as entities in this file would need to be added if not present)

	// Get all people entities for the tenant
	filter := ListEntitiesFilter{
		TenantID:   tenantID,
		EntityType: EntityTypePerson,
		Limit:      1000, // Get candidates for comparison
	}

	entities, _, err := r.ListEntities(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list entities for duplicate detection: %w", err)
	}

	var duplicatePairs []DuplicatePair

	// Compare each pair of entities (blocking by name prefix for efficiency)
	// Group entities by first letter of name for blocking
	blocked := make(map[string][]Entity)
	for _, ent := range entities {
		if ent.Name == "" {
			continue
		}
		key := strings.ToLower(string(ent.Name[0]))
		blocked[key] = append(blocked[key], ent)
	}

	// Also block by email domain if available
	domainBlocked := make(map[string][]Entity)
	for _, ent := range entities {
		if email, ok := ent.Metadata["email"]; ok && email != "" {
			parts := strings.Split(email, "@")
			if len(parts) == 2 {
				domain := strings.ToLower(parts[1])
				domainBlocked[domain] = append(domainBlocked[domain], ent)
			}
		}
	}

	// Compare entities within the same blocks
	compared := make(map[string]bool) // Track compared pairs to avoid duplicates

	// Compare within name blocks
	for _, block := range blocked {
		for i := 0; i < len(block); i++ {
			for j := i + 1; j < len(block); j++ {
				e1, e2 := block[i], block[j]
				pairKey := fmt.Sprintf("%s:%s", e1.ID, e2.ID)
				if compared[pairKey] {
					continue
				}
				compared[pairKey] = true

				// Calculate similarity
				similarity, signals := r.calculateEntitySimilarity(e1, e2)

				if similarity >= minSimilarity {
					duplicatePairs = append(duplicatePairs, DuplicatePair{
						EntityID1:   e1.ID,
						EntityID2:   e2.ID,
						EntityName1: e1.Name,
						EntityName2: e2.Name,
						Similarity:  similarity,
						Signals:     signals,
					})
				}
			}
		}
	}

	// Compare within domain blocks (additional comparisons)
	for _, block := range domainBlocked {
		for i := 0; i < len(block); i++ {
			for j := i + 1; j < len(block); j++ {
				e1, e2 := block[i], block[j]
				pairKey := fmt.Sprintf("%s:%s", e1.ID, e2.ID)
				if compared[pairKey] {
					continue
				}
				compared[pairKey] = true

				// Calculate similarity
				similarity, signals := r.calculateEntitySimilarity(e1, e2)

				if similarity >= minSimilarity {
					duplicatePairs = append(duplicatePairs, DuplicatePair{
						EntityID1:   e1.ID,
						EntityID2:   e2.ID,
						EntityName1: e1.Name,
						EntityName2: e2.Name,
						Similarity:  similarity,
						Signals:     signals,
					})
				}
			}
		}
	}

	// Limit results
	if len(duplicatePairs) > 100 {
		duplicatePairs = duplicatePairs[:100]
	}

	return duplicatePairs, nil
}

// calculateEntitySimilarity is a helper that converts Entity to EntityComparisonData
// and calls the similarity function from the entities package.
// It returns the similarity score and a list of signals explaining why they match.
func (r *Repository) calculateEntitySimilarity(e1, e2 Entity) (float64, []string) {
	// Extract email from metadata
	email1 := e1.Metadata["email"]
	email2 := e2.Metadata["email"]

	// Extract domain from emails
	domain1 := entities.ExtractDomain(email1)
	domain2 := entities.ExtractDomain(email2)

	// Build source IDs from aliases (if available)
	// Note: Entity struct has Aliases []string, which we can use as source IDs
	sourceIDs1 := e1.Aliases
	sourceIDs2 := e2.Aliases

	// Build EntityComparisonData for both entities
	data1 := &entities.EntityComparisonData{
		Name:      e1.Name,
		Email:     email1,
		Domain:    domain1,
		SourceIDs: sourceIDs1,
	}

	data2 := &entities.EntityComparisonData{
		Name:      e2.Name,
		Email:     email2,
		Domain:    domain2,
		SourceIDs: sourceIDs2,
	}

	// Call the correct similarity function
	score := entities.EntitySimilarity(data1, data2)

	// Build signals based on what matched
	signals := []string{}

	// Name similarity check
	nameSim := entities.NameSimilarity(e1.Name, e2.Name)
	if nameSim >= 0.85 {
		signals = append(signals, "name_match")
	}

	// Domain match check
	if domain1 != "" && domain2 != "" && domain1 == domain2 {
		signals = append(signals, "domain_match")
	}

	// Shared sources check
	if len(sourceIDs1) > 0 && len(sourceIDs2) > 0 {
		// Check if they share any source IDs
		seen := make(map[string]bool)
		for _, id := range sourceIDs1 {
			seen[id] = true
		}
		for _, id := range sourceIDs2 {
			if seen[id] {
				signals = append(signals, "shared_sources")
				break
			}
		}
	}

	return score, signals
}


// GetMergePreview shows what would happen if two entities were merged, without actually merging them.
// It returns a preview of the merged entity, transferring aliases, relationships, and any conflicts.
func (r *Repository) GetMergePreview(ctx context.Context, tenantID string, entityID1, entityID2 string) (*MergePreview, error) {
	// Get both entities
	entity1, err := r.GetEntity(ctx, tenantID, entityID1)
	if err != nil {
		return nil, fmt.Errorf("get entity1: %w", err)
	}
	if entity1 == nil {
		return nil, fmt.Errorf("entity1 not found: %s", entityID1)
	}

	entity2, err := r.GetEntity(ctx, tenantID, entityID2)
	if err != nil {
		return nil, fmt.Errorf("get entity2: %w", err)
	}
	if entity2 == nil {
		return nil, fmt.Errorf("entity2 not found: %s", entityID2)
	}

	// Only support person entities for now
	if entity1.Type != EntityTypePerson || entity2.Type != EntityTypePerson {
		return nil, fmt.Errorf("only person entities can be merged")
	}

	// Parse entity IDs to get database IDs
	parts2 := strings.Split(entityID2, "-")
	if len(parts2) < 3 {
		return nil, fmt.Errorf("invalid entity ID format")
	}

	dbID2 := parts2[2]

	// Get aliases that would be transferred from entity2 to entity1
	var transferringAliases []string
	query := `
		SELECT alias_type, alias_value
		FROM person_aliases
		WHERE person_id = $1
	`
	rows, err := r.db.Query(ctx, query, dbID2)
	if err != nil {
		return nil, fmt.Errorf("query aliases: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var aliasType, aliasValue string
		if err := rows.Scan(&aliasType, &aliasValue); err != nil {
			continue
		}
		transferringAliases = append(transferringAliases, fmt.Sprintf("%s: %s", aliasType, aliasValue))
	}

	// Get relationships that would be transferred
	var transferringRels []Relationship

	// Team memberships
	teamQuery := `
		SELECT t.name, tm.role
		FROM team_members tm
		JOIN teams t ON tm.team_id = t.id
		WHERE tm.person_id = $1
	`
	rows, err = r.db.Query(ctx, teamQuery, dbID2)
	if err != nil {
		return nil, fmt.Errorf("query team memberships: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var teamName string
		var role *string
		if err := rows.Scan(&teamName, &role); err != nil {
			continue
		}

		roleStr := "member"
		if role != nil {
			roleStr = *role
		}

		transferringRels = append(transferringRels, Relationship{
			SourceName: entity2.Name,
			TargetName: teamName,
			Type:       RelationshipTypeMemberOf,
			Notes:      fmt.Sprintf("Role: %s", roleStr),
		})
	}

	// Project memberships
	projQuery := `
		SELECT p.name, pm.role
		FROM project_members pm
		JOIN projects p ON pm.project_id = p.id
		WHERE pm.person_id = $1
	`
	rows, err = r.db.Query(ctx, projQuery, dbID2)
	if err != nil {
		return nil, fmt.Errorf("query project memberships: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var projectName string
		var role *string
		if err := rows.Scan(&projectName, &role); err != nil {
			continue
		}

		roleStr := "contributor"
		if role != nil {
			roleStr = *role
		}

		transferringRels = append(transferringRels, Relationship{
			SourceName: entity2.Name,
			TargetName: projectName,
			Type:       RelationshipTypeWorksOn,
			Notes:      fmt.Sprintf("Role: %s", roleStr),
		})
	}

	// Create merged entity (preview only)
	mergedEntity := *entity1 // Copy entity1

	// Combine metadata
	if mergedEntity.Metadata == nil {
		mergedEntity.Metadata = make(map[string]string)
	}

	// Detect conflicts
	var conflictFields []string
	if entity1.Metadata["email"] != entity2.Metadata["email"] && entity2.Metadata["email"] != "" {
		conflictFields = append(conflictFields, "email")
	}
	if entity1.Metadata["title"] != entity2.Metadata["title"] && entity2.Metadata["title"] != "" {
		conflictFields = append(conflictFields, "title")
	}
	if entity1.Metadata["department"] != entity2.Metadata["department"] && entity2.Metadata["department"] != "" {
		conflictFields = append(conflictFields, "department")
	}

	// Merge metadata from entity2 (entity1 takes precedence)
	for key, value := range entity2.Metadata {
		if _, exists := mergedEntity.Metadata[key]; !exists && value != "" {
			mergedEntity.Metadata[key] = value
		}
	}

	// Update counts
	mergedEntity.SourceCount = entity1.SourceCount + entity2.SourceCount
	mergedEntity.RelationCount = entity1.RelationCount + entity2.RelationCount

	return &MergePreview{
		MergedEntity:              &mergedEntity,
		TransferringAliases:       transferringAliases,
		TransferringRelationships: transferringRels,
		ConflictFields:            conflictFields,
	}, nil
}

// AutoMergeDuplicates automatically merges high-confidence duplicate entities.
// Enforces a minimum similarity threshold of 0.90 on the server side.
// If dryRun is true, returns what would be merged without actually doing it.
// Records each merge with timestamp, trigger="auto_merge", and similarity score.
func (r *Repository) AutoMergeDuplicates(ctx context.Context, tenantID string, minSimilarity float64, dryRun bool) (*AutoMergeResult, error) {
	// Enforce minimum threshold of 0.90
	if minSimilarity < 0.90 {
		return nil, fmt.Errorf("minimum similarity threshold for auto-merge is 0.90, requested: %.2f", minSimilarity)
	}

	// Find duplicate pairs
	pairs, err := r.FindDuplicateEntities(ctx, tenantID, minSimilarity)
	if err != nil {
		return nil, fmt.Errorf("find duplicate entities: %w", err)
	}

	result := &AutoMergeResult{
		MergedCount:  0,
		MergedPairs:  []DuplicatePair{},
		SkippedPairs: []DuplicatePair{},
	}

	// If dry run, just return the pairs that would be merged
	if dryRun {
		result.MergedPairs = pairs
		result.MergedCount = len(pairs)
		return result, nil
	}

	// Actually merge the entities
	for _, pair := range pairs {
		// Use existing MergeEntities method
		mergeReq := MergeEntitiesRequest{
			TenantID:        tenantID,
			PrimaryEntityID: pair.EntityID1,
			MergedEntityID:  pair.EntityID2,
		}

		_, err := r.MergeEntities(ctx, mergeReq)
		if err != nil {
			r.logger.Warn("Failed to merge duplicate pair",
				logging.F("entity1", pair.EntityID1),
				logging.F("entity2", pair.EntityID2),
				logging.F("similarity", pair.Similarity),
				logging.Err(err))
			result.SkippedPairs = append(result.SkippedPairs, pair)
			continue
		}

		// Record merge in audit log
		err = r.recordMerge(ctx, tenantID, pair.EntityID1, pair.EntityID2, pair.Similarity, "auto_merge")
		if err != nil {
			r.logger.Error("Failed to record merge audit",
				logging.F("entity1", pair.EntityID1),
				logging.F("entity2", pair.EntityID2),
				logging.Err(err))
			// Continue anyway - the merge succeeded
		}

		result.MergedPairs = append(result.MergedPairs, pair)
		result.MergedCount++
	}

	return result, nil
}

// recordMerge records a merge operation in the audit log.
func (r *Repository) recordMerge(ctx context.Context, tenantID, primaryID, mergedID string, similarity float64, trigger string) error {
	query := `
		INSERT INTO entity_merge_log (
			tenant_id,
			merge_id,
			primary_entity_id,
			merged_entity_id,
			similarity_score,
			merge_trigger,
			merged_at
		) VALUES ($1, gen_random_uuid(), $2, $3, $4, $5, NOW())
	`

	_, err := r.db.Exec(ctx, query, tenantID, primaryID, mergedID, similarity, trigger)
	if err != nil {
		return fmt.Errorf("insert merge audit record: %w", err)
	}

	return nil
}
