import { createFileRoute } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { useAuth } from "@/lib/auth-context";
import DashboardLayout from "@/components/dashboard/layout";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { toast } from "sonner";
import { Settings2 } from "lucide-react";
import { apiClient } from "@/lib/api-client";

const VALID_RANKS = [
  'REC', 'PTE', 'LCP', 'CPL', 'CFC',
  '3SG', '2SG', '1SG', 'SSG', 'MSG',
  '3WO', '2WO', '1WO', 'MWO', 'SWO',
  '2LT', 'LTA', 'CPT', 'MAJ', 'LTC',
];

const BATTERIES = ['HQ', 'Alpha', 'Bravo'];

export const Route = createFileRoute("/dashboard/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  const { user, refetch } = useAuth();
  const [name, setName] = useState("");
  const [rank, setRank] = useState<string>("");
  const [battery, setBattery] = useState<string>("");
  const [loading, setLoading] = useState(false);

  // Initialize form with user data
  useEffect(() => {
    if (user) {
      setName(user.fullName || "");
      // Only set rank/battery if they exist (not null/undefined)
      setRank(user.rank ?? "");
      setBattery(user.battery ?? "");
    } else {
      // Reset form when user is not available
      setName("");
      setRank("");
      setBattery("");
    }
  }, [user]);

  const handleProfileUpdate = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!name.trim()) {
      toast.error("Name cannot be empty");
      return;
    }

    setLoading(true);
    try {
      const updateData: { fullName: string; rank?: string; battery?: string } = {
        fullName: name,
      };
      
      if (rank) {
        updateData.rank = rank;
      }
      
      if (battery) {
        updateData.battery = battery;
      }
      
      await apiClient.updateProfile(updateData);
      await refetch();
      toast.success("Profile updated successfully");
    } catch (error) {
      console.error("Profile update error:", error);
      toast.error("Failed to update profile");
    } finally {
      setLoading(false);
    }
  };

  if (!user) {
    return (
      <DashboardLayout>
        <div className="flex flex-col gap-6 p-6">
          <p>Loading...</p>
        </div>
      </DashboardLayout>
    );
  }

  const userInitial = user.fullName && user.fullName.length > 0 ? user.fullName.charAt(0).toUpperCase() : "U";

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-6 p-6">
        {/* Header */}
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Settings</h1>
          <p className="text-muted-foreground mt-2">
            Manage your account settings and preferences
          </p>
        </div>

        <div className="w-full max-w-4xl">
          <div className="space-y-6">
            {/* Profile Information */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Settings2 className="h-5 w-5" />
                  Profile Information
                </CardTitle>
                <CardDescription>
                  Update your personal information and profile settings
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="flex items-center gap-4">
                  <Avatar className="h-20 w-20">
                    <AvatarFallback className="text-xl">{userInitial}</AvatarFallback>
                  </Avatar>
                  <div className="text-sm text-muted-foreground">
                    <p>Profile information</p>
                  </div>
                </div>

                <form onSubmit={handleProfileUpdate}>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="name">Name</Label>
                      <Input
                        id="name"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        placeholder="Enter your name"
                        disabled={loading}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="rank">Rank</Label>
                      <Select
                        key={`rank-select-${user?.id}-${rank || 'none'}`}
                        value={rank || "none"}
                        onValueChange={(value) => setRank(value === "none" ? "" : value)}
                        disabled={loading}
                      >
                        <SelectTrigger id="rank">
                          <SelectValue placeholder="Select Rank" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">None</SelectItem>
                          {VALID_RANKS.map((r) => (
                            <SelectItem key={r} value={r}>
                              {r}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="battery">Battery</Label>
                      <Select
                        key={`battery-select-${user?.id}-${battery || 'none'}`}
                        value={battery || "none"}
                        onValueChange={(value) => setBattery(value === "none" ? "" : value)}
                        disabled={loading}
                      >
                        <SelectTrigger id="battery">
                          <SelectValue placeholder="Select Battery" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">None</SelectItem>
                          {BATTERIES.map((b) => (
                            <SelectItem key={b} value={b}>
                              {b}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  <Button type="submit" disabled={loading} className="mt-6">
                    {loading ? "Saving..." : "Save Changes"}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </DashboardLayout>
  );
}
