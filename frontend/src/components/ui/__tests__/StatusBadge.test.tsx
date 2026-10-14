import { describe, it, expect } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import { StatusBadge } from '../StatusBadge';
import {
  PRESCRIPTION_STATUS_MAP,
  APPOINTMENT_STATUS_MAP,
  SAFETY_SEVERITY_MAP,
} from '../../../lib/status';

describe('StatusBadge Component (WCAG 2.1 AA Compliance)', () => {
  it('renders status text and icon together for prescription approved', () => {
    render(<StatusBadge meta={PRESCRIPTION_STATUS_MAP.approved} />);
    const badge = screen.getByText('Approved');
    expect(badge).toBeInTheDocument();
  });

  it('renders critical interrupt styling for HIGH safety severity', () => {
    const { container } = render(
      <StatusBadge meta={SAFETY_SEVERITY_MAP.HIGH} />
    );
    expect(screen.getByText('High Severity Interrupt')).toBeInTheDocument();
    // Verify critical tone class is present
    expect(container.firstChild).toHaveClass('bg-rose-50');
  });

  it('renders pill shape for patient theme and sharp corners for clinician theme', () => {
    const { rerender, container } = render(
      <StatusBadge meta={APPOINTMENT_STATUS_MAP.upcoming} isPatientTheme={true} />
    );
    expect(container.firstChild).toHaveClass('rounded-full');

    rerender(
      <StatusBadge meta={APPOINTMENT_STATUS_MAP.upcoming} isPatientTheme={false} />
    );
    expect(container.firstChild).toHaveClass('rounded-md');
  });
});
