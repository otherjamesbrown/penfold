// Package productservice implements the ProductService gRPC server.
package productservice

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	productv1 "github.com/otherjamesbrown/penfold/api/proto/product/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

// Service implements the ProductService gRPC server.
type Service struct {
	productv1.UnimplementedProductServiceServer
	repo         *products.Repository
	entitiesRepo *entities.Repository
	logger       logging.Logger
}

// NewService creates a new product service.
func NewService(repo *products.Repository, entitiesRepo *entities.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:         repo,
		entitiesRepo: entitiesRepo,
		logger:       logger,
	}
}

// CreateProduct creates a new product.
func (s *Service) CreateProduct(ctx context.Context, req *productv1.CreateProductRequest) (*productv1.CreateProductResponse, error) {
	s.logger.Debug("CreateProduct called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Input.Name),
	)

	if req.Input == nil || req.Input.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "product name is required")
	}

	// Resolve parent if specified
	var parentID *int64
	if req.Input.Parent != "" {
		parent, err := s.repo.ResolveProduct(ctx, req.TenantId, req.Input.Parent)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "parent product not found: %s", req.Input.Parent)
		}
		parentID = &parent.ID
	}

	product := &products.Product{
		TenantID:    req.TenantId,
		Name:        req.Input.Name,
		Description: nullIfEmpty(req.Input.Description),
		ParentID:    parentID,
		ProductType: productTypeFromProto(req.Input.ProductType),
		Status:      productStatusFromProto(req.Input.Status),
		Keywords:    req.Input.Keywords,
	}

	if err := s.repo.CreateProduct(ctx, product); err != nil {
		s.logger.Error("Error creating product", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create product: %v", err)
	}

	return &productv1.CreateProductResponse{
		Product: productToProto(product),
	}, nil
}

// GetProduct retrieves a product by ID, name, or alias.
func (s *Service) GetProduct(ctx context.Context, req *productv1.GetProductRequest) (*productv1.GetProductResponse, error) {
	s.logger.Debug("GetProduct called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.Identifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.Identifier)
	}

	// Load aliases
	aliases, _ := s.repo.GetAliases(ctx, product.ID)

	// Load parent name if exists
	var parentName string
	if product.ParentID != nil {
		parent, err := s.repo.GetProductByID(ctx, *product.ParentID)
		if err == nil {
			parentName = parent.Name
		}
	}

	proto := productToProto(product)
	proto.ParentName = parentName
	for _, a := range aliases {
		proto.Aliases = append(proto.Aliases, a.Alias)
	}

	return &productv1.GetProductResponse{
		Product: proto,
	}, nil
}

// UpdateProduct updates an existing product.
func (s *Service) UpdateProduct(ctx context.Context, req *productv1.UpdateProductRequest) (*productv1.UpdateProductResponse, error) {
	s.logger.Debug("UpdateProduct called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "product id is required")
	}

	// Get existing product
	product, err := s.repo.GetProductByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %d", req.Id)
	}

	// Apply updates
	if req.Input != nil {
		if req.Input.Name != "" {
			product.Name = req.Input.Name
		}
		if req.Input.Description != "" {
			product.Description = &req.Input.Description
		}
		if req.Input.ProductType != productv1.ProductType_PRODUCT_TYPE_UNSPECIFIED {
			product.ProductType = productTypeFromProto(req.Input.ProductType)
		}
		if req.Input.Status != productv1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED {
			product.Status = productStatusFromProto(req.Input.Status)
		}
		if len(req.Input.Keywords) > 0 {
			product.Keywords = req.Input.Keywords
		}
		if req.Input.Parent != "" {
			parent, err := s.repo.ResolveProduct(ctx, product.TenantID, req.Input.Parent)
			if err != nil {
				return nil, status.Errorf(codes.NotFound, "parent product not found: %s", req.Input.Parent)
			}
			product.ParentID = &parent.ID
		}
	}

	if err := s.repo.UpdateProduct(ctx, product); err != nil {
		s.logger.Error("Error updating product", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update product: %v", err)
	}

	return &productv1.UpdateProductResponse{
		Product: productToProto(product),
	}, nil
}

// DeleteProduct deletes a product by ID.
func (s *Service) DeleteProduct(ctx context.Context, req *productv1.DeleteProductRequest) (*productv1.DeleteProductResponse, error) {
	s.logger.Debug("DeleteProduct called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "product id is required")
	}

	if err := s.repo.DeleteProduct(ctx, req.Id); err != nil {
		s.logger.Error("Error deleting product", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete product: %v", err)
	}

	return &productv1.DeleteProductResponse{
		Success: true,
	}, nil
}

// ListProducts lists products with optional filtering.
func (s *Service) ListProducts(ctx context.Context, req *productv1.ListProductsRequest) (*productv1.ListProductsResponse, error) {
	s.logger.Debug("ListProducts called",
		logging.F("tenant_id", req.Filter.TenantId),
	)

	filter := products.ProductFilter{
		TenantID: req.Filter.TenantId,
		Limit:    int(req.Filter.Limit),
		Offset:   int(req.Filter.Offset),
	}

	if req.Filter.NameSearch != "" {
		filter.NameSearch = req.Filter.NameSearch
	}

	if req.Filter.ProductType != productv1.ProductType_PRODUCT_TYPE_UNSPECIFIED {
		pt := productTypeFromProto(req.Filter.ProductType)
		filter.ProductType = &pt
	}

	if req.Filter.Status != productv1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED {
		ps := productStatusFromProto(req.Filter.Status)
		filter.Status = &ps
	}

	var productsList []*products.Product
	var err error

	if req.Filter.Parent != "" {
		// List children of a specific product
		parent, err := s.repo.ResolveProduct(ctx, req.Filter.TenantId, req.Filter.Parent)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "parent product not found: %s", req.Filter.Parent)
		}
		productsList, err = s.repo.ListChildren(ctx, parent.ID)
	} else if req.Filter.IncludeAll {
		// List all products
		productsList, err = s.repo.ListProducts(ctx, filter)
	} else {
		// List top-level products only
		productsList, err = s.repo.ListTopLevelProducts(ctx, req.Filter.TenantId)
	}

	if err != nil {
		s.logger.Error("Error listing products", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list products: %v", err)
	}

	protoProducts := make([]*productv1.Product, len(productsList))
	for i, p := range productsList {
		protoProducts[i] = productToProto(p)
	}

	return &productv1.ListProductsResponse{
		Products:   protoProducts,
		TotalCount: int64(len(productsList)),
	}, nil
}

// GetHierarchy retrieves the product hierarchy tree.
func (s *Service) GetHierarchy(ctx context.Context, req *productv1.GetHierarchyRequest) (*productv1.GetHierarchyResponse, error) {
	s.logger.Debug("GetHierarchy called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.Identifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.Identifier)
	}

	hierarchy, err := s.repo.GetHierarchy(ctx, product.ID)
	if err != nil {
		s.logger.Error("Error getting hierarchy", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get hierarchy: %v", err)
	}

	protoHierarchy := make([]*productv1.ProductWithHierarchy, len(hierarchy))
	for i, h := range hierarchy {
		protoHierarchy[i] = &productv1.ProductWithHierarchy{
			Product: productToProto(h.Product),
			Depth:   int32(h.Depth),
			Path:    h.Path,
		}
	}

	return &productv1.GetHierarchyResponse{
		Hierarchy: protoHierarchy,
	}, nil
}

// AddAlias adds an alias to a product.
func (s *Service) AddAlias(ctx context.Context, req *productv1.AddAliasRequest) (*productv1.AliasResponse, error) {
	s.logger.Debug("AddAlias called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
		logging.F("alias", req.Alias),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}
	if req.Alias == "" {
		return nil, status.Error(codes.InvalidArgument, "alias is required")
	}

	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.Identifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.Identifier)
	}

	if err := s.repo.AddAlias(ctx, product.ID, req.Alias); err != nil {
		s.logger.Error("Error adding alias", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to add alias: %v", err)
	}

	return &productv1.AliasResponse{
		Success:     true,
		ProductName: product.Name,
		Alias:       req.Alias,
	}, nil
}

// RemoveAlias removes an alias from a product.
func (s *Service) RemoveAlias(ctx context.Context, req *productv1.RemoveAliasRequest) (*productv1.AliasResponse, error) {
	s.logger.Debug("RemoveAlias called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
		logging.F("alias", req.Alias),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}
	if req.Alias == "" {
		return nil, status.Error(codes.InvalidArgument, "alias is required")
	}

	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.Identifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.Identifier)
	}

	if err := s.repo.RemoveAlias(ctx, product.ID, req.Alias); err != nil {
		s.logger.Error("Error removing alias", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to remove alias: %v", err)
	}

	return &productv1.AliasResponse{
		Success:     true,
		ProductName: product.Name,
		Alias:       req.Alias,
	}, nil
}

// ListAliases lists all aliases for a product.
func (s *Service) ListAliases(ctx context.Context, req *productv1.ListAliasesRequest) (*productv1.ListAliasesResponse, error) {
	s.logger.Debug("ListAliases called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.Identifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.Identifier)
	}

	aliases, err := s.repo.GetAliases(ctx, product.ID)
	if err != nil {
		s.logger.Error("Error listing aliases", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list aliases: %v", err)
	}

	protoAliases := make([]*productv1.ProductAlias, len(aliases))
	for i, a := range aliases {
		protoAliases[i] = &productv1.ProductAlias{
			Id:        a.ID,
			ProductId: a.ProductID,
			Alias:     a.Alias,
			CreatedAt: timestamppb.New(a.CreatedAt),
		}
	}

	return &productv1.ListAliasesResponse{
		ProductName: product.Name,
		Aliases:     protoAliases,
	}, nil
}

// ==================== Event Methods ====================

// ListProductEvents lists events for a product with optional filtering.
func (s *Service) ListProductEvents(ctx context.Context, req *productv1.ListProductEventsRequest) (*productv1.ListProductEventsResponse, error) {
	s.logger.Debug("ListProductEvents called",
		logging.F("tenant_id", req.Filter.TenantId),
	)

	if req.Filter == nil || req.Filter.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required in filter")
	}

	filter := eventFilterFromProto(req.Filter)

	events, err := s.repo.ListEvents(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing events", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list events: %v", err)
	}

	protoEvents := make([]*productv1.ProductEvent, len(events))
	for i, e := range events {
		protoEvents[i] = eventToProto(e)
	}

	return &productv1.ListProductEventsResponse{
		Events:     protoEvents,
		TotalCount: int64(len(events)),
	}, nil
}

// CreateProductEvent creates a new timeline event for a product.
func (s *Service) CreateProductEvent(ctx context.Context, req *productv1.CreateProductEventRequest) (*productv1.CreateProductEventResponse, error) {
	s.logger.Debug("CreateProductEvent called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_identifier", req.ProductIdentifier),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.ProductIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "product_identifier is required")
	}
	if req.Input == nil {
		return nil, status.Error(codes.InvalidArgument, "input is required")
	}
	if req.Input.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	if req.Input.OccurredAt == nil {
		return nil, status.Error(codes.InvalidArgument, "occurred_at is required")
	}

	// Resolve product
	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.ProductIdentifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.ProductIdentifier)
	}

	// Parse metadata if provided
	var metadata map[string]any
	if req.Input.MetadataJson != "" {
		if err := json.Unmarshal([]byte(req.Input.MetadataJson), &metadata); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid metadata JSON: %v", err)
		}
	}

	event := &products.ProductEvent{
		TenantID:    req.TenantId,
		ProductID:   product.ID,
		EventType:   eventTypeFromProto(req.Input.EventType),
		Visibility:  eventVisibilityFromProto(req.Input.Visibility),
		SourceType:  eventSourceTypeFromProto(req.Input.SourceType),
		Title:       req.Input.Title,
		Description: req.Input.Description,
		OccurredAt:  req.Input.OccurredAt.AsTime(),
		RecordedBy:  req.Input.RecordedBy,
		Metadata:    metadata,
	}

	if err := s.repo.CreateEvent(ctx, event); err != nil {
		s.logger.Error("Error creating event", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create event: %v", err)
	}

	event.ProductName = product.Name

	return &productv1.CreateProductEventResponse{
		Event: eventToProto(event),
	}, nil
}

// GetProductEvent retrieves a specific event by ID or UUID.
func (s *Service) GetProductEvent(ctx context.Context, req *productv1.GetProductEventRequest) (*productv1.GetProductEventResponse, error) {
	s.logger.Debug("GetProductEvent called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	var event *products.ProductEvent
	var err error

	// Try as numeric ID first
	if id, parseErr := strconv.ParseInt(req.Identifier, 10, 64); parseErr == nil {
		event, err = s.repo.GetEvent(ctx, id)
	} else {
		// Try as UUID
		eventUUID, parseErr := uuid.Parse(req.Identifier)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid identifier: must be numeric ID or UUID")
		}
		event, err = s.repo.GetEventByUUID(ctx, eventUUID)
	}

	if err != nil {
		return nil, status.Errorf(codes.NotFound, "event not found: %s", req.Identifier)
	}

	// Verify tenant
	if req.TenantId != "" && event.TenantID != req.TenantId {
		return nil, status.Error(codes.NotFound, "event not found")
	}

	// Load links
	links, _ := s.repo.GetEventLinks(ctx, event.ID)
	event.Links = links

	return &productv1.GetProductEventResponse{
		Event: eventToProto(event),
	}, nil
}

// DeleteProductEvent deletes an event by ID or UUID.
func (s *Service) DeleteProductEvent(ctx context.Context, req *productv1.DeleteProductEventRequest) (*productv1.DeleteProductEventResponse, error) {
	s.logger.Debug("DeleteProductEvent called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	var eventID int64

	// Try as numeric ID first
	if id, parseErr := strconv.ParseInt(req.Identifier, 10, 64); parseErr == nil {
		eventID = id
	} else {
		// Try as UUID - need to resolve to ID
		eventUUID, parseErr := uuid.Parse(req.Identifier)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid identifier: must be numeric ID or UUID")
		}
		event, err := s.repo.GetEventByUUID(ctx, eventUUID)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "event not found: %s", req.Identifier)
		}
		// Verify tenant
		if req.TenantId != "" && event.TenantID != req.TenantId {
			return nil, status.Error(codes.NotFound, "event not found")
		}
		eventID = event.ID
	}

	if err := s.repo.DeleteEvent(ctx, eventID); err != nil {
		s.logger.Error("Error deleting event", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete event: %v", err)
	}

	return &productv1.DeleteProductEventResponse{
		Success: true,
	}, nil
}

// LinkProductEvent links an event to another entity.
func (s *Service) LinkProductEvent(ctx context.Context, req *productv1.LinkProductEventRequest) (*productv1.LinkProductEventResponse, error) {
	s.logger.Debug("LinkProductEvent called",
		logging.F("tenant_id", req.TenantId),
		logging.F("event_identifier", req.EventIdentifier),
		logging.F("linked_entity_type", req.LinkedEntityType),
		logging.F("linked_entity_id", req.LinkedEntityId),
	)

	if req.EventIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "event_identifier is required")
	}
	if req.LinkedEntityType == "" {
		return nil, status.Error(codes.InvalidArgument, "linked_entity_type is required")
	}
	if req.LinkedEntityId == 0 {
		return nil, status.Error(codes.InvalidArgument, "linked_entity_id is required")
	}

	// Resolve event ID
	var eventID int64
	if id, parseErr := strconv.ParseInt(req.EventIdentifier, 10, 64); parseErr == nil {
		eventID = id
	} else {
		eventUUID, parseErr := uuid.Parse(req.EventIdentifier)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid event_identifier: must be numeric ID or UUID")
		}
		event, err := s.repo.GetEventByUUID(ctx, eventUUID)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "event not found: %s", req.EventIdentifier)
		}
		if req.TenantId != "" && event.TenantID != req.TenantId {
			return nil, status.Error(codes.NotFound, "event not found")
		}
		eventID = event.ID
	}

	link := &products.ProductEventLink{
		EventID:          eventID,
		LinkedEntityType: req.LinkedEntityType,
		LinkedEntityID:   req.LinkedEntityId,
		LinkType:         linkTypeFromProto(req.LinkType),
	}

	if err := s.repo.LinkEventToSource(ctx, link); err != nil {
		s.logger.Error("Error linking event", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to link event: %v", err)
	}

	return &productv1.LinkProductEventResponse{
		Link: eventLinkToProto(link),
	}, nil
}

// GetEventContext retrieves events around a specific point in time.
func (s *Service) GetEventContext(ctx context.Context, req *productv1.GetEventContextRequest) (*productv1.GetEventContextResponse, error) {
	s.logger.Debug("GetEventContext called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_identifier", req.ProductIdentifier),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.ProductIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "product_identifier is required")
	}

	// Resolve product
	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.ProductIdentifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.ProductIdentifier)
	}

	// Determine center time
	var centerTime time.Time
	if req.EventIdentifier != "" {
		// Use the event's time as center
		var event *products.ProductEvent
		if id, parseErr := strconv.ParseInt(req.EventIdentifier, 10, 64); parseErr == nil {
			event, err = s.repo.GetEvent(ctx, id)
		} else {
			eventUUID, parseErr := uuid.Parse(req.EventIdentifier)
			if parseErr != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid event_identifier: must be numeric ID or UUID")
			}
			event, err = s.repo.GetEventByUUID(ctx, eventUUID)
		}
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "event not found: %s", req.EventIdentifier)
		}
		centerTime = event.OccurredAt
	} else if req.CenterTime != nil {
		centerTime = req.CenterTime.AsTime()
	} else {
		return nil, status.Error(codes.InvalidArgument, "either center_time or event_identifier is required")
	}

	// Calculate window days based on requested event counts
	// Default to 30 days if not specified
	windowDays := 30
	if req.EventsBeforeCount > 0 || req.EventsAfterCount > 0 {
		// Estimate window size - this is a simplification
		// A more sophisticated approach would use adaptive windowing
		windowDays = 90
	}

	window, err := s.repo.GetContextWindow(ctx, product.ID, centerTime, windowDays)
	if err != nil {
		s.logger.Error("Error getting context window", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get context window: %v", err)
	}

	// Apply limits if specified
	if req.EventsBeforeCount > 0 && len(window.EventsBefore) > int(req.EventsBeforeCount) {
		// Keep the events closest to center time (last N)
		start := len(window.EventsBefore) - int(req.EventsBeforeCount)
		window.EventsBefore = window.EventsBefore[start:]
	}
	if req.EventsAfterCount > 0 && len(window.EventsAfter) > int(req.EventsAfterCount) {
		// Keep the events closest to center time (first N)
		window.EventsAfter = window.EventsAfter[:req.EventsAfterCount]
	}

	return &productv1.GetEventContextResponse{
		Context: contextWindowToProto(window),
	}, nil
}

// Conversion helpers

func productToProto(p *products.Product) *productv1.Product {
	if p == nil {
		return nil
	}

	proto := &productv1.Product{
		Id:          p.ID,
		TenantId:    p.TenantID,
		Name:        p.Name,
		ProductType: productTypeToProto(p.ProductType),
		Status:      productStatusToProto(p.Status),
		Keywords:    p.Keywords,
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}

	if p.Description != nil {
		proto.Description = *p.Description
	}
	if p.ParentID != nil {
		proto.ParentId = p.ParentID
	}

	return proto
}

func productTypeToProto(pt products.ProductType) productv1.ProductType {
	switch pt {
	case products.ProductTypeProduct:
		return productv1.ProductType_PRODUCT_TYPE_PRODUCT
	case products.ProductTypeSubProduct:
		return productv1.ProductType_PRODUCT_TYPE_SUB_PRODUCT
	case products.ProductTypeFeature:
		return productv1.ProductType_PRODUCT_TYPE_FEATURE
	default:
		return productv1.ProductType_PRODUCT_TYPE_UNSPECIFIED
	}
}

func productTypeFromProto(pt productv1.ProductType) products.ProductType {
	switch pt {
	case productv1.ProductType_PRODUCT_TYPE_PRODUCT:
		return products.ProductTypeProduct
	case productv1.ProductType_PRODUCT_TYPE_SUB_PRODUCT:
		return products.ProductTypeSubProduct
	case productv1.ProductType_PRODUCT_TYPE_FEATURE:
		return products.ProductTypeFeature
	default:
		return products.ProductTypeProduct
	}
}

func productStatusToProto(ps products.ProductStatus) productv1.ProductStatus {
	switch ps {
	case products.ProductStatusActive:
		return productv1.ProductStatus_PRODUCT_STATUS_ACTIVE
	case products.ProductStatusBeta:
		return productv1.ProductStatus_PRODUCT_STATUS_BETA
	case products.ProductStatusSunset:
		return productv1.ProductStatus_PRODUCT_STATUS_SUNSET
	case products.ProductStatusDeprecated:
		return productv1.ProductStatus_PRODUCT_STATUS_DEPRECATED
	default:
		return productv1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED
	}
}

func productStatusFromProto(ps productv1.ProductStatus) products.ProductStatus {
	switch ps {
	case productv1.ProductStatus_PRODUCT_STATUS_ACTIVE:
		return products.ProductStatusActive
	case productv1.ProductStatus_PRODUCT_STATUS_BETA:
		return products.ProductStatusBeta
	case productv1.ProductStatus_PRODUCT_STATUS_SUNSET:
		return products.ProductStatusSunset
	case productv1.ProductStatus_PRODUCT_STATUS_DEPRECATED:
		return products.ProductStatusDeprecated
	default:
		return products.ProductStatusActive
	}
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ==================== Event Conversion Helpers ====================

func eventToProto(e *products.ProductEvent) *productv1.ProductEvent {
	if e == nil {
		return nil
	}

	proto := &productv1.ProductEvent{
		Id:          e.ID,
		EventUuid:   e.EventUUID.String(),
		TenantId:    e.TenantID,
		ProductId:   e.ProductID,
		EventType:   eventTypeToProto(e.EventType),
		Visibility:  eventVisibilityToProto(e.Visibility),
		SourceType:  eventSourceTypeToProto(e.SourceType),
		Title:       e.Title,
		Description: e.Description,
		OccurredAt:  timestamppb.New(e.OccurredAt),
		RecordedBy:  e.RecordedBy,
		CreatedAt:   timestamppb.New(e.CreatedAt),
		UpdatedAt:   timestamppb.New(e.UpdatedAt),
		ProductName: e.ProductName,
	}

	// Serialize metadata to JSON
	if e.Metadata != nil {
		metadataJSON, err := json.Marshal(e.Metadata)
		if err == nil {
			proto.MetadataJson = string(metadataJSON)
		}
	}

	// Convert links
	for _, link := range e.Links {
		proto.Links = append(proto.Links, eventLinkToProto(link))
	}

	return proto
}

func eventTypeToProto(et products.EventType) productv1.EventType {
	switch et {
	case products.EventTypeDecision:
		return productv1.EventType_EVENT_TYPE_DECISION
	case products.EventTypeMilestone:
		return productv1.EventType_EVENT_TYPE_MILESTONE
	case products.EventTypeRisk:
		return productv1.EventType_EVENT_TYPE_RISK
	case products.EventTypeRelease:
		return productv1.EventType_EVENT_TYPE_RELEASE
	case products.EventTypeCompetitor:
		return productv1.EventType_EVENT_TYPE_COMPETITOR
	case products.EventTypeOrgChange:
		return productv1.EventType_EVENT_TYPE_ORG_CHANGE
	case products.EventTypeMarket:
		return productv1.EventType_EVENT_TYPE_MARKET
	case products.EventTypeNote:
		return productv1.EventType_EVENT_TYPE_NOTE
	default:
		return productv1.EventType_EVENT_TYPE_UNSPECIFIED
	}
}

func eventTypeFromProto(et productv1.EventType) products.EventType {
	switch et {
	case productv1.EventType_EVENT_TYPE_DECISION:
		return products.EventTypeDecision
	case productv1.EventType_EVENT_TYPE_MILESTONE:
		return products.EventTypeMilestone
	case productv1.EventType_EVENT_TYPE_RISK:
		return products.EventTypeRisk
	case productv1.EventType_EVENT_TYPE_RELEASE:
		return products.EventTypeRelease
	case productv1.EventType_EVENT_TYPE_COMPETITOR:
		return products.EventTypeCompetitor
	case productv1.EventType_EVENT_TYPE_ORG_CHANGE:
		return products.EventTypeOrgChange
	case productv1.EventType_EVENT_TYPE_MARKET:
		return products.EventTypeMarket
	case productv1.EventType_EVENT_TYPE_NOTE:
		return products.EventTypeNote
	default:
		return products.EventTypeNote // Default to note for unspecified
	}
}

func eventVisibilityToProto(ev products.EventVisibility) productv1.EventVisibility {
	switch ev {
	case products.EventVisibilityInternal:
		return productv1.EventVisibility_EVENT_VISIBILITY_INTERNAL
	case products.EventVisibilityExternal:
		return productv1.EventVisibility_EVENT_VISIBILITY_EXTERNAL
	default:
		return productv1.EventVisibility_EVENT_VISIBILITY_UNSPECIFIED
	}
}

func eventVisibilityFromProto(ev productv1.EventVisibility) products.EventVisibility {
	switch ev {
	case productv1.EventVisibility_EVENT_VISIBILITY_INTERNAL:
		return products.EventVisibilityInternal
	case productv1.EventVisibility_EVENT_VISIBILITY_EXTERNAL:
		return products.EventVisibilityExternal
	default:
		return products.EventVisibilityInternal // Default to internal
	}
}

func eventSourceTypeToProto(st products.EventSourceType) productv1.EventSourceType {
	switch st {
	case products.EventSourceManual:
		return productv1.EventSourceType_EVENT_SOURCE_TYPE_MANUAL
	case products.EventSourceDerived:
		// Map derived to unspecified since proto has different options
		return productv1.EventSourceType_EVENT_SOURCE_TYPE_UNSPECIFIED
	default:
		return productv1.EventSourceType_EVENT_SOURCE_TYPE_UNSPECIFIED
	}
}

func eventSourceTypeFromProto(st productv1.EventSourceType) products.EventSourceType {
	switch st {
	case productv1.EventSourceType_EVENT_SOURCE_TYPE_MANUAL:
		return products.EventSourceManual
	case productv1.EventSourceType_EVENT_SOURCE_TYPE_EMAIL,
		productv1.EventSourceType_EVENT_SOURCE_TYPE_MEETING,
		productv1.EventSourceType_EVENT_SOURCE_TYPE_DOCUMENT:
		return products.EventSourceDerived
	default:
		return products.EventSourceManual // Default to manual
	}
}

func linkTypeToProto(lt string) productv1.LinkType {
	switch lt {
	case "source":
		return productv1.LinkType_LINK_TYPE_SOURCE
	case "reference":
		return productv1.LinkType_LINK_TYPE_REFERENCE
	case "follow_up":
		return productv1.LinkType_LINK_TYPE_FOLLOW_UP
	default:
		return productv1.LinkType_LINK_TYPE_UNSPECIFIED
	}
}

func linkTypeFromProto(lt productv1.LinkType) string {
	switch lt {
	case productv1.LinkType_LINK_TYPE_SOURCE:
		return "source"
	case productv1.LinkType_LINK_TYPE_REFERENCE:
		return "reference"
	case productv1.LinkType_LINK_TYPE_FOLLOW_UP:
		return "follow_up"
	default:
		return "reference" // Default to reference
	}
}

func eventLinkToProto(link *products.ProductEventLink) *productv1.ProductEventLink {
	if link == nil {
		return nil
	}
	return &productv1.ProductEventLink{
		Id:               link.ID,
		EventId:          link.EventID,
		LinkedEntityType: link.LinkedEntityType,
		LinkedEntityId:   link.LinkedEntityID,
		LinkType:         linkTypeToProto(link.LinkType),
		CreatedAt:        timestamppb.New(link.CreatedAt),
	}
}

func eventFilterFromProto(f *productv1.EventFilter) products.EventFilter {
	filter := products.EventFilter{
		TenantID: f.TenantId,
		Limit:    int(f.Limit),
		Offset:   int(f.Offset),
	}

	if f.ProductId != nil {
		filter.ProductID = f.ProductId
	}

	if len(f.EventTypes) > 0 {
		filter.EventTypes = make([]products.EventType, len(f.EventTypes))
		for i, et := range f.EventTypes {
			filter.EventTypes[i] = eventTypeFromProto(et)
		}
	}

	if f.Visibility != productv1.EventVisibility_EVENT_VISIBILITY_UNSPECIFIED {
		vis := eventVisibilityFromProto(f.Visibility)
		filter.Visibility = &vis
	}

	if f.Since != nil {
		t := f.Since.AsTime()
		filter.Since = &t
	}

	if f.Until != nil {
		t := f.Until.AsTime()
		filter.Until = &t
	}

	return filter
}

func contextWindowToProto(w *products.ContextWindow) *productv1.ContextWindow {
	if w == nil {
		return nil
	}

	proto := &productv1.ContextWindow{
		CenterTime:  timestamppb.New(w.CenterTime),
		WindowStart: timestamppb.New(w.WindowStart),
		WindowEnd:   timestamppb.New(w.WindowEnd),
	}

	if w.CenterEvent != nil {
		proto.CenterEvent = eventToProto(w.CenterEvent)
	}

	for _, e := range w.EventsBefore {
		proto.EventsBefore = append(proto.EventsBefore, eventToProto(e))
	}

	for _, e := range w.EventsAfter {
		proto.EventsAfter = append(proto.EventsAfter, eventToProto(e))
	}

	return proto
}

// ==================== Team Management Methods ====================

// ListProductTeams lists teams associated with a product.
func (s *Service) ListProductTeams(ctx context.Context, req *productv1.ListProductTeamsRequest) (*productv1.ListProductTeamsResponse, error) {
	s.logger.Debug("ListProductTeams called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_identifier", req.ProductIdentifier),
	)

	if req.ProductIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "product_identifier is required")
	}

	// Resolve product
	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.ProductIdentifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.ProductIdentifier)
	}

	// Get teams for the product
	teams, err := s.repo.GetTeamsForProduct(ctx, product.ID)
	if err != nil {
		s.logger.Error("Error listing product teams", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list product teams: %v", err)
	}

	protoTeams := make([]*productv1.ProductTeam, len(teams))
	for i, pt := range teams {
		protoTeams[i] = productTeamToProto(pt)
	}

	return &productv1.ListProductTeamsResponse{
		Teams: protoTeams,
	}, nil
}

// AddProductTeam associates a team with a product.
func (s *Service) AddProductTeam(ctx context.Context, req *productv1.AddProductTeamRequest) (*productv1.AddProductTeamResponse, error) {
	s.logger.Debug("AddProductTeam called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_identifier", req.ProductIdentifier),
		logging.F("team_identifier", req.TeamIdentifier),
	)

	if req.ProductIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "product_identifier is required")
	}
	if req.TeamIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "team_identifier is required")
	}

	// Resolve product
	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.ProductIdentifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.ProductIdentifier)
	}

	// Resolve team - try by ID first, then by name
	var team *entities.Team
	if teamID, parseErr := strconv.ParseInt(req.TeamIdentifier, 10, 64); parseErr == nil {
		team, err = s.entitiesRepo.GetTeamByID(ctx, teamID)
	} else {
		team, err = s.entitiesRepo.GetTeamByName(ctx, req.TenantId, req.TeamIdentifier)
	}
	if err != nil {
		s.logger.Error("Error resolving team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve team: %v", err)
	}
	if team == nil {
		return nil, status.Errorf(codes.NotFound, "team not found: %s", req.TeamIdentifier)
	}

	// Create association
	pt := &products.ProductTeam{
		TenantID:  req.TenantId,
		ProductID: product.ID,
		TeamID:    team.ID,
		Context:   req.Context,
	}

	if err := s.repo.AssociateTeam(ctx, pt); err != nil {
		if err == products.ErrTeamAssociationExists {
			return nil, status.Error(codes.AlreadyExists, "team is already associated with this product")
		}
		s.logger.Error("Error associating team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to associate team: %v", err)
	}

	// Populate names for response
	pt.ProductName = product.Name
	pt.TeamName = team.Name

	return &productv1.AddProductTeamResponse{
		ProductTeam: productTeamToProto(pt),
	}, nil
}

// RemoveProductTeam removes a team association from a product.
func (s *Service) RemoveProductTeam(ctx context.Context, req *productv1.RemoveProductTeamRequest) (*productv1.RemoveProductTeamResponse, error) {
	s.logger.Debug("RemoveProductTeam called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_team_id", req.ProductTeamId),
	)

	if req.ProductTeamId == 0 {
		return nil, status.Error(codes.InvalidArgument, "product_team_id is required")
	}

	// Get the product team to find the product and team IDs
	pt, err := s.repo.GetProductTeam(ctx, req.ProductTeamId)
	if err != nil {
		if err == products.ErrNotFound {
			return nil, status.Error(codes.NotFound, "product team association not found")
		}
		s.logger.Error("Error getting product team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get product team: %v", err)
	}

	// Dissociate the team
	if err := s.repo.DissociateTeam(ctx, pt.ProductID, pt.TeamID, pt.Context); err != nil {
		if err == products.ErrNotFound {
			return nil, status.Error(codes.NotFound, "product team association not found")
		}
		s.logger.Error("Error removing product team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to remove product team: %v", err)
	}

	return &productv1.RemoveProductTeamResponse{
		Success: true,
	}, nil
}

// ListProductTeamRoles lists roles for a product team.
func (s *Service) ListProductTeamRoles(ctx context.Context, req *productv1.ListProductTeamRolesRequest) (*productv1.ListProductTeamRolesResponse, error) {
	s.logger.Debug("ListProductTeamRoles called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_team_id", req.ProductTeamId),
		logging.F("active_only", req.ActiveOnly),
	)

	if req.ProductTeamId == 0 {
		return nil, status.Error(codes.InvalidArgument, "product_team_id is required")
	}

	// Verify the product team exists
	_, err := s.repo.GetProductTeam(ctx, req.ProductTeamId)
	if err != nil {
		if err == products.ErrNotFound {
			return nil, status.Error(codes.NotFound, "product team not found")
		}
		s.logger.Error("Error getting product team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get product team: %v", err)
	}

	// Get roles for the product team
	roles, err := s.repo.GetRolesForProductTeam(ctx, req.ProductTeamId, req.ActiveOnly)
	if err != nil {
		s.logger.Error("Error listing product team roles", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list product team roles: %v", err)
	}

	protoRoles := make([]*productv1.ProductTeamRole, len(roles))
	for i, r := range roles {
		protoRoles[i] = productTeamRoleToProto(r)
	}

	return &productv1.ListProductTeamRolesResponse{
		Roles: protoRoles,
	}, nil
}

// AddProductTeamRole assigns a role to a person in a product team.
func (s *Service) AddProductTeamRole(ctx context.Context, req *productv1.AddProductTeamRoleRequest) (*productv1.AddProductTeamRoleResponse, error) {
	s.logger.Debug("AddProductTeamRole called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_team_id", req.ProductTeamId),
		logging.F("person_identifier", req.PersonIdentifier),
		logging.F("role", req.Role),
	)

	if req.ProductTeamId == 0 {
		return nil, status.Error(codes.InvalidArgument, "product_team_id is required")
	}
	if req.PersonIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "person_identifier is required")
	}
	if req.Role == "" {
		return nil, status.Error(codes.InvalidArgument, "role is required")
	}

	// Verify the product team exists
	pt, err := s.repo.GetProductTeam(ctx, req.ProductTeamId)
	if err != nil {
		if err == products.ErrNotFound {
			return nil, status.Error(codes.NotFound, "product team not found")
		}
		s.logger.Error("Error getting product team", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get product team: %v", err)
	}

	// Resolve person - try by ID first, then by email
	var person *entities.Person
	if personID, parseErr := strconv.ParseInt(req.PersonIdentifier, 10, 64); parseErr == nil {
		person, err = s.entitiesRepo.GetPersonByID(ctx, personID)
	} else {
		person, err = s.entitiesRepo.GetPersonByEmail(ctx, req.TenantId, req.PersonIdentifier)
	}
	if err != nil {
		s.logger.Error("Error resolving person", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve person: %v", err)
	}
	if person == nil {
		return nil, status.Errorf(codes.NotFound, "person not found: %s", req.PersonIdentifier)
	}

	// Create role
	role := &products.ProductTeamRole{
		TenantID:      req.TenantId,
		ProductTeamID: req.ProductTeamId,
		PersonID:      person.ID,
		Role:          req.Role,
		Scope:         req.Scope,
	}

	if req.StartedAt != nil {
		role.StartedAt = req.StartedAt.AsTime()
	}

	if err := s.repo.AddRole(ctx, role); err != nil {
		s.logger.Error("Error adding role", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to add role: %v", err)
	}

	// Populate names for response
	role.ProductName = pt.ProductName
	role.TeamName = pt.TeamName
	role.TeamContext = pt.Context
	role.PersonName = person.CanonicalName
	role.PersonEmail = person.PrimaryEmail

	return &productv1.AddProductTeamRoleResponse{
		Role: productTeamRoleToProto(role),
	}, nil
}

// EndProductTeamRole ends an active role assignment.
func (s *Service) EndProductTeamRole(ctx context.Context, req *productv1.EndProductTeamRoleRequest) (*productv1.EndProductTeamRoleResponse, error) {
	s.logger.Debug("EndProductTeamRole called",
		logging.F("tenant_id", req.TenantId),
		logging.F("role_id", req.RoleId),
	)

	if req.RoleId == 0 {
		return nil, status.Error(codes.InvalidArgument, "role_id is required")
	}

	// End the role
	if err := s.repo.EndRole(ctx, req.RoleId); err != nil {
		if err == products.ErrNotFound {
			return nil, status.Error(codes.NotFound, "role not found or already ended")
		}
		s.logger.Error("Error ending role", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to end role: %v", err)
	}

	// Fetch the updated role for response
	role, err := s.repo.GetRole(ctx, req.RoleId)
	if err != nil {
		s.logger.Error("Error fetching ended role", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch ended role: %v", err)
	}

	return &productv1.EndProductTeamRoleResponse{
		Role: productTeamRoleToProto(role),
	}, nil
}

// GetProductTeamRole retrieves a specific role assignment.
func (s *Service) GetProductTeamRole(ctx context.Context, req *productv1.GetProductTeamRoleRequest) (*productv1.GetProductTeamRoleResponse, error) {
	s.logger.Debug("GetProductTeamRole called",
		logging.F("tenant_id", req.TenantId),
		logging.F("role_id", req.RoleId),
	)

	if req.RoleId == 0 {
		return nil, status.Error(codes.InvalidArgument, "role_id is required")
	}

	role, err := s.repo.GetRole(ctx, req.RoleId)
	if err != nil {
		if err == products.ErrNotFound {
			return nil, status.Error(codes.NotFound, "role not found")
		}
		s.logger.Error("Error getting role", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get role: %v", err)
	}

	return &productv1.GetProductTeamRoleResponse{
		Role: productTeamRoleToProto(role),
	}, nil
}

// FindByRole finds people by role across products.
func (s *Service) FindByRole(ctx context.Context, req *productv1.FindByRoleRequest) (*productv1.FindByRoleResponse, error) {
	s.logger.Debug("FindByRole called",
		logging.F("tenant_id", req.TenantId),
		logging.F("role", req.Role),
		logging.F("product_identifier", req.ProductIdentifier),
		logging.F("team_identifier", req.TeamIdentifier),
		logging.F("scope", req.Scope),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Build role query
	query := products.RoleQuery{
		TenantID:   req.TenantId,
		Role:       req.Role,
		Scope:      req.Scope,
		ActiveOnly: req.ActiveOnly,
	}

	// Resolve product name if provided
	if req.ProductIdentifier != "" {
		product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.ProductIdentifier)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "product not found: %s", req.ProductIdentifier)
		}
		query.ProductName = product.Name
	}

	// Resolve team name if provided
	if req.TeamIdentifier != "" {
		var team *entities.Team
		var err error
		if teamID, parseErr := strconv.ParseInt(req.TeamIdentifier, 10, 64); parseErr == nil {
			team, err = s.entitiesRepo.GetTeamByID(ctx, teamID)
		} else {
			team, err = s.entitiesRepo.GetTeamByName(ctx, req.TenantId, req.TeamIdentifier)
		}
		if err != nil {
			s.logger.Error("Error resolving team", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to resolve team: %v", err)
		}
		if team == nil {
			return nil, status.Errorf(codes.NotFound, "team not found: %s", req.TeamIdentifier)
		}
		query.TeamName = team.Name
	}

	// Find matching roles
	roles, err := s.repo.FindByRole(ctx, query)
	if err != nil {
		s.logger.Error("Error finding by role", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to find by role: %v", err)
	}

	protoRoles := make([]*productv1.ProductTeamRole, len(roles))
	for i, r := range roles {
		protoRoles[i] = productTeamRoleToProto(r)
	}

	return &productv1.FindByRoleResponse{
		Roles: protoRoles,
	}, nil
}

// ListProductPeople lists all people associated with a product through teams.
func (s *Service) ListProductPeople(ctx context.Context, req *productv1.ListProductPeopleRequest) (*productv1.ListProductPeopleResponse, error) {
	s.logger.Debug("ListProductPeople called",
		logging.F("tenant_id", req.TenantId),
		logging.F("product_identifier", req.ProductIdentifier),
		logging.F("active_only", req.ActiveOnly),
	)

	if req.ProductIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "product_identifier is required")
	}

	// Resolve product
	product, err := s.repo.ResolveProduct(ctx, req.TenantId, req.ProductIdentifier)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %s", req.ProductIdentifier)
	}

	// Get all roles for people on this product
	roles, err := s.repo.GetPeopleOnProduct(ctx, product.ID, req.ActiveOnly)
	if err != nil {
		s.logger.Error("Error listing product people", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list product people: %v", err)
	}

	// Group roles by person to build summaries
	personRoles := make(map[int64][]*products.ProductTeamRole)
	personTeams := make(map[int64]map[string]bool)
	personInfo := make(map[int64]*products.ProductTeamRole)

	for _, r := range roles {
		personRoles[r.PersonID] = append(personRoles[r.PersonID], r)
		if personTeams[r.PersonID] == nil {
			personTeams[r.PersonID] = make(map[string]bool)
		}
		personTeams[r.PersonID][r.TeamName] = true
		personInfo[r.PersonID] = r // Store last one for person info
	}

	// Build response
	var people []*productv1.ProductPersonSummary
	for personID, roles := range personRoles {
		info := personInfo[personID]

		// Build team names list
		var teamNames []string
		for teamName := range personTeams[personID] {
			teamNames = append(teamNames, teamName)
		}

		// Convert roles to proto
		protoRoles := make([]*productv1.ProductTeamRole, len(roles))
		for i, r := range roles {
			protoRoles[i] = productTeamRoleToProto(r)
		}

		summary := &productv1.ProductPersonSummary{
			Person: &productv1.Person{
				Id:            personID,
				TenantId:      req.TenantId,
				CanonicalName: info.PersonName,
				PrimaryEmail:  info.PersonEmail,
			},
			TeamNames: teamNames,
			Roles:     protoRoles,
		}
		people = append(people, summary)
	}

	return &productv1.ListProductPeopleResponse{
		People: people,
	}, nil
}

// ==================== Team Conversion Helpers ====================

func productTeamToProto(pt *products.ProductTeam) *productv1.ProductTeam {
	if pt == nil {
		return nil
	}

	return &productv1.ProductTeam{
		Id:          pt.ID,
		TenantId:    pt.TenantID,
		ProductId:   pt.ProductID,
		TeamId:      pt.TeamID,
		Context:     pt.Context,
		TeamName:    pt.TeamName,
		ProductName: pt.ProductName,
		CreatedAt:   timestamppb.New(pt.CreatedAt),
		UpdatedAt:   timestamppb.New(pt.UpdatedAt),
	}
}

func productTeamRoleToProto(r *products.ProductTeamRole) *productv1.ProductTeamRole {
	if r == nil {
		return nil
	}

	proto := &productv1.ProductTeamRole{
		Id:            r.ID,
		TenantId:      r.TenantID,
		ProductTeamId: r.ProductTeamID,
		PersonId:      r.PersonID,
		Role:          r.Role,
		Scope:         r.Scope,
		IsActive:      r.IsActive,
		StartedAt:     timestamppb.New(r.StartedAt),
		PersonName:    r.PersonName,
		PersonEmail:   r.PersonEmail,
		TeamName:      r.TeamName,
		ProductName:   r.ProductName,
		TeamContext:   r.TeamContext,
		CreatedAt:     timestamppb.New(r.CreatedAt),
		UpdatedAt:     timestamppb.New(r.UpdatedAt),
	}

	if r.EndedAt != nil {
		proto.EndedAt = timestamppb.New(*r.EndedAt)
	}

	return proto
}

// ==================== Query Methods ====================

// QueryProducts executes a natural language query about products.
func (s *Service) QueryProducts(ctx context.Context, req *productv1.QueryProductsRequest) (*productv1.QueryProductsResponse, error) {
	s.logger.Debug("QueryProducts called",
		logging.F("tenant_id", req.TenantId),
		logging.F("query", req.Query),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	// Create a query service using the repository's pool
	querySvc := products.NewQueryService(s.repo.Pool(), s.logger)

	// Execute the query
	result, err := querySvc.Query(ctx, req.TenantId, req.Query)
	if err != nil {
		s.logger.Error("Error executing query", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}

	// Build response
	response := &productv1.QueryProductsResponse{
		Type:    queryTypeToProto(result.Type),
		Message: result.Message,
	}

	// Add type-specific results
	if len(result.Products) > 0 {
		for _, p := range result.Products {
			response.Products = append(response.Products, productToProto(p))
		}
	}

	if len(result.Persons) > 0 {
		for _, p := range result.Persons {
			response.Persons = append(response.Persons, personRoleInfoToProto(p))
		}
	}

	if len(result.Teams) > 0 {
		for _, t := range result.Teams {
			response.Teams = append(response.Teams, productTeamToProto(t))
		}
	}

	if len(result.Events) > 0 {
		for _, e := range result.Events {
			response.Events = append(response.Events, eventToProto(e))
		}
	}

	if result.Hierarchy != nil {
		response.Hierarchy = append(response.Hierarchy, &productv1.ProductWithHierarchy{
			Product: productToProto(result.Hierarchy.Product),
			Depth:   int32(result.Hierarchy.Depth),
			Path:    result.Hierarchy.Path,
		})
	}

	return response, nil
}

// ==================== Query Conversion Helpers ====================

func queryTypeToProto(qt products.QueryType) productv1.QueryType {
	switch qt {
	case products.QueryTypeSearch:
		return productv1.QueryType_QUERY_TYPE_SEARCH
	case products.QueryTypeRole:
		return productv1.QueryType_QUERY_TYPE_ROLE
	case products.QueryTypeTeam:
		return productv1.QueryType_QUERY_TYPE_TEAM
	case products.QueryTypeTimeline:
		return productv1.QueryType_QUERY_TYPE_TIMELINE
	case products.QueryTypeHierarchy:
		return productv1.QueryType_QUERY_TYPE_HIERARCHY
	default:
		return productv1.QueryType_QUERY_TYPE_UNSPECIFIED
	}
}

func personRoleInfoToProto(p *products.PersonRoleInfo) *productv1.PersonRoleInfo {
	if p == nil {
		return nil
	}
	return &productv1.PersonRoleInfo{
		PersonId:    p.PersonID,
		PersonName:  p.PersonName,
		PersonEmail: p.PersonEmail,
		Role:        p.Role,
		Scope:       p.Scope,
		TeamName:    p.TeamName,
		TeamContext: p.TeamContext,
		ProductName: p.ProductName,
	}
}
