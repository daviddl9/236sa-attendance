import { createFileRoute, Navigate } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Fragment, useState } from 'react';
import { apiClient, type PendingRegistration, type RegistrationCandidate } from '../../../../lib/api-client';
import DashboardLayout from '../../../../components/dashboard/layout';
import { Button } from '../../../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../../components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../../../components/ui/table';
import { Badge } from '../../../../components/ui/badge';
import { toast } from 'sonner';
import { Check, X, Clock } from 'lucide-react';
import { useAuth } from '../../../../lib/auth-context';
import { isSuperadmin } from '../../../../lib/user-utils';

export const Route = createFileRoute('/dashboard/admin/registrations/')({
  component: RegistrationsPage,
});

function RegistrationsPage() {
  const { user, isLoading } = useAuth();

  // user is null while the session is still loading, so redirecting on that
  // alone bounces a genuine commander who opened this page directly or
  // refreshed it. Wait for the session to resolve before deciding.
  if (isLoading) {
    return null;
  }

  if (!isSuperadmin(user)) {
    return <Navigate to="/dashboard" />;
  }

  return <RegistrationsContent />;
}

function RegistrationsContent() {
  const queryClient = useQueryClient();
  const [reviewId, setReviewId] = useState<string | null>(null);
  const [selectedUserId, setSelectedUserId] = useState('');
  const [battery, setBattery] = useState('');
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [bulkResults, setBulkResults] = useState<{ id: string; success: boolean; error?: string }[] | null>(null);
  const [createConfirmation, setCreateConfirmation] = useState<{ id: string; candidate: RegistrationCandidate } | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['pending-registrations', battery],
    queryFn: () => apiClient.listPendingRegistrations(battery || undefined),
    refetchInterval: 30_000,
  });

  const { data: candidateData, isLoading: candidatesLoading } = useQuery({
    queryKey: ['registration-candidates', reviewId],
    queryFn: () => apiClient.listRegistrationCandidates(reviewId!),
    enabled: reviewId !== null,
  });

  const candidates = candidateData?.candidates ?? [];
  const strongCandidates = candidates.filter((candidate) => candidate.strong);
  const weakCandidates = candidates.filter((candidate) => !candidate.strong);
  const suggestedUserId = strongCandidates.find((candidate) => candidate.preselected && candidate.selectable)?.id ?? '';
  const activeUserId = selectedUserId || suggestedUserId;

  const renderCandidate = (candidate: RegistrationCandidate) => (
    <button
      key={candidate.id}
      type="button"
      disabled={!candidate.selectable}
      onClick={() => setSelectedUserId(candidate.id)}
      className={`rounded-md border p-3 text-left ${activeUserId === candidate.id ? 'border-primary bg-primary/5' : ''} ${!candidate.strong ? 'border-dashed bg-muted/30' : ''} ${!candidate.selectable ? 'cursor-not-allowed opacity-60' : ''}`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{candidate.fullName}</span>
        <Badge variant={candidate.strong ? 'default' : 'outline'}>Score {candidate.score}</Badge>
      </div>
      <p className="text-sm text-muted-foreground">{candidate.rank} · {candidate.battery}</p>
      <div className="mt-1 flex flex-wrap gap-1">
        {!candidate.strong && <Badge variant="outline">Weaker match</Badge>}
        {candidate.mismatchReasons.map((reason) => <Badge key={reason} variant="outline">{reason}</Badge>)}
        {candidate.strong && candidate.preselected && <Badge variant="secondary">Suggested</Badge>}
        {candidate.alreadyClaimed && <Badge variant="destructive">Already claimed{candidate.claimedBy ? ` by @${candidate.claimedBy}` : ''}</Badge>}
      </div>
    </button>
  );

  const approveMutation = useMutation({
    mutationFn: (input: { id: string; mode: 'link' | 'create'; userId?: string; acknowledgeStrongMatch?: boolean }) => {
      if (input.mode === 'link') {
        if (!input.userId) throw new Error('Select a roster candidate');
        return apiClient.approveRegistration(input.id, { mode: 'link', userId: input.userId });
      }
      return apiClient.approveRegistration(input.id, {
        mode: 'create',
        acknowledgeStrongMatch: input.acknowledgeStrongMatch,
      });
    },
    onSuccess: (_, { id }) => {
      toast.success('Registration approved');
      setCreateConfirmation(null);
      setReviewId(null);
      setSelectedUserId('');
      setSelectedIds((current) => current.filter((value) => value !== id));
      queryClient.setQueryData(['pending-registrations', battery], (old: { registrations: PendingRegistration[]; total: number } | undefined) => {
        if (!old) return old;
        const registrations = old.registrations.filter((r) => r.id !== id);
        return { registrations, total: registrations.length };
      });
    },
    onError: () => toast.error('Failed to approve registration'),
  });

  const requestCreate = (id: string) => {
    const candidate = strongCandidates[0];
    if (candidate) {
      setCreateConfirmation({ id, candidate });
      return;
    }
    approveMutation.mutate({ id, mode: 'create' });
  };

  const rejectMutation = useMutation({
    mutationFn: (id: string) => apiClient.rejectRegistration(id),
    onSuccess: (_, id) => {
      toast.success('Registration rejected');
      setSelectedIds((current) => current.filter((value) => value !== id));
      queryClient.setQueryData(['pending-registrations', battery], (old: { registrations: PendingRegistration[]; total: number } | undefined) => {
        if (!old) return old;
        const registrations = old.registrations.filter((r) => r.id !== id);
        return { registrations, total: registrations.length };
      });
    },
    onError: () => toast.error('Failed to reject registration'),
  });

  const registrations = data?.registrations ?? [];
  const allSelected = registrations.length > 0 && registrations.every((registration) => selectedIds.includes(registration.id));
  const toggleSelected = (id: string) => {
    setSelectedIds((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id]);
  };
  const toggleAll = () => {
    setSelectedIds(allSelected ? [] : registrations.map((registration) => registration.id));
  };

  const bulkApproveMutation = useMutation({
    mutationFn: () => apiClient.bulkApproveRegistrations(selectedIds),
    onSuccess: (result) => {
      setBulkResults(result.results);
      const approvedIds = new Set(result.results.filter((item) => item.success).map((item) => item.id));
      setSelectedIds((current) => current.filter((id) => !approvedIds.has(id)));
      queryClient.invalidateQueries({ queryKey: ['pending-registrations'] });
      toast.success(`${result.approved} approved; ${result.failed} failed`);
    },
    onError: (error: Error) => toast.error(error.message || 'Bulk approval failed'),
  });

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Pending Registrations</h1>
          <p className="text-muted-foreground">
            Review and approve self-registered users before they can access the system.
          </p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Awaiting Approval
              {registrations.length > 0 && (
                <Badge variant="destructive" className="ml-1">
                  {registrations.length}
                </Badge>
              )}
            </CardTitle>
            <CardDescription>
              These users registered themselves and are not on the pre-loaded personnel list.
            </CardDescription>
            <div className="flex flex-wrap items-center gap-3 pt-3">
              <label className="text-sm font-medium" htmlFor="battery-filter">Battery</label>
              <select id="battery-filter" className="rounded-md border bg-background px-3 py-2 text-sm" value={battery} onChange={(event) => { setBattery(event.target.value); setSelectedIds([]); }}>
                <option value="">All batteries</option>
                <option value="HQ">HQ</option>
                <option value="Alpha">Alpha</option>
                <option value="Bravo">Bravo</option>
              </select>
              {selectedIds.length > 0 && (
                <div className="flex items-center gap-2">
                  <Button size="sm" onClick={() => bulkApproveMutation.mutate()} disabled={bulkApproveMutation.isPending}>
                    {bulkApproveMutation.isPending ? 'Approving...' : `Bulk approve (${selectedIds.length})`}
                  </Button>
                  <span className="text-xs text-muted-foreground">Bulk approve only creates new people. Likely matches are refused and must be reviewed individually to link them.</span>
                </div>
              )}
            </div>
          </CardHeader>
          <CardContent>
            {bulkResults && (
              <div className="mb-4 rounded-md border p-3 text-sm">
                <p className="font-medium">Bulk approval results</p>
                <ul className="mt-2 space-y-1">
                  {bulkResults.map((result) => (
                    <li key={result.id} className={result.success ? 'text-green-700' : 'text-destructive'}>
                      {result.success ? 'Approved' : `Failed: ${result.error || 'Unknown error'}`} — {result.id}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {isLoading ? (
              <p className="text-sm text-muted-foreground py-4 text-center">Loading...</p>
            ) : registrations.length === 0 ? (
              <p className="text-sm text-muted-foreground py-8 text-center">
                No pending registrations.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>
                      <input type="checkbox" aria-label="Select all registrations" checked={allSelected} onChange={toggleAll} />
                    </TableHead>
                    <TableHead>Full Name</TableHead>
                    <TableHead>Rank</TableHead>
                    <TableHead>Battery</TableHead>
                    <TableHead>Submitted</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {registrations.map((reg) => (
                    <Fragment key={reg.id}>
                      <TableRow>
                        <TableCell>
                          <input type="checkbox" aria-label={`Select ${reg.fullName}`} checked={selectedIds.includes(reg.id)} onChange={() => toggleSelected(reg.id)} />
                        </TableCell>
                        <TableCell className="font-medium">{reg.fullName ?? '—'}</TableCell>
                        <TableCell>{reg.rank ?? '—'}</TableCell>
                        <TableCell>{reg.battery ?? '—'}</TableCell>
                        <TableCell className="text-muted-foreground text-sm">
                          {new Date(reg.createdAt).toLocaleDateString()}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => { setReviewId(reg.id); setSelectedUserId(''); }}
                              disabled={approveMutation.isPending || rejectMutation.isPending}
                            >
                              <Check className="h-4 w-4 mr-1" />
                              Review
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              className="text-destructive border-destructive/30 hover:bg-destructive/5"
                              onClick={() => rejectMutation.mutate(reg.id)}
                              disabled={approveMutation.isPending || rejectMutation.isPending}
                            >
                              <X className="h-4 w-4 mr-1" />
                              Reject
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                      {reviewId === reg.id && (
                        <TableRow>
                          <TableCell colSpan={6}>
                            <div className="grid gap-3 rounded-md bg-muted/40 p-4">
                              <p className="font-medium">Roster candidates</p>
                              {candidatesLoading ? <p className="text-sm text-muted-foreground">Finding matches...</p> : (
                                <>
                                  {strongCandidates.length === 0 && (
                                    <p className="text-sm text-amber-700">No close matches — check weaker matches before creating a new person.</p>
                                  )}
                                  {strongCandidates.length > 0 && <div className="grid gap-2">{strongCandidates.map(renderCandidate)}</div>}
                                  {weakCandidates.length > 0 && (
                                    <details className="rounded-md border border-dashed p-3">
                                      <summary className="cursor-pointer font-medium">Show weaker matches ({weakCandidates.length})</summary>
                                      <div className="mt-2 grid gap-2">{weakCandidates.map(renderCandidate)}</div>
                                    </details>
                                  )}
                                </>
                              )}
                              {createConfirmation?.id === reg.id && (
                                <div role="alert" className="rounded-md border border-amber-500/50 bg-amber-50 p-3 text-sm">
                                  <p>
                                    A close roster match was found: <strong>{createConfirmation.candidate.fullName}</strong> (score {createConfirmation.candidate.score}).
                                    Only continue if this registration is a different person.
                                  </p>
                                  <div className="mt-3 flex justify-end gap-2">
                                    <Button variant="outline" onClick={() => setCreateConfirmation(null)} disabled={approveMutation.isPending}>
                                      Cancel
                                    </Button>
                                    <Button onClick={() => { approveMutation.mutate({ id: reg.id, mode: 'create', acknowledgeStrongMatch: true }); setCreateConfirmation(null); }} disabled={approveMutation.isPending}>
                                      I confirm — create roster row
                                    </Button>
                                  </div>
                                </div>
                              )}
                              <div className="flex justify-end gap-2">
                                <Button variant="outline" onClick={() => requestCreate(reg.id)} disabled={approveMutation.isPending || candidatesLoading}>
                                  Create new roster row
                                </Button>
                                <Button onClick={() => approveMutation.mutate({ id: reg.id, mode: 'link', userId: activeUserId })} disabled={!activeUserId || approveMutation.isPending}>
                                  Link selected row
                                </Button>
                              </div>
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}
