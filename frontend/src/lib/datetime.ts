/**
 * Clinical Date & Time Formatting Utilities
 * Adheres to VitalWatch dual-timezone display rules and telehealth temporal constraints.
 */

/**
 * Returns a YYYY-MM-DD string for a local Date object without UTC date shifting.
 */
export function toLocalDateString(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

/**
 * Formats an ISO string into an editorial long date (e.g. "Wednesday, October 14, 2026").
 */
export function formatDisplayDate(isoString: string): string {
  try {
    const d = new Date(isoString);
    if (isNaN(d.getTime())) return isoString;
    return new Intl.DateTimeFormat('en-US', {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    }).format(d);
  } catch {
    return isoString;
  }
}

/**
 * Formats a short date for tables and chips (e.g. "Oct 14, 2026").
 */
export function formatShortDate(isoString: string): string {
  try {
    const d = new Date(isoString);
    if (isNaN(d.getTime())) return isoString;
    return new Intl.DateTimeFormat('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    }).format(d);
  } catch {
    return isoString;
  }
}

/**
 * Formats time in a specific timezone or user's local timezone.
 */
export function formatTime(isoString: string, timeZone?: string): string {
  try {
    const d = new Date(isoString);
    if (isNaN(d.getTime())) return isoString;
    return new Intl.DateTimeFormat('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      hour12: true,
      timeZone: timeZone || undefined,
    }).format(d);
  } catch {
    return isoString;
  }
}

/**
 * Formats a consultation slot range with both patient local time and clinic timezone.
 */
export function formatSlotRange(
  startIso: string,
  endIso: string,
  clinicTimezone?: string
): { local: string; clinic?: string; clinicTzName?: string } {
  try {
    const start = new Date(startIso);
    const end = new Date(endIso);
    if (isNaN(start.getTime()) || isNaN(end.getTime())) {
      return { local: `${startIso} - ${endIso}` };
    }

    const localStart = new Intl.DateTimeFormat('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    }).format(start);

    const localEnd = new Intl.DateTimeFormat('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    }).format(end);

    const local = `${localStart} – ${localEnd}`;

    // If clinic timezone differs, format clinic time range
    if (clinicTimezone && clinicTimezone.trim() !== '') {
      try {
        const clinicStart = new Intl.DateTimeFormat('en-US', {
          hour: 'numeric',
          minute: '2-digit',
          hour12: true,
          timeZone: clinicTimezone,
        }).format(start);

        const clinicEnd = new Intl.DateTimeFormat('en-US', {
          hour: 'numeric',
          minute: '2-digit',
          hour12: true,
          timeZone: clinicTimezone,
          timeZoneName: 'short',
        }).format(end);

        return {
          local,
          clinic: `${clinicStart} – ${clinicEnd}`,
          clinicTzName: clinicTimezone,
        };
      } catch {
        // Fallback if timezone string is invalid
      }
    }

    return { local };
  } catch {
    return { local: `${startIso} – ${endIso}` };
  }
}

/**
 * Returns a human-friendly relative time for an upcoming appointment.
 */
export function formatRelativeAppointmentTime(startIso: string): string {
  try {
    const start = new Date(startIso);
    const now = new Date();
    const diffMs = start.getTime() - now.getTime();

    if (diffMs < 0) {
      return 'Completed / In Progress';
    }

    const diffMinutes = Math.floor(diffMs / (1000 * 60));
    const diffHours = Math.floor(diffMinutes / 60);
    const diffDays = Math.floor(diffHours / 24);

    if (diffMinutes < 60) {
      return `In ${diffMinutes} min${diffMinutes === 1 ? '' : 's'}`;
    }
    if (diffHours < 24) {
      return `In ${diffHours} hr${diffHours === 1 ? '' : 's'}`;
    }
    if (diffDays === 1) {
      return 'Tomorrow';
    }
    return `In ${diffDays} days`;
  } catch {
    return '';
  }
}

/**
 * Checks whether the current time falls inside the 15-minute pre-appointment
 * telehealth access window (start - 15m to end + 24h).
 */
export function isWithinTelehealthWindow(
  startIso: string,
  endIso?: string
): { canJoin: boolean; message: string } {
  try {
    const start = new Date(startIso);
    const end = endIso ? new Date(endIso) : new Date(start.getTime() + 30 * 60 * 1000);
    const now = new Date();

    const openTime = new Date(start.getTime() - 15 * 60 * 1000);
    const closeTime = new Date(end.getTime() + 24 * 60 * 60 * 1000);

    if (now < openTime) {
      const formattedOpen = formatTime(openTime.toISOString());
      return {
        canJoin: false,
        message: `Video room opens at ${formattedOpen} (15m before consult)`,
      };
    }

    if (now > closeTime) {
      return {
        canJoin: false,
        message: 'Consultation access window expired',
      };
    }

    return {
      canJoin: true,
      message: 'Room active — join available',
    };
  } catch {
    return { canJoin: false, message: 'Invalid consultation schedule' };
  }
}
