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
import { PublicFooter } from '../components/public-footer';

export const Route = createFileRoute('/')({
  component: IndexComponent,
});

function SignInContent() {
  const [loading, setLoading] = useState(false);
  const [identifier, setIdentifier] = useState('');
  const [dob, setDob] = useState('');
  const { refetch } = useAuth();

  const handleSignIn = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!identifier || !dob) {
      toast.error('Please enter your full name and date of birth');
      return;
    }
    try {
      setLoading(true);
      const response = await apiClient.signIn({ identifier, dob });
      if (response.outcome === 'pending_approval') {
        setLoading(false);
        toast.info('Your registration is awaiting approval');
        return;
      }
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
                Full name
              </Label>
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
              <Input
                id="dob"
                type="date"
                value={dob}
                onChange={(e) => setDob(e.target.value)}
                disabled={loading}
                required
              />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? 'Signing in...' : 'Sign In'}
            </Button>
            <p className="text-center text-sm text-muted-foreground">
              Need an account?{' '}
              <a href="/sign-up" className="underline hover:text-foreground">
                Sign up
              </a>
            </p>
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
