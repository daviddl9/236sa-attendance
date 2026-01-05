import { createFileRoute } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  apiClient,
  STATUS_DISPLAY_CONFIG,
  type StatusType,
  type CreateStatusRequest,
  type UserStatusWithUser,
} from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import { Input } from '../../../components/ui/input';
import { Label } from '../../../components/ui/label';
import { Switch } from '../../../components/ui/switch';
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../../components/ui/table';
import { Card, CardContent } from '../../../components/ui/card';
import { useState } from 'react';
import { Plus, Trash2, Pencil, X } from 'lucide-react';
import { toast } from 'sonner';
import { StatusBadge } from '../../../components/status-badge';
import { cn } from '@/lib/utils';
import { DatePicker } from '../../../components/ui/date-picker';

export const Route = createFileRoute('/dashboard/statuses/')({
  component: StatusesPage,
});

interface StatusDateEntry {
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
}

function StatusesPage() {
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [statusTypeFilter, setStatusTypeFilter] = useState<string>('');
  const [activeOnly, setActiveOnly] = useState(true);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [statusToDelete, setStatusToDelete] = useState<UserStatusWithUser | null>(null);
  const [statusToEdit, setStatusToEdit] = useState<UserStatusWithUser | null>(null);

  // Form state
  const [selectedUserId, setSelectedUserId] = useState('');
  const [selectedStatusType, setSelectedStatusType] = useState<StatusType>('stay_out');
  const [dailyAfterTime, setDailyAfterTime] = useState('18:30');
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');
  const [notes, setNotes] = useState('');
  const [statusDates, setStatusDates] = useState<StatusDateEntry[]>([]);
  const [userSearch, setUserSearch] = useState('');

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['statuses', page, statusTypeFilter, activeOnly],
    queryFn: () =>
      apiClient.listStatuses({
        page,
        limit: 20,
        statusType: statusTypeFilter as StatusType | undefined,
        active: activeOnly,
      }),
  });

  const { data: usersData } = useQuery({
    queryKey: ['users-for-status', userSearch],
    queryFn: () => apiClient.listUsers({ limit: 10, search: userSearch || undefined }),
    enabled: createDialogOpen || editDialogOpen,
  });

  const createMutation = useMutation({
    mutationFn: (data: CreateStatusRequest) => apiClient.createStatus(data),
    onSuccess: () => {
      toast.success('Status created successfully');
      queryClient.invalidateQueries({ queryKey: ['statuses'] });
      resetForm();
      setCreateDialogOpen(false);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to create status');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.deleteStatus(id),
    onSuccess: () => {
      toast.success('Status deleted successfully');
      queryClient.invalidateQueries({ queryKey: ['statuses'] });
      setDeleteDialogOpen(false);
      setStatusToDelete(null);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to delete status');
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: CreateStatusRequest }) =>
      apiClient.updateStatus(id, data),
    onSuccess: () => {
      toast.success('Status updated successfully');
      queryClient.invalidateQueries({ queryKey: ['statuses'] });
      resetForm();
      setEditDialogOpen(false);
      setStatusToEdit(null);
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to update status');
    },
  });

  const resetForm = () => {
    setSelectedUserId('');
    setSelectedStatusType('stay_out');
    setDailyAfterTime('18:30');
    setStartDate('');
    setEndDate('');
    setNotes('');
    setStatusDates([]);
    setUserSearch('');
  };

  const handleCreate = () => {
    if (!selectedUserId) {
      toast.error('Please select a user');
      return;
    }

    const request: CreateStatusRequest = {
      userId: selectedUserId,
      statusType: selectedStatusType,
    };

    if (selectedStatusType === 'stay_out') {
      if (!dailyAfterTime) {
        toast.error('Please enter daily after time');
        return;
      }
      request.dailyAfterTime = dailyAfterTime;
    } else if (selectedStatusType === 'mc') {
      if (!startDate || !endDate) {
        toast.error('Please select start and end dates');
        return;
      }
      request.startDate = startDate;
      request.endDate = endDate;
    } else {
      if (statusDates.length === 0) {
        toast.error('Please add at least one date');
        return;
      }
      request.dates = statusDates;
    }

    if (selectedStatusType === 'other' && !notes.trim()) {
      toast.error('Notes are required for Other status type');
      return;
    }

    if (notes.trim()) {
      request.notes = notes.trim();
    }

    createMutation.mutate(request);
  };

  const handleUpdate = () => {
    if (!statusToEdit) return;

    const request: CreateStatusRequest = {
      userId: statusToEdit.userId,
      statusType: selectedStatusType,
    };

    if (selectedStatusType === 'stay_out') {
      request.dailyAfterTime = dailyAfterTime;
    } else if (selectedStatusType === 'mc') {
      if (startDate) request.startDate = startDate;
      if (endDate) request.endDate = endDate;
    } else {
      request.dates = statusDates;
    }

    if (notes.trim()) {
      request.notes = notes.trim();
    }

    updateMutation.mutate({ id: statusToEdit.id, data: request });
  };

  const openEditDialog = (status: UserStatusWithUser) => {
    setStatusToEdit(status);
    setSelectedStatusType(status.statusType as StatusType);
    setDailyAfterTime(status.dailyAfterTime || '18:30');
    setStartDate(status.startDate || '');
    setEndDate(status.endDate || '');
    setNotes(status.notes || '');
    setStatusDates(
      status.dates?.map((d) => ({
        startDate: d.startDate,
        startTime: d.startTime,
        endDate: d.endDate,
        endTime: d.endTime,
      })) || []
    );
    setEditDialogOpen(true);
  };

  const addStatusDate = () => {
    const today = new Date().toISOString().split('T')[0];
    setStatusDates([
      ...statusDates,
      { startDate: today, startTime: '09:00', endDate: today, endTime: '18:00' },
    ]);
  };

  const removeStatusDate = (index: number) => {
    setStatusDates(statusDates.filter((_, i) => i !== index));
  };

  const updateStatusDate = (
    index: number,
    field: keyof StatusDateEntry,
    value: string
  ) => {
    const newDates = [...statusDates];
    newDates[index] = { ...newDates[index], [field]: value };
    setStatusDates(newDates);
  };

  const formatTime = (time: string) => {
    // Convert "18:30:00.00000" to "1830 SGT" (strip seconds and beyond)
    const cleanTime = time.split(':').slice(0, 2).join('');
    return cleanTime + ' SGT';
  };

  const formatStatusDetails = (status: UserStatusWithUser) => {
    if (status.statusType === 'stay_out') {
      return `Daily after ${formatTime(status.dailyAfterTime || '')}`;
    }
    if (status.statusType === 'mc') {
      return `${status.startDate} to ${status.endDate}`;
    }
    if (status.dates && status.dates.length > 0) {
      return `${status.dates.length} date(s)`;
    }
    return '-';
  };

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Statuses</h1>
            <p className="text-muted-foreground">
              Manage personnel statuses (Stay Out, Off Pass, MC, etc.)
            </p>
          </div>
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Status
          </Button>
        </div>

        <div className="flex gap-4 items-center">
          <Select
            value={statusTypeFilter || 'all'}
            onValueChange={(value) => {
              setStatusTypeFilter(value === 'all' ? '' : value);
              setPage(1);
            }}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="All Types" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Types</SelectItem>
              {Object.entries(STATUS_DISPLAY_CONFIG).map(([key, config]) => (
                <SelectItem key={key} value={key}>
                  {config.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="flex items-center space-x-2">
            <Switch
              id="active-only"
              checked={activeOnly}
              onCheckedChange={setActiveOnly}
            />
            <Label htmlFor="active-only">Active only</Label>
          </div>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-muted-foreground">Loading...</div>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2">
            <div className="text-destructive font-semibold">
              Error loading statuses
            </div>
            <Button onClick={() => refetch()} variant="outline" className="mt-2">
              Retry
            </Button>
          </div>
        ) : (
          <>
            {data && (
              <div className="text-sm text-muted-foreground mb-2">
                Found {data.total} status{data.total !== 1 ? 'es' : ''}
              </div>
            )}
            <Card>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>User</TableHead>
                      <TableHead>Type</TableHead>
                      <TableHead>Details</TableHead>
                      <TableHead>Notes</TableHead>
                      <TableHead className="w-[100px]">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data?.statuses && data.statuses.length > 0 ? (
                      data.statuses.map((status) => (
                        <TableRow key={status.id}>
                          <TableCell>
                            <div>
                              <div className="font-medium">
                                {status.userFullName || 'Unknown'}
                              </div>
                              <div className="text-xs text-muted-foreground">
                                {status.userRank} - {status.userBattery}
                              </div>
                            </div>
                          </TableCell>
                          <TableCell>
                            <StatusBadge statusType={status.statusType as StatusType} />
                          </TableCell>
                          <TableCell className="text-sm">
                            {formatStatusDetails(status)}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground max-w-[200px] truncate">
                            {status.notes || '-'}
                          </TableCell>
                          <TableCell>
                            <div className="flex gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => openEditDialog(status)}
                              >
                                <Pencil className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => {
                                  setStatusToDelete(status);
                                  setDeleteDialogOpen(true);
                                }}
                              >
                                <Trash2 className="h-4 w-4 text-destructive" />
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                          No statuses found
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>

            {data && data.total > 20 && (
              <div className="flex items-center justify-between">
                <div className="text-sm text-muted-foreground">
                  Showing {(page - 1) * 20 + 1} to {Math.min(page * 20, data.total)} of{' '}
                  {data.total} statuses
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

      {/* Create Dialog */}
      <Dialog open={createDialogOpen} onOpenChange={(open) => {
        if (!open) resetForm();
        setCreateDialogOpen(open);
      }}>
        <DialogContent className="w-[calc(100vw-2rem)] max-w-lg max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Add Status</DialogTitle>
            <DialogDescription>
              Assign a status to a user
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {/* User Search */}
            <div className="space-y-2">
              <Label>User</Label>
              <Input
                placeholder="Search user by name..."
                value={userSearch}
                onChange={(e) => setUserSearch(e.target.value)}
              />
              {usersData && usersData.users.length > 0 && (
                <div className="border rounded-md max-h-32 overflow-y-auto overflow-x-hidden">
                  {usersData.users.map((user) => (
                    <div
                      key={user.id}
                      className={cn(
                        'px-3 py-2 cursor-pointer hover:bg-accent text-sm break-words',
                        selectedUserId === user.id && 'bg-accent'
                      )}
                      onClick={() => {
                        setSelectedUserId(user.id);
                        setUserSearch(user.fullName || '');
                      }}
                    >
                      {user.fullName} ({user.rank} - {user.battery})
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Status Type */}
            <div className="space-y-2">
              <Label>Status Type</Label>
              <Select
                value={selectedStatusType}
                onValueChange={(v) => setSelectedStatusType(v as StatusType)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(STATUS_DISPLAY_CONFIG).map(([key, config]) => (
                    <SelectItem key={key} value={key}>
                      {config.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Conditional Fields */}
            {selectedStatusType === 'stay_out' && (
              <div className="space-y-2">
                <Label>Daily After Time</Label>
                <Input
                  type="time"
                  value={dailyAfterTime}
                  onChange={(e) => setDailyAfterTime(e.target.value)}
                />
              </div>
            )}

            {selectedStatusType === 'mc' && (
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Start Date</Label>
                  <DatePicker
                    value={startDate}
                    onChange={setStartDate}
                    placeholder="Pick start date"
                  />
                </div>
                <div className="space-y-2">
                  <Label>End Date</Label>
                  <DatePicker
                    value={endDate}
                    onChange={setEndDate}
                    placeholder="Pick end date"
                  />
                </div>
              </div>
            )}

            {['off_pass', 'course', 'other'].includes(selectedStatusType) && (
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>Dates</Label>
                  <Button type="button" variant="outline" size="sm" onClick={addStatusDate}>
                    <Plus className="h-4 w-4 mr-1" /> Add Date
                  </Button>
                </div>
                {statusDates.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No dates added yet</p>
                ) : (
                  <div className="space-y-3">
                    {statusDates.map((entry, index) => (
                      <div key={index} className="border rounded-md p-3 space-y-2">
                        <div className="flex items-center justify-between">
                          <span className="text-sm font-medium">Date Entry {index + 1}</span>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            onClick={() => removeStatusDate(index)}
                          >
                            <X className="h-4 w-4" />
                          </Button>
                        </div>
                        <div className="grid grid-cols-2 gap-2">
                          <div className="space-y-1">
                            <Label className="text-xs">Start Date</Label>
                            <DatePicker
                              value={entry.startDate}
                              onChange={(date) => updateStatusDate(index, 'startDate', date)}
                              placeholder="Start date"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Start Time</Label>
                            <Input
                              type="time"
                              value={entry.startTime}
                              onChange={(e) => updateStatusDate(index, 'startTime', e.target.value)}
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">End Date</Label>
                            <DatePicker
                              value={entry.endDate}
                              onChange={(date) => updateStatusDate(index, 'endDate', date)}
                              placeholder="End date"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">End Time</Label>
                            <Input
                              type="time"
                              value={entry.endTime}
                              onChange={(e) => updateStatusDate(index, 'endTime', e.target.value)}
                            />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Notes */}
            <div className="space-y-2">
              <Label>
                Notes {selectedStatusType === 'other' && <span className="text-destructive">*</span>}
              </Label>
              <Input
                placeholder={selectedStatusType === 'other' ? 'Required for Other status' : 'Optional notes...'}
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
              />
            </div>
          </div>

          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button variant="outline" onClick={() => setCreateDialogOpen(false)} className="w-full sm:w-auto">
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={createMutation.isPending} className="w-full sm:w-auto">
              {createMutation.isPending ? 'Creating...' : 'Create'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={(open) => {
        if (!open) {
          resetForm();
          setStatusToEdit(null);
        }
        setEditDialogOpen(open);
      }}>
        <DialogContent className="w-[calc(100vw-2rem)] max-w-lg max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Edit Status</DialogTitle>
            <DialogDescription className="break-words">
              Update status for {statusToEdit?.userFullName}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {/* Status Type */}
            <div className="space-y-2">
              <Label>Status Type</Label>
              <Select
                value={selectedStatusType}
                onValueChange={(v) => setSelectedStatusType(v as StatusType)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(STATUS_DISPLAY_CONFIG).map(([key, config]) => (
                    <SelectItem key={key} value={key}>
                      {config.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Conditional Fields - same as create */}
            {selectedStatusType === 'stay_out' && (
              <div className="space-y-2">
                <Label>Daily After Time</Label>
                <Input
                  type="time"
                  value={dailyAfterTime}
                  onChange={(e) => setDailyAfterTime(e.target.value)}
                />
              </div>
            )}

            {selectedStatusType === 'mc' && (
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Start Date</Label>
                  <DatePicker
                    value={startDate}
                    onChange={setStartDate}
                    placeholder="Pick start date"
                  />
                </div>
                <div className="space-y-2">
                  <Label>End Date</Label>
                  <DatePicker
                    value={endDate}
                    onChange={setEndDate}
                    placeholder="Pick end date"
                  />
                </div>
              </div>
            )}

            {['off_pass', 'course', 'other'].includes(selectedStatusType) && (
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>Dates</Label>
                  <Button type="button" variant="outline" size="sm" onClick={addStatusDate}>
                    <Plus className="h-4 w-4 mr-1" /> Add Date
                  </Button>
                </div>
                {statusDates.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No dates added yet</p>
                ) : (
                  <div className="space-y-3">
                    {statusDates.map((entry, index) => (
                      <div key={index} className="border rounded-md p-3 space-y-2">
                        <div className="flex items-center justify-between">
                          <span className="text-sm font-medium">Date Entry {index + 1}</span>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            onClick={() => removeStatusDate(index)}
                          >
                            <X className="h-4 w-4" />
                          </Button>
                        </div>
                        <div className="grid grid-cols-2 gap-2">
                          <div className="space-y-1">
                            <Label className="text-xs">Start Date</Label>
                            <DatePicker
                              value={entry.startDate}
                              onChange={(date) => updateStatusDate(index, 'startDate', date)}
                              placeholder="Start date"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">Start Time</Label>
                            <Input
                              type="time"
                              value={entry.startTime}
                              onChange={(e) => updateStatusDate(index, 'startTime', e.target.value)}
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">End Date</Label>
                            <DatePicker
                              value={entry.endDate}
                              onChange={(date) => updateStatusDate(index, 'endDate', date)}
                              placeholder="End date"
                            />
                          </div>
                          <div className="space-y-1">
                            <Label className="text-xs">End Time</Label>
                            <Input
                              type="time"
                              value={entry.endTime}
                              onChange={(e) => updateStatusDate(index, 'endTime', e.target.value)}
                            />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Notes */}
            <div className="space-y-2">
              <Label>
                Notes {selectedStatusType === 'other' && <span className="text-destructive">*</span>}
              </Label>
              <Input
                placeholder={selectedStatusType === 'other' ? 'Required for Other status' : 'Optional notes...'}
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
              />
            </div>
          </div>

          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button variant="outline" onClick={() => setEditDialogOpen(false)} className="w-full sm:w-auto">
              Cancel
            </Button>
            <Button onClick={handleUpdate} disabled={updateMutation.isPending} className="w-full sm:w-auto">
              {updateMutation.isPending ? 'Saving...' : 'Save'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Status</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this status for{' '}
              <strong>{statusToDelete?.userFullName}</strong>? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => statusToDelete && deleteMutation.mutate(statusToDelete.id)}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
}
