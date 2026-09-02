ALTER TABLE tenants ADD COLUMN external_ref TEXT;
CREATE UNIQUE INDEX idx_tenants_external_ref ON tenants(external_ref);
