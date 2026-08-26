import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { apiClient, type OutSubtype } from '../../../lib/api-client';
import { Button } from '../../../components/ui/button';
import { ArrowLeft, Save } from 'lucide-react';
import DashboardLayout from '../../../components/dashboard/layout';

export const Route = createFileRoute('/dashboard/out/create')({
  component: CreateOutSessionPage,
});

// Renders an instant as the value a datetime-local input expects, in SGT, so
// the commander edits Singapore wall-clock time rather than their own.
function toSgtLocalInput(iso: string): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Singapore',
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hour12: false,
  }).formatToParts(new Date(iso));
  const get = (t: string) => parts.find((p) => p.type === t)?.value ?? '00';
  return `${get('year')}-${get('month')}-${get('day')}T${get('hour')}:${get('minute')}`;
}

// Converts an SGT wall-clock value back to an absolute instant.
function fromSgtLocalInput(value: string): string {
  return new Date(`${value}:00+08:00`).toISOString();
}

function CreateOutSessionPage() {
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [subtype, setSubtype] = useState<OutSubtype>('nights_out');
  const [returnAt, setReturnAt] = useState('');
  const [touchedReturn, setTouchedReturn] = useState(false);
  const [error, setError] = useState('');

  const { data } = useQuery({
    queryKey: ['out-subtypes'],
    queryFn: () => apiClient.listOutSubtypes(),
  });
  const options = data?.subtypes ?? [];

  // Follow the sub-type's default until the commander edits the field.
  useEffect(() => {
    if (touchedReturn) return;
    const opt = options.find((o) => o.subtype === subtype);
    if (opt) setReturnAt(toSgtLocalInput(opt.defaultReturnAt));
  }, [subtype, options, touchedReturn]);

  const create = useMutation({
    mutationFn: () =>
      apiClient.createOutSession({
        name,
        subtype,
        expectedReturnAt: returnAt ? fromSgtLocalInput(returnAt) : undefined,
      }),
    onSuccess: (session) =>
      navigate({ to: '/dashboard/out/$sessionId', params: { sessionId: session.id } }),
    onError: (e: Error) => setError(e.message || 'Failed to create Out session'),
  });

  return (
    <DashboardLayout>
    <div className="p-8 max-w-3xl">
      <div className="flex items-center gap-3 mb-6">
        <button onClick={() => navigate({ to: '/dashboard/out' })} aria-label="Back">
          <ArrowLeft className="h-5 w-5" />
        </button>
        <div>
          <h1 className="text-3xl font-bold">Create Out Session</h1>
          <p className="text-muted-foreground">Soldiers enrol themselves by scanning at the gate</p>
        </div>
      </div>

      <div className="border rounded-lg p-6 space-y-5">
        <div>
          <label className="block text-sm font-medium mb-1" htmlFor="out-name">
            Session Name *
          </label>
          <input
            id="out-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g., Nights Out 24 Aug"
            className="w-full border rounded-md px-3 py-2"
          />
        </div>

        <div>
          <span className="block text-sm font-medium mb-2">Type *</span>
          <div className="grid gap-2 sm:grid-cols-3">
            {(options.length ? options : [{ subtype: 'nights_out' as OutSubtype, label: 'Nights Out', defaultReturnAt: '' }]).map((o) => (
              <button
                key={o.subtype}
                type="button"
                onClick={() => { setSubtype(o.subtype); setTouchedReturn(false); }}
                className={`border rounded-md px-3 py-2 text-sm text-left ${
                  subtype === o.subtype ? 'border-primary bg-accent font-medium' : ''
                }`}
              >
                {o.label}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium mb-1" htmlFor="out-return">
            Expected return (SGT) *
          </label>
          <input
            id="out-return"
            type="datetime-local"
            value={returnAt}
            onChange={(e) => { setReturnAt(e.target.value); setTouchedReturn(true); }}
            className="border rounded-md px-3 py-2"
          />
          <p className="text-xs text-muted-foreground mt-1">
            Anyone who has not scanned back in by this time is shown as overdue.
          </p>
        </div>

        {error && <p className="text-sm text-red-600">{error}</p>}

        <div className="flex justify-end">
          <Button onClick={() => { setError(''); create.mutate(); }} disabled={!name.trim() || create.isPending}>
            <Save className="h-4 w-4 mr-2" />
            {create.isPending ? 'Creating…' : 'Create Out Session'}
          </Button>
        </div>
      </div>
    </div>
    </DashboardLayout>
  );
}
