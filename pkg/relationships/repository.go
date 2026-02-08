package relationships

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
			COUNT(DISTINCT tm.team_id) + COUNT(DISTINCT pm.project_id) as relation_count
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

		if err := rows.Scan(&id, &canonicalName, &primaryEmail, &jobTitle, &department,
			&confidence, &createdAt, &updatedAt, &aliasCount, &relationCount); err != nil {
			return nil, 0, fmt.Errorf("scan people entity: %w", err)
		}

		metadata := map[string]string{}
		if primaryEmail != nil && *primaryEmail != "" {
			metadata["email"] = *primaryEmail
		}
		if jobTitle != nil && *jobTitle != "" {
			metadata["title"] = *jobTitle
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
