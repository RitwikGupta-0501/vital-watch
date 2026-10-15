import { fetchApi } from './client';
import type {
  Prescription,
  PaginatedResponse,
  PrescriptionUploadURLRequest,
  PrescriptionUploadURLResponse,
  CreateUploadedPrescriptionRequest,
  CreateUploadedPrescriptionResponse,
  VerifyPrescriptionRequest,
  VerifyPrescriptionResponse,
  PrescriptionDownloadResponse,
} from './types';

/**
 * Requests a presigned URL to directly upload a paper prescription scan/image.
 */
export async function getPrescriptionUploadUrl(
  params: PrescriptionUploadURLRequest
): Promise<PrescriptionUploadURLResponse> {
  return fetchApi<PrescriptionUploadURLResponse>('/api/prescriptions/upload-url', {
    method: 'POST',
    body: JSON.stringify(params),
  });
}

/**
 * Persists the prescription metadata after presigned upload completes and triggers background OCR.
 */
export async function createUploadedPrescription(
  params: CreateUploadedPrescriptionRequest
): Promise<CreateUploadedPrescriptionResponse> {
  return fetchApi<CreateUploadedPrescriptionResponse>('/api/prescriptions', {
    method: 'POST',
    body: JSON.stringify(params),
  });
}

/**
 * Retrieves prescriptions pending clinical review or in active OCR processing for the authenticated doctor.
 */
export async function getPendingReviewPrescriptions(
  limit = 50,
  offset = 0
): Promise<PaginatedResponse<Prescription>> {
  return fetchApi<PaginatedResponse<Prescription>>(
    `/api/prescriptions/pending-review?limit=${limit}&offset=${offset}`
  );
}

/**
 * Fetches complete prescription record by ID, including extracted items, provenance, and clinical notes.
 */
export async function getPrescriptionById(id: string): Promise<Prescription> {
  return fetchApi<Prescription>(`/api/prescriptions/${id}`);
}

/**
 * Retrieves a short-lived (300s) presigned download URL for the original paper prescription document.
 */
export async function getPrescriptionDownloadUrl(id: string): Promise<PrescriptionDownloadResponse> {
  return fetchApi<PrescriptionDownloadResponse>(`/api/prescriptions/${id}/download-url`);
}

/**
 * Clinician verification sign-off (approval or rejection) with validated medication lines and DDI overrides.
 */
export async function verifyPrescription(
  id: string,
  request: VerifyPrescriptionRequest
): Promise<VerifyPrescriptionResponse> {
  return fetchApi<VerifyPrescriptionResponse>(`/api/prescriptions/${id}/verify`, {
    method: 'PATCH',
    body: JSON.stringify(request),
  });
}
