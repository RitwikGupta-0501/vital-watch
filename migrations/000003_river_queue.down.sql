-- Clean, idempotent teardown for River task queue database objects
DROP TRIGGER IF EXISTS river_notify ON river_job;
DROP FUNCTION IF EXISTS river_job_notify() CASCADE;

DROP TABLE IF EXISTS river_notification CASCADE;
DROP TABLE IF EXISTS river_queue CASCADE;
DROP TABLE IF EXISTS river_leader CASCADE;
DROP TABLE IF EXISTS river_job CASCADE;
DROP TYPE IF EXISTS river_job_state CASCADE;

-- Client tracking tables if present
DROP TABLE IF EXISTS river_client_queue CASCADE;
DROP TABLE IF EXISTS river_client CASCADE;

-- River migration tracking table
DROP TABLE IF EXISTS river_migration CASCADE;
