import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { useAuth } from '../../context/useAuth';
import { fetchApi } from '../../lib/api/client';
import type {
  Appointment,
  Prescription,
  PaginatedResponse,
  PrescriptionDownloadResponse,
} from '../../lib/api/types';
import {
  APPOINTMENT_STATUS_MAP,
  APPOINTMENT_TYPE_MAP,
} from '../../lib/status';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Button } from '../../components/ui/Button';
import { Card } from '../../components/ui/Card';
import { EmptyState } from '../../components/ui/EmptyState';
import {
  formatDisplayDate,
  formatSlotRange,
  formatRelativeAppointmentTime,
  isWithinTelehealthWindow,
} from '../../lib/datetime';
import { downloadFromPresignedUrl } from '../../lib/storage';
import {
  Calendar,
  Stethoscope,
  FileText,
  Download,
  ArrowRight,
  Video,
  Clock,
  Activity,
  ChevronRight,
} from 'lucide-react';

export function PatientDashboard() {
  const { user } = useAuth();
  const [downloadingId, setDownloadingId] = useState<string | null>(null);

  // Fetch Patient Appointments
  const {
    data: appointmentsData,
    isLoading: isLoadingAppts,
  } = useQuery({
    queryKey: ['patient-appointments'],
    queryFn: () =>
      fetchApi<PaginatedResponse<Appointment>>('/api/patient/appointments?limit=10&offset=0'),
  });

  // Fetch Patient Prescriptions (approved only by backend design)
  const {
    data: prescriptionsData,
    isLoading: isLoadingRx,
  } = useQuery({
    queryKey: ['patient-prescriptions'],
    queryFn: () =>
      fetchApi<PaginatedResponse<Prescription>>('/api/patient/prescriptions?limit=5&offset=0'),
  });

  const appointments = appointmentsData?.data || [];
  const prescriptions = prescriptionsData?.data || [];

  // Filter for the nearest upcoming appointment
  const now = new Date();
  const upcomingAppointments = appointments
    .filter(
      (a) =>
        a.status === 'upcoming' && new Date(a.end_time || a.start_time).getTime() >= now.getTime()
    )
    .sort((a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime());

  const nextAppointment = upcomingAppointments[0];

  const handleDownloadPdf = async (prescriptionId: string, filename?: string) => {
    try {
      setDownloadingId(prescriptionId);
      const res = await fetchApi<PrescriptionDownloadResponse>(
        `/api/prescriptions/${prescriptionId}/download-url`
      );
      if (res.download_url) {
        downloadFromPresignedUrl(res.download_url, res.file_name || filename || 'prescription.pdf');
      }
    } catch (err) {
      console.error('Failed to download prescription PDF:', err);
    } finally {
      setDownloadingId(null);
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      {/* Welcome Banner */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-900">
            Welcome back, {user?.first_name || 'Patient'}
          </h1>
          <p className="text-sm text-slate-500 mt-1">
            Access your scheduled consultations, clinical records, and prescription slips.
          </p>
        </div>
        <div>
          <Link to="/patient/doctors">
            <Button isPatientTheme size="md" className="gap-2 shadow-sm">
              <Stethoscope className="w-4 h-4" />
              <span>Book Consultation</span>
            </Button>
          </Link>
        </div>
      </div>

      {/* Main Grid: Bento Box */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Next Appointment Hero (2 cols wide on desktop) */}
        <div className="lg:col-span-2 space-y-6">
          <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border">
            <div className="flex items-center justify-between gap-4 mb-6">
              <div className="flex items-center gap-2.5">
                <div className="w-9 h-9 rounded-full bg-brand-50 text-brand-700 flex items-center justify-center">
                  <Calendar className="w-5 h-5" />
                </div>
                <div>
                  <h2 className="text-base font-semibold text-slate-900 tracking-tight">
                    Next Consultation
                  </h2>
                  <p className="text-xs text-slate-400">Upcoming clinical appointment</p>
                </div>
              </div>
              {nextAppointment && (
                <span className="font-mono text-xs font-medium px-2.5 py-1 rounded-full bg-slate-100 text-slate-700 border border-slate-200">
                  {formatRelativeAppointmentTime(nextAppointment.start_time)}
                </span>
              )}
            </div>

            {isLoadingAppts ? (
              <div className="animate-pulse space-y-4 py-6">
                <div className="h-5 bg-slate-100 rounded w-1/3" />
                <div className="h-10 bg-slate-100 rounded w-3/4" />
                <div className="h-4 bg-slate-100 rounded w-1/2" />
              </div>
            ) : nextAppointment ? (
              <div className="space-y-6">
                <div className="p-5 sm:p-6 rounded-2xl bg-slate-50/80 border border-slate-200/70 space-y-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <h3 className="text-lg font-bold text-slate-900 tracking-tight">
                        {nextAppointment.doctor_name || 'Assigned Clinician'}
                      </h3>
                      {nextAppointment.doctor_specialty && (
                        <p className="text-xs font-medium text-brand-700 mt-0.5">
                          {nextAppointment.doctor_specialty}
                        </p>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      <StatusBadge
                        meta={APPOINTMENT_TYPE_MAP[nextAppointment.type]}
                        size="sm"
                        isPatientTheme
                      />
                      <StatusBadge
                        meta={APPOINTMENT_STATUS_MAP[nextAppointment.status]}
                        size="sm"
                        isPatientTheme
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-slate-200/60 text-xs text-slate-600">
                    <div className="flex items-center gap-2">
                      <Calendar className="w-4 h-4 text-slate-400 shrink-0" />
                      <span className="font-medium text-slate-900">
                        {formatDisplayDate(nextAppointment.start_time)}
                      </span>
                    </div>
                    <div className="flex items-center gap-2">
                      <Clock className="w-4 h-4 text-slate-400 shrink-0" />
                      <span className="font-mono font-medium text-slate-900">
                        {formatSlotRange(nextAppointment.start_time, nextAppointment.end_time).local}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Telehealth Room Link / In-Person Notice */}
                {nextAppointment.type === 'virtual' ? (
                  (() => {
                    const windowInfo = isWithinTelehealthWindow(
                      nextAppointment.start_time,
                      nextAppointment.end_time
                    );
                    return (
                      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-2xl bg-sky-50/70 border border-sky-100">
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-full bg-sky-100 text-sky-800 flex items-center justify-center shrink-0">
                            <Video className="w-4 h-4" />
                          </div>
                          <div>
                            <p className="text-xs font-semibold text-sky-950">
                              Encrypted Telehealth Room
                            </p>
                            <p className="text-[11px] text-sky-700">{windowInfo.message}</p>
                          </div>
                        </div>

                        {windowInfo.canJoin && nextAppointment.meeting_link ? (
                          <a
                            href={nextAppointment.meeting_link}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            <Button size="sm" isPatientTheme className="w-full sm:w-auto gap-2">
                              <span>Join Video Call</span>
                              <ArrowRight className="w-3.5 h-3.5" />
                            </Button>
                          </a>
                        ) : (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled
                            isPatientTheme
                            className="w-full sm:w-auto text-xs"
                          >
                            Room Inactive
                          </Button>
                        )}
                      </div>
                    );
                  })()
                ) : (
                  <p className="text-xs text-slate-500 italic">
                    Location: Main Clinical Facility. Please check in at reception 10 minutes prior.
                  </p>
                )}

                <div className="flex justify-end pt-2">
                  <Link
                    to="/patient/appointments"
                    className="text-xs font-semibold text-brand-700 hover:text-brand-800 flex items-center gap-1 transition-colors"
                  >
                    <span>View all appointments</span>
                    <ChevronRight className="w-3.5 h-3.5" />
                  </Link>
                </div>
              </div>
            ) : (
              <EmptyState
                variant="no-records"
                title="No Upcoming Consultations"
                description="You currently have no scheduled appointments. Discover clinicians and choose a time that fits your day."
                actionLabel="Find a Doctor"
                onAction={() => (window.location.href = '/patient/doctors')}
                isPatientTheme
              />
            )}
          </Card>

          {/* Quick Actions & Navigation Cards */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Link to="/patient/doctors" className="group">
              <Card className="rounded-2xl p-5 bg-white border border-border hover:border-brand-300 hover:shadow-xs transition-all h-full flex flex-col justify-between">
                <div>
                  <div className="w-8 h-8 rounded-full bg-emerald-50 text-emerald-700 flex items-center justify-center mb-3">
                    <Stethoscope className="w-4 h-4" />
                  </div>
                  <h3 className="text-sm font-semibold text-slate-900 group-hover:text-brand-700 transition-colors">
                    Clinical Specialists
                  </h3>
                  <p className="text-xs text-slate-500 mt-1 leading-relaxed">
                    Browse doctors by specialty, check working hours, and reserve slots.
                  </p>
                </div>
                <span className="text-xs font-semibold text-brand-700 flex items-center gap-1 mt-4">
                  Browse directory <ArrowRight className="w-3 h-3 group-hover:translate-x-0.5 transition-transform" />
                </span>
              </Card>
            </Link>

            <Link to="/patient/appointments" className="group">
              <Card className="rounded-2xl p-5 bg-white border border-border hover:border-brand-300 hover:shadow-xs transition-all h-full flex flex-col justify-between">
                <div>
                  <div className="w-8 h-8 rounded-full bg-sky-50 text-sky-700 flex items-center justify-center mb-3">
                    <Calendar className="w-4 h-4" />
                  </div>
                  <h3 className="text-sm font-semibold text-slate-900 group-hover:text-brand-700 transition-colors">
                    Visit History
                  </h3>
                  <p className="text-xs text-slate-500 mt-1 leading-relaxed">
                    View upcoming visits, past records, and manage cancellations.
                  </p>
                </div>
                <span className="text-xs font-semibold text-brand-700 flex items-center gap-1 mt-4">
                  View visits <ArrowRight className="w-3 h-3 group-hover:translate-x-0.5 transition-transform" />
                </span>
              </Card>
            </Link>
          </div>
        </div>

        {/* Right Column: Prescriptions & Vitals Summary (1 col wide) */}
        <div className="space-y-6">
          {/* Recent Prescriptions */}
          <Card className="rounded-3xl p-6 bg-white border border-border">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 rounded-full bg-brand-50 text-brand-700 flex items-center justify-center">
                  <FileText className="w-4 h-4" />
                </div>
                <h3 className="text-sm font-semibold text-slate-900 tracking-tight">
                  Approved Scripts
                </h3>
              </div>
              <Link
                to="/patient/prescriptions"
                className="text-xs font-semibold text-brand-700 hover:text-brand-800"
              >
                All
              </Link>
            </div>

            {isLoadingRx ? (
              <div className="animate-pulse space-y-3 py-4">
                <div className="h-12 bg-slate-100 rounded-xl" />
                <div className="h-12 bg-slate-100 rounded-xl" />
              </div>
            ) : prescriptions.length > 0 ? (
              <div className="space-y-3">
                {prescriptions.slice(0, 3).map((rx) => (
                  <div
                    key={rx.id}
                    className="p-3.5 rounded-2xl bg-slate-50 border border-slate-200/60 hover:bg-slate-100/60 transition-colors flex items-center justify-between gap-3"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="text-xs font-semibold text-slate-900 truncate">
                        {rx.doctor_name ? `Dr. ${rx.doctor_name}` : 'Prescription Slip'}
                      </p>
                      <p className="text-[11px] text-slate-500 font-mono mt-0.5">
                        {formatDisplayDate(rx.created_at)}
                      </p>
                      {rx.items && rx.items.length > 0 && (
                        <p className="text-[11px] text-slate-600 truncate mt-1">
                          {rx.items.map((i) => i.medication_name).join(', ')}
                        </p>
                      )}
                    </div>
                    <button
                      type="button"
                      onClick={() => handleDownloadPdf(rx.id, rx.file_name)}
                      disabled={downloadingId === rx.id}
                      className="p-2 text-slate-600 hover:text-brand-700 hover:bg-white rounded-lg border border-transparent hover:border-slate-200 transition-all cursor-pointer shrink-0"
                      title="Download PDF"
                    >
                      <Download className="w-4 h-4" />
                    </button>
                  </div>
                ))}

                <Link
                  to="/patient/prescriptions"
                  className="block text-center text-xs font-semibold text-slate-600 hover:text-slate-900 pt-2"
                >
                  View full medication ledger →
                </Link>
              </div>
            ) : (
              <div className="text-center py-8 text-xs text-slate-400">
                <p>No active prescriptions on file.</p>
              </div>
            )}
          </Card>

          {/* Vitals Overview Placeholder (Prepared for M5) */}
          <Card className="rounded-3xl p-6 bg-white border border-border">
            <div className="flex items-center gap-2 mb-3">
              <div className="w-8 h-8 rounded-full bg-rose-50 text-rose-700 flex items-center justify-center">
                <Activity className="w-4 h-4" />
              </div>
              <div>
                <h3 className="text-sm font-semibold text-slate-900 tracking-tight">
                  Longitudinal Vitals
                </h3>
                <p className="text-[11px] text-slate-400">Cardiovascular & Metabolic</p>
              </div>
            </div>

            <p className="text-xs text-slate-500 leading-relaxed">
              Track blood pressure, heart rate, and glucose trends logged during clinical encounters.
            </p>

            <div className="mt-4 pt-3 border-t border-slate-100 flex items-center justify-between text-xs text-slate-400">
              <span className="font-mono text-[11px]">HIPAA § 164.312(b)</span>
              <span className="px-2 py-0.5 rounded-full bg-slate-100 font-medium text-slate-600 text-[10px]">
                Active Ledger
              </span>
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
