import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, useRef, useMemo } from 'react';
import { apiClient, type ParticipantMatch, type UnmatchedRow } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { Input } from '../../../components/ui/input';
import { Label } from '../../../components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '../../../components/ui/dialog';
import { Badge } from '../../../components/ui/badge';
import { AddUserDialog } from '../../../components/users/add-user-dialog';
import { UserTable } from '../../../components/users/user-table';
import { Users, Upload, Trash2, Play, Lock, Plus, Search, X } from 'lucide-react';
import { toast } from 'sonner';

export const Route = createFileRoute('/dashboard/groups/')({
  component: GroupsPage,
});

function GroupsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [previewLoading, setPreviewLoading] = useState(false);
  const [matched, setMatched] = useState<ParticipantMatch[]>([]);
  const [unmatched, setUnmatched] = useState<UnmatchedRow[]>([]);
  const [previewDone, setPreviewDone] = useState(false);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [sessionDialog, setSessionDialog] = useState<{ groupId: string; groupName: string } | null>(null);
  const [sessionName, setSessionName] = useState('');

  // Member management state.
  const [memberDialog, setMemberDialog] = useState<{ groupId: string; groupName: string } | null>(null);
  const [addMembersOpen, setAddMembersOpen] = useState(false);
  const [memberSearch, setMemberSearch] = useState('');
  const [addUserOpen, setAddUserOpen] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ['groups'],
    queryFn: () => apiClient.listGroups(),
  });
  const groups = data?.groups ?? [];

  const deleteMutation = useMutation({
    mutationFn: (id: string) => apiClient.deleteGroup(id),
    onSuccess: () => {
      toast.success('Group deleted');
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to delete group'),
  });

  // Members of the group currently open in the member dialog.
  const { data: groupDetail, isLoading: membersLoading } = useQuery({
    queryKey: ['group-members', memberDialog?.groupId],
    queryFn: () => apiClient.getGroup(memberDialog!.groupId),
    enabled: !!memberDialog,
  });
  const members = useMemo(() => groupDetail?.members ?? [], [groupDetail]);

  const removeMemberMutation = useMutation({
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      apiClient.deleteGroupMember(groupId, userId),
    onSuccess: (_data, vars) => {
      toast.success('Member removed');
      queryClient.invalidateQueries({ queryKey: ['group-members', vars.groupId] });
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to remove member'),
  });

  const setMembersMutation = useMutation({
    mutationFn: ({ groupId, memberIds }: { groupId: string; memberIds: string[] }) =>
      apiClient.setGroupMembers(groupId, memberIds),
    onSuccess: (_data, vars) => {
      toast.success('Members updated');
      queryClient.invalidateQueries({ queryKey: ['group-members', vars.groupId] });
      queryClient.invalidateQueries({ queryKey: ['groups'] });
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to update members'),
  });

  // Load the full roster once; the dialog filters it client-side as the user
  // types (same pattern as manual attendance marking).
  const { data: allUsers, isLoading: rosterLoading } = useQuery({
    queryKey: ['users', 'all'],
    queryFn: () => apiClient.listAllUsers(),
    enabled: addMembersOpen,
  });

  // Roster rows that are not yet group members, live-filtered by name/rank.
  const rosterCandidates = useMemo(() => {
    const existing = new Set(members.map((m) => m.userId));
    const q = memberSearch.trim().toLowerCase();
    return (allUsers ?? []).filter((u) => {
      if (existing.has(u.id)) return false;
      if (!q) return true;
      return (
        u.fullName?.toLowerCase().includes(q) ||
        u.rank?.toLowerCase().includes(q)
      );
    });
  }, [allUsers, memberSearch, members]);

  // Add a single person to the group immediately (the per-row add action).
  const [addingUserId, setAddingUserId] = useState<string | null>(null);
  const handleAddOne = (userId: string) => {
    if (!memberDialog) return;
    const currentIds = members.map((m) => m.userId);
    setAddingUserId(userId);
    setMembersMutation.mutate(
      { groupId: memberDialog.groupId, memberIds: [...currentIds, userId] },
      { onSettled: () => setAddingUserId(null) },
    );
  };

  const createGroupMutation = useMutation({
    mutationFn: (data: { name: string; participantIds: string[] }) =>
      apiClient.createGroup(data),
    onSuccess: () => {
      toast.success('Group created');
      queryClient.invalidateQueries({ queryKey: ['groups'] });
      setCreateDialogOpen(false);
      setPreviewDone(false);
      setMatched([]);
      setUnmatched([]);
      setName('');
      setPassword('');
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to create group'),
  });

  const startSessionMutation = useMutation({
    mutationFn: (data: { groupId: string; name: string }) =>
      apiClient.createSessionFromGroup(data.groupId, { name: data.name, participantIds: [] }),
    onSuccess: (session) => {
      toast.success('Session created from group');
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      setSessionDialog(null);
      navigate({ to: '/dashboard/sessions/$sessionId', params: { sessionId: session.id } });
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to create session'),
  });

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setPreviewLoading(true);
    setPreviewDone(false);
    setMatched([]);
    setUnmatched([]);

    try {
      const result = await apiClient.previewGroupFromExcel(file, password || undefined);
      setMatched(result.matched);
      setUnmatched(result.unmatched);
      setPreviewDone(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to parse file');
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleCreate = () => {
    if (!name.trim()) {
      toast.error('Please enter a group name');
      return;
    }
    if (matched.length === 0) {
      toast.error('No matched participants to save');
      return;
    }
    createGroupMutation.mutate({
      name: name.trim(),
      participantIds: matched.map((m) => m.userId),
    });
  };

  const handleStartSession = () => {
    if (!sessionDialog) return;
    if (!sessionName.trim()) {
      toast.error('Please enter a session name');
      return;
    }
    startSessionMutation.mutate({ groupId: sessionDialog.groupId, name: sessionName.trim() });
  };

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Groups</h1>
            <p className="text-muted-foreground">
              Reusable participant lists — create a session from a group in one click
            </p>
          </div>
        </div>

        <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
          <DialogTrigger asChild>
            <Button className="w-fit">
              <Upload className="mr-2 h-4 w-4" />
              Create Group from Excel
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>Create a reusable group</DialogTitle>
              <DialogDescription>
                Upload a roster Excel (e.g. a call-up list). Matched rows are saved as a group you can reuse for every session.
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="group-name">Group name</Label>
                <Input
                  id="group-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Advance Party"
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="group-password">
                  <span className="flex items-center gap-1">
                    <Lock className="h-3 w-3" /> File password (optional)
                  </span>
                </Label>
                <Input
                  id="group-password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Password if the Excel is protected"
                />
              </div>

              <div className="grid gap-2">
                <Label>Excel file</Label>
                <Input
                  ref={fileInputRef as never}
                  type="file"
                  accept=".xlsx,.xls"
                  onChange={handleFileChange}
                  disabled={previewLoading}
                />
              </div>

              {previewLoading && <div className="text-sm text-muted-foreground">Parsing and matching…</div>}

              {previewDone && (
                <div className="rounded-md border p-4 space-y-3">
                  <div className="flex items-center gap-2 text-sm">
                    <Users className="h-4 w-4" />
                    <strong>{matched.length} matched</strong>
                    {unmatched.length > 0 && (
                      <Badge variant="destructive" className="ml-2">
                        {unmatched.length} unmatched
                      </Badge>
                    )}
                  </div>
                  {unmatched.length > 0 && (
                    <div className="max-h-32 overflow-y-auto text-sm text-destructive/80 space-y-1">
                      {unmatched.map((u) => (
                        <div key={u.rowNum}>
                          Row {u.rowNum}: <strong>{u.fullName}</strong> — {u.reason}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>

            <DialogFooter>
              <Button variant="outline" onClick={() => setCreateDialogOpen(false)}>
                Cancel
              </Button>
              <Button
                onClick={handleCreate}
                disabled={!previewDone || matched.length === 0 || createGroupMutation.isPending}
              >
                <Users className="mr-2 h-4 w-4" />
                Save Group
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {isLoading ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">Loading…</div>
        ) : groups.length === 0 ? (
          <Card>
            <CardContent className="p-8 text-center text-muted-foreground">
              No groups yet. Create one from an Excel roster above.
            </CardContent>
          </Card>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {groups.map((group) => (
              <Card key={group.id}>
                <CardHeader>
                  <CardTitle className="text-base">{group.name}</CardTitle>
                  <CardDescription className="flex items-center gap-1">
                    <Users className="h-3 w-3" />
                    {group.memberCount ?? 0} members
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setMemberDialog({ groupId: group.id, groupName: group.name })}
                  >
                    <Users className="mr-2 h-4 w-4" />
                    Members
                  </Button>
                  <Button
                    size="sm"
                    onClick={() => {
                      setSessionDialog({ groupId: group.id, groupName: group.name });
                      setSessionName(group.name);
                    }}
                  >
                    <Play className="mr-2 h-4 w-4" />
                    Start Session
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-destructive"
                    onClick={() => deleteMutation.mutate(group.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        <Dialog open={!!memberDialog} onOpenChange={(o) => !o && setMemberDialog(null)}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>Members — {memberDialog?.groupName}</DialogTitle>
              <DialogDescription>
                {membersLoading ? 'Loading members…' : `${members.length} members`}
              </DialogDescription>
            </DialogHeader>

            <div className="max-h-80 space-y-1 overflow-y-auto">
              {members.length === 0 && !membersLoading && (
                <p className="py-6 text-center text-sm text-muted-foreground">No members yet.</p>
              )}
              {members.map((m) => (
                <div
                  key={m.userId}
                  className="flex items-center justify-between rounded-md border px-3 py-2 text-sm"
                >
                  <div className="min-w-0">
                    <div className="truncate font-medium">{m.fullName}</div>
                    <div className="text-xs text-muted-foreground">
                      {[m.rank, m.battery].filter(Boolean).join(' · ') || '—'}
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-destructive"
                    disabled={removeMemberMutation.isPending}
                    onClick={() =>
                      removeMemberMutation.mutate({ groupId: memberDialog!.groupId, userId: m.userId })
                    }
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>

            <DialogFooter className="gap-2">
              <Button variant="outline" onClick={() => setAddUserOpen(true)}>
                <Plus className="mr-2 h-4 w-4" />
                New Person
              </Button>
              <Button onClick={() => setAddMembersOpen(true)}>
                <Search className="mr-2 h-4 w-4" />
                Add People
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={addMembersOpen} onOpenChange={(o) => !o && setAddMembersOpen(false)}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>Add people to {memberDialog?.groupName}</DialogTitle>
              <DialogDescription>
                Type to filter the roster, then tap a row to add. Or create a new person.
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-2">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={memberSearch}
                  onChange={(e) => setMemberSearch(e.target.value)}
                  placeholder="Search by name or rank…"
                  className="pl-9"
                  autoFocus
                />
              </div>
            </div>

            {rosterLoading ? (
              <p className="py-6 text-center text-sm text-muted-foreground">Loading roster…</p>
            ) : rosterCandidates.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                {memberSearch.trim() ? 'No matching users' : 'Everyone is already in this group'}
              </p>
            ) : (
              <div className="max-h-72 overflow-y-auto rounded-md">
                <UserTable
                  users={rosterCandidates}
                  showActions={false}
                  emptyMessage="No matching users"
                  onMark={handleAddOne}
                  markingUserId={addingUserId ?? undefined}
                />
              </div>
            )}

            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setAddMembersOpen(false);
                  setAddUserOpen(true);
                }}
              >
                <Plus className="mr-2 h-4 w-4" />
                New Person
              </Button>
              <Button variant="outline" onClick={() => setAddMembersOpen(false)}>Done</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <AddUserDialog
          open={addUserOpen}
          onOpenChange={setAddUserOpen}
          onCreated={(user) => {
            if (!memberDialog) return;
            setMembersMutation.mutate({
              groupId: memberDialog.groupId,
              memberIds: [...members.map((m) => m.userId), user.id],
            });
          }}
        />

        <Dialog open={!!sessionDialog} onOpenChange={(o) => !o && setSessionDialog(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Start session from group</DialogTitle>
              <DialogDescription>
                Create a new attendance session from <strong>{sessionDialog?.groupName}</strong>.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-2">
              <Label htmlFor="session-name">Session name</Label>
              <Input
                id="session-name"
                value={sessionName}
                onChange={(e) => setSessionName(e.target.value)}
                placeholder="e.g. Advance Party — Day 1"
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setSessionDialog(null)}>Cancel</Button>
              <Button onClick={handleStartSession} disabled={startSessionMutation.isPending}>
                <Play className="mr-2 h-4 w-4" />
                Start Session
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  );
}
