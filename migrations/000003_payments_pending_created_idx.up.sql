BEGIN;

-- Backs the sweeper's stale-pending scan: WHERE status='pending' AND created_at < cutoff
-- ORDER BY created_at ASC LIMIT n. Partial on status='pending' mirrors idx_notifications_pending.
CREATE INDEX idx_payments_pending_created ON payments (created_at) WHERE status = 'pending';

COMMIT;
