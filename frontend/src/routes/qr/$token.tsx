import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../../lib/api-client';
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
  const [status, setStatus] = useState<'loading' | 'success' | 'error' | null>(null);
  const [errorMessage, setErrorMessage] = useState<string>('');
  const sessionId = token.split(':')[0];

  const { data: session } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => apiClient.getSessionById(sessionId),
    enabled: status === 'success' && !!sessionId,
  });

  useEffect(() => {
    const handleQRScan = async () => {
      // If already processed, don't process again
      if (status !== null) {
        return;
      }

      setStatus('loading');

      // Call backend API - backend will handle authentication and redirects
      try {
        const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';
        const response = await fetch(`${apiUrl}/api/qr/${token}`, {
          method: 'GET',
          credentials: 'include',
          redirect: 'manual',
        });

        // Handle redirects manually
        if (response.status >= 300 && response.status < 400) {
          const location = response.headers.get('Location');
          if (location) {
            // Extract the path from the full URL (backend returns full URL)
            const url = new URL(location);
            const path = url.pathname + url.search;
            // Follow redirect (could be to registration, sign-in, or success)
            window.location.href = path;
            return;
          }
        }

        // If successful (shouldn't happen, backend always redirects)
        if (response.ok) {
          setStatus('success');
        } else {
          const errorText = await response.text();
          throw new Error(errorText || 'Failed to mark attendance');
        }
      } catch (error) {
        console.error('QR scan error:', error);
        setStatus('error');
        setErrorMessage(error instanceof Error ? error.message : 'Failed to mark attendance');
      }
    };

    handleQRScan();
  }, [token, status]);

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
                  {errorMessage || 'Failed to mark attendance'}
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

