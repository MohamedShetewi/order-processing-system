BEGIN;

-- Backs replay-on-connect for the per-order notification WebSocket: on connect the
-- handler loads a subscriber's notifications via WHERE order_id = ?. Postgres does
-- not auto-index the order_id foreign key, so add it explicitly.
CREATE INDEX idx_notifications_order_id ON notifications (order_id);

COMMIT;
