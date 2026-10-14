import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { DoctorPrescriptionsList } from '../PrescriptionsList';
import * as client from '../../../lib/api/client';

describe('DoctorPrescriptionsList Component (Screen C11)', () => {
  let queryClient: QueryClient;

  const mockPatients = {
    data: [
      {
        id: 'pat-101',
        first_name: 'Sarah',
        last_name: 'Connor',
        email: 'sarah@skynet.com',
        phone: '+1 555-0101',
        created_at: '2026-01-01T00:00:00Z',
      },
      {
        id: 'pat-102',
        first_name: 'John',
        last_name: 'Doe',
        email: 'john@example.com',
        phone: '+1 555-0102',
        created_at: '2026-01-02T00:00:00Z',
      },
    ],
    total: 2,
    limit: 100,
    offset: 0,
  };

  const mockPrescriptions = {
    data: [
      {
        id: 'rx-101',
        patient_id: 'pat-101',
        doctor_id: 'doc-1',
        patient_name: 'Sarah Connor',
        source: 'digital' as const,
        status: 'approved' as const,
        notes: 'Hypertension management plan',
        items: [
          {
            id: 'item-1',
            prescription_id: 'rx-101',
            medication_name: 'Lisinopril',
            dosage: '10 mg',
            frequency: 'Daily',
            duration: '30 days',
            created_at: '2026-10-10T08:00:00Z',
          },
        ],
        created_at: '2026-10-10T08:00:00Z',
        updated_at: '2026-10-10T08:00:00Z',
      },
      {
        id: 'rx-102',
        patient_id: 'pat-101',
        doctor_id: 'doc-1',
        patient_name: 'Sarah Connor',
        source: 'uploaded' as const,
        status: 'pending_ocr' as const,
        notes: 'Paper slip digitization awaiting verification',
        items: [],
        created_at: '2026-10-11T08:00:00Z',
        updated_at: '2026-10-11T08:00:00Z',
      },
    ],
    total: 2,
    limit: 50,
    offset: 0,
  };

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
          <DoctorPrescriptionsList />
        </MemoryRouter>
      </QueryClientProvider>
    );
  };

  it('renders patient selector, prescription cards, and action links', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation((path: string) => {
      if (path.includes('/api/doctor/patients?')) {
        return Promise.resolve(mockPatients);
      }
      if (path.includes('/api/doctor/patients/pat-101/prescriptions')) {
        return Promise.resolve(mockPrescriptions);
      }
      return Promise.resolve({ data: [] });
    });

    renderComponent();

    expect(screen.getByText('Clinical Pharmacy • Screen C11')).toBeInTheDocument();
    expect(screen.getByText('Prescriptions Registry')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getAllByText(/Sarah Connor/i).length).toBeGreaterThan(0);
      expect(screen.getByText('Lisinopril')).toBeInTheDocument();
      expect(screen.getByText('10 mg')).toBeInTheDocument();
      expect(screen.getByText(/Daily • 30 days/)).toBeInTheDocument();
      expect(screen.getByText('Hypertension management plan')).toBeInTheDocument();
      expect(
        screen.getByText('Paper slip digitization awaiting verification')
      ).toBeInTheDocument();
    });

    // Check Write E-Prescription button points to effective patient
    const writeBtn = screen.getByRole('link', { name: /Write E-Prescription/i });
    expect(writeBtn).toHaveAttribute('href', '/doctor/patients/pat-101/prescribe/new');
  });

  it('filters prescriptions by status tab', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation((path: string) => {
      if (path.includes('/api/doctor/patients?')) {
        return Promise.resolve(mockPatients);
      }
      if (path.includes('/api/doctor/patients/pat-101/prescriptions')) {
        return Promise.resolve(mockPrescriptions);
      }
      return Promise.resolve({ data: [] });
    });

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Lisinopril')).toBeInTheDocument();
      expect(screen.getByText('Paper slip digitization awaiting verification')).toBeInTheDocument();
    });

    // Filter by pending_ocr status tab button
    const pendingTab = screen.getByRole('button', { name: /pending ocr/i });
    fireEvent.click(pendingTab);

    expect(screen.queryByText('Lisinopril')).not.toBeInTheDocument();
    expect(screen.getByText('Paper slip digitization awaiting verification')).toBeInTheDocument();
  });

  it('downloads PDF via prescription download url endpoint', async () => {
    const windowOpenSpy = vi.spyOn(window, 'open').mockImplementation(() => null);

    vi.spyOn(client, 'fetchApi').mockImplementation((path: string) => {
      if (path.includes('/api/doctor/patients?')) {
        return Promise.resolve(mockPatients);
      }
      if (path.includes('/api/doctor/patients/pat-101/prescriptions')) {
        return Promise.resolve(mockPrescriptions);
      }
      if (path.includes('/api/prescriptions/rx-101/download-url')) {
        return Promise.resolve({ download_url: 'https://cdn.vitalwatch.local/rx-101.pdf' });
      }
      return Promise.resolve({ data: [] });
    });

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Lisinopril')).toBeInTheDocument();
    });

    const downloadButtons = screen.getAllByTitle('Download prescription PDF');
    fireEvent.click(downloadButtons[0]);

    await waitFor(() => {
      expect(windowOpenSpy).toHaveBeenCalledWith(
        'https://cdn.vitalwatch.local/rx-101.pdf',
        '_blank'
      );
    });
  });

  it('renders empty state when patient has no prescriptions', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation((path: string) => {
      if (path.includes('/api/doctor/patients?')) {
        return Promise.resolve(mockPatients);
      }
      if (path.includes('/api/doctor/patients/pat-101/prescriptions')) {
        return Promise.resolve({ data: [], total: 0, limit: 50, offset: 0 });
      }
      return Promise.resolve({ data: [] });
    });

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('No Prescriptions Found')).toBeInTheDocument();
      expect(
        screen.getByText('No prescriptions have been issued for this patient yet.')
      ).toBeInTheDocument();
    });
  });
});
