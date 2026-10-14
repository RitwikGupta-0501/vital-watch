import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { fetchApi } from '../../lib/api/client';
import type { DoctorSchedule, DoctorScheduleInput } from '../../lib/api/types';
import {
  Clock,
  CheckCircle2,
  AlertCircle,
  Trash2,
  Save,
} from 'lucide-react';

const DAYS_OF_WEEK = [
  'Sunday',
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
];

const SLOT_DURATIONS = [15, 20, 30, 45, 60];

const COMMON_TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'Europe/London',
  'Europe/Paris',
  'Asia/Kolkata',
  'Asia/Tokyo',
  'Australia/Sydney',
];

interface DayState {
  day_of_week: number;
  is_active: boolean;
  start_time: string;
  end_time: string;
  slot_duration: number;
  timezone: string;
}

export function DoctorScheduleSettings() {
  const queryClient = useQueryClient();
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // Initialize 7 days state
  const defaultTz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const [scheduleState, setScheduleState] = useState<DayState[]>(() =>
    Array.from({ length: 7 }, (_, i) => ({
      day_of_week: i,
      // Default Mon-Fri on, Sat-Sun off
      is_active: i >= 1 && i <= 5,
      start_time: '09:00',
      end_time: '17:00',
      slot_duration: 30,
      timezone: defaultTz,
    }))
  );

  // Fetch configured doctor schedules
  const { data, isLoading } = useQuery<{ data: DoctorSchedule[] }>({
    queryKey: ['doctor-schedules'],
    queryFn: () => fetchApi<{ data: DoctorSchedule[] }>('/api/doctor/schedules'),
  });

  // Populate from server when loaded
  useEffect(() => {
    const list = Array.isArray(data) ? data : (data as { data?: DoctorSchedule[] })?.data;
    if (list && list.length > 0) {
      setScheduleState((prev) =>
        prev.map((day) => {
          const match = list.find((s) => s.day_of_week === day.day_of_week);
          if (match) {
            return {
              day_of_week: match.day_of_week,
              is_active: match.is_active,
              start_time: match.start_time.slice(0, 5),
              end_time: match.end_time.slice(0, 5),
              slot_duration: match.slot_duration,
              timezone: match.timezone || defaultTz,
            };
          }
          return day;
        })
      );
    }
  }, [data, defaultTz]);

  // Save schedules mutation
  const saveMutation = useMutation({
    mutationFn: async (inputs: DoctorScheduleInput[]) => {
      setFeedback(null);
      return fetchApi<{ data: DoctorSchedule[] }>('/api/doctor/schedules', {
        method: 'PUT',
        body: JSON.stringify(inputs),
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['doctor-schedules'] });
      setFeedback({ type: 'success', message: 'Office hours and slot durations saved successfully.' });
    },
    onError: (err: { message?: string; data?: { error?: string } }) => {
      const msg = err.data?.error || err.message || 'Failed to save schedule';
      setFeedback({ type: 'error', message: msg });
    },
  });

  // Delete day schedule mutation
  const deleteMutation = useMutation({
    mutationFn: async (dayOfWeek: number) => {
      setFeedback(null);
      return fetchApi<{ message?: string }>(`/api/doctor/schedules/${dayOfWeek}`, {
        method: 'DELETE',
      });
    },
    onSuccess: (_, dayOfWeek) => {
      queryClient.invalidateQueries({ queryKey: ['doctor-schedules'] });
      setScheduleState((prev) =>
        prev.map((d) => (d.day_of_week === dayOfWeek ? { ...d, is_active: false } : d))
      );
      setFeedback({
        type: 'success',
        message: `${DAYS_OF_WEEK[dayOfWeek]} schedule removed from available booking.`,
      });
    },
    onError: (err: { message?: string }) => {
      setFeedback({ type: 'error', message: err.message || 'Failed to remove schedule for day' });
    },
  });

  const updateDay = (dayOfWeek: number, patch: Partial<DayState>) => {
    setScheduleState((prev) =>
      prev.map((d) => (d.day_of_week === dayOfWeek ? { ...d, ...patch } : d))
    );
  };

  const calculateSlots = (day: DayState): number => {
    if (!day.is_active) return 0;
    try {
      const [sh, sm] = day.start_time.split(':').map(Number);
      const [eh, em] = day.end_time.split(':').map(Number);
      const startMinutes = sh * 60 + sm;
      const endMinutes = eh * 60 + em;
      if (endMinutes <= startMinutes) return 0;
      return Math.floor((endMinutes - startMinutes) / day.slot_duration);
    } catch {
      return 0;
    }
  };

  const handleSave = () => {
    setFeedback(null);

    // Filter only active days or send all configured
    const activeDays = scheduleState.filter((d) => d.is_active);

    if (activeDays.length === 0) {
      setFeedback({
        type: 'error',
        message: 'At least one day must be configured and marked active for patient booking.',
      });
      return;
    }

    // Validate times
    for (const d of activeDays) {
      const slots = calculateSlots(d);
      if (slots <= 0) {
        setFeedback({
          type: 'error',
          message: `${DAYS_OF_WEEK[d.day_of_week]}: End time must be after start time and provide at least one slot duration (${d.slot_duration}m).`,
        });
        return;
      }
    }

    const payload: DoctorScheduleInput[] = activeDays.map((d) => ({
      day_of_week: d.day_of_week,
      start_time: d.start_time,
      end_time: d.end_time,
      slot_duration: d.slot_duration,
      timezone: d.timezone,
      is_active: true,
    }));

    saveMutation.mutate(payload);
  };

  return (
    <div className="space-y-6 max-w-5xl mx-auto">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-200">
        <div>
          <span className="font-mono text-xs uppercase tracking-wider text-slate-500 font-semibold">
            Availability & Slots • Screen C12
          </span>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900 mt-1">
            Office Hours & Consultation Grid
          </h1>
          <p className="text-sm text-slate-500 mt-0.5">
            Configure weekly recurring consultation windows, slot durations, and clinic timezones
          </p>
        </div>

        <button
          onClick={handleSave}
          disabled={saveMutation.isPending}
          className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 rounded-md transition-colors shadow-2xs disabled:opacity-50 cursor-pointer"
        >
          <Save className="w-4 h-4" />
          {saveMutation.isPending ? 'Saving Schedule...' : 'Save Weekly Schedule'}
        </button>
      </div>

      {feedback && (
        <div
          className={`p-4 rounded-lg border text-xs flex items-center justify-between ${
            feedback.type === 'success'
              ? 'bg-emerald-50 border-emerald-200 text-emerald-800'
              : 'bg-rose-50 border-rose-200 text-rose-800'
          }`}
        >
          <div className="flex items-center gap-2">
            {feedback.type === 'success' ? (
              <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0" />
            ) : (
              <AlertCircle className="w-4 h-4 text-rose-600 shrink-0" />
            )}
            <span>{feedback.message}</span>
          </div>
          <button
            onClick={() => setFeedback(null)}
            className="text-[11px] underline font-medium ml-4 hover:opacity-80"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Schedule Configuration Card */}
      <div className="bg-white border border-slate-200 rounded-lg shadow-2xs overflow-hidden">
        {isLoading ? (
          <div className="p-8 space-y-4">
            {[1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="h-16 bg-slate-100 rounded animate-pulse" />
            ))}
          </div>
        ) : (
          <div className="divide-y divide-slate-100">
            {scheduleState.map((day) => {
              const slotsCount = calculateSlots(day);
              const dayName = DAYS_OF_WEEK[day.day_of_week];

              return (
                <div
                  key={day.day_of_week}
                  className={`p-4.5 transition-colors flex flex-col lg:flex-row lg:items-center justify-between gap-4 ${
                    day.is_active ? 'bg-white' : 'bg-slate-50/50 opacity-60'
                  }`}
                >
                  {/* Day Toggle & Name */}
                  <div className="flex items-center gap-3.5 min-w-[170px]">
                    <input
                      type="checkbox"
                      checked={day.is_active}
                      onChange={(e) => updateDay(day.day_of_week, { is_active: e.target.checked })}
                      className="w-4 h-4 rounded text-slate-900 border-slate-300 focus:ring-slate-400 cursor-pointer"
                    />
                    <div>
                      <div className="text-sm font-bold text-slate-900">{dayName}</div>
                      <span
                        className={`text-[11px] font-mono uppercase tracking-wider ${
                          day.is_active ? 'text-emerald-700 font-semibold' : 'text-slate-400'
                        }`}
                      >
                        {day.is_active ? 'Active Office Day' : 'Day Off'}
                      </span>
                    </div>
                  </div>

                  {/* Form Controls */}
                  {day.is_active ? (
                    <div className="flex-1 flex flex-wrap items-center gap-3 text-xs">
                      {/* Hours Window */}
                      <div className="flex items-center gap-1.5 font-mono">
                        <Clock className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                        <input
                          type="time"
                          value={day.start_time}
                          onChange={(e) => updateDay(day.day_of_week, { start_time: e.target.value })}
                          className="px-2 py-1 bg-slate-50 border border-slate-200 rounded text-slate-900 text-xs focus:bg-white focus:outline-none"
                        />
                        <span className="text-slate-400">to</span>
                        <input
                          type="time"
                          value={day.end_time}
                          onChange={(e) => updateDay(day.day_of_week, { end_time: e.target.value })}
                          className="px-2 py-1 bg-slate-50 border border-slate-200 rounded text-slate-900 text-xs focus:bg-white focus:outline-none"
                        />
                      </div>

                      {/* Slot Duration */}
                      <div className="flex items-center gap-1.5">
                        <label htmlFor={`duration-${day.day_of_week}`} className="text-[11px] font-medium text-slate-500">
                          Duration:
                        </label>
                        <select
                          id={`duration-${day.day_of_week}`}
                          value={day.slot_duration}
                          onChange={(e) =>
                            updateDay(day.day_of_week, { slot_duration: Number(e.target.value) })
                          }
                          className="px-2 py-1 bg-slate-50 border border-slate-200 rounded text-slate-900 text-xs focus:bg-white focus:outline-none font-mono"
                        >
                          {SLOT_DURATIONS.map((dur) => (
                            <option key={dur} value={dur}>
                              {dur} mins
                            </option>
                          ))}
                        </select>
                      </div>

                      {/* Timezone */}
                      <div className="flex items-center gap-1.5">
                        <label htmlFor={`timezone-${day.day_of_week}`} className="text-[11px] font-medium text-slate-500">
                          Timezone:
                        </label>
                        <select
                          id={`timezone-${day.day_of_week}`}
                          value={day.timezone}
                          onChange={(e) => updateDay(day.day_of_week, { timezone: e.target.value })}
                          className="px-2 py-1 bg-slate-50 border border-slate-200 rounded text-slate-700 text-xs focus:bg-white focus:outline-none font-mono max-w-[150px] truncate"
                        >
                          {COMMON_TIMEZONES.map((tz) => (
                            <option key={tz} value={tz}>
                              {tz}
                            </option>
                          ))}
                        </select>
                      </div>
                    </div>
                  ) : (
                    <div className="flex-1 text-xs text-slate-400 italic">
                      No office hours configured. Toggle checkbox to enable booking for this day.
                    </div>
                  )}

                  {/* Capacity & Remove Action */}
                  <div className="flex items-center gap-3 shrink-0 justify-between lg:justify-end">
                    {day.is_active && (
                      <span className="text-xs font-mono font-medium text-slate-600 bg-slate-100 px-2 py-1 rounded border border-slate-200">
                        {slotsCount} slots available
                      </span>
                    )}

                    {day.is_active && (
                      <button
                        type="button"
                        onClick={() => deleteMutation.mutate(day.day_of_week)}
                        className="p-1.5 text-slate-400 hover:text-rose-600 rounded transition-colors cursor-pointer"
                        title={`Clear ${dayName} office hours`}
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
