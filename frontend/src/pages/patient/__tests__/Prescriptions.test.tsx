import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { PrescriptionsList } from '../PrescriptionsList';
import { PrescriptionDetail } from '../PrescriptionDetail';
import * as client from '../../../lib/api/client';
import * as storage from '../../../lib/storage';

describe('Prescriptions Modules (Screens B8 & B9)', () => {
  let queryClient: QueryClient;

  const mockPrescription = {
    id: 'rx-777',
    patient_id: 'pat-1',
    doctor_id: 'doc-1',
    doctor_name: 'Sarah Connor',
    source: 'digital' as const,
    status: 'approved' as const,
    created_at: '2026-10-14T08:00:00Z',
    updated_at: '2026-10-14T08:00:00Z',
    file_name: 'prescription-rx-777.pdf',
    notes: 'Take with food. Complete the full course of antibiotics.',
    items: [
      {
        id: 'item-1',
        prescription_id: 'rx-777',
        medication_name: 'Amoxicillin',
        dosage: '500mg',
        frequency: 'Three times daily',
        duration: '10 days',
        timing: 'with meals',
        instructions: 'Do not crush tablets',
        created_at: '2026-10-14T08:00:00Z',
      },
    ],
  };

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
  });

  describe('PrescriptionsList (Screen B8)', () => {
    it('renders approved prescription list and triggers presigned PDF download', async () => {
      const downloadSpy = vi.spyOn(storage, 'downloadFromPresignedUrl').mockImplementation(() => {});

      vi.spyOn(client, 'fetchApi').mockImplementation((url: string) => {
        if (url.includes('/api/patient/prescriptions')) {
          return Promise.resolve({ data: [mockPrescription], limit: 20, offset: 0 });
        }
        if (url === '/api/prescriptions/rx-777/download-url') {
          return Promise.resolve({
            download_url: 'https://storage.local/prescriptions/rx-777.pdf',
            file_name: 'prescription-rx-777.pdf',
            expires_in: 300,
          });
        }
        return Promise.reject(new Error(`Unhandled URL: ${url}`));
      });

      render(
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>
            <PrescriptionsList />
          </MemoryRouter>
        </QueryClientProvider>
      );

      await waitFor(() => {
        expect(screen.getByText('Prescription Records')).toBeInTheDocument();
        expect(screen.getByText('Dr. Sarah Connor')).toBeInTheDocument();
        expect(screen.getByText('Amoxicillin')).toBeInTheDocument();
        expect(screen.getByText('500mg')).toBeInTheDocument();
      });

      // Click Download PDF
      const downloadBtn = screen.getByText('Download PDF');
      fireEvent.click(downloadBtn);

      await waitFor(() => {
        expect(downloadSpy).toHaveBeenCalledWith(
          'https://storage.local/prescriptions/rx-777.pdf',
          'prescription-rx-777.pdf'
        );
      });
    });
  });

  describe('PrescriptionDetail (Screen B9)', () => {
    it('renders structured medication items table and clinical notes', async () => {
      vi.spyOn(client, 'fetchApi').mockImplementation((url: string) => {
        if (url === '/api/prescriptions/rx-777') {
          return Promise.resolve(mockPrescription);
        }
        return Promise.reject(new Error(`Unhandled URL: ${url}`));
      });

      render(
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={['/patient/prescriptions/rx-777']}>
            <Routes>
              <Route path="/patient/prescriptions/:id" element={<PrescriptionDetail />} />
            </Routes>
          </MemoryRouter>
        </QueryClientProvider>
      );

      await waitFor(() => {
        expect(screen.getByText('Prescription Slip')).toBeInTheDocument();
        expect(screen.getByText('Prescribed Medications (1)')).toBeInTheDocument();
        expect(screen.getByText('Amoxicillin')).toBeInTheDocument();
        expect(screen.getByText('Three times daily')).toBeInTheDocument();
        expect(screen.getByText('Take with food. Complete the full course of antibiotics.')).toBeInTheDocument();
      });
    });
  });
});
