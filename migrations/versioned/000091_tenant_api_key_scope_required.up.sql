DO $$ BEGIN RAISE NOTICE '[Migration 000091] Requiring explicit API key scope'; END $$;

ALTER TABLE tenant_api_keys
    ALTER COLUMN scope_type DROP DEFAULT;

DO $$ BEGIN RAISE NOTICE '[Migration 000091] Explicit API key scope required'; END $$;
