import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiClient, API_URL } from '../../lib/api-client';
import { useAuth } from '../../lib/auth-context';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '../../components/ui/dialog';
import { Button } from '../../components/ui/button';
import { CheckCircle2, XCircle } from 'lucide-react';

export const Route = createFileRoute('/qr/$token')({
  component: QRScanPage,
});

function QRScanPage() {
  const { token } = Route.useParams();
  const navigate = useNavigate();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const [status, setStatus] = useState<'loading' | 'success' | 'error' | null>(null);
  const sessionId = token.split(':')[0];

  // Try to fetch session details for display, but silently fail if user doesn't have permission
  const { data: session } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => apiClient.getSessionById(sessionId),
    enabled: status === 'success' && !!sessionId,
    retry: false,
  });

  useEffect(() => {
    const handleQRScan = async () => {
      // If already processed, don't process again
      if (status !== null) {
        return;
      }

      // Wait for auth to load
      if (authLoading) {
        return;
      }

      // If not authenticated, redirect to sign-in with QR token params
      if (!isAuthenticated) {
        window.location.href = `/sign-in?redirect=/qr/${token}&qrToken=${token}`;
        return;
      }

      // User is authenticated - proceed with QR scan
      setStatus('loading');

      // Call backend API - backend will handle authentication and redirects
      try {
        const response = await fetch(`${API_URL}/api/qr/${token}`, {
          method: 'GET',
          credentials: 'include',
          redirect: 'manual',
        });

        // Handle opaque redirects (CORS) - backend returned a redirect
        if (response.type === 'opaqueredirect') {
          // Attendance marked (or already marked) - land on the session page
          // with the confirmation modal, matching the in-app camera scan flow.
          window.location.href = `/dashboard/sessions/${sessionId}?scanned=true`;
          return;
        }

        // Handle transparent redirects (same-origin)
        if (response.status >= 300 && response.status < 400) {
          const location = response.headers.get('Location');
          if (location) {
            // Extract the path from the full URL (backend returns full URL)
            try {
              const url = new URL(location);
              const path = url.pathname + url.search;
              window.location.href = path;
              return;
            } catch {
              // If Location is a relative URL, use it directly
              window.location.href = location;
              return;
            }
          } else {
            // If Location header is not accessible (CORS), fallback to sign-in
            window.location.href = `/sign-in?redirect=/qr/${token}&qrToken=${token}`;
            return;
          }
        }

        // If successful (shouldn't happen, backend always redirects)
        if (response.ok) {
          window.location.href = `/dashboard/sessions/${sessionId}?scanned=true`;
        } else {
          // For non-redirect errors, try to get error text
          const errorText = await response.text().catch(() => '');
          // If it's a 401/403, redirect to sign-in
          if (response.status === 401 || response.status === 403) {
            window.location.href = `/sign-in?redirect=/qr/${token}&qrToken=${token}`;
            return;
          }
          throw new Error(errorText || 'Failed to mark attendance');
        }
      } catch (error) {
        console.error('QR scan error:', error);
        // On any error, redirect to sign-in as fallback
        window.location.href = `/sign-in?redirect=/qr/${token}&qrToken=${token}`;
      }
    };

    handleQRScan();
  }, [token, status, sessionId, isAuthenticated, authLoading]);

  const handleClose = () => {
    if (status === 'success') {
      navigate({ to: '/dashboard' });
    } else {
      navigate({ to: '/dashboard/attendance/scan' });
    }
  };

  return (
    <>
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-muted-foreground">
          {status === 'loading' && 'Processing QR code...'}
          {status === null && 'Loading...'}
        </div>
      </div>

      <Dialog open={status === 'success' || status === 'error'} onOpenChange={handleClose}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            {status === 'success' ? (
              <>
                <div className="flex justify-center mb-4">
                  <div className="rounded-full bg-green-100 dark:bg-green-900 p-3">
                    <CheckCircle2 className="h-8 w-8 text-green-600 dark:text-green-400" />
                  </div>
                </div>
                <DialogTitle className="text-center text-xl">Attendance Marked!</DialogTitle>
                <DialogDescription className="text-center">
                  Your attendance has been successfully recorded
                  {session && (
                    <>
                      <br />
                      <span className="font-semibold text-foreground">{session.name}</span>
                    </>
                  )}
                </DialogDescription>
              </>
            ) : (
              <>
                <div className="flex justify-center mb-4">
                  <div className="rounded-full bg-red-100 dark:bg-red-900 p-3">
                    <XCircle className="h-8 w-8 text-red-600 dark:text-red-400" />
                  </div>
                </div>
                <DialogTitle className="text-center text-xl">Error</DialogTitle>
                <DialogDescription className="text-center">
                  Failed to mark attendance. Please try again.
                </DialogDescription>
              </>
            )}
          </DialogHeader>
          <div className="flex justify-end pt-4">
            <Button onClick={handleClose}>
              {status === 'success' ? 'Go to Dashboard' : 'Close'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

