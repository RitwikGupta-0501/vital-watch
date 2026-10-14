import { useParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchApi, RateLimitError } from '../lib/api/client';
import type { PublicRxVerification, PrescriptionItem } from '../lib/api/types';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { formatDisplayDate } from '../lib/datetime';
import {
  ShieldCheck,
  ShieldAlert,
  AlertTriangle,
  Pill,
  Lock,
} from 'lucide-react';

export function VerifyRx() {
  const { id } = useParams<{ id: string }>();

  const { data, isLoading, error } = useQuery({
    queryKey: ['public-rx-verify', id],
    queryFn: () => fetchApi<PublicRxVerification>(`/verify/rx/${id}`),
    enabled: !!id,
    retry: false,
  });

  const isRateLimited = error instanceof RateLimitError;
  const items: PrescriptionItem[] = data?.items || [];

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col justify-between p-4 sm:p-8 animate-in fade-in duration-300">
      <div className="max-w-2xl mx-auto w-full space-y-6 pt-4 sm:pt-8">
        {/* Verification Brand Badge */}
        <div className="flex items-center justify-between pb-4 border-b border-border">
          <Link to="/" className="flex items-center gap-2">
            <div className="w-8 h-8 bg-brand-700 rounded-lg flex items-center justify-center text-white font-bold text-base">
              V
            </div>
            <div>
              <span className="font-bold text-base tracking-tight text-slate-900 block leading-tight">
                VitalWatch
              </span>
              <span className="text-[10px] text-slate-400 font-mono block">
                Official Authenticity Service
              </span>
            </div>
          </Link>

          <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-mono font-medium bg-slate-100 text-slate-700 border border-slate-200">
            <Lock className="w-3 h-3 text-brand-700" />
            <span>Digital Certificate</span>
          </span>
        </div>

        {/* Content Body */}
        {isLoading ? (
          <Card className="rounded-3xl p-8 bg-white border border-border animate-pulse space-y-4">
            <div className="w-12 h-12 bg-slate-100 rounded-full" />
            <div className="h-6 bg-slate-100 rounded w-1/3" />
            <div className="h-24 bg-slate-100 rounded-2xl" />
          </Card>
        ) : isRateLimited ? (
          <Card className="rounded-3xl p-8 bg-white border border-amber-200 space-y-4 text-center">
            <div className="w-12 h-12 rounded-full bg-amber-50 text-amber-700 flex items-center justify-center mx-auto">
              <AlertTriangle className="w-6 h-6" />
            </div>
            <h1 className="text-xl font-bold text-slate-900">
              Verification Rate Limit Reached
            </h1>
            <p className="text-xs text-slate-500 max-w-md mx-auto leading-relaxed">
              Security rate limit engaged. To safeguard clinical registries against automated scraping, public scans are throttled. Please wait before refreshing this verification.
            </p>
            <Button
              variant="outline"
              size="sm"
              isPatientTheme
              onClick={() => window.location.reload()}
              className="mt-2"
            >
              Retry Verification
            </Button>
          </Card>
        ) : error || !data ? (
          <Card className="rounded-3xl p-8 bg-white border border-rose-200 space-y-4 text-center">
            <div className="w-12 h-12 rounded-full bg-rose-50 text-rose-700 flex items-center justify-center mx-auto">
              <ShieldAlert className="w-6 h-6" />
            </div>
            <h1 className="text-xl font-bold text-slate-900">
              Prescription Not Verified
            </h1>
            <p className="text-xs text-slate-500 max-w-md mx-auto leading-relaxed">
              This prescription identifier does not match an approved, active record in our clinical ledger, or may have been revoked.
            </p>
            <div className="pt-2">
              <Link to="/">
                <Button isPatientTheme size="sm">
                  Return to VitalWatch
                </Button>
              </Link>
            </div>
          </Card>
        ) : (
          <Card className="rounded-3xl p-6 sm:p-8 bg-white border border-border space-y-6 shadow-xs">
            {/* Authenticity Certificate Seal */}
            <div className="flex items-center gap-3 p-4 rounded-2xl bg-emerald-50 border border-emerald-200">
              <div className="w-10 h-10 rounded-full bg-emerald-100 text-emerald-800 flex items-center justify-center shrink-0">
                <ShieldCheck className="w-6 h-6" />
              </div>
              <div>
                <h2 className="text-sm font-bold text-emerald-950">
                  Authentic Clinical Prescription
                </h2>
                <p className="text-xs text-emerald-800">
                  Digitally issued & verified by an authorized medical provider.
                </p>
              </div>
            </div>

            {/* Verification Metadata Grid */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
              <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200/60 space-y-1">
                <span className="text-slate-400 font-medium">Masked Patient ID</span>
                <p className="font-mono font-semibold text-slate-900 text-sm">
                  {data.masked_patient_id}
                </p>
              </div>

              <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200/60 space-y-1">
                <span className="text-slate-400 font-medium">Issue Timestamp</span>
                <p className="font-mono font-semibold text-slate-900 text-sm">
                  {formatDisplayDate(data.created_at)}
                </p>
              </div>

              <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200/60 space-y-1 sm:col-span-2">
                <span className="text-slate-400 font-medium">Authorized Prescriber</span>
                <p className="font-semibold text-slate-900 text-sm">
                  {data.doctor_name ? `Dr. ${data.doctor_name}` : `Clinician ID: ${data.doctor_id}`}
                </p>
              </div>
            </div>

            {/* Prescribed Medications Table */}
            <div className="space-y-3 pt-2">
              <div className="flex items-center gap-2">
                <Pill className="w-4 h-4 text-brand-700" />
                <h3 className="text-xs font-bold uppercase tracking-wider text-slate-700">
                  Verified Medication List ({items.length})
                </h3>
              </div>

              {items.length > 0 ? (
                <div className="overflow-x-auto rounded-2xl border border-slate-200">
                  <table className="w-full text-left border-collapse text-xs">
                    <thead>
                      <tr className="bg-slate-50 border-b border-slate-200 text-slate-400 font-semibold uppercase tracking-wider text-[10px]">
                        <th className="py-2.5 px-3">Medication</th>
                        <th className="py-2.5 px-3">Dosage</th>
                        <th className="py-2.5 px-3">Frequency</th>
                        <th className="py-2.5 px-3">Instructions</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-100">
                      {items.map((item: PrescriptionItem, idx: number) => (
                        <tr key={item.id || idx}>
                          <td className="py-2.5 px-3 font-semibold text-slate-900">
                            {item.medication_name}
                          </td>
                          <td className="py-2.5 px-3 font-mono text-slate-700">
                            {item.dosage || '—'}
                          </td>
                          <td className="py-2.5 px-3 text-slate-600">
                            {item.frequency || '—'}
                          </td>
                          <td className="py-2.5 px-3 text-slate-500 italic">
                            {item.instructions || 'Standard dosage'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-xs text-slate-500 italic">
                  Digital prescription record has no itemized drugs.
                </p>
              )}
            </div>

            {/* Cleaned Notes */}
            {data.notes && (
              <div className="p-4 rounded-2xl bg-slate-50 border border-slate-200/60 text-xs text-slate-600 leading-relaxed">
                <span className="font-semibold block text-slate-900 mb-1">
                  Physician Directions:
                </span>
                {data.notes}
              </div>
            )}
          </Card>
        )}
      </div>

      {/* Trust Footer */}
      <footer className="text-center py-6 text-[11px] text-slate-400 font-mono">
        VitalWatch Digital Verification Protocol • Verified via Cryptographic Audit Trail
      </footer>
    </div>
  );
}
