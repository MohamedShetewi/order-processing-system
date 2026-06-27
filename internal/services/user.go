package services

import (
	"context"

	"github.com/MohamedShetewi/order-processing-system/internal/dto"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
)

type UserService interface {
	CreateUser(ctx context.Context, req dto.CreateUserRequest) (*dto.UserResponse, error)
	GetUser(ctx context.Context, id int) (*dto.UserResponse, error)
	UpdateUser(ctx context.Context, id int, req dto.UpdateUserRequest) (*dto.UserResponse, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) CreateUser(ctx context.Context, req dto.CreateUserRequest) (*dto.UserResponse, error) {
	user := &models.User{
		Name:           req.Name,
		Email:          req.Email,
		HashedPassword: req.Password,
		Role:           req.Role,
	}
	if user.Role == "" {
		user.Role = models.UserRoleCustomer
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return toUserResponse(user), nil
}

func (s *userService) GetUser(ctx context.Context, id int) (*dto.UserResponse, error) {
	user, err := s.repo.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}

	return toUserResponse(user), nil
}

func (s *userService) UpdateUser(ctx context.Context, id int, req dto.UpdateUserRequest) (*dto.UserResponse, error) {
	user, err := s.repo.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Role != "" {
		user.Role = req.Role
	}

	if err = s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return toUserResponse(user), nil
}

func toUserResponse(u *models.User) *dto.UserResponse {
	return &dto.UserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
