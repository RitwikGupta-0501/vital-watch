import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DoctorScheduleSettings } from '../ScheduleSettings';
import * as client from '../../../lib/api/client';
import type { DoctorSchedule } from '../../../lib/api/types';

describe('DoctorScheduleSettings Component (Screen C12)', () => {
  let queryClient: QueryClient;

  const mockSchedules: DoctorSchedule[] = [
    {
      id: 'sched-1',
      doctor_id: 'doc-1',
      day_of_week: 1, // Monday
      start_time: '09:00',
      end_time: '17:00',
      slot_duration: 30,
      is_active: true,
      timezone: 'UTC',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    {
      id: 'sched-2',
      doctor_id: 'doc-1',
      day_of_week: 2, // Tuesday
      start_time: '10:00',
      end_time: '14:00',
      slot_duration: 20,
      is_active: true,
      timezone: 'UTC',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
  ];

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    vi.clearAllMocks();
  });

  const renderComponent = () => {
    return render(
      <QueryClientProvider client={queryClient}>
        <DoctorScheduleSettings />
      </QueryClientProvider>
    );
  };

  it('renders weekly schedule grid and slot capacity counts', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce(mockSchedules);

    renderComponent();

    expect(screen.getByText('Availability & Slots • Screen C12')).toBeInTheDocument();
    expect(screen.getByText('Office Hours & Consultation Grid')).toBeInTheDocument();

    // Check all days of the week are present and capacity from API is reflected
    await waitFor(() => {
      expect(screen.getByText('Sunday')).toBeInTheDocument();
      expect(screen.getByText('Monday')).toBeInTheDocument();
      expect(screen.getByText('Tuesday')).toBeInTheDocument();
      expect(screen.getByText('Wednesday')).toBeInTheDocument();
      expect(screen.getByText('Thursday')).toBeInTheDocument();
      expect(screen.getByText('Friday')).toBeInTheDocument();
      expect(screen.getByText('Saturday')).toBeInTheDocument();
      expect(screen.getAllByText('16 slots available').length).toBeGreaterThan(0);
      expect(screen.getByText('12 slots available')).toBeInTheDocument();
    });
  });

  it('saves weekly schedule via PUT /api/doctor/schedules endpoint', async () => {
    const fetchApiSpy = vi
      .spyOn(client, 'fetchApi')
      .mockResolvedValueOnce(mockSchedules) // Initial GET
      .mockResolvedValueOnce({ data: mockSchedules }); // PUT

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Monday')).toBeInTheDocument();
    });

    const saveBtn = screen.getByRole('button', { name: /Save Weekly Schedule/i });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(fetchApiSpy).toHaveBeenCalledWith(
        '/api/doctor/schedules',
        expect.objectContaining({
          method: 'PUT',
          body: expect.stringContaining('"slot_duration":30'),
        })
      );
    });

    await waitFor(() => {
      expect(
        screen.getByText('Office hours and slot durations saved successfully.')
      ).toBeInTheDocument();
    });
  });

  it('clears a day schedule via DELETE /api/doctor/schedules/:day endpoint', async () => {
    const fetchApiSpy = vi
      .spyOn(client, 'fetchApi')
      .mockResolvedValueOnce(mockSchedules) // Initial GET
      .mockResolvedValueOnce({ message: 'Deleted' }); // DELETE

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Monday')).toBeInTheDocument();
    });

    // Click trash button for Monday
    const clearMondayBtn = screen.getByTitle('Clear Monday office hours');
    fireEvent.click(clearMondayBtn);

    await waitFor(() => {
      expect(fetchApiSpy).toHaveBeenCalledWith(
        '/api/doctor/schedules/1',
        expect.objectContaining({
          method: 'DELETE',
        })
      );
    });

    await waitFor(() => {
      expect(
        screen.getByText('Monday schedule removed from available booking.')
      ).toBeInTheDocument();
    });
  });

  it('restricts slot duration selection to backend-whitelisted values only', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce(mockSchedules);

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Monday')).toBeInTheDocument();
    });

    // Check slot duration options on Monday
    const durationSelects = screen.getAllByLabelText(/Duration:/i);
    const options = Array.from(durationSelects[0].querySelectorAll('option')).map((o) =>
      Number(o.value)
    );

    expect(options).toEqual([15, 20, 30, 45, 60]);
  });
});
