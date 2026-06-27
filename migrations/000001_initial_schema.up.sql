BEGIN;

CREATE TYPE user_role            AS ENUM ('customer', 'admin');
CREATE TYPE order_status         AS ENUM ('pending', 'confirmed', 'shipped', 'delivered', 'cancelled');
CREATE TYPE payment_status       AS ENUM ('pending', 'paid', 'failed', 'refunded');
CREATE TYPE notification_status  AS ENUM ('pending', 'sent', 'failed');
CREATE TYPE notification_channel AS ENUM ('email', 'sms', 'push');


CREATE TABLE users (
    id              INTEGER     GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            TEXT        NOT NULL,
    email           TEXT        NOT NULL UNIQUE,
    hashed_password TEXT        NOT NULL,
    role            user_role   NOT NULL DEFAULT 'customer',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE products (
    id          INTEGER       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT          NOT NULL,
    image       TEXT,
    description TEXT,
    price       NUMERIC(12,2) NOT NULL CHECK (price >= 0),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE TABLE inventory (
    id         INTEGER     GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id INTEGER     NOT NULL UNIQUE REFERENCES products(id) ON DELETE CASCADE,
    quantity   INTEGER     NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id          INTEGER       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     INTEGER       NOT NULL REFERENCES users(id),
    total_price NUMERIC(12,2) NOT NULL CHECK (total_price >= 0),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE TABLE order_items (
    id                INTEGER       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id          INTEGER       NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id        INTEGER       NOT NULL REFERENCES products(id),
    quantity          INTEGER       NOT NULL CHECK (quantity > 0),
    price_at_purchase NUMERIC(12,2) NOT NULL CHECK (price_at_purchase >= 0),
    UNIQUE (order_id, product_id)
);
CREATE INDEX idx_order_items_order ON order_items (order_id);

CREATE TABLE payments (
    id              INTEGER        GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id        INTEGER        NOT NULL REFERENCES orders(id),
    idempotency_key TEXT           NOT NULL UNIQUE,
    amount          NUMERIC(12,2)  NOT NULL CHECK (amount >= 0),
    provider_txn_id TEXT,
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id         INTEGER              GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id   INTEGER              NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    message    TEXT                 NOT NULL,
    status     notification_status  NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ          NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ
);
CREATE INDEX idx_notifications_pending ON notifications (status) WHERE status = 'pending';

CREATE TABLE audit_logs (
    id          INTEGER     GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id    INTEGER     REFERENCES users(id),
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   INTEGER     NOT NULL,
    old_value   JSONB,
    new_value   JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_entity ON audit_logs (entity_type, entity_id);

COMMIT;
