import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import { fetchApi } from '../../lib/api/client';
import type { Appointment, PaginatedResponse } from '../../lib/api/types';
import { APPOINTMENT_STATUS_MAP, APPOINTMENT_TYPE_MAP } from '../../lib/status';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Modal } from '../../components/ui/Modal';
import { EmptyState } from '../../components/ui/EmptyState';
import {
  formatDisplayDate,
  formatSlotRange,
  isWithinTelehealthWindow,
} from '../../lib/datetime';
import { cn } from '../../lib/utils';
import {
  Calendar,
  Clock,
  Video,
  Stethoscope,
  CheckCircle2,
} from 'lucide-react';

type Tab = 'upcoming' | 'past' | 'cancelled';

export function AppointmentsList() {
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const justBooked = searchParams.get('booked') === 'true';

  const [activeTab, setActiveTab] = useState<Tab>('upcoming');
  const [cancelModalAppointment, setCancelModalAppointment] = useState<Appointment | null>(null);
  const [cancelError, setCancelError] = useState<string | null>(null);

  // Fetch Patient Appointments
  const { data, isLoading, error } = useQuery({
    queryKey: ['patient-appointments'],
    queryFn: () =>
      fetchApi<PaginatedResponse<Appointment>>('/api/patient/appointments?limit=50&offset=0'),
  });

  const appointments = data?.data || [];
  const now = new Date().getTime();

  // Filter appointments per tab
  const upcomingAppointments = appointments
    .filter(
      (a) =>
        a.status === 'upcoming' &&
        new Date(a.end_time || a.start_time).getTime() >= now
    )
    .sort((a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime());

  const pastAppointments = appointments
    .filter(
      (a) =>
        a.status === 'completed' ||
        (a.status === 'upcoming' && new Date(a.end_time || a.start_time).getTime() < now)
    )
    .sort((a, b) => new Date(b.start_time).getTime() - new Date(a.start_time).getTime());

  const cancelledAppointments = appointments
    .filter((a) => a.status === 'cancelled')
    .sort((a, b) => new Date(b.start_time).getTime() - new Date(a.start_time).getTime());

  const displayedAppointments =
    activeTab === 'upcoming'
      ? upcomingAppointments
      : activeTab === 'past'
      ? pastAppointments
      : cancelledAppointments;

  // Cancellation Mutation
  const cancelMutation = useMutation({
    mutationFn: (appointmentId: string) =>
      fetchApi<{ message: string }>(`/api/appointments/${appointmentId}/cancel`, {
        method: 'PATCH',
      }),
    onSuccess: () => {
      setCancelModalAppointment(null);
      setCancelError(null);
      queryClient.invalidateQueries({ queryKey: ['patient-appointments'] });
    },
    onError: (err: unknown) => {
      if (err instanceof Error) {
        setCancelError(err.message);
      } else {
        setCancelError('Failed to cancel appointment. Please try again.');
      }
    },
  });

  const handleConfirmCancel = () => {
    if (!cancelModalAppointment) return;
    cancelMutation.mutate(cancelModalAppointment.id);
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      {/* Booking Confirmation Toast */}
      {justBooked && (
        <div className="p-4 rounded-2xl bg-emerald-50 border border-emerald-200 flex items-center justify-between gap-3 text-emerald-950 animate-in slide-in-from-top-2">
          <div className="flex items-center gap-2.5">
            <CheckCircle2 className="w-5 h-5 text-emerald-600 shrink-0" />
            <p className="text-xs sm:text-sm font-semibold">
              Consultation confirmed! Your appointment has been successfully scheduled.
            </p>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-900">
            Consultation Ledger
          </h1>
          <p className="text-sm text-slate-500 mt-1">
            Manage upcoming consultations, join telehealth sessions, and inspect historical visits.
          </p>
        </div>

        <Link to="/patient/doctors">
          <Button isPatientTheme size="md" className="gap-2 shadow-sm">
            <Stethoscope className="w-4 h-4" />
            <span>Book New Visit</span>
          </Button>
        </Link>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-2 border-b border-border pb-3">
        {(['upcoming', 'past', 'cancelled'] as Tab[]).map((tab) => {
          const count =
            tab === 'upcoming'
              ? upcomingAppointments.length
              : tab === 'past'
              ? pastAppointments.length
              : cancelledAppointments.length;
          const label = tab === 'upcoming' ? 'Upcoming' : tab === 'past' ? 'Past Records' : 'Cancelled';

          return (
            <button
              key={tab}
              type="button"
              onClick={() => setActiveTab(tab)}
              className={cn(
                'px-4 py-2 rounded-xl text-xs sm:text-sm font-semibold transition-all flex items-center gap-2 cursor-pointer',
                activeTab === tab
                  ? 'bg-brand-50 text-brand-700 shadow-xs'
                  : 'text-slate-500 hover:text-slate-900 hover:bg-slate-50'
              )}
            >
              <span>{label}</span>
              <span
                className={cn(
                  'px-1.5 py-0.5 rounded-full text-[10px] font-mono',
                  activeTab === tab ? 'bg-brand-100 text-brand-800' : 'bg-slate-100 text-slate-600'
                )}
              >
                {count}
              </span>
            </button>
          );
        })}
      </div>

      {/* Appointment List Content */}
      {isLoading ? (
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Card key={i} className="rounded-3xl p-6 bg-white border border-border animate-pulse space-y-3">
              <div className="h-5 bg-slate-100 rounded w-1/4" />
              <div className="h-4 bg-slate-100 rounded w-1/2" />
            </Card>
          ))}
        </div>
      ) : error ? (
        <EmptyState
          variant="error"
          title="Unable to Load Appointments"
          description="Failed to retrieve consultation history. Please try refreshing."
          isPatientTheme
        />
      ) : displayedAppointments.length > 0 ? (
        <div className="space-y-4">
          {displayedAppointments.map((appt) => {
            const timeRange = formatSlotRange(appt.start_time, appt.end_time);
            const telehealthInfo = isWithinTelehealthWindow(appt.start_time, appt.end_time);

            return (
              <Card
                key={appt.id}
                className="rounded-3xl p-6 bg-white border border-border hover:border-slate-300 transition-all flex flex-col md:flex-row md:items-center justify-between gap-6"
              >
                <div className="space-y-3 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge meta={APPOINTMENT_STATUS_MAP[appt.status]} size="sm" isPatientTheme />
                    <StatusBadge meta={APPOINTMENT_TYPE_MAP[appt.type]} size="sm" isPatientTheme />
                  </div>

                  <div>
                    <h3 className="text-base sm:text-lg font-bold text-slate-900 tracking-tight">
                      {appt.doctor_name || 'Assigned Clinician'}
                    </h3>
                    {appt.doctor_specialty && (
                      <p className="text-xs font-medium text-brand-700 mt-0.5">
                        {appt.doctor_specialty}
                      </p>
                    )}
                  </div>

                  <div className="flex flex-wrap items-center gap-4 text-xs text-slate-600">
                    <div className="flex items-center gap-1.5 font-medium">
                      <Calendar className="w-3.5 h-3.5 text-slate-400" />
                      <span>{formatDisplayDate(appt.start_time)}</span>
                    </div>
                    <div className="flex items-center gap-1.5 font-mono">
                      <Clock className="w-3.5 h-3.5 text-slate-400" />
                      <span>{timeRange.local}</span>
                    </div>
                  </div>
                </div>

                {/* Actions Panel */}
                <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3 shrink-0">
                  {/* Telehealth Room Link */}
                  {appt.type === 'virtual' && appt.status === 'upcoming' && (
                    <>
                      {telehealthInfo.canJoin && appt.meeting_link ? (
                        <a
                          href={appt.meeting_link}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <Button isPatientTheme size="sm" className="gap-2 w-full sm:w-auto">
                            <Video className="w-4 h-4" />
                            <span>Join Video Call</span>
                          </Button>
                        </a>
                      ) : (
                        <span className="text-[11px] font-mono text-slate-400 px-3 py-1.5 rounded-full bg-slate-50 border border-slate-200 text-center">
                          {telehealthInfo.message}
                        </span>
                      )}
                    </>
                  )}

                  {/* Cancel Action */}
                  {appt.status === 'upcoming' && (
                    <Button
                      variant="outline"
                      isPatientTheme
                      size="sm"
                      onClick={() => {
                        setCancelError(null);
                        setCancelModalAppointment(appt);
                      }}
                      className="text-slate-600 hover:text-rose-700 hover:bg-rose-50 hover:border-rose-200 text-xs"
                    >
                      Cancel Visit
                    </Button>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      ) : activeTab === 'upcoming' ? (
        <EmptyState
          variant="no-records"
          title="No Upcoming Consultations"
          description="You do not have any upcoming visits booked. Discover our clinical specialists and reserve your time."
          actionLabel="Find a Specialist"
          onAction={() => (window.location.href = '/patient/doctors')}
          isPatientTheme
        />
      ) : activeTab === 'past' ? (
        <EmptyState
          variant="no-records"
          title="No Completed Visits"
          description="Past consultations and completed records will be archived here."
          isPatientTheme
        />
      ) : (
        <EmptyState
          variant="no-records"
          title="No Cancelled Visits"
          description="You have no cancelled appointments."
          isPatientTheme
        />
      )}

      {/* Cancellation Confirmation Modal */}
      {cancelModalAppointment && (
        <Modal
          isOpen={!!cancelModalAppointment}
          onClose={() => setCancelModalAppointment(null)}
          title="Cancel Consultation"
          description={`Are you sure you want to cancel your consultation with ${
            cancelModalAppointment.doctor_name || 'your clinician'
          } on ${formatDisplayDate(cancelModalAppointment.start_time)}?`}
          isPatientTheme
        >
          <div className="space-y-4 pt-2">
            {cancelError && (
              <div className="p-3 rounded-xl bg-rose-50 border border-rose-200 text-xs text-rose-800">
                {cancelError}
              </div>
            )}

            <p className="text-xs text-slate-500 leading-relaxed">
              This consultation slot will be released back to the clinic's calendar for other patients. You can book an alternate time at any point.
            </p>

            <div className="flex items-center justify-end gap-3 pt-4 border-t border-border">
              <Button
                variant="outline"
                isPatientTheme
                size="sm"
                onClick={() => setCancelModalAppointment(null)}
                disabled={cancelMutation.isPending}
              >
                Keep Appointment
              </Button>
              <Button
                isPatientTheme
                size="sm"
                onClick={handleConfirmCancel}
                disabled={cancelMutation.isPending}
                className="bg-rose-600 hover:bg-rose-700 text-white border-transparent"
              >
                {cancelMutation.isPending ? 'Cancelling...' : 'Confirm Cancellation'}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
