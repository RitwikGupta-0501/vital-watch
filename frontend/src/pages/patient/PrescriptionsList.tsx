import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchApi } from '../../lib/api/client';
import type {
  Prescription,
  PaginatedResponse,
  PrescriptionDownloadResponse,
} from '../../lib/api/types';
import { PRESCRIPTION_STATUS_MAP } from '../../lib/status';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { EmptyState } from '../../components/ui/EmptyState';
import { formatDisplayDate } from '../../lib/datetime';
import { downloadFromPresignedUrl } from '../../lib/storage';
import {
  Download,
  Calendar,
  Pill,
  ArrowRight,
  ShieldCheck,
} from 'lucide-react';

export function PrescriptionsList() {
  const [downloadingId, setDownloadingId] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ['patient-prescriptions'],
    queryFn: () =>
      fetchApi<PaginatedResponse<Prescription>>('/api/patient/prescriptions?limit=20&offset=0'),
  });

  const prescriptions = data?.data || [];

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
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-900">
            Prescription Records
          </h1>
          <p className="text-sm text-slate-500 mt-1">
            Access authorized clinical prescriptions, medication directions, and authenticated slips.
          </p>
        </div>

        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-100 text-slate-600 text-xs font-mono self-start sm:self-auto border border-slate-200">
          <ShieldCheck className="w-3.5 h-3.5 text-brand-700" />
          <span>Approved Records Only</span>
        </div>
      </div>

      {/* Prescription Cards */}
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
          title="Unable to Load Prescriptions"
          description="Failed to retrieve prescription ledger. Please check your connection."
          isPatientTheme
        />
      ) : prescriptions.length > 0 ? (
        <div className="space-y-4">
          {prescriptions.map((rx) => {
            const hasItems = rx.items && rx.items.length > 0;
            return (
              <Card
                key={rx.id}
                className="rounded-3xl p-6 bg-white border border-border hover:border-slate-300 transition-all flex flex-col md:flex-row md:items-center justify-between gap-6"
              >
                <div className="space-y-3 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge
                      meta={PRESCRIPTION_STATUS_MAP[rx.status] || PRESCRIPTION_STATUS_MAP.approved}
                      size="sm"
                      isPatientTheme
                    />
                    <span className="font-mono text-[11px] text-slate-400">
                      ID: {rx.id.slice(0, 8)}...
                    </span>
                  </div>

                  <div>
                    <h3 className="text-base sm:text-lg font-bold text-slate-900 tracking-tight">
                      {rx.doctor_name ? `Dr. ${rx.doctor_name}` : 'Prescribing Physician'}
                    </h3>
                    <div className="flex items-center gap-2 text-xs text-slate-500 mt-1">
                      <Calendar className="w-3.5 h-3.5 text-slate-400" />
                      <span>Issued: {formatDisplayDate(rx.created_at)}</span>
                      {rx.expires_at && (
                        <>
                          <span>•</span>
                          <span>Expires: {formatDisplayDate(rx.expires_at)}</span>
                        </>
                      )}
                    </div>
                  </div>

                  {/* Medications Pills */}
                  {hasItems ? (
                    <div className="flex flex-wrap items-center gap-2 pt-1">
                      {rx.items.map((item, idx) => (
                        <span
                          key={item.id || idx}
                          className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium bg-slate-100 text-slate-800 border border-slate-200"
                        >
                          <Pill className="w-3 h-3 text-brand-700" />
                          <span>{item.medication_name}</span>
                          {item.dosage && (
                            <span className="text-slate-400 font-mono text-[11px]">
                              {item.dosage}
                            </span>
                          )}
                        </span>
                      ))}
                    </div>
                  ) : rx.notes ? (
                    <p className="text-xs text-slate-600 line-clamp-1 italic">
                      Notes: {rx.notes}
                    </p>
                  ) : null}
                </div>

                {/* Actions Panel */}
                <div className="flex items-center gap-3 shrink-0">
                  <Button
                    variant="outline"
                    isPatientTheme
                    size="sm"
                    disabled={downloadingId === rx.id}
                    onClick={() => handleDownloadPdf(rx.id, rx.file_name)}
                    className="gap-2 text-xs"
                  >
                    <Download className="w-3.5 h-3.5" />
                    <span>{downloadingId === rx.id ? 'Preparing...' : 'Download PDF'}</span>
                  </Button>

                  <Link to={`/patient/prescriptions/${rx.id}`}>
                    <Button isPatientTheme size="sm" className="gap-1.5 text-xs">
                      <span>View Details</span>
                      <ArrowRight className="w-3.5 h-3.5" />
                    </Button>
                  </Link>
                </div>
              </Card>
            );
          })}
        </div>
      ) : (
        <EmptyState
          variant="no-records"
          title="No Prescriptions Issued"
          description="You currently have no active prescription records on file with the clinic."
          isPatientTheme
        />
      )}
    </div>
  );
}
