-- =============================================================================
-- Gem & Jewelry Store Management System — Initial Schema
-- Target: PostgreSQL 14+
-- =============================================================================

BEGIN;

-- ---------------------------------------------------------------------------
-- Extensions
-- ---------------------------------------------------------------------------
-- gen_random_uuid() for primary keys. citext for case-insensitive email.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- ---------------------------------------------------------------------------
-- Enums
-- ---------------------------------------------------------------------------
CREATE TYPE user_role AS ENUM ('customer', 'admin');

CREATE TYPE jewelry_category AS ENUM ('ring', 'bracelet', 'necklace');

CREATE TYPE gender_type AS ENUM ('men', 'women', 'unisex');

CREATE TYPE order_type AS ENUM ('standard', 'custom');

-- Mirrors the status list in the proposal's order-tracking page (Section 5.1)
CREATE TYPE order_status AS ENUM (
    'cart',             -- not yet submitted
    'pending_review',   -- custom order awaiting owner decision
    'confirmed',        -- owner confirmed a custom order (or standard order paid)
    'declined',         -- owner declined a custom order
    'in_progress',      -- confirmed order in production/fulfilment
    'shipped',
    'delivered',
    'cancelled'
);

CREATE TYPE approval_decision AS ENUM ('confirmed', 'declined');

-- ---------------------------------------------------------------------------
-- Trigger helper: auto-maintain updated_at
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         CITEXT NOT NULL,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    phone         TEXT,
    role          user_role NOT NULL DEFAULT 'customer',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_email_unique UNIQUE (email)
);

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- products  (ready-made catalogue items — Section 5.1 "Buy Now" path)
-- ---------------------------------------------------------------------------
CREATE TABLE products (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sku             TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT,
    category        jewelry_category NOT NULL,
    gender          gender_type NOT NULL DEFAULT 'unisex',
    metal_type      TEXT,
    gemstone_type   TEXT,
    base_price      NUMERIC(12, 2) NOT NULL CHECK (base_price >= 0),
    stock_quantity  INTEGER NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
    image_url       TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT products_sku_unique UNIQUE (sku)
);

CREATE INDEX idx_products_category ON products (category);
CREATE INDEX idx_products_gender ON products (gender);
CREATE INDEX idx_products_active_category_gender ON products (is_active, category, gender);

CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- customizations  (the customer-defined spec from the customization workspace)
-- ---------------------------------------------------------------------------
CREATE TABLE customizations (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category           jewelry_category NOT NULL,
    gender             gender_type NOT NULL DEFAULT 'unisex',
    metal_type         TEXT NOT NULL,
    gemstone_type      TEXT,
    size               TEXT,             -- ring size / bracelet & necklace length
    engraving_text     TEXT,
    preview_image_url  TEXT,             -- snapshot of the live preview at submit time
    estimated_price    NUMERIC(12, 2) CHECK (estimated_price >= 0),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_customizations_user_id ON customizations (user_id);

CREATE TRIGGER trg_customizations_updated_at
    BEFORE UPDATE ON customizations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- orders
-- ---------------------------------------------------------------------------
CREATE TABLE orders (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id),
    order_type        order_type NOT NULL,
    status            order_status NOT NULL DEFAULT 'cart',
    shipping_address  JSONB,
    total_amount      NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_user_id ON orders (user_id);
CREATE INDEX idx_orders_status ON orders (status);
CREATE INDEX idx_orders_type_status ON orders (order_type, status);

CREATE TRIGGER trg_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- order_items
-- An order line references EITHER a ready-made product OR a customization —
-- never both, never neither. This lets one order mix standard + custom lines
-- while keeping each line's origin unambiguous for the admin review screen.
-- ---------------------------------------------------------------------------
CREATE TABLE order_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id        UUID REFERENCES products(id),
    customization_id  UUID REFERENCES customizations(id),
    quantity          INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_price        NUMERIC(12, 2) NOT NULL CHECK (unit_price >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT order_items_exactly_one_source CHECK (
        (product_id IS NOT NULL AND customization_id IS NULL) OR
        (product_id IS NULL AND customization_id IS NOT NULL)
    )
);

CREATE INDEX idx_order_items_order_id ON order_items (order_id);
CREATE INDEX idx_order_items_product_id ON order_items (product_id);
CREATE INDEX idx_order_items_customization_id ON order_items (customization_id);

-- ---------------------------------------------------------------------------
-- order_approvals
-- Audit trail of the owner's confirm/decline decisions (Section 8, step 4-5).
-- One row per decision — if you ever allow re-review, history is preserved.
-- ---------------------------------------------------------------------------
CREATE TABLE order_approvals (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    admin_id     UUID NOT NULL REFERENCES users(id),
    decision     approval_decision NOT NULL,
    notes        TEXT,
    decided_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_approvals_order_id ON order_approvals (order_id);
CREATE INDEX idx_order_approvals_admin_id ON order_approvals (admin_id);

-- ---------------------------------------------------------------------------
-- Guard rail: admin_id on order_approvals must actually be an admin.
-- Enforced in application code (service layer) rather than a DB trigger,
-- to keep the trigger graph simple — see OrderService.ApproveOrder.
-- ---------------------------------------------------------------------------

COMMIT;
