ALTER TABLE tenant_api_keys ADD COLUMN external_ref UUID;
ALTER TABLE tenant_api_keys ADD CONSTRAINT tenant_api_keys_external_ref_key UNIQUE (external_ref);
