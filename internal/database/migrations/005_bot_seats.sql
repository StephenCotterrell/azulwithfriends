ALTER TABLE room_seats
    ALTER COLUMN player_id DROP NOT NULL,
    ADD COLUMN is_bot boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT room_seat_occupant CHECK (
        (is_bot AND number = 2 AND player_id IS NULL) OR
        (NOT is_bot AND player_id IS NOT NULL)
    );
