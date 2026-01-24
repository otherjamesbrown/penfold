// Package productservice implements the ProductService gRPC server.
package productservice

import (
	"context"

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
