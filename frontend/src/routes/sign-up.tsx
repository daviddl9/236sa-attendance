import { createFileRoute, Link, Navigate } from '@tanstack/react-router';
import { useState } from 'react';
import { Clock, Eye, EyeOff } from 'lucide-react';
import { toast } from 'sonner';

import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';
import { PublicFooter } from '../components/public-footer';
import { useAuth } from '../lib/auth-context';
import { apiClient } from '../lib/api-client';
import {
  type SignUpFormValues,
  VALID_BATTERIES,
  VALID_RANKS,
  validateSignUp,
} from '../lib/signup-validation';

export const Route = createFileRoute('/sign-up')({
  component: SignUpPage,
});

function SignUpPage() {
  const { isAuthenticated, isLoading } = useAuth();
  const [submitted, setSubmitted] = useState(false);
  const [loading, setLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmation, setShowConfirmation] = useState(false);
  const [form, setForm] = useState<SignUpFormValues>({
    username: '',
    password: '',
    confirmPassword: '',
    fullName: '',
    rank: '',
    battery: '',
  });

  if (isLoading) return null;
  if (isAuthenticated) return <Navigate to="/dashboard" />;

  const update = (field: keyof typeof form, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const validationError = validateSignUp(form);
    if (validationError) {
      toast.error(validationError);
      return;
    }

    try {
      setLoading(true);
      const response = await apiClient.signUp({
        ...form,
        username: form.username.trim(),
        fullName: form.fullName.trim(),
        rank: form.rank.trim(),
        battery: form.battery.trim(),
      });
      if (response.outcome === 'pending_approval') {
        setSubmitted(true);
        return;
      }
      toast.error('Unexpected signup response');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to create account');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4 py-8">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>236 Attendance</CardTitle>
          <CardDescription>
            {submitted ? 'Registration submitted' : 'Create your account'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {submitted ? (
            <div className="grid gap-4">
              <div className="rounded-md border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/20">
                <div className="flex items-start gap-3 text-sm text-amber-800 dark:text-amber-300">
                  <Clock className="mt-0.5 h-4 w-4 shrink-0" />
                  <div>
                    <p className="mb-1 font-medium">Pending admin approval</p>
                    <p>Your registration has been submitted. An administrator will review and approve your account before you can sign in.</p>
                  </div>
                </div>
              </div>
              <Link to="/sign-in" search={{ redirect: undefined, qrToken: undefined }} className="text-center text-sm underline">
                Back to sign in
              </Link>
            </div>
          ) : (
            <form onSubmit={handleSubmit} noValidate className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="username">Username</Label>
                <Input id="username" value={form.username} onChange={(e) => update('username', e.target.value)} disabled={loading} required />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password">Password</Label>
                <PasswordInput id="password" value={form.password} visible={showPassword} onChange={(value) => update('password', value)} onToggle={() => setShowPassword((value) => !value)} disabled={loading} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="confirmPassword">Confirm password</Label>
                <PasswordInput id="confirmPassword" value={form.confirmPassword} visible={showConfirmation} onChange={(value) => update('confirmPassword', value)} onToggle={() => setShowConfirmation((value) => !value)} disabled={loading} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="fullName">Full name</Label>
                <Input id="fullName" value={form.fullName} onChange={(e) => update('fullName', e.target.value)} disabled={loading} required />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="rank">Rank</Label>
                <select id="rank" value={form.rank} onChange={(e) => update('rank', e.target.value)} disabled={loading} required className="h-10 rounded-md border border-input bg-background px-3 text-sm">
                  <option value="">Select rank</option>
                  {VALID_RANKS.map((rank) => <option key={rank} value={rank}>{rank}</option>)}
                </select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="battery">Battery</Label>
                <select id="battery" value={form.battery} onChange={(e) => update('battery', e.target.value)} disabled={loading} required className="h-10 rounded-md border border-input bg-background px-3 text-sm">
                  <option value="">Select battery</option>
                  {VALID_BATTERIES.map((battery) => <option key={battery} value={battery}>{battery}</option>)}
                </select>
              </div>
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? 'Submitting...' : 'Create account'}
              </Button>
              <p className="text-center text-sm text-muted-foreground">
                Already have an account? <Link to="/sign-in" search={{ redirect: undefined, qrToken: undefined }} className="underline hover:text-foreground">Sign in</Link>
              </p>
            </form>
          )}
        </CardContent>
      </Card>
      <PublicFooter />
    </div>
  );
}

function PasswordInput({ id, value, visible, onChange, onToggle, disabled }: {
  id: string;
  value: string;
  visible: boolean;
  onChange: (value: string) => void;
  onToggle: () => void;
  disabled: boolean;
}) {
  return (
    <div className="relative">
      <Input id={id} type={visible ? 'text' : 'password'} value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled} required className="pr-10" />
      <button type="button" onClick={onToggle} disabled={disabled} aria-label={visible ? 'Hide password' : 'Show password'} className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">
        {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  );
}
