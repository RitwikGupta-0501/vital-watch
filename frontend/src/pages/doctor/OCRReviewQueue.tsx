import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getPendingReviewPrescriptions } from '../../lib/api/prescriptions';
import type { Prescription, PaginatedResponse } from '../../lib/api/types';
import { useNotificationStream } from '../../hooks/useNotificationStream';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { getStatusMeta } from '../../lib/status';
import { formatDisplayDate } from '../../lib/datetime';
import { EmptyState } from '../../components/ui/EmptyState';
import {
  ClipboardCheck,
  Search,
  CheckCircle2,
  Sparkles,
  ArrowRight,
  ExternalLink,
  RefreshCw,
  AlertTriangle,
} from 'lucide-react';

export function OCRReviewQueue() {
  const queryClient = useQueryClient();
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'needs_review' | 'pending_ocr'>('all');
  const [providerFilter, setProviderFilter] = useState<string>('all');

  // 1. Fetch Pending Prescriptions
  const {
    data: prescriptionsData,
    isLoading,
    isError,
    refetch,
    isFetching,
  } = useQuery<PaginatedResponse<Prescription>>({
    queryKey: ['pending-review-prescriptions'],
    queryFn: () => getPendingReviewPrescriptions(50, 0),
    refetchInterval: 15000, // Background polling safety net
  });

  // 2. Real-Time SSE Listener for Asynchronous OCR Completion
  useNotificationStream({
    enabled: true,
    onEvent: (event) => {
      if (
        event.type === 'prescription.ocr_completed' ||
        event.type === 'prescription.approved' ||
        event.type === 'prescription.rejected'
      ) {
        queryClient.invalidateQueries({ queryKey: ['pending-review-prescriptions'] });
      }
    },
  });

  const prescriptions = prescriptionsData?.data || [];

  // Filter list
  const filteredPrescriptions = prescriptions.filter((rx) => {
    if (statusFilter !== 'all' && rx.status !== statusFilter) return false;
    if (providerFilter !== 'all') {
      const prov = (rx.ocr_provider || 'manual').toLowerCase();
      if (!prov.includes(providerFilter.toLowerCase())) return false;
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const hasPatientName = rx.patient_name?.toLowerCase().includes(q);
      const hasId = rx.id.toLowerCase().includes(q);
      const hasPatientId = rx.patient_id.toLowerCase().includes(q);
      const hasNote = rx.notes?.toLowerCase().includes(q);
      const hasMeds = rx.items?.some((it) => it.medication_name.toLowerCase().includes(q));
      if (!hasPatientName && !hasId && !hasPatientId && !hasNote && !hasMeds) return false;
    }
    return true;
  });

  const needsReviewCount = prescriptions.filter((rx) => rx.status === 'needs_review').length;
  const pendingOcrCount = prescriptions.filter((rx) => rx.status === 'pending_ocr').length;

  return (
    <div className="max-w-7xl mx-auto py-8 px-4 sm:px-6 space-y-6 font-sans">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-slate-200">
        <div>
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-mono text-slate-500 uppercase tracking-wider">
              Screen C9 • Clinical Verification Queue
            </span>
          </div>
          <h1 className="text-xl font-bold text-slate-900 tracking-tight flex items-center gap-2.5 mt-1">
            <ClipboardCheck className="w-5 h-5 text-brand-700" />
            Prescription Review Queue
          </h1>
          <p className="text-xs text-slate-600 mt-1 max-w-2xl">
            Human-in-the-Loop verification for multimodal prescription OCR. Review extracted medications, confirm dosages, and approve scripts into active longitudinal records.
          </p>
        </div>

        {/* Status Counters */}
        <div className="flex items-center gap-3">
          <div className="px-3.5 py-2 bg-amber-50 border border-amber-200 rounded-md text-xs">
            <div className="text-[10px] font-mono uppercase text-amber-800 font-semibold tracking-wider">
              Needs Review
            </div>
            <div className="text-lg font-bold font-mono text-amber-950">{needsReviewCount}</div>
          </div>

          <div className="px-3.5 py-2 bg-sky-50 border border-sky-200 rounded-md text-xs">
            <div className="text-[10px] font-mono uppercase text-sky-800 font-semibold tracking-wider">
              Pending OCR
            </div>
            <div className="text-lg font-bold font-mono text-sky-950">{pendingOcrCount}</div>
          </div>

          <button
            onClick={() => refetch()}
            disabled={isFetching}
            title="Refresh review queue"
            className="p-2.5 bg-white border border-slate-200 hover:bg-slate-50 text-slate-600 rounded-md transition-colors"
          >
            <RefreshCw className={`w-4 h-4 ${isFetching ? 'animate-spin text-brand-600' : ''}`} />
          </button>
        </div>
      </div>

      {/* Filter and Search Controls */}
      <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3">
        <div className="relative flex-1">
          <Search className="w-4 h-4 absolute left-3 top-2.5 text-slate-400" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search by patient name, medication, or prescription ID..."
            className="w-full pl-9 pr-3 py-1.5 text-xs text-slate-900 bg-white border border-slate-300 rounded focus:outline-hidden focus:ring-1 focus:ring-slate-900 font-sans"
          />
        </div>

        <div className="flex items-center gap-3">
          {/* Status Filter */}
          <div className="flex items-center gap-1.5">
            <span className="text-[11px] font-mono text-slate-500 uppercase">Status:</span>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as any)}
              className="text-xs text-slate-800 bg-white border border-slate-300 rounded px-2.5 py-1.5 focus:outline-hidden focus:ring-1 focus:ring-slate-900"
            >
              <option value="all">All Pending ({prescriptions.length})</option>
              <option value="needs_review">Needs Review ({needsReviewCount})</option>
              <option value="pending_ocr">In OCR Queue ({pendingOcrCount})</option>
            </select>
          </div>

          {/* Provider Filter */}
          <div className="flex items-center gap-1.5">
            <span className="text-[11px] font-mono text-slate-500 uppercase">Provider:</span>
            <select
              value={providerFilter}
              onChange={(e) => setProviderFilter(e.target.value)}
              className="text-xs text-slate-800 bg-white border border-slate-300 rounded px-2.5 py-1.5 focus:outline-hidden focus:ring-1 focus:ring-slate-900"
            >
              <option value="all">All Models</option>
              <option value="gemini">Gemini</option>
              <option value="claude">Claude</option>
              <option value="openai">OpenAI</option>
              <option value="manual">Manual / Disabled</option>
            </select>
          </div>
        </div>
      </div>

      {/* Main Table / Bento List */}
      <div className="bg-white border border-slate-200 rounded-lg shadow-2xs overflow-hidden">
        {isLoading ? (
          <div className="p-12 text-center text-xs font-mono text-slate-500 flex flex-col items-center gap-2">
            <RefreshCw className="w-5 h-5 animate-spin text-brand-600" />
            Loading clinical review queue...
          </div>
        ) : isError ? (
          <div className="p-8">
            <EmptyState
              variant="error"
              title="Failed to Load Review Queue"
              description="Could not synchronize pending prescriptions from the clinical backend."
              actionLabel="Retry Connection"
              onAction={() => refetch()}
            />
          </div>
        ) : filteredPrescriptions.length === 0 ? (
          <div className="p-12 text-center">
            <div className="w-12 h-12 bg-emerald-50 border border-emerald-200 rounded-full flex items-center justify-center text-emerald-700 mx-auto mb-3">
              <CheckCircle2 className="w-6 h-6" />
            </div>
            <h3 className="text-sm font-bold text-slate-900">Review Queue Cleared</h3>
            <p className="text-xs text-slate-500 max-w-sm mx-auto mt-1">
              All paper uploads and multimodal OCR extractions have been reviewed and approved or rejected.
            </p>
            <div className="mt-4 flex items-center justify-center gap-3">
              <Link
                to="/doctor/patients"
                className="px-3.5 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-300 rounded hover:bg-slate-50 transition-colors"
              >
                Go to Patient Roster
              </Link>
              <Link
                to="/doctor/prescriptions"
                className="px-3.5 py-1.5 text-xs font-medium text-white bg-slate-900 hover:bg-slate-800 rounded transition-colors"
              >
                View Approved Prescriptions
              </Link>
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50/70 text-[11px] font-mono uppercase tracking-wider text-slate-500">
                  <th className="py-3 px-4 font-semibold">Patient & Chart</th>
                  <th className="py-3 px-4 font-semibold">Received</th>
                  <th className="py-3 px-4 font-semibold">OCR Engine</th>
                  <th className="py-3 px-4 font-semibold">Extracted Items</th>
                  <th className="py-3 px-4 font-semibold">Status</th>
                  <th className="py-3 px-4 font-semibold text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-xs">
                {filteredPrescriptions.map((rx) => {
                  const itemCount = rx.items?.length || 0;
                  const isPendingOCR = rx.status === 'pending_ocr';
                  const hasFailureNote = rx.notes?.includes('[OCR') || rx.notes?.includes('[Storage');

                  return (
                    <tr
                      key={rx.id}
                      className="hover:bg-slate-50/60 transition-colors group"
                    >
                      {/* Patient Info */}
                      <td className="py-3.5 px-4">
                        <div className="font-semibold text-slate-900">
                          {rx.patient_name || 'Patient'}
                        </div>
                        <div className="flex items-center gap-2 mt-0.5">
                          <Link
                            to={`/doctor/patients/${rx.patient_id}`}
                            className="text-[11px] font-mono text-brand-700 hover:underline flex items-center gap-0.5"
                          >
                            Chart: {rx.patient_id.slice(0, 8)}...
                            <ExternalLink className="w-2.5 h-2.5" />
                          </Link>
                        </div>
                      </td>

                      {/* Received Timestamp */}
                      <td className="py-3.5 px-4 font-mono text-slate-600">
                        {formatDisplayDate(rx.created_at)}
                      </td>

                      {/* OCR Provider */}
                      <td className="py-3.5 px-4">
                        {isPendingOCR ? (
                          <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-mono bg-sky-50 text-sky-800 border border-sky-200">
                            <span className="w-1.5 h-1.5 rounded-full bg-sky-500 animate-pulse" />
                            River Worker...
                          </span>
                        ) : rx.ocr_provider ? (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-mono bg-slate-100 text-slate-800 border border-slate-200">
                            <Sparkles className="w-3 h-3 text-brand-600" />
                            {rx.ocr_provider}
                          </span>
                        ) : (
                          <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-mono bg-slate-100 text-slate-600 border border-slate-200">
                            Manual / Fallback
                          </span>
                        )}
                      </td>

                      {/* Extracted Items */}
                      <td className="py-3.5 px-4">
                        {isPendingOCR ? (
                          <span className="text-slate-400 italic">Processing document...</span>
                        ) : itemCount > 0 ? (
                          <div className="space-y-0.5">
                            <span className="font-medium text-slate-900">
                              {itemCount} medication {itemCount === 1 ? 'item' : 'items'}
                            </span>
                            <div className="text-[11px] text-slate-500 truncate max-w-xs font-mono">
                              {rx.items?.map((it) => it.medication_name).join(', ')}
                            </div>
                          </div>
                        ) : (
                          <div className="flex items-center gap-1.5 text-amber-800">
                            <AlertTriangle className="w-3.5 h-3.5 shrink-0 text-amber-600" />
                            <span className="font-medium text-[11px]">
                              {hasFailureNote ? 'Extraction failed' : '0 items detected'} (Manual Entry)
                            </span>
                          </div>
                        )}
                      </td>

                      {/* Status */}
                      <td className="py-3.5 px-4">
                        <StatusBadge meta={getStatusMeta('prescription', rx.status)} size="sm" />
                      </td>

                      {/* Action */}
                      <td className="py-3.5 px-4 text-right">
                        <Link
                          to={`/doctor/review/${rx.id}`}
                          className={`inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded transition-colors shadow-2xs ${
                            isPendingOCR
                              ? 'bg-slate-100 text-slate-700 border border-slate-300 hover:bg-slate-200'
                              : 'bg-slate-900 text-white hover:bg-slate-800'
                          }`}
                        >
                          {isPendingOCR ? 'View Progress' : 'Review & Verify'}
                          <ArrowRight className="w-3.5 h-3.5" />
                        </Link>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
