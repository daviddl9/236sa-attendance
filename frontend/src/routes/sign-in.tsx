import { createFileRoute, Navigate, useSearch } from '@tanstack/react-router';
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '../components/ui/tooltip';
import { toast } from 'sonner';
import { useAuth } from '../lib/auth-context';
import { apiClient } from '../lib/api-client';
import { isValidNricLast5, normalizeNricLast5, NRIC_LAST5_FORMAT_MESSAGE } from '../lib/nric-password';
import { NoPasteInput } from '../components/no-paste-input';
import { Info, Eye, EyeOff, UserPlus, Clock } from 'lucide-react';

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
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  // Signup state — shown when sign-in returns signup_required
  const [signupRequired, setSignupRequired] = useState(false);
  const [signupName, setSignupName] = useState('');
  const [confirmNric, setConfirmNric] = useState('');
  const [nricMismatch, setNricMismatch] = useState(false);
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

    const isAdmin = identifier.toLowerCase() === 'admin';
    if (!isAdmin && !isValidNricLast5(password)) {
      toast.error(NRIC_LAST5_FORMAT_MESSAGE);
      return;
    }

    try {
      setLoading(true);
      const data = await apiClient.signIn({
        identifier,
        password: isAdmin ? password : normalizeNricLast5(password),
      });

      if (data.outcome === 'signup_required') {
        setSignupRequired(true);
        setSignupName(data.fullName);
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

  const handleSignUp = async (e: React.FormEvent) => {
    e.preventDefault();
    setNricMismatch(false);

    const nric = normalizeNricLast5(password);
    const confirm = normalizeNricLast5(confirmNric);

    if (!isValidNricLast5(password)) {
      toast.error(NRIC_LAST5_FORMAT_MESSAGE);
      return;
    }
    if (!isValidNricLast5(confirmNric)) {
      toast.error('Confirmation: ' + NRIC_LAST5_FORMAT_MESSAGE);
      return;
    }
    if (nric !== confirm) {
      setNricMismatch(true);
      return;
    }

    try {
      setLoading(true);
      const data = await apiClient.signUp({
        fullName: signupName,
        nricLast5: nric,
        confirmNricLast5: confirm,
      });
      if (data.outcome === 'pending_approval') {
        setPendingApproval(true);
        setSignupRequired(false);
        setLoading(false);
        return;
      }
      if (data.outcome === 'authenticated') {
        await finishAuth(data.user.rank, data.user.battery);
      }
    } catch (error) {
      setLoading(false);
      const errorMessage = error instanceof Error ? error.message : 'Failed to create account';
      toast.error(errorMessage);
    }
  };

  return (
    <div className="flex flex-col justify-center items-center w-full h-screen">
      <Card className="max-w-md w-full">
        <CardHeader>
          <CardTitle className="text-lg md:text-xl">
            236SA Attendance System
          </CardTitle>
          <CardDescription className="text-xs md:text-sm">
            {pendingApproval
              ? 'Registration submitted'
              : signupRequired
              ? `Create your account for ${signupName}`
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
                <Label htmlFor="identifier">Full Name (as in NRIC)</Label>
                <Input
                  id="identifier"
                  type="text"
                  placeholder="Enter your full name as in NRIC"
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value.toUpperCase())}
                  disabled={loading}
                  required
                />
              </div>
              <div className="grid gap-2">
                <div className="flex items-center gap-2">
                  <Label htmlFor="password">NRIC Last 5 (e.g 1234A)</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button type="button" className="inline-flex items-center justify-center">
                        <Info className="h-4 w-4 text-muted-foreground" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs">
                      <p className="text-xs">
                        Last 5 characters of NRIC: 4 numbers and the final alphabet letter.
                        <br />
                        Example: <strong>4567A</strong>
                      </p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    placeholder="e.g. 1234A"
                    value={password}
                    onChange={(e) => setPassword(e.target.value.toUpperCase())}
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
            </form>
          ) : (
            <form onSubmit={handleSignUp} className="grid gap-4">
              <div className="rounded-md border border-amber-200 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/20">
                <div className="flex items-center gap-2 text-sm text-amber-800 dark:text-amber-300">
                  <UserPlus className="h-4 w-4 shrink-0" />
                  <span>
                    <strong>{signupName}</strong> is not in the system yet. Complete the fields below to create your account.
                  </span>
                </div>
              </div>

              <div className="grid gap-2">
                <div className="flex items-center gap-2">
                  <Label htmlFor="nric-entry">NRIC Last 5</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button type="button" className="inline-flex items-center justify-center">
                        <Info className="h-4 w-4 text-muted-foreground" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs">
                      <p className="text-xs">
                        4 digits followed by 1 letter. Example: <strong>4567A</strong>
                      </p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  id="nric-entry"
                  type="text"
                  placeholder="e.g. 1234A"
                  value={password}
                  onChange={(e) => { setPassword(e.target.value); setNricMismatch(false); }}
                  disabled={loading}
                  maxLength={5}
                  required
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="nric-confirm">Confirm NRIC Last 5</Label>
                <p className="text-xs text-muted-foreground -mt-1">
                  Type it again — paste and autofill are disabled here.
                </p>
                <NoPasteInput
                  id="nric-confirm"
                  type="text"
                  placeholder="Type it again"
                  value={confirmNric}
                  onChange={(e) => { setConfirmNric(e.target.value); setNricMismatch(false); }}
                  disabled={loading}
                  maxLength={5}
                  required
                />
                {nricMismatch && (
                  <p className="text-sm text-destructive">NRIC Last 5 values do not match.</p>
                )}
              </div>

              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  className="flex-1"
                  onClick={() => { setSignupRequired(false); setConfirmNric(''); setNricMismatch(false); }}
                  disabled={loading}
                >
                  Back
                </Button>
                <Button type="submit" className="flex-1" disabled={loading}>
                  {loading ? 'Creating account...' : 'Create account'}
                </Button>
              </div>
            </form>
          )}
        </CardContent>
      </Card>
      <p className="mt-6 text-xs text-center text-gray-500 dark:text-gray-400 max-w-md">
        By signing in, you agree to our Terms of Service and Privacy Policy
      </p>
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
