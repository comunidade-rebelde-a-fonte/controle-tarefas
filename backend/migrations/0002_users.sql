-- +goose Up
-- Usuários [PRD §34, §38.20]. Sem login no MVP: o usuário implícito "Administrador"
-- (UUID fixo) é injetado em toda requisição e registrado em todas as escritas [plan D9].
-- Os demais usuários servem como responsáveis por setor.
CREATE TABLE users (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL CHECK (btrim(name) <> ''),
    email       text        NULL CHECK (email IS NULL OR email ~ '^[^@\s]+@[^@\s]+$'),
    role        text        NOT NULL CHECK (role IN ('ADMIN', 'MANAGER', 'OPERATOR', 'READ_ONLY')),
    active      boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_uq ON users (lower(email)) WHERE email IS NOT NULL;

INSERT INTO users (id, name, role)
VALUES ('00000000-0000-0000-0000-000000000001', 'Administrador', 'ADMIN');

-- +goose Down
DROP TABLE users;
