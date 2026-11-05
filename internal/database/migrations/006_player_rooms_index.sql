CREATE INDEX room_seats_player_rooms_idx ON room_seats (player_id, room_id)
    WHERE player_id IS NOT NULL;
