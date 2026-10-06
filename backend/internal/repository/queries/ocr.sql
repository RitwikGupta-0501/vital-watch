-- name: CreateOCRProviderConfig :one
INSERT INTO ocr_provider_configs (
    tenant_id,
    provider_name,
    encrypted_key,
    nonce
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetOCRProviderConfigsForTenant :many
SELECT * FROM ocr_provider_configs
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: GetOCRProviderConfigByID :one
SELECT * FROM ocr_provider_configs
WHERE id = $1 AND tenant_id = $2 LIMIT 1;

-- name: DeleteOCRProviderConfig :exec
DELETE FROM ocr_provider_configs
WHERE id = $1 AND tenant_id = $2;

-- name: UpsertTenantSettings :one
INSERT INTO tenant_settings (
    tenant_id,
    ocr_fallback_chain,
    updated_at
) VALUES (
    $1, $2, now()
)
ON CONFLICT (tenant_id) DO UPDATE 
SET ocr_fallback_chain = EXCLUDED.ocr_fallback_chain,
    updated_at = now()
RETURNING *;

-- name: GetTenantSettings :one
SELECT * FROM tenant_settings
WHERE tenant_id = $1 LIMIT 1;

-- name: GetOCRProviderConfigsByIDs :many
SELECT * FROM ocr_provider_configs
WHERE id = ANY($1::uuid[]) AND tenant_id = $2;
