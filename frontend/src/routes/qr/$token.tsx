import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../../lib/api-client';
import { completeScan } from '../../lib/qr-scan';
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

// Map backend error statuses to a message a soldier can act on. The backend
// returns 404 when the session was deleted, 400 when it is no longer active,
// and 403 when the user falls outside its scope.
function errorMessageForStatus(status: number): string {
  switch (status) {
    case 404:
      return 'This session no longer exists. It may have been deleted.';
    case 403:
      return "You're outside this session's scope.";
    case 400:
      return 'This session is not active.';
    default:
      return 'Failed to mark attendance. Please try again.';
  }
}

function QRScanPage() {
  const { token } = Route.useParams();
  const navigate = useNavigate();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const [status, setStatus] = useState<'loading' | 'success' | 'error' | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [errorTarget, setErrorTarget] = useState<'sessions' | 'scan'>('scan');
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

      // One shared resolver decides where a scan lands, so this page, the
      // sign-in hand-off and the camera scanner cannot disagree about it.
      try {
        const outcome = await completeScan(token);
        if (outcome.kind === 'error') {
          setErrorMessage(errorMessageForStatus(outcome.status));
          setErrorTarget(outcome.status === 404 ? 'sessions' : 'scan');
          setStatus('error');
          return;
        }
        window.location.href = outcome.path;
        return;
      } catch (error) {
        console.error('QR scan error:', error);
        setErrorMessage('Failed to mark attendance. Please try again.');
        setErrorTarget('scan');
        setStatus('error');
      }
    };

    handleQRScan();
  }, [token, status, sessionId, isAuthenticated, authLoading]);

  const handleClose = () => {
    if (status === 'success') {
      navigate({ to: '/dashboard' });
    } else if (errorTarget === 'sessions') {
      navigate({ to: '/dashboard/sessions' });
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
                  {errorMessage ?? 'Failed to mark attendance. Please try again.'}
                </DialogDescription>
              </>
            )}
          </DialogHeader>
          <div className="flex justify-end pt-4">
            <Button onClick={handleClose}>
              {status === 'success'
                ? 'Go to Dashboard'
                : errorTarget === 'sessions'
                  ? 'Go to Sessions'
                  : 'Close'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

