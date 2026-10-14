import { useState } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { fetchApi, ApiError } from '../../lib/api/client';
import type {
  CreateDigitalPrescriptionRequest,
  CreateDigitalPrescriptionResponse,
  SafetyReport,
  MedicationTimeOfDay
} from '../../lib/api/types';
import { SafetyInterruptModal } from '../../components/doctor/SafetyInterruptModal';
import {
  Plus,
  Trash2,
  CheckCircle2,
  ArrowLeft,
  AlertCircle,
  Download,
  ExternalLink,
  ShieldCheck,
} from 'lucide-react';

interface FormItem {
  id: string;
  medication_name: string;
  dosage: string;
  frequency: string;
  duration: string;
  timing: MedicationTimeOfDay;
  instructions: string;
}

const DEFAULT_ITEM: () => FormItem = () => ({
  id: Math.random().toString(36).substring(2, 9),
  medication_name: '',
  dosage: '',
  frequency: '',
  duration: '',
  timing: 'morning',
  instructions: '',
});

export function DigitalPrescribe() {
  const { id: patientId = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [items, setItems] = useState<FormItem[]>([DEFAULT_ITEM()]);
  const [generalNotes, setGeneralNotes] = useState('');
  const [formError, setFormError] = useState<string | null>(null);

  // Safety Interrupt Modal state
  const [safetyReport, setSafetyReport] = useState<SafetyReport | null>(null);
  const [safetyErrorMsg, setSafetyErrorMsg] = useState<string>('');
  const [isSafetyModalOpen, setIsSafetyModalOpen] = useState(false);

  // Success state after 201
  const [createdPrescription, setCreatedPrescription] = useState<CreateDigitalPrescriptionResponse | null>(null);

  const prescribeMutation = useMutation({
    mutationFn: async (payload: CreateDigitalPrescriptionRequest) => {
      setFormError(null);
      return fetchApi<CreateDigitalPrescriptionResponse>('/api/prescriptions/digital', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
    },
    onSuccess: (data) => {
      setCreatedPrescription(data);
      setIsSafetyModalOpen(false);
      queryClient.invalidateQueries({ queryKey: ['doctor-patient-prescriptions', patientId] });
      queryClient.invalidateQueries({ queryKey: ['doctor-prescriptions'] });
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError && err.status === 409 && err.data) {
        const conflictData = err.data as {
          error?: string;
          safety_report?: SafetyReport;
          requires_override?: boolean;
        };
        if (conflictData.safety_report) {
          setSafetyReport(conflictData.safety_report);
          setSafetyErrorMsg(conflictData.error || 'Drug interaction detected');
          setIsSafetyModalOpen(true);
          return;
        }
      }

      const msg =
        err instanceof ApiError && err.message
          ? err.message
          : (err as { message?: string })?.message || 'Failed to issue digital prescription';
      setFormError(msg);
    },
  });

  const addItem = () => {
    setItems((prev) => [...prev, DEFAULT_ITEM()]);
  };

  const removeItem = (id: string) => {
    if (items.length <= 1) return;
    setItems((prev) => prev.filter((it) => it.id !== id));
  };

  const updateItem = (id: string, field: keyof FormItem, value: string) => {
    setItems((prev) =>
      prev.map((it) => (it.id === id ? { ...it, [field]: value } : it))
    );
  };

  const validateAndBuildPayload = (overrideSafety = false, overrideReason = ''): CreateDigitalPrescriptionRequest | null => {
    for (let i = 0; i < items.length; i++) {
      const it = items[i];
      if (!it.medication_name.trim()) {
        setFormError(`Item #${i + 1}: Medication name is required.`);
        return null;
      }
    }

    return {
      patient_id: patientId,
      notes: generalNotes.trim() || undefined,
      override_safety: overrideSafety,
      override_reason: overrideReason || undefined,
      items: items.map((it) => ({
        medication_name: it.medication_name.trim(),
        dosage: it.dosage.trim() || undefined,
        frequency: it.frequency.trim() || undefined,
        duration: it.duration.trim() || undefined,
        timing: it.timing || undefined,
        instructions: it.instructions.trim() || undefined,
      })),
    };
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);
    const payload = validateAndBuildPayload(false);
    if (!payload) return;
    prescribeMutation.mutate(payload);
  };

  const handleAuthorizeOverride = (overrideReason: string) => {
    const payload = validateAndBuildPayload(true, overrideReason);
    if (!payload) return;
    prescribeMutation.mutate(payload);
  };

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
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* Top Breadcrumb */}
      <div className="flex items-center justify-between gap-4">
        <Link
          to={`/doctor/patients/${patientId}`}
          className="inline-flex items-center gap-1.5 text-xs font-medium text-slate-500 hover:text-slate-900 transition-colors"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          Back to Patient Chart
        </Link>
      </div>

      {createdPrescription ? (
        /* Success Screen */
        <div className="p-8 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-6 text-center">
          <div className="w-12 h-12 bg-emerald-50 border border-emerald-200 text-emerald-700 rounded-full flex items-center justify-center mx-auto">
            <ShieldCheck className="w-6 h-6" />
          </div>

          <div className="space-y-1">
            <span className="font-mono text-xs uppercase tracking-wider text-emerald-700 font-bold">
              Prescription Committed & Signed
            </span>
            <h2 className="text-2xl font-bold text-slate-900">
              Digital E-Prescription Issued Successfully
            </h2>
            <p className="text-sm text-slate-500 max-w-md mx-auto mt-2">
              The prescription has been committed with status <strong className="font-mono text-slate-900">approved</strong>. A clinic-branded PDF and public verification QR code have been compiled.
            </p>
          </div>

          <div className="p-4 bg-slate-50 rounded-lg border border-slate-200 inline-block font-mono text-xs text-slate-600">
            <span>Prescription ID: </span>
            <strong className="text-slate-900">{createdPrescription.id}</strong>
          </div>

          <div className="flex items-center justify-center gap-3 pt-4 flex-wrap">
            <button
              onClick={() => handleDownloadPDF(createdPrescription.id)}
              className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-slate-800 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs"
            >
              <Download className="w-3.5 h-3.5 text-slate-600" />
              Download Generated PDF
            </button>

            <Link
              to={`/verify/rx/${createdPrescription.id}`}
              target="_blank"
              className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-slate-800 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs"
            >
              Verify Public QR Target
              <ExternalLink className="w-3.5 h-3.5 text-slate-400" />
            </Link>

            <button
              onClick={() => navigate(`/doctor/patients/${patientId}`)}
              className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 rounded-md transition-colors shadow-2xs"
            >
              Return to Patient Chart
            </button>
          </div>
        </div>
      ) : (
        /* Prescribe Form */
        <form onSubmit={handleSubmit} className="space-y-6">
          {/* Header */}
          <div className="pb-4 border-b border-slate-200">
            <span className="font-mono text-xs uppercase tracking-wider text-slate-500 font-semibold">
              Clinical Workflow • Screen C6
            </span>
            <h1 className="text-2xl font-bold tracking-tight text-slate-900 mt-1">
              Author Digital E-Prescription
            </h1>
            <p className="text-sm text-slate-500 mt-0.5">
              Target Patient UUID: <span className="font-mono font-medium text-slate-700">{patientId}</span>
            </p>
          </div>

          {formError && (
            <div className="p-3.5 bg-rose-50 border border-rose-200 rounded-lg flex items-center gap-2 text-xs text-rose-800">
              <AlertCircle className="w-4 h-4 text-rose-600 shrink-0" />
              <span>{formError}</span>
            </div>
          )}

          {/* Regimen Items Builder */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-bold text-slate-900">Medication Regimen</h3>
                <p className="text-xs text-slate-500">
                  Add line items with dosage, timing, and administration guidance.
                </p>
              </div>
              <button
                type="button"
                onClick={addItem}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors shadow-2xs cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5 text-slate-500" />
                Add Medication Item
              </button>
            </div>

            <div className="space-y-3">
              {items.map((item, index) => (
                <div
                  key={item.id}
                  className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-3"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs font-bold text-slate-700 uppercase tracking-wider">
                      Item #{index + 1}
                    </span>
                    {items.length > 1 && (
                      <button
                        type="button"
                        onClick={() => removeItem(item.id)}
                        className="text-slate-400 hover:text-rose-600 p-1 rounded transition-colors cursor-pointer"
                        title="Remove medication"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    )}
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div>
                      <label className="block text-[11px] font-medium text-slate-600 mb-1">
                        Medication Name *
                      </label>
                      <input
                        type="text"
                        placeholder="e.g. Amoxicillin, Lisinopril, Warfarin"
                        value={item.medication_name}
                        onChange={(e) => updateItem(item.id, 'medication_name', e.target.value)}
                        className="w-full px-3 py-1.5 text-xs bg-slate-50/50 border border-slate-200 rounded-md font-mono focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400"
                      />
                    </div>

                    <div>
                      <label className="block text-[11px] font-medium text-slate-600 mb-1">
                        Dosage
                      </label>
                      <input
                        type="text"
                        placeholder="e.g. 500 mg, 10 ml, 25 mcg"
                        value={item.dosage}
                        onChange={(e) => updateItem(item.id, 'dosage', e.target.value)}
                        className="w-full px-3 py-1.5 text-xs bg-slate-50/50 border border-slate-200 rounded-md font-mono focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400"
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <div>
                      <label className="block text-[11px] font-medium text-slate-600 mb-1">
                        Frequency
                      </label>
                      <input
                        type="text"
                        placeholder="e.g. Twice daily, Every 8 hours"
                        value={item.frequency}
                        onChange={(e) => updateItem(item.id, 'frequency', e.target.value)}
                        className="w-full px-3 py-1.5 text-xs bg-slate-50/50 border border-slate-200 rounded-md font-mono focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400"
                      />
                    </div>

                    <div>
                      <label className="block text-[11px] font-medium text-slate-600 mb-1">
                        Duration
                      </label>
                      <input
                        type="text"
                        placeholder="e.g. 7 days, 1 month"
                        value={item.duration}
                        onChange={(e) => updateItem(item.id, 'duration', e.target.value)}
                        className="w-full px-3 py-1.5 text-xs bg-slate-50/50 border border-slate-200 rounded-md font-mono focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400"
                      />
                    </div>

                    <div>
                      <label className="block text-[11px] font-medium text-slate-600 mb-1">
                        Scheduled Timing
                      </label>
                      <select
                        value={item.timing}
                        onChange={(e) =>
                          updateItem(item.id, 'timing', e.target.value as MedicationTimeOfDay)
                        }
                        className="w-full px-2.5 py-1.5 text-xs bg-slate-50/50 border border-slate-200 rounded-md focus:bg-white focus:outline-none text-slate-700 capitalize"
                      >
                        <option value="morning">Morning</option>
                        <option value="afternoon">Afternoon</option>
                        <option value="evening">Evening</option>
                        <option value="bedtime">Bedtime</option>
                        <option value="as_needed">As Needed (PRN)</option>
                      </select>
                    </div>
                  </div>

                  <div>
                    <label className="block text-[11px] font-medium text-slate-600 mb-1">
                      Instructions & Administration Details
                    </label>
                    <input
                      type="text"
                      placeholder="e.g. Take with food, swallow whole with water..."
                      value={item.instructions}
                      onChange={(e) => updateItem(item.id, 'instructions', e.target.value)}
                      className="w-full px-3 py-1.5 text-xs bg-slate-50/50 border border-slate-200 rounded-md focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400"
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* General Notes */}
          <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-2">
            <label className="block text-xs font-semibold text-slate-900">
              General Clinical Notes & Diagnostics
            </label>
            <textarea
              rows={3}
              placeholder="Diagnosis summary, clinical advice, follow-up indications..."
              value={generalNotes}
              onChange={(e) => setGeneralNotes(e.target.value)}
              className="w-full px-3 py-2 text-xs bg-slate-50/50 border border-slate-200 rounded-md focus:bg-white focus:outline-none focus:ring-1 focus:ring-slate-400 resize-none font-sans"
            />
          </div>

          {/* Submit Action */}
          <div className="flex items-center justify-end gap-3 pt-4 border-t border-slate-200">
            <Link
              to={`/doctor/patients/${patientId}`}
              className="px-4 py-2 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors"
            >
              Cancel
            </Link>

            <button
              type="submit"
              disabled={prescribeMutation.isPending}
              className="inline-flex items-center gap-2 px-5 py-2 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 rounded-md transition-colors shadow-2xs disabled:opacity-50 cursor-pointer"
            >
              <CheckCircle2 className="w-4 h-4" />
              {prescribeMutation.isPending ? 'Verifying Safety & Committing...' : 'Issue Verified Prescription'}
            </button>
          </div>
        </form>
      )}

      {/* C7 Safety Interrupt Modal */}
      <SafetyInterruptModal
        isOpen={isSafetyModalOpen}
        onClose={() => setIsSafetyModalOpen(false)}
        safetyReport={safetyReport}
        errorMessage={safetyErrorMsg}
        onAuthorizeOverride={handleAuthorizeOverride}
        isPending={prescribeMutation.isPending}
      />
    </div>
  );
}
