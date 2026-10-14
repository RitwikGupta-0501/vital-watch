import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchApi } from '../../lib/api/client';
import type { Prescription, DoctorPatient, PaginatedResponse, PrescriptionStatus, PrescriptionSource } from '../../lib/api/types';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { getStatusMeta } from '../../lib/status';
import { formatDisplayDate } from '../../lib/datetime';
import { EmptyState } from '../../components/ui/EmptyState';
import {
  Download,
  ExternalLink,
  Plus,
  User,
  Filter,
  Search
} from 'lucide-react';

export function DoctorPrescriptionsList() {
  const [selectedPatientId, setSelectedPatientId] = useState<string>('');
  const [statusFilter, setStatusFilter] = useState<'all' | PrescriptionStatus>('all');
  const [sourceFilter, setSourceFilter] = useState<'all' | PrescriptionSource>('all');
  const [searchQuery, setSearchQuery] = useState('');

  // 1. Fetch Patients Roster for selector
  const { data: patientsData } = useQuery<PaginatedResponse<DoctorPatient>>({
    queryKey: ['doctor-patients-selector'],
    queryFn: () => fetchApi<PaginatedResponse<DoctorPatient>>('/api/doctor/patients?limit=100&offset=0'),
  });

  const patients = patientsData?.data || [];

  // Default to first patient if none selected yet
  const effectivePatientId = selectedPatientId || (patients[0]?.id ?? '');

  // 2. Fetch Prescriptions for Selected Patient
  const {
    data: prescriptionsData,
    isLoading,
    isError,
    refetch,
  } = useQuery<PaginatedResponse<Prescription>>({
    queryKey: ['doctor-patient-prescriptions', effectivePatientId],
    queryFn: () =>
      fetchApi<PaginatedResponse<Prescription>>(
        `/api/doctor/patients/${effectivePatientId}/prescriptions?limit=50&offset=0`
      ),
    enabled: !!effectivePatientId,
  });

  const prescriptions = prescriptionsData?.data || [];

  const filteredPrescriptions = prescriptions.filter((rx) => {
    if (statusFilter !== 'all' && rx.status !== statusFilter) return false;
    if (sourceFilter !== 'all' && rx.source !== sourceFilter) return false;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const hasMed = rx.items?.some((it) => it.medication_name.toLowerCase().includes(q));
      const hasNotes = rx.notes?.toLowerCase().includes(q);
      const hasId = rx.id.toLowerCase().includes(q);
      if (!hasMed && !hasNotes && !hasId) return false;
    }
    return true;
  });

  const handleDownloadPDF = async (rxId: string) => {
    try {
      const res = await fetchApi<{ download_url: string }>(`/api/prescriptions/${rxId}/download-url`);
      if (res.download_url) {
        window.open(res.download_url, '_blank');
      }
    } catch (e) {
      console.error('Failed to download PDF', e);
    }
  };

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-200">
        <div>
          <span className="font-mono text-xs uppercase tracking-wider text-slate-500 font-semibold">
            Clinical Pharmacy • Screen C11
          </span>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900 mt-1">
            Prescriptions Registry
          </h1>
          <p className="text-sm text-slate-500 mt-0.5">
            Audit log of issued digital e-prescriptions and paper slip digitizations by patient
          </p>
        </div>

        {effectivePatientId && (
          <Link
            to={`/doctor/patients/${effectivePatientId}/prescribe/new`}
            className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 rounded-md transition-colors shadow-2xs"
          >
            <Plus className="w-4 h-4" />
            Write E-Prescription
          </Link>
        )}
      </div>

      {/* Patient Selector & Filters */}
      <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 flex-wrap">
          {/* Patient Selector */}
          <div className="flex items-center gap-2">
            <User className="w-4 h-4 text-slate-500 shrink-0" />
            <label className="text-xs font-semibold text-slate-700 shrink-0">
              Select Patient:
            </label>
            <select
              value={effectivePatientId}
              onChange={(e) => setSelectedPatientId(e.target.value)}
              className="px-3 py-1.5 text-xs bg-slate-50 border border-slate-200 rounded-md focus:outline-none focus:bg-white text-slate-900 font-medium min-w-[200px]"
            >
              {patients.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.first_name} {p.last_name} ({p.id.slice(-6)})
                </option>
              ))}
            </select>
          </div>

          {/* Search Bar */}
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Filter by drug, note, RX ID..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-8 pr-3 py-1.5 text-xs bg-slate-50 border border-slate-200 rounded-md focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400 w-52 md:w-64 font-mono"
            />
          </div>
        </div>

        {/* Filter Badges & Modality */}
        <div className="flex items-center justify-between gap-3 pt-3 border-t border-slate-100 flex-wrap">
          {/* Status Tabs */}
          <div className="flex items-center gap-1 bg-slate-100 p-1 rounded-md border border-slate-200 text-xs font-medium">
            {(['all', 'approved', 'needs_review', 'pending_ocr', 'rejected'] as const).map((s) => (
              <button
                key={s}
                onClick={() => setStatusFilter(s)}
                className={`px-3 py-1 rounded transition-colors capitalize ${
                  statusFilter === s
                    ? 'bg-white text-slate-900 shadow-2xs font-semibold'
                    : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                {s.replace('_', ' ')}
              </button>
            ))}
          </div>

          {/* Source Dropdown */}
          <div className="flex items-center gap-1.5 text-xs text-slate-500">
            <Filter className="w-3.5 h-3.5" />
            <select
              value={sourceFilter}
              onChange={(e) => setSourceFilter(e.target.value as 'all' | PrescriptionSource)}
              className="px-2.5 py-1 text-xs bg-white border border-slate-200 rounded-md focus:outline-none text-slate-700"
            >
              <option value="all">All Sources</option>
              <option value="digital">Digital Only</option>
              <option value="uploaded">Uploaded Slip Only</option>
            </select>
          </div>
        </div>
      </div>

      {/* Prescriptions List */}
      <div className="space-y-3">
        {isLoading ? (
          <div className="space-y-3">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-28 bg-white border border-slate-200 rounded-lg animate-pulse" />
            ))}
          </div>
        ) : isError ? (
          <EmptyState
            variant="error"
            title="Failed to Retrieve Prescriptions"
            description="Could not synchronize clinical prescription records with the server."
            actionLabel="Retry"
            onAction={() => refetch()}
          />
        ) : filteredPrescriptions.length === 0 ? (
          <EmptyState
            variant="no-records"
            title="No Prescriptions Found"
            description={
              searchQuery || statusFilter !== 'all' || sourceFilter !== 'all'
                ? 'No prescriptions match the active filter criteria.'
                : 'No prescriptions have been issued for this patient yet.'
            }
            actionLabel={effectivePatientId ? 'Write E-Prescription' : undefined}
            onAction={
              effectivePatientId
                ? () => (window.location.href = `/doctor/patients/${effectivePatientId}/prescribe/new`)
                : undefined
            }
          />
        ) : (
          filteredPrescriptions.map((rx) => (
            <div
              key={rx.id}
              className="p-5 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-3"
            >
              {/* Header Row */}
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-2 border-b border-slate-100 pb-3">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-mono text-xs font-bold text-slate-900">
                    RX ID: {rx.id.slice(-8)}
                  </span>
                  <StatusBadge meta={getStatusMeta('prescription', rx.status)} size="sm" />
                  <span className="text-xs font-mono text-slate-500 bg-slate-100 px-2 py-0.5 rounded border border-slate-200 capitalize">
                    {rx.source}
                  </span>
                  {rx.patient_name && (
                    <span className="text-xs text-slate-600 font-medium">
                      • Patient: {rx.patient_name}
                    </span>
                  )}
                </div>

                <div className="flex items-center gap-2">
                  <span className="text-xs text-slate-500 font-mono">
                    Issued: {formatDisplayDate(rx.created_at)}
                  </span>

                  <button
                    onClick={() => handleDownloadPDF(rx.id)}
                    className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-50 transition-colors shadow-2xs"
                    title="Download prescription PDF"
                  >
                    <Download className="w-3 h-3 text-slate-500" />
                    PDF
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

                  <Link
                    to={`/doctor/patients/${rx.patient_id}`}
                    className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-50 transition-colors"
                  >
                    Chart
                  </Link>
                </div>
              </div>

              {/* Items List */}
              <div className="space-y-1.5">
                <div className="text-[11px] font-mono uppercase tracking-wider text-slate-500 font-semibold">
                  Medication Items ({rx.items?.length || 0})
                </div>
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
                  <p className="text-xs text-slate-500 italic">No items recorded.</p>
                )}
              </div>

              {/* Notes */}
              {rx.notes && (
                <div className="text-xs text-slate-600 bg-slate-50/60 p-2.5 rounded border border-slate-200">
                  <span className="font-semibold text-slate-700">Clinical Notes: </span>
                  {rx.notes}
                </div>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
