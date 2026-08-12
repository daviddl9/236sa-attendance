import { createFileRoute, Navigate } from '@tanstack/react-router';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { AlertTriangle, Link2Off } from 'lucide-react';
import { toast } from 'sonner';
import DashboardLayout from '../../../components/dashboard/layout';
import { Badge } from '../../../components/ui/badge';
import { Button } from '../../../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../../components/ui/card';
import { useAuth } from '../../../lib/auth-context';
import { apiClient, type TelegramPairingAttemptRecord, type TelegramPairingRequestRecord } from '../../../lib/api-client';
import { canManageUnit } from '../../../lib/user-utils';

export const Route = createFileRoute('/dashboard/admin/telegram-pairings')({
  component: TelegramPairingsPage,
});

function TelegramPairingsPage() {
  const { user, isLoading } = useAuth();
  if (isLoading) return null;
  if (!canManageUnit(user)) return <Navigate to="/dashboard" />;
  return <TelegramPairingsContent />;
}

function TelegramPairingsContent() {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<Record<string, string>>({});
  const { data, isLoading } = useQuery({
    queryKey: ['telegram-pairings'],
    queryFn: () => apiClient.listTelegramPairings(),
    refetchInterval: 30_000,
  });

  const unpairMutation = useMutation({
    mutationFn: (telegramId: number) => apiClient.unpairTelegramAccount(telegramId),
    onSuccess: () => {
      toast.success('Telegram account unpaired');
      queryClient.invalidateQueries({ queryKey: ['telegram-pairings'] });
    },
    onError: () => toast.error('Failed to unpair Telegram account'),
  });
  const confirmMutation = useMutation({
    mutationFn: (input: { telegramId: number; userId: string }) => apiClient.confirmTelegramPairing(input.telegramId, input.userId),
    onSuccess: (_, input) => {
      toast.success('Pairing confirmed');
      setSelected((current) => {
        const next = { ...current };
        delete next[String(input.telegramId)];
        return next;
      });
      queryClient.invalidateQueries({ queryKey: ['telegram-pairings'] });
    },
    onError: () => toast.error('Pairing could not be confirmed'),
  });

  const attempts = data?.attempts ?? [];
  const attention = attempts.filter((attempt) => attempt.outcome === 'refused_conflict' || attempt.outcome === 'no_match');
  const routine = attempts.filter((attempt) => !attention.includes(attempt));

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Telegram Pairings</h1>
          <p className="text-muted-foreground">Review exceptions first, then inspect routine self-confirmed pairings.</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4" />
              Attention required
              {attention.length > 0 && <Badge variant="destructive">{attention.length}</Badge>}
            </CardTitle>
            <CardDescription>Conflicts and weak-match attempts are shown before routine activity.</CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? <p className="text-sm text-muted-foreground">Loading...</p> : attention.length === 0 ? (
              <p className="text-sm text-muted-foreground">No recent conflicts or refusals.</p>
            ) : (
              <AttemptList attempts={attention} />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Pending commander review</CardTitle>
            <CardDescription>Weak matches are never proposed to a soldier. Choose one roster row only after checking the request.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {data?.requests.length ? data.requests.map((request) => (
              <PairingRequest
                key={request.telegramId}
                request={request}
                selectedUserId={selected[String(request.telegramId)] ?? ''}
                onSelect={(userId) => setSelected((current) => ({ ...current, [String(request.telegramId)]: userId }))}
                onConfirm={() => confirmMutation.mutate({ telegramId: request.telegramId, userId: selected[String(request.telegramId)] })}
                confirming={confirmMutation.isPending}
              />
            )) : <p className="text-sm text-muted-foreground">No pending requests.</p>}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Recent pairing activity</CardTitle>
            <CardDescription>Every confirmed pairing is visible here. Unpairing does not remove attendance history.</CardDescription>
          </CardHeader>
          <CardContent>
            {data?.pairings.length ? (
              <div className="space-y-2">
                {data.pairings.map((pairing) => (
                  <div key={pairing.telegramId} className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3">
                    <div>
                      <p className="font-medium">{pairing.fullName || 'Unnamed roster row'}</p>
                      <p className="text-sm text-muted-foreground">Telegram {pairing.telegramId} · {pairing.selfConfirmed ? 'Self-confirmed' : `Confirmed by ${pairing.confirmedBy || 'commander'}`}</p>
                    </div>
                    <Button variant="outline" size="sm" onClick={() => unpairMutation.mutate(pairing.telegramId)} disabled={unpairMutation.isPending}>
                      <Link2Off className="mr-1 h-4 w-4" />
                      Unpair
                    </Button>
                  </div>
                ))}
              </div>
            ) : <p className="text-sm text-muted-foreground">No pairings yet.</p>}
            {routine.length > 0 && <div className="mt-4 border-t pt-4"><p className="mb-2 text-sm font-medium">Routine attempts</p><AttemptList attempts={routine} /></div>}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}

function AttemptList({ attempts }: { attempts: TelegramPairingAttemptRecord[] }) {
  return <div className="space-y-2">{attempts.map((attempt, index) => (
    <div key={`${attempt.telegramId}-${attempt.createdAt}-${index}`} className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-3 text-sm">
      <span>Telegram {attempt.telegramId}{attempt.fullName ? ` · ${attempt.fullName}` : ''}</span>
      <Badge variant={attempt.outcome === 'refused_conflict' ? 'destructive' : 'outline'}>{attempt.outcome.replace('_', ' ')}</Badge>
    </div>
  ))}</div>;
}

function PairingRequest({
  request,
  selectedUserId,
  onSelect,
  onConfirm,
  confirming,
}: {
  request: TelegramPairingRequestRecord;
  selectedUserId: string;
  onSelect: (userId: string) => void;
  onConfirm: () => void;
  confirming: boolean;
}) {
  return (
    <div className="rounded-md border p-4">
      <p className="font-medium">Telegram {request.telegramId}: {request.displayName}</p>
      <div className="mt-3 grid gap-2">
        {request.candidates.map((candidate) => (
          <button
            key={candidate.id}
            type="button"
            disabled={!candidate.selectable}
            onClick={() => onSelect(candidate.id)}
            className={`rounded-md border p-3 text-left ${selectedUserId === candidate.id ? 'border-primary bg-primary/5' : ''} ${!candidate.selectable ? 'cursor-not-allowed opacity-60' : ''}`}
          >
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium">{candidate.fullName}</span>
              <Badge variant={candidate.strong ? 'default' : 'outline'}>Score {candidate.score}</Badge>
            </div>
            <p className="text-sm text-muted-foreground">{candidate.rank} · {candidate.battery}</p>
            {candidate.alreadyClaimed && <Badge className="mt-1" variant="destructive">Already claimed</Badge>}
          </button>
        ))}
      </div>
      <div className="mt-3 flex justify-end">
        <Button onClick={onConfirm} disabled={!selectedUserId || confirming}>Confirm selected row</Button>
      </div>
    </div>
  );
}
