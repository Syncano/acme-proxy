-- +goose Up
CREATE TABLE acme_user (
    id SERIAL PRIMARY KEY,
    email TEXT NOT NULL,
    private_key BYTEA NOT NULL,
    private_key_type INT NOT NULL,
    registration JSONB NOT NULL,

    created_at TIMESTAMPTZ NOT NULL
);


CREATE TABLE certificate (
    id SERIAL PRIMARY KEY,
    acme_user_id INT NOT NULL,
    domain TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    stable_url TEXT NOT NULL,
    private_key BYTEA NOT NULL,
    private_key_type INT NOT NULL,
    certificate BYTEA[] NOT NULL,
    issuer_certificate BYTEA NOT NULL,
    csr BYTEA NOT NULL,

    status INT NOT NULL,
    failures INT NOT NULL,

    auto_refresh BOOLEAN NOT NULL,
    refresh_before_days INT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT fk_acme_user_id
    FOREIGN KEY(acme_user_id)
    REFERENCES acme_user(id)
);

CREATE INDEX certificate_domain ON certificate USING btree(domain);
CREATE INDEX certificate_expires_at ON certificate USING btree(expires_at) WHERE auto_refresh IS TRUE;
CREATE INDEX certificate_status ON certificate USING hash(status);


-- +goose Down
DROP INDEX certificate_domain;
DROP INDEX certificate_expires_at;
DROP INDEX certificate_status;

DROP TABLE certificate;
DROP TABLE acme_user;
