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
import { ArrowLeft, Save } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '../../../lib/auth-context';
import { Link } from '@tanstack/react-router';
import { isValidNricLast5, normalizeNricLast5, NRIC_LAST5_FIELD_MESSAGE } from '../../../lib/nric-password';

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
  const [rank, setRank] = useState('');
  const [battery, setBattery] = useState('');
  const [nricLast5, setNricLast5] = useState('');

  const isSuperadmin = currentUser?.isSuperadmin || false;
  const canEdit = isSuperadmin || currentUser?.id === userId;

  useEffect(() => {
    if (user) {
      /* eslint-disable react-hooks/set-state-in-effect */
      setFullName(user.fullName || '');
      setRank(user.rank || '');
      setBattery(user.battery || '');
      setNricLast5(user.nricLast5 || '');
      /* eslint-enable react-hooks/set-state-in-effect */
    }
  }, [user]);

  const updateMutation = useMutation({
    mutationFn: (data: { fullName?: string; rank?: string; battery?: string; nricLast5?: string }) =>
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

    const updates: { fullName?: string; rank?: string; battery?: string; nricLast5?: string } = {};
    if (fullName !== user?.fullName) updates.fullName = fullName;
    if (rank !== user?.rank) updates.rank = rank;
    if (battery !== user?.battery) updates.battery = battery;
    if (isSuperadmin && nricLast5 !== (user?.nricLast5 || '')) {
      if (!isValidNricLast5(nricLast5)) {
        toast.error(NRIC_LAST5_FIELD_MESSAGE);
        return;
      }
      updates.nricLast5 = normalizeNricLast5(nricLast5);
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
                  <Label htmlFor="nricLast5">NRIC Last 5</Label>
                  <Input
                    id="nricLast5"
                    value={nricLast5}
                    onChange={(e) =>
                      setNricLast5(normalizeNricLast5(e.target.value).slice(0, 5))
                    }
                    placeholder="e.g., 1234A"
                    maxLength={5}
                    disabled={!canEdit}
                  />
                  <p className="text-xs text-muted-foreground">
                    Four digits followed by a letter. Updating this also updates the user's password.
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
