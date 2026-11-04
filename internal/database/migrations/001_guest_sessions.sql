CREATE TABLE players (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 24),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);

CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);
