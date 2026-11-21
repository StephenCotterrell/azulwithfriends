-- A null level keeps existing bots (and seats created by older servers) on Competitive.
ALTER TABLE room_seats
    ADD COLUMN bot_level text,
    ADD CONSTRAINT room_seat_bot_level CHECK (
        bot_level IS NULL OR (is_bot AND bot_level IN (
            'noob', 'beginner', 'intermediate', 'competitive', 'tactical', 'advanced', 'master'
        ))
    );
