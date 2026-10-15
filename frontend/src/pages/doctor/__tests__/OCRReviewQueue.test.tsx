import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { OCRReviewQueue } from '../OCRReviewQueue';
import * as rxApi from '../../../lib/api/prescriptions';
import type { Prescription } from '../../../lib/api/types';

describe('OCRReviewQueue Component (Screen C9)', () => {
  let queryClient: QueryClient;

  const mockPrescriptions: Prescription[] = [
    {
      id: 'rx-001',
      patient_id: 'pat-100',
      doctor_id: 'doc-1',
      patient_name: 'Arthur Pendelton',
      source: 'uploaded',
      status: 'needs_review',
      ocr_provider: 'gemini-1.5-pro',
      file_name: 'prescription-pat-100-1.jpg',
      notes: 'Handwritten slip from outpatient visit',
      items: [
        {
          id: 'item-1',
          prescription_id: 'rx-001',
          medication_name: 'Amoxicillin',
          dosage: '500mg',
          frequency: 'Three times daily',
          duration: '7 days',
          created_at: '2026-10-14T10:00:00Z',
        },
      ],
      created_at: '2026-10-14T10:00:00Z',
      updated_at: '2026-10-14T10:02:00Z',
    },
    {
      id: 'rx-002',
      patient_id: 'pat-200',
      doctor_id: 'doc-1',
      patient_name: 'Elena Rostova',
      source: 'uploaded',
      status: 'needs_review',
      ocr_provider: 'claude-3-5-sonnet',
      file_name: 'prescription-pat-200-2.png',
      notes: '[OCR Extraction failed: model rate limited]',
      items: [],
      created_at: '2026-10-14T10:05:00Z',
      updated_at: '2026-10-14T10:06:00Z',
    },
    {
      id: 'rx-003',
      patient_id: 'pat-300',
      doctor_id: 'doc-1',
      patient_name: 'Marcus Vance',
      source: 'uploaded',
      status: 'pending_ocr',
      ocr_provider: undefined,
      file_name: 'prescription-pat-300-3.pdf',
      notes: '',
      items: [],
      created_at: '2026-10-14T10:10:00Z',
      updated_at: '2026-10-14T10:10:00Z',
    },
  ];

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    vi.clearAllMocks();
  });

  const renderComponent = () => {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <OCRReviewQueue />
        </MemoryRouter>
      </QueryClientProvider>
    );
  };

  it('renders queue header, status counters, and review table', async () => {
    vi.spyOn(rxApi, 'getPendingReviewPrescriptions').mockResolvedValue({
      data: mockPrescriptions,
      limit: 50,
      offset: 0,
    });

    renderComponent();

    expect(screen.getByText(/Screen C9 • Clinical Verification Queue/i)).toBeInTheDocument();
    expect(screen.getByText('Prescription Review Queue')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText('Arthur Pendelton')).toBeInTheDocument();
      expect(screen.getByText('Elena Rostova')).toBeInTheDocument();
      expect(screen.getByText('Marcus Vance')).toBeInTheDocument();
    });

    // Counts: 2 needs review, 1 pending OCR
    expect(screen.getAllByText(/Needs Review/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Pending OCR/i).length).toBeGreaterThan(0);

    // Provider labels
    expect(screen.getByText('gemini-1.5-pro')).toBeInTheDocument();
    expect(screen.getByText('claude-3-5-sonnet')).toBeInTheDocument();
    expect(screen.getByText(/River Worker\.\.\./i)).toBeInTheDocument();

    // 0-items failure note state handling
    expect(screen.getByText(/Extraction failed \(Manual Entry\)/i)).toBeInTheDocument();
  });

  it('filters prescriptions by status dropdown', async () => {
    vi.spyOn(rxApi, 'getPendingReviewPrescriptions').mockResolvedValue({
      data: mockPrescriptions,
      limit: 50,
      offset: 0,
    });

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Arthur Pendelton')).toBeInTheDocument();
    });

    const statusSelect = screen.getByDisplayValue(/All Pending/i);

    // Filter to only In OCR Queue (pending_ocr)
    fireEvent.change(statusSelect, { target: { value: 'pending_ocr' } });

    expect(screen.queryByText('Arthur Pendelton')).not.toBeInTheDocument();
    expect(screen.queryByText('Elena Rostova')).not.toBeInTheDocument();
    expect(screen.getByText('Marcus Vance')).toBeInTheDocument();
  });

  it('filters prescriptions by search input', async () => {
    vi.spyOn(rxApi, 'getPendingReviewPrescriptions').mockResolvedValue({
      data: mockPrescriptions,
      limit: 50,
      offset: 0,
    });

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Arthur Pendelton')).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/Search by patient name, medication, or prescription ID/i);
    fireEvent.change(searchInput, { target: { value: 'Amoxicillin' } });

    expect(screen.getByText('Arthur Pendelton')).toBeInTheDocument();
    expect(screen.queryByText('Elena Rostova')).not.toBeInTheDocument();
    expect(screen.queryByText('Marcus Vance')).not.toBeInTheDocument();
  });

  it('renders cleared empty state when no prescriptions are pending', async () => {
    vi.spyOn(rxApi, 'getPendingReviewPrescriptions').mockResolvedValue({
      data: [],
      limit: 50,
      offset: 0,
    });

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Review Queue Cleared')).toBeInTheDocument();
    });

    expect(
      screen.getByText(/All paper uploads and multimodal OCR extractions have been reviewed/i)
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Go to Patient Roster/i })).toBeInTheDocument();
  });
});
