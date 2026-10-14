import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchApi } from '../../lib/api/client';
import type { Appointment, Prescription, PatientVital, PaginatedResponse } from '../../lib/api/types';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { getStatusMeta } from '../../lib/status';
import { formatDisplayDate, formatSlotRange } from '../../lib/datetime';
import { EmptyState } from '../../components/ui/EmptyState';
import { RecordVitalsModal } from '../../components/doctor/RecordVitalsModal';
import {
  Activity,
  FileText,
  Calendar,
  Download,
  ExternalLink,
  Plus,
  ArrowLeft,
  Video,
  Building2,
  AlertTriangle
} from 'lucide-react';

interface PatientChartProps {
  autoOpenVitals?: boolean;
}

export function DoctorPatientChart({ autoOpenVitals = false }: PatientChartProps) {
  const { id: patientId = '' } = useParams<{ id: string }>();
  const [activeTab, setActiveTab] = useState<'prescriptions' | 'visits' | 'vitals'>('prescriptions');
  const [isVitalsModalOpen, setIsVitalsModalOpen] = useState(autoOpenVitals);

  // 1. Fetch Patient Appointments History
  const {
    data: appointmentsData,
    isLoading: isLoadingAppointments,
  } = useQuery<PaginatedResponse<Appointment>>({
    queryKey: ['doctor-patient-appointments', patientId],
    queryFn: () =>
      fetchApi<PaginatedResponse<Appointment>>(
        `/api/doctor/patients/${patientId}/appointments?limit=50&offset=0`
      ),
    enabled: !!patientId,
  });

  // 2. Fetch Patient Prescriptions History
  const {
    data: prescriptionsData,
    isLoading: isLoadingPrescriptions,
  } = useQuery<PaginatedResponse<Prescription>>({
    queryKey: ['doctor-patient-prescriptions', patientId],
    queryFn: () =>
      fetchApi<PaginatedResponse<Prescription>>(
        `/api/doctor/patients/${patientId}/prescriptions?limit=50&offset=0`
      ),
    enabled: !!patientId,
  });

  // 3. Fetch Longitudinal Vitals
  const {
    data: vitalsData,
    isLoading: isLoadingVitals,
    refetch: refetchVitals,
  } = useQuery<PaginatedResponse<PatientVital>>({
    queryKey: ['patient-vitals', patientId],
    queryFn: () =>
      fetchApi<PaginatedResponse<PatientVital>>(
        `/api/vitals?patient_id=${patientId}&limit=50&offset=0`
      ),
    enabled: !!patientId,
  });

  const appointments = appointmentsData?.data || [];
  const prescriptions = prescriptionsData?.data || [];
  const vitals = vitalsData?.data || [];

  // Derive patient name from appointment records if available
  const derivedPatientName =
    appointments[0]?.patient_name ||
    prescriptions[0]?.patient_name ||
    'Clinical Patient';

  const handleDownloadPrescription = async (prescriptionId: string) => {
    try {
      const res = await fetchApi<{ download_url: string }>(
        `/api/prescriptions/${prescriptionId}/download-url`
      );
      if (res.download_url) {
        window.open(res.download_url, '_blank');
      }
    } catch (e) {
      console.error('Failed to get download URL', e);
    }
  };

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      {/* Top Breadcrumb & Actions */}
      <div className="flex items-center justify-between gap-4">
        <Link
          to="/doctor/patients"
          className="inline-flex items-center gap-1.5 text-xs font-medium text-slate-500 hover:text-slate-900 transition-colors"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          Back to Patient Roster
        </Link>
      </div>

      {/* Clinical Patient Header Card */}
      <div className="p-6 bg-white border border-slate-200 rounded-lg shadow-2xs flex flex-col md:flex-row md:items-center justify-between gap-6">
        <div className="flex items-center gap-4">
          <div className="w-14 h-14 rounded-xl bg-slate-900 text-white flex items-center justify-center text-lg font-bold font-mono shrink-0">
            {derivedPatientName
              .split(' ')
              .map((n) => n[0])
              .join('')
              .slice(0, 2)
              .toUpperCase()}
          </div>
          <div>
            <span className="font-mono text-xs uppercase tracking-wider text-slate-500 font-semibold">
              Longitudinal Clinical Record • Screen C4
            </span>
            <h1 className="text-2xl font-bold tracking-tight text-slate-900 mt-0.5">
              {derivedPatientName}
            </h1>
            <div className="flex items-center gap-2 mt-1 flex-wrap text-xs text-slate-500 font-mono">
              <span className="bg-slate-100 px-2 py-0.5 rounded border border-slate-200">
                Patient UUID: {patientId}
              </span>
              <span>•</span>
              <span className="text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded border border-emerald-200 font-sans font-medium">
                Verified Clinical Relationship
              </span>
            </div>
          </div>
        </div>

        {/* Primary Action Buttons */}
        <div className="flex items-center gap-2.5 flex-wrap">
          <button
            type="button"
            onClick={() => setIsVitalsModalOpen(true)}
            className="inline-flex items-center gap-1.5 px-3.5 py-2 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs cursor-pointer"
          >
            <Activity className="w-4 h-4 text-slate-500" />
            Record Vitals
          </button>
          <Link
            to={`/doctor/patients/${patientId}/prescribe/new`}
            className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 rounded-md transition-colors shadow-2xs"
          >
            <Plus className="w-4 h-4" />
            Write E-Prescription
          </Link>
        </div>
      </div>

      {/* Bento Tabs Header */}
      <div className="border-b border-slate-200 flex items-center gap-8 text-xs font-medium">
        <button
          onClick={() => setActiveTab('prescriptions')}
          className={`pb-3 transition-colors flex items-center gap-2 ${
            activeTab === 'prescriptions'
              ? 'text-slate-900 font-semibold border-b-2 border-slate-900'
              : 'text-slate-500 hover:text-slate-900'
          }`}
        >
          <FileText className="w-4 h-4" />
          Prescriptions History ({prescriptions.length})
        </button>
        <button
          onClick={() => setActiveTab('visits')}
          className={`pb-3 transition-colors flex items-center gap-2 ${
            activeTab === 'visits'
              ? 'text-slate-900 font-semibold border-b-2 border-slate-900'
              : 'text-slate-500 hover:text-slate-900'
          }`}
        >
          <Calendar className="w-4 h-4" />
          Consultation Visits ({appointments.length})
        </button>
        <button
          onClick={() => setActiveTab('vitals')}
          className={`pb-3 transition-colors flex items-center gap-2 ${
            activeTab === 'vitals'
              ? 'text-slate-900 font-semibold border-b-2 border-slate-900'
              : 'text-slate-500 hover:text-slate-900'
          }`}
        >
          <Activity className="w-4 h-4" />
          Vitals & Biometrics ({vitals.length})
        </button>
      </div>

      {/* Tab 1: Prescriptions History */}
      {activeTab === 'prescriptions' && (
        <div className="space-y-4">
          {isLoadingPrescriptions ? (
            <div className="space-y-3">
              {[1, 2, 3].map((i) => (
                <div key={i} className="h-24 bg-white border border-slate-200 rounded-lg animate-pulse" />
              ))}
            </div>
          ) : prescriptions.length === 0 ? (
            <EmptyState
              variant="no-records"
              title="No Prescriptions Issued"
              description="No clinical prescriptions have been issued for this patient yet."
              actionLabel="Write First Prescription"
              onAction={() => (window.location.href = `/doctor/patients/${patientId}/prescribe/new`)}
            />
          ) : (
            <div className="space-y-3">
              {prescriptions.map((rx) => (
                <div
                  key={rx.id}
                  className="p-5 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-3"
                >
                  <div className="flex flex-col md:flex-row md:items-center justify-between gap-2 border-b border-slate-100 pb-3">
                    <div className="flex items-center gap-2.5 flex-wrap">
                      <span className="font-mono text-xs font-semibold text-slate-900">
                        RX ID: {rx.id.slice(-8)}
                      </span>
                      <StatusBadge meta={getStatusMeta('prescription', rx.status)} size="sm" />
                      <span className="text-xs font-mono text-slate-500 bg-slate-100 px-2 py-0.5 rounded border border-slate-200 capitalize">
                        Source: {rx.source}
                      </span>
                    </div>

                    <div className="flex items-center gap-2">
                      <span className="text-xs text-slate-500 font-mono">
                        Issued: {formatDisplayDate(rx.created_at)}
                      </span>
                      <button
                        onClick={() => handleDownloadPrescription(rx.id)}
                        className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-50 transition-colors shadow-2xs"
                        title="Download authenticated PDF slip"
                      >
                        <Download className="w-3 h-3 text-slate-500" />
                        PDF Slip
                      </button>
                      <Link
                        to={`/verify/rx/${rx.id}`}
                        target="_blank"
                        className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-50 transition-colors"
                        title="Open public authenticity verification page"
                      >
                        Verify QR
                        <ExternalLink className="w-3 h-3 text-slate-400" />
                      </Link>
                    </div>
                  </div>

                  {/* Medication Line Items */}
                  <div className="space-y-2">
                    <h4 className="text-[11px] font-mono uppercase tracking-wider text-slate-500 font-semibold">
                      Prescribed Medication Regimen
                    </h4>
                    {rx.items && rx.items.length > 0 ? (
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
                        {rx.items.map((item) => (
                          <div
                            key={item.id}
                            className="p-3 bg-slate-50 border border-slate-200 rounded-md text-xs space-y-1"
                          >
                            <div className="font-bold text-slate-900 flex items-center justify-between">
                              <span>{item.medication_name}</span>
                              <span className="font-mono text-brand-700 font-medium">
                                {item.dosage || 'Standard dose'}
                              </span>
                            </div>
                            <div className="text-slate-600 font-mono text-[11px]">
                              {item.frequency || 'As directed'} • {item.duration || 'Full course'}
                            </div>
                            {item.instructions && (
                              <div className="text-slate-500 italic text-[11px] pt-1 border-t border-slate-200/60">
                                "{item.instructions}"
                              </div>
                            )}
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p className="text-xs text-slate-500 italic">No structured items recorded.</p>
                    )}
                  </div>

                  {rx.notes && (
                    <div className="text-xs text-slate-600 bg-slate-50/60 p-2.5 rounded border border-slate-200">
                      <span className="font-semibold text-slate-700">Clinical Notes: </span>
                      {rx.notes}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Tab 2: Consultation Visits */}
      {activeTab === 'visits' && (
        <div className="bg-white border border-slate-200 rounded-lg shadow-2xs overflow-hidden">
          {isLoadingAppointments ? (
            <div className="p-8 space-y-3">
              {[1, 2, 3].map((i) => (
                <div key={i} className="h-12 bg-slate-100 rounded animate-pulse" />
              ))}
            </div>
          ) : appointments.length === 0 ? (
            <div className="p-8">
              <EmptyState
                variant="calendar"
                title="No Consultation History"
                description="There are no past or upcoming appointments recorded with this patient."
              />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse text-xs">
                <thead>
                  <tr className="border-b border-slate-200 bg-slate-50/75 text-slate-500 font-mono uppercase tracking-wider">
                    <th className="py-3 px-4 font-semibold">Date & Time</th>
                    <th className="py-3 px-4 font-semibold">Modality</th>
                    <th className="py-3 px-4 font-semibold">Status</th>
                    <th className="py-3 px-4 font-semibold">Appt ID</th>
                    <th className="py-3 px-4 font-semibold text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {appointments.map((appt) => {
                    const slotTimes = formatSlotRange(appt.start_time, appt.end_time);

                    return (
                      <tr key={appt.id} className="hover:bg-slate-50/60 transition-colors">
                        <td className="py-3.5 px-4 font-mono">
                          <div className="font-semibold text-slate-900">
                            {formatDisplayDate(appt.start_time)}
                          </div>
                          <div className="text-slate-500 text-[11px]">{slotTimes.local}</div>
                        </td>

                        <td className="py-3.5 px-4">
                          <span className="inline-flex items-center gap-1.5 text-slate-700 capitalize font-medium">
                            {appt.type === 'virtual' ? (
                              <Video className="w-3.5 h-3.5 text-sky-600" />
                            ) : (
                              <Building2 className="w-3.5 h-3.5 text-slate-600" />
                            )}
                            {appt.type === 'virtual' ? 'Virtual' : 'In-Person'}
                          </span>
                        </td>

                        <td className="py-3.5 px-4">
                          <StatusBadge meta={getStatusMeta('appointment', appt.status)} size="sm" />
                        </td>

                        <td className="py-3.5 px-4 font-mono text-slate-400 text-[11px]">
                          {appt.id.slice(-8)}
                        </td>

                        <td className="py-3.5 px-4 text-right">
                          {appt.type === 'virtual' && appt.status === 'upcoming' && appt.meeting_link && (
                            <a
                              href={appt.meeting_link}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1 px-2.5 py-1 text-sky-700 bg-sky-50 border border-sky-200 rounded hover:bg-sky-100 transition-colors"
                            >
                              <Video className="w-3 h-3" />
                              Join Room
                            </a>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Tab 3: Longitudinal Vitals & Telemetry */}
      {activeTab === 'vitals' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <p className="text-xs text-slate-500">
              Longitudinal biometric measurements recorded by clinical staff and patient portal.
            </p>
            <button
              onClick={() => setIsVitalsModalOpen(true)}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-900 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs"
            >
              <Plus className="w-3.5 h-3.5" />
              Add Vitals Reading
            </button>
          </div>

          <div className="bg-white border border-slate-200 rounded-lg shadow-2xs overflow-hidden">
            {isLoadingVitals ? (
              <div className="p-8 space-y-3">
                {[1, 2, 3].map((i) => (
                  <div key={i} className="h-12 bg-slate-100 rounded animate-pulse" />
                ))}
              </div>
            ) : vitals.length === 0 ? (
              <div className="p-8">
                <EmptyState
                  variant="no-records"
                  title="No Vitals Recorded"
                  description="No biometric readings have been recorded for this patient yet."
                  actionLabel="Record Vitals Now"
                  onAction={() => setIsVitalsModalOpen(true)}
                />
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse text-xs">
                  <thead>
                    <tr className="border-b border-slate-200 bg-slate-50/75 text-slate-500 font-mono uppercase tracking-wider">
                      <th className="py-3 px-4 font-semibold">Recorded At</th>
                      <th className="py-3 px-4 font-semibold">Blood Pressure</th>
                      <th className="py-3 px-4 font-semibold">Heart Rate</th>
                      <th className="py-3 px-4 font-semibold">Blood Glucose</th>
                      <th className="py-3 px-4 font-semibold">SpO₂</th>
                      <th className="py-3 px-4 font-semibold">Temp / Wt</th>
                      <th className="py-3 px-4 font-semibold">Provenance</th>
                      <th className="py-3 px-4 font-semibold text-right">Alerts</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {vitals.map((v) => (
                      <tr key={v.id} className="hover:bg-slate-50/60 transition-colors">
                        <td className="py-3.5 px-4 font-mono">
                          <div className="font-semibold text-slate-900">
                            {formatDisplayDate(v.recorded_at)}
                          </div>
                        </td>

                        {/* Blood Pressure */}
                        <td className="py-3.5 px-4 font-mono">
                          {v.systolic_bp && v.diastolic_bp ? (
                            <span
                              className={`font-semibold ${
                                v.systolic_bp >= 140 || v.diastolic_bp >= 90
                                  ? 'text-rose-700'
                                  : 'text-slate-900'
                              }`}
                            >
                              {v.systolic_bp}/{v.diastolic_bp} mmHg
                            </span>
                          ) : (
                            <span className="text-slate-400">—</span>
                          )}
                        </td>

                        {/* Heart Rate */}
                        <td className="py-3.5 px-4 font-mono">
                          {v.heart_rate ? (
                            <span>{v.heart_rate} bpm</span>
                          ) : (
                            <span className="text-slate-400">—</span>
                          )}
                        </td>

                        {/* Blood Glucose */}
                        <td className="py-3.5 px-4 font-mono">
                          {v.blood_glucose ? (
                            <span>{v.blood_glucose} mg/dL</span>
                          ) : (
                            <span className="text-slate-400">—</span>
                          )}
                        </td>

                        {/* SpO2 */}
                        <td className="py-3.5 px-4 font-mono">
                          {v.oxygen_saturation ? (
                            <span
                              className={
                                v.oxygen_saturation < 95 ? 'text-amber-700 font-semibold' : ''
                              }
                            >
                              {v.oxygen_saturation}%
                            </span>
                          ) : (
                            <span className="text-slate-400">—</span>
                          )}
                        </td>

                        {/* Temp / Weight */}
                        <td className="py-3.5 px-4 font-mono text-slate-600">
                          {v.temperature ? `${v.temperature}°C` : '—'} /{' '}
                          {v.weight_kg ? `${v.weight_kg}kg` : '—'}
                        </td>

                        {/* Provenance */}
                        <td className="py-3.5 px-4">
                          <span className="text-[11px] font-mono text-slate-500 capitalize bg-slate-100 px-1.5 py-0.5 rounded border border-slate-200">
                            {v.recorder_role || 'staff'}
                          </span>
                        </td>

                        {/* Critical Alerts */}
                        <td className="py-3.5 px-4 text-right">
                          {v.is_critical ? (
                            <span
                              title={v.critical_alerts?.join(', ')}
                              className="inline-flex items-center gap-1 text-[11px] font-semibold text-rose-800 bg-rose-50 border border-rose-200 px-2 py-0.5 rounded"
                            >
                              <AlertTriangle className="w-3 h-3 text-rose-600" />
                              Critical Alert
                            </span>
                          ) : (
                            <span className="text-slate-400 font-mono text-[11px]">Normal</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Record Vitals Modal */}
      <RecordVitalsModal
        isOpen={isVitalsModalOpen}
        onClose={() => setIsVitalsModalOpen(false)}
        patientId={patientId}
        patientName={derivedPatientName}
        onSuccess={() => refetchVitals()}
      />
    </div>
  );
}
