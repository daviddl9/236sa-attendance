import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient, type UserProfile } from '../../../lib/api-client';
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
import { useMemo, useState } from 'react';
import { Plus, Search, Trash2, Upload, X } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '../../../lib/auth-context';
import { UserTable } from '../../../components/users/user-table';
import { AddUserDialog } from '../../../components/users/add-user-dialog';
import { BulkDeleteConfirmDialog } from '../../../components/users/bulk-delete-confirm-dialog';

const SEEDED_ADMIN_ID = '00000000000000000000000000000000';

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

  // Single-row delete (unchanged behaviour).
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [userToDelete, setUserToDelete] = useState<{ id: string; name: string } | null>(null);

  // Add-user dialog (feature 003).
  const [addDialogOpen, setAddDialogOpen] = useState(false);

  // Selection state for bulk delete (feature 004). The cache holds enough
  // info to render the confirmation dialog for ids that are no longer
  // visible under the current page / filters.
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set());
  const [selectionCache, setSelectionCache] = useState<Map<string, UserProfile>>(
    () => new Map(),
  );
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);

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
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Failed to delete user');
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

  // Selection helpers — keep `selectedIds` and `selectionCache` in lockstep
  // so the confirmation dialog always has Full Name / Rank / Battery to
  // show for every selected id, even ones that have scrolled off the page.
  const rememberUser = (u: UserProfile) => {
    setSelectionCache((prev) => {
      if (prev.has(u.id)) return prev;
      const next = new Map(prev);
      next.set(u.id, u);
      return next;
    });
  };

  const handleToggleRow = (userId: string) => {
    const targetUser = data?.users.find((u) => u.id === userId);
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(userId)) {
        next.delete(userId);
        setSelectionCache((cache) => {
          const c = new Map(cache);
          c.delete(userId);
          return c;
        });
      } else {
        next.add(userId);
        if (targetUser) rememberUser(targetUser);
      }
      return next;
    });
  };

  const handleTogglePage = (visibleIds: string[]) => {
    if (visibleIds.length === 0) return;
    const allSelected = visibleIds.every((id) => selectedIds.has(id));
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (allSelected) {
        for (const id of visibleIds) next.delete(id);
        setSelectionCache((cache) => {
          const c = new Map(cache);
          for (const id of visibleIds) c.delete(id);
          return c;
        });
      } else {
        for (const id of visibleIds) {
          next.add(id);
          const u = data?.users.find((x) => x.id === id);
          if (u) rememberUser(u);
        }
      }
      return next;
    });
  };

  const clearSelection = () => {
    setSelectedIds(new Set());
    setSelectionCache(new Map());
  };

  // The current user cannot delete themselves; the seeded admin row is
  // already excluded from the list, but we add it to disabledIds defensively.
  const currentUserId = user?.id;
  const disabledIds = useMemo(() => {
    const s = new Set<string>([SEEDED_ADMIN_ID]);
    if (currentUserId) s.add(currentUserId);
    return s;
  }, [currentUserId]);

  // The list passed into the confirmation dialog — ordered, deduplicated.
  const selectedUsers = useMemo(() => {
    const out: UserProfile[] = [];
    for (const id of selectedIds) {
      const cached = selectionCache.get(id);
      if (cached) out.push(cached);
    }
    return out;
  }, [selectedIds, selectionCache]);

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
              <Button onClick={() => setAddDialogOpen(true)}>
                <Plus className="mr-2 h-4 w-4" />
                Add User
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

        {isSuperadmin && selectedIds.size > 0 && (
          <div className="flex items-center justify-between rounded-md border bg-muted/30 px-3 py-2 text-sm">
            <div className="flex items-center gap-3">
              <span className="font-medium">
                {selectedIds.size} selected
              </span>
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground"
                onClick={clearSelection}
              >
                <X className="inline h-3 w-3" /> Clear
              </button>
            </div>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setBulkDeleteOpen(true)}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete selected
            </Button>
          </div>
        )}

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
            <UserTable
              users={data?.users || []}
              showActions={true}
              onDelete={handleDeleteClick}
              selectable={isSuperadmin}
              selectedIds={selectedIds}
              onToggleRow={handleToggleRow}
              onTogglePage={handleTogglePage}
              disabledIds={disabledIds}
            />

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

      {/* Single-row delete dialog (unchanged behaviour) */}
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

      {/* Add-user dialog (feature 003) */}
      {isSuperadmin && (
        <AddUserDialog open={addDialogOpen} onOpenChange={setAddDialogOpen} />
      )}

      {/* Selection-based bulk delete (feature 004) */}
      {isSuperadmin && (
        <BulkDeleteConfirmDialog
          open={bulkDeleteOpen}
          onOpenChange={setBulkDeleteOpen}
          selectedUsers={selectedUsers}
          onDeleted={() => clearSelection()}
        />
      )}
    </DashboardLayout>
  );
}
