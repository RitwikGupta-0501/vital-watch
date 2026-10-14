import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { VerifyRx } from '../VerifyRx';
import * as client from '../../lib/api/client';

describe('VerifyRx Public Screen (Screen A4)', () => {
  let queryClient: QueryClient;

  const mockPublicVerifyData = {
    id: 'rx-verified-100',
    status: 'approved' as const,
    created_at: '2026-10-14T09:00:00Z',
    doctor_id: 'doc-1',
    doctor_name: 'Sarah Connor',
    masked_patient_id: '********-****-****-****-0123456789ab',
    notes: 'Standard antibiotic protocol.',
    items: [
      {
        id: 'item-10',
        prescription_id: 'rx-verified-100',
        medication_name: 'Azithromycin',
        dosage: '250mg',
        frequency: 'Once daily',
        instructions: 'Take 2 tablets on day 1, then 1 tablet daily',
        created_at: '2026-10-14T09:00:00Z',
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

  const renderWithRouter = (rxId = 'rx-verified-100') =>
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/verify/rx/${rxId}`]}>
          <Routes>
            <Route path="/verify/rx/:id" element={<VerifyRx />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders authentic certificate with masked patient identifier and verified medications', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValue(mockPublicVerifyData);

    renderWithRouter();

    await waitFor(() => {
      expect(screen.getByText('Authentic Clinical Prescription')).toBeInTheDocument();
      expect(screen.getByText('********-****-****-****-0123456789ab')).toBeInTheDocument();
      expect(screen.getByText('Dr. Sarah Connor')).toBeInTheDocument();
      expect(screen.getByText('Azithromycin')).toBeInTheDocument();
      expect(screen.getByText('250mg')).toBeInTheDocument();
      expect(screen.getByText('Standard antibiotic protocol.')).toBeInTheDocument();
    });
  });

  it('renders rate limit warning state on 429', async () => {
    vi.spyOn(client, 'fetchApi').mockRejectedValue(new client.RateLimitError(60));

    renderWithRouter();

    await waitFor(() => {
      expect(screen.getByText('Verification Rate Limit Reached')).toBeInTheDocument();
      expect(screen.getByText('Retry Verification')).toBeInTheDocument();
    });
  });

  it('renders unverified state on 404', async () => {
    vi.spyOn(client, 'fetchApi').mockRejectedValue(new client.ApiError(404, 'Prescription not found'));

    renderWithRouter();

    await waitFor(() => {
      expect(screen.getByText('Prescription Not Verified')).toBeInTheDocument();
      expect(screen.getByText('Return to VitalWatch')).toBeInTheDocument();
    });
  });
});
