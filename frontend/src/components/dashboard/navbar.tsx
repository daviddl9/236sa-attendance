import { Button } from "@/components/ui/button";
import { Dialog, DialogClose } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import {
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import UserProfile from "./user-profile";
import { Menu, Settings, Users, Calendar, ScanLine, BarChart3, Clock, UserCheck } from "lucide-react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/lib/auth-context";
import { isSuperadmin, getUserTier } from "@/lib/user-utils";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api-client";

interface NavItem {
  label: string;
  href: string;
  icon: React.ComponentType<React.SVGProps<SVGSVGElement>>;
  minTier?: number;
}

const navItems: NavItem[] = [
  {
    label: "Sessions",
    href: "/dashboard/sessions",
    icon: Calendar,
    minTier: 2,
  },
  {
    label: "Groups",
    href: "/dashboard/groups",
    icon: Users,
    minTier: 3,
  },
  {
    label: "Scan QR",
    href: "/dashboard/attendance/scan",
    icon: ScanLine,
  },
  {
    label: "Users",
    href: "/dashboard/users",
    icon: Users,
    minTier: 2,
  },
  {
    label: "Statuses",
    href: "/dashboard/statuses",
    icon: Clock,
    minTier: 3,
  },
  {
    label: "Reports",
    href: "/dashboard/reports",
    icon: BarChart3,
    minTier: 2,
  },
];

export default function DashboardTopNav({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  const { user } = useAuth();

  const tier = getUserTier(user);
  const isAdmin = isSuperadmin(user);

  const { data: pendingData } = useQuery({
    queryKey: ['pending-registrations'],
    queryFn: () => apiClient.listPendingRegistrations(),
    enabled: isAdmin,
    refetchInterval: 60_000,
    staleTime: 30_000,
  });
  const pendingCount = pendingData?.total ?? 0;

  const filteredNavItems = navItems.filter((item) => {
    if (item.minTier && tier < item.minTier) return false;
    return true;
  });

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <header className="sticky top-0 z-10 flex h-14 lg:h-[52px] items-center gap-4 border-b bg-background px-3 flex-shrink-0">
        <Dialog>
          <SheetTrigger className="min-[1024px]:hidden p-2 transition" asChild>
            <Button variant="ghost" size="icon">
              <Menu className="h-5 w-5" />
              <span className="sr-only">Toggle menu</span>
            </Button>
          </SheetTrigger>
          <SheetContent side="left">
            <SheetHeader>
              <Link to="/">
                <SheetTitle>236SA Attendance</SheetTitle>
              </Link>
            </SheetHeader>
            <div className="flex flex-col space-y-3 mt-[1rem] px-2">
              {filteredNavItems.map((item) => (
                <DialogClose key={item.href} asChild>
                  <Button
                    variant="outline"
                    className="w-full justify-start px-4"
                    onClick={() => navigate({ to: item.href })}
                  >
                    <item.icon className="mr-3 h-4 w-4" />
                    {item.label}
                  </Button>
                </DialogClose>
              ))}

              {isAdmin && (
                <DialogClose asChild>
                  <Button
                    variant="outline"
                    className="w-full justify-start px-4"
                    onClick={() => navigate({ to: '/dashboard/admin/registrations' })}
                  >
                    <UserCheck className="mr-3 h-4 w-4" />
                    Registrations
                    {pendingCount > 0 && (
                      <span className="ml-auto flex items-center justify-center h-5 min-w-5 px-1 rounded-full bg-destructive text-destructive-foreground text-xs font-semibold">
                        {pendingCount}
                      </span>
                    )}
                  </Button>
                </DialogClose>
              )}

              <Separator className="my-3" />
              <DialogClose asChild>
                <Link to="/dashboard/settings">
                  <Button variant="outline" className="w-full justify-start px-4">
                    <Settings className="mr-3 h-4 w-4" />
                    Settings
                  </Button>
                </Link>
              </DialogClose>
            </div>
          </SheetContent>
        </Dialog>
        <div className="flex justify-center items-center gap-2 ml-auto">
          <UserProfile mini={true} />
        </div>
      </header>
      <div className="flex-1 overflow-y-auto">
        {children}
      </div>
    </div>
  );
}
