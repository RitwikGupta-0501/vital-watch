import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { AppointmentsList } from '../AppointmentsList';
import * as client from '../../../lib/api/client';

describe('AppointmentsList Component (Screen B5)', () => {
  let queryClient: QueryClient;

  // Set appointment in future
  const futureStart = new Date(Date.now() + 2 * 24 * 60 * 60 * 1000).toISOString();
  const futureEnd = new Date(Date.now() + 2 * 24 * 60 * 60 * 1000 + 30 * 60 * 1000).toISOString();

  const mockAppointments = [
    {
      id: 'appt-1',
      doctor_id: 'doc-1',
      patient_id: 'pat-1',
      start_time: futureStart,
      end_time: futureEnd,
      status: 'upcoming' as const,
      type: 'virtual' as const,
      doctor_name: 'Sarah Connor',
      doctor_specialty: 'Cardiology',
      created_at: '2026-10-14T10:00:00Z',
    },
    {
      id: 'appt-2',
      doctor_id: 'doc-2',
      patient_id: 'pat-1',
      start_time: '2026-09-01T10:00:00Z',
      end_time: '2026-09-01T10:30:00Z',
      status: 'completed' as const,
      type: 'in_person' as const,
      doctor_name: 'Gregory House',
      doctor_specialty: 'Diagnostics',
      created_at: '2026-08-25T10:00:00Z',
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

  const renderWithProviders = () =>
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AppointmentsList />
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders upcoming appointments and switches tabs', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValue({
      data: mockAppointments,
      limit: 50,
      offset: 0,
    });

    renderWithProviders();

    await waitFor(() => {
      expect(screen.getByText('Sarah Connor')).toBeInTheDocument();
      expect(screen.getByText('Cardiology')).toBeInTheDocument();
      expect(screen.getByText('Virtual Telehealth')).toBeInTheDocument();
      expect(screen.getByText('Cancel Visit')).toBeInTheDocument();
    });

    // Switch to Past Records tab
    const pastTab = screen.getByText('Past Records');
    fireEvent.click(pastTab);

    expect(screen.getByText('Gregory House')).toBeInTheDocument();
    expect(screen.getByText('Diagnostics')).toBeInTheDocument();
    expect(screen.queryByText('Cancel Visit')).not.toBeInTheDocument();
  });

  it('opens confirmation modal and cancels upcoming appointment', async () => {
    const fetchSpy = vi
      .spyOn(client, 'fetchApi')
      .mockImplementation((url: string, opts?: RequestInit) => {
        if (url.includes('/api/patient/appointments')) {
          return Promise.resolve({ data: mockAppointments, limit: 50, offset: 0 });
        }
        if (url === '/api/appointments/appt-1/cancel' && opts?.method === 'PATCH') {
          return Promise.resolve({ message: 'Appointment cancelled successfully' });
        }
        return Promise.reject(new Error(`Unhandled URL: ${url}`));
      });

    renderWithProviders();

    await waitFor(() => {
      expect(screen.getByText('Cancel Visit')).toBeInTheDocument();
    });

    // Click Cancel Visit
    fireEvent.click(screen.getByText('Cancel Visit'));

    // Modal dialog is open
    expect(screen.getByText('Cancel Consultation')).toBeInTheDocument();
    expect(
      screen.getByText('Confirm Cancellation', { selector: 'button' })
    ).toBeInTheDocument();

    // Confirm Cancellation
    fireEvent.click(screen.getByText('Confirm Cancellation', { selector: 'button' }));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith('/api/appointments/appt-1/cancel', {
        method: 'PATCH',
      });
    });
  });
});
