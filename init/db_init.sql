-- users (если ещё нет)
CREATE TABLE users (
    uuid        UUID PRIMARY KEY,
    login       TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- индекс для case-insensitive уникальности
CREATE UNIQUE INDEX uq_users_login_lower ON users (lower(login));
CREATE INDEX idx_users_created_at_uuid
    ON users (created_at ASC, uuid ASC);


-- documents
CREATE TABLE documents (
    uuid           UUID PRIMARY KEY,
    owner_uuid     UUID NOT NULL REFERENCES users(uuid) ON DELETE CASCADE,
    filename_original TEXT NOT NULL,
    size_bytes     BIGINT NOT NULL,
    mime_type      TEXT NOT NULL,
    sha256         TEXT NOT NULL,
    storage_path   TEXT NOT NULL,
    is_file        BOOLEAN NOT NULL DEFAULT true,
    is_public      BOOLEAN NOT NULL DEFAULT false,
    access_token   TEXT UNIQUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ NULL
);
CREATE INDEX idx_documents_owner_created ON documents(owner_uuid, created_at DESC, uuid);
CREATE INDEX idx_documents_sha256 ON documents(sha256);
CREATE INDEX idx_documents_access_token ON documents(access_token);
CREATE INDEX idx_documents_owner_name_created
    ON documents(owner_uuid, filename_original ASC, created_at ASC);

-- sharing ACL
CREATE TABLE document_grants (
    document_uuid  UUID NOT NULL REFERENCES documents(uuid) ON DELETE CASCADE,
    target_user_uuid UUID NOT NULL REFERENCES users(uuid) ON DELETE CASCADE,
    -- permission     TEXT NOT NULL CHECK (permission IN ('read','write')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ NULL,
    PRIMARY KEY (document_uuid, target_user_uuid)
);
CREATE INDEX idx_document_grants_document ON document_grants(document_uuid);
CREATE INDEX idx_document_grants_user ON document_grants(target_user_uuid);

-- public links (на будущее)
CREATE TABLE document_links (
    uuid        UUID PRIMARY KEY,
    document_uuid UUID NOT NULL REFERENCES documents(uuid) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    used_once   BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX idx_document_links_doc ON document_links(document_uuid);
CREATE INDEX idx_document_links_expires ON document_links(expires_at);

-- refresh tokens (one-to-many)
CREATE TABLE refresh_tokens (
    uuid          UUID PRIMARY KEY,        -- или BIGSERIAL, но UUID удобнее как внешняя ссылка
    user_uuid   UUID NOT NULL REFERENCES users(uuid) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    expire_at   TIMESTAMPTZ NOT NULL,
    used        BOOLEAN NOT NULL DEFAULT false,
    user_agent  TEXT,
    ip_address  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_rt_user_exp ON refresh_tokens(user_uuid, expire_at);
CREATE UNIQUE INDEX uq_rt_token_hash ON refresh_tokens(token_hash);
