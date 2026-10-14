import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { DoctorPatientChart } from '../PatientChart';
import * as client from '../../../lib/api/client';

describe('DoctorPatientChart Component (Screen C4)', () => {
  let queryClient: QueryClient;

  const mockPrescriptions = [
    {
      id: 'rx-101',
      patient_id: 'pat-101',
      doctor_id: 'doc-1',
      patient_name: 'Sarah Connor',
      source: 'digital' as const,
      status: 'approved' as const,
      notes: 'Initial therapy regimen',
      items: [
        {
          id: 'item-1',
          prescription_id: 'rx-101',
          medication_name: 'Metformin',
          dosage: '500 mg',
          frequency: 'Twice daily with meals',
          duration: '30 days',
          created_at: '2026-10-10T08:00:00Z',
        },
      ],
      created_at: '2026-10-10T08:00:00Z',
      updated_at: '2026-10-10T08:00:00Z',
    },
  ];

  const mockAppointments = [
    {
      id: 'appt-101',
      doctor_id: 'doc-1',
      patient_id: 'pat-101',
      patient_name: 'Sarah Connor',
      start_time: '2026-10-10T09:00:00Z',
      end_time: '2026-10-10T09:30:00Z',
      status: 'completed' as const,
      type: 'in_person' as const,
      created_at: '2026-10-01T08:00:00Z',
    },
  ];

  const mockVitals = [
    {
      id: 'vital-1',
      patient_id: 'pat-101',
      recorder_id: 'doc-1',
      recorder_role: 'doctor' as const,
      recorded_at: '2026-10-10T09:15:00Z',
      systolic_bp: 185,
      diastolic_bp: 125,
      heart_rate: 98,
      blood_glucose: 140,
      oxygen_saturation: 97,
      temperature: 37.1,
      weight_kg: 68.5,
      is_critical: true,
      critical_alerts: ['Hypertensive crisis: SBP >= 180 or DBP >= 120'],
      created_at: '2026-10-10T09:15:00Z',
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
  });

  const renderWithRouter = (patientId = 'pat-101') =>
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/doctor/patients/${patientId}`]}>
          <Routes>
            <Route path="/doctor/patients/:id" element={<DoctorPatientChart />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders longitudinal patient chart with header banner and prescription tab', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation(async (url) => {
      if (url.includes('/appointments')) {
        return { data: mockAppointments, limit: 50, offset: 0 };
      }
      if (url.includes('/prescriptions')) {
        return { data: mockPrescriptions, limit: 50, offset: 0 };
      }
      if (url.includes('/vitals')) {
        return { data: mockVitals, limit: 50, offset: 0 };
      }
      return { data: [], limit: 50, offset: 0 };
    });

    renderWithRouter();

    expect(await screen.findByText('Longitudinal Clinical Record • Screen C4')).toBeInTheDocument();
    expect(await screen.findByText('Sarah Connor')).toBeInTheDocument();
    expect(screen.getByText(/Patient UUID: pat-101/i)).toBeInTheDocument();

    // Default tab: Prescriptions
    expect(await screen.findByText('Metformin')).toBeInTheDocument();
    expect(screen.getByText('500 mg')).toBeInTheDocument();
    expect(screen.getByText(/Initial therapy regimen/i)).toBeInTheDocument();
  });

  it('switches to Consultation Visits tab and displays appointment history', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation(async (url) => {
      if (url.includes('/appointments')) {
        return { data: mockAppointments, limit: 50, offset: 0 };
      }
      if (url.includes('/prescriptions')) {
        return { data: mockPrescriptions, limit: 50, offset: 0 };
      }
      if (url.includes('/vitals')) {
        return { data: mockVitals, limit: 50, offset: 0 };
      }
      return { data: [], limit: 50, offset: 0 };
    });

    renderWithRouter();

    await screen.findByText('Sarah Connor');

    const visitsTab = screen.getByRole('button', { name: /Consultation Visits/i });
    fireEvent.click(visitsTab);

    expect(await screen.findByText(/In-Person/i)).toBeInTheDocument();
  });

  it('switches to Vitals tab and renders biometrics with critical alert flag', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation(async (url) => {
      if (url.includes('/appointments')) {
        return { data: mockAppointments, limit: 50, offset: 0 };
      }
      if (url.includes('/prescriptions')) {
        return { data: mockPrescriptions, limit: 50, offset: 0 };
      }
      if (url.includes('/vitals')) {
        return { data: mockVitals, limit: 50, offset: 0 };
      }
      return { data: [], limit: 50, offset: 0 };
    });

    renderWithRouter();

    await screen.findByText('Sarah Connor');

    const vitalsTab = screen.getByRole('button', { name: /Vitals & Biometrics/i });
    fireEvent.click(vitalsTab);

    expect(await screen.findByText('185/125 mmHg')).toBeInTheDocument();
    expect(screen.getByText('Critical Alert')).toBeInTheDocument();
  });

  it('opens Record Vitals modal and submits new vital reading', async () => {
    const fetchSpy = vi.spyOn(client, 'fetchApi').mockImplementation(async (url, options) => {
      if (options?.method === 'POST' && url === '/api/vitals') {
        return {
          id: 'vital-new',
          patient_id: 'pat-101',
          recorder_id: 'doc-1',
          recorder_role: 'doctor',
          recorded_at: new Date().toISOString(),
          systolic_bp: 120,
          diastolic_bp: 80,
          is_critical: false,
          created_at: new Date().toISOString(),
        };
      }
      if (url.includes('/appointments')) return { data: mockAppointments, limit: 50, offset: 0 };
      if (url.includes('/prescriptions')) return { data: mockPrescriptions, limit: 50, offset: 0 };
      if (url.includes('/vitals')) return { data: mockVitals, limit: 50, offset: 0 };
      return { data: [], limit: 50, offset: 0 };
    });

    renderWithRouter();

    await screen.findByText('Sarah Connor');

    const recordVitalsBtn = screen.getByRole('button', { name: /Record Vitals/i });
    fireEvent.click(recordVitalsBtn);

    // Modal dialog opens
    expect(await screen.findByText('Record Consultation Vitals')).toBeInTheDocument();

    // Fill in Systolic & Diastolic
    const systolicInput = screen.getByPlaceholderText('e.g. 120');
    const diastolicInput = screen.getByPlaceholderText('e.g. 80');
    fireEvent.change(systolicInput, { target: { value: '120' } });
    fireEvent.change(diastolicInput, { target: { value: '80' } });

    const submitBtn = screen.getByRole('button', { name: /Commit Vitals Record/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/vitals',
        expect.objectContaining({
          method: 'POST',
          body: expect.stringContaining('"systolic_bp":120'),
        })
      );
    });
  });
});
