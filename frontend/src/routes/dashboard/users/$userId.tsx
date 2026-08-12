import { createFileRoute } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import { Input } from '../../../components/ui/input';
import { Label } from '../../../components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../../components/ui/select';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { useState, useEffect } from 'react';
import { ArrowLeft, KeyRound, Save } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '../../../lib/auth-context';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/dashboard/users/$userId')({
  component: UserDetailPage,
});

function UserDetailPage() {
  const { userId } = Route.useParams();
  const { user: currentUser } = useAuth();
  const queryClient = useQueryClient();

  const { data: user, isLoading } = useQuery({
    queryKey: ['user', userId],
    queryFn: () => apiClient.getUser(userId),
  });

  const [fullName, setFullName] = useState('');
  const [username, setUsername] = useState('');
  const [temporaryPassword, setTemporaryPassword] = useState<string | null>(null);
  const [rank, setRank] = useState('');
  const [battery, setBattery] = useState('');
  const [tierOverride, setTierOverride] = useState<'none' | '2' | '3'>('none');

  const isSuperadmin = currentUser?.isSuperadmin || false;
  const canEdit = isSuperadmin || currentUser?.id === userId;

  useEffect(() => {
    if (user) {
      /* eslint-disable react-hooks/set-state-in-effect */
      setFullName(user.fullName || '');
      setUsername(user.username || '');
      setRank(user.rank || '');
      setBattery(user.battery || '');
      setTierOverride(user.tierOverride ? String(user.tierOverride) as '2' | '3' : 'none');
      /* eslint-enable react-hooks/set-state-in-effect */
    }
  }, [user]);

  const provisionMutation = useMutation({
    mutationFn: () => apiClient.provisionUserCredentials(userId, username),
    onSuccess: (result) => {
      setTemporaryPassword(result.temporaryPassword);
      setUsername(result.username);
      queryClient.invalidateQueries({ queryKey: ['user', userId] });
      toast.success('Credentials provisioned. Share the temporary password now.');
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to provision credentials'),
  });

  const updateMutation = useMutation({
    mutationFn: (data: { fullName?: string; rank?: string; battery?: string; tierOverride?: 2 | 3 | null }) =>
      apiClient.updateUser(userId, data),
    onSuccess: () => {
      toast.success('User updated successfully');
      queryClient.invalidateQueries({ queryKey: ['user', userId] });
      queryClient.invalidateQueries({ queryKey: ['users'] });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to update user');
    },
  });

  const handleSave = () => {
    if (!canEdit) {
      toast.error('You do not have permission to edit this user');
      return;
    }

    const updates: { fullName?: string; rank?: string; battery?: string; tierOverride?: 2 | 3 | null } = {};
    if (fullName !== user?.fullName) updates.fullName = fullName;
    if (rank !== user?.rank) updates.rank = rank;
    if (battery !== user?.battery) updates.battery = battery;
    if (isSuperadmin) {
      const currentOverride = user?.tierOverride ?? null;
      const newOverride = tierOverride === 'none' ? null : (Number(tierOverride) as 2 | 3);
      if (newOverride !== currentOverride) updates.tierOverride = newOverride;
    }

    if (Object.keys(updates).length === 0) {
      toast.info('No changes to save');
      return;
    }

    updateMutation.mutate(updates);
  };

  if (isLoading) {
    return (
      <DashboardLayout>
        <div className="flex items-center justify-center py-12">
          <div className="text-muted-foreground">Loading...</div>
        </div>
      </DashboardLayout>
    );
  }

  if (!user) {
    return (
      <DashboardLayout>
        <div className="flex items-center justify-center py-12">
          <div className="text-muted-foreground">User not found</div>
        </div>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center gap-4">
          <Link to="/dashboard/users">
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">
              {user.fullName || 'User Details'}
            </h1>
            <p className="text-muted-foreground">View and edit user information</p>
          </div>
        </div>

        {isSuperadmin && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><KeyRound className="h-5 w-5" />Provision credentials</CardTitle>
              <CardDescription>Set a username and issue a one-time temporary password.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-2">
                <Label htmlFor="username">Username</Label>
                <Input id="username" value={username} onChange={(event) => setUsername(event.target.value)} disabled={provisionMutation.isPending} />
              </div>
              <Button onClick={() => provisionMutation.mutate()} disabled={provisionMutation.isPending || !username.trim()}>
                {provisionMutation.isPending ? 'Provisioning...' : 'Issue temporary password'}
              </Button>
              {temporaryPassword && (
                <div className="rounded-md border border-amber-500/50 bg-amber-50 p-3 text-sm">
                  <p className="font-medium">Temporary password — share it now</p>
                  <code className="mt-1 block select-all text-base">{temporaryPassword}</code>
                  <p className="mt-1 text-xs text-muted-foreground">It will not be shown again after leaving this page.</p>
                </div>
              )}
            </CardContent>
          </Card>
        )}

        <Card>
          <CardHeader>
            <CardTitle>User Information</CardTitle>
            <CardDescription>Update user details</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="fullName">Full Name</Label>
              <Input
                id="fullName"
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                disabled={!canEdit}
              />
            </div>

            {isSuperadmin && (
              <>
                <div className="grid gap-2">
                  <Label htmlFor="rank">Rank</Label>
                  <Select
                    value={rank || 'none'}
                    onValueChange={(value) => setRank(value === 'none' ? '' : value)}
                    disabled={!canEdit}
                  >
                    <SelectTrigger id="rank">
                      <SelectValue placeholder="Select Rank" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">None</SelectItem>
                      <SelectItem value="REC">REC</SelectItem>
                      <SelectItem value="PTE">PTE</SelectItem>
                      <SelectItem value="LCP">LCP</SelectItem>
                      <SelectItem value="CPL">CPL</SelectItem>
                      <SelectItem value="CFC">CFC</SelectItem>
                      <SelectItem value="3SG">3SG</SelectItem>
                      <SelectItem value="2SG">2SG</SelectItem>
                      <SelectItem value="1SG">1SG</SelectItem>
                      <SelectItem value="SSG">SSG</SelectItem>
                      <SelectItem value="MSG">MSG</SelectItem>
                      <SelectItem value="3WO">3WO</SelectItem>
                      <SelectItem value="2WO">2WO</SelectItem>
                      <SelectItem value="1WO">1WO</SelectItem>
                      <SelectItem value="MWO">MWO</SelectItem>
                      <SelectItem value="SWO">SWO</SelectItem>
                      <SelectItem value="2LT">2LT</SelectItem>
                      <SelectItem value="LTA">LTA</SelectItem>
                      <SelectItem value="CPT">CPT</SelectItem>
                      <SelectItem value="MAJ">MAJ</SelectItem>
                      <SelectItem value="LTC">LTC</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="grid gap-2">
                  <Label htmlFor="battery">Battery</Label>
                  <Select
                    value={battery || 'none'}
                    onValueChange={(value) => setBattery(value === 'none' ? '' : value)}
                    disabled={!canEdit}
                  >
                    <SelectTrigger id="battery">
                      <SelectValue placeholder="Select Battery" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">None</SelectItem>
                      <SelectItem value="HQ">HQ</SelectItem>
                      <SelectItem value="Alpha">Alpha</SelectItem>
                      <SelectItem value="Bravo">Bravo</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="grid gap-2">
                  <Label htmlFor="tierOverride">Access Tier Override</Label>
                  <Select
                    value={tierOverride}
                    onValueChange={(value) => setTierOverride(value as 'none' | '2' | '3')}
                    disabled={!canEdit}
                  >
                    <SelectTrigger id="tierOverride">
                      <SelectValue placeholder="Rank-derived (default)" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">Rank-derived (default)</SelectItem>
                      <SelectItem value="2">Tier 2 — Battery NCO (override)</SelectItem>
                      <SelectItem value="3">Tier 3 — Unit Commander (override)</SelectItem>
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-muted-foreground">
                    Override this user's access tier. Leave as rank-derived unless manually elevating.
                  </p>
                </div>
              </>
            )}

            {canEdit && (
              <div className="flex justify-end gap-2 pt-4">
                <Button onClick={handleSave} disabled={updateMutation.isPending}>
                  <Save className="mr-2 h-4 w-4" />
                  {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}
