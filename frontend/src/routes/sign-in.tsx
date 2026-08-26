import { createFileRoute, Link, Navigate, useSearch } from '@tanstack/react-router';
import { useState } from 'react';
import { Button } from '../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';
import { toast } from 'sonner';
import { completeScan } from '../lib/qr-scan';
import { useAuth } from '../lib/auth-context';
import { apiClient } from '../lib/api-client';
import { PublicFooter } from '../components/public-footer';
import { Clock } from 'lucide-react';
import { DobFields } from '../components/dob-fields';
import { isValidDob, joinDob } from '../lib/dob-format';

export const Route = createFileRoute('/sign-in')({
  component: SignInComponent,
  validateSearch: (search: Record<string, unknown>) => {
    return {
      redirect: (search.redirect as string) || undefined,
      qrToken: (search.qrToken as string) || undefined,
    };
  },
});

function SignInContent() {
  const [loading, setLoading] = useState(false);
  const [identifier, setIdentifier] = useState('');
  const [dob, setDob] = useState({ day: '', month: '', year: '' });

  const [pendingApproval, setPendingApproval] = useState(false);

  const { refetch } = useAuth();
  const search = useSearch({ from: '/sign-in' }) as { redirect?: string; qrToken?: string };

  const finishAuth = async (rank: string | null | undefined, battery: string | null | undefined) => {
    const profileComplete = rank && battery;

    if (search.qrToken && !profileComplete) {
      sessionStorage.setItem('pendingQrToken', search.qrToken);
    }

    await refetch();
    toast.success('Signed in successfully');

    if (search.qrToken) {
      if (!profileComplete) {
        window.location.href = '/dashboard';
        return;
      }
      // Finish the scan that sent this person to sign in. The shared resolver
      // knows an Out code belongs on the Out screen; assuming attendance here
      // sent them to a session page that cannot load an Out session.
      try {
        const outcome = await completeScan(search.qrToken);
        if (outcome.kind !== 'error') {
          window.location.href = outcome.path;
          return;
        }
        toast.error('Signed in, but that QR code could not be used. Please scan again.');
      } catch {
        toast.error('Signed in but failed to record the scan. Please scan the QR code again.');
      }
    }

    window.location.href = search.redirect ?? '/dashboard';
  };

  const handleSignIn = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!identifier || !dob.day || !dob.month || !dob.year) {
      toast.error('Please enter your full name and date of birth');
      return;
    }
    if (!isValidDob(joinDob(dob.day, dob.month, dob.year))) {
      toast.error('Please enter a valid date of birth');
      return;
    }

    try {
      setLoading(true);
      const data = await apiClient.signIn({ identifier, dob: joinDob(dob.day, dob.month, dob.year) });

      if (data.outcome === 'pending_approval') {
        setPendingApproval(true);
        setLoading(false);
        return;
      }

      // outcome === 'authenticated'
      await finishAuth(data.user.rank, data.user.battery);
    } catch (error) {
      setLoading(false);
      const errorMessage = error instanceof Error ? error.message : 'Invalid identifier or password';
      toast.error(errorMessage);
    }
  };

  return (
    <div className="flex flex-col justify-center items-center w-full h-screen">
      <Card className="max-w-md w-full">
        <CardHeader>
          <CardTitle className="text-lg md:text-xl">
            236 Attendance
          </CardTitle>
          <CardDescription className="text-xs md:text-sm">
            {pendingApproval ? 'Registration submitted' : 'Sign in to your account'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {pendingApproval ? (
            <div className="grid gap-4">
              <div className="rounded-md border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/20">
                <div className="flex items-start gap-3 text-sm text-amber-800 dark:text-amber-300">
                  <Clock className="h-4 w-4 mt-0.5 shrink-0" />
                  <div>
                    <p className="font-medium mb-1">Pending admin approval</p>
                    <p>Your registration has been submitted. An administrator will review and approve your account before you can sign in.</p>
                  </div>
                </div>
              </div>
              <Button
                type="button"
                variant="outline"
                className="w-full"
                onClick={() => setPendingApproval(false)}
              >
                Back to sign in
              </Button>
            </div>
          ) : (
            <form onSubmit={handleSignIn} className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="identifier">Full name</Label>
                <Input
                  id="identifier"
                  type="text"
                  placeholder="Your full name"
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value)}
                  disabled={loading}
                  required
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="dob">Date of birth</Label>
                <DobFields id="dob" value={dob} onChange={setDob} disabled={loading} />
              </div>
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? 'Signing in...' : 'Sign In'}
              </Button>
              <p className="text-center text-sm text-muted-foreground">
                Need an account?{' '}
                <Link to="/sign-up" search={{ qrToken: search.qrToken }} className="underline hover:text-foreground">
                  Sign up
                </Link>
              </p>
            </form>
          )}
        </CardContent>
      </Card>
      <PublicFooter showAgreement />
    </div>
  );
}

function SignInComponent() {
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="flex flex-col justify-center items-center w-full h-screen">
        <div className="max-w-md w-full bg-gray-200 dark:bg-gray-800 animate-pulse rounded-lg h-96"></div>
      </div>
    );
  }

  if (isAuthenticated) {
    return <Navigate to="/dashboard" />;
  }

  return <SignInContent />;
}
