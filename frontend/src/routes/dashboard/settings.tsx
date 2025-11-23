import { createFileRoute } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { useAuth } from "@/lib/auth-context";
import DashboardLayout from "@/components/dashboard/layout";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
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
import { toast } from "sonner";
import { Settings2 } from "lucide-react";
import { apiClient } from "@/lib/api-client";

export const Route = createFileRoute("/dashboard/settings")({
  component: SettingsPage,
});

function SettingsPage() {
  const { user, refetch } = useAuth();
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);

  // Initialize form with user data
  useEffect(() => {
    if (user) {
      setName(user.name || "");
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
      await apiClient.updateProfile({ name });
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

  const userInitial = user.name?.charAt(0).toUpperCase() || user.email?.charAt(0).toUpperCase() || "U";

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
                    <AvatarImage src={user.image || undefined} />
                    <AvatarFallback className="text-xl">{userInitial}</AvatarFallback>
                  </Avatar>
                  <div className="text-sm text-muted-foreground">
                    <p>Profile picture is synced from your Google account</p>
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
                      <Label htmlFor="email">Email</Label>
                      <Input
                        id="email"
                        type="email"
                        value={user.email}
                        disabled
                      />
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
