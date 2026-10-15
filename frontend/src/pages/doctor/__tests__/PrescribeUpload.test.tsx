import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { PrescribeUpload } from '../PrescribeUpload';
import * as rxApi from '../../../lib/api/prescriptions';
import * as storage from '../../../lib/storage';
import * as client from '../../../lib/api/client';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('PrescribeUpload Component (Screen C8)', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    vi.clearAllMocks();

    // Mock patient query
    vi.spyOn(client, 'fetchApi').mockResolvedValue({
      data: [
        {
          id: 'pat-123',
          first_name: 'Arthur',
          last_name: 'Pendelton',
          email: 'arthur@example.com',
          role: 'patient',
          tenant_id: 'tenant-1',
          created_at: '2026-10-01T00:00:00Z',
        },
      ],
      limit: 100,
      offset: 0,
    });
  });

  const renderComponent = (patientId = 'pat-123') => {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/doctor/patients/${patientId}/prescribe/upload`]}>
          <Routes>
            <Route
              path="/doctor/patients/:id/prescribe/upload"
              element={<PrescribeUpload />}
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );
  };

  it('renders patient context and file dropzone', async () => {
    renderComponent('pat-123');

    expect(screen.getByText(/Screen C8 • Paper Prescription Ingestion/i)).toBeInTheDocument();
    expect(screen.getByText(/Document Ingestion & Multimodal OCR/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Click to select file or drag & drop prescription/i)
    ).toBeInTheDocument();
    expect(screen.getByText(/Max file size: 15 MB/i)).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText('Arthur Pendelton')).toBeInTheDocument();
    });
  });

  it('validates file extension and rejects unsupported formats', async () => {
    renderComponent('pat-123');

    // Create a mock invalid .exe file
    const file = new File(['dummy content'], 'malware.exe', { type: 'application/x-msdownload' });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;

    fireEvent.change(input, { target: { files: [file] } });

    expect(
      screen.getByText(/Invalid file format "\.exe"\. Only JPG, PNG, and PDF documents are supported\./i)
    ).toBeInTheDocument();
  });

  it('validates file size limit (> 15MB)', async () => {
    renderComponent('pat-123');

    // Create 16 MB mock file
    const oversizedFile = new File([new Uint8Array(16 * 1024 * 1024)], 'giant-scan.png', {
      type: 'image/png',
    });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;

    fireEvent.change(input, { target: { files: [oversizedFile] } });

    expect(screen.getByText(/File exceeds 15 MB size limit/i)).toBeInTheDocument();
  });

  it('displays OpenAI compatibility warning when selecting a PDF', async () => {
    renderComponent('pat-123');

    const pdfFile = new File(['%PDF-1.4 dummy'], 'prescription.pdf', {
      type: 'application/pdf',
    });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;

    fireEvent.change(input, { target: { files: [pdfFile] } });

    expect(screen.getByText(/MIME Compatibility Notice:/i)).toBeInTheDocument();
    expect(
      screen.getByText(/PDF documents are supported by Gemini and Claude vision models, but rejected by OpenAI/i)
    ).toBeInTheDocument();
  });

  it('executes two-step presigned upload and navigates to review on submit', async () => {
    const uploadUrlSpy = vi.spyOn(rxApi, 'getPrescriptionUploadUrl').mockResolvedValue({
      upload_url: 'https://storage.vitalwatch.local/upload/signed-token-123',
      file_key: 'prescription-pat-123-uuid-456.png',
      expires_in: 300,
    });
    const uploadBytesSpy = vi.spyOn(storage, 'uploadFileToPresignedUrl').mockResolvedValue();
    const createPrescriptionSpy = vi.spyOn(rxApi, 'createUploadedPrescription').mockResolvedValue({
      id: 'rx-new-999',
      filename: 'prescription-pat-123-uuid-456.png',
      status: 'pending_ocr',
    });

    renderComponent('pat-123');

    const validFile = new File(['valid image bytes'], 'rx-slip.png', { type: 'image/png' });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;

    fireEvent.change(input, { target: { files: [validFile] } });

    expect(screen.getByText('rx-slip.png')).toBeInTheDocument();

    const submitBtn = screen.getByRole('button', { name: /Upload & Begin OCR Extraction/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(uploadUrlSpy).toHaveBeenCalledWith({
        patient_id: 'pat-123',
        filename: 'rx-slip.png',
        content_type: 'image/png',
        file_size: 17,
      });
      expect(uploadBytesSpy).toHaveBeenCalledWith(
        'https://storage.vitalwatch.local/upload/signed-token-123',
        validFile
      );
      expect(createPrescriptionSpy).toHaveBeenCalledWith({
        patient_id: 'pat-123',
        file_name: 'prescription-pat-123-uuid-456.png',
        notes: '',
      });
    });

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/doctor/review/rx-new-999');
    });
  });
});
