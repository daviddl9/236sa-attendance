import { useState } from 'react';
import { useAuth } from '@/lib/auth-context';
import { apiClient } from '@/lib/api-client';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { toast } from 'sonner';

export default function PasswordChangeGuard({ children }: { children: React.ReactNode }) {
  const { user, refetch } = useAuth();
  const [password, setPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [loading, setLoading] = useState(false);
  if (!user?.passwordChangeRequired) return <>{children}</>;

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setLoading(true);
    try {
      await apiClient.changePassword({ password, confirmPassword: confirmation });
      await refetch();
      toast.success('Password changed successfully');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to change password');
    } finally {
      setLoading(false);
    }
  };

  return <main className="flex min-h-screen items-center justify-center bg-background p-6">
    <Card className="w-full max-w-md">
      <CardHeader><CardTitle>Set a new password</CardTitle><CardDescription>Your temporary password must be replaced before you can continue.</CardDescription></CardHeader>
      <CardContent><form className="space-y-4" onSubmit={submit}>
        <div className="grid gap-2"><Label htmlFor="new-password">New password</Label><Input id="new-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} disabled={loading} required autoFocus /></div>
        <div className="grid gap-2"><Label htmlFor="confirm-new-password">Confirm new password</Label><Input id="confirm-new-password" type="password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} disabled={loading} required /></div>
        <Button className="w-full" type="submit" disabled={loading}>{loading ? 'Saving...' : 'Set password'}</Button>
      </form></CardContent>
    </Card>
  </main>;
}
