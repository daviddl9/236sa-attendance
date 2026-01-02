import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import { Input } from '../../../components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../../components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../../../components/ui/dialog';
import { useState } from 'react';
import { AlertTriangle, Search, Trash2, Upload } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '../../../lib/auth-context';
import { UserTable } from '../../../components/users/user-table';

export const Route = createFileRoute('/dashboard/users/')({
  component: UsersPage,
});

function UsersPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [batteryFilter, setBatteryFilter] = useState('');
  const [rankFilter, setRankFilter] = useState('');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [userToDelete, setUserToDelete] = useState<{ id: string; name: string } | null>(null);
  const [bulkDeleteDialogOpen, setBulkDeleteDialogOpen] = useState(false);

  const isSuperadmin = user?.isSuperadmin || false;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['users', page, search, batteryFilter, rankFilter],
    queryFn: async () => {
      try {
        const result = await apiClient.listUsers({
          page,
          limit: 20,
          search: search || undefined,
          battery: batteryFilter || undefined,
          rank: rankFilter || undefined,
        });
        console.log('Users API response:', result);
        return result;
      } catch (err) {
        console.error('Error fetching users:', err);
        throw err;
      }
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (userId: string) => apiClient.deleteUser(userId),
    onSuccess: () => {
      toast.success('User deleted successfully');
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setDeleteDialogOpen(false);
      setUserToDelete(null);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete user');
    },
  });

  const bulkDeleteMutation = useMutation({
    mutationFn: () =>
      apiClient.bulkDeleteUsers({
        search: search || undefined,
        battery: batteryFilter || undefined,
        rank: rankFilter || undefined,
      }),
    onSuccess: (result) => {
      toast.success(`${result.deletedCount} users deleted successfully`);
      queryClient.invalidateQueries({ queryKey: ['users'] });
      setBulkDeleteDialogOpen(false);
      setPage(1);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete users');
    },
  });

  const handleDeleteClick = (userId: string) => {
    const targetUser = data?.users.find((u) => u.id === userId);
    setUserToDelete({
      id: userId,
      name: targetUser?.fullName || 'Unknown User',
    });
    setDeleteDialogOpen(true);
  };

  const confirmDelete = () => {
    if (userToDelete) {
      deleteMutation.mutate(userToDelete.id);
    }
  };

  const handleBulkUpload = () => {
    navigate({ to: '/dashboard/users/bulk-upload' });
  };

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Users</h1>
            <p className="text-muted-foreground">
              Manage users and their information
            </p>
          </div>
          {isSuperadmin && (
            <div className="flex gap-2">
              <Button
                onClick={() => setBulkDeleteDialogOpen(true)}
                variant="destructive"
                disabled={!data || data.total === 0}
              >
                <Trash2 className="mr-2 h-4 w-4" />
              </Button>
              <Button onClick={handleBulkUpload} variant="outline">
                <Upload className="mr-2 h-4 w-4" />
                Bulk Upload
              </Button>
            </div>
          )}
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <div className="relative">
              <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by name..."
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setPage(1);
                }}
                className="pl-8"
              />
            </div>
          </div>
          <Select
            value={batteryFilter || 'all'}
            onValueChange={(value) => {
              setBatteryFilter(value === 'all' ? '' : value);
              setPage(1);
            }}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="All Batteries" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Batteries</SelectItem>
              <SelectItem value="HQ">HQ</SelectItem>
              <SelectItem value="Alpha">Alpha</SelectItem>
              <SelectItem value="Bravo">Bravo</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={rankFilter || 'all'}
            onValueChange={(value) => {
              setRankFilter(value === 'all' ? '' : value);
              setPage(1);
            }}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="All Ranks" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Ranks</SelectItem>
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

        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-muted-foreground">Loading...</div>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2">
            <div className="text-destructive font-semibold">
              Error loading users: {error instanceof Error ? error.message : 'Unknown error'}
            </div>
            {error instanceof Error && error.message.includes('403') && (
              <div className="text-sm text-muted-foreground">
                You may not have permission to view users. Commander or superadmin access required.
              </div>
            )}
            <Button onClick={() => refetch()} variant="outline" className="mt-2">
              Retry
            </Button>
          </div>
        ) : (
          <>
            {data && (
              <div className="text-sm text-muted-foreground mb-2">
                Found {data.total} user{data.total !== 1 ? 's' : ''}
              </div>
            )}
            <UserTable users={data?.users || []} showActions={true} onDelete={handleDeleteClick} />

            {data && data.total > 20 && (
              <div className="flex items-center justify-between">
                <div className="text-sm text-muted-foreground">
                  Showing {(page - 1) * 20 + 1} to {Math.min(page * 20, data.total)} of{' '}
                  {data.total} users
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page === 1}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => setPage((p) => p + 1)}
                    disabled={page * 20 >= data.total}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete User</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete <strong>{userToDelete?.name}</strong>? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={confirmDelete}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={bulkDeleteDialogOpen} onOpenChange={setBulkDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-destructive flex items-center gap-2">
              <AlertTriangle className="h-5 w-5" />
              Delete Multiple Users
            </DialogTitle>
            <DialogDescription className="space-y-4">
              <div className="bg-destructive/10 border border-destructive/20 rounded-md p-3 text-destructive text-sm">
                <strong>Warning:</strong> This action is permanent and cannot be
                undone. All associated data (attendance records, sessions) will
                also be deleted.
              </div>

              <div className="text-foreground">
                You are about to delete{' '}
                <strong className="text-destructive">{data?.total} users</strong>
                {(search || batteryFilter || rankFilter) && (
                  <span> matching the current filters:</span>
                )}
              </div>

              {(search || batteryFilter || rankFilter) && (
                <ul className="text-sm text-muted-foreground list-disc list-inside">
                  {search && <li>Name contains: &quot;{search}&quot;</li>}
                  {batteryFilter && <li>Battery: {batteryFilter}</li>}
                  {rankFilter && <li>Rank: {rankFilter}</li>}
                </ul>
              )}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setBulkDeleteDialogOpen(false)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => bulkDeleteMutation.mutate()}
              disabled={bulkDeleteMutation.isPending || (data?.total ?? 0) === 0}
            >
              {bulkDeleteMutation.isPending
                ? 'Deleting...'
                : `Delete ${data?.total} Users`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
}

