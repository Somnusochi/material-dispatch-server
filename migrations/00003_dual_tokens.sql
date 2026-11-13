-- +goose Up
CREATE TABLE material_dispatch.session_families (
 id text PRIMARY KEY,
 user_id bigint NOT NULL REFERENCES material_dispatch.sys_users(id) ON DELETE CASCADE,
 csrf_token text NOT NULL,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_session_families_user ON material_dispatch.session_families(user_id);
CREATE INDEX idx_session_families_expiry ON material_dispatch.session_families(expires_at);
ALTER TABLE material_dispatch.sessions ADD COLUMN family_id text REFERENCES material_dispatch.session_families(id) ON DELETE CASCADE;
CREATE INDEX idx_sessions_family ON material_dispatch.sessions(family_id);
CREATE TABLE material_dispatch.refresh_tokens (
 token_hash char(64) PRIMARY KEY,
 family_id text NOT NULL REFERENCES material_dispatch.session_families(id) ON DELETE CASCADE,
 used_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_refresh_tokens_family ON material_dispatch.refresh_tokens(family_id);

-- +goose Down
DROP TABLE material_dispatch.refresh_tokens;
-- Reverting authentication requires sign-in again, without changing business data.
DELETE FROM material_dispatch.sessions WHERE family_id IS NOT NULL;
ALTER TABLE material_dispatch.sessions DROP COLUMN family_id;
DROP TABLE material_dispatch.session_families;
