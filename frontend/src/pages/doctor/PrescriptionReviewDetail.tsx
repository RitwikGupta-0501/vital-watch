import React, { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getPrescriptionById, verifyPrescription } from '../../lib/api/prescriptions';
import type {
  Prescription,
  VerifyPrescriptionItemInput,
  SafetyReport,
} from '../../lib/api/types';
import { ApiError } from '../../lib/api/client';
import { PrescriptionDocumentViewer } from '../../components/doctor/PrescriptionDocumentViewer';
import { SafetyInterruptModal } from '../../components/doctor/SafetyInterruptModal';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { getStatusMeta } from '../../lib/status';
import { Modal } from '../../components/ui/Modal';
import { EmptyState } from '../../components/ui/EmptyState';
import { useNotificationStream } from '../../hooks/useNotificationStream';
import { formatDisplayDate } from '../../lib/datetime';
import {
  ArrowLeft,
  Plus,
  Trash2,
  CheckCircle2,
  AlertTriangle,
  Sparkles,
  ShieldAlert,
  Loader2,
  Copy,
  ExternalLink,
} from 'lucide-react';

export function PrescriptionReviewDetail() {
  const { id: prescriptionId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  // Local state for interactive editing
  const [items, setItems] = useState<VerifyPrescriptionItemInput[]>([]);
  const [notes, setNotes] = useState<string>('');
  const [hasInitialized, setHasInitialized] = useState(false);
  const [validationErrors, setValidationErrors] = useState<Record<number, string>>({});
  const [formError, setFormError] = useState<string | null>(null);

  // Submitting state
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Safety Interrupt Modal state (C7)
  const [isSafetyModalOpen, setIsSafetyModalOpen] = useState(false);
  const [safetyReport, setSafetyReport] = useState<SafetyReport | null>(null);
  const [safetyErrorMessage, setSafetyErrorMessage] = useState<string>('');

  // Rejection Modal state
  const [isRejectModalOpen, setIsRejectModalOpen] = useState(false);
  const [rejectionReason, setRejectionReason] = useState('');
  const [rejectionError, setRejectionError] = useState<string | null>(null);

  // 1. Fetch Prescription Details
  const {
    data: prescription,
    isLoading,
    isError,
  } = useQuery<Prescription>({
    queryKey: ['prescription-detail', prescriptionId],
    queryFn: () => getPrescriptionById(prescriptionId!),
    enabled: !!prescriptionId,
    refetchOnWindowFocus: false,
  });

  // 2. Real-Time SSE Listener: transition from pending_ocr to needs_review automatically
  useNotificationStream({
    enabled: true,
    onEvent: (event) => {
      if (
        event.type === 'prescription.ocr_completed' &&
        event.data &&
        typeof event.data === 'object' &&
        'prescription_id' in event.data &&
        (event.data as any).prescription_id === prescriptionId
      ) {
        queryClient.invalidateQueries({ queryKey: ['prescription-detail', prescriptionId] });
      }
    },
  });

  // 3. Initialize editable state once data arrives
  useEffect(() => {
    if (prescription && !hasInitialized) {
      const initialItems: VerifyPrescriptionItemInput[] = (prescription.items || []).map((it) => ({
        medication_name: it.medication_name || '',
        dosage: it.dosage || '',
        frequency: it.frequency || '',
        duration: it.duration || '',
        timing: it.timing || '',
        instructions: it.instructions || '',
      }));

      // If OCR detected 0 items (e.g. failure note or manual required), give clinician an empty item to start
      if (initialItems.length === 0 && prescription.status === 'needs_review') {
        initialItems.push({
          medication_name: '',
          dosage: '',
          frequency: '',
          duration: '',
          timing: '',
          instructions: '',
        });
      }

      setItems(initialItems);
      setNotes(prescription.notes || '');
      setHasInitialized(true);
    }
  }, [prescription, hasInitialized]);

  // Medication Item Handlers
  const handleAddItem = useCallback(() => {
    setItems((prev) => [
      ...prev,
      {
        medication_name: '',
        dosage: '',
        frequency: '',
        duration: '',
        timing: '',
        instructions: '',
      },
    ]);
  }, []);

  const handleDuplicateItem = (index: number) => {
    const itemToCopy = items[index];
    if (!itemToCopy) return;
    setItems((prev) => [
      ...prev.slice(0, index + 1),
      { ...itemToCopy },
      ...prev.slice(index + 1),
    ]);
  };

  const handleRemoveItem = (index: number) => {
    setItems((prev) => prev.filter((_, i) => i !== index));
    setValidationErrors((prev) => {
      const next = { ...prev };
      delete next[index];
      return next;
    });
  };

  const handleItemFieldChange = (
    index: number,
    field: keyof VerifyPrescriptionItemInput,
    value: string
  ) => {
    setItems((prev) => {
      const updated = [...prev];
      updated[index] = { ...updated[index], [field]: value };
      return updated;
    });

    if (field === 'medication_name' && value.trim()) {
      setValidationErrors((prev) => {
        const next = { ...prev };
        delete next[index];
        return next;
      });
    }
  };

  // Approval Submission Workflow
  const handleApprove = useCallback(
    async (overrideSafety = false, overrideReason = '') => {
      if (!prescriptionId) return;

      // Validate Items
      if (items.length === 0) {
        setFormError('At least one medication item is required to approve this prescription (ADR-004).');
        return;
      }

      const errors: Record<number, string> = {};
      items.forEach((it, idx) => {
        if (!it.medication_name.trim()) {
          errors[idx] = 'Medication name is required';
        }
      });

      if (Object.keys(errors).length > 0) {
        setValidationErrors(errors);
        setFormError('Please provide a valid medication name for all listed items.');
        return;
      }

      setFormError(null);
      setIsSubmitting(true);

      try {
        await verifyPrescription(prescriptionId, {
          status: 'approved',
          items,
          notes: notes.trim(),
          override_safety: overrideSafety,
          override_reason: overrideReason,
        });

        setIsSafetyModalOpen(false);
        queryClient.invalidateQueries({ queryKey: ['pending-review-prescriptions'] });
        navigate('/doctor/review');
      } catch (err: unknown) {
        if (err instanceof ApiError && err.status === 409) {
          const errorData = err.data as Record<string, unknown> | undefined;
          if (errorData?.requires_override && errorData?.safety_report) {
            // Trigger Safety Interrupt Modal (C7)
            setSafetyReport(errorData.safety_report as SafetyReport);
            setSafetyErrorMessage(typeof errorData.error === 'string' ? errorData.error : err.message);
            setIsSafetyModalOpen(true);
            setIsSubmitting(false);
            return;
          }
        }

        if (err instanceof Error) {
          setFormError(err.message);
        } else {
          setFormError('Verification failed. Please review your entries and retry.');
        }
      } finally {
        setIsSubmitting(false);
      }
    },
    [prescriptionId, items, notes, queryClient, navigate]
  );

  // Rejection Submission Workflow
  const handleRejectConfirm = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!prescriptionId) return;

    if (!rejectionReason.trim()) {
      setRejectionError('Please state a reason for rejecting this prescription.');
      return;
    }

    setIsSubmitting(true);
    setRejectionError(null);

    try {
      await verifyPrescription(prescriptionId, {
        status: 'rejected',
        notes: notes.trim()
          ? `${notes.trim()} [Rejected: ${rejectionReason.trim()}]`
          : `[Rejected: ${rejectionReason.trim()}]`,
      });

      setIsRejectModalOpen(false);
      queryClient.invalidateQueries({ queryKey: ['pending-review-prescriptions'] });
      navigate('/doctor/review');
    } catch (err: unknown) {
      if (err instanceof Error) {
        setRejectionError(err.message);
      } else {
        setRejectionError('Failed to reject prescription. Please try again.');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  // Keyboard Shortcuts Handler
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      // Don't trigger shortcuts if safety or rejection modal is open
      if (isSafetyModalOpen || isRejectModalOpen) return;

      // Cmd+Enter or Ctrl+Enter -> Approve
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        e.preventDefault();
        handleApprove();
      }
      // Alt+A -> Add Item
      if (e.altKey && (e.key === 'a' || e.key === 'A')) {
        e.preventDefault();
        handleAddItem();
      }
      // Escape -> Back to queue
      if (e.key === 'Escape' && !isSafetyModalOpen && !isRejectModalOpen) {
        navigate('/doctor/review');
      }
    },
    [isSafetyModalOpen, isRejectModalOpen, handleApprove, handleAddItem, navigate]
  );

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center p-12 bg-slate-50 text-slate-500 font-mono text-xs gap-2">
        <Loader2 className="w-5 h-5 animate-spin text-brand-600" />
        Loading clinical prescription review...
      </div>
    );
  }

  if (isError || !prescription) {
    return (
      <div className="max-w-xl mx-auto py-12 px-4">
        <EmptyState
          variant="error"
          title="Prescription Not Found"
          description="The requested prescription could not be retrieved, or you do not have permission to verify it."
          actionLabel="Return to Review Queue"
          onAction={() => navigate('/doctor/review')}
        />
      </div>
    );
  }

  const isPendingOCR = prescription.status === 'pending_ocr';
  const isAlreadyProcessed = prescription.status === 'approved' || prescription.status === 'rejected';
  const hasFailureNote = prescription.notes?.includes('[OCR') || prescription.notes?.includes('[Storage');

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem)] overflow-hidden font-sans bg-slate-100">
      {/* Top Clinical Header Bar */}
      <header className="h-14 bg-white border-b border-slate-200 px-4 sm:px-6 flex items-center justify-between shrink-0 z-20">
        <div className="flex items-center gap-3">
          <Link
            to="/doctor/review"
            className="p-1.5 rounded hover:bg-slate-100 text-slate-600 transition-colors"
            title="Back to Review Queue (Esc)"
          >
            <ArrowLeft className="w-4 h-4" />
          </Link>

          <div>
            <div className="flex items-center gap-2">
              <span className="text-[10px] font-mono text-slate-500 uppercase tracking-wider font-semibold">
                Screen C10 • HITL Verification
              </span>
              <span className="text-slate-300">•</span>
              <span className="text-xs font-bold text-slate-900">
                {prescription.patient_name || 'Patient'}
              </span>
              <Link
                to={`/doctor/patients/${prescription.patient_id}`}
                className="text-[11px] font-mono text-brand-700 hover:underline flex items-center gap-0.5"
                title="View longitudinal chart"
              >
                Chart
                <ExternalLink className="w-2.5 h-2.5" />
              </Link>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <StatusBadge meta={getStatusMeta('prescription', prescription.status)} />

          {/* Keyboard Hints Micro-UI */}
          <div className="hidden lg:flex items-center gap-2 text-[11px] font-mono text-slate-500 bg-slate-50 px-2.5 py-1 rounded border border-slate-200">
            <span>Approve: <kbd className="px-1 py-0.2 bg-white border border-slate-300 rounded font-semibold text-slate-700">⌘+↵</kbd></span>
            <span>•</span>
            <span>Add Item: <kbd className="px-1 py-0.2 bg-white border border-slate-300 rounded font-semibold text-slate-700">Alt+A</kbd></span>
          </div>
        </div>
      </header>

      {/* Main Split-Screen Workspace (50% Viewer / 50% Ledger) */}
      <div className="flex-1 flex flex-col md:flex-row overflow-hidden min-h-0">
        {/* Left Pane: High-Resolution Document Viewer */}
        <div className="w-full md:w-1/2 h-1/2 md:h-full border-b md:border-b-0 md:border-r border-slate-200 overflow-hidden flex flex-col">
          <PrescriptionDocumentViewer
            prescriptionId={prescription.id}
            fileName={prescription.file_name}
          />
        </div>

        {/* Right Pane: Clinical Verification Ledger */}
        <div className="w-full md:w-1/2 h-1/2 md:h-full bg-white overflow-y-auto flex flex-col">
          {/* Status & Model Banner */}
          <div className="p-4 bg-slate-50 border-b border-slate-200 space-y-2">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="text-xs font-semibold text-slate-700">Engine:</span>
                {prescription.ocr_provider ? (
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-mono bg-white text-slate-800 border border-slate-200 font-medium">
                    <Sparkles className="w-3.5 h-3.5 text-brand-600" />
                    {prescription.ocr_provider}
                  </span>
                ) : (
                  <span className="text-xs font-mono text-slate-500">Manual / Fallback</span>
                )}
              </div>

              <span className="text-[11px] font-mono text-slate-500">
                Uploaded: {formatDisplayDate(prescription.created_at)}
              </span>
            </div>

            {/* In Progress Warning */}
            {isPendingOCR ? (
              <div className="p-3 bg-sky-50 border border-sky-200 rounded text-xs text-sky-900 flex items-start gap-2">
                <Loader2 className="w-4 h-4 animate-spin text-sky-600 shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold">OCR Analysis in Progress: </span>
                  The background multimodal vision worker is extracting medications. This panel will update automatically when processing finishes.
                </div>
              </div>
            ) : isAlreadyProcessed ? (
              <div className="p-3 bg-slate-100 border border-slate-300 rounded text-xs text-slate-800 flex items-start gap-2">
                <CheckCircle2 className="w-4 h-4 text-emerald-600 shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold">Prescription Already Finalized: </span>
                  This record has already been marked as <span className="font-mono uppercase font-bold">{prescription.status}</span>. Edits are disabled.
                </div>
              </div>
            ) : (
              /* Unverified AI Output Notice (ADR-004) */
              <div className="p-2.5 bg-amber-50/70 border border-amber-200/80 rounded text-[11px] text-amber-900 flex items-start gap-2">
                <AlertTriangle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold">Human Clinical Sign-Off Required (ADR-004): </span>
                  AI extraction is an assistive suggestion. Verify every medication name, dosage, and frequency against the original slip before issuing.
                </div>
              </div>
            )}

            {/* Zero-Item Extraction Failure State */}
            {hasFailureNote && (
              <div className="p-2.5 bg-rose-50 border border-rose-200 rounded text-[11px] text-rose-900 flex items-start gap-2">
                <ShieldAlert className="w-4 h-4 text-rose-700 shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold">Extraction Incomplete: </span>
                  {prescription.notes || 'The vision model could not automatically extract line items. Please transcribe medications from the left scan.'}
                </div>
              </div>
            )}
          </div>

          {/* Form Error Banner */}
          {formError && (
            <div className="m-4 p-3 bg-rose-50 border border-rose-200 rounded text-xs text-rose-900 flex items-start gap-2">
              <ShieldAlert className="w-4 h-4 text-rose-700 shrink-0 mt-0.5" />
              <div>
                <span className="font-semibold">Verification Error: </span>
                {formError}
              </div>
            </div>
          )}

          {/* Medication Items List */}
          <div className="p-4 sm:p-6 space-y-4 flex-1">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-mono font-bold uppercase tracking-wider text-slate-700">
                Prescribed Medication Line Items ({items.length})
              </h3>

              {!isAlreadyProcessed && !isPendingOCR && (
                <button
                  type="button"
                  onClick={handleAddItem}
                  className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-semibold text-brand-700 bg-brand-50 hover:bg-brand-100 border border-brand-200 rounded transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" />
                  Add Medication Item
                </button>
              )}
            </div>

            {items.length === 0 ? (
              <div className="p-8 text-center border-2 border-dashed border-slate-200 rounded-lg text-xs text-slate-500 space-y-2">
                <p>No medication items recorded.</p>
                {!isAlreadyProcessed && !isPendingOCR && (
                  <button
                    type="button"
                    onClick={handleAddItem}
                    className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-semibold text-slate-900 bg-slate-100 hover:bg-slate-200 rounded border border-slate-300 transition-colors"
                  >
                    <Plus className="w-3.5 h-3.5" />
                    Add First Medication
                  </button>
                )}
              </div>
            ) : (
              <div className="space-y-3">
                {items.map((item, index) => {
                  const error = validationErrors[index];

                  return (
                    <div
                      key={index}
                      className={`p-3.5 rounded-lg border transition-all ${
                        error
                          ? 'border-rose-300 bg-rose-50/20'
                          : 'border-slate-200 bg-slate-50/50 hover:bg-slate-50'
                      }`}
                    >
                      {/* Item Header */}
                      <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-200/60 text-xs">
                        <span className="font-mono text-slate-500 font-semibold">
                          #{index + 1} Medication
                        </span>

                        {!isAlreadyProcessed && !isPendingOCR && (
                          <div className="flex items-center gap-1">
                            <button
                              type="button"
                              onClick={() => handleDuplicateItem(index)}
                              title="Duplicate item"
                              className="p-1 text-slate-400 hover:text-slate-700 rounded transition-colors"
                            >
                              <Copy className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => handleRemoveItem(index)}
                              title="Remove item"
                              className="p-1 text-slate-400 hover:text-rose-600 rounded transition-colors"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        )}
                      </div>

                      {/* Fields Grid */}
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
                        {/* Drug Name */}
                        <div className="sm:col-span-2 space-y-1">
                          <label className="block text-[10px] font-mono uppercase text-slate-600 font-semibold">
                            Medication Name *
                          </label>
                          <input
                            type="text"
                            value={item.medication_name}
                            disabled={isAlreadyProcessed || isPendingOCR}
                            onChange={(e) =>
                              handleItemFieldChange(index, 'medication_name', e.target.value)
                            }
                            placeholder="e.g. Amoxicillin, Metformin, Lisinopril"
                            className={`w-full px-2.5 py-1.5 text-xs rounded border font-medium ${
                              error
                                ? 'border-rose-300 bg-rose-50/40 text-rose-900 focus:ring-rose-500'
                                : 'border-slate-300 bg-white text-slate-900 focus:ring-slate-900'
                            } focus:outline-hidden focus:ring-1`}
                          />
                          {error && <span className="text-[10px] text-rose-600">{error}</span>}
                        </div>

                        {/* Dosage */}
                        <div className="space-y-1">
                          <label className="block text-[10px] font-mono uppercase text-slate-600 font-semibold">
                            Dosage
                          </label>
                          <input
                            type="text"
                            value={item.dosage || ''}
                            disabled={isAlreadyProcessed || isPendingOCR}
                            onChange={(e) => handleItemFieldChange(index, 'dosage', e.target.value)}
                            placeholder="e.g. 500mg, 10ml, 1 tab"
                            className="w-full px-2.5 py-1.5 text-xs rounded border border-slate-300 bg-white text-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900 font-mono"
                          />
                        </div>

                        {/* Frequency */}
                        <div className="space-y-1">
                          <label className="block text-[10px] font-mono uppercase text-slate-600 font-semibold">
                            Frequency
                          </label>
                          <input
                            type="text"
                            value={item.frequency || ''}
                            disabled={isAlreadyProcessed || isPendingOCR}
                            onChange={(e) => handleItemFieldChange(index, 'frequency', e.target.value)}
                            placeholder="e.g. Three times daily, q8h, PRN"
                            className="w-full px-2.5 py-1.5 text-xs rounded border border-slate-300 bg-white text-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900 font-mono"
                          />
                        </div>

                        {/* Duration */}
                        <div className="space-y-1">
                          <label className="block text-[10px] font-mono uppercase text-slate-600 font-semibold">
                            Duration
                          </label>
                          <input
                            type="text"
                            value={item.duration || ''}
                            disabled={isAlreadyProcessed || isPendingOCR}
                            onChange={(e) => handleItemFieldChange(index, 'duration', e.target.value)}
                            placeholder="e.g. 7 days, 1 month, Ongoing"
                            className="w-full px-2.5 py-1.5 text-xs rounded border border-slate-300 bg-white text-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900 font-mono"
                          />
                        </div>

                        {/* Timing */}
                        <div className="space-y-1">
                          <label className="block text-[10px] font-mono uppercase text-slate-600 font-semibold">
                            Timing
                          </label>
                          <input
                            type="text"
                            value={item.timing || ''}
                            disabled={isAlreadyProcessed || isPendingOCR}
                            onChange={(e) => handleItemFieldChange(index, 'timing', e.target.value)}
                            placeholder="e.g. After meals, Bedtime"
                            className="w-full px-2.5 py-1.5 text-xs rounded border border-slate-300 bg-white text-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900 font-mono"
                          />
                        </div>

                        {/* Instructions */}
                        <div className="sm:col-span-2 space-y-1">
                          <label className="block text-[10px] font-mono uppercase text-slate-600 font-semibold">
                            Patient Instructions
                          </label>
                          <input
                            type="text"
                            value={item.instructions || ''}
                            disabled={isAlreadyProcessed || isPendingOCR}
                            onChange={(e) =>
                              handleItemFieldChange(index, 'instructions', e.target.value)
                            }
                            placeholder="e.g. Complete full antibiotic course even if feeling better"
                            className="w-full px-2.5 py-1.5 text-xs rounded border border-slate-300 bg-white text-slate-900 focus:outline-hidden focus:ring-1 focus:ring-slate-900"
                          />
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            {/* General Clinical Notes */}
            <div className="space-y-1.5 pt-3 border-t border-slate-100">
              <label htmlFor="notes" className="block text-xs font-mono font-semibold text-slate-700 uppercase">
                Clinical Prescription Notes
              </label>
              <textarea
                id="notes"
                rows={3}
                value={notes}
                disabled={isAlreadyProcessed || isPendingOCR}
                onChange={(e) => setNotes(e.target.value)}
                placeholder="Diagnostic reasoning or notes appended to prescription..."
                className="w-full px-3 py-2 text-xs text-slate-900 bg-white border border-slate-300 rounded focus:outline-hidden focus:ring-1 focus:ring-slate-900"
              />
            </div>
          </div>

          {/* Sticky Action Footer */}
          <div className="h-16 bg-white border-t border-slate-200 px-6 flex items-center justify-between shrink-0">
            <button
              type="button"
              onClick={() => setIsRejectModalOpen(true)}
              disabled={isAlreadyProcessed || isPendingOCR || isSubmitting}
              className="px-3.5 py-2 text-xs font-semibold text-rose-700 bg-white border border-rose-300 hover:bg-rose-50 disabled:opacity-40 disabled:cursor-not-allowed rounded transition-colors"
            >
              Reject Prescription
            </button>

            <button
              type="button"
              onClick={() => handleApprove()}
              disabled={isAlreadyProcessed || isPendingOCR || isSubmitting}
              className="inline-flex items-center gap-2 px-5 py-2 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed rounded transition-colors shadow-2xs"
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  Verifying Safety & Signing...
                </>
              ) : (
                <>
                  <CheckCircle2 className="w-4 h-4" />
                  Approve & Issue Prescription
                </>
              )}
            </button>
          </div>
        </div>
      </div>

      {/* Safety Interrupt Modal (C7) for Drug-Drug Interaction Overrides */}
      <SafetyInterruptModal
        isOpen={isSafetyModalOpen}
        onClose={() => setIsSafetyModalOpen(false)}
        safetyReport={safetyReport}
        errorMessage={safetyErrorMessage}
        onAuthorizeOverride={(reason) => handleApprove(true, reason)}
        isPending={isSubmitting}
      />

      {/* Rejection Confirmation Modal */}
      <Modal
        isOpen={isRejectModalOpen}
        onClose={() => setIsRejectModalOpen(false)}
        title="Reject Prescription Slip"
        maxWidth="md"
        isPatientTheme={false}
      >
        <form onSubmit={handleRejectConfirm} className="space-y-4">
          <p className="text-xs text-slate-600">
            Please provide a clinical rationale for rejecting this prescription. This will be recorded in the audit trail and the script will be marked as rejected.
          </p>

          <div className="space-y-1">
            <label className="block text-xs font-mono font-semibold text-slate-700 uppercase">
              Rejection Reason *
            </label>
            <textarea
              rows={3}
              value={rejectionReason}
              onChange={(e) => setRejectionReason(e.target.value)}
              placeholder="e.g. Unreadable handwriting, patient name discrepancy, incorrect physician stamp."
              className="w-full px-3 py-2 text-xs text-slate-900 bg-white border border-slate-300 rounded focus:outline-hidden focus:ring-1 focus:ring-slate-900"
              autoFocus
            />
            {rejectionError && (
              <span className="text-[11px] text-rose-600 font-medium">{rejectionError}</span>
            )}
          </div>

          <div className="flex items-center justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setIsRejectModalOpen(false)}
              className="px-3.5 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-300 rounded hover:bg-slate-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="px-4 py-1.5 text-xs font-semibold text-white bg-rose-700 hover:bg-rose-800 rounded transition-colors"
            >
              Confirm Rejection
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
