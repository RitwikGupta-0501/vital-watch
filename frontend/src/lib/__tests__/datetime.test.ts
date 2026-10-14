import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  toLocalDateString,
  formatDisplayDate,
  formatShortDate,
  formatTime,
  formatSlotRange,
  formatRelativeAppointmentTime,
  isWithinTelehealthWindow,
} from '../datetime';

describe('datetime utilities', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // Pin system time to 2026-10-14 12:00:00 UTC
    vi.setSystemTime(new Date('2026-10-14T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('toLocalDateString formats date consistently without UTC shifting', () => {
    const d = new Date(2026, 9, 14); // Oct 14, 2026
    expect(toLocalDateString(d)).toBe('2026-10-14');
  });

  it('formatDisplayDate formats valid ISO string into long readable format', () => {
    const formatted = formatDisplayDate('2026-10-14T14:30:00Z');
    expect(formatted).toContain('2026');
    expect(formatted).toContain('Oct');
  });

  it('formatShortDate formats short date for chips', () => {
    const formatted = formatShortDate('2026-10-14T14:30:00Z');
    expect(formatted).toContain('Oct');
    expect(formatted).toContain('2026');
  });

  it('formatTime formats valid time string', () => {
    const formatted = formatTime('2026-10-14T14:30:00Z', 'UTC');
    expect(formatted).toBe('02:30 PM');
  });

  it('formatSlotRange renders local time and clinic timezone', () => {
    const res = formatSlotRange(
      '2026-10-14T14:00:00Z',
      '2026-10-14T14:30:00Z',
      'America/New_York'
    );
    expect(res.local).toBeDefined();
    expect(res.clinic).toBeDefined();
    expect(res.clinicTzName).toBe('America/New_York');
  });

  it('formatRelativeAppointmentTime returns friendly relative intervals', () => {
    // 30 minutes in the future
    const in30m = formatRelativeAppointmentTime('2026-10-14T12:30:00Z');
    expect(in30m).toBe('In 30 mins');

    // 2 hours in the future
    const in2h = formatRelativeAppointmentTime('2026-10-14T14:00:00Z');
    expect(in2h).toBe('In 2 hrs');

    // Past date
    const past = formatRelativeAppointmentTime('2026-10-13T12:00:00Z');
    expect(past).toBe('Completed / In Progress');
  });

  describe('isWithinTelehealthWindow', () => {
    it('returns canJoin: false when current time is > 15m before start', () => {
      // Start is at 14:00, current time is 12:00 (2h before)
      const res = isWithinTelehealthWindow('2026-10-14T14:00:00Z', '2026-10-14T14:30:00Z');
      expect(res.canJoin).toBe(false);
      expect(res.message).toContain('15m before consult');
    });

    it('returns canJoin: true when current time is within 15m before start', () => {
      // Start is at 12:10 (current time is 12:00 -> 10m before)
      const res = isWithinTelehealthWindow('2026-10-14T12:10:00Z', '2026-10-14T12:40:00Z');
      expect(res.canJoin).toBe(true);
      expect(res.message).toContain('Room active');
    });

    it('returns canJoin: false when consultation has exceeded 24h after end', () => {
      // Appointment ended 30h ago
      const res = isWithinTelehealthWindow('2026-10-12T08:00:00Z', '2026-10-12T08:30:00Z');
      expect(res.canJoin).toBe(false);
      expect(res.message).toBe('Consultation access window expired');
    });
  });
});
