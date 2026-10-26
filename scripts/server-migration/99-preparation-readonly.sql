-- The official entrypoint creates the empty database during its temporary server.
-- PERSIST_ONLY leaves that bootstrap writable, then protects the final server
-- and subsequent starts. Never run this against the source production server.
SET PERSIST_ONLY read_only = ON;
SET PERSIST_ONLY super_read_only = ON;
