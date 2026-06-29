# Senior Backend Engineer - Golang Task

## Overview

Build a **Concurrent Order Processing System** for an e-commerce platform that can handle high-volume order processing with real-time inventory management, payment processing, and notifications.

## Technical Requirements

### Core Technologies

- **Language**: Go 1.21+
- **Database**: PostgreSQL 15+
- **ORM**: GORM v2
- **Framework**: Gin (recommended) or Echo
- **Concurrency**: Goroutines, Channels, Sync packages
- **Testing**: Built-in testing + testify

### Mandatory Features

#### 1. **API Design & Implementation**

- RESTful API with proper HTTP status codes
- Input validation and error handling
- Middleware for logging, authentication, and rate limiting
- API documentation (OpenAPI/Swagger)

#### 2. **Database Design & Management**

- PostgreSQL with proper indexing strategy
- Database migrations using GORM
- Connection pooling and transaction management
- Referential integrity and constraints

#### 3. **Concurrency Requirements**

- **Order Processing Pipeline**: Process multiple orders simultaneously
- **Inventory Management**: Handle concurrent inventory updates without race conditions
- **Notification System**: Send notifications asynchronously
- **Report Generation**: Generate reports concurrently with order processing
- **Background Jobs**: Implement job queue for heavy operations

#### 4. **Business Logic**

Implement a complete order processing workflow:

```
Order Placement → Inventory Check → Payment Processing → Order Fulfillment → Notification → Reporting
```

## System Architecture

### Database Schema

Design and implement the following entities:

1. **Users** (customers and admins)
2. **Products** (with inventory tracking)
3. **Orders** (with status tracking)
4. **OrderItems** (order line items)
5. **Payments** (payment transactions)
6. **Inventory** (stock management)
7. **Notifications** (system notifications)
8. **AuditLogs** (for tracking changes)

### API Endpoints

#### User Management

- `POST /api/v1/users` - Create user
- `GET /api/v1/users/{id}` - Get user profile
- `PUT /api/v1/users/{id}` - Update user profile

#### Product Management

- `GET /api/v1/products` - List products (with pagination)
- `GET /api/v1/products/{id}` - Get product details
- `POST /api/v1/products` - Create product (admin)
- `PUT /api/v1/products/{id}` - Update product (admin)
- `GET /api/v1/products/{id}/inventory` - Check inventory

#### Order Management

- `POST /api/v1/orders` - Create order
- `GET /api/v1/orders` - List user orders
- `GET /api/v1/orders/{id}` - Get order details
- `PUT /api/v1/orders/{id}/cancel` - Cancel order
- `GET /api/v1/orders/{id}/status` - Get order status

#### Admin Endpoints

- `GET /api/v1/admin/orders` - List all orders
- `PUT /api/v1/admin/orders/{id}/status` - Update order status
- `GET /api/v1/admin/reports/daily` - Daily sales report
- `GET /api/v1/admin/inventory/low-stock` - Low stock alerts

## Concurrency Challenges

### 1. **Race Condition Prevention**

- Implement proper locking mechanisms for inventory updates
- Use database transactions for critical operations
- Handle concurrent order processing without overselling

### 2. **Performance Optimization**

- Process multiple orders simultaneously
- Implement worker pools for background tasks
- Use channels for communication between goroutines

### 3. **Real-time Features**

- Real-time inventory updates
- Live order status tracking
- Instant notifications

## Implementation Guidelines

### Project Structure

```
/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── handlers/
│   │   ├── middleware/
│   │   └── routes/
│   ├── models/
│   ├── services/
│   ├── repository/
│   ├── config/
│   └── workers/
├── pkg/
│   ├── database/
│   ├── logger/
│   └── utils/
├── migrations/
├── tests/
├── docker/
├── docs/
├── go.mod
├── go.sum
└── .env.example
```

### Best Practices to Implement

1. **Clean Architecture**

   - Separation of concerns
   - Dependency injection
   - Interface-driven design

2. **Error Handling**

   - Custom error types
   - Proper error propagation
   - Logging and monitoring

3. **Testing**

   - Unit tests with >80% coverage
   - Integration tests for APIs
   - Concurrent testing scenarios

4. **Security**

   - JWT authentication
   - Input validation and sanitization
   - SQL injection prevention
   - Rate limiting

5. **Performance**
   - Database query optimization
   - Connection pooling
   - Caching strategies (optional: Redis)

## Specific Concurrency Scenarios to Handle

### Scenario 1: High-Volume Order Processing

```
Challenge: Process 1000 orders simultaneously while maintaining data consistency
Solution: Implement worker pool pattern with proper synchronization
```

### Scenario 2: Inventory Race Conditions

```
Challenge: Multiple customers trying to buy the last item simultaneously
Solution: Use database transactions with proper isolation levels
```

### Scenario 3: Payment Processing

```
Challenge: Handle payment failures and retries without double-charging
Solution: Implement idempotent operations with proper state management
```

### Scenario 4: Notification System

```
Challenge: Send notifications without blocking order processing
Solution: Asynchronous notification system using goroutines and channels
```

## Evaluation Criteria

### Technical Implementation (40%)

- Code quality and organization
- Proper use of Go idioms and patterns
- GORM usage and database design
- Concurrency implementation

### API Design (20%)

- RESTful design principles
- Proper HTTP status codes
- Input validation and error handling
- Documentation quality

### Concurrency & Performance (25%)

- Effective use of goroutines and channels
- Race condition handling
- Performance under load
- Resource management

### Best Practices (15%)

- Error handling
- Testing coverage
- Security considerations
- Code documentation

## Deliverables

1. **Complete Go application** with all required features
2. **Database migrations** and seed data
3. **API documentation** (Swagger/OpenAPI)
4. **Test suite** with good coverage
5. **Docker setup** for easy deployment
6. **Performance benchmarks** for concurrent operations
7. **README with setup instructions**

## Time Limit

**5-7 days** (adjust based on candidate's availability)

## Bonus Points

- WebSocket integration for real-time updates
- Metrics and monitoring (Prometheus)
- Distributed tracing
- Message queue integration (RabbitMQ/Kafka)
- Kubernetes deployment manifests
- Load testing scenarios

## Getting Started

1. Fork this repository
2. Set up PostgreSQL database
3. Copy `.env.example` to `.env` and configure
4. Run `go mod init` and install dependencies
5. Implement the solution step by step
6. Document your design decisions

## Questions to Consider

- How would you handle database failover?
- What's your strategy for horizontal scaling?
- How would you implement distributed locks?
- What monitoring would you add in production?

---

**Good luck! We're excited to see your implementation.** 🚀