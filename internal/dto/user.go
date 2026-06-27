package dto

import (
	"time"

	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

type CreateUserRequest struct {
	Name     string          `json:"name"     binding:"required"`
	Email    string          `json:"email"    binding:"required,email"`
	Password string          `json:"password" binding:"required,min=8"`
	Role     models.UserRole `json:"role"`
}

type UpdateUserRequest struct {
	Name  string          `json:"name"`
	Email string          `json:"email" binding:"omitempty,email"`
	Role  models.UserRole `json:"role"`
}

type UserResponse struct {
	ID        int             `json:"id"`
	Name      string          `json:"name"`
	Email     string          `json:"email"`
	Role      models.UserRole `json:"role"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
