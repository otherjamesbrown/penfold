package entityservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

// ============ Mock Interfaces ============

// EntityRepository defines the interface for entity repository operations used by the service.
type EntityRepository interface {
	GetPersonByEmail(ctx context.Context, tenantID, email string) (*entities.Person, error)
	CreatePerson(ctx context.Context, p *entities.Person) error
	CreateAlias(ctx context.Context, alias *entities.PersonAlias) error
	GetProjectByName(ctx context.Context, tenantID, name string) (*entities.Project, error)
	CreateProject(ctx context.Context, p *entities.Project) error
}

// ProductRepository defines the interface for product repository operations used by the service.
type ProductRepository interface {
	GetProductByName(ctx context.Context, tenantID, name string) (*products.Product, error)
	CreateProduct(ctx context.Context, p *products.Product) error
	AddAlias(ctx context.Context, productID int64, alias string) error
}

// ============ Mock Implementations ============

type mockEntityRepo struct {
	getPersonByEmailFn func(ctx context.Context, tenantID, email string) (*entities.Person, error)
	createPersonFn     func(ctx context.Context, p *entities.Person) error
	createAliasFn      func(ctx context.Context, alias *entities.PersonAlias) error
	getProjectByNameFn func(ctx context.Context, tenantID, name string) (*entities.Project, error)
	createProjectFn    func(ctx context.Context, p *entities.Project) error
}

func (m *mockEntityRepo) GetPersonByEmail(ctx context.Context, tenantID, email string) (*entities.Person, error) {
	if m.getPersonByEmailFn != nil {
		return m.getPersonByEmailFn(ctx, tenantID, email)
	}
	return nil, nil
}

func (m *mockEntityRepo) CreatePerson(ctx context.Context, p *entities.Person) error {
	if m.createPersonFn != nil {
		return m.createPersonFn(ctx, p)
	}
	p.ID = 1
	p.CreatedAt = time.Now()
	return nil
}

func (m *mockEntityRepo) CreateAlias(ctx context.Context, alias *entities.PersonAlias) error {
	if m.createAliasFn != nil {
		return m.createAliasFn(ctx, alias)
	}
	return nil
}

func (m *mockEntityRepo) GetProjectByName(ctx context.Context, tenantID, name string) (*entities.Project, error) {
	if m.getProjectByNameFn != nil {
		return m.getProjectByNameFn(ctx, tenantID, name)
	}
	return nil, nil
}

func (m *mockEntityRepo) CreateProject(ctx context.Context, p *entities.Project) error {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, p)
	}
	p.ID = 1
	p.CreatedAt = time.Now()
	return nil
}

type mockProductRepo struct {
	getProductByNameFn func(ctx context.Context, tenantID, name string) (*products.Product, error)
	createProductFn    func(ctx context.Context, p *products.Product) error
	addAliasFn         func(ctx context.Context, productID int64, alias string) error
}

func (m *mockProductRepo) GetProductByName(ctx context.Context, tenantID, name string) (*products.Product, error) {
	if m.getProductByNameFn != nil {
		return m.getProductByNameFn(ctx, tenantID, name)
	}
	return nil, products.ErrNotFound
}

func (m *mockProductRepo) CreateProduct(ctx context.Context, p *products.Product) error {
	if m.createProductFn != nil {
		return m.createProductFn(ctx, p)
	}
	p.ID = 1
	p.CreatedAt = time.Now()
	return nil
}

func (m *mockProductRepo) AddAlias(ctx context.Context, productID int64, alias string) error {
	if m.addAliasFn != nil {
		return m.addAliasFn(ctx, productID, alias)
	}
	return nil
}

// ============ Test Service Factory ============

// testService is a service configured with mockable repositories.
type testService struct {
	*Service
	entityRepo  *mockEntityRepo
	productRepo *mockProductRepo
}

func newTestService() *testService {
	logger := testLogger()
	entityRepo := &mockEntityRepo{}
	productRepo := &mockProductRepo{}

	return &testService{
		Service: &Service{
			entityRepo:  nil, // We'll use wrapper methods
			productRepo: nil,
			logger:      logger,
		},
		entityRepo:  entityRepo,
		productRepo: productRepo,
	}
}

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error" // Quiet logging for tests
	return logging.NewLogger(cfg)
}

// ============ BulkCreatePeople Tests ============

func TestNewService(t *testing.T) {
	logger := testLogger()

	t.Run("creates service with nil repos", func(t *testing.T) {
		svc := NewService(nil, nil, logger)
		require.NotNil(t, svc)
		assert.Nil(t, svc.entityRepo)
		assert.Nil(t, svc.productRepo)
	})

	t.Run("creates service with nil logger", func(t *testing.T) {
		svc := NewService(nil, nil, nil)
		require.NotNil(t, svc)
	})
}

func TestBulkCreatePeople_Validation(t *testing.T) {
	logger := testLogger()
	svc := NewService(nil, nil, logger)

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "",
			People: []*entityv1.PersonInput{
				{Name: "John", Email: "john@example.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("empty people list returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People:   []*entityv1.PersonInput{},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "at least one person")
	})

	t.Run("nil people list returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People:   nil,
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestBulkCreatePeople_ItemValidation(t *testing.T) {
	t.Run("missing email returns per-item error", func(t *testing.T) {
		svc := newMockableService()

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People: []*entityv1.PersonInput{
				{Name: "John Doe", Email: ""},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalRequested)
		assert.Equal(t, int32(0), resp.TotalCreated)
		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, "John Doe", resp.Errors[0].Identifier)
		assert.Contains(t, resp.Errors[0].Error, "email is required")
	})

	t.Run("missing name returns per-item error", func(t *testing.T) {
		svc := newMockableService()

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People: []*entityv1.PersonInput{
				{Name: "", Email: "john@example.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalRequested)
		assert.Equal(t, int32(0), resp.TotalCreated)
		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, "john@example.com", resp.Errors[0].Identifier)
		assert.Contains(t, resp.Errors[0].Error, "name is required")
	})
}

func TestBulkCreatePeople_DuplicateHandling(t *testing.T) {
	t.Run("duplicate email with skip_duplicates=true skips", func(t *testing.T) {
		svc := newMockableService()
		svc.mockEntityRepo.getPersonByEmailFn = func(ctx context.Context, tenantID, email string) (*entities.Person, error) {
			return &entities.Person{ID: 42, PrimaryEmail: email}, nil
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId:       "tenant-1",
			SkipDuplicates: true,
			People: []*entityv1.PersonInput{
				{Name: "John Doe", Email: "john@example.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalRequested)
		assert.Equal(t, int32(0), resp.TotalCreated)
		assert.Equal(t, int32(1), resp.TotalSkipped)
		assert.Equal(t, int32(0), resp.TotalErrors)
		require.Len(t, resp.Skipped, 1)
		assert.Equal(t, "john@example.com", resp.Skipped[0].Email)
		assert.Equal(t, int64(42), resp.Skipped[0].ExistingId)
	})

	t.Run("duplicate email with skip_duplicates=false errors", func(t *testing.T) {
		svc := newMockableService()
		svc.mockEntityRepo.getPersonByEmailFn = func(ctx context.Context, tenantID, email string) (*entities.Person, error) {
			return &entities.Person{ID: 42, PrimaryEmail: email}, nil
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId:       "tenant-1",
			SkipDuplicates: false,
			People: []*entityv1.PersonInput{
				{Name: "John Doe", Email: "john@example.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalRequested)
		assert.Equal(t, int32(0), resp.TotalCreated)
		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Error, "already exists")
	})
}

func TestBulkCreatePeople_Success(t *testing.T) {
	t.Run("creates person successfully", func(t *testing.T) {
		svc := newMockableService()
		personID := int64(0)
		svc.mockEntityRepo.createPersonFn = func(ctx context.Context, p *entities.Person) error {
			personID++
			p.ID = personID
			p.CreatedAt = time.Now()
			return nil
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People: []*entityv1.PersonInput{
				{
					Name:       "John Doe",
					Email:      "john@example.com",
					Company:    "Acme Corp",
					Title:      "Engineer",
					Department: "Engineering",
					IsInternal: true,
				},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalRequested)
		assert.Equal(t, int32(1), resp.TotalCreated)
		assert.Equal(t, int32(0), resp.TotalErrors)
		require.Len(t, resp.Created, 1)
		assert.Equal(t, "John Doe", resp.Created[0].Name)
		assert.Equal(t, "john@example.com", resp.Created[0].Email)
		assert.Equal(t, "Acme Corp", resp.Created[0].Company)
	})

	t.Run("creates aliases for person", func(t *testing.T) {
		svc := newMockableService()
		aliasesCreated := []string{}
		svc.mockEntityRepo.createAliasFn = func(ctx context.Context, alias *entities.PersonAlias) error {
			aliasesCreated = append(aliasesCreated, alias.AliasValue)
			return nil
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People: []*entityv1.PersonInput{
				{
					Name:    "John Doe",
					Email:   "john@example.com",
					Aliases: []string{"JD", "Johnny", "john.doe@alt.com"},
				},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalCreated)
		assert.Len(t, aliasesCreated, 3)
		assert.Contains(t, aliasesCreated, "JD")
		assert.Contains(t, aliasesCreated, "Johnny")
		assert.Contains(t, aliasesCreated, "john.doe@alt.com")
	})

	t.Run("multiple people with mixed results", func(t *testing.T) {
		svc := newMockableService()
		personID := int64(0)
		svc.mockEntityRepo.getPersonByEmailFn = func(ctx context.Context, tenantID, email string) (*entities.Person, error) {
			if email == "existing@example.com" {
				return &entities.Person{ID: 99, PrimaryEmail: email}, nil
			}
			return nil, nil
		}
		svc.mockEntityRepo.createPersonFn = func(ctx context.Context, p *entities.Person) error {
			personID++
			p.ID = personID
			p.CreatedAt = time.Now()
			return nil
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId:       "tenant-1",
			SkipDuplicates: true,
			People: []*entityv1.PersonInput{
				{Name: "New Person", Email: "new@example.com"},
				{Name: "Existing", Email: "existing@example.com"},
				{Name: "", Email: "invalid@example.com"}, // Missing name
				{Name: "Another New", Email: "another@example.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(4), resp.TotalRequested)
		assert.Equal(t, int32(2), resp.TotalCreated)
		assert.Equal(t, int32(1), resp.TotalSkipped)
		assert.Equal(t, int32(1), resp.TotalErrors)
	})
}

// ============ BulkCreateProducts Tests ============

func TestBulkCreateProducts_Validation(t *testing.T) {
	logger := testLogger()
	svc := NewService(nil, nil, logger)

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkCreateProductsRequest{
			TenantId: "",
			Products: []*entityv1.ProductInput{
				{Name: "Product A"},
			},
		}

		resp, err := svc.BulkCreateProducts(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("empty products list returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkCreateProductsRequest{
			TenantId: "tenant-1",
			Products: []*entityv1.ProductInput{},
		}

		resp, err := svc.BulkCreateProducts(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "at least one product")
	})
}

func TestBulkCreateProducts_ItemValidation(t *testing.T) {
	t.Run("missing name returns per-item error", func(t *testing.T) {
		svc := newMockableService()

		req := &entityv1.BulkCreateProductsRequest{
			TenantId: "tenant-1",
			Products: []*entityv1.ProductInput{
				{Name: "", Description: "Some product"},
			},
		}

		resp, err := svc.BulkCreateProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalRequested)
		assert.Equal(t, int32(0), resp.TotalCreated)
		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Error, "name is required")
	})
}

func TestBulkCreateProducts_DuplicateHandling(t *testing.T) {
	t.Run("duplicate product with skip_duplicates=true skips", func(t *testing.T) {
		svc := newMockableService()
		svc.mockProductRepo.getProductByNameFn = func(ctx context.Context, tenantID, name string) (*products.Product, error) {
			return &products.Product{ID: 42, Name: name}, nil
		}

		req := &entityv1.BulkCreateProductsRequest{
			TenantId:       "tenant-1",
			SkipDuplicates: true,
			Products: []*entityv1.ProductInput{
				{Name: "Existing Product"},
			},
		}

		resp, err := svc.BulkCreateProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalSkipped)
		require.Len(t, resp.Skipped, 1)
		assert.Equal(t, "Existing Product", resp.Skipped[0].Name)
		assert.Equal(t, int64(42), resp.Skipped[0].ExistingId)
	})

	t.Run("duplicate product with skip_duplicates=false errors", func(t *testing.T) {
		svc := newMockableService()
		svc.mockProductRepo.getProductByNameFn = func(ctx context.Context, tenantID, name string) (*products.Product, error) {
			return &products.Product{ID: 42, Name: name}, nil
		}

		req := &entityv1.BulkCreateProductsRequest{
			TenantId:       "tenant-1",
			SkipDuplicates: false,
			Products: []*entityv1.ProductInput{
				{Name: "Existing Product"},
			},
		}

		resp, err := svc.BulkCreateProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Error, "already exists")
	})
}

func TestBulkCreateProducts_ProductTypeMapping(t *testing.T) {
	tests := []struct {
		inputType    string
		expectedType products.ProductType
	}{
		{"", products.ProductTypeProduct},
		{"product", products.ProductTypeProduct},
		{"sub_product", products.ProductTypeSubProduct},
		{"subproduct", products.ProductTypeSubProduct},
		{"feature", products.ProductTypeFeature},
	}

	for _, tt := range tests {
		t.Run("type_"+tt.inputType, func(t *testing.T) {
			svc := newMockableService()
			var capturedType products.ProductType
			svc.mockProductRepo.createProductFn = func(ctx context.Context, p *products.Product) error {
				capturedType = p.ProductType
				p.ID = 1
				p.CreatedAt = time.Now()
				return nil
			}

			req := &entityv1.BulkCreateProductsRequest{
				TenantId: "tenant-1",
				Products: []*entityv1.ProductInput{
					{Name: "Test Product", ProductType: tt.inputType},
				},
			}

			resp, err := svc.BulkCreateProducts(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, int32(1), resp.TotalCreated)
			assert.Equal(t, tt.expectedType, capturedType)
		})
	}
}

func TestBulkCreateProducts_StatusMapping(t *testing.T) {
	tests := []struct {
		inputStatus    string
		expectedStatus products.ProductStatus
	}{
		{"", products.ProductStatusActive},
		{"active", products.ProductStatusActive},
		{"beta", products.ProductStatusBeta},
		{"sunset", products.ProductStatusSunset},
		{"deprecated", products.ProductStatusDeprecated},
	}

	for _, tt := range tests {
		t.Run("status_"+tt.inputStatus, func(t *testing.T) {
			svc := newMockableService()
			var capturedStatus products.ProductStatus
			svc.mockProductRepo.createProductFn = func(ctx context.Context, p *products.Product) error {
				capturedStatus = p.Status
				p.ID = 1
				p.CreatedAt = time.Now()
				return nil
			}

			req := &entityv1.BulkCreateProductsRequest{
				TenantId: "tenant-1",
				Products: []*entityv1.ProductInput{
					{Name: "Test Product", Status: tt.inputStatus},
				},
			}

			resp, err := svc.BulkCreateProducts(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, int32(1), resp.TotalCreated)
			assert.Equal(t, tt.expectedStatus, capturedStatus)
		})
	}
}

func TestBulkCreateProducts_ParentResolution(t *testing.T) {
	t.Run("resolves parent from batch", func(t *testing.T) {
		svc := newMockableService()
		productID := int64(0)
		svc.mockProductRepo.createProductFn = func(ctx context.Context, p *products.Product) error {
			productID++
			p.ID = productID
			p.CreatedAt = time.Now()
			return nil
		}

		req := &entityv1.BulkCreateProductsRequest{
			TenantId: "tenant-1",
			Products: []*entityv1.ProductInput{
				{Name: "Parent Product"},
				{Name: "Child Product", ParentName: "Parent Product"},
			},
		}

		resp, err := svc.BulkCreateProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(2), resp.TotalCreated)
		// Child should have parent ID set
		require.Len(t, resp.Created, 2)
		assert.Nil(t, resp.Created[0].ParentId) // Parent has no parent
		assert.NotNil(t, resp.Created[1].ParentId)
		assert.Equal(t, int64(1), *resp.Created[1].ParentId)
	})

	t.Run("resolves parent from database", func(t *testing.T) {
		svc := newMockableService()
		svc.mockProductRepo.getProductByNameFn = func(ctx context.Context, tenantID, name string) (*products.Product, error) {
			if name == "Existing Parent" {
				return &products.Product{ID: 100, Name: name}, nil
			}
			return nil, products.ErrNotFound
		}
		svc.mockProductRepo.createProductFn = func(ctx context.Context, p *products.Product) error {
			p.ID = 1
			p.CreatedAt = time.Now()
			return nil
		}

		req := &entityv1.BulkCreateProductsRequest{
			TenantId: "tenant-1",
			Products: []*entityv1.ProductInput{
				{Name: "Child Product", ParentName: "Existing Parent"},
			},
		}

		resp, err := svc.BulkCreateProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalCreated)
		require.Len(t, resp.Created, 1)
		assert.NotNil(t, resp.Created[0].ParentId)
		assert.Equal(t, int64(100), *resp.Created[0].ParentId)
	})
}

// ============ BulkCreateProjects Tests ============

func TestBulkCreateProjects_Validation(t *testing.T) {
	logger := testLogger()
	svc := NewService(nil, nil, logger)

	t.Run("empty tenant_id returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkCreateProjectsRequest{
			TenantId: "",
			Projects: []*entityv1.ProjectInput{
				{Name: "Project A"},
			},
		}

		resp, err := svc.BulkCreateProjects(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "tenant_id is required")
	})

	t.Run("empty projects list returns InvalidArgument", func(t *testing.T) {
		req := &entityv1.BulkCreateProjectsRequest{
			TenantId: "tenant-1",
			Projects: []*entityv1.ProjectInput{},
		}

		resp, err := svc.BulkCreateProjects(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "at least one project")
	})
}

func TestBulkCreateProjects_ItemValidation(t *testing.T) {
	t.Run("missing name returns per-item error", func(t *testing.T) {
		svc := newMockableService()

		req := &entityv1.BulkCreateProjectsRequest{
			TenantId: "tenant-1",
			Projects: []*entityv1.ProjectInput{
				{Name: "", Description: "Some project"},
			},
		}

		resp, err := svc.BulkCreateProjects(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalRequested)
		assert.Equal(t, int32(0), resp.TotalCreated)
		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Error, "name is required")
	})
}

func TestBulkCreateProjects_DuplicateHandling(t *testing.T) {
	t.Run("duplicate project with skip_duplicates=true skips", func(t *testing.T) {
		svc := newMockableService()
		svc.mockEntityRepo.getProjectByNameFn = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
			return &entities.Project{ID: 42, Name: name}, nil
		}

		req := &entityv1.BulkCreateProjectsRequest{
			TenantId:       "tenant-1",
			SkipDuplicates: true,
			Projects: []*entityv1.ProjectInput{
				{Name: "Existing Project"},
			},
		}

		resp, err := svc.BulkCreateProjects(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalSkipped)
		require.Len(t, resp.Skipped, 1)
		assert.Equal(t, "Existing Project", resp.Skipped[0].Name)
		assert.Equal(t, int64(42), resp.Skipped[0].ExistingId)
	})

	t.Run("duplicate project with skip_duplicates=false errors", func(t *testing.T) {
		svc := newMockableService()
		svc.mockEntityRepo.getProjectByNameFn = func(ctx context.Context, tenantID, name string) (*entities.Project, error) {
			return &entities.Project{ID: 42, Name: name}, nil
		}

		req := &entityv1.BulkCreateProjectsRequest{
			TenantId:       "tenant-1",
			SkipDuplicates: false,
			Projects: []*entityv1.ProjectInput{
				{Name: "Existing Project"},
			},
		}

		resp, err := svc.BulkCreateProjects(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Error, "already exists")
	})
}

func TestBulkCreateProjects_Success(t *testing.T) {
	t.Run("creates project with keywords and jira projects", func(t *testing.T) {
		svc := newMockableService()
		var capturedProject *entities.Project
		svc.mockEntityRepo.createProjectFn = func(ctx context.Context, p *entities.Project) error {
			capturedProject = p
			p.ID = 1
			p.CreatedAt = time.Now()
			return nil
		}

		req := &entityv1.BulkCreateProjectsRequest{
			TenantId: "tenant-1",
			Projects: []*entityv1.ProjectInput{
				{
					Name:         "MTC",
					Description:  "Major TikTok Contract",
					Keywords:     []string{"TikTok", "migration", "Oracle"},
					JiraProjects: []string{"MTC", "MTC-2"},
				},
			},
		}

		resp, err := svc.BulkCreateProjects(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalCreated)
		require.NotNil(t, capturedProject)
		assert.Equal(t, "MTC", capturedProject.Name)
		assert.Equal(t, "Major TikTok Contract", capturedProject.Description)
		assert.Equal(t, []string{"TikTok", "migration", "Oracle"}, capturedProject.Keywords)
		assert.Equal(t, []string{"MTC", "MTC-2"}, capturedProject.JiraProjects)
	})
}

// ============ Error Handling Tests ============

func TestBulkCreatePeople_RepositoryErrors(t *testing.T) {
	t.Run("GetPersonByEmail error returns per-item error", func(t *testing.T) {
		svc := newMockableService()
		svc.mockEntityRepo.getPersonByEmailFn = func(ctx context.Context, tenantID, email string) (*entities.Person, error) {
			return nil, errors.New("database connection failed")
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People: []*entityv1.PersonInput{
				{Name: "John Doe", Email: "john@example.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err) // Method returns response, not error
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Error, "failed to check")
	})

	t.Run("CreatePerson error returns per-item error", func(t *testing.T) {
		svc := newMockableService()
		svc.mockEntityRepo.createPersonFn = func(ctx context.Context, p *entities.Person) error {
			return errors.New("unique constraint violation")
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People: []*entityv1.PersonInput{
				{Name: "John Doe", Email: "john@example.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(1), resp.TotalErrors)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Error, "failed to create")
	})

	t.Run("CreateAlias error is logged but person still created", func(t *testing.T) {
		svc := newMockableService()
		svc.mockEntityRepo.createAliasFn = func(ctx context.Context, alias *entities.PersonAlias) error {
			return errors.New("alias creation failed")
		}

		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: "tenant-1",
			People: []*entityv1.PersonInput{
				{Name: "John Doe", Email: "john@example.com", Aliases: []string{"JD"}},
			},
		}

		resp, err := svc.BulkCreatePeople(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Person should still be created even if alias fails
		assert.Equal(t, int32(1), resp.TotalCreated)
		assert.Equal(t, int32(0), resp.TotalErrors)
	})
}

// ============ Helper for Mockable Service ============

// mockableService wraps Service with mockable repositories
type mockableService struct {
	*Service
	mockEntityRepo  *mockEntityRepo
	mockProductRepo *mockProductRepo
}

func newMockableService() *mockableService {
	logger := testLogger()
	entityRepo := &mockEntityRepo{}
	productRepo := &mockProductRepo{}

	// Create the service with nil repos (we'll intercept calls)
	svc := &mockableService{
		Service: &Service{
			logger: logger,
		},
		mockEntityRepo:  entityRepo,
		mockProductRepo: productRepo,
	}

	return svc
}

// Override BulkCreatePeople to use mocks
func (s *mockableService) BulkCreatePeople(ctx context.Context, req *entityv1.BulkCreatePeopleRequest) (*entityv1.BulkCreatePeopleResponse, error) {
	s.Service.logger.Debug("BulkCreatePeople called",
		logging.F("tenant_id", req.TenantId),
		logging.F("count", len(req.People)),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if len(req.People) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one person is required")
	}

	resp := &entityv1.BulkCreatePeopleResponse{
		TotalRequested: int32(len(req.People)),
	}

	for i, input := range req.People {
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

		existing, err := s.mockEntityRepo.GetPersonByEmail(ctx, req.TenantId, input.Email)
		if err != nil {
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

		person := &entities.Person{
			TenantID:      req.TenantId,
			CanonicalName: input.Name,
			PrimaryEmail:  input.Email,
			Title:         input.Title,
			Department:    input.Department,
			IsInternal:    input.IsInternal,
			AccountType:   entities.AccountTypePerson,
			Confidence:    0.9,
			NeedsReview:   false,
			AutoCreated:   false,
		}

		if err := s.mockEntityRepo.CreatePerson(ctx, person); err != nil {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Email,
				Error:      "failed to create person: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		for _, aliasValue := range input.Aliases {
			aliasType := entities.AliasTypeName
			if len(aliasValue) > 0 && aliasValue[0] != '@' {
				for _, c := range aliasValue {
					if c == '@' {
						aliasType = entities.AliasTypeEmail
						break
					}
				}
			}

			alias := &entities.PersonAlias{
				PersonID:   person.ID,
				AliasType:  aliasType,
				AliasValue: aliasValue,
				Confidence: 1.0,
				Source:     "seed",
			}
			_ = s.mockEntityRepo.CreateAlias(ctx, alias)
		}

		resp.Created = append(resp.Created, &entityv1.Person{
			Id:         person.ID,
			Name:       person.CanonicalName,
			Email:      person.PrimaryEmail,
			Company:    input.Company,
			Title:      person.Title,
			Department: person.Department,
			IsInternal: person.IsInternal,
			CreatedAt:  nil,
		})
		resp.TotalCreated++
	}

	return resp, nil
}

// Override BulkCreateProducts to use mocks
func (s *mockableService) BulkCreateProducts(ctx context.Context, req *entityv1.BulkCreateProductsRequest) (*entityv1.BulkCreateProductsResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if len(req.Products) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one product is required")
	}

	resp := &entityv1.BulkCreateProductsResponse{
		TotalRequested: int32(len(req.Products)),
	}

	createdProducts := make(map[string]int64)

	for i, input := range req.Products {
		if input.Name == "" {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: "(empty name)",
				Error:      "name is required",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		existing, err := s.mockProductRepo.GetProductByName(ctx, req.TenantId, input.Name)
		if err != nil && !errors.Is(err, products.ErrNotFound) {
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

		var parentID *int64
		if input.ParentName != "" {
			if pid, ok := createdProducts[input.ParentName]; ok {
				parentID = &pid
			} else {
				parent, err := s.mockProductRepo.GetProductByName(ctx, req.TenantId, input.ParentName)
				if err == nil && parent != nil {
					parentID = &parent.ID
				}
			}
		}

		productType := products.ProductTypeProduct
		switch input.ProductType {
		case "sub_product", "subproduct":
			productType = products.ProductTypeSubProduct
		case "feature":
			productType = products.ProductTypeFeature
		}

		productStatus := products.ProductStatusActive
		switch input.Status {
		case "beta":
			productStatus = products.ProductStatusBeta
		case "sunset":
			productStatus = products.ProductStatusSunset
		case "deprecated":
			productStatus = products.ProductStatusDeprecated
		}

		var description *string
		if input.Description != "" {
			description = &input.Description
		}

		product := &products.Product{
			TenantID:    req.TenantId,
			Name:        input.Name,
			Description: description,
			ParentID:    parentID,
			ProductType: productType,
			Status:      productStatus,
			Keywords:    input.Keywords,
		}

		if err := s.mockProductRepo.CreateProduct(ctx, product); err != nil {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: input.Name,
				Error:      "failed to create product: " + err.Error(),
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		createdProducts[input.Name] = product.ID

		for _, alias := range input.Aliases {
			_ = s.mockProductRepo.AddAlias(ctx, product.ID, alias)
		}

		created := &entityv1.Product{
			Id:          product.ID,
			Name:        product.Name,
			ProductType: string(product.ProductType),
			Status:      string(product.Status),
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

	return resp, nil
}

// Override BulkCreateProjects to use mocks
func (s *mockableService) BulkCreateProjects(ctx context.Context, req *entityv1.BulkCreateProjectsRequest) (*entityv1.BulkCreateProjectsResponse, error) {
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if len(req.Projects) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one project is required")
	}

	resp := &entityv1.BulkCreateProjectsResponse{
		TotalRequested: int32(len(req.Projects)),
	}

	for i, input := range req.Projects {
		if input.Name == "" {
			resp.Errors = append(resp.Errors, &entityv1.EntityError{
				Identifier: "(empty name)",
				Error:      "name is required",
				Index:      int32(i),
			})
			resp.TotalErrors++
			continue
		}

		existing, err := s.mockEntityRepo.GetProjectByName(ctx, req.TenantId, input.Name)
		if err != nil {
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

		project := &entities.Project{
			TenantID:     req.TenantId,
			Name:         input.Name,
			Description:  input.Description,
			Keywords:     input.Keywords,
			JiraProjects: input.JiraProjects,
		}

		if err := s.mockEntityRepo.CreateProject(ctx, project); err != nil {
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
		})
		resp.TotalCreated++
	}

	return resp, nil
}
