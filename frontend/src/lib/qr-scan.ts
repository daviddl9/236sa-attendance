import { API_URL, apiClient } from './api-client';

/**
 * Where a scanned QR code should take the person.
 *
 * Three screens can receive a scan — the /qr/ landing route, the sign-in page
 * finishing a scan that required logging in, and the in-app camera scanner —
 * and each previously decided this for itself. They disagreed: two assumed
 * every scan was ordinary attendance, so an Out code sent the scanner to a
 * session page that cannot load, and the camera scanner posted Out codes to
 * the attendance endpoint, which refuses them.
 */
export type ScanOutcome =
  | { kind: 'out'; sessionId: string; path: string }
  | { kind: 'attendance'; sessionId: string; path: string }
  | { kind: 'signIn'; path: string }
  | { kind: 'error'; status: number };

/**
 * Runs a scan to completion and reports where to go next.
 *
 * The backend marks ordinary attendance and, for an Out session, hands the
 * scanned secret over in a short-lived cookie. Its redirect target cannot be
 * read from JavaScript — a fetch with redirect:"manual" yields an opaque
 * response — so the session type is established separately through /me, which
 * answers for any authenticated member rather than commanders only.
 */
export async function completeScan(token: string): Promise<ScanOutcome> {
  const sessionId = token.split(':')[0];

  const response = await fetch(`${API_URL}/api/qr/${token}`, {
    method: 'GET',
    credentials: 'include',
    redirect: 'manual',
  });

  const redirected =
    response.type === 'opaqueredirect' ||
    (response.status >= 300 && response.status < 400) ||
    response.ok;

  if (!redirected) {
    if (response.status === 401) {
      return { kind: 'signIn', path: `/sign-in?redirect=/qr/${token}&qrToken=${token}` };
    }
    return { kind: 'error', status: response.status };
  }

  try {
    await apiClient.getOutSelfState(sessionId);
    return { kind: 'out', sessionId, path: `/out/${sessionId}` };
  } catch {
    return {
      kind: 'attendance',
      sessionId,
      path: `/dashboard/sessions/${sessionId}?scanned=true`,
    };
  }
}

/**
 * Extracts the session:secret pair from a scanned URL or raw string. Codes may
 * carry a trailing timestamp segment; /api/qr/ wants the first two parts only.
 */
export function tokenFromScan(raw: string): string | null {
  const text = raw.trim();
  const token = text.includes('/qr/') ? text.split('/qr/')[1] : text;
  const parts = token.split(/[?#]/)[0].split(':');
  if (parts.length < 2 || !parts[0] || !parts[1]) return null;
  return `${parts[0]}:${parts[1]}`;
}
