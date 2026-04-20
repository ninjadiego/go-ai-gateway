-- Seed data for local development.
-- Password hashes here are placeholder bcrypt — replace with real ones in production.

INSERT INTO users (email, password_hash, name) VALUES
    ('admin@example.com',
     '$2a$10$PLACEHOLDER_HASH_NEVER_USE_IN_PRODUCTION',
     'Admin User')
ON DUPLICATE KEY UPDATE name = VALUES(name);

-- Note: API keys are created via POST /admin/keys, not via SQL,
-- because the raw key must be generated and returned to the caller once.
