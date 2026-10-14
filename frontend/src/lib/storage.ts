/**
 * Storage helpers for presigned S3 and local HMAC uploads/downloads.
 */

export interface PresignedUploadResponse {
  upload_url: string;
  key: string;
  expires_in?: number;
}

export interface PresignedDownloadResponse {
  download_url: string;
  expires_in: number;
}

/**
 * Uploads a raw File/Blob to a presigned URL using HTTP PUT.
 * Works seamlessly with both AWS S3 presigned URLs and local HMAC endpoints (/storage/upload).
 */
export async function uploadFileToPresignedUrl(
  uploadUrl: string,
  file: File | Blob
): Promise<void> {
  const response = await fetch(uploadUrl, {
    method: 'PUT',
    body: file,
    headers: {
      'Content-Type': file.type || 'application/octet-stream',
    },
  });

  if (!response.ok) {
    let errorDetail = `Upload failed with HTTP ${response.status}`;
    try {
      const errText = await response.text();
      if (errText) {
        errorDetail = errText;
      }
    } catch {
      // Ignore
    }
    throw new Error(errorDetail);
  }
}

/**
 * Triggers an immediate browser download for a presigned download URL.
 * Presigned URLs have a short TTL (e.g., 300s), so we immediately open/save
 * rather than persisting them in state across idle sessions.
 */
export function downloadFromPresignedUrl(downloadUrl: string, filename?: string): void {
  const link = document.createElement('a');
  link.href = downloadUrl;
  if (filename) {
    link.download = filename;
  }
  link.target = '_blank';
  link.rel = 'noopener noreferrer';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}
