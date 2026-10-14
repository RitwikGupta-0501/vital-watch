import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { TodayTimeline } from '../TodayTimeline';
import * as client from '../../../lib/api/client';

describe('TodayTimeline Component (Screen C1)', () => {
  let queryClient: QueryClient;

  const now = new Date();

  // Future appointment today (cannot complete)
  const futureStart = new Date(now.getTime() + 2 * 60 * 60 * 1000).toISOString();
  const futureEnd = new Date(now.getTime() + 2.5 * 60 * 60 * 1000).toISOString();

  // Past / ongoing appointment today (can complete)
  const pastStart = new Date(now.getTime() - 30 * 60 * 1000).toISOString();
  const pastEnd = new Date(now.getTime() + 15 * 60 * 1000).toISOString();

  const mockAppointments = [
    {
      id: 'appt-future',
      doctor_id: 'doc-1',
      patient_id: 'pat-101',
      patient_name: 'Arthur Dent',
      start_time: futureStart,
      end_time: futureEnd,
      status: 'upcoming' as const,
      type: 'in_person' as const,
      created_at: '2026-10-14T08:00:00Z',
    },
    {
      id: 'appt-ready',
      doctor_id: 'doc-1',
      patient_id: 'pat-102',
      patient_name: 'Ford Prefect',
      start_time: pastStart,
      end_time: pastEnd,
      status: 'upcoming' as const,
      type: 'virtual' as const,
      meeting_link: 'https://vitalwatch.internal/room/102',
      created_at: '2026-10-14T08:00:00Z',
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
          <TodayTimeline />
        </MemoryRouter>
      </QueryClientProvider>
    );

  it("renders today's clinical agenda with KPI metrics and slots", async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: mockAppointments,
      limit: 100,
      offset: 0,
    });

    renderWithProviders();

    expect(await screen.findByText("Today's Clinical Agenda")).toBeInTheDocument();
    expect(await screen.findByText('Arthur Dent')).toBeInTheDocument();
    expect(screen.getByText('Ford Prefect')).toBeInTheDocument();

    // Verify KPI metrics (2 total, 1 in-person, 1 virtual)
    expect(screen.getByText('Scheduled Today')).toBeInTheDocument();
    expect(screen.getByText('In-Person')).toBeInTheDocument();
    expect(screen.getByText('Virtual Calls')).toBeInTheDocument();
  });

  it('enforces temporal completion gating: future appointment cannot be completed before start time', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: mockAppointments,
      limit: 100,
      offset: 0,
    });

    renderWithProviders();

    await screen.findByText('Arthur Dent');

    const completeButtons = screen.getAllByRole('button', { name: /Complete Visit/i });
    expect(completeButtons.length).toBe(2);

    // Chronologically sorted: appt-ready (past start) is index 0, appt-future (future start) is index 1
    // The ready/past appointment's button must be enabled
    expect(completeButtons[0]).toBeEnabled();

    // The future appointment's button must be disabled
    expect(completeButtons[1]).toBeDisabled();
  });

  it('completes consultation successfully when button is clicked on a valid appointment', async () => {
    const fetchSpy = vi.spyOn(client, 'fetchApi').mockImplementation(async (url, options) => {
      if (options?.method === 'PATCH' && url === '/api/appointments/appt-ready') {
        return { message: 'Appointment marked as completed' };
      }
      return {
        data: mockAppointments,
        limit: 100,
        offset: 0,
      };
    });

    renderWithProviders();

    await screen.findByText('Ford Prefect');

    const completeButtons = screen.getAllByRole('button', { name: /Complete Visit/i });
    const readyButton = completeButtons[0];
    expect(readyButton).toBeEnabled();

    fireEvent.click(readyButton);

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/appointments/appt-ready',
        expect.objectContaining({ method: 'PATCH' })
      );
    });
  });

  it('renders clean empty state when no appointments are scheduled for today', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: [],
      limit: 100,
      offset: 0,
    });

    renderWithProviders();

    expect(await screen.findByText('No Consultations Scheduled Today')).toBeInTheDocument();
    expect(screen.getByText('View Appointments Ledger')).toBeInTheDocument();
  });
});
