import React, { useState, useRef } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchApi } from '../../lib/api/client';
import {
  getPrescriptionUploadUrl,
  createUploadedPrescription,
} from '../../lib/api/prescriptions';
import { uploadFileToPresignedUrl } from '../../lib/storage';
import type { DoctorPatient, PaginatedResponse } from '../../lib/api/types';
import {
  ArrowLeft,
  Upload,
  FileText,
  Camera,
  CheckCircle2,
  AlertCircle,
  X,
  Loader2,
  ShieldAlert,
} from 'lucide-react';

const MAX_FILE_SIZE = 15 * 1024 * 1024; // 15 MB
const ALLOWED_EXTENSIONS = ['.jpg', '.jpeg', '.png', '.pdf'];

export function PrescribeUpload() {
  const { id: patientId } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [notes, setNotes] = useState('');
  const [isDragging, setIsDragging] = useState(false);
  const [uploadStatus, setUploadStatus] = useState<
    'idle' | 'presigning' | 'uploading' | 'registering' | 'success' | 'error'
  >('idle');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const cameraInputRef = useRef<HTMLInputElement>(null);

  // 1. Fetch Patient Info for clinical header
  const { data: patientsData } = useQuery<PaginatedResponse<DoctorPatient>>({
    queryKey: ['doctor-patients-roster'],
    queryFn: () => fetchApi<PaginatedResponse<DoctorPatient>>('/api/doctor/patients?limit=100&offset=0'),
  });

  const patient = patientsData?.data?.find((p) => p.id === patientId);

  // File selection validation
  const validateAndSetFile = (selectedFile: File) => {
    setErrorMessage(null);

    const ext = '.' + selectedFile.name.split('.').pop()?.toLowerCase();
    if (!ALLOWED_EXTENSIONS.includes(ext)) {
      setErrorMessage(`Invalid file format "${ext}". Only JPG, PNG, and PDF documents are supported.`);
      return;
    }

    if (selectedFile.size > MAX_FILE_SIZE) {
      setErrorMessage(`File exceeds 15 MB size limit (${(selectedFile.size / (1024 * 1024)).toFixed(1)} MB).`);
      return;
    }

    setFile(selectedFile);

    // Create thumbnail preview for images
    if (selectedFile.type.startsWith('image/')) {
      const url = URL.createObjectURL(selectedFile);
      setPreviewUrl(url);
    } else {
      setPreviewUrl(null);
    }
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      validateAndSetFile(e.dataTransfer.files[0]);
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      validateAndSetFile(e.target.files[0]);
    }
  };

  const clearFile = () => {
    if (previewUrl) {
      URL.revokeObjectURL(previewUrl);
    }
    setFile(null);
    setPreviewUrl(null);
    setErrorMessage(null);
    if (fileInputRef.current) fileInputRef.current.value = '';
    if (cameraInputRef.current) cameraInputRef.current.value = '';
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!file || !patientId) return;

    setErrorMessage(null);
    setUploadStatus('presigning');

    try {
      // Step A: Request Presigned Upload URL
      const presignRes = await getPrescriptionUploadUrl({
        patient_id: patientId,
        filename: file.name,
        content_type: file.type || 'application/octet-stream',
        file_size: file.size,
      });

      // Step B: Direct Binary PUT to Storage (S3 or Local)
      setUploadStatus('uploading');
      await uploadFileToPresignedUrl(presignRes.upload_url, file);

      // Step C: Register Uploaded Prescription in Database & Enqueue OCR Job
      setUploadStatus('registering');
      const createRes = await createUploadedPrescription({
        patient_id: patientId,
        file_name: presignRes.file_key,
        notes: notes.trim(),
      });

      setUploadStatus('success');

      // Navigate to review interface for this prescription
      setTimeout(() => {
        navigate(`/doctor/review/${createRes.id}`);
      }, 600);
    } catch (err: unknown) {
      setUploadStatus('error');
      if (err instanceof Error) {
        setErrorMessage(err.message);
      } else {
        setErrorMessage('Failed to upload prescription. Please verify your connection and try again.');
      }
    }
  };

  const isPDF = file?.name.toLowerCase().endsWith('.pdf');

  return (
    <div className="max-w-4xl mx-auto py-8 px-4 sm:px-6 space-y-6 font-sans">
      {/* Navigation & Clinical Breadcrumbs */}
      <div className="flex items-center justify-between">
        <Link
          to={`/doctor/patients/${patientId}`}
          className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-600 hover:text-slate-900 transition-colors"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Patient Chart
        </Link>

        <span className="text-[11px] font-mono text-slate-500 uppercase tracking-wider">
          Screen C8 • Paper Prescription Ingestion
        </span>
      </div>

      {/* Patient Clinical Context Card */}
      <div className="p-4 bg-white border border-slate-200 rounded-lg shadow-2xs flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <div className="text-xs font-mono text-slate-500 uppercase tracking-wider">Patient Record</div>
          <h2 className="text-base font-bold text-slate-900">
            {patient ? `${patient.first_name} ${patient.last_name}` : 'Clinical Patient'}
          </h2>
          <div className="text-xs font-mono text-slate-600 mt-0.5">
            ID: <span className="bg-slate-100 px-1 py-0.5 rounded border border-slate-200">{patientId}</span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <span className="text-xs px-2.5 py-1 font-mono rounded bg-emerald-50 text-emerald-800 border border-emerald-200">
            Established Relationship
          </span>
        </div>
      </div>

      {/* Main Upload Form Card */}
      <form onSubmit={handleSubmit} className="p-6 bg-white border border-slate-200 rounded-lg shadow-2xs space-y-6">
        <div>
          <h3 className="text-sm font-bold text-slate-900 uppercase tracking-wider font-mono">
            Document Ingestion & Multimodal OCR
          </h3>
          <p className="text-xs text-slate-600 mt-1">
            Upload a handwritten prescription slip, physical script photograph, or external doctor consultation scan.
          </p>
        </div>

        {/* Dropzone */}
        {!file ? (
          <div
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
            className={`border-2 border-dashed rounded-lg p-8 sm:p-12 text-center cursor-pointer transition-all ${
              isDragging
                ? 'border-brand-600 bg-brand-50/40'
                : 'border-slate-300 hover:border-slate-400 bg-slate-50/60 hover:bg-slate-50'
            }`}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".jpg,.jpeg,.png,.pdf"
              className="hidden"
              onChange={handleFileChange}
            />
            <input
              ref={cameraInputRef}
              type="file"
              accept="image/*"
              capture="environment"
              className="hidden"
              onChange={handleFileChange}
            />

            <div className="flex flex-col items-center gap-3">
              <div className="w-12 h-12 rounded-full bg-slate-100 border border-slate-200 flex items-center justify-center text-slate-600">
                <Upload className="w-6 h-6" />
              </div>

              <div className="space-y-1">
                <div className="text-sm font-semibold text-slate-900">
                  Click to select file or drag & drop prescription
                </div>
                <div className="text-xs font-mono text-slate-500">
                  Supported formats: JPEG, PNG, PDF • Max file size: 15 MB
                </div>
              </div>

              <div className="flex items-center gap-3 mt-2">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    fileInputRef.current?.click();
                  }}
                  className="px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-300 rounded hover:bg-slate-50 transition-colors"
                >
                  Browse Files
                </button>

                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    cameraInputRef.current?.click();
                  }}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-300 rounded hover:bg-slate-50 transition-colors"
                >
                  <Camera className="w-3.5 h-3.5 text-slate-500" />
                  Capture Photo
                </button>
              </div>
            </div>
          </div>
        ) : (
          /* File Selected Card */
          <div className="p-4 bg-slate-50 border border-slate-200 rounded-lg space-y-4">
            <div className="flex items-start justify-between gap-3">
              <div className="flex items-center gap-3">
                {previewUrl ? (
                  <img
                    src={previewUrl}
                    alt="Prescription preview"
                    className="w-16 h-16 object-cover rounded border border-slate-200 shadow-2xs"
                  />
                ) : (
                  <div className="w-16 h-16 rounded bg-slate-200/70 border border-slate-300 flex items-center justify-center text-slate-700">
                    <FileText className="w-8 h-8" />
                  </div>
                )}

                <div>
                  <div className="text-sm font-semibold text-slate-900 flex items-center gap-2">
                    {file.name}
                    {isPDF ? (
                      <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-amber-100 text-amber-900 border border-amber-200 font-semibold">
                        PDF
                      </span>
                    ) : (
                      <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-blue-100 text-blue-900 border border-blue-200 font-semibold">
                        IMAGE
                      </span>
                    )}
                  </div>
                  <div className="text-xs font-mono text-slate-500 mt-0.5">
                    {(file.size / (1024 * 1024)).toFixed(2)} MB • {file.type || 'Unknown MIME'}
                  </div>
                  <div className="text-xs text-emerald-700 flex items-center gap-1 mt-1 font-medium">
                    <CheckCircle2 className="w-3.5 h-3.5" /> Ready for upload & OCR analysis
                  </div>
                </div>
              </div>

              <button
                type="button"
                onClick={clearFile}
                disabled={uploadStatus !== 'idle' && uploadStatus !== 'error'}
                className="p-1.5 text-slate-400 hover:text-slate-700 hover:bg-slate-200 rounded transition-colors"
                title="Remove file"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Model Compatibility Nuance Banner */}
            {isPDF && (
              <div className="p-3 bg-amber-50/80 border border-amber-200/70 rounded text-xs text-amber-900 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-amber-700 shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold">MIME Compatibility Notice: </span>
                  PDF documents are supported by Gemini and Claude vision models, but rejected by OpenAI vision.
                  If the clinic uses an OpenAI-only fallback chain, high-resolution JPG or PNG photographs are recommended.
                </div>
              </div>
            )}
          </div>
        )}

        {/* Clinical Notes Input */}
        <div className="space-y-1.5">
          <label htmlFor="notes" className="block text-xs font-mono font-semibold text-slate-700 uppercase">
            Clinical Context / Prescriber Notes (Optional)
          </label>
          <textarea
            id="notes"
            rows={3}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            disabled={uploadStatus !== 'idle' && uploadStatus !== 'error'}
            placeholder="e.g. Patient brought handwritten slip from outpatient clinic. Preliminary diagnosis: acute pharyngitis."
            className="w-full px-3 py-2 text-xs text-slate-900 bg-white border border-slate-300 rounded focus:outline-hidden focus:ring-1 focus:ring-slate-900 placeholder:text-slate-400 font-sans"
          />
        </div>

        {/* Error Alert */}
        {errorMessage && (
          <div className="p-3 bg-rose-50 border border-rose-200 rounded text-xs text-rose-900 flex items-start gap-2">
            <ShieldAlert className="w-4 h-4 text-rose-700 shrink-0 mt-0.5" />
            <div>
              <span className="font-semibold">Upload Error: </span>
              {errorMessage}
            </div>
          </div>
        )}

        {/* Action Controls */}
        <div className="flex items-center justify-between pt-4 border-t border-slate-100">
          <button
            type="button"
            onClick={() => navigate(`/doctor/patients/${patientId}`)}
            disabled={uploadStatus !== 'idle' && uploadStatus !== 'error'}
            className="px-4 py-2 text-xs font-semibold text-slate-700 bg-white border border-slate-300 rounded hover:bg-slate-50 transition-colors"
          >
            Cancel
          </button>

          <button
            type="submit"
            disabled={!file || (uploadStatus !== 'idle' && uploadStatus !== 'error')}
            className="inline-flex items-center gap-2 px-5 py-2 text-xs font-semibold text-white bg-slate-900 hover:bg-slate-800 disabled:opacity-50 disabled:cursor-not-allowed rounded transition-colors shadow-2xs"
          >
            {uploadStatus === 'idle' || uploadStatus === 'error' ? (
              <>
                <Upload className="w-4 h-4" />
                Upload & Begin OCR Extraction
              </>
            ) : (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                {uploadStatus === 'presigning' && 'Requesting upload URL...'}
                {uploadStatus === 'uploading' && 'Transferring document...'}
                {uploadStatus === 'registering' && 'Triggering OCR pipeline...'}
                {uploadStatus === 'success' && 'Redirecting to review...'}
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}
