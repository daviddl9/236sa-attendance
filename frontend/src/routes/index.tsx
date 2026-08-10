import { createFileRoute, Navigate } from '@tanstack/react-router';
import { useState } from 'react';
import { useAuth } from '../lib/auth-context';
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
import { apiClient } from '../lib/api-client';
import { normalizeLegacyPassword } from '../lib/legacy-password';
import { PublicFooter } from '../components/public-footer';
import { Eye, EyeOff } from 'lucide-react';

export const Route = createFileRoute('/')({
  component: IndexComponent,
});

function SignInContent() {
  const [loading, setLoading] = useState(false);
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const { refetch } = useAuth();

  const handleSignIn = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!identifier || !password) {
      toast.error('Please enter your identifier and password');
      return;
    }
    const isAdmin = identifier.toLowerCase() === 'admin';

    try {
      setLoading(true);
      await apiClient.signIn({
        identifier,
        password: isAdmin ? password : normalizeLegacyPassword(password),
      });
      await refetch(); // Refresh auth context to get updated user data
      toast.success('Signed in successfully');
      window.location.href = '/dashboard/attendance/scan';
    } catch (error) {
      setLoading(false);
      const errorMessage = error instanceof Error ? error.message : 'Invalid identifier or password';
      toast.error(errorMessage);
    }
  };

  return (
    <div className="flex flex-col justify-center items-center w-full h-screen px-4">
      <Card className="max-w-md w-full">
        <CardHeader>
          <CardTitle className="text-lg md:text-xl">
            236 Attendance
          </CardTitle>
          <CardDescription className="text-xs md:text-sm">
            Sign in to your account
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSignIn} className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="identifier">
                Full Name
              </Label>
              <Input
                id="identifier"
                type="text"
                placeholder="Enter your full name"
                value={identifier}
                onChange={(e) => setIdentifier(e.target.value)}
                disabled={loading}
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
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
      <PublicFooter showAgreement />
    </div>
  );
}

function IndexComponent() {
  const { user, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="flex flex-col justify-center items-center w-full h-screen bg-background px-4">
        <div className="max-w-md w-full bg-gray-200 dark:bg-gray-800 animate-pulse rounded-lg h-96"></div>
      </div>
    );
  }

  if (user) {
    return <Navigate to="/dashboard/attendance/scan" />;
  }

  return <SignInContent />;
}
