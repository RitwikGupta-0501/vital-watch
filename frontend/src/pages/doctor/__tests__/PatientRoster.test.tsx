import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { DoctorPatientRoster } from '../PatientRoster';
import * as client from '../../../lib/api/client';

describe('DoctorPatientRoster Component (Screen C3)', () => {
  let queryClient: QueryClient;

  const mockPatients = [
    {
      id: 'pat-uuid-1',
      first_name: 'Dana',
      last_name: 'Scully',
      email: 'scully@fbi.gov',
      role: 'patient' as const,
      tenant_id: 'tenant-1',
      created_at: '2026-08-15T09:00:00Z',
    },
    {
      id: 'pat-uuid-2',
      first_name: 'Fox',
      last_name: 'Mulder',
      email: 'mulder@fbi.gov',
      role: 'patient' as const,
      tenant_id: 'tenant-1',
      created_at: '2026-09-01T10:00:00Z',
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
          <DoctorPatientRoster />
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders associated patient roster with names, emails, and action links', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: mockPatients,
      limit: 20,
      offset: 0,
    });

    renderWithProviders();

    expect(await screen.findByText('Associated Patient Roster')).toBeInTheDocument();
    expect(await screen.findByText('Dana Scully')).toBeInTheDocument();
    expect(screen.getByText('Fox Mulder')).toBeInTheDocument();
    expect(screen.getByText('scully@fbi.gov')).toBeInTheDocument();
    expect(screen.getByText('mulder@fbi.gov')).toBeInTheDocument();

    const chartLinks = screen.getAllByRole('link', { name: /Open Chart/i });
    expect(chartLinks.length).toBe(2);
    expect(chartLinks[0]).toHaveAttribute('href', '/doctor/patients/pat-uuid-1');
  });

  it('filters roster by search query', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: mockPatients,
      limit: 20,
      offset: 0,
    });

    renderWithProviders();

    await screen.findByText('Dana Scully');

    const searchInput = screen.getByPlaceholderText(/Search patients/i);
    fireEvent.change(searchInput, { target: { value: 'Mulder' } });

    expect(screen.getByText('Fox Mulder')).toBeInTheDocument();
    expect(screen.queryByText('Dana Scully')).not.toBeInTheDocument();
  });

  it('renders clean empty state when roster is empty', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValueOnce({
      data: [],
      limit: 20,
      offset: 0,
    });

    renderWithProviders();

    expect(await screen.findByText('No Patients in Roster')).toBeInTheDocument();
  });
});
