import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getPrescriptionDownloadUrl } from '../../lib/api/prescriptions';
import { downloadFromPresignedUrl } from '../../lib/storage';
import {
  ZoomIn,
  ZoomOut,
  RotateCw,
  Sliders,
  Download,
  AlertCircle,
  RefreshCw,
  Clock,
} from 'lucide-react';

interface PrescriptionDocumentViewerProps {
  prescriptionId: string;
  fileName?: string;
}

export function PrescriptionDocumentViewer({
  prescriptionId,
  fileName,
}: PrescriptionDocumentViewerProps) {
  const [zoomLevel, setZoomLevel] = useState<number>(100);
  const [rotation, setRotation] = useState<number>(0);
  const [isInverted, setIsInverted] = useState<boolean>(false);
  const [timeLeft, setTimeLeft] = useState<number>(300);

  // 1. Fetch short-lived presigned download URL
  const {
    data: downloadData,
    isLoading,
    isError,
    refetch,
  } = useQuery({
    queryKey: ['prescription-download-url', prescriptionId],
    queryFn: () => getPrescriptionDownloadUrl(prescriptionId),
    staleTime: 240000, // 4 minutes
    refetchOnWindowFocus: false,
  });

  // 2. Countdown timer for 300s presigned URL expiration
  useEffect(() => {
    if (!downloadData?.expires_in) return;
    setTimeLeft(downloadData.expires_in);

    const interval = setInterval(() => {
      setTimeLeft((prev) => {
        if (prev <= 1) {
          clearInterval(interval);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [downloadData]);

  const isPDF =
    fileName?.toLowerCase().endsWith('.pdf') ||
    downloadData?.download_url.toLowerCase().includes('.pdf');

  const handleZoomIn = () => setZoomLevel((prev) => Math.min(prev + 25, 250));
  const handleZoomOut = () => setZoomLevel((prev) => Math.max(prev - 25, 50));
  const handleResetZoom = () => {
    setZoomLevel(100);
    setRotation(0);
    setIsInverted(false);
  };
  const handleRotate = () => setRotation((prev) => (prev + 90) % 360);
  const handleToggleInvert = () => setIsInverted((prev) => !prev);

  const handleDownload = () => {
    if (downloadData?.download_url) {
      downloadFromPresignedUrl(downloadData.download_url, fileName || `prescription-${prescriptionId}`);
    }
  };

  return (
    <div className="flex flex-col h-full bg-slate-900 border-r border-slate-800 text-slate-100 select-none overflow-hidden">
      {/* Top Toolbar */}
      <div className="h-12 bg-slate-950/80 border-b border-slate-800 px-3 flex items-center justify-between shrink-0 gap-2">
        {/* Left: Viewport Controls */}
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={handleZoomOut}
            disabled={zoomLevel <= 50 || isPDF}
            title="Zoom out (-)"
            className="p-1.5 rounded hover:bg-slate-800 disabled:opacity-40 text-slate-300 transition-colors"
          >
            <ZoomOut className="w-4 h-4" />
          </button>

          <button
            type="button"
            onClick={handleResetZoom}
            disabled={isPDF}
            title="Reset zoom to 100%"
            className="px-2 py-1 text-xs font-mono rounded hover:bg-slate-800 text-slate-300 transition-colors"
          >
            {zoomLevel}%
          </button>

          <button
            type="button"
            onClick={handleZoomIn}
            disabled={zoomLevel >= 250 || isPDF}
            title="Zoom in (+)"
            className="p-1.5 rounded hover:bg-slate-800 disabled:opacity-40 text-slate-300 transition-colors"
          >
            <ZoomIn className="w-4 h-4" />
          </button>

          <div className="w-px h-4 bg-slate-800 mx-1" />

          <button
            type="button"
            onClick={handleRotate}
            disabled={isPDF}
            title="Rotate 90° clockwise"
            className="p-1.5 rounded hover:bg-slate-800 disabled:opacity-40 text-slate-300 transition-colors"
          >
            <RotateCw className="w-4 h-4" />
          </button>

          <button
            type="button"
            onClick={handleToggleInvert}
            disabled={isPDF}
            title="Toggle high-contrast monochrome inversion (for faint handwriting)"
            className={`p-1.5 rounded transition-colors ${
              isInverted
                ? 'bg-brand-500/20 text-brand-400 border border-brand-500/40'
                : 'hover:bg-slate-800 text-slate-300'
            }`}
          >
            <Sliders className="w-4 h-4" />
          </button>
        </div>

        {/* Right: TTL Expiration & Download */}
        <div className="flex items-center gap-2">
          {timeLeft > 0 ? (
            <span
              title="Presigned S3/Storage URL Time-to-Live countdown"
              className="text-[10px] font-mono text-slate-400 flex items-center gap-1 bg-slate-900 px-2 py-1 rounded border border-slate-800"
            >
              <Clock className="w-3 h-3 text-slate-500" />
              TTL: {Math.floor(timeLeft / 60)}:{(timeLeft % 60).toString().padStart(2, '0')}
            </span>
          ) : (
            <button
              type="button"
              onClick={() => refetch()}
              className="text-[10px] font-mono text-amber-400 bg-amber-950/40 border border-amber-800 px-2 py-0.5 rounded flex items-center gap-1 hover:bg-amber-900/60"
            >
              <RefreshCw className="w-3 h-3 animate-spin" /> Renew URL
            </button>
          )}

          <button
            type="button"
            onClick={handleDownload}
            disabled={!downloadData?.download_url}
            title="Download original file"
            className="p-1.5 rounded hover:bg-slate-800 text-slate-300 transition-colors"
          >
            <Download className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Main Document Viewport */}
      <div className="flex-1 overflow-auto p-4 flex items-center justify-center bg-slate-950 relative min-h-0">
        {isLoading ? (
          <div className="text-center space-y-2 text-xs font-mono text-slate-400">
            <RefreshCw className="w-6 h-6 animate-spin text-brand-500 mx-auto" />
            <div>Retrieving clinical document...</div>
          </div>
        ) : isError || !downloadData?.download_url ? (
          <div className="text-center p-6 space-y-3 max-w-sm">
            <AlertCircle className="w-8 h-8 text-rose-500 mx-auto" />
            <h4 className="text-xs font-bold text-slate-200 uppercase font-mono tracking-wider">
              Document Preview Unavailable
            </h4>
            <p className="text-xs text-slate-400">
              The storage key could not be retrieved or the temporary link has expired.
            </p>
            <button
              type="button"
              onClick={() => refetch()}
              className="px-3 py-1.5 text-xs font-medium bg-slate-800 hover:bg-slate-700 text-slate-100 rounded border border-slate-700"
            >
              Retry Retrieval
            </button>
          </div>
        ) : isPDF ? (
          /* Native PDF Document Embed */
          <div className="w-full h-full bg-slate-900 rounded border border-slate-800 overflow-hidden flex flex-col">
            <iframe
              src={downloadData.download_url}
              title="Prescription PDF Document"
              className="w-full h-full border-0"
            />
          </div>
        ) : (
          /* Interactive High-Res Image Viewer with Contrast & Rotation */
          <div
            className="relative transition-transform duration-150 ease-out flex items-center justify-center"
            style={{
              transform: `scale(${zoomLevel / 100}) rotate(${rotation}deg)`,
              filter: isInverted ? 'contrast(1.6) invert(1) brightness(1.1)' : 'none',
            }}
          >
            <img
              src={downloadData.download_url}
              alt="Handwritten prescription slip"
              className="max-w-full max-h-[calc(100vh-10rem)] object-contain rounded border border-slate-800 shadow-xl"
            />
          </div>
        )}
      </div>

      {/* Footer Info Strip */}
      <div className="h-7 bg-slate-950 border-t border-slate-800/80 px-3 flex items-center justify-between text-[10px] font-mono text-slate-500 shrink-0">
        <div className="truncate max-w-xs">{fileName || `file-key: ${prescriptionId}`}</div>
        <div className="flex items-center gap-2">
          {isInverted && <span className="text-brand-400">High-Contrast Active</span>}
          <span>{isPDF ? 'PDF Document' : 'Raster Image'}</span>
        </div>
      </div>
    </div>
  );
}
