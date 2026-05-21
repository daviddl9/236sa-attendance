import { createFileRoute, useNavigate, useSearch } from '@tanstack/react-router';
import { useState } from 'react';
import { Button } from '../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../components/ui/select';
import { toast } from 'sonner';
import { apiClient, type RegisterUserRequest } from '../../lib/api-client';
import { useAuth } from '../../lib/auth-context';
import { isValidNricLast5, normalizeNricLast5, NRIC_LAST5_FIELD_MESSAGE } from '../../lib/nric-password';

export const Route = createFileRoute('/attendance/register')({
  component: RegisterPage,
  validateSearch: (search: Record<string, unknown>) => {
    return {
      redirect: (search.redirect as string) || undefined,
      session: (search.session as string) || undefined,
    };
  },
});

// Valid ranks (matching backend)
const VALID_RANKS = [
  'REC', 'PTE', 'LCP', 'CPL', 'CFC',
  '3SG', '2SG', '1SG', 'SSG', 'MSG',
  '3WO', '2WO', '1WO', 'MWO', 'SWO',
  '2LT', 'LTA', 'CPT', 'MAJ', 'LTC',
];

const BATTERIES = ['HQ', 'Alpha', 'Bravo'];

function RegisterPage() {
  const navigate = useNavigate();
  const { refetch } = useAuth();
  const search = useSearch({ from: '/attendance/register' }) as {
    redirect?: string;
    session?: string;
  };

  const [loading, setLoading] = useState(false);
  const [formData, setFormData] = useState<RegisterUserRequest>({
    fullName: '',
    rank: '',
    battery: '',
    nricLast5: '',
    dob: '',
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Validation
    if (!formData.fullName.trim()) {
      toast.error('Full Name is required');
      return;
    }
    if (!formData.rank) {
      toast.error('Rank is required');
      return;
    }
    if (!formData.battery) {
      toast.error('Battery is required');
      return;
    }
    if (!isValidNricLast5(formData.nricLast5)) {
      toast.error(NRIC_LAST5_FIELD_MESSAGE);
      return;
    }
    if (formData.dob.length !== 6) {
      toast.error('Date of Birth must be in DDMMYY format (6 characters)');
      return;
    }

    try {
      setLoading(true);
      await apiClient.registerUser({
        ...formData,
        nricLast5: normalizeNricLast5(formData.nricLast5),
      });
      await refetch(); // Refresh auth context to get updated user data
      toast.success('Account created successfully!');

      // Navigate to redirect URL if present, otherwise go to dashboard
      if (search.redirect) {
        window.location.href = search.redirect;
      } else {
        navigate({ to: '/dashboard' });
      }
    } catch (error) {
      setLoading(false);
      const errorMessage =
        error instanceof Error
          ? error.message
          : 'Failed to create account. Please try again.';

      // Check if user already exists
      if (
        errorMessage.includes('already exists') ||
        errorMessage.includes('409')
      ) {
        toast.error(
          'An account with this information already exists. Please sign in instead.',
          {
            action: {
              label: 'Sign In',
              onClick: () => {
                const redirect = search.redirect
                  ? `?redirect=${encodeURIComponent(search.redirect)}`
                  : '';
                window.location.href = `/sign-in${redirect}`;
              },
            },
          }
        );
      } else {
        toast.error(errorMessage);
      }
    }
  };

  return (
    <div className="flex flex-col justify-center items-center w-full min-h-screen p-4">
      <Card className="max-w-md w-full">
        <CardHeader>
          <CardTitle className="text-lg md:text-xl">
            Create Account
          </CardTitle>
          <CardDescription className="text-xs md:text-sm">
            Enter your information to mark attendance
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="fullName">Full Name (as in NRIC) *</Label>
              <Input
                id="fullName"
                type="text"
                placeholder="Enter your full name"
                value={formData.fullName}
                onChange={(e) =>
                  setFormData({ ...formData, fullName: e.target.value })
                }
                disabled={loading}
                required
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="rank">Rank *</Label>
              <Select
                value={formData.rank}
                onValueChange={(value) =>
                  setFormData({ ...formData, rank: value })
                }
                disabled={loading}
                required
              >
                <SelectTrigger id="rank">
                  <SelectValue placeholder="Select your rank" />
                </SelectTrigger>
                <SelectContent>
                  {VALID_RANKS.map((rank) => (
                    <SelectItem key={rank} value={rank}>
                      {rank}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="battery">Battery *</Label>
              <Select
                value={formData.battery}
                onValueChange={(value) =>
                  setFormData({ ...formData, battery: value })
                }
                disabled={loading}
                required
              >
                <SelectTrigger id="battery">
                  <SelectValue placeholder="Select your battery" />
                </SelectTrigger>
                <SelectContent>
                  {BATTERIES.map((battery) => (
                    <SelectItem key={battery} value={battery}>
                      {battery}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="nricLast5">NRIC Last 5 Characters *</Label>
              <Input
                id="nricLast5"
                type="text"
                placeholder="e.g., 1234A"
                value={formData.nricLast5}
                onChange={(e) => {
                  const value = normalizeNricLast5(e.target.value).slice(0, 5);
                  setFormData({ ...formData, nricLast5: value });
                }}
                disabled={loading}
                maxLength={5}
                required
              />
              <p className="text-xs text-muted-foreground">
                Last 5 characters of your NRIC, e.g. 1234A
              </p>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="dob">Date of Birth (DDMMYY) *</Label>
              <Input
                id="dob"
                type="text"
                placeholder="e.g., 010196"
                value={formData.dob}
                onChange={(e) => {
                  const value = e.target.value.replace(/\D/g, '').slice(0, 6);
                  setFormData({ ...formData, dob: value });
                }}
                disabled={loading}
                maxLength={6}
                required
              />
              <p className="text-xs text-muted-foreground">
                Format: DDMMYY (e.g., 010196 for 1 Jan 1996)
              </p>
            </div>

            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? 'Creating Account...' : 'Create Account'}
            </Button>
          </form>
        </CardContent>
      </Card>
      <p className="mt-6 text-xs text-center text-gray-500 dark:text-gray-400 max-w-md">
        Already have an account?{' '}
        <a
          href={`/sign-in${search.redirect ? `?redirect=${encodeURIComponent(search.redirect)}` : ''}`}
          className="underline hover:text-gray-700 dark:hover:text-gray-300"
        >
          Sign in
        </a>
      </p>
    </div>
  );
}
