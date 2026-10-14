import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchApi } from '../../lib/api/client';
import type { Appointment, PaginatedResponse } from '../../lib/api/types';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { getStatusMeta } from '../../lib/status';
import { formatDisplayDate, formatSlotRange, formatTime } from '../../lib/datetime';
import { EmptyState } from '../../components/ui/EmptyState';
import { 
  Calendar, 
  Video, 
  Building2, 
  CheckCircle2, 
  Clock, 
  ExternalLink, 
  User, 
  AlertCircle
} from 'lucide-react';

export function TodayTimeline() {
  const queryClient = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);
  const [completingId, setCompletingId] = useState<string | null>(null);

  // Fetch doctor's appointments
  const { data, isLoading, isError, refetch } = useQuery<PaginatedResponse<Appointment>>({
    queryKey: ['doctor-appointments'],
    queryFn: () => fetchApi<PaginatedResponse<Appointment>>('/api/doctor/appointments?limit=100&offset=0'),
  });

  // Mutation to mark appointment as completed
  const completeMutation = useMutation({
    mutationFn: async (appointmentId: string) => {
      setCompletingId(appointmentId);
      setActionError(null);
      return fetchApi<{ message: string }>(`/api/appointments/${appointmentId}`, {
        method: 'PATCH',
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['doctor-appointments'] });
      setCompletingId(null);
    },
    onError: (err: { message?: string; data?: { error?: string } }) => {
      setCompletingId(null);
      const msg = err.data?.error || err.message || 'Failed to complete appointment';
      setActionError(msg);
    },
  });

  const appointments = data?.data || [];
  const now = new Date();

  // Local calendar day matching
  const isSameDay = (iso: string) => {
    try {
      const d = new Date(iso);
      return (
        d.getFullYear() === now.getFullYear() &&
        d.getMonth() === now.getMonth() &&
        d.getDate() === now.getDate()
      );
    } catch {
      return false;
    }
  };

  // Filter appointments for today
  const todayAppointments = appointments
    .filter((a) => isSameDay(a.start_time))
    .sort((a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime());

  // Metrics
  const totalToday = todayAppointments.length;
  const inPersonToday = todayAppointments.filter((a) => a.type === 'in_person').length;
  const virtualToday = todayAppointments.filter((a) => a.type === 'virtual').length;
  const completedToday = todayAppointments.filter((a) => a.status === 'completed').length;

  return (
    <div className="space-y-8 max-w-6xl mx-auto">
      {/* Editorial Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-200">
        <div>
          <span className="font-mono text-xs uppercase tracking-wider text-slate-500 font-semibold">
            Clinical Workflow • Screen C1
          </span>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900 mt-1">
            Today's Clinical Agenda
          </h1>
          <p className="text-sm text-slate-500 mt-0.5">
            {formatDisplayDate(now.toISOString())} • Consultation schedule & patient queue
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Link
            to="/doctor/appointments"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs"
          >
            <Calendar className="w-3.5 h-3.5 text-slate-500" />
            Full Ledger
          </Link>
          <Link
            to="/doctor/settings/schedule"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs"
          >
            <Clock className="w-3.5 h-3.5 text-slate-500" />
            Configure Hours
          </Link>
        </div>
      </div>

      {actionError && (
        <div className="p-4 bg-rose-50 border border-rose-200 rounded-lg flex items-center justify-between text-sm text-rose-800">
          <div className="flex items-center gap-2">
            <AlertCircle className="w-4 h-4 text-rose-600 shrink-0" />
            <span>{actionError}</span>
          </div>
          <button
            onClick={() => setActionError(null)}
            className="text-xs font-medium text-rose-700 hover:text-rose-900 underline ml-4"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* KPI Bento Box */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-xs font-medium uppercase tracking-wider">Scheduled Today</span>
            <Calendar className="w-4 h-4 text-slate-400" />
          </div>
          <div className="text-2xl font-bold font-mono text-slate-900 mt-2">{totalToday}</div>
          <p className="text-xs text-slate-500 mt-1">Booked patient sessions</p>
        </div>

        <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-xs font-medium uppercase tracking-wider">In-Person</span>
            <Building2 className="w-4 h-4 text-slate-400" />
          </div>
          <div className="text-2xl font-bold font-mono text-slate-900 mt-2">{inPersonToday}</div>
          <p className="text-xs text-slate-500 mt-1">Clinic examination visits</p>
        </div>

        <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-xs font-medium uppercase tracking-wider">Virtual Calls</span>
            <Video className="w-4 h-4 text-slate-400" />
          </div>
          <div className="text-2xl font-bold font-mono text-slate-900 mt-2">{virtualToday}</div>
          <p className="text-xs text-slate-500 mt-1">Telehealth encrypted rooms</p>
        </div>

        <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-xs font-medium uppercase tracking-wider">Completed</span>
            <CheckCircle2 className="w-4 h-4 text-emerald-600" />
          </div>
          <div className="text-2xl font-bold font-mono text-emerald-700 mt-2">{completedToday}</div>
          <p className="text-xs text-slate-500 mt-1">Concluded consultations</p>
        </div>
      </div>

      {/* Timeline Section */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-slate-900 flex items-center gap-2">
            <span>Consultation Timeline</span>
            <span className="text-xs font-normal text-slate-500 font-mono">
              ({todayAppointments.length} slots)
            </span>
          </h2>
        </div>

        {isLoading ? (
          <div className="space-y-3">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-24 bg-slate-100 rounded-lg animate-pulse border border-slate-200" />
            ))}
          </div>
        ) : isError ? (
          <EmptyState
            variant="error"
            title="Failed to Load Schedule"
            description="Could not synchronize with the clinic server. Please check network connectivity."
            actionLabel="Retry Synchronization"
            onAction={() => refetch()}
          />
        ) : todayAppointments.length === 0 ? (
          <EmptyState
            variant="calendar"
            title="No Consultations Scheduled Today"
            description="You have no booked patient consultations for today. Check your full appointment ledger or update your availability settings."
            actionLabel="View Appointments Ledger"
            onAction={() => (window.location.href = '/doctor/appointments')}
          />
        ) : (
          <div className="space-y-3">
            {todayAppointments.map((appt) => {
              const apptStart = new Date(appt.start_time);
              const apptEnd = new Date(appt.end_time);
              const isStarted = now >= apptStart;
              const isPast = now > apptEnd;
              const canComplete = isStarted && appt.status === 'upcoming';
              const slotTimes = formatSlotRange(appt.start_time, appt.end_time);

              // Relative status badge
              let timingLabel = 'Scheduled';
              let timingClass = 'bg-slate-100 text-slate-600 border-slate-200';
              if (appt.status === 'completed') {
                timingLabel = 'Concluded';
                timingClass = 'bg-emerald-50 text-emerald-700 border-emerald-200';
              } else if (appt.status === 'cancelled') {
                timingLabel = 'Cancelled';
                timingClass = 'bg-slate-100 text-slate-500 border-slate-200 line-through';
              } else if (isStarted && !isPast) {
                timingLabel = 'In Progress';
                timingClass = 'bg-amber-50 text-amber-800 border-amber-200 animate-pulse';
              } else if (isPast) {
                timingLabel = 'Overdue for completion';
                timingClass = 'bg-rose-50 text-rose-800 border-rose-200';
              } else {
                const diffMin = Math.round((apptStart.getTime() - now.getTime()) / 60000);
                if (diffMin <= 60) {
                  timingLabel = `Starts in ${diffMin}m`;
                  timingClass = 'bg-sky-50 text-sky-800 border-sky-200';
                }
              }

              return (
                <div
                  key={appt.id}
                  className={`p-5 bg-white border rounded-lg transition-all shadow-2xs flex flex-col md:flex-row md:items-center justify-between gap-4 ${
                    appt.status === 'cancelled'
                      ? 'border-slate-200 opacity-60 bg-slate-50/50'
                      : isStarted && !isPast && appt.status === 'upcoming'
                      ? 'border-brand-500 ring-1 ring-brand-500/20'
                      : 'border-slate-200 hover:border-slate-300'
                  }`}
                >
                  {/* Slot Time & Timing Cue */}
                  <div className="flex items-start md:items-center gap-4 min-w-[200px]">
                    <div className="w-10 h-10 rounded-lg bg-slate-100 border border-slate-200 flex items-center justify-center text-slate-700 shrink-0">
                      {appt.type === 'virtual' ? (
                        <Video className="w-5 h-5 text-sky-600" />
                      ) : (
                        <Building2 className="w-5 h-5 text-slate-600" />
                      )}
                    </div>
                    <div>
                      <div className="font-mono text-sm font-bold text-slate-900">
                        {slotTimes.local}
                      </div>
                      <span className={`inline-block mt-1 text-[11px] font-mono px-2 py-0.5 rounded border uppercase tracking-wider ${timingClass}`}>
                        {timingLabel}
                      </span>
                    </div>
                  </div>

                  {/* Patient Info */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="font-semibold text-slate-900 text-sm">
                        {appt.patient_name || 'Patient'}
                      </span>
                      <span className="font-mono text-xs text-slate-500 bg-slate-100 px-1.5 py-0.5 rounded border border-slate-200">
                        ID: {appt.patient_id.slice(-6)}
                      </span>
                      <StatusBadge meta={getStatusMeta('appointment', appt.status)} size="sm" />
                      <span className="text-xs font-mono text-slate-500 capitalize">
                        • {appt.type.replace('_', ' ')}
                      </span>
                    </div>
                    <div className="text-xs text-slate-500 mt-1 flex items-center gap-3">
                      <span>Appt ID: <span className="font-mono">{appt.id.slice(-8)}</span></span>
                    </div>
                  </div>

                  {/* Actions Area */}
                  <div className="flex items-center gap-2 flex-wrap shrink-0">
                    {/* Open Patient Chart */}
                    <Link
                      to={`/doctor/patients/${appt.patient_id}`}
                      className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors"
                      title="Open longitudinal chart for this patient"
                    >
                      <User className="w-3.5 h-3.5 text-slate-500" />
                      Patient Chart
                    </Link>

                    {/* Virtual Meeting Room */}
                    {appt.type === 'virtual' && appt.status === 'upcoming' && appt.meeting_link && (
                      <a
                        href={appt.meeting_link}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-sky-700 bg-sky-50 border border-sky-200 rounded-md hover:bg-sky-100 transition-colors"
                      >
                        <Video className="w-3.5 h-3.5" />
                        Join Telehealth
                        <ExternalLink className="w-3 h-3 text-sky-500 ml-0.5" />
                      </a>
                    )}

                    {/* Temporal Gated Consultation Completion Action */}
                    {appt.status === 'upcoming' && (
                      <div className="relative group">
                        <button
                          type="button"
                          disabled={!canComplete || completeMutation.isPending}
                          onClick={() => completeMutation.mutate(appt.id)}
                          className={`inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-md transition-all ${
                            canComplete
                              ? 'bg-slate-900 text-white hover:bg-slate-800 cursor-pointer shadow-2xs'
                              : 'bg-slate-100 text-slate-400 border border-slate-200 cursor-not-allowed opacity-75'
                          }`}
                        >
                          <CheckCircle2 className="w-3.5 h-3.5" />
                          {completingId === appt.id ? 'Completing...' : 'Complete Visit'}
                        </button>

                        {!canComplete && (
                          <div className="absolute right-0 bottom-full mb-1.5 hidden group-hover:block z-30 w-52 p-2 bg-slate-900 text-white text-[11px] rounded shadow-lg pointer-events-none">
                            Temporal lock: Visit cannot be marked as completed before its scheduled start time ({formatTime(appt.start_time)}).
                          </div>
                        )}
                      </div>
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
