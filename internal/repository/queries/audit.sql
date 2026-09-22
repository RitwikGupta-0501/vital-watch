-- name: CreateAuditLog :one
INSERT INTO phi_audit_logs (
    user_id, user_role, action, resource_type, resource_id, patient_id, 
    ip_address, user_agent, request_id, status_code, metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, user_id, user_role, action, resource_type, resource_id, patient_id, 
    ip_address, user_agent, request_id, status_code, metadata, created_at;

-- name: GetAuditLogsByPatientID :many
SELECT id, user_id, user_role, action, resource_type, resource_id, patient_id, 
    ip_address, user_agent, request_id, status_code, metadata, created_at
FROM phi_audit_logs
WHERE patient_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetAuditLogs :many
SELECT id, user_id, user_role, action, resource_type, resource_id, patient_id, 
    ip_address, user_agent, request_id, status_code, metadata, created_at
FROM phi_audit_logs
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
