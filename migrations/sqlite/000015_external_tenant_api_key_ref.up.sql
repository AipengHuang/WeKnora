ALTER TABLE tenant_api_keys ADD COLUMN external_ref TEXT;
CREATE UNIQUE INDEX idx_tenant_api_keys_external_ref ON tenant_api_keys(external_ref);
