import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { DigitalPrescribe } from '../DigitalPrescribe';
import * as client from '../../../lib/api/client';

describe('DigitalPrescribe Component (Screen C6)', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    vi.clearAllMocks();
  });

  const renderComponent = (patientId = 'pat-test-123') => {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/doctor/patients/${patientId}/prescribe/new`]}>
          <Routes>
            <Route
              path="/doctor/patients/:id/prescribe/new"
              element={<DigitalPrescribe />}
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );
  };

  it('renders initial empty drug item row and form controls', () => {
    renderComponent('pat-456');

    expect(screen.getByText('Clinical Workflow • Screen C6')).toBeInTheDocument();
    expect(screen.getByText('Author Digital E-Prescription')).toBeInTheDocument();
    expect(screen.getByText(/Target Patient UUID:/i)).toBeInTheDocument();
    expect(screen.getByText('pat-456')).toBeInTheDocument();

    expect(
      screen.getByPlaceholderText(/Amoxicillin, Lisinopril, Warfarin/i)
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/500 mg, 10 ml/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Add Medication Item/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Issue Verified Prescription/i })).toBeInTheDocument();
  });

  it('adds and removes drug items dynamically', () => {
    renderComponent('pat-456');

    const addBtn = screen.getByRole('button', { name: /Add Medication Item/i });

    // Initially 1 drug item (Item #1)
    expect(screen.getByText(/Item #1/i)).toBeInTheDocument();
    expect(screen.queryByText(/Item #2/i)).not.toBeInTheDocument();

    // Add another drug
    fireEvent.click(addBtn);
    expect(screen.getByText(/Item #1/i)).toBeInTheDocument();
    expect(screen.getByText(/Item #2/i)).toBeInTheDocument();

    // Remove drug #2
    const removeButtons = screen.getAllByTitle('Remove medication');
    fireEvent.click(removeButtons[0]);

    expect(screen.queryByText(/Item #2/i)).not.toBeInTheDocument();
  });

  it('validates required fields before submitting', () => {
    renderComponent('pat-456');

    const submitBtn = screen.getByRole('button', { name: /Issue Verified Prescription/i });
    fireEvent.click(submitBtn);

    expect(
      screen.getByText('Item #1: Medication name is required.')
    ).toBeInTheDocument();
  });

  it('completes 201 success flow and displays confirmation with PDF and verify links', async () => {
    const fetchApiSpy = vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      id: 'rx-created-999',
      prescription_number: 'RX-2026-999',
      source: 'digital',
      status: 'approved',
      qr_code: 'vitalwatch:rx:verify:rx-created-999',
      download_url: 'https://vitalwatch.local/prescriptions/rx-created-999.pdf',
    });

    renderComponent('pat-456');

    // Fill in required fields
    fireEvent.change(screen.getByPlaceholderText(/Amoxicillin, Lisinopril, Warfarin/i), {
      target: { value: 'Atorvastatin' },
    });
    fireEvent.change(screen.getByPlaceholderText(/500 mg, 10 ml/i), {
      target: { value: '20 mg' },
    });
    fireEvent.change(screen.getByPlaceholderText(/Twice daily, Every 8 hours/i), {
      target: { value: 'Once daily' },
    });
    fireEvent.change(screen.getByPlaceholderText(/7 days, 1 month/i), {
      target: { value: '30 days' },
    });

    const submitBtn = screen.getByRole('button', { name: /Issue Verified Prescription/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(fetchApiSpy).toHaveBeenCalledWith(
        '/api/prescriptions/digital',
        expect.objectContaining({
          method: 'POST',
        })
      );
    });

    // Check success screen
    await waitFor(() => {
      expect(
        screen.getByText('Digital E-Prescription Issued Successfully')
      ).toBeInTheDocument();
      expect(screen.getByText('rx-created-999')).toBeInTheDocument();
      expect(screen.getByRole('link', { name: /Verify Public QR Target/i })).toHaveAttribute(
        'href',
        '/verify/rx/rx-created-999'
      );
      expect(screen.getByRole('button', { name: /Download Generated PDF/i })).toBeInTheDocument();
    });
  });

  it('intercepts 409 conflict and triggers safety interrupt modal', async () => {
    const error409 = new client.ApiError(409, 'Clinical contraindication detected', {
      error: 'Drug-Drug Interaction contraindicated',
      requires_override: true,
      safety_report: {
        service_degraded: false,
        has_high_severity_alerts: true,
        allergy_alerts: [],
        interaction_alerts: [
          {
            drug1: 'Warfarin',
            drug2: 'Aspirin',
            severity: 'HIGH',
            description: 'Concurrent use markedly increases the risk of serious bleeding complications.',
          },
        ],
      },
    });

    const fetchApiSpy = vi
      .spyOn(client, 'fetchApi')
      .mockRejectedValueOnce(error409) // First call triggers 409
      .mockResolvedValueOnce({
        id: 'rx-overridden-888',
        prescription_number: 'RX-2026-888',
        source: 'digital',
        status: 'approved',
        qr_code: 'vitalwatch:rx:verify:rx-overridden-888',
        download_url: 'https://vitalwatch.local/prescriptions/rx-overridden-888.pdf',
      });

    renderComponent('pat-456');

    // Fill in required fields
    fireEvent.change(screen.getByPlaceholderText(/Amoxicillin, Lisinopril, Warfarin/i), {
      target: { value: 'Warfarin' },
    });
    fireEvent.change(screen.getByPlaceholderText(/500 mg, 10 ml/i), {
      target: { value: '5 mg' },
    });
    fireEvent.change(screen.getByPlaceholderText(/Twice daily, Every 8 hours/i), {
      target: { value: 'Once daily' },
    });
    fireEvent.change(screen.getByPlaceholderText(/7 days, 1 month/i), {
      target: { value: '30 days' },
    });

    const submitBtn = screen.getByRole('button', { name: /Issue Verified Prescription/i });
    fireEvent.click(submitBtn);

    // Modal should now be open
    await waitFor(() => {
      expect(screen.getByText('Clinical Safety Contraindication Detected')).toBeInTheDocument();
      expect(
        screen.getByText('Concurrent use markedly increases the risk of serious bleeding complications.')
      ).toBeInTheDocument();
    });

    // Authorize override with clinical justification
    const textarea = screen.getByPlaceholderText(/Patient requires acute combination therapy/i);
    fireEvent.change(textarea, {
      target: { value: 'Cardiology reviewed INR daily, essential short term indication.' },
    });

    const authorizeBtn = screen.getByRole('button', { name: /Authorize Override & Commit/i });
    fireEvent.click(authorizeBtn);

    // Should call API again with override_safety: true and override_reason
    await waitFor(() => {
      expect(fetchApiSpy).toHaveBeenLastCalledWith(
        '/api/prescriptions/digital',
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"override_safety":true'),
        })
      );
    });

    await waitFor(() => {
      expect(
        screen.getByText('Digital E-Prescription Issued Successfully')
      ).toBeInTheDocument();
    });
  });
});
