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
import {
  isValidNricLast5,
  normalizeNricLast5,
  NRIC_LAST5_FIELD_MESSAGE,
} from '../../lib/nric-password';

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
}

interface ConflictState {
  existingUserId: string;
  verified: boolean;
  fullName: string;
}

export function AddUserDialog({ open, onOpenChange }: AddUserDialogProps) {
  const queryClient = useQueryClient();
  const [fullName, setFullName] = useState('');
  const [rank, setRank] = useState('');
  const [battery, setBattery] = useState('');
  const [nricLast5, setNricLast5] = useState('');
  const [showOptional, setShowOptional] = useState(false);
  const [dob, setDob] = useState('');
  const [extras, setExtras] = useState<Array<{ key: string; value: string }>>([]);
  const [conflict, setConflict] = useState<ConflictState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const reset = () => {
    setFullName('');
    setRank('');
    setBattery('');
    setNricLast5('');
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

  const nricTouched = nricLast5.length > 0;
  const nricInvalid = nricTouched && !isValidNricLast5(nricLast5);

  const formValid =
    fullName.trim().length > 0 &&
    VALID_RANKS.includes(rank) &&
    VALID_BATTERIES.includes(battery) &&
    isValidNricLast5(nricLast5) &&
    (dob.length === 0 || dob.length === 6);

  const mutation = useMutation({
    mutationFn: (payload: CreateUserRequest) => apiClient.createUser(payload),
    onSuccess: (created) => {
      toast.success(`Created ${created.fullName ?? 'user'}`);
      queryClient.invalidateQueries({ queryKey: ['users'] });
      reset();
      onOpenChange(false);
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
      nricLast5: normalizeNricLast5(nricLast5),
    };
    if (dob.length === 6) payload.dob = dob;
    if (Object.keys(extrasMap).length > 0) payload.extras = extrasMap;
    mutation.mutate(payload);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add User</DialogTitle>
          <DialogDescription>
            Create a single user. The NRIC last 5 doubles as the user's
            password. CPT and above are automatically superadmin.
          </DialogDescription>
        </DialogHeader>

        {conflict && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm">
            <p className="font-medium text-destructive">User already exists</p>
            <p className="text-muted-foreground">
              A user named <strong>{conflict.fullName}</strong> with this NRIC
              last 5 is already in the system
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
            <Label htmlFor="add-user-nric">NRIC Last 5</Label>
            <Input
              id="add-user-nric"
              value={nricLast5}
              onChange={(e) =>
                setNricLast5(normalizeNricLast5(e.target.value).slice(0, 5))
              }
              placeholder="e.g., 1234A"
              maxLength={5}
              aria-invalid={nricInvalid}
            />
            <p className={`text-xs ${nricInvalid ? 'text-destructive' : 'text-muted-foreground'}`}>
              {nricInvalid ? NRIC_LAST5_FIELD_MESSAGE : 'This will be the user\'s password.'}
            </p>
          </div>

          <button
            type="button"
            className="text-xs text-muted-foreground underline-offset-4 hover:underline"
            onClick={() => setShowOptional((v) => !v)}
          >
            {showOptional ? 'Hide optional fields' : 'Show optional fields (DOB, extras)'}
          </button>

          {showOptional && (
            <div className="space-y-3 rounded-md border p-3">
              <div className="grid gap-2">
                <Label htmlFor="add-user-dob">Date of Birth (DDMMYY)</Label>
                <Input
                  id="add-user-dob"
                  value={dob}
                  onChange={(e) =>
                    setDob(e.target.value.replace(/\D/g, '').slice(0, 6))
                  }
                  placeholder="e.g., 150393"
                  maxLength={6}
                />
              </div>
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
