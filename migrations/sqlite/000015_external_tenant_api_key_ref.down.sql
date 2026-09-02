DROP INDEX idx_tenant_api_keys_external_ref;
ALTER TABLE tenant_api_keys DROP COLUMN external_ref;
