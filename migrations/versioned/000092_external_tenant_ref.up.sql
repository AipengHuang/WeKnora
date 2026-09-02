ALTER TABLE tenants ADD COLUMN external_ref UUID;
ALTER TABLE tenants ADD CONSTRAINT tenants_external_ref_key UNIQUE (external_ref);
