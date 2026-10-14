import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchApi } from '../../lib/api/client';
import type { Appointment, AppointmentStatus, AppointmentType, PaginatedResponse } from '../../lib/api/types';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { getStatusMeta } from '../../lib/status';
import { formatDisplayDate, formatSlotRange, formatTime } from '../../lib/datetime';
import { EmptyState } from '../../components/ui/EmptyState';
import { Modal } from '../../components/ui/Modal';
import {
  Video,
  Building2,
  CheckCircle2,
  XCircle,
  User,
  ChevronLeft,
  ChevronRight,
  AlertCircle
} from 'lucide-react';

export function DoctorAppointmentsList() {
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<'all' | AppointmentStatus>('all');
  const [typeFilter, setTypeFilter] = useState<'all' | AppointmentType>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [cancelTarget, setCancelTarget] = useState<Appointment | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const limit = 20;

  // Fetch appointments
  const { data, isLoading, isError, refetch } = useQuery<PaginatedResponse<Appointment>>({
    queryKey: ['doctor-appointments', page],
    queryFn: () =>
      fetchApi<PaginatedResponse<Appointment>>(
        `/api/doctor/appointments?limit=${limit}&offset=${page * limit}`
      ),
  });

  // Complete appointment mutation
  const completeMutation = useMutation({
    mutationFn: async (appointmentId: string) => {
      setActionError(null);
      return fetchApi<{ message: string }>(`/api/appointments/${appointmentId}`, {
        method: 'PATCH',
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['doctor-appointments'] });
    },
    onError: (err: { message?: string; data?: { error?: string } }) => {
      const msg = err.data?.error || err.message || 'Failed to complete appointment';
      setActionError(msg);
    },
  });

  // Cancel appointment mutation
  const cancelMutation = useMutation({
    mutationFn: async (appointmentId: string) => {
      setActionError(null);
      return fetchApi<{ message: string }>(`/api/appointments/${appointmentId}/cancel`, {
        method: 'PATCH',
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['doctor-appointments'] });
      setCancelTarget(null);
    },
    onError: (err: { message?: string; data?: { error?: string } }) => {
      const msg = err.data?.error || err.message || 'Failed to cancel appointment';
      setActionError(msg);
    },
  });

  const appointments = data?.data || [];
  const now = new Date();

  // Client-side filtering across the loaded batch
  const filteredAppointments = appointments.filter((appt) => {
    if (statusFilter !== 'all' && appt.status !== statusFilter) return false;
    if (typeFilter !== 'all' && appt.type !== typeFilter) return false;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const patientName = (appt.patient_name || '').toLowerCase();
      const apptId = appt.id.toLowerCase();
      const patientId = appt.patient_id.toLowerCase();
      if (!patientName.includes(q) && !apptId.includes(q) && !patientId.includes(q)) {
        return false;
      }
    }
    return true;
  });

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-200">
        <div>
          <span className="font-mono text-xs uppercase tracking-wider text-slate-500 font-semibold">
            Clinical Ledger • Screen C2
          </span>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900 mt-1">
            Appointments Ledger
          </h1>
          <p className="text-sm text-slate-500 mt-0.5">
            Full audit register of scheduled, concluded, and cancelled patient sessions
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Link
            to="/doctor/today"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs"
          >
            Today's Agenda
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

      {/* Filter Bar */}
      <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
          {/* Status Tabs */}
          <div className="flex items-center gap-1 bg-slate-100 p-1 rounded-md border border-slate-200 text-xs font-medium">
            {(['all', 'upcoming', 'completed', 'cancelled'] as const).map((s) => (
              <button
                key={s}
                onClick={() => setStatusFilter(s)}
                className={`px-3 py-1 rounded transition-colors capitalize ${
                  statusFilter === s
                    ? 'bg-white text-slate-900 shadow-2xs font-semibold'
                    : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                {s}
              </button>
            ))}
          </div>

          {/* Search Input & Modality Filter */}
          <div className="flex items-center gap-2 flex-wrap">
            <div className="relative">
              <input
                type="text"
                placeholder="Search patient, ID..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="px-3 py-1.5 text-xs bg-slate-50 border border-slate-200 rounded-md focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400 w-48 md:w-60 font-mono"
              />
            </div>

            <select
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value as 'all' | AppointmentType)}
              className="px-2.5 py-1.5 text-xs bg-white border border-slate-200 rounded-md focus:outline-none text-slate-700"
            >
              <option value="all">All Modalities</option>
              <option value="in_person">In-Person</option>
              <option value="virtual">Virtual</option>
            </select>
          </div>
        </div>
      </div>

      {/* Data Table */}
      <div className="bg-white border border-slate-200 rounded-lg shadow-2xs overflow-hidden">
        {isLoading ? (
          <div className="p-8 space-y-3">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="h-12 bg-slate-100 rounded animate-pulse" />
            ))}
          </div>
        ) : isError ? (
          <div className="p-8">
            <EmptyState
              variant="error"
              title="Error Loading Appointments"
              description="Could not synchronize with the clinic server. Please try again."
              actionLabel="Retry"
              onAction={() => refetch()}
            />
          </div>
        ) : filteredAppointments.length === 0 ? (
          <div className="p-8">
            <EmptyState
              variant="calendar"
              title="No Matching Appointments Found"
              description="No clinical records matched your active filter or search criteria."
              actionLabel="Clear Filters"
              onAction={() => {
                setStatusFilter('all');
                setTypeFilter('all');
                setSearchQuery('');
              }}
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50/75 text-slate-500 font-mono uppercase tracking-wider">
                  <th className="py-3 px-4 font-semibold">Date & Time</th>
                  <th className="py-3 px-4 font-semibold">Patient</th>
                  <th className="py-3 px-4 font-semibold">Modality</th>
                  <th className="py-3 px-4 font-semibold">Status</th>
                  <th className="py-3 px-4 font-semibold text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {filteredAppointments.map((appt) => {
                  const apptStart = new Date(appt.start_time);
                  const isStarted = now >= apptStart;
                  const canComplete = isStarted && appt.status === 'upcoming';
                  const slotTimes = formatSlotRange(appt.start_time, appt.end_time);

                  return (
                    <tr key={appt.id} className="hover:bg-slate-50/60 transition-colors">
                      {/* Date & Time */}
                      <td className="py-3.5 px-4 font-mono">
                        <div className="font-semibold text-slate-900">
                          {formatDisplayDate(appt.start_time)}
                        </div>
                        <div className="text-slate-500 text-[11px] mt-0.5">
                          {slotTimes.local}
                        </div>
                      </td>

                      {/* Patient */}
                      <td className="py-3.5 px-4">
                        <div className="font-medium text-slate-900">
                          {appt.patient_name || 'Patient'}
                        </div>
                        <div className="font-mono text-[11px] text-slate-400">
                          ID: {appt.patient_id.slice(-8)}
                        </div>
                      </td>

                      {/* Modality */}
                      <td className="py-3.5 px-4">
                        <span className="inline-flex items-center gap-1.5 text-slate-700 capitalize font-medium">
                          {appt.type === 'virtual' ? (
                            <Video className="w-3.5 h-3.5 text-sky-600" />
                          ) : (
                            <Building2 className="w-3.5 h-3.5 text-slate-600" />
                          )}
                          {appt.type.replace('_', ' ')}
                        </span>
                      </td>

                      {/* Status */}
                      <td className="py-3.5 px-4">
                        <StatusBadge meta={getStatusMeta('appointment', appt.status)} size="sm" />
                      </td>

                      {/* Actions */}
                      <td className="py-3.5 px-4 text-right">
                        <div className="flex items-center justify-end gap-2">
                          {/* Chart shortcut */}
                          <Link
                            to={`/doctor/patients/${appt.patient_id}`}
                            className="inline-flex items-center gap-1 px-2.5 py-1 text-slate-700 hover:text-slate-900 bg-white border border-slate-200 rounded hover:bg-slate-50 transition-colors"
                            title="Inspect patient record"
                          >
                            <User className="w-3 h-3 text-slate-400" />
                            Chart
                          </Link>

                          {/* Telehealth Room Link */}
                          {appt.type === 'virtual' && appt.status === 'upcoming' && appt.meeting_link && (
                            <a
                              href={appt.meeting_link}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1 px-2.5 py-1 text-sky-700 bg-sky-50 border border-sky-200 rounded hover:bg-sky-100 transition-colors"
                            >
                              <Video className="w-3 h-3" />
                              Room
                            </a>
                          )}

                          {/* Complete action */}
                          {appt.status === 'upcoming' && (
                            <button
                              type="button"
                              disabled={!canComplete || completeMutation.isPending}
                              onClick={() => completeMutation.mutate(appt.id)}
                              className={`inline-flex items-center gap-1 px-2.5 py-1 rounded transition-colors ${
                                canComplete
                                  ? 'bg-slate-900 text-white hover:bg-slate-800 cursor-pointer shadow-2xs'
                                  : 'bg-slate-100 text-slate-400 border border-slate-200 cursor-not-allowed opacity-75'
                              }`}
                              title={
                                canComplete
                                  ? 'Mark consultation as completed'
                                  : `Temporal restriction: Available at ${formatTime(appt.start_time)}`
                              }
                            >
                              <CheckCircle2 className="w-3 h-3" />
                              Complete
                            </button>
                          )}

                          {/* Cancel action */}
                          {appt.status === 'upcoming' && (
                            <button
                              type="button"
                              onClick={() => setCancelTarget(appt)}
                              className="inline-flex items-center gap-1 px-2.5 py-1 text-rose-700 bg-rose-50 border border-rose-200 rounded hover:bg-rose-100 transition-colors cursor-pointer"
                              title="Cancel appointment"
                            >
                              <XCircle className="w-3 h-3 text-rose-600" />
                              Cancel
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {/* Pagination footer */}
        <div className="py-3 px-4 bg-slate-50 border-t border-slate-200 flex items-center justify-between text-xs text-slate-500 font-mono">
          <span>
            Page {page + 1} • Displaying up to {limit} records
          </span>
          <div className="flex items-center gap-1">
            <button
              disabled={page === 0}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              className="p-1.5 border border-slate-200 rounded bg-white hover:bg-slate-50 disabled:opacity-40 disabled:pointer-events-none"
            >
              <ChevronLeft className="w-3.5 h-3.5" />
            </button>
            <button
              disabled={appointments.length < limit}
              onClick={() => setPage((p) => p + 1)}
              className="p-1.5 border border-slate-200 rounded bg-white hover:bg-slate-50 disabled:opacity-40 disabled:pointer-events-none"
            >
              <ChevronRight className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>

      {/* Cancellation Confirmation Dialog */}
      {cancelTarget && (
        <Modal
          isOpen={true}
          onClose={() => setCancelTarget(null)}
          title="Confirm Consultation Cancellation"
        >
          <div className="space-y-4">
            <p className="text-sm text-slate-600">
              Are you sure you want to cancel the scheduled consultation with{' '}
              <strong className="text-slate-900">{cancelTarget.patient_name || 'Patient'}</strong> on{' '}
              <strong className="font-mono text-slate-900">{formatDisplayDate(cancelTarget.start_time)}</strong>?
            </p>
            <p className="text-xs text-slate-500 bg-slate-50 p-2.5 rounded border border-slate-200 font-mono">
              Appt ID: {cancelTarget.id}
            </p>

            <div className="flex items-center justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setCancelTarget(null)}
                className="px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50"
              >
                Keep Appointment
              </button>
              <button
                type="button"
                disabled={cancelMutation.isPending}
                onClick={() => cancelMutation.mutate(cancelTarget.id)}
                className="px-3 py-1.5 text-xs font-medium text-white bg-rose-600 rounded-md hover:bg-rose-700 disabled:opacity-50"
              >
                {cancelMutation.isPending ? 'Cancelling...' : 'Confirm Cancellation'}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
