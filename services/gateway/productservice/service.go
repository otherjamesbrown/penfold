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
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

// Service implements the ProductService gRPC server.
type Service struct {
	productv1.UnimplementedProductServiceServer
	repo   *products.Repository
	logger logging.Logger
}

// NewService creates a new product service.
func NewService(repo *products.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
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
