import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { DoctorDetail } from '../DoctorDetail';
import * as client from '../../../lib/api/client';

describe('DoctorDetail Component (Screen B3)', () => {
  let queryClient: QueryClient;

  const mockDoctor = {
    id: 'doc-123',
    first_name: 'Elena',
    last_name: 'Rostova',
    specialty: 'Neurology',
    experience_years: 15,
    available: true,
    email: 'erostova@clinic.test',
  };

  const mockSlotsResponse = {
    date: '2026-10-15',
    day_of_week: 4,
    slot_duration: 30,
    timezone: 'America/New_York',
    slots: [
      {
        start_time: '2026-10-15T14:00:00Z',
        end_time: '2026-10-15T14:30:00Z',
        available: true,
      },
      {
        start_time: '2026-10-15T14:30:00Z',
        end_time: '2026-10-15T15:00:00Z',
        available: false,
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

  const renderWithRouter = (doctorId = 'doc-123') =>
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/patient/doctors/${doctorId}`]}>
          <Routes>
            <Route path="/patient/doctors/:id" element={<DoctorDetail />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders physician profile and timezone-aware slots', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation((url: string) => {
      if (url.includes('/api/doctors?')) {
        return Promise.resolve({ data: [mockDoctor], limit: 50, offset: 0 });
      }
      if (url.includes('/available-slots')) {
        return Promise.resolve(mockSlotsResponse);
      }
      return Promise.reject(new Error(`Unhandled URL: ${url}`));
    });

    renderWithRouter();

    await waitFor(() => {
      expect(screen.getByText('Dr. Elena Rostova')).toBeInTheDocument();
      expect(screen.getByText('Neurology')).toBeInTheDocument();
      expect(screen.getByText('30 min consultations')).toBeInTheDocument();
    });

    // Verify slot rendering
    const buttons = screen.getAllByRole('button');
    const availableSlotBtn = buttons.find((btn) => btn.textContent?.includes('Clinic:'));
    expect(availableSlotBtn).toBeInTheDocument();

    // Select available slot
    if (availableSlotBtn) {
      fireEvent.click(availableSlotBtn);
      expect(screen.getByText('Continue to Booking')).toBeInTheDocument();
    }
  });

  it('renders "Schedule Not Configured" empty state when no slots are configured', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation((url: string) => {
      if (url.includes('/api/doctors?')) {
        return Promise.resolve({ data: [mockDoctor], limit: 50, offset: 0 });
      }
      if (url.includes('/available-slots')) {
        return Promise.resolve({
          date: '2026-10-15',
          day_of_week: 4,
          slot_duration: 30,
          timezone: 'America/New_York',
          slots: [],
          message: 'Doctor is not available on this day',
        });
      }
      return Promise.reject(new Error(`Unhandled URL: ${url}`));
    });

    renderWithRouter();

    await waitFor(() => {
      expect(screen.getByText('Schedule Not Configured')).toBeInTheDocument();
    });
  });
});
