package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/MohamedShetewi/order-processing-system/internal/apperrors"
	"github.com/MohamedShetewi/order-processing-system/internal/dto"
	"github.com/MohamedShetewi/order-processing-system/internal/idempotency"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
	"github.com/MohamedShetewi/order-processing-system/internal/repository"
)

// idempotencyTTL bounds the window during which a duplicate submission of the
// same cart collapses to a single order. After it expires, re-orders are allowed.
const idempotencyTTL = 5 * time.Minute

type OrderService interface {
	CreateOrder(ctx context.Context, userID int, req dto.CreateOrderRequest) (dto.OrderResponse, error)
}

type orderService struct {
	orders      repository.OrderRepository
	products    repository.ProductRepository
	idempotency idempotency.Store
	processor   OrderProcessor
}

func NewOrderService(
	orders repository.OrderRepository,
	products repository.ProductRepository,
	idempotency idempotency.Store,
	processor OrderProcessor,
) OrderService {
	return &orderService{
		orders:      orders,
		products:    products,
		idempotency: idempotency,
		processor:   processor,
	}
}

func (s *orderService) CreateOrder(ctx context.Context, userID int, req dto.CreateOrderRequest) (dto.OrderResponse, error) {
	// 1. Reject duplicate product lines in the same request.
	seen := make(map[int]struct{}, len(req.Items))
	for _, item := range req.Items {
		if _, dup := seen[item.ProductID]; dup {
			return dto.OrderResponse{}, apperrors.ErrDuplicateLineItem
		}
		seen[item.ProductID] = struct{}{}
	}

	// 2. Reserve the cart's idempotency key transiently.
	key := idempotencyKey(userID, req.Items)
	acquired, err := s.idempotency.Reserve(ctx, key, idempotencyTTL)
	if err != nil {
		// Degrade open: prefer availability over strict dedup when the store is
		// unavailable.
		log.Printf("idempotency reserve failed for key %s: %v (continuing)", key, err)
	} else if !acquired {
		return dto.OrderResponse{}, apperrors.ErrDuplicateOrder
	}

	// 3. Fetch products to obtain prices and validate existence.
	ids := make([]int, 0, len(req.Items))
	for _, item := range req.Items {
		ids = append(ids, item.ProductID)
	}
	products, err := s.products.GetProductsByIDs(ctx, ids)
	if err != nil {
		s.release(ctx, key)
		return dto.OrderResponse{}, err
	}
	priceByID := make(map[int]float64, len(products))
	for _, p := range products {
		priceByID[p.ID] = p.Price
	}

	// 4. Build the order, items, and payment. Items are sorted by ProductID so the
	//    repository locks inventory rows in a deadlock-safe order.
	items := make([]models.OrderItem, 0, len(req.Items))
	var total float64
	for _, item := range req.Items {
		price, ok := priceByID[item.ProductID]
		if !ok {
			s.release(ctx, key)
			return dto.OrderResponse{}, apperrors.ProductNotFoundError{ProductID: item.ProductID}
		}
		items = append(items, models.OrderItem{
			ProductID:       item.ProductID,
			Quantity:        item.Quantity,
			PriceAtPurchase: price,
		})
		total += price * float64(item.Quantity)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ProductID < items[j].ProductID
	})

	order := models.Order{
		UserID:     userID,
		Status:     models.OrderStatusPending,
		TotalPrice: total,
		Items:      items,
	}
	payment := models.Payment{
		IdempotencyKey: uuid.NewString(),
		Status:         models.PaymentStatusPending,
		Amount:         total,
	}

	// 5. Persist atomically.
	if err := s.orders.CreateOrder(ctx, &order, &payment); err != nil {
		// Release so the client can retry within the TTL.
		s.release(ctx, key)
		return dto.OrderResponse{}, err
	}

	// 6. Hand the order off for asynchronous fulfillment.
	s.processor.Process(order.ID)

	return toOrderResponse(&order), nil
}

func (s *orderService) release(ctx context.Context, key string) {
	if err := s.idempotency.Release(ctx, key); err != nil {
		log.Printf("idempotency release failed for key %s: %v", key, err)
	}
}

// idempotencyKey derives a stable key from the user and their cart contents,
// independent of item ordering.
func idempotencyKey(userID int, items []dto.OrderItemRequest) string {
	sorted := make([]dto.OrderItemRequest, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ProductID < sorted[j].ProductID
	})

	h := sha256.New()
	h.Write([]byte("u:" + strconv.Itoa(userID)))
	for _, item := range sorted {
		h.Write([]byte("|" + strconv.Itoa(item.ProductID) + ":" + strconv.Itoa(item.Quantity)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func toOrderResponse(o *models.Order) dto.OrderResponse {
	items := make([]dto.OrderItemResponse, len(o.Items))
	for i := range o.Items {
		items[i] = dto.OrderItemResponse{
			ProductID:       o.Items[i].ProductID,
			Quantity:        o.Items[i].Quantity,
			PriceAtPurchase: o.Items[i].PriceAtPurchase,
		}
	}
	return dto.OrderResponse{
		ID:         o.ID,
		UserID:     o.UserID,
		Status:     o.Status,
		TotalPrice: o.TotalPrice,
		Items:      items,
		CreatedAt:  o.CreatedAt,
		UpdatedAt:  o.UpdatedAt,
	}
}
