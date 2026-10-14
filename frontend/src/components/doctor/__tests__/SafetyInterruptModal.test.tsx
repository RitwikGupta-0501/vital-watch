import { describe, it, expect, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SafetyInterruptModal } from '../SafetyInterruptModal';
import type { SafetyReport } from '../../../lib/api/types';

describe('SafetyInterruptModal Component (Screen C7)', () => {
  const mockSafetyReport: SafetyReport = {
    service_degraded: false,
    has_high_severity_alerts: true,
    allergy_alerts: [],
    interaction_alerts: [
      {
        drug1: 'Warfarin',
        drug2: 'Aspirin',
        severity: 'HIGH',
        description: 'Concurrent use markedly increases the risk of serious bleeding complications.',
      },
    ],
  };

  it('renders nothing when isOpen is false', () => {
    const { container } = render(
      <SafetyInterruptModal
        isOpen={false}
        onClose={vi.fn()}
        safetyReport={mockSafetyReport}
        onAuthorizeOverride={vi.fn()}
      />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when safetyReport is null', () => {
    const { container } = render(
      <SafetyInterruptModal
        isOpen={true}
        onClose={vi.fn()}
        safetyReport={null}
        onAuthorizeOverride={vi.fn()}
      />
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('renders DDI interaction alerts and drug pairs', () => {
    render(
      <SafetyInterruptModal
        isOpen={true}
        onClose={vi.fn()}
        safetyReport={mockSafetyReport}
        errorMessage="FDA Drug-Drug Contraindication detected"
        onAuthorizeOverride={vi.fn()}
      />
    );

    expect(screen.getByText('Clinical Safety Contraindication Detected')).toBeInTheDocument();
    expect(screen.getByText('FDA Drug-Drug Contraindication detected')).toBeInTheDocument();
    expect(screen.getByText('Warfarin')).toBeInTheDocument();
    expect(screen.getByText('Aspirin')).toBeInTheDocument();
    expect(
      screen.getByText('Concurrent use markedly increases the risk of serious bleeding complications.')
    ).toBeInTheDocument();
  });

  it('displays degraded service alert banner when service_degraded is true', () => {
    const degradedReport: SafetyReport = {
      ...mockSafetyReport,
      service_degraded: true,
    };

    render(
      <SafetyInterruptModal
        isOpen={true}
        onClose={vi.fn()}
        safetyReport={degradedReport}
        onAuthorizeOverride={vi.fn()}
      />
    );

    expect(
      screen.getByText(/The OpenFDA Drug Interaction service is offline or unreachable/i)
    ).toBeInTheDocument();
  });

  it('enforces >= 10 characters clinical justification before authorizing override', () => {
    const handleAuthorize = vi.fn();
    render(
      <SafetyInterruptModal
        isOpen={true}
        onClose={vi.fn()}
        safetyReport={mockSafetyReport}
        onAuthorizeOverride={handleAuthorize}
      />
    );

    const submitBtn = screen.getByRole('button', { name: /Authorize Override & Commit/i });
    expect(submitBtn).toBeDisabled();

    const textarea = screen.getByPlaceholderText(/Patient requires acute combination therapy/i);

    // Type 5 chars (less than 10)
    fireEvent.change(textarea, { target: { value: 'Short' } });
    expect(submitBtn).toBeDisabled();
    expect(handleAuthorize).not.toHaveBeenCalled();

    // Type valid rationale >= 10 chars
    fireEvent.change(textarea, {
      target: { value: 'Patient INR monitored weekly, low dose aspirin indicated.' },
    });
    expect(submitBtn).not.toBeDisabled();

    fireEvent.click(submitBtn);
    expect(handleAuthorize).toHaveBeenCalledWith(
      'Patient INR monitored weekly, low dose aspirin indicated.'
    );
  });

  it('calls onClose when Cancel & Modify Regimen is clicked', () => {
    const handleClose = vi.fn();
    render(
      <SafetyInterruptModal
        isOpen={true}
        onClose={handleClose}
        safetyReport={mockSafetyReport}
        onAuthorizeOverride={vi.fn()}
      />
    );

    const cancelBtn = screen.getByRole('button', { name: /Cancel & Modify Regimen/i });
    fireEvent.click(cancelBtn);
    expect(handleClose).toHaveBeenCalledTimes(1);
  });
});
