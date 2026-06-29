package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

type ProductRepository interface {
	ListProducts(ctx context.Context, limit, offset int) ([]models.Product, int64, error)
	GetProduct(ctx context.Context, id int) (*models.Product, error)
	CreateProduct(ctx context.Context, product *models.Product) error
}

type productRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepository{db: db}
}

func (r *productRepository) ListProducts(ctx context.Context, limit, offset int) ([]models.Product, int64, error) {
	var (
		products []models.Product
		total    int64
	)

	if err := r.db.WithContext(ctx).Model(&models.Product{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).
		Preload("Inventory").
		Order("id").
		Limit(limit).
		Offset(offset).
		Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func (r *productRepository) CreateProduct(ctx context.Context, product *models.Product) error {
	return r.db.WithContext(ctx).Create(product).Error
}

func (r *productRepository) GetProduct(ctx context.Context, id int) (*models.Product, error) {
	var product models.Product
	if err := r.db.WithContext(ctx).
		Preload("Inventory").
		First(&product, id).Error; err != nil {
		return nil, err
	}
	return &product, nil
}
