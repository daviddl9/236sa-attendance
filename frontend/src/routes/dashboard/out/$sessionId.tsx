import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { apiClient, API_URL, type OutMemberState } from '../../../lib/api-client';
import { useAuth } from '../../../lib/auth-context';
import { canManageUnit } from '../../../lib/user-utils';
import { Button } from '../../../components/ui/button';
import { ArrowLeft, AlertTriangle, DoorOpen, DoorClosed, XCircle } from 'lucide-react';
import DashboardLayout from '../../../components/dashboard/layout';

export const Route = createFileRoute('/dashboard/out/$sessionId')({
  component: OutBoardPage,
});

function formatSgt(iso: string) {
  return new Date(iso).toLocaleString('en-SG', {
    day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit',
    hour12: false, timeZone: 'Asia/Singapore',
  });
}

function timeOnly(iso: string) {
  return new Date(iso).toLocaleTimeString('en-SG', {
    hour: '2-digit', minute: '2-digit', hour12: false, timeZone: 'Asia/Singapore',
  });
}

function OutBoardPage() {
  const { sessionId } = Route.useParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const qc = useQueryClient();
  const [notice, setNotice] = useState('');

  const { data: board, isLoading } = useQuery({
    queryKey: ['out-board', sessionId],
    queryFn: () => apiClient.getOutBoard(sessionId),
    refetchInterval: 5000,
  });

  // Live updates; polling above is the backstop if the stream drops.
  useEffect(() => {
    const src = new EventSource(`${API_URL}/api/sessions/${sessionId}/stream`, { withCredentials: true });
    const refresh = () => qc.invalidateQueries({ queryKey: ['out-board', sessionId] });
    src.onmessage = (e) => {
      try {
        const ev = JSON.parse(e.data);
        if (ev.type === 'out_movement' || ev.type === 'session_closed') refresh();
      } catch { /* ignore malformed frame */ }
    };
    return () => src.close();
  }, [sessionId, qc]);

  const close = useMutation({
    mutationFn: (ack: boolean) => apiClient.closeOutSession(sessionId, ack),
    onSuccess: () => { setNotice('Session closed'); qc.invalidateQueries({ queryKey: ['out-board', sessionId] }); },
    onError: async (e: Error) => {
      if (e.message.includes('still') || e.message.includes('scanned back')) {
        if (window.confirm('Some people have not scanned back in. Close the session anyway?')) {
          close.mutate(true);
          return;
        }
        return;
      }
      setNotice(e.message);
    },
  });

  // Records the next movement on someone's behalf when they forgot to scan.
  // The server derives the direction, so this reads as "mark returned" for
  // anyone currently out.
  const markReturned = useMutation({
    mutationFn: (m: OutMemberState) =>
      apiClient.recordOutMovementManually(sessionId, m.userId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['out-board', sessionId] }),
    onError: (e: Error) => setNotice(e.message || 'Could not record that movement'),
  });

  if (isLoading || !board) return <DashboardLayout><div className="p-8 text-muted-foreground">Loading…</div></DashboardLayout>;

  const { session, members, outCount, returnedCount, overdueCount } = board;

  // Same convention as the attendance session QR: point at the deployed
  // dashboard with the opaque session:secret token.
  const attendanceOrigin = 'https://236sa-attendance.ddl-tdh.workers.dev';
  const outQrUrl = session.qrCode ? `${attendanceOrigin}/qr/${session.qrCode}` : '';
  const out = members.filter((m) => m.direction === 'out');
  const back = members.filter((m) => m.direction === 'in');

  return (
    <DashboardLayout>
    <div className="p-8">
      <div className="flex items-center gap-3 mb-6 flex-wrap">
        <button onClick={() => navigate({ to: '/dashboard/out' })} aria-label="Back">
          <ArrowLeft className="h-5 w-5" />
        </button>
        <h1 className="text-2xl font-bold">{session.name}</h1>
        <span className="text-xs px-2 py-0.5 rounded bg-muted">{session.subtypeLabel}</span>
        <span className={`text-xs px-2 py-0.5 rounded ${
          session.status === 'active' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-700'}`}>
          {session.status === 'active' ? 'ACTIVE' : 'CLOSED'}
        </span>
        {session.status === 'active' && canManageUnit(user) && (
          <Button variant="destructive" className="ml-auto" onClick={() => close.mutate(false)}>
            Close Session
          </Button>
        )}
      </div>

      {notice && <p className="mb-4 text-sm text-muted-foreground">{notice}</p>}

      <div className="grid gap-4 md:grid-cols-4 mb-6">
        <Stat label="Currently out" value={outCount} tone="out" />
        <Stat label="Returned" value={returnedCount} tone="in" />
        <Stat label="Overdue" value={overdueCount} tone={overdueCount ? 'alarm' : 'muted'} />
        <div className="border rounded-lg p-4">
          <p className="text-xs uppercase tracking-wide text-muted-foreground">Expected back</p>
          <p className="text-lg font-semibold mt-1">{formatSgt(session.expectedReturnAt)}</p>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        {session.qrCode && (
          <div className="border rounded-lg p-5">
            <h2 className="font-semibold mb-1">Gate QR code</h2>
            <p className="text-sm text-muted-foreground mb-3">
              Scan on the way out, and again on return.
            </p>
            <div className="flex justify-center">
              <QRCodeSVG value={outQrUrl} size={256} level="H" data-qr-code />
            </div>
          </div>
        )}

        <div className="lg:col-span-2 space-y-6">
          <MemberList
            title="Still out"
            icon={<DoorOpen className="h-4 w-4" />}
            members={out}
            empty="Nobody is out."
            canCorrect={canManageUnit(user) && session.status === 'active'}
            actionLabel="Mark returned"
            onAction={(m) => markReturned.mutate(m)}
          />
          <MemberList
            title="Returned"
            icon={<DoorClosed className="h-4 w-4" />}
            members={back}
            empty="Nobody has scanned back in yet."
            canCorrect={false}
          />
        </div>
      </div>
    </div>
    </DashboardLayout>
  );
}

function Stat({ label, value, tone }: { label: string; value: number; tone: 'out' | 'in' | 'alarm' | 'muted' }) {
  const colour =
    tone === 'alarm' ? 'text-red-600' : tone === 'out' ? 'text-amber-600' : tone === 'in' ? 'text-green-700' : '';
  return (
    <div className="border rounded-lg p-4">
      <p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className={`text-3xl font-bold mt-1 ${colour}`}>{value}</p>
    </div>
  );
}

function MemberList({
  title, icon, members, empty, canCorrect, actionLabel, onAction,
}: {
  title: string;
  icon: React.ReactNode;
  members: OutMemberState[];
  empty: string;
  canCorrect: boolean;
  actionLabel?: string;
  onAction?: (m: OutMemberState) => void;
}) {
  return (
    <div className="border rounded-lg p-5">
      <h2 className="font-semibold mb-3 inline-flex items-center gap-2">{icon}{title} ({members.length})</h2>
      {members.length === 0 ? (
        <p className="text-sm text-muted-foreground">{empty}</p>
      ) : (
        <ul className="divide-y">
          {members.map((m) => (
            <li key={m.userId} className="py-2 flex items-center gap-3 flex-wrap">
              <span className="font-medium">{m.rank} {m.fullName}</span>
              <span className="text-xs px-1.5 py-0.5 rounded bg-muted">{m.battery}</span>
              {m.overdue && (
                <span className="text-xs px-2 py-0.5 rounded bg-red-100 text-red-800 inline-flex items-center gap-1">
                  <AlertTriangle className="h-3 w-3" /> Overdue
                </span>
              )}
              <span className="text-sm text-muted-foreground ml-auto">
                {m.direction === 'out' ? 'Out since' : 'Back at'} {timeOnly(m.occurredAt)}
                {m.method === 'manual' && ' · recorded by commander'}
              </span>
              {canCorrect && onAction && (
                <button
                  onClick={() => onAction(m)}
                  className="text-xs inline-flex items-center gap-1 border rounded px-2 py-1 hover:bg-accent"
                >
                  <XCircle className="h-3 w-3" /> {actionLabel}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
