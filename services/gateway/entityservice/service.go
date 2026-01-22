// Package entityservice implements the EntityService gRPC server for bulk entity seeding.
package entityservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

const (
	// DefaultMaxBatchSize is the default maximum number of entities per bulk request.
	DefaultMaxBatchSize = 500
)

// Service implements the EntityService gRPC server.
type Service struct {
	entityv1.UnimplementedEntityServiceServer
	entityRepo   *entities.Repository
	productRepo  *products.Repository
	logger       logging.Logger
	maxBatchSize int
}

// NewService creates a new entity service.
func NewService(entityRepo *entities.Repository, productRepo *products.Repository, logger logging.Logger) *Service {
	return &Service{
		entityRepo:   entityRepo,
		productRepo:  productRepo,
		logger:       logger,
		maxBatchSize: DefaultMaxBatchSize,
	}
}

// WithMaxBatchSize sets a custom maximum batch size.
func (s *Service) WithMaxBatchSize(size int) *Service {
	if size > 0 {
		s.maxBatchSize = size
	}
	return s
}

// BulkCreatePeople creates multiple people in a single request.
func (s *Service) BulkCreatePeople(ctx context.Context, req *entityv1.BulkCreatePeopleRequest) (*entityv1.BulkCreatePeopleResponse, error) {
	s.logger.Debug("BulkCreatePeople called",
		logging.F("tenant_id", req.TenantId),
		logging.F("count", len(req.People)),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if len(req.People) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one person is required")
	}
	if len(req.People) > s.maxBatchSize {
		return nil, status.Error(codes.ResourceExhausted,
			fmt.Sprintf("batch size %d exceeds maximum of %d; split into smaller batches", len(req.People), s.maxBatchSize))
	}

	resp := &entityv1.BulkCreatePeopleResponse{
		TotalRequested: int32(len(req.People)),
	}

	for i, input := range req.People {
		// Validate required fields
		if input.Email == "" {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "email is required",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}
		if input.Name == "" {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Email,
				Error:      "name is required",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Check for existing person by email
		existing, err := s.entityRepo.GetPersonByEmail(ctx, req.TenantId, input.Email)
		if err != nil {
			s.logger.Error("Error checking existing person", logging.Err(err))
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Email,
				Error:      "failed to check for existing person: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		if existing != nil {
			if req.SkipDuplicates {
				resp.Skipped = append(resp.Skipped, &entityv1.PersonSkipped{
					Email:      input.Email,
					Reason:     "email already exists",
					ExistingId: existing.ID,
				})
				resp.TotalSkipped++
				continue
			}
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Email,
				Error:      "person with this email already exists",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Create the person
		person := &entities.Person{
			TenantID:      req.TenantId,
			CanonicalName: input.Name,
			PrimaryEmail:  input.Email,
			Title:         input.Title,
			Department:    input.Department,
			Company:       input.Company,
			IsInternal:    input.IsInternal,
			AccountType:   entities.AccountTypePerson,
			Confidence:    0.9, // High confidence for manually seeded
			NeedsReview:   false,
			AutoCreated:   false,
		}

		if err := s.entityRepo.CreatePerson(ctx, person); err != nil {
			s.logger.Error("Error creating person", logging.Err(err))
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Email,
				Error:      "failed to create person: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Create aliases
		for _, aliasValue := range input.Aliases {
			// Determine alias type based on content
			aliasType := entities.AliasTypeName
			if strings.Contains(aliasValue, "@") {
				aliasType = entities.AliasTypeEmail
			}

			alias := &entities.PersonAlias{
				PersonID:   person.ID,
				AliasType:  aliasType,
				AliasValue: aliasValue,
				Confidence: 1.0,
				Source:     "seed",
			}
			if err := s.entityRepo.CreateAlias(ctx, alias); err != nil {
				s.logger.Warn("Failed to create alias",
					logging.F("person_id", person.ID),
					logging.F("alias", aliasValue),
					logging.Err(err),
				)
				// Don't fail the whole person creation for alias errors
			}
		}

		resp.Created = append(resp.Created, &entityv1.Person{
			Id:         person.ID,
			Name:       person.CanonicalName,
			Email:      person.PrimaryEmail,
			Company:    person.Company,
			Title:      person.Title,
			Department: person.Department,
			IsInternal: person.IsInternal,
			CreatedAt:  timestamppb.New(person.CreatedAt),
		})
		resp.TotalCreated++
	}

	s.logger.Info("BulkCreatePeople completed",
		logging.F("created", resp.TotalCreated),
		logging.F("skipped", resp.TotalSkipped),
		logging.F("errors", resp.TotalErrors),
	)

	return resp, nil
}

// BulkCreateProducts creates multiple products in a single request.
func (s *Service) BulkCreateProducts(ctx context.Context, req *entityv1.BulkCreateProductsRequest) (*entityv1.BulkCreateProductsResponse, error) {
	s.logger.Debug("BulkCreateProducts called",
		logging.F("tenant_id", req.TenantId),
		logging.F("count", len(req.Products)),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if len(req.Products) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one product is required")
	}
	if len(req.Products) > s.maxBatchSize {
		return nil, status.Error(codes.ResourceExhausted,
			fmt.Sprintf("batch size %d exceeds maximum of %d; split into smaller batches", len(req.Products), s.maxBatchSize))
	}

	resp := &entityv1.BulkCreateProductsResponse{
		TotalRequested: int32(len(req.Products)),
	}

	// First pass: create a map to track parent relationships for deferred creation
	createdProducts := make(map[string]int64) // name -> id

	for i, input := range req.Products {
		// Validate required fields
		if input.Name == "" {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: "(empty name)",
				Error:      "name is required",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Check for existing product by name
		existing, err := s.productRepo.GetProductByName(ctx, req.TenantId, input.Name)
		if err != nil && !errors.Is(err, products.ErrNotFound) {
			s.logger.Error("Error checking existing product", logging.Err(err))
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "failed to check for existing product: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		if existing != nil {
			if req.SkipDuplicates {
				resp.Skipped = append(resp.Skipped, &entityv1.ProductSkipped{
					Name:       input.Name,
					Reason:     "product already exists",
					ExistingId: existing.ID,
				})
				resp.TotalSkipped++
				createdProducts[input.Name] = existing.ID
				continue
			}
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "product with this name already exists",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Resolve parent if specified
		var parentID *int64
		if input.ParentName != "" {
			// Check if parent was created in this batch
			if pid, ok := createdProducts[input.ParentName]; ok {
				parentID = &pid
			} else {
				// Try to find existing parent
				parent, err := s.productRepo.GetProductByName(ctx, req.TenantId, input.ParentName)
				if err != nil && !errors.Is(err, products.ErrNotFound) {
					s.logger.Warn("Error finding parent product",
						logging.F("parent_name", input.ParentName),
						logging.Err(err),
					)
				}
				if parent != nil {
					parentID = &parent.ID
				} else {
					s.logger.Warn("Parent product not found, creating without parent",
						logging.F("product", input.Name),
						logging.F("parent_name", input.ParentName),
					)
				}
			}
		}

		// Determine product type
		productType := products.ProductTypeProduct
		switch strings.ToLower(input.ProductType) {
		case "sub_product", "subproduct":
			productType = products.ProductTypeSubProduct
		case "feature":
			productType = products.ProductTypeFeature
		}

		// Determine status
		productStatus := products.ProductStatusActive
		switch strings.ToLower(input.Status) {
		case "beta":
			productStatus = products.ProductStatusBeta
		case "sunset":
			productStatus = products.ProductStatusSunset
		case "deprecated":
			productStatus = products.ProductStatusDeprecated
		}

		// Create description pointer
		var description *string
		if input.Description != "" {
			description = &input.Description
		}

		// Create the product
		product := &products.Product{
			TenantID:    req.TenantId,
			Name:        input.Name,
			Description: description,
			ParentID:    parentID,
			ProductType: productType,
			Status:      productStatus,
			Keywords:    input.Keywords,
		}

		if err := s.productRepo.CreateProduct(ctx, product); err != nil {
			s.logger.Error("Error creating product", logging.Err(err))
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "failed to create product: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Track created product for parent resolution
		createdProducts[input.Name] = product.ID

		// Create aliases
		for _, alias := range input.Aliases {
			if err := s.productRepo.AddAlias(ctx, product.ID, alias); err != nil {
				s.logger.Warn("Failed to create product alias",
					logging.F("product_id", product.ID),
					logging.F("alias", alias),
					logging.Err(err),
				)
				// Don't fail the whole product creation for alias errors
			}
		}

		created := &entityv1.Product{
			Id:          product.ID,
			Name:        product.Name,
			ProductType: string(product.ProductType),
			Status:      string(product.Status),
			CreatedAt:   timestamppb.New(product.CreatedAt),
		}
		if product.Description != nil {
			created.Description = *product.Description
		}
		if product.ParentID != nil {
			created.ParentId = product.ParentID
		}

		resp.Created = append(resp.Created, created)
		resp.TotalCreated++
	}

	s.logger.Info("BulkCreateProducts completed",
		logging.F("created", resp.TotalCreated),
		logging.F("skipped", resp.TotalSkipped),
		logging.F("errors", resp.TotalErrors),
	)

	return resp, nil
}

// BulkCreateProjects creates multiple projects in a single request.
func (s *Service) BulkCreateProjects(ctx context.Context, req *entityv1.BulkCreateProjectsRequest) (*entityv1.BulkCreateProjectsResponse, error) {
	s.logger.Debug("BulkCreateProjects called",
		logging.F("tenant_id", req.TenantId),
		logging.F("count", len(req.Projects)),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if len(req.Projects) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one project is required")
	}
	if len(req.Projects) > s.maxBatchSize {
		return nil, status.Error(codes.ResourceExhausted,
			fmt.Sprintf("batch size %d exceeds maximum of %d; split into smaller batches", len(req.Projects), s.maxBatchSize))
	}

	resp := &entityv1.BulkCreateProjectsResponse{
		TotalRequested: int32(len(req.Projects)),
	}

	for i, input := range req.Projects {
		// Validate required fields
		if input.Name == "" {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: "(empty name)",
				Error:      "name is required",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Check for existing project by name
		existing, err := s.entityRepo.GetProjectByName(ctx, req.TenantId, input.Name)
		if err != nil {
			s.logger.Error("Error checking existing project", logging.Err(err))
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "failed to check for existing project: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		if existing != nil {
			if req.SkipDuplicates {
				resp.Skipped = append(resp.Skipped, &entityv1.ProjectSkipped{
					Name:       input.Name,
					Reason:     "project already exists",
					ExistingId: existing.ID,
				})
				resp.TotalSkipped++
				continue
			}
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "project with this name already exists",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		// Create the project
		project := &entities.Project{
			TenantID:     req.TenantId,
			Name:         input.Name,
			Description:  input.Description,
			Keywords:     input.Keywords,
			JiraProjects: input.JiraProjects,
		}

		if err := s.entityRepo.CreateProject(ctx, project); err != nil {
			s.logger.Error("Error creating project", logging.Err(err))
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "failed to create project: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		resp.Created = append(resp.Created, &entityv1.Project{
			Id:           project.ID,
			Name:         project.Name,
			Description:  project.Description,
			Keywords:     project.Keywords,
			JiraProjects: project.JiraProjects,
			CreatedAt:    timestamppb.New(project.CreatedAt),
		})
		resp.TotalCreated++
	}

	s.logger.Info("BulkCreateProjects completed",
		logging.F("created", resp.TotalCreated),
		logging.F("skipped", resp.TotalSkipped),
		logging.F("errors", resp.TotalErrors),
	)

	return resp, nil
}
