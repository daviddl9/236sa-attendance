import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { AlertTriangle, ChevronDown, ChevronUp } from 'lucide-react';
import {
  apiClient,
  type BulkDeleteResponse,
  type UserProfile,
} from '../../lib/api-client';
import { Button } from '../ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog';

interface BulkDeleteConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedUsers: UserProfile[];
  onDeleted: (deletedIds: string[]) => void;
}

const SKIP_REASON_LABEL: Record<string, string> = {
  self: 'your own account',
  system_admin: 'system admin account',
  not_found: 'already removed',
};

export function BulkDeleteConfirmDialog({
  open,
  onOpenChange,
  selectedUsers,
  onDeleted,
}: BulkDeleteConfirmDialogProps) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [showDetails, setShowDetails] = useState(false);

  const mutation = useMutation({
    mutationFn: () =>
      apiClient.bulkDeleteUsers({
        userIds: selectedUsers.map((u) => u.id),
      }),
    onSuccess: (result: BulkDeleteResponse) => {
      const partial = result.summary.skipped > 0 || result.summary.failed > 0;
      if (partial) {
        const parts = [`Deleted ${result.summary.deleted}.`];
        if (result.summary.skipped > 0) parts.push(`Skipped ${result.summary.skipped}.`);
        if (result.summary.failed > 0) parts.push(`Failed ${result.summary.failed}.`);
        toast.warning(parts.join(' '), {
          description: 'See dashboard for details.',
        });
      } else {
        toast.success(
          `Deleted ${result.summary.deleted} user${result.summary.deleted === 1 ? '' : 's'}`,
        );
      }
      queryClient.invalidateQueries({ queryKey: ['users'] });
      onDeleted(result.deleted);
      setError(null);
      onOpenChange(false);
    },
    onError: (err) => {
      setError(err instanceof Error ? err.message : 'Failed to delete users');
    },
  });

  const count = selectedUsers.length;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        // Do not allow closing while a request is in flight.
        if (mutation.isPending) return;
        if (!next) setError(null);
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-destructive flex items-center gap-2">
            <AlertTriangle className="h-5 w-5" />
            Delete {count} user{count === 1 ? '' : 's'}
          </DialogTitle>
          <DialogDescription className="space-y-3">
            <span className="block rounded-md border border-destructive/50 bg-destructive/10 p-3 text-destructive">
              <strong>This action is permanent and cannot be undone.</strong>{' '}
              Deleting these users also removes their attendance records,
              statuses, and session participation entries.
            </span>
            <span className="block text-foreground">
              The following user{count === 1 ? '' : 's'} will be deleted:
            </span>
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[40vh] overflow-y-auto rounded-md border">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-muted">
              <tr>
                <th className="p-2 text-left font-medium">Name</th>
                <th className="p-2 text-left font-medium">Rank</th>
                <th className="p-2 text-left font-medium">Battery</th>
              </tr>
            </thead>
            <tbody>
              {selectedUsers.map((u) => (
                <tr key={u.id} className="border-t">
                  <td className="p-2">{u.fullName || 'Unknown'}</td>
                  <td className="p-2">{u.rank || '—'}</td>
                  <td className="p-2">{u.battery || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        {mutation.data &&
          (mutation.data.summary.skipped > 0 ||
            mutation.data.summary.failed > 0) && (
            <div className="rounded-md border bg-muted/30 p-3 text-sm">
              <button
                type="button"
                className="flex items-center gap-1 text-muted-foreground hover:text-foreground"
                onClick={() => setShowDetails((v) => !v)}
              >
                {showDetails ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
                Per-id outcome
              </button>
              {showDetails && (
                <div className="mt-2 space-y-2">
                  {mutation.data.skipped.length > 0 && (
                    <div>
                      <p className="text-xs font-medium">Skipped:</p>
                      <ul className="list-disc pl-5 text-xs text-muted-foreground">
                        {mutation.data.skipped.map((s) => (
                          <li key={s.id}>
                            {s.id}: {SKIP_REASON_LABEL[s.reason] ?? s.reason}
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                  {mutation.data.failed.length > 0 && (
                    <div>
                      <p className="text-xs font-medium">Failed:</p>
                      <ul className="list-disc pl-5 text-xs text-muted-foreground">
                        {mutation.data.failed.map((f) => (
                          <li key={f.id}>
                            {f.id} ({f.code})
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={mutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending || count === 0}
          >
            {mutation.isPending
              ? 'Deleting...'
              : `Delete ${count} user${count === 1 ? '' : 's'}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
