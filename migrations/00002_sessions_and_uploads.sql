-- +goose Up
ALTER TABLE material_dispatch.sys_users ADD COLUMN failed_login_attempts integer NOT NULL DEFAULT 0 CHECK (failed_login_attempts >= 0), ADD COLUMN locked_until timestamptz;
CREATE TABLE material_dispatch.sessions (
 token_hash char(64) PRIMARY KEY,
 user_id bigint NOT NULL REFERENCES material_dispatch.sys_users(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_sessions_user ON material_dispatch.sessions(user_id);
CREATE INDEX idx_sessions_expiry ON material_dispatch.sessions(expires_at);
CREATE TABLE material_dispatch.uploads (
 id text PRIMARY KEY,
 owner_id bigint NOT NULL REFERENCES material_dispatch.sys_users(id),
 storage_key text NOT NULL UNIQUE,
 original_name varchar(255) NOT NULL,
 content_type varchar(64) NOT NULL CHECK (content_type IN ('image/jpeg','image/png','image/webp')),
 size_bytes bigint NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 5242880),
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_uploads_owner ON material_dispatch.uploads(owner_id);

-- +goose Down
DROP TABLE material_dispatch.uploads;
DROP TABLE material_dispatch.sessions;
ALTER TABLE material_dispatch.sys_users DROP COLUMN locked_until, DROP COLUMN failed_login_attempts;
