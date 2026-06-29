package dto

import "time"

type ProductResponse struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Image       *string   `json:"image"`
	Description *string   `json:"description"`
	Price       float64   `json:"price"`
	Quantity    int       `json:"quantity"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateProductRequest struct {
	Name        string  `json:"name"        binding:"required"`
	Image       *string `json:"image"`
	Description *string `json:"description"`
	Price       float64 `json:"price"       binding:"required,gte=0"`
	Quantity    int     `json:"quantity"    binding:"gte=0"`
}

type UpdateProductRequest struct {
	Name        string  `json:"name"        binding:"required"`
	Image       *string `json:"image"`
	Description *string `json:"description"`
	Price       float64 `json:"price"       binding:"required,gte=0"`
	Quantity    int     `json:"quantity"    binding:"gte=0"`
}

type ListProductsRequest struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

type ListProductsResponse struct {
	Items    []ProductResponse `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type InventoryResponse struct {
	ProductID int       `json:"product_id"`
	Quantity  int       `json:"quantity"`
	UpdatedAt time.Time `json:"updated_at"`
}
