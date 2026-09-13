DROP INDEX IF EXISTS catalog.idx_outbox_unpublished;
DROP INDEX IF EXISTS catalog.idx_order_items_order_id;
DROP INDEX IF EXISTS catalog.idx_orders_user_id;

DROP TABLE IF EXISTS catalog.outbox;
DROP TABLE IF EXISTS catalog.order_items;
DROP TABLE IF EXISTS catalog.orders;