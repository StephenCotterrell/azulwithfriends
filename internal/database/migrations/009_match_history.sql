-- One ordered batch of public positions per committed game action, including
-- any automatic bot drafts and round transitions in that transaction.
CREATE TABLE room_history (
    room_id uuid NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    revision integer NOT NULL CHECK (revision > 0),
    events jsonb NOT NULL CHECK (jsonb_typeof(events) = 'array' AND jsonb_array_length(events) > 0),
    PRIMARY KEY (room_id, revision)
);

-- The application and migration owner can be different roles. Preserve the
-- named roles and groups already granted these room operations, even when
-- the owner has no default grants for future tables.
DO $$
DECLARE
    permission record;
BEGIN
    FOR permission IN
        SELECT DISTINCT grant_entry.privilege_type, grantee.rolname
        FROM pg_class AS rooms_table
        CROSS JOIN LATERAL aclexplode(COALESCE(rooms_table.relacl, acldefault('r', rooms_table.relowner))) AS grant_entry
        JOIN pg_roles AS grantee ON grantee.oid = grant_entry.grantee
        WHERE rooms_table.oid = 'rooms'::regclass
          AND grantee.rolname <> current_user
          AND grant_entry.privilege_type IN ('SELECT', 'INSERT', 'DELETE')
    LOOP
        EXECUTE format('GRANT %s ON TABLE room_history TO %I', permission.privilege_type, permission.rolname);
    END LOOP;
END;
$$;
