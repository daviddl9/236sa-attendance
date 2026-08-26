import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { apiClient, type OutDirection } from '../../lib/api-client';
import { Button } from '../../components/ui/button';
import { DoorOpen, DoorClosed, CheckCircle2, AlertTriangle } from 'lucide-react';

export const Route = createFileRoute('/out/$sessionId')({
  component: OutScanPage,
});

function timeOnly(iso: string) {
  return new Date(iso).toLocaleTimeString('en-SG', {
    hour: '2-digit', minute: '2-digit', hour12: false, timeZone: 'Asia/Singapore',
  });
}

/**
 * The gate confirm screen. A scan lands here rather than recording straight
 * away: the direction is stated in words and nothing is written until the
 * soldier confirms, so a double tap at the gate cannot silently record them
 * as back inside camp.
 */
function OutScanPage() {
  const { sessionId } = Route.useParams();
  const navigate = useNavigate();
  const [done, setDone] = useState<null | { direction: OutDirection }>(null);
  const [error, setError] = useState('');

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['out-self', sessionId],
    queryFn: () => apiClient.getOutSelfState(sessionId),
    retry: false,
  });


  const record = useMutation({
    mutationFn: (dir: OutDirection) => apiClient.recordOutMovement(sessionId, dir),
    onSuccess: (res) => {
      setDone({ direction: res.direction });
    },
    onError: async (e: Error) => {
      // A stale screen: re-read the truth and let them try again.
      if (e.message.toLowerCase().includes('changed')) {
        setError('Your status changed since this screen loaded. Check it and try again.');
        await refetch();
        return;
      }
      setError(e.message || 'Could not record your movement');
    },
  });

  if (isLoading) return <Centered><p className="text-muted-foreground">Loading…</p></Centered>;

  if (!data) {
    return (
      <Centered>
        <h1 className="text-xl font-semibold mb-2">Session unavailable</h1>
        <p className="text-muted-foreground mb-4">This Out session could not be found or has closed.</p>
        <Button onClick={() => navigate({ to: '/dashboard' })}>Go to dashboard</Button>
      </Centered>
    );
  }

  const { session, nextDirection, last, scanned } = data;

  if (done) {
    const goingOut = done.direction === 'out';
    return (
      <Centered>
        <CheckCircle2 className={`h-14 w-14 mb-4 ${goingOut ? 'text-amber-500' : 'text-green-600'}`} />
        <h1 className="text-2xl font-bold mb-1">
          {goingOut ? "You're marked OUT" : "You're marked IN"}
        </h1>
        <p className="text-muted-foreground mb-1">{session.name}</p>
        {goingOut && (
          <p className="text-sm mb-4">
            Expected back by <strong>{timeOnly(session.expectedReturnAt)}</strong>. Scan again when you return.
          </p>
        )}
        <Button variant="outline" onClick={() => navigate({ to: '/dashboard' })}>Done</Button>
      </Centered>
    );
  }

  if (!scanned) {
    return (
      <Centered>
        <AlertTriangle className="h-10 w-10 text-amber-500 mb-3" />
        <h1 className="text-xl font-semibold mb-2">Scan the gate QR code</h1>
        <p className="text-muted-foreground">
          Open this page by scanning the session's QR code at the guardhouse.
        </p>
      </Centered>
    );
  }

  const goingOut = nextDirection === 'out';

  return (
    <Centered>
      {goingOut ? <DoorOpen className="h-12 w-12 text-amber-500 mb-3" />
                : <DoorClosed className="h-12 w-12 text-green-600 mb-3" />}
      <p className="text-sm text-muted-foreground">{session.name} · {session.subtypeLabel}</p>
      <h1 className="text-2xl font-bold mt-1 mb-2">
        {goingOut ? 'Mark yourself OUT?' : 'Mark yourself IN?'}
      </h1>

      {last ? (
        <p className="text-muted-foreground mb-4">
          You scanned <strong>{last.direction === 'out' ? 'OUT' : 'IN'}</strong> at {timeOnly(last.occurredAt)}.
        </p>
      ) : (
        <p className="text-muted-foreground mb-4">
          Expected back by <strong>{timeOnly(session.expectedReturnAt)}</strong>.
        </p>
      )}

      {error && <p className="text-sm text-red-600 mb-3">{error}</p>}

      <Button
        size="lg"
        className="w-full max-w-xs"
        disabled={record.isPending}
        onClick={() => { setError(''); record.mutate(nextDirection); }}
      >
        {record.isPending ? 'Recording…' : goingOut ? 'Confirm — going OUT' : 'Confirm — coming IN'}
      </Button>
      <p className="text-xs text-muted-foreground mt-3">Nothing is recorded until you confirm.</p>
    </Centered>
  );
}

function Centered({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center text-center px-6">
      {children}
    </div>
  );
}
