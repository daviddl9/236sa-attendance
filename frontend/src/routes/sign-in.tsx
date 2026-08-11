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
import { useAuth } from '../lib/auth-context';
import { APIError, apiClient } from '../lib/api-client';
import { normalizeLegacyPassword } from '../lib/legacy-password';
import { PublicFooter } from '../components/public-footer';
import { Eye, EyeOff, Clock } from 'lucide-react';

export const Route = createFileRoute('/sign-in')({
  component: SignInComponent,
  validateSearch: (search: Record<string, unknown>) => {
    return {
      redirect: (search.redirect as string) || undefined,
      qrToken: (search.qrToken as string) || undefined,
    };
  },
});

async function signInWithLegacyFallback(identifier: string, password: string) {
  try {
    return await apiClient.signIn({ identifier, password });
  } catch (error) {
    const isLegacyIdentifier = identifier.trim().includes(' ');
    if (!(error instanceof APIError) || error.status !== 401 || !isLegacyIdentifier) {
      throw error;
    }
    return apiClient.signIn({
      identifier,
      password: normalizeLegacyPassword(password),
    });
  }
}

function SignInContent() {
  const [loading, setLoading] = useState(false);
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  // Account-not-found state — shown when sign-in returns signup_required
  const [signupRequired, setSignupRequired] = useState(false);
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
      try {
        const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';
        const response = await fetch(`${apiUrl}/api/qr/${search.qrToken}`, {
          method: 'GET',
          credentials: 'include',
          redirect: 'manual',
        });
        if (response.type === 'opaqueredirect') {
          const sessionId = search.qrToken.split(':')[0];
          window.location.href = `/attendance/marked?session=${sessionId}`;
          return;
        }
        if (response.status >= 300 && response.status < 400) {
          const location = response.headers.get('Location');
          if (location) {
            try {
              const url = new URL(location);
              window.location.href = url.pathname + url.search;
            } catch {
              window.location.href = location;
            }
            return;
          }
        }
        if (response.ok) {
          const sessionId = search.qrToken.split(':')[0];
          window.location.href = `/attendance/marked?session=${sessionId}`;
          return;
        }
      } catch {
        toast.error('Signed in but failed to mark attendance. Please scan the QR code again.');
      }
    }

    window.location.href = search.redirect ?? '/dashboard';
  };

  const handleSignIn = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!identifier || !password) {
      toast.error('Please enter your identifier and password');
      return;
    }

    try {
      setLoading(true);
      const data = await signInWithLegacyFallback(identifier, password);

      if (data.outcome === 'signup_required') {
        setSignupRequired(true);
        setLoading(false);
        return;
      }

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
            {pendingApproval
              ? 'Registration submitted'
              : signupRequired
              ? 'No account found'
              : 'Sign in to your account'}
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
                onClick={() => { setPendingApproval(false); setSignupRequired(false); }}
              >
                Back to sign in
              </Button>
            </div>
          ) : !signupRequired ? (
            <form onSubmit={handleSignIn} className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="identifier">Username</Label>
                <Input
                  id="identifier"
                  type="text"
                  placeholder="Enter your username"
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value.toUpperCase())}
                  disabled={loading}
                  required
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password">Password</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    placeholder="Enter your password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    disabled={loading}
                    required
                    className="pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                    disabled={loading}
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
              </div>
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? 'Signing in...' : 'Sign In'}
              </Button>
              <p className="text-center text-sm text-muted-foreground">
                Need an account?{' '}
                <Link to="/sign-up" className="underline hover:text-foreground">
                  Sign up
                </Link>
              </p>
            </form>
          ) : (
            <div className="grid gap-4">
              <div className="rounded-md border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/20">
                <p className="text-sm text-amber-800 dark:text-amber-300">
                  No account found. Contact your commander to be added.
                </p>
              </div>
              <Link
                to="/sign-up"
                className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
              >
                Create an account
              </Link>
              <Button
                type="button"
                variant="outline"
                className="w-full"
                onClick={() => setSignupRequired(false)}
              >
                Back to sign in
              </Button>
            </div>
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
