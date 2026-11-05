ALTER TABLE rooms
    ADD COLUMN game_revision integer NOT NULL DEFAULT 0,
    ADD COLUMN game_state jsonb,
    ADD CONSTRAINT room_game_revision_matches_state CHECK (
        (game_revision = 0 AND game_state IS NULL) OR
        (game_revision > 0 AND game_state IS NOT NULL)
    );
