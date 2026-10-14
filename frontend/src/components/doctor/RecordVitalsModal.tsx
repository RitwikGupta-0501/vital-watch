import React, { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { fetchApi } from '../../lib/api/client';
import type { PatientVital, VitalInput } from '../../lib/api/types';
import { Modal } from '../ui/Modal';
import { Activity, AlertTriangle, AlertCircle, CheckCircle2 } from 'lucide-react';

interface RecordVitalsModalProps {
  isOpen: boolean;
  onClose: () => void;
  patientId: string;
  patientName?: string;
  onSuccess?: (vital: PatientVital) => void;
}

export function RecordVitalsModal({
  isOpen,
  onClose,
  patientId,
  patientName,
  onSuccess,
}: RecordVitalsModalProps) {
  const queryClient = useQueryClient();

  const [systolic, setSystolic] = useState<string>('');
  const [diastolic, setDiastolic] = useState<string>('');
  const [heartRate, setHeartRate] = useState<string>('');
  const [glucose, setGlucose] = useState<string>('');
  const [oxygen, setOxygen] = useState<string>('');
  const [temp, setTemp] = useState<string>('');
  const [weight, setWeight] = useState<string>('');
  const [notes, setNotes] = useState<string>('');

  const [formError, setFormError] = useState<string | null>(null);
  const [criticalAlerts, setCriticalAlerts] = useState<string[] | null>(null);

  const mutation = useMutation({
    mutationFn: async (payload: VitalInput) => {
      setFormError(null);
      return fetchApi<PatientVital>('/api/vitals', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['patient-vitals', patientId] });
      queryClient.invalidateQueries({ queryKey: ['vitals'] });

      if (data.is_critical && data.critical_alerts && data.critical_alerts.length > 0) {
        setCriticalAlerts(data.critical_alerts);
      } else {
        if (onSuccess) onSuccess(data);
        onClose();
      }
    },
    onError: (err: { message?: string; data?: { error?: string } }) => {
      const msg = err.data?.error || err.message || 'Failed to record vitals';
      setFormError(msg);
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);

    const sVal = systolic ? parseInt(systolic, 10) : undefined;
    const dVal = diastolic ? parseInt(diastolic, 10) : undefined;
    const hrVal = heartRate ? parseInt(heartRate, 10) : undefined;
    const gVal = glucose ? parseFloat(glucose) : undefined;
    const oxVal = oxygen ? parseFloat(oxygen) : undefined;
    const tVal = temp ? parseFloat(temp) : undefined;
    const wVal = weight ? parseFloat(weight) : undefined;

    // Check at least one metric provided
    if (
      sVal === undefined &&
      dVal === undefined &&
      hrVal === undefined &&
      gVal === undefined &&
      oxVal === undefined &&
      tVal === undefined &&
      wVal === undefined
    ) {
      setFormError('At least one physiological measurement must be provided.');
      return;
    }

    // Physiological boundary validations (mirrored from backend)
    if (sVal !== undefined && (sVal < 50 || sVal > 300)) {
      setFormError('Systolic BP must be between 50 and 300 mmHg.');
      return;
    }
    if (dVal !== undefined && (dVal < 30 || dVal > 200)) {
      setFormError('Diastolic BP must be between 30 and 200 mmHg.');
      return;
    }
    if (sVal !== undefined && dVal !== undefined && sVal <= dVal) {
      setFormError('Systolic BP must be strictly greater than Diastolic BP.');
      return;
    }
    if (hrVal !== undefined && (hrVal < 30 || hrVal > 250)) {
      setFormError('Heart rate must be between 30 and 250 bpm.');
      return;
    }
    if (gVal !== undefined && (gVal < 10 || gVal > 1000)) {
      setFormError('Blood glucose must be between 10 and 1000 mg/dL.');
      return;
    }
    if (oxVal !== undefined && (oxVal < 50 || oxVal > 100)) {
      setFormError('Oxygen saturation (SpO₂) must be between 50% and 100%.');
      return;
    }
    if (tVal !== undefined && (tVal < 30 || tVal > 45)) {
      setFormError('Body temperature must be between 30.0°C and 45.0°C.');
      return;
    }
    if (wVal !== undefined && (wVal < 1 || wVal > 500)) {
      setFormError('Body weight must be between 1.0 kg and 500.0 kg.');
      return;
    }
    if (notes.length > 2000) {
      setFormError('Clinical notes must not exceed 2000 characters.');
      return;
    }

    const payload: VitalInput = {
      patient_id: patientId,
      systolic_bp: sVal,
      diastolic_bp: dVal,
      heart_rate: hrVal,
      blood_glucose: gVal,
      oxygen_saturation: oxVal,
      temperature: tVal,
      weight_kg: wVal,
      notes: notes.trim() || undefined,
    };

    mutation.mutate(payload);
  };

  const handleDismissCritical = () => {
    setCriticalAlerts(null);
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Record Consultation Vitals">
      {criticalAlerts ? (
        <div className="space-y-4">
          <div className="p-4 bg-rose-50 border border-rose-200 rounded-lg">
            <div className="flex items-start gap-3">
              <AlertTriangle className="w-5 h-5 text-rose-600 shrink-0 mt-0.5" />
              <div>
                <h4 className="text-sm font-bold text-rose-900">
                  Critical Threshold Alert Triggered
                </h4>
                <p className="text-xs text-rose-700 mt-1">
                  The biometrics submitted for {patientName || 'this patient'} have exceeded clinical safety thresholds:
                </p>
                <ul className="mt-2 space-y-1 text-xs font-mono font-medium text-rose-900 list-disc list-inside">
                  {criticalAlerts.map((alert, idx) => (
                    <li key={idx}>{alert}</li>
                  ))}
                </ul>
              </div>
            </div>
          </div>

          <div className="flex justify-end pt-2">
            <button
              type="button"
              onClick={handleDismissCritical}
              className="px-4 py-2 text-xs font-medium text-white bg-slate-900 hover:bg-slate-800 rounded-md transition-colors"
            >
              Acknowledge & Close
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="text-xs text-slate-500 pb-2 border-b border-slate-100 flex items-center justify-between">
            <span>
              Patient: <strong className="text-slate-900">{patientName || 'Clinical Record'}</strong>
            </span>
            <span className="font-mono text-slate-400">ID: {patientId.slice(-8)}</span>
          </div>

          {formError && (
            <div className="p-3 bg-rose-50 border border-rose-200 rounded-md flex items-center gap-2 text-xs text-rose-800">
              <AlertCircle className="w-4 h-4 text-rose-600 shrink-0" />
              <span>{formError}</span>
            </div>
          )}

          {/* Blood Pressure Bento */}
          <div className="p-3.5 bg-slate-50/70 border border-slate-200 rounded-lg space-y-3">
            <div className="flex items-center justify-between text-xs font-semibold text-slate-700">
              <span className="flex items-center gap-1.5">
                <Activity className="w-3.5 h-3.5 text-slate-500" />
                Blood Pressure (mmHg)
              </span>
              <span className="text-[11px] text-slate-400 font-normal">Standard 120/80</span>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-[11px] font-medium text-slate-500 uppercase tracking-wider mb-1">
                  Systolic (50–300)
                </label>
                <input
                  type="number"
                  placeholder="e.g. 120"
                  value={systolic}
                  onChange={(e) => setSystolic(e.target.value)}
                  className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md font-mono focus:outline-none focus:ring-1 focus:ring-slate-400"
                />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-slate-500 uppercase tracking-wider mb-1">
                  Diastolic (30–200)
                </label>
                <input
                  type="number"
                  placeholder="e.g. 80"
                  value={diastolic}
                  onChange={(e) => setDiastolic(e.target.value)}
                  className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md font-mono focus:outline-none focus:ring-1 focus:ring-slate-400"
                />
              </div>
            </div>
          </div>

          {/* Pulse & Oxygen Grid */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-slate-600 mb-1">
                Heart Rate (30–250 bpm)
              </label>
              <input
                type="number"
                placeholder="e.g. 72"
                value={heartRate}
                onChange={(e) => setHeartRate(e.target.value)}
                className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md font-mono focus:outline-none focus:ring-1 focus:ring-slate-400"
              />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-slate-600 mb-1">
                Oxygen Saturation (50–100 %)
              </label>
              <input
                type="number"
                step="0.1"
                placeholder="e.g. 98.5"
                value={oxygen}
                onChange={(e) => setOxygen(e.target.value)}
                className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md font-mono focus:outline-none focus:ring-1 focus:ring-slate-400"
              />
            </div>
          </div>

          {/* Glucose & Temp Grid */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-slate-600 mb-1">
                Blood Glucose (10–1000 mg/dL)
              </label>
              <input
                type="number"
                step="0.1"
                placeholder="e.g. 95.0"
                value={glucose}
                onChange={(e) => setGlucose(e.target.value)}
                className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md font-mono focus:outline-none focus:ring-1 focus:ring-slate-400"
              />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-slate-600 mb-1">
                Temperature (30.0–45.0 °C)
              </label>
              <input
                type="number"
                step="0.1"
                placeholder="e.g. 36.8"
                value={temp}
                onChange={(e) => setTemp(e.target.value)}
                className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md font-mono focus:outline-none focus:ring-1 focus:ring-slate-400"
              />
            </div>
          </div>

          {/* Weight */}
          <div>
            <label className="block text-[11px] font-medium text-slate-600 mb-1">
              Body Weight (1.0–500.0 kg)
            </label>
            <input
              type="number"
              step="0.1"
              placeholder="e.g. 70.5"
              value={weight}
              onChange={(e) => setWeight(e.target.value)}
              className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md font-mono focus:outline-none focus:ring-1 focus:ring-slate-400"
            />
          </div>

          {/* Notes */}
          <div>
            <label className="block text-[11px] font-medium text-slate-600 mb-1">
              Consultation Notes (Optional)
            </label>
            <textarea
              rows={2}
              maxLength={2000}
              placeholder="Clinical context, symptoms observed, post-exercise or resting state..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="w-full px-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md focus:outline-none focus:ring-1 focus:ring-slate-400 resize-none"
            />
          </div>

          {/* Footer CTA */}
          <div className="flex items-center justify-end gap-2 pt-3 border-t border-slate-100">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-xs font-medium text-slate-600 hover:text-slate-900 bg-white border border-slate-200 rounded-md hover:bg-slate-50 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="inline-flex items-center gap-1.5 px-4 py-1.5 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 rounded-md transition-colors shadow-2xs disabled:opacity-50"
            >
              <CheckCircle2 className="w-3.5 h-3.5" />
              {mutation.isPending ? 'Committing...' : 'Commit Vitals Record'}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
