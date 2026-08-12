import { Link, useLocation, useNavigate } from "@tanstack/react-router";
import UserProfile from "./dashboard/user-profile";
import clsx from "clsx";
import { Users, Calendar, ScanLine, BarChart3, Clock, UserCheck, Link2 } from "lucide-react";
import { useAuth } from "../lib/auth-context";
import { canManageUnit, isSuperadmin, getUserTier } from "../lib/user-utils";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "../lib/api-client";

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

export default function Sidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();

  const tier = getUserTier(user);
  const isAdmin = isSuperadmin(user);
  const canManageTelegram = canManageUnit(user);

  // Poll pending registration count for superadmins.
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
    <div className="min-[1024px]:block hidden w-64 border-r h-full bg-background">
      <div className="flex h-full flex-col">
        <div className="flex h-[3.45rem] items-center border-b px-4">
          <Link
            to="/"
            className="flex items-center font-semibold hover:cursor-pointer"
          >
            <span>236SA Attendance</span>
          </Link>
        </div>

        <nav className="flex flex-col h-full justify-between items-start w-full space-y-1">
          <div className="w-full space-y-1 p-4">
            {filteredNavItems.map((item) => (
              <div
                key={item.href}
                onClick={() => navigate({ to: item.href })}
                className={clsx(
                  "flex items-center gap-2 w-full rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:cursor-pointer",
                  location.pathname === item.href || location.pathname.startsWith(item.href + '/')
                    ? "bg-primary/10 text-primary hover:bg-primary/20"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </div>
            ))}

            {isAdmin && (
              <div
                onClick={() => navigate({ to: '/dashboard/admin/registrations' })}
                className={clsx(
                  "flex items-center gap-2 w-full rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:cursor-pointer",
                  location.pathname.startsWith('/dashboard/admin/registrations')
                    ? "bg-primary/10 text-primary hover:bg-primary/20"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <UserCheck className="h-4 w-4" />
                <span className="flex-1">Registrations</span>
                {pendingCount > 0 && (
                  <span className="flex items-center justify-center h-5 min-w-5 px-1 rounded-full bg-destructive text-destructive-foreground text-xs font-semibold">
                    {pendingCount}
                  </span>
                )}
              </div>
            )}

            {canManageTelegram && (
              <div
                onClick={() => navigate({ to: '/dashboard/admin/telegram-pairings' })}
                className={clsx(
                  "flex items-center gap-2 w-full rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:cursor-pointer",
                  location.pathname.startsWith('/dashboard/admin/telegram-pairings')
                    ? "bg-primary/10 text-primary hover:bg-primary/20"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <Link2 className="h-4 w-4" />
                <span>Telegram pairings</span>
              </div>
            )}
          </div>

          <UserProfile />
        </nav>
      </div>
    </div>
  );
}
