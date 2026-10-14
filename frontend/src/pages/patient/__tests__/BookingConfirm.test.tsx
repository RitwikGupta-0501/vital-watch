import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { BookingConfirm } from '../BookingConfirm';
import * as client from '../../../lib/api/client';

describe('BookingConfirm Component (Screen B4)', () => {
  let queryClient: QueryClient;

  const mockDoctor = {
    id: 'doc-999',
    first_name: 'Marcus',
    last_name: 'Vance',
    specialty: 'Internal Medicine',
    experience_years: 10,
    available: true,
    email: 'mvance@clinic.test',
  };

  const validUrl =
    '/patient/doctors/doc-999/book?start=2026-10-15T14%3A00%3A00Z&end=2026-10-15T14%3A30%3A00Z';

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
  });

  const renderWithRouter = (initialUrl = validUrl) =>
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[initialUrl]}>
          <Routes>
            <Route path="/patient/doctors/:id/book" element={<BookingConfirm />} />
            <Route path="/patient/appointments" element={<div>Appointments Page</div>} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders confirmation review with doctor details and modality selection', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation((url: string) => {
      if (url.includes('/api/doctors?')) {
        return Promise.resolve({ data: [mockDoctor], limit: 50, offset: 0 });
      }
      if (url.includes('/api/patient/appointments?')) {
        return Promise.resolve({ data: [], limit: 50, offset: 0 });
      }
      return Promise.reject(new Error(`Unhandled URL: ${url}`));
    });

    renderWithRouter();

    await waitFor(() => {
      expect(screen.getByText('Confirm Consultation')).toBeInTheDocument();
      expect(screen.getByText('Dr. Marcus Vance')).toBeInTheDocument();
      expect(screen.getByText('Virtual Telehealth')).toBeInTheDocument();
      expect(screen.getByText('In-Person Visit')).toBeInTheDocument();
    });

    // Toggle in-person visit
    const inPersonCard = screen.getByText('In-Person Visit');
    fireEvent.click(inPersonCard);
    expect(screen.getByText('Confirm & Reserve Slot')).toBeInTheDocument();
  });

  it('handles 409 double-booking conflict gracefully with empathetic recovery UI', async () => {
    vi.spyOn(client, 'fetchApi').mockImplementation((url: string, opts?: RequestInit) => {
      if (url.includes('/api/doctors?')) {
        return Promise.resolve({ data: [mockDoctor], limit: 50, offset: 0 });
      }
      if (url.includes('/api/patient/appointments?')) {
        return Promise.resolve({ data: [], limit: 50, offset: 0 });
      }
      if (url === '/api/appointments' && opts?.method === 'POST') {
        return Promise.reject(
          new client.ApiError(409, 'The selected time slot is already booked')
        );
      }
      return Promise.reject(new Error(`Unhandled URL: ${url}`));
    });

    renderWithRouter();

    await waitFor(() => {
      expect(screen.getByText('Confirm & Reserve Slot')).toBeInTheDocument();
    });

    const submitBtn = screen.getByText('Confirm & Reserve Slot');
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByText('Slot No Longer Available')).toBeInTheDocument();
      expect(screen.getByText('Pick Another Slot')).toBeInTheDocument();
    });
  });
});
