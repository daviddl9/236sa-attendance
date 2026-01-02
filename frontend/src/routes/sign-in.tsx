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
import { Info, Eye, EyeOff } from 'lucide-react';

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
  const { refetch } = useAuth();
  const search = useSearch({ from: '/sign-in' }) as { redirect?: string; qrToken?: string };

  const handleSignIn = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!identifier || !password) {
      toast.error('Please enter your identifier and password');
      return;
    }

    if (identifier.toLowerCase() !== 'admin' && password.length !== 10) {
      toast.error('Password must be 10 characters (NRIC last 4 + DOB in DDMMYY format)');
      return;
    }

    try {
      setLoading(true);
      const data = await apiClient.signIn({ identifier, password });

      // Check if profile is complete (has rank and battery)
      const profileComplete = data.user.rank && data.user.battery;

      // IMPORTANT: Store qrToken BEFORE refetch() to avoid race condition
      // refetch() updates isAuthenticated which triggers Navigate before this code completes
      console.log('[SignIn] search.qrToken:', search.qrToken, 'profileComplete:', profileComplete);
      if (search.qrToken && !profileComplete) {
        console.log('[SignIn] Storing pendingQrToken in sessionStorage');
        sessionStorage.setItem('pendingQrToken', search.qrToken);
      }

      await refetch(); // Refresh auth context to get updated user data
      toast.success('Signed in successfully');

      // If there's a QR token, mark attendance (after profile completion if needed)
      if (search.qrToken) {
        // If profile is not complete, redirect to dashboard where modal will show
        if (!profileComplete) {
          window.location.href = '/dashboard';
          return;
        }
        
        // Profile is complete, mark attendance immediately
        try {
          const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';
          const response = await fetch(`${apiUrl}/api/qr/${search.qrToken}`, {
            method: 'GET',
            credentials: 'include',
            redirect: 'manual',
          });

          // Handle opaque redirects (CORS) - backend returned a redirect
          if (response.type === 'opaqueredirect') {
            const sessionId = search.qrToken.split(':')[0];
            window.location.href = `/attendance/marked?session=${sessionId}`;
            return;
          }

          // Handle transparent redirects (same-origin)
          if (response.status >= 300 && response.status < 400) {
            const location = response.headers.get('Location');
            if (location) {
              try {
                const url = new URL(location);
                const path = url.pathname + url.search;
                window.location.href = path;
                return;
              } catch {
                window.location.href = location;
                return;
              }
            }
          }

          // If successful, redirect to marked page
          if (response.ok) {
            const sessionId = search.qrToken.split(':')[0];
            window.location.href = `/attendance/marked?session=${sessionId}`;
            return;
          }
        } catch (qrError) {
          console.error('Failed to mark attendance:', qrError);
          toast.error('Signed in but failed to mark attendance. Please scan the QR code again.');
        }
      }
      
      // Redirect to the redirect URL if present, otherwise go to dashboard
      if (search.redirect) {
        window.location.href = search.redirect;
      } else {
        window.location.href = '/dashboard';
      }
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
            236SA Attendance System
          </CardTitle>
          <CardDescription className="text-xs md:text-sm">
            Sign in to your account
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSignIn} className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="identifier">
                Full Name (as in NRIC)
              </Label>
              <Input
                id="identifier"
                type="text"
                placeholder="Enter your full name as in NRIC"
                value={identifier}
                onChange={(e) => setIdentifier(e.target.value)}
                disabled={loading}
                required
              />
            </div>
            <div className="grid gap-2">
              <div className="flex items-center gap-2">
                <Label htmlFor="password">Password</Label>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      className="inline-flex items-center justify-center"
                    >
                      <Info className="h-4 w-4 text-muted-foreground" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-xs">
                    <p className="text-xs">
                      Last 5 characters of NRIC.
                      <br />
                      Example: <strong>4567A</strong>
                    </p>
                  </TooltipContent>
                </Tooltip>
              </div>
              <div className="relative">
                <Input
                  id="password"
                  type={showPassword ? "text" : "password"}
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
                  {showPassword ? (
                    <EyeOff className="h-4 w-4" />
                  ) : (
                    <Eye className="h-4 w-4" />
                  )}
                </button>
              </div>
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? 'Signing in...' : 'Sign In'}
            </Button>
          </form>
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

