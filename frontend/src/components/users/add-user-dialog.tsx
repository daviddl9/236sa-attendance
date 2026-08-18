import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { toast } from 'sonner';
import {
  apiClient,
  CreateUserConflictError,
  type CreateUserRequest,
} from '../../lib/api-client';
import { Button } from '../ui/button';
import { Input } from '../ui/input';
import { Label } from '../ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select';
const VALID_RANKS = [
  'REC', 'PTE', 'LCP', 'CPL', 'CFC',
  '3SG', '2SG', '1SG', 'SSG', 'MSG',
  '3WO', '2WO', '1WO', 'MWO', 'SWO',
  '2LT', 'LTA', 'CPT', 'MAJ', 'LTC',
];

const VALID_BATTERIES = ['HQ', 'Alpha', 'Bravo'];

interface AddUserDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Invoked with the created user after a successful create. */
  onCreated?: (user: import('../../lib/api-client').UserProfile) => void;
}

interface ConflictState {
  existingUserId: string;
  verified: boolean;
  fullName: string;
}

export function AddUserDialog({ open, onOpenChange, onCreated }: AddUserDialogProps) {
  const queryClient = useQueryClient();
  const [fullName, setFullName] = useState('');
  const [rank, setRank] = useState('');
  const [battery, setBattery] = useState('');
  const [showOptional, setShowOptional] = useState(false);
  const [dob, setDob] = useState('');
  const [extras, setExtras] = useState<Array<{ key: string; value: string }>>([]);
  const [conflict, setConflict] = useState<ConflictState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const reset = () => {
    setFullName('');
    setRank('');
    setBattery('');
    setShowOptional(false);
    setDob('');
    setExtras([]);
    setConflict(null);
    setFormError(null);
  };

  const handleOpenChange = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const formValid =
    fullName.trim().length > 0 &&
    VALID_RANKS.includes(rank) &&
    VALID_BATTERIES.includes(battery) &&
    dob.length > 0;

  const mutation = useMutation({
    mutationFn: (payload: CreateUserRequest) => apiClient.createUser(payload),
    onSuccess: (created) => {
      toast.success(`Created ${created.fullName ?? 'user'}`);
      queryClient.invalidateQueries({ queryKey: ['users'] });
      reset();
      onOpenChange(false);
      onCreated?.(created);
    },
    onError: (err) => {
      if (err instanceof CreateUserConflictError) {
        setConflict({
          existingUserId: err.details.existingUserId,
          verified: err.details.verified,
          fullName: err.details.fullName,
        });
        return;
      }
      setFormError(err instanceof Error ? err.message : 'Failed to create user');
    },
  });

  const handleSubmit = () => {
    setConflict(null);
    setFormError(null);
    if (!formValid) return;
    const extrasMap: Record<string, string> = {};
    for (const { key, value } of extras) {
      const k = key.trim();
      if (k.length > 0) {
        extrasMap[k] = value;
      }
    }
    const payload: CreateUserRequest = {
      fullName: fullName.trim().replace(/\s+/g, ' '),
      rank,
      battery,
      dob,
    };
    if (Object.keys(extrasMap).length > 0) payload.extras = extrasMap;
    mutation.mutate(payload);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add User</DialogTitle>
          <DialogDescription>
            Create a single user. Date of birth is used for sign-in. CPT and
            above are automatically superadmin.
          </DialogDescription>
        </DialogHeader>

        {conflict && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm">
            <p className="font-medium text-destructive">User already exists</p>
            <p className="text-muted-foreground">
              A user named <strong>{conflict.fullName}</strong> with this date
              of birth is already in the system
              {conflict.verified ? '.' : ' and is awaiting approval.'}
            </p>
            {conflict.existingUserId && (
              <div className="pt-2">
                {conflict.verified ? (
                  <Link
                    to="/dashboard/users/$userId"
                    params={{ userId: conflict.existingUserId }}
                    className="text-primary underline-offset-4 hover:underline"
                    onClick={() => onOpenChange(false)}
                  >
                    View existing user →
                  </Link>
                ) : (
                  <Link
                    to="/dashboard/admin/registrations"
                    className="text-primary underline-offset-4 hover:underline"
                    onClick={() => onOpenChange(false)}
                  >
                    Review pending registration →
                  </Link>
                )}
              </div>
            )}
          </div>
        )}

        {formError && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
            {formError}
          </div>
        )}

        <div className="space-y-4">
          <div className="grid gap-2">
            <Label htmlFor="add-user-fullname">Full Name</Label>
            <Input
              id="add-user-fullname"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              placeholder="e.g., John Doe"
              autoFocus
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-2">
              <Label htmlFor="add-user-rank">Rank</Label>
              <Select value={rank} onValueChange={setRank}>
                <SelectTrigger id="add-user-rank">
                  <SelectValue placeholder="Select rank" />
                </SelectTrigger>
                <SelectContent>
                  {VALID_RANKS.map((r) => (
                    <SelectItem key={r} value={r}>
                      {r}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="add-user-battery">Battery</Label>
              <Select value={battery} onValueChange={setBattery}>
                <SelectTrigger id="add-user-battery">
                  <SelectValue placeholder="Select battery" />
                </SelectTrigger>
                <SelectContent>
                  {VALID_BATTERIES.map((b) => (
                    <SelectItem key={b} value={b}>
                      {b}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="add-user-dob">Date of Birth</Label>
            <Input
              id="add-user-dob"
              type="date"
              value={dob}
              onChange={(e) => setDob(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Used to sign in with your full name.
            </p>
          </div>

          <button
            type="button"
            className="text-xs text-muted-foreground underline-offset-4 hover:underline"
            onClick={() => setShowOptional((v) => !v)}
          >
            {showOptional ? 'Hide optional fields' : 'Show optional fields (extras)'}
          </button>

          {showOptional && (
            <div className="space-y-3 rounded-md border p-3">
              <div className="grid gap-2">
                <Label>Extras</Label>
                {extras.map((row, i) => (
                  <div key={i} className="flex gap-2">
                    <Input
                      placeholder="Key (e.g., Section)"
                      value={row.key}
                      onChange={(e) => {
                        const next = [...extras];
                        next[i] = { ...next[i], key: e.target.value };
                        setExtras(next);
                      }}
                    />
                    <Input
                      placeholder="Value"
                      value={row.value}
                      onChange={(e) => {
                        const next = [...extras];
                        next[i] = { ...next[i], value: e.target.value };
                        setExtras(next);
                      }}
                    />
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        setExtras(extras.filter((_, j) => j !== i))
                      }
                    >
                      Remove
                    </Button>
                  </div>
                ))}
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setExtras([...extras, { key: '', value: '' }])}
                >
                  Add row
                </Button>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!formValid || mutation.isPending}
          >
            {mutation.isPending ? 'Creating...' : 'Create User'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
