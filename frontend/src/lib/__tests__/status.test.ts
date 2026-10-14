import { describe, it, expect } from 'vitest';
import {
  APPOINTMENT_STATUS_MAP,
  APPOINTMENT_TYPE_MAP,
  PRESCRIPTION_STATUS_MAP,
  PRESCRIPTION_SOURCE_MAP,
  MEDICATION_LOG_STATUS_MAP,
  MEDICATION_TIME_OF_DAY_MAP,
  SAFETY_SEVERITY_MAP,
  USER_ROLE_MAP,
  type StatusMeta,
} from '../status';
import type {
  AppointmentStatus,
  AppointmentType,
  PrescriptionStatus,
  PrescriptionSource,
  MedicationLogStatus,
  MedicationTimeOfDay,
  SafetySeverity,
  UserRole,
} from '../api/types';

function assertAllKeysMapped<K extends string>(
  keys: K[],
  map: Record<K, StatusMeta>
) {
  for (const key of keys) {
    const meta = map[key];
    expect(meta).toBeDefined();
    expect(typeof meta.label).toBe('string');
    expect(meta.label.trim().length).toBeGreaterThan(0);
    expect(['neutral', 'info', 'success', 'warning', 'critical']).toContain(meta.tone);
    expect(meta.icon).toBeDefined();
  }
}

describe('Status Vocabulary Module (WCAG AA Compliance)', () => {
  it('maps all AppointmentStatus enums', () => {
    const statuses: AppointmentStatus[] = ['upcoming', 'completed', 'cancelled'];
    assertAllKeysMapped(statuses, APPOINTMENT_STATUS_MAP);
  });

  it('maps all AppointmentType enums', () => {
    const types: AppointmentType[] = ['in_person', 'virtual'];
    assertAllKeysMapped(types, APPOINTMENT_TYPE_MAP);
  });

  it('maps all PrescriptionStatus enums', () => {
    const statuses: PrescriptionStatus[] = ['pending_ocr', 'needs_review', 'approved', 'rejected'];
    assertAllKeysMapped(statuses, PRESCRIPTION_STATUS_MAP);
  });

  it('maps all PrescriptionSource enums', () => {
    const sources: PrescriptionSource[] = ['uploaded', 'digital'];
    assertAllKeysMapped(sources, PRESCRIPTION_SOURCE_MAP);
  });

  it('maps all MedicationLogStatus enums', () => {
    const statuses: MedicationLogStatus[] = ['pending', 'taken', 'skipped'];
    assertAllKeysMapped(statuses, MEDICATION_LOG_STATUS_MAP);
  });

  it('maps all MedicationTimeOfDay enums', () => {
    const times: MedicationTimeOfDay[] = ['morning', 'afternoon', 'evening', 'bedtime', 'as_needed'];
    assertAllKeysMapped(times, MEDICATION_TIME_OF_DAY_MAP);
  });

  it('maps all SafetySeverity enums', () => {
    const severities: SafetySeverity[] = ['HIGH', 'MODERATE', 'LOW'];
    assertAllKeysMapped(severities, SAFETY_SEVERITY_MAP);
  });

  it('maps all UserRole enums', () => {
    const roles: UserRole[] = ['patient', 'doctor', 'admin', 'tenant_admin', 'platform_admin'];
    assertAllKeysMapped(roles, USER_ROLE_MAP);
  });
});
