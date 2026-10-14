import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { DoctorAppointmentsList } from '../AppointmentsList';
import * as client from '../../../lib/api/client';

describe('DoctorAppointmentsList Component (Screen C2)', () => {
  let queryClient: QueryClient;

  const mockAppointments = [
    {
      id: 'appt-1',
      doctor_id: 'doc-1',
      patient_id: 'pat-101',
      patient_name: 'Ellen Ripley',
      start_time: '2026-10-20T10:00:00Z',
      end_time: '2026-10-20T10:30:00Z',
      status: 'upcoming' as const,
      type: 'in_person' as const,
      created_at: '2026-10-14T08:00:00Z',
    },
    {
      id: 'appt-2',
      doctor_id: 'doc-1',
      patient_id: 'pat-102',
      patient_name: 'Luke Skywalker',
      start_time: '2026-10-10T14:00:00Z',
      end_time: '2026-10-10T14:30:00Z',
      status: 'completed' as const,
      type: 'virtual' as const,
      created_at: '2026-10-05T08:00:00Z',
    },
    {
      id: 'appt-3',
      doctor_id: 'doc-1',
      patient_id: 'pat-103',
      patient_name: 'Han Solo',
      start_time: '2026-10-12T16:00:00Z',
      end_time: '2026-10-12T16:30:00Z',
      status: 'cancelled' as const,
      type: 'in_person' as const,
      created_at: '2026-10-06T08:00:00Z',
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
          <DoctorAppointmentsList />
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders appointments ledger with tabular data', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: mockAppointments,
      limit: 20,
      offset: 0,
    });

    renderWithProviders();

    expect(await screen.findByText('Appointments Ledger')).toBeInTheDocument();
    expect(await screen.findByText('Ellen Ripley')).toBeInTheDocument();
    expect(screen.getByText('Luke Skywalker')).toBeInTheDocument();
    expect(screen.getByText('Han Solo')).toBeInTheDocument();
  });

  it('filters appointments by status tabs', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: mockAppointments,
      limit: 20,
      offset: 0,
    });

    renderWithProviders();

    await screen.findByText('Ellen Ripley');

    // Click 'completed' filter
    const completedTab = screen.getByRole('button', { name: /^completed$/i });
    fireEvent.click(completedTab);

    // Luke Skywalker should be visible, Ellen Ripley and Han Solo should not
    expect(screen.getByText('Luke Skywalker')).toBeInTheDocument();
    expect(screen.queryByText('Ellen Ripley')).not.toBeInTheDocument();
    expect(screen.queryByText('Han Solo')).not.toBeInTheDocument();
  });

  it('opens cancellation dialog and submits cancellation request', async () => {
    const fetchSpy = vi.spyOn(client, 'fetchApi').mockImplementation(async (url, options) => {
      if (options?.method === 'PATCH' && url === '/api/appointments/appt-1/cancel') {
        return { message: 'Appointment cancelled' };
      }
      return {
        data: mockAppointments,
        limit: 20,
        offset: 0,
      };
    });

    renderWithProviders();

    await screen.findByText('Ellen Ripley');

    const cancelButton = screen.getByRole('button', { name: /^cancel$/i });
    fireEvent.click(cancelButton);

    // Modal dialog should open
    expect(await screen.findByText('Confirm Consultation Cancellation')).toBeInTheDocument();

    const confirmButton = screen.getByRole('button', { name: /Confirm Cancellation/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/appointments/appt-1/cancel',
        expect.objectContaining({ method: 'PATCH' })
      );
    });
  });
});
