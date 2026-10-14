import { useState } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchApi } from '../../lib/api/client';
import type {
  Doctor,
  PaginatedResponse,
  DoctorAvailableSlotsResponse,
  TimeSlot,
} from '../../lib/api/types';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { EmptyState } from '../../components/ui/EmptyState';
import {
  toLocalDateString,
  formatDisplayDate,
  formatSlotRange,
} from '../../lib/datetime';
import { cn } from '../../lib/utils';
import {
  Calendar as CalendarIcon,
  Globe,
  ArrowLeft,
  ArrowRight,
  Stethoscope,
  Check,
} from 'lucide-react';

export function DoctorDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  // Selected date state (defaults to today in local time)
  const todayStr = toLocalDateString(new Date());
  const maxDate = new Date();
  maxDate.setFullYear(maxDate.getFullYear() + 1);
  const maxDateStr = toLocalDateString(maxDate);

  const [selectedDate, setSelectedDate] = useState(todayStr);
  const [selectedSlot, setSelectedSlot] = useState<TimeSlot | null>(null);

  // Fetch Doctor Profile
  const { data: doctorsData, isLoading: isLoadingDoc } = useQuery({
    queryKey: ['doctors-list'],
    queryFn: () => fetchApi<PaginatedResponse<Doctor>>('/api/doctors?limit=50&offset=0'),
  });

  const doctor = doctorsData?.data?.find((d) => d.id === id);

  // Fetch Available Slots for selected date
  const {
    data: slotData,
    isLoading: isLoadingSlots,
    error: slotError,
  } = useQuery({
    queryKey: ['doctor-available-slots', id, selectedDate],
    queryFn: () =>
      fetchApi<DoctorAvailableSlotsResponse>(
        `/api/doctors/${id}/available-slots?date=${selectedDate}`
      ),
    enabled: !!id,
  });

  const slots = slotData?.slots || [];
  const clinicTz = slotData?.timezone || 'UTC';
  const hasAvailableSlots = slots.some((s) => s.available);
  const isScheduleConfigured = slots.length > 0;

  // Generate 7-day strip (today + 6 days)
  const weekDays = Array.from({ length: 7 }).map((_, i) => {
    const d = new Date();
    d.setDate(d.getDate() + i);
    return {
      dateStr: toLocalDateString(d),
      dayName: i === 0 ? 'Today' : new Intl.DateTimeFormat('en-US', { weekday: 'short' }).format(d),
      dayNumber: d.getDate(),
      month: new Intl.DateTimeFormat('en-US', { month: 'short' }).format(d),
    };
  });

  const handleSlotSelect = (slot: TimeSlot) => {
    if (!slot.available) return;
    setSelectedSlot(slot);
  };

  const handleContinueToBooking = () => {
    if (!selectedSlot || !id) return;
    const startEnc = encodeURIComponent(selectedSlot.start_time);
    const endEnc = encodeURIComponent(selectedSlot.end_time);
    navigate(`/patient/doctors/${id}/book?start=${startEnc}&end=${endEnc}`);
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      {/* Back link */}
      <div>
        <Link
          to="/patient/doctors"
          className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-500 hover:text-slate-900 transition-colors"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          <span>Back to Clinicians</span>
        </Link>
      </div>

      {/* Doctor Header Banner */}
      <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border">
        {isLoadingDoc ? (
          <div className="animate-pulse space-y-3">
            <div className="w-16 h-16 bg-slate-100 rounded-2xl" />
            <div className="h-6 bg-slate-100 rounded w-1/4" />
            <div className="h-4 bg-slate-100 rounded w-1/3" />
          </div>
        ) : doctor ? (
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-6">
            <div className="flex items-center gap-4">
              <div className="w-16 h-16 rounded-2xl bg-brand-50 text-brand-800 flex items-center justify-center font-bold text-xl shrink-0">
                {doctor.first_name[0]}
                {doctor.last_name[0]}
              </div>
              <div>
                <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-slate-900">
                  Dr. {doctor.first_name} {doctor.last_name}
                </h1>
                <p className="text-xs sm:text-sm font-semibold text-brand-700 flex items-center gap-1.5 mt-0.5">
                  <Stethoscope className="w-4 h-4" />
                  <span>{doctor.specialty || 'General Practitioner'}</span>
                </p>
                <div className="flex flex-wrap items-center gap-3 mt-2 text-xs text-slate-500">
                  <span>
                    {(doctor.experience_years ?? doctor.experience)
                      ? `${doctor.experience_years ?? doctor.experience} years experience`
                      : 'Licensed Practitioner'}
                  </span>
                  <span>•</span>
                  <span className="flex items-center gap-1">
                    <Globe className="w-3.5 h-3.5 text-slate-400" />
                    <span className="font-mono text-[11px]">{clinicTz}</span>
                  </span>
                </div>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <span className="px-3 py-1 rounded-full text-xs font-medium bg-emerald-50 text-emerald-800 border border-emerald-200">
                Verified Provider
              </span>
            </div>
          </div>
        ) : (
          <p className="text-sm text-slate-500">Clinician details loaded.</p>
        )}
      </Card>

      {/* Booking Calendar & Slot Picker Section */}
      <div className="space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-bold tracking-tight text-slate-900">
              Select Consultation Date
            </h2>
            <p className="text-xs text-slate-500">
              Slots are generated according to the physician's active clinic hours.
            </p>
          </div>

          {/* Native Date Input Picker for deep dates */}
          <div className="flex items-center gap-2">
            <CalendarIcon className="w-4 h-4 text-slate-400" />
            <input
              type="date"
              value={selectedDate}
              min={todayStr}
              max={maxDateStr}
              onChange={(e) => {
                if (e.target.value) {
                  setSelectedDate(e.target.value);
                  setSelectedSlot(null);
                }
              }}
              className="text-xs font-mono font-medium bg-white border border-border rounded-xl px-3 py-2 text-slate-900 focus:outline-none focus:ring-2 focus:ring-brand-700/20"
            />
          </div>
        </div>

        {/* 7-Day Quick Strip */}
        <div className="grid grid-cols-4 sm:grid-cols-7 gap-2">
          {weekDays.map((item) => {
            const isSelected = selectedDate === item.dateStr;
            return (
              <button
                key={item.dateStr}
                type="button"
                onClick={() => {
                  setSelectedDate(item.dateStr);
                  setSelectedSlot(null);
                }}
                className={cn(
                  'flex flex-col items-center justify-center p-3 rounded-2xl border transition-all cursor-pointer',
                  isSelected
                    ? 'bg-brand-700 text-white border-brand-700 shadow-sm'
                    : 'bg-white text-slate-700 border-border hover:bg-slate-50'
                )}
              >
                <span
                  className={cn(
                    'text-[10px] font-semibold uppercase tracking-wider',
                    isSelected ? 'text-brand-100' : 'text-slate-400'
                  )}
                >
                  {item.dayName}
                </span>
                <span className="text-base font-bold font-mono mt-0.5">
                  {item.dayNumber}
                </span>
                <span
                  className={cn(
                    'text-[10px]',
                    isSelected ? 'text-brand-200' : 'text-slate-400'
                  )}
                >
                  {item.month}
                </span>
              </button>
            );
          })}
        </div>

        {/* Available Time Slots Grid */}
        <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border space-y-6">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-border pb-4">
            <div>
              <h3 className="text-sm font-semibold text-slate-900">
                Available Slots for {formatDisplayDate(selectedDate)}
              </h3>
              <p className="text-xs text-slate-500 mt-0.5">
                Displays your local time alongside clinic timezone ({clinicTz}).
              </p>
            </div>
            {slotData?.slot_duration && (
              <span className="font-mono text-xs text-slate-500 px-2.5 py-1 rounded-full bg-slate-100 border border-slate-200 self-start sm:self-auto">
                {slotData.slot_duration} min consultations
              </span>
            )}
          </div>

          {isLoadingSlots ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3 py-6">
              {Array.from({ length: 8 }).map((_, i) => (
                <div key={i} className="h-16 bg-slate-100 rounded-xl animate-pulse" />
              ))}
            </div>
          ) : slotError ? (
            <EmptyState
              variant="error"
              title="Unable to Load Slots"
              description="Failed to fetch available consultation times for this date."
              isPatientTheme
            />
          ) : !isScheduleConfigured ? (
            <EmptyState
              variant="not-configured"
              title="Schedule Not Configured"
              description="This clinician has not configured consultation hours for this day of the week. Please select another date."
              isPatientTheme
            />
          ) : !hasAvailableSlots ? (
            <EmptyState
              variant="no-records"
              title="Fully Booked Today"
              description="All appointment slots for this date are already reserved. Please select another date on the calendar above."
              isPatientTheme
            />
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
              {slots.map((slot) => {
                const times = formatSlotRange(slot.start_time, slot.end_time, clinicTz);
                const isSelected =
                  selectedSlot?.start_time === slot.start_time &&
                  selectedSlot?.end_time === slot.end_time;

                return (
                  <button
                    key={slot.start_time}
                    type="button"
                    disabled={!slot.available}
                    onClick={() => handleSlotSelect(slot)}
                    className={cn(
                      'p-3.5 rounded-2xl border text-left transition-all flex flex-col justify-between cursor-pointer min-h-[72px]',
                      !slot.available
                        ? 'bg-slate-50/70 border-slate-200/50 text-slate-300 cursor-not-allowed opacity-60'
                        : isSelected
                        ? 'bg-brand-50 border-brand-700 text-brand-900 shadow-xs ring-2 ring-brand-700/20'
                        : 'bg-white border-border text-slate-800 hover:border-brand-400 hover:bg-slate-50/70'
                    )}
                  >
                    <div className="flex items-center justify-between w-full">
                      <span className="font-mono text-sm font-semibold tracking-tight">
                        {times.local}
                      </span>
                      {isSelected && (
                        <Check className="w-4 h-4 text-brand-700 shrink-0" />
                      )}
                    </div>
                    {times.clinic && (
                      <span className="font-mono text-[10px] text-slate-400 mt-1">
                        Clinic: {times.clinic}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
          )}

          {/* Sticky Bottom Bar / Confirmation Trigger */}
          {selectedSlot && (
            <div className="pt-6 border-t border-border flex flex-col sm:flex-row sm:items-center justify-between gap-4 animate-in slide-in-from-bottom-2">
              <div>
                <p className="text-xs text-slate-500 font-medium">Selected Slot</p>
                <p className="text-sm font-bold font-mono text-slate-900 mt-0.5">
                  {formatDisplayDate(selectedSlot.start_time)} •{' '}
                  {formatSlotRange(selectedSlot.start_time, selectedSlot.end_time).local}
                </p>
              </div>

              <Button
                isPatientTheme
                size="lg"
                onClick={handleContinueToBooking}
                className="gap-2 shadow-sm font-semibold"
              >
                <span>Continue to Booking</span>
                <ArrowRight className="w-4 h-4" />
              </Button>
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}
