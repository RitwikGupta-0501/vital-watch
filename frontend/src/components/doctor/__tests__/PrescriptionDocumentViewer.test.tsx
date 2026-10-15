import { describe, it, expect, vi, beforeEach } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { PrescriptionDocumentViewer } from '../PrescriptionDocumentViewer';
import * as rxApi from '../../../lib/api/prescriptions';
import * as storage from '../../../lib/storage';

describe('PrescriptionDocumentViewer Component', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    vi.clearAllMocks();
  });

  const renderComponent = (prescriptionId = 'rx-123', fileName = 'test-scan.jpg') => {
    return render(
      <QueryClientProvider client={queryClient}>
        <PrescriptionDocumentViewer prescriptionId={prescriptionId} fileName={fileName} />
      </QueryClientProvider>
    );
  };

  it('renders loading state initially and then displays document image', async () => {
    vi.spyOn(rxApi, 'getPrescriptionDownloadUrl').mockResolvedValue({
      download_url: 'https://storage.vitalwatch.local/prescriptions/test-scan.jpg',
      expires_in: 300,
    });

    renderComponent('rx-123', 'test-scan.jpg');

    expect(screen.getByText(/Retrieving clinical document.../i)).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByAltText(/Handwritten prescription slip/i)).toBeInTheDocument();
    });

    expect(screen.getByText('100%')).toBeInTheDocument();
    expect(screen.getByText('test-scan.jpg')).toBeInTheDocument();
    expect(screen.getByText('Raster Image')).toBeInTheDocument();
  });

  it('handles zoom in, zoom out, and reset zoom controls', async () => {
    vi.spyOn(rxApi, 'getPrescriptionDownloadUrl').mockResolvedValue({
      download_url: 'https://storage.vitalwatch.local/prescriptions/test-scan.jpg',
      expires_in: 300,
    });

    renderComponent('rx-123', 'test-scan.jpg');

    await waitFor(() => {
      expect(screen.getByText('100%')).toBeInTheDocument();
    });

    const zoomInBtn = screen.getByTitle('Zoom in (+)');
    fireEvent.click(zoomInBtn);
    expect(screen.getByText('125%')).toBeInTheDocument();

    const zoomOutBtn = screen.getByTitle('Zoom out (-)');
    fireEvent.click(zoomOutBtn);
    fireEvent.click(zoomOutBtn);
    expect(screen.getByText('75%')).toBeInTheDocument();

    const resetBtn = screen.getByTitle('Reset zoom to 100%');
    fireEvent.click(resetBtn);
    expect(screen.getByText('100%')).toBeInTheDocument();
  });

  it('toggles high-contrast monochrome inversion filter', async () => {
    vi.spyOn(rxApi, 'getPrescriptionDownloadUrl').mockResolvedValue({
      download_url: 'https://storage.vitalwatch.local/prescriptions/test-scan.jpg',
      expires_in: 300,
    });

    renderComponent('rx-123', 'test-scan.jpg');

    await waitFor(() => {
      expect(screen.getByAltText(/Handwritten prescription slip/i)).toBeInTheDocument();
    });

    const invertBtn = screen.getByTitle(/Toggle high-contrast monochrome inversion/i);
    fireEvent.click(invertBtn);

    expect(screen.getByText('High-Contrast Active')).toBeInTheDocument();
  });

  it('renders native PDF iframe for PDF files', async () => {
    vi.spyOn(rxApi, 'getPrescriptionDownloadUrl').mockResolvedValue({
      download_url: 'https://storage.vitalwatch.local/prescriptions/test-doc.pdf',
      expires_in: 300,
    });

    renderComponent('rx-123', 'test-doc.pdf');

    await waitFor(() => {
      expect(screen.getByTitle('Prescription PDF Document')).toBeInTheDocument();
    });

    expect(screen.getByText('PDF Document')).toBeInTheDocument();
  });

  it('triggers presigned download via download button', async () => {
    const downloadSpy = vi.spyOn(storage, 'downloadFromPresignedUrl').mockImplementation(() => {});
    vi.spyOn(rxApi, 'getPrescriptionDownloadUrl').mockResolvedValue({
      download_url: 'https://storage.vitalwatch.local/prescriptions/test-scan.jpg',
      expires_in: 300,
    });

    renderComponent('rx-123', 'test-scan.jpg');

    await waitFor(() => {
      expect(screen.getByAltText(/Handwritten prescription slip/i)).toBeInTheDocument();
    });

    const dlBtn = screen.getByTitle('Download original file');
    fireEvent.click(dlBtn);

    expect(downloadSpy).toHaveBeenCalledWith(
      'https://storage.vitalwatch.local/prescriptions/test-scan.jpg',
      'test-scan.jpg'
    );
  });
});
