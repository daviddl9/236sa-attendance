import { useState, useEffect } from 'react';
import { useAuth } from '@/lib/auth-context';
import { useQueryClient } from '@tanstack/react-query';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogOverlay,
  DialogPortal,
} from '@/components/ui/dialog';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { apiClient } from '@/lib/api-client';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';

const VALID_RANKS = [
  'REC', 'PTE', 'LCP', 'CPL', 'CFC',
  '3SG', '2SG', '1SG', 'SSG', 'MSG',
  '3WO', '2WO', '1WO', 'MWO', 'SWO',
  '2LT', 'LTA', 'CPT', 'MAJ', 'LTC',
];

const BATTERIES = ['HQ', 'Alpha', 'Bravo'];

export default function FirstTimeSignInModal() {
  const { user, refetch } = useAuth();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [rank, setRank] = useState<string>('');
  const [battery, setBattery] = useState<string>('');
  const [loading, setLoading] = useState(false);

  // Check if user needs to complete profile (missing rank or battery)
  useEffect(() => {
    if (user && (!user.rank || !user.battery)) {
      setOpen(true);
      setRank(user.rank || '');
      setBattery(user.battery || '');
    } else {
      setOpen(false);
    }
  }, [user]);

  const handleSubmit = async () => {
    if (!rank || !battery) {
      toast.error('Please select both rank and battery');
      return;
    }

    setLoading(true);
    try {
      await apiClient.updateProfile({ rank, battery });
      // Invalidate and refetch session to get updated user data
      await queryClient.invalidateQueries({ queryKey: ['session'] });
      await refetch();
      toast.success('Profile updated successfully');
      setOpen(false);
    } catch (error) {
      console.error('Profile update error:', error);
      toast.error('Failed to update profile');
    } finally {
      setLoading(false);
    }
  };

  // Don't render if user is not loaded or already has rank and battery
  if (!user || (user.rank && user.battery)) {
    return null;
  }

  return (
    <Dialog open={open} onOpenChange={(newOpen) => {
      // Prevent closing until profile is complete
      if (!newOpen && (!user?.rank || !user?.battery)) {
        return;
      }
      setOpen(newOpen);
    }}>
      <DialogPortal>
        {/* Custom overlay with blur - prevents interaction with background */}
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-50 bg-black/60 backdrop-blur-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
          onPointerDown={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
        />
        <DialogPrimitive.Content
          className={cn(
            "bg-background data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 fixed top-[50%] left-[50%] z-50 grid w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-4 rounded-md border p-6 shadow-lg duration-200 sm:max-w-md",
          )}
          onPointerDownOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
          onInteractOutside={(e) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>Complete Your Profile</DialogTitle>
            <DialogDescription>
              Please provide your rank and battery to complete your profile setup.
            </DialogDescription>
          </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="modal-rank">Rank *</Label>
            <Select
              value={rank || 'none'}
              onValueChange={(value) => setRank(value === 'none' ? '' : value)}
              disabled={loading}
            >
              <SelectTrigger id="modal-rank">
                <SelectValue placeholder="Select Rank" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">Select Rank</SelectItem>
                {VALID_RANKS.map((r) => (
                  <SelectItem key={r} value={r}>
                    {r}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="modal-battery">Battery *</Label>
            <Select
              value={battery || 'none'}
              onValueChange={(value) => setBattery(value === 'none' ? '' : value)}
              disabled={loading}
            >
              <SelectTrigger id="modal-battery">
                <SelectValue placeholder="Select Battery" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">Select Battery</SelectItem>
                {BATTERIES.map((b) => (
                  <SelectItem key={b} value={b}>
                    {b}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
          <div className="flex justify-end gap-2">
            <Button onClick={handleSubmit} disabled={loading || !rank || !battery}>
              {loading ? 'Saving...' : 'Save'}
            </Button>
          </div>
        </DialogPrimitive.Content>
      </DialogPortal>
    </Dialog>
  );
}

