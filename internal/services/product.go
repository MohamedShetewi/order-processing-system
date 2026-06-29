package services

import (
	"context"

	"github.com/MohamedShetewi/order-processing-system/internal/dto"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

type ProductService interface {
	ListProducts(ctx context.Context, req dto.ListProductsRequest) (dto.ListProductsResponse, error)
	GetProduct(ctx context.Context, id int) (dto.ProductResponse, error)
	CreateProduct(ctx context.Context, req dto.CreateProductRequest) (dto.ProductResponse, error)
}

type productService struct {
	repo repository.ProductRepository
}

func NewProductService(repo repository.ProductRepository) ProductService {
	return &productService{repo: repo}
}

func (s *productService) ListProducts(ctx context.Context, req dto.ListProductsRequest) (dto.ListProductsResponse, error) {
	page := req.Page
	if page < 1 {
		page = defaultPage
	}

	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	products, total, err := s.repo.ListProducts(ctx, pageSize, (page-1)*pageSize)
	if err != nil {
		return dto.ListProductsResponse{}, err
	}

	items := make([]dto.ProductResponse, len(products))
	for i := range products {
		items[i] = toProductResponse(&products[i])
	}

	return dto.ListProductsResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *productService) CreateProduct(ctx context.Context, req dto.CreateProductRequest) (dto.ProductResponse, error) {
	product := &models.Product{
		Name:        req.Name,
		Image:       req.Image,
		Description: req.Description,
		Price:       req.Price,
		Inventory:   &models.Inventory{Quantity: req.Quantity},
	}

	if err := s.repo.CreateProduct(ctx, product); err != nil {
		return dto.ProductResponse{}, err
	}

	return toProductResponse(product), nil
}

func (s *productService) GetProduct(ctx context.Context, id int) (dto.ProductResponse, error) {
	product, err := s.repo.GetProduct(ctx, id)
	if err != nil {
		return dto.ProductResponse{}, err
	}
	return toProductResponse(product), nil
}

func toProductResponse(p *models.Product) dto.ProductResponse {
	resp := dto.ProductResponse{
		ID:          p.ID,
		Name:        p.Name,
		Image:       p.Image,
		Description: p.Description,
		Price:       p.Price,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	if p.Inventory != nil {
		resp.Quantity = p.Inventory.Quantity
	}
	return resp
}
