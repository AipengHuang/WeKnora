ALTER TABLE tenant_api_keys DROP CONSTRAINT tenant_api_keys_external_ref_key;
ALTER TABLE tenant_api_keys DROP COLUMN external_ref;
