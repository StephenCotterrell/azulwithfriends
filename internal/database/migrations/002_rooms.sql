-- An empty name means the player uses their room's seat label.
ALTER TABLE players
    DROP CONSTRAINT players_display_name_check,
    ADD CONSTRAINT players_display_name_check CHECK (char_length(display_name) <= 24);

CREATE TABLE rooms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE room_seats (
    room_id uuid NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    number smallint NOT NULL CHECK (number BETWEEN 1 AND 2),
    player_id uuid NOT NULL REFERENCES players(id),
    PRIMARY KEY (room_id, number),
    UNIQUE (room_id, player_id)
);
