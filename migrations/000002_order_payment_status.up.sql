BEGIN;

ALTER TABLE orders   ADD COLUMN status order_status   NOT NULL DEFAULT 'pending';
ALTER TABLE payments ADD COLUMN status payment_status NOT NULL DEFAULT 'pending';

COMMIT;
