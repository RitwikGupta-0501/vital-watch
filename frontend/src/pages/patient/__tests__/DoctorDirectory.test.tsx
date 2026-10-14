import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { DoctorDirectory } from '../DoctorDirectory';
import * as client from '../../../lib/api/client';

describe('DoctorDirectory Component (Screen B2)', () => {
  let queryClient: QueryClient;

  const mockDoctors = [
    {
      id: 'doc-1',
      first_name: 'Sarah',
      last_name: 'Connor',
      specialty: 'Cardiology',
      experience_years: 12,
      available: true,
      email: 'sconnor@clinic.test',
    },
    {
      id: 'doc-2',
      first_name: 'Gregory',
      last_name: 'House',
      specialty: 'Diagnostic Medicine',
      experience_years: 20,
      available: false,
      email: 'ghouse@clinic.test',
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
  });

  const renderWithProviders = () =>
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <DoctorDirectory />
        </MemoryRouter>
      </QueryClientProvider>
    );

  it('renders doctors list with specialties, experience, and availability badges', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValue({
      data: mockDoctors,
      limit: 12,
      offset: 0,
    });

    renderWithProviders();

    await waitFor(() => {
      expect(screen.getByText('Dr. Sarah Connor')).toBeInTheDocument();
      expect(screen.getByText('Cardiology')).toBeInTheDocument();
      expect(screen.getByText('12 years practice')).toBeInTheDocument();
      expect(screen.getByText('Available')).toBeInTheDocument();
      expect(screen.getByText('Dr. Gregory House')).toBeInTheDocument();
      expect(screen.getByText('Unavailable')).toBeInTheDocument();
    });

    // Check button states
    expect(screen.getByText('View Schedule & Book')).toBeInTheDocument();
    expect(screen.getByText('Not Accepting Bookings')).toBeDisabled();
  });

  it('filters doctors dynamically when searching by name or specialty', async () => {
    vi.spyOn(client, 'fetchApi').mockResolvedValue({
      data: mockDoctors,
      limit: 12,
      offset: 0,
    });

    renderWithProviders();

    await waitFor(() => {
      expect(screen.getByText('Dr. Sarah Connor')).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText('Search name or specialty...');
    fireEvent.change(searchInput, { target: { value: 'Cardiology' } });

    expect(screen.getByText('Dr. Sarah Connor')).toBeInTheDocument();
    expect(screen.queryByText('Dr. Gregory House')).not.toBeInTheDocument();

    // Search query with no match
    fireEvent.change(searchInput, { target: { value: 'Pediatrics' } });
    expect(screen.getByText('No Clinicians Found')).toBeInTheDocument();
  });
});
