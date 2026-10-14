import React, { useState } from 'react';
import type { SafetyReport } from '../../lib/api/types';
import { Modal } from '../ui/Modal';
import { StatusBadge } from '../ui/StatusBadge';
import { getStatusMeta } from '../../lib/status';
import { ShieldAlert, AlertCircle, CheckCircle2, X } from 'lucide-react';

interface SafetyInterruptModalProps {
  isOpen: boolean;
  onClose: () => void;
  safetyReport: SafetyReport | null;
  errorMessage?: string;
  onAuthorizeOverride: (overrideReason: string) => void;
  isPending?: boolean;
}

export function SafetyInterruptModal({
  isOpen,
  onClose,
  safetyReport,
  errorMessage,
  onAuthorizeOverride,
  isPending = false,
}: SafetyInterruptModalProps) {
  const [overrideReason, setOverrideReason] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);

  if (!isOpen || !safetyReport) return null;

  const handleAuthorize = (e: React.FormEvent) => {
    e.preventDefault();
    if (overrideReason.trim().length < 10) {
      setValidationError('A clinical justification of at least 10 characters is required to override safety alerts.');
      return;
    }
    setValidationError(null);
    onAuthorizeOverride(overrideReason.trim());
  };

  const interactionAlerts = safetyReport.interaction_alerts || [];
  const isDegraded = safetyReport.service_degraded;

  return (
    <Modal
      isOpen={isOpen}
      onClose={() => {
        /* Intentionally prevent casual closing without explicit cancel or override */
      }}
      title="Clinical Safety Contraindication Detected"
      maxWidth="lg"
      isPatientTheme={false}
    >
      <div className="space-y-5">
        {/* Urgent Warning Header */}
        <div className="p-4 bg-rose-50 border border-rose-200 rounded-lg space-y-2">
          <div className="flex items-start gap-3">
            <ShieldAlert className="w-5 h-5 text-rose-700 shrink-0 mt-0.5" />
            <div>
              <h4 className="text-sm font-bold text-rose-900">
                {errorMessage || 'Potential Drug Interaction Warning (OpenFDA)'}
              </h4>
              <p className="text-xs text-rose-800 mt-1">
                The prescribed medication regimen has triggered an automated safety interrupt against active patient therapies. Medical supervision and written justification are required to proceed.
              </p>
            </div>
          </div>
        </div>

        {/* Degraded Service Warning */}
        {isDegraded && (
          <div className="p-3 bg-amber-50 border border-amber-200 rounded-lg flex items-start gap-2.5 text-xs text-amber-900">
            <AlertCircle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
            <div>
              <span className="font-semibold">Service Degraded: </span>
              {safetyReport.degraded_reason || 'The OpenFDA Drug Interaction service is offline or unreachable. Interaction checks may be incomplete.'}
              {safetyReport.unchecked_drugs && safetyReport.unchecked_drugs.length > 0 && (
                <div className="mt-1 font-mono text-[11px] text-amber-800">
                  Unchecked drugs: {safetyReport.unchecked_drugs.join(', ')}
                </div>
              )}
            </div>
          </div>
        )}

        {/* Interacting Drug Pairs Table */}
        {interactionAlerts.length > 0 && (
          <div className="space-y-2">
            <div className="text-xs font-semibold text-slate-700 uppercase tracking-wider font-mono">
              Interacting Medication Pairs ({interactionAlerts.length})
            </div>
            <div className="border border-slate-200 rounded-lg overflow-hidden bg-white">
              <table className="w-full text-left text-xs border-collapse">
                <thead>
                  <tr className="bg-slate-50 border-b border-slate-200 text-slate-500 font-mono uppercase text-[11px]">
                    <th className="py-2.5 px-3 font-semibold">Prescribed Drug</th>
                    <th className="py-2.5 px-3 font-semibold">Interacting Drug</th>
                    <th className="py-2.5 px-3 font-semibold">Severity</th>
                    <th className="py-2.5 px-3 font-semibold">Clinical Mechanism</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {interactionAlerts.map((alert, idx) => (
                    <tr key={idx} className="hover:bg-slate-50/60">
                      <td className="py-2.5 px-3 font-bold text-slate-900 font-mono">
                        {alert.drug1}
                      </td>
                      <td className="py-2.5 px-3 font-medium text-slate-700 font-mono">
                        {alert.drug2}
                      </td>
                      <td className="py-2.5 px-3">
                        <StatusBadge meta={getStatusMeta('safety', alert.severity)} size="sm" />
                      </td>
                      <td className="py-2.5 px-3 text-slate-600 text-[11px] max-w-xs">
                        <p className="line-clamp-2" title={alert.description}>
                          {alert.description}
                        </p>
                        {alert.mechanism && (
                          <span className="text-[10px] text-slate-400 font-mono mt-0.5 block">
                            Mech: {alert.mechanism}
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Note on Allergy Checks Deferred */}
        <div className="p-3 bg-slate-50 border border-slate-200 rounded-md text-[11px] text-slate-500 font-mono">
          <span className="font-semibold text-slate-700">Notice (ADR-004): </span>
          Patient documented allergy checking is currently unconfigured pending schema migration. Screening reflects OpenFDA Drug-Drug Interactions only.
        </div>

        {/* Clinical Justification Form */}
        <form onSubmit={handleAuthorize} className="space-y-3 pt-2 border-t border-slate-200">
          <div>
            <label className="block text-xs font-semibold text-slate-900 mb-1">
              Mandatory Clinician Justification / Override Reason
            </label>
            <p className="text-[11px] text-slate-500 mb-2">
              Explain why this prescription is clinically necessary despite the detected interaction (min. 10 characters). This will be printed on the signed prescription and permanently recorded in the HIPAA audit ledger.
            </p>
            <textarea
              rows={3}
              value={overrideReason}
              onChange={(e) => setOverrideReason(e.target.value)}
              placeholder="e.g. Patient requires acute combination therapy; doses staggered and closely monitored for adverse effects..."
              className="w-full px-3 py-2 text-xs bg-white border border-slate-200 rounded-md focus:outline-none focus:ring-1 focus:ring-slate-400 font-sans resize-none"
            />
            {validationError && (
              <p className="text-xs text-rose-600 mt-1 flex items-center gap-1 font-medium">
                <AlertCircle className="w-3.5 h-3.5" />
                {validationError}
              </p>
            )}
          </div>

          <div className="flex items-center justify-between gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              disabled={isPending}
              className="inline-flex items-center gap-1.5 px-3.5 py-2 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors cursor-pointer"
            >
              <X className="w-3.5 h-3.5 text-slate-400" />
              Cancel & Modify Regimen
            </button>

            <button
              type="submit"
              disabled={isPending || overrideReason.trim().length < 10}
              className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-bold text-white bg-rose-600 hover:bg-rose-700 rounded-md transition-colors shadow-2xs disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
            >
              <CheckCircle2 className="w-3.5 h-3.5" />
              {isPending ? 'Committing Override...' : 'Authorize Override & Commit'}
            </button>
          </div>
        </form>
      </div>
    </Modal>
  );
}
