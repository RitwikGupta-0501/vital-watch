import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchApi } from '../../lib/api/client';
import type { Prescription, PrescriptionDownloadResponse } from '../../lib/api/types';
import { PRESCRIPTION_STATUS_MAP } from '../../lib/status';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { EmptyState } from '../../components/ui/EmptyState';
import { formatDisplayDate } from '../../lib/datetime';
import { downloadFromPresignedUrl } from '../../lib/storage';
import {
  ArrowLeft,
  Download,
  ShieldCheck,
  Calendar,
  Pill,
  ExternalLink,
  Clock,
  FileCheck2,
} from 'lucide-react';

export function PrescriptionDetail() {
  const { id } = useParams<{ id: string }>();
  const [isDownloading, setIsDownloading] = useState(false);

  const { data: prescription, isLoading, error } = useQuery({
    queryKey: ['prescription-detail', id],
    queryFn: () => fetchApi<Prescription>(`/api/prescriptions/${id}`),
    enabled: !!id,
  });

  const handleDownloadPdf = async () => {
    if (!id) return;
    try {
      setIsDownloading(true);
      const res = await fetchApi<PrescriptionDownloadResponse>(
        `/api/prescriptions/${id}/download-url`
      );
      if (res.download_url) {
        downloadFromPresignedUrl(
          res.download_url,
          res.file_name || prescription?.file_name || 'prescription.pdf'
        );
      }
    } catch (err) {
      console.error('Failed to download prescription PDF:', err);
    } finally {
      setIsDownloading(false);
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-6 animate-pulse max-w-4xl mx-auto py-8">
        <div className="h-4 bg-slate-100 rounded w-24" />
        <div className="h-10 bg-slate-100 rounded w-1/3" />
        <div className="h-48 bg-slate-100 rounded-3xl" />
      </div>
    );
  }

  if (error || !prescription) {
    return (
      <div className="max-w-2xl mx-auto py-12">
        <EmptyState
          variant="error"
          title="Prescription Not Found"
          description="The requested prescription record does not exist or has not yet been approved."
          actionLabel="Back to Prescriptions"
          onAction={() => (window.location.href = '/patient/prescriptions')}
          isPatientTheme
        />
      </div>
    );
  }

  const items = prescription.items || [];

  return (
    <div className="max-w-4xl mx-auto space-y-8 animate-in fade-in duration-300">
      {/* Back Link */}
      <div>
        <Link
          to="/patient/prescriptions"
          className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-500 hover:text-slate-900 transition-colors"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          <span>Back to Prescription Records</span>
        </Link>
      </div>

      {/* Main Clinical Document Header Card */}
      <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4 border-b border-border pb-6">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <StatusBadge
                meta={
                  PRESCRIPTION_STATUS_MAP[prescription.status] ||
                  PRESCRIPTION_STATUS_MAP.approved
                }
                size="sm"
                isPatientTheme
              />
              <span className="font-mono text-xs text-slate-400">
                Prescription #{prescription.id.slice(0, 8)}
              </span>
            </div>
            <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-slate-900 mt-2">
              Prescription Slip
            </h1>
            <p className="text-xs text-slate-500">
              Authored by Dr. {prescription.doctor_name || 'Prescribing Physician'}
            </p>
          </div>

          <div className="flex flex-col sm:items-end gap-1 text-xs text-slate-500 font-mono">
            <div className="flex items-center gap-1.5">
              <Calendar className="w-3.5 h-3.5 text-slate-400" />
              <span>Issued: {formatDisplayDate(prescription.created_at)}</span>
            </div>
            {prescription.expires_at && (
              <div className="flex items-center gap-1.5 text-amber-800">
                <Clock className="w-3.5 h-3.5" />
                <span>Expires: {formatDisplayDate(prescription.expires_at)}</span>
              </div>
            )}
          </div>
        </div>

        {/* Action Buttons Panel */}
        <div className="flex flex-wrap items-center gap-3">
          <Button
            isPatientTheme
            size="md"
            onClick={handleDownloadPdf}
            disabled={isDownloading}
            className="gap-2 font-medium shadow-sm"
          >
            <Download className="w-4 h-4" />
            <span>{isDownloading ? 'Fetching Secure PDF...' : 'Download Official PDF'}</span>
          </Button>

          <Link to={`/verify/rx/${prescription.id}`} target="_blank" rel="noopener noreferrer">
            <Button variant="outline" isPatientTheme size="md" className="gap-2 text-xs">
              <ShieldCheck className="w-4 h-4 text-brand-700" />
              <span>Verify Digital Authenticity</span>
              <ExternalLink className="w-3 h-3 text-slate-400" />
            </Button>
          </Link>
        </div>
      </Card>

      {/* Structured Medication Schedule Table */}
      <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border space-y-6">
        <div className="flex items-center gap-2 border-b border-border pb-4">
          <Pill className="w-5 h-5 text-brand-700" />
          <h2 className="text-base font-bold text-slate-900 tracking-tight">
            Prescribed Medications ({items.length})
          </h2>
        </div>

        {items.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="border-b border-border text-slate-400 font-semibold uppercase tracking-wider text-[10px]">
                  <th className="py-3 px-3">Medication</th>
                  <th className="py-3 px-3">Dosage</th>
                  <th className="py-3 px-3">Frequency</th>
                  <th className="py-3 px-3">Duration</th>
                  <th className="py-3 px-3">Timing</th>
                  <th className="py-3 px-3">Instructions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/60">
                {items.map((item) => (
                  <tr key={item.id} className="hover:bg-slate-50/50 transition-colors">
                    <td className="py-3 px-3 font-semibold text-slate-900">
                      {item.medication_name}
                    </td>
                    <td className="py-3 px-3 font-mono font-medium text-slate-700">
                      {item.dosage || '—'}
                    </td>
                    <td className="py-3 px-3 text-slate-600">
                      {item.frequency || '—'}
                    </td>
                    <td className="py-3 px-3 text-slate-600">
                      {item.duration || '—'}
                    </td>
                    <td className="py-3 px-3 text-slate-600">
                      {item.timing || '—'}
                    </td>
                    <td className="py-3 px-3 text-slate-500 italic max-w-xs">
                      {item.instructions || 'Follow physician directions.'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-xs text-slate-500 italic py-4">
            No itemized entries found for this prescription.
          </p>
        )}
      </Card>

      {/* Clinical Notes Section */}
      {prescription.notes && (
        <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border space-y-3">
          <div className="flex items-center gap-2">
            <FileCheck2 className="w-4 h-4 text-brand-700" />
            <h3 className="text-sm font-bold text-slate-900 tracking-tight">
              Physician's Instructions & Notes
            </h3>
          </div>
          <p className="text-xs text-slate-600 leading-relaxed whitespace-pre-line bg-slate-50 p-4 rounded-2xl border border-slate-200/60 font-sans">
            {prescription.notes}
          </p>
        </Card>
      )}
    </div>
  );
}
