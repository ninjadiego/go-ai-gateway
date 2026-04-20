-- Initial schema for Go AI Gateway
-- MySQL 8.0+

SET NAMES utf8mb4;

-- ─── users ──────────────────────────────────────────────────────────────────
-- Gateway owners/admins who create and manage API keys.
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    email         VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name          VARCHAR(100) NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── api_keys ───────────────────────────────────────────────────────────────
-- Tenant-scoped keys used by client applications.
-- The full key (`gw_live_...`) is NEVER stored — only the SHA-256 hash.
CREATE TABLE IF NOT EXISTS api_keys (
    id                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id              BIGINT UNSIGNED NOT NULL,
    name                 VARCHAR(100) NOT NULL,
    key_hash             CHAR(64) NOT NULL,            -- SHA-256 hex digest
    key_prefix           VARCHAR(16) NOT NULL,         -- e.g. "gw_live_abcd" for display
    rate_limit_rpm       INT UNSIGNED NOT NULL DEFAULT 60,
    daily_token_limit    BIGINT UNSIGNED NOT NULL DEFAULT 1000000,
    monthly_budget_usd   DECIMAL(10,2) DEFAULT NULL,
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    last_used_at         TIMESTAMP NULL DEFAULT NULL,
    expires_at           TIMESTAMP NULL DEFAULT NULL,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_api_keys_hash (key_hash),
    KEY idx_api_keys_user (user_id),
    CONSTRAINT fk_api_keys_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── requests ───────────────────────────────────────────────────────────────
-- Append-only log of every proxied request. Used for audit + billing.
CREATE TABLE IF NOT EXISTS requests (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    api_key_id         BIGINT UNSIGNED NOT NULL,
    provider           VARCHAR(50)  NOT NULL,           -- 'anthropic' | 'openai'
    model              VARCHAR(100) NOT NULL,           -- e.g. 'claude-sonnet-4-6'
    input_tokens       INT UNSIGNED NOT NULL DEFAULT 0,
    output_tokens      INT UNSIGNED NOT NULL DEFAULT 0,
    cache_read_tokens  INT UNSIGNED NOT NULL DEFAULT 0,
    cache_write_tokens INT UNSIGNED NOT NULL DEFAULT 0,
    cost_usd           DECIMAL(12,8) NOT NULL DEFAULT 0,
    latency_ms         INT UNSIGNED NOT NULL DEFAULT 0,
    status_code        SMALLINT UNSIGNED NOT NULL,
    error_message      TEXT DEFAULT NULL,
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_requests_key_time (api_key_id, created_at),
    KEY idx_requests_created (created_at),
    CONSTRAINT fk_requests_api_key
        FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── daily_usage ────────────────────────────────────────────────────────────
-- Pre-aggregated rollup so analytics endpoints stay fast even at millions
-- of requests/day. Updated on every request via INSERT ... ON DUPLICATE KEY.
CREATE TABLE IF NOT EXISTS daily_usage (
    id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    api_key_id          BIGINT UNSIGNED NOT NULL,
    usage_date          DATE NOT NULL,
    request_count       INT UNSIGNED NOT NULL DEFAULT 0,
    total_input_tokens  BIGINT UNSIGNED NOT NULL DEFAULT 0,
    total_output_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0,
    total_cost_usd      DECIMAL(14,6) NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uk_daily_usage_key_date (api_key_id, usage_date),
    CONSTRAINT fk_daily_usage_api_key
        FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
