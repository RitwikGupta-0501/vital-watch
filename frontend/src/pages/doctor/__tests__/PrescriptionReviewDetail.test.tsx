import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { PrescriptionReviewDetail } from '../PrescriptionReviewDetail';
import * as rxApi from '../../../lib/api/prescriptions';
import { ApiError } from '../../../lib/api/client';
import type { Prescription } from '../../../lib/api/types';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('PrescriptionReviewDetail Component (Screen C10)', () => {
  let queryClient: QueryClient;

  const mockPrescription: Prescription = {
    id: 'rx-test-456',
    patient_id: 'pat-100',
    doctor_id: 'doc-1',
    patient_name: 'Arthur Pendelton',
    source: 'uploaded',
    status: 'needs_review',
    ocr_provider: 'gemini-1.5-pro',
    file_name: 'prescription-pat-100-test.jpg',
    notes: 'Outpatient clinical visit note',
    items: [
      {
        id: 'item-1',
        prescription_id: 'rx-test-456',
        medication_name: 'Amoxicillin',
        dosage: '500mg',
        frequency: 'TID',
        duration: '7 days',
        timing: 'After meals',
        instructions: 'Take with water',
        created_at: '2026-10-14T10:00:00Z',
      },
    ],
    created_at: '2026-10-14T10:00:00Z',
    updated_at: '2026-10-14T10:02:00Z',
  };

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    vi.clearAllMocks();

    vi.spyOn(rxApi, 'getPrescriptionDownloadUrl').mockResolvedValue({
      download_url: 'https://storage.vitalwatch.local/prescriptions/prescription-pat-100-test.jpg',
      expires_in: 300,
    });
  });

  const renderComponent = (prescriptionId = 'rx-test-456') => {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/doctor/review/${prescriptionId}`]}>
          <Routes>
            <Route path="/doctor/review/:id" element={<PrescriptionReviewDetail />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );
  };

  it('renders split-screen view with document viewer and editable medication fields', async () => {
    vi.spyOn(rxApi, 'getPrescriptionById').mockResolvedValue(mockPrescription);

    renderComponent('rx-test-456');

    expect(screen.getByText(/Loading clinical prescription review/i)).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText('Arthur Pendelton')).toBeInTheDocument();
      expect(screen.getByText(/Screen C10 • HITL Verification/i)).toBeInTheDocument();
      expect(screen.getByText('gemini-1.5-pro')).toBeInTheDocument();
    });

    // Check pre-populated medication item
    expect(screen.getByDisplayValue('Amoxicillin')).toBeInTheDocument();
    expect(screen.getByDisplayValue('500mg')).toBeInTheDocument();
    expect(screen.getByDisplayValue('TID')).toBeInTheDocument();
    expect(screen.getByDisplayValue('7 days')).toBeInTheDocument();

    // Check actions
    expect(screen.getByRole('button', { name: /Approve & Issue Prescription/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Reject Prescription/i })).toBeInTheDocument();
  });

  it('dynamically adds and removes medication items', async () => {
    vi.spyOn(rxApi, 'getPrescriptionById').mockResolvedValue(mockPrescription);

    renderComponent('rx-test-456');

    await waitFor(() => {
      expect(screen.getByDisplayValue('Amoxicillin')).toBeInTheDocument();
    });

    // Click Add Medication Item
    const addBtn = screen.getByRole('button', { name: /Add Medication Item/i });
    fireEvent.click(addBtn);

    expect(screen.getByText('#2 Medication')).toBeInTheDocument();

    // Remove the added item
    const deleteBtns = screen.getAllByTitle('Remove item');
    fireEvent.click(deleteBtns[1]);

    expect(screen.queryByText('#2 Medication')).not.toBeInTheDocument();
  });

  it('refuses approval with 0 items when list is cleared', async () => {
    vi.spyOn(rxApi, 'getPrescriptionById').mockResolvedValue(mockPrescription);

    renderComponent('rx-test-456');

    await waitFor(() => {
      expect(screen.getByDisplayValue('Amoxicillin')).toBeInTheDocument();
    });

    // Remove the only item
    const deleteBtn = screen.getByTitle('Remove item');
    fireEvent.click(deleteBtn);

    // Click Approve
    const approveBtn = screen.getByRole('button', { name: /Approve & Issue Prescription/i });
    fireEvent.click(approveBtn);

    expect(
      screen.getByText(/At least one medication item is required to approve this prescription/i)
    ).toBeInTheDocument();
  });

  it('submits clinical approval via verifyPrescription and navigates to review queue', async () => {
    vi.spyOn(rxApi, 'getPrescriptionById').mockResolvedValue(mockPrescription);
    const verifySpy = vi.spyOn(rxApi, 'verifyPrescription').mockResolvedValue({
      message: 'Prescription verified successfully',
      status: 'approved',
    });

    renderComponent('rx-test-456');

    await waitFor(() => {
      expect(screen.getByDisplayValue('Amoxicillin')).toBeInTheDocument();
    });

    // Edit dosage
    const dosageInput = screen.getByDisplayValue('500mg');
    fireEvent.change(dosageInput, { target: { value: '875mg' } });

    const approveBtn = screen.getByRole('button', { name: /Approve & Issue Prescription/i });
    fireEvent.click(approveBtn);

    await waitFor(() => {
      expect(verifySpy).toHaveBeenCalledWith('rx-test-456', {
        status: 'approved',
        items: [
          {
            medication_name: 'Amoxicillin',
            dosage: '875mg',
            frequency: 'TID',
            duration: '7 days',
            timing: 'After meals',
            instructions: 'Take with water',
          },
        ],
        notes: 'Outpatient clinical visit note',
        override_safety: false,
        override_reason: '',
      });
    });

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/doctor/review');
    });
  });

  it('handles 409 DDI safety interrupt by opening SafetyInterruptModal and resubmitting with override', async () => {
    vi.spyOn(rxApi, 'getPrescriptionById').mockResolvedValue(mockPrescription);

    // Mock first verification call failing with 409 Conflict
    const safetyReport = {
      has_high_severity_alerts: true,
      service_degraded: false,
      interaction_alerts: [
        {
          drug_1: 'Amoxicillin',
          drug_2: 'Methotrexate',
          severity: 'HIGH' as const,
          description: 'Penicillins may decrease the excretion of methotrexate leading to toxicity.',
        },
      ],
    };

    const verifySpy = vi
      .spyOn(rxApi, 'verifyPrescription')
      .mockRejectedValueOnce(
        new ApiError(409, 'Critical drug interaction detected', {
          requires_override: true,
          safety_report: safetyReport,
          error: 'Critical drug interaction detected',
        })
      )
      .mockResolvedValueOnce({
        message: 'Prescription verified successfully',
        status: 'approved',
      });

    renderComponent('rx-test-456');

    await waitFor(() => {
      expect(screen.getByDisplayValue('Amoxicillin')).toBeInTheDocument();
    });

    const approveBtn = screen.getByRole('button', { name: /Approve & Issue Prescription/i });
    fireEvent.click(approveBtn);

    // Expect Safety Interrupt Modal (C7) to open
    await waitFor(() => {
      expect(
        screen.getByText('Clinical Safety Contraindication Detected')
      ).toBeInTheDocument();
      expect(screen.getByText(/Critical drug interaction detected/i)).toBeInTheDocument();
      expect(screen.getByText(/Penicillins may decrease the excretion/i)).toBeInTheDocument();
    });

    // Enter clinical justification override
    const overrideInput = screen.getByPlaceholderText(
      /Patient requires acute combination therapy/i
    );
    fireEvent.change(overrideInput, {
      target: { value: 'Patient instructed on close therapeutic drug monitoring and regular hydration.' },
    });

    const overrideBtn = screen.getByRole('button', {
      name: /Authorize Override & Commit/i,
    });
    fireEvent.click(overrideBtn);

    await waitFor(() => {
      expect(verifySpy).toHaveBeenCalledTimes(2);
      expect(verifySpy).toHaveBeenLastCalledWith('rx-test-456', {
        status: 'approved',
        items: [
          {
            medication_name: 'Amoxicillin',
            dosage: '500mg',
            frequency: 'TID',
            duration: '7 days',
            timing: 'After meals',
            instructions: 'Take with water',
          },
        ],
        notes: 'Outpatient clinical visit note',
        override_safety: true,
        override_reason: 'Patient instructed on close therapeutic drug monitoring and regular hydration.',
      });
    });

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/doctor/review');
    });
  });

  it('opens rejection modal, inputs reason, and submits rejection', async () => {
    vi.spyOn(rxApi, 'getPrescriptionById').mockResolvedValue(mockPrescription);
    const verifySpy = vi.spyOn(rxApi, 'verifyPrescription').mockResolvedValue({
      message: 'Prescription rejected',
      status: 'rejected',
    });

    renderComponent('rx-test-456');

    await waitFor(() => {
      expect(screen.getByDisplayValue('Amoxicillin')).toBeInTheDocument();
    });

    const rejectBtn = screen.getByRole('button', { name: /Reject Prescription/i });
    fireEvent.click(rejectBtn);

    // Expect Rejection Modal
    expect(screen.getByText('Reject Prescription Slip')).toBeInTheDocument();

    const reasonInput = screen.getByPlaceholderText(/Unreadable handwriting, patient name discrepancy/i);
    fireEvent.change(reasonInput, { target: { value: 'Illegible doctor signature on physical slip' } });

    const confirmRejectBtn = screen.getByRole('button', { name: /Confirm Rejection/i });
    fireEvent.click(confirmRejectBtn);

    await waitFor(() => {
      expect(verifySpy).toHaveBeenCalledWith('rx-test-456', {
        status: 'rejected',
        notes: 'Outpatient clinical visit note [Rejected: Illegible doctor signature on physical slip]',
      });
    });

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/doctor/review');
    });
  });
});
