import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { apiClient, type OutSession } from '../../../lib/api-client';
import { useAuth } from '../../../lib/auth-context';
import { canManageUnit } from '../../../lib/user-utils';
import { Button } from '../../../components/ui/button';
import { Plus, DoorOpen, AlertTriangle } from 'lucide-react';
import DashboardLayout from '../../../components/dashboard/layout';

export const Route = createFileRoute('/dashboard/out/')({
  component: OutSessionsPage,
});

function formatSgt(iso: string) {
  return new Date(iso).toLocaleString('en-SG', {
    day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit',
    hour12: false, timeZone: 'Asia/Singapore',
  });
}

function OutSessionsPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const [includeClosed, setIncludeClosed] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ['out-sessions', includeClosed],
    queryFn: () => apiClient.listOutSessions(includeClosed),
    refetchInterval: 30000,
  });

  const sessions: OutSession[] = data?.sessions ?? [];

  return (
    <DashboardLayout>
    <div className="p-8">
      <div className="flex items-start justify-between mb-6">
        <div>
          <h1 className="text-3xl font-bold">Out</h1>
          <p className="text-muted-foreground">
            Nights Out, Stay Out and Off Pass — soldiers scan themselves out and back in
          </p>
        </div>
        {canManageUnit(user) && (
          <Button onClick={() => navigate({ to: '/dashboard/out/create' })}>
            <Plus className="h-4 w-4 mr-2" /> Create Out Session
          </Button>
        )}
      </div>

      <div className="inline-flex rounded-lg bg-muted p-1 mb-6">
        <button
          onClick={() => setIncludeClosed(false)}
          className={`px-3 py-1.5 text-sm rounded-md ${!includeClosed ? 'bg-background shadow' : ''}`}
        >
          Active
        </button>
        <button
          onClick={() => setIncludeClosed(true)}
          className={`px-3 py-1.5 text-sm rounded-md ${includeClosed ? 'bg-background shadow' : ''}`}
        >
          All Sessions
        </button>
      </div>

      {isLoading ? (
        <p className="text-muted-foreground">Loading…</p>
      ) : sessions.length === 0 ? (
        <div className="text-center py-16">
          <DoorOpen className="h-10 w-10 mx-auto text-muted-foreground mb-3" />
          <p className="text-muted-foreground mb-4">No Out sessions found</p>
          {canManageUnit(user) && (
            <Button onClick={() => navigate({ to: '/dashboard/out/create' })}>
              Create First Out Session
            </Button>
          )}
        </div>
      ) : (
        <div className="grid gap-3">
          {sessions.map((s) => {
            const overdue = s.status === 'active' && new Date(s.expectedReturnAt) < new Date();
            return (
              <button
                key={s.id}
                onClick={() => navigate({ to: '/dashboard/out/$sessionId', params: { sessionId: s.id } })}
                className="text-left border rounded-lg p-4 hover:bg-accent transition-colors"
              >
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-semibold text-lg">{s.name}</span>
                  <span className="text-xs px-2 py-0.5 rounded bg-muted">{s.subtypeLabel}</span>
                  <span
                    className={`text-xs px-2 py-0.5 rounded ${
                      s.status === 'active' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-700'
                    }`}
                  >
                    {s.status === 'active' ? 'ACTIVE' : 'CLOSED'}
                  </span>
                  {overdue && (
                    <span className="text-xs px-2 py-0.5 rounded bg-red-100 text-red-800 inline-flex items-center gap-1">
                      <AlertTriangle className="h-3 w-3" /> Past return time
                    </span>
                  )}
                </div>
                <p className="text-sm text-muted-foreground mt-1">
                  Started {formatSgt(s.startTime)} · Expected back {formatSgt(s.expectedReturnAt)}
                </p>
              </button>
            );
          })}
        </div>
      )}
    </div>
    </DashboardLayout>
  );
}
