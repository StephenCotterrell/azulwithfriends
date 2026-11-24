ALTER TABLE rooms
    ADD COLUMN rematch_from_revision integer NOT NULL DEFAULT 0,
    ADD CONSTRAINT room_rematch_precedes_current_revision CHECK (
        rematch_from_revision = 0 OR (rematch_from_revision > 0 AND rematch_from_revision < game_revision)
    );
