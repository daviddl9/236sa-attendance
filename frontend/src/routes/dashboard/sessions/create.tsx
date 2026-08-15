import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient, type ParticipantMatch, type UnmatchedRow } from '../../../lib/api-client';
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
import { Switch } from '../../../components/ui/switch';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { Badge } from '../../../components/ui/badge';
import { useState, useRef } from 'react';
import { ArrowLeft, Save, Upload, Users, AlertCircle } from 'lucide-react';
import { toast } from 'sonner';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/dashboard/sessions/create')({
  component: CreateSessionPage,
});

function CreateSessionPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [name, setName] = useState('');
  const [scope, setScope] = useState('unit_wide');
  const [batteries, setBatteries] = useState<string[]>([]);

  // Custom list state
  const [previewLoading, setPreviewLoading] = useState(false);
  const [matched, setMatched] = useState<ParticipantMatch[]>([]);
  const [unmatched, setUnmatched] = useState<UnmatchedRow[]>([]);
  const [previewDone, setPreviewDone] = useState(false);
  const [listSource, setListSource] = useState<'excel' | 'group'>('excel');
  const [selectedGroupId, setSelectedGroupId] = useState('');

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: () => apiClient.listGroups(),
  });
  const groups = groupsData?.groups ?? [];

  const createMutation = useMutation({
    mutationFn: (data: { name: string; scope: string; batteries?: string[] }) =>
      apiClient.createSession(data),
    onSuccess: (data) => {
      toast.success('Session created');
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      navigate({ to: '/dashboard/sessions/$sessionId', params: { sessionId: data.id } });
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to create session'),
  });

  const createCustomMutation = useMutation({
    mutationFn: () =>
      apiClient.createCustomSession({
        name,
        participantIds: matched.map((m) => m.userId),
      }),
    onSuccess: (data) => {
      toast.success('Session created');
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      navigate({ to: '/dashboard/sessions/$sessionId', params: { sessionId: data.id } });
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to create session'),
  });

  const createFromGroupMutation = useMutation({
    mutationFn: (groupId: string) =>
      apiClient.createSessionFromGroup(groupId, { name, participantIds: [] }),
    onSuccess: (data) => {
      toast.success('Session created from group');
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      navigate({ to: '/dashboard/sessions/$sessionId', params: { sessionId: data.id } });
    },
    onError: (error: Error) => toast.error(error.message || 'Failed to create session'),
  });

  const handleBatteryChange = (battery: string, checked: boolean) => {
    setBatteries(checked ? [...batteries, battery] : batteries.filter((b) => b !== battery));
  };

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setPreviewLoading(true);
    setPreviewDone(false);
    setMatched([]);
    setUnmatched([]);

    try {
      const result = await apiClient.previewCustomSession(file);
      setMatched(result.matched);
      setUnmatched(result.unmatched);
      setPreviewDone(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to parse file');
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    if (!name) {
      toast.error('Session name is required');
      return;
    }

    if (scope === 'battery_specific' && batteries.length === 0) {
      toast.error('Please select at least one battery');
      return;
    }

    if (scope === 'custom_list') {
      if (listSource === 'group') {
        if (!selectedGroupId) {
          toast.error('Please pick a saved group');
          return;
        }
        createFromGroupMutation.mutate(selectedGroupId);
        return;
      }
      if (!previewDone || matched.length === 0) {
        toast.error('Please upload and preview a participant list first');
        return;
      }
      if (unmatched.length > 0) {
        toast.error('Resolve all unmatched rows before creating the session');
        return;
      }
      createCustomMutation.mutate();
      return;
    }

    createMutation.mutate({
      name,
      scope,
      batteries: scope === 'battery_specific' ? batteries : undefined,
    });
  };

  const isPending =
    createMutation.isPending || createCustomMutation.isPending || createFromGroupMutation.isPending;

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center gap-4">
          <Link to="/dashboard/sessions">
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Create Session</h1>
            <p className="text-muted-foreground">Create a new attendance session</p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Session Details</CardTitle>
            <CardDescription>Fill in the session information</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid gap-2">
                <Label htmlFor="name">Session Name *</Label>
                <Input
                  id="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g., Monday Morning Formation"
                  required
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="scope">Scope *</Label>
                <Select
                  value={scope}
                  onValueChange={(value) => {
                    setScope(value);
                    if (value !== 'battery_specific') setBatteries([]);
                    if (value !== 'custom_list') {
                      setPreviewDone(false);
                      setMatched([]);
                      setUnmatched([]);
                      setSelectedGroupId('');
                    }
                  }}
                >
                  <SelectTrigger id="scope">
                    <SelectValue placeholder="Select scope" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="unit_wide">Unit Wide</SelectItem>
                    <SelectItem value="battery_specific">Battery Specific</SelectItem>
                    <SelectItem value="custom_list">Custom List</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {scope === 'battery_specific' && (
                <div className="grid gap-2">
                  <Label>Batteries *</Label>
                  <div className="space-y-2">
                    {['HQ', 'Alpha', 'Bravo'].map((battery) => (
                      <div key={battery} className="flex items-center gap-2">
                        <Switch
                          id={`battery-${battery}`}
                          checked={batteries.includes(battery)}
                          onCheckedChange={(checked) => handleBatteryChange(battery, checked)}
                        />
                        <Label htmlFor={`battery-${battery}`} className="cursor-pointer">
                          {battery}
                        </Label>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {scope === 'custom_list' && (
                <div className="grid gap-3">
                  <Label>Participant List *</Label>

                  <div className="flex gap-2">
                    <Button
                      type="button"
                      variant={listSource === 'excel' ? 'default' : 'outline'}
                      onClick={() => setListSource('excel')}
                      className="gap-2"
                    >
                      <Upload className="h-4 w-4" />
                      Upload Excel
                    </Button>
                    <Button
                      type="button"
                      variant={listSource === 'group' ? 'default' : 'outline'}
                      onClick={() => setListSource('group')}
                      className="gap-2"
                    >
                      <Users className="h-4 w-4" />
                      Saved Group
                    </Button>
                  </div>

                  {listSource === 'excel' ? (
                    <>
                      <p className="text-xs text-muted-foreground -mt-1">
                        Upload an Excel file with a <strong>Full Name</strong> column (and optionally <strong>NRIC Last 5</strong>).
                      </p>
                      <div className="flex gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          onClick={() => fileInputRef.current?.click()}
                          disabled={previewLoading}
                          className="gap-2"
                        >
                          <Upload className="h-4 w-4" />
                          {previewLoading ? 'Parsing...' : 'Upload Excel'}
                        </Button>
                        <input
                          ref={fileInputRef}
                          type="file"
                          accept=".xlsx,.xls"
                          className="hidden"
                          onChange={handleFileChange}
                        />
                        {previewDone && (
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Users className="h-4 w-4" />
                            {matched.length} matched
                            {unmatched.length > 0 && (
                              <span className="text-destructive font-medium">
                                · {unmatched.length} unmatched
                              </span>
                            )}
                          </div>
                        )}
                      </div>
                    </>
                  ) : (
                    <div className="grid gap-2">
                      <p className="text-xs text-muted-foreground -mt-1">
                        Pick a saved group to reuse its roster.
                      </p>
                      <Select value={selectedGroupId} onValueChange={setSelectedGroupId}>
                        <SelectTrigger>
                          <SelectValue placeholder="Select a group" />
                        </SelectTrigger>
                        <SelectContent>
                          {groups.length === 0 && (
                            <div className="px-2 py-1.5 text-sm text-muted-foreground">
                              No saved groups yet.
                            </div>
                          )}
                          {groups.map((g) => (
                            <SelectItem key={g.id} value={g.id}>
                              {g.name} ({g.memberCount ?? 0})
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {selectedGroupId && (
                        <p className="text-xs text-muted-foreground">
                          This session will include all members of the selected group.
                        </p>
                      )}
                    </div>
                  )}

                  {listSource === 'excel' && previewDone && matched.length > 0 && (
                    <div className="rounded-md border p-3 space-y-1 max-h-48 overflow-y-auto">
                      <p className="text-xs font-medium text-muted-foreground mb-2">
                        Matched participants ({matched.length})
                      </p>
                      {matched.map((m) => (
                        <div key={m.userId} className="flex items-center gap-2 text-sm">
                          <Badge variant="outline" className="text-xs shrink-0">
                            {m.rank ?? '—'}
                          </Badge>
                          <span>{m.fullName}</span>
                          {m.battery && (
                            <span className="text-muted-foreground text-xs">{m.battery}</span>
                          )}
                        </div>
                      ))}
                    </div>
                  )}

                  {listSource === 'excel' && previewDone && unmatched.length > 0 && (
                    <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 space-y-1">
                      <div className="flex items-center gap-2 text-sm font-medium text-destructive mb-2">
                        <AlertCircle className="h-4 w-4" />
                        Unmatched rows — resolve before creating
                      </div>
                      {unmatched.map((u) => (
                        <div key={u.rowNum} className="text-sm text-destructive/80">
                          Row {u.rowNum}: <strong>{u.fullName}</strong> — {u.reason}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              <div className="flex justify-end gap-2 pt-4">
                <Button
                  type="submit"
                  disabled={
                    isPending ||
                    (scope === 'custom_list' &&
                      (listSource === 'group'
                        ? !selectedGroupId
                        : !previewDone || unmatched.length > 0))
                  }
                >
                  <Save className="mr-2 h-4 w-4" />
                  {isPending ? 'Creating...' : 'Create Session'}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}
