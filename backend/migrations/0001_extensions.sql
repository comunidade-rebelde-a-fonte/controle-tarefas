-- +goose Up
-- Extensões usadas pelo modelo de dados:
--   pgcrypto   gen_random_uuid()
--   btree_gist EXCLUDE de vigências (regime tributário)
--   pg_trgm    busca textual por nome/razão social
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- +goose Down
DROP EXTENSION IF EXISTS pg_trgm;
DROP EXTENSION IF EXISTS btree_gist;
DROP EXTENSION IF EXISTS pgcrypto;
