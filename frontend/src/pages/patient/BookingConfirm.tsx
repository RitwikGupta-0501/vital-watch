import { useState, useMemo } from 'react';
import { useParams, useSearchParams, useNavigate, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { fetchApi, ApiError } from '../../lib/api/client';
import type {
  Doctor,
  PaginatedResponse,
  Appointment,
  AppointmentType,
  CreateAppointmentRequest,
  CreateAppointmentResponse,
} from '../../lib/api/types';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import {
  formatDisplayDate,
  formatSlotRange,
} from '../../lib/datetime';
import { cn } from '../../lib/utils';
import {
  Calendar,
  Clock,
  ArrowLeft,
  Building2,
  Video,
  CheckCircle2,
  AlertTriangle,
  Stethoscope,
  ShieldCheck,
} from 'lucide-react';

export function BookingConfirm() {
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const startParam = searchParams.get('start');
  const endParam = searchParams.get('end');

  const [appointmentType, setAppointmentType] = useState<AppointmentType>('virtual');
  const [conflictError, setConflictError] = useState<string | null>(null);

  // Validate presence of required params
  const hasValidParams = !!(id && startParam && endParam);

  // Fetch Doctor Profile
  const { data: doctorsData } = useQuery({
    queryKey: ['doctors-list'],
    queryFn: () => fetchApi<PaginatedResponse<Doctor>>('/api/doctors?limit=50&offset=0'),
    enabled: hasValidParams,
  });

  const doctor = doctorsData?.data?.find((d) => d.id === id);

  // Fetch Patient's Existing Appointments for Client-Side Pre-Conflict Check
  const { data: existingApptsData } = useQuery({
    queryKey: ['patient-appointments-all'],
    queryFn: () =>
      fetchApi<PaginatedResponse<Appointment>>('/api/patient/appointments?limit=50&offset=0'),
    enabled: hasValidParams,
  });

  const existingAppointments = existingApptsData?.data || [];

  // Check if patient already holds an overlapping appointment
  const hasClientSideOverlap = useMemo(() => {
    if (!startParam || !endParam) return false;
    const reqStart = new Date(startParam).getTime();
    const reqEnd = new Date(endParam).getTime();

    return existingAppointments.some((appt) => {
      if (appt.status === 'cancelled') return false;
      const apptStart = new Date(appt.start_time).getTime();
      const apptEnd = new Date(appt.end_time).getTime();
      return reqStart < apptEnd && reqEnd > apptStart;
    });
  }, [existingAppointments, startParam, endParam]);

  // Appointment Creation Mutation
  const bookMutation = useMutation({
    mutationFn: (payload: CreateAppointmentRequest) =>
      fetchApi<CreateAppointmentResponse>('/api/appointments', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    onSuccess: () => {
      // Invalidate appointment ledgers and doctor slot grids
      queryClient.invalidateQueries({ queryKey: ['patient-appointments'] });
      queryClient.invalidateQueries({ queryKey: ['doctor-available-slots'] });
      navigate('/patient/appointments?booked=true');
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError && err.status === 409) {
        setConflictError(
          'This consultation slot was just reserved by another patient. Please select an alternate time.'
        );
      } else if (err instanceof Error) {
        setConflictError(err.message);
      } else {
        setConflictError('An unexpected conflict occurred while booking this appointment.');
      }
    },
  });

  if (!hasValidParams) {
    return (
      <div className="p-8 text-center space-y-4">
        <p className="text-sm text-slate-500">Missing slot parameters.</p>
        <Link to="/patient/doctors">
          <Button isPatientTheme size="sm">
            Return to Directory
          </Button>
        </Link>
      </div>
    );
  }

  const handleConfirmBooking = () => {
    if (!id || !startParam || !endParam) return;
    setConflictError(null);

    bookMutation.mutate({
      doctor_id: id,
      start_time: startParam,
      end_time: endParam,
      type: appointmentType,
    });
  };

  const slotTimes = formatSlotRange(startParam, endParam);

  return (
    <div className="max-w-2xl mx-auto space-y-8 animate-in fade-in duration-300">
      {/* Back Link */}
      <div>
        <Link
          to={`/patient/doctors/${id}`}
          className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-500 hover:text-slate-900 transition-colors"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          <span>Change Slot</span>
        </Link>
      </div>

      <div className="space-y-1">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-900">
          Confirm Consultation
        </h1>
        <p className="text-sm text-slate-500">
          Review your appointment details and select consultation modality.
        </p>
      </div>

      {/* 409 Double-Booking Conflict Alert */}
      {conflictError && (
        <div className="p-5 rounded-3xl bg-rose-50 border border-rose-200 space-y-3 animate-in shake duration-200">
          <div className="flex items-start gap-3">
            <AlertTriangle className="w-5 h-5 text-rose-600 shrink-0 mt-0.5" />
            <div>
              <h3 className="text-sm font-bold text-rose-950">Slot No Longer Available</h3>
              <p className="text-xs text-rose-700 mt-1 leading-relaxed">
                {conflictError}
              </p>
            </div>
          </div>
          <div className="pt-2">
            <Link to={`/patient/doctors/${id}`}>
              <Button isPatientTheme size="sm" variant="outline" className="border-rose-300 text-rose-800 hover:bg-rose-100">
                Pick Another Slot
              </Button>
            </Link>
          </div>
        </div>
      )}

      {/* Client-Side Pre-Conflict Alert */}
      {hasClientSideOverlap && (
        <div className="p-4 rounded-2xl bg-amber-50 border border-amber-200 flex items-start gap-3 text-amber-900">
          <AlertTriangle className="w-4 h-4 text-amber-600 shrink-0 mt-0.5" />
          <p className="text-xs leading-relaxed">
            <span className="font-semibold">Scheduling Conflict Detected:</span> You currently have another consultation scheduled during this time window. Booking this will result in a 409 conflict error.
          </p>
        </div>
      )}

      {/* Appointment Summary Card */}
      <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border space-y-6">
        <div className="flex items-start justify-between gap-4 border-b border-border pb-6">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-2xl bg-brand-50 text-brand-800 flex items-center justify-center font-bold text-base shrink-0">
              {doctor ? `${doctor.first_name[0]}${doctor.last_name[0]}` : 'MD'}
            </div>
            <div>
              <h2 className="text-base font-bold text-slate-900">
                {doctor ? `Dr. ${doctor.first_name} ${doctor.last_name}` : 'Clinical Specialist'}
              </h2>
              <p className="text-xs text-brand-700 font-medium flex items-center gap-1 mt-0.5">
                <Stethoscope className="w-3.5 h-3.5" />
                <span>{doctor?.specialty || 'General Practice'}</span>
              </p>
            </div>
          </div>
          <span className="font-mono text-xs px-2.5 py-1 rounded-full bg-slate-100 text-slate-700 border border-slate-200">
            Standard Visit
          </span>
        </div>

        {/* Date & Time Row */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
          <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200/60 space-y-1">
            <span className="text-slate-400 font-medium">Scheduled Date</span>
            <div className="flex items-center gap-2 text-slate-900 font-semibold text-sm">
              <Calendar className="w-4 h-4 text-slate-500" />
              <span>{formatDisplayDate(startParam)}</span>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200/60 space-y-1">
            <span className="text-slate-400 font-medium">Time Window</span>
            <div className="flex items-center gap-2 text-slate-900 font-semibold font-mono text-sm">
              <Clock className="w-4 h-4 text-slate-500" />
              <span>{slotTimes.local}</span>
            </div>
          </div>
        </div>

        {/* Modality Selection */}
        <div className="space-y-3 pt-2">
          <label className="text-xs font-bold uppercase tracking-wider text-slate-500">
            Consultation Modality
          </label>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {/* Virtual Telehealth Option */}
            <button
              type="button"
              onClick={() => setAppointmentType('virtual')}
              className={cn(
                'p-4 rounded-2xl border text-left transition-all flex flex-col justify-between cursor-pointer space-y-2',
                appointmentType === 'virtual'
                  ? 'bg-sky-50/70 border-sky-400 ring-2 ring-sky-400/20 text-sky-950'
                  : 'bg-white border-border text-slate-700 hover:bg-slate-50'
              )}
            >
              <div className="flex items-center justify-between w-full">
                <div className="flex items-center gap-2">
                  <Video className="w-4 h-4 text-sky-700" />
                  <span className="text-sm font-bold">Virtual Telehealth</span>
                </div>
                {appointmentType === 'virtual' && (
                  <CheckCircle2 className="w-4 h-4 text-sky-700 shrink-0" />
                )}
              </div>
              <p className="text-[11px] text-slate-500 leading-relaxed">
                Encrypted video consultation accessible via the patient portal 15 minutes prior to appointment.
              </p>
            </button>

            {/* In-Person Option */}
            <button
              type="button"
              onClick={() => setAppointmentType('in_person')}
              className={cn(
                'p-4 rounded-2xl border text-left transition-all flex flex-col justify-between cursor-pointer space-y-2',
                appointmentType === 'in_person'
                  ? 'bg-emerald-50/70 border-emerald-400 ring-2 ring-emerald-400/20 text-emerald-950'
                  : 'bg-white border-border text-slate-700 hover:bg-slate-50'
              )}
            >
              <div className="flex items-center justify-between w-full">
                <div className="flex items-center gap-2">
                  <Building2 className="w-4 h-4 text-emerald-700" />
                  <span className="text-sm font-bold">In-Person Visit</span>
                </div>
                {appointmentType === 'in_person' && (
                  <CheckCircle2 className="w-4 h-4 text-emerald-700 shrink-0" />
                )}
              </div>
              <p className="text-[11px] text-slate-500 leading-relaxed">
                Physical consultation at the clinical facility. Please arrive 10 minutes before your slot.
              </p>
            </button>
          </div>
        </div>

        {/* Security / HIPAA Assurance Note */}
        <div className="p-4 rounded-2xl bg-slate-50 text-[11px] text-slate-500 flex items-center gap-2.5">
          <ShieldCheck className="w-4 h-4 text-brand-700 shrink-0" />
          <span>
            Protected health encounter. Scheduled appointments conform to HIPAA § 164.312 privacy requirements.
          </span>
        </div>

        {/* Submit Button */}
        <div className="pt-4 border-t border-border">
          <Button
            isPatientTheme
            size="lg"
            onClick={handleConfirmBooking}
            disabled={bookMutation.isPending || hasClientSideOverlap}
            className="w-full font-bold shadow-sm"
          >
            {bookMutation.isPending ? 'Confirming Appointment...' : 'Confirm & Reserve Slot'}
          </Button>
        </div>
      </Card>
    </div>
  );
}
