import { Link, useLocation, useNavigate } from "@tanstack/react-router";
import UserProfile from "./dashboard/user-profile";
import clsx from "clsx";
import { Users, Calendar, ScanLine, BarChart3, Clock } from "lucide-react";
import { useAuth } from "../lib/auth-context";
import { canAccessCommanderFeatures, isSuperadmin } from "../lib/user-utils";

interface NavItem {
  label: string;
  href: string;
  icon: React.ComponentType<React.SVGProps<SVGSVGElement>>;
  requiresCommander?: boolean;
  requiresSuperadmin?: boolean;
}

const navItems: NavItem[] = [
  {
    label: "Users",
    href: "/dashboard/users",
    icon: Users,
    requiresSuperadmin: true, // Only superadmins can see users
  },
  {
    label: "Sessions",
    href: "/dashboard/sessions",
    icon: Calendar,
    requiresCommander: true, // Commanders and superadmins can see sessions
  },
  {
    label: "Scan QR",
    href: "/dashboard/attendance/scan",
    icon: ScanLine,
  },
  {
    label: "Statuses",
    href: "/dashboard/statuses",
    icon: Clock,
    requiresSuperadmin: true, // Only superadmins can manage statuses
  },
  {
    label: "Reports",
    href: "/dashboard/reports",
    icon: BarChart3,
    requiresSuperadmin: true, // Only superadmins can see reports
  },
];

export default function Sidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();

  // Use utility functions that match backend logic
  const canAccessCommander = canAccessCommanderFeatures(user);
  const isSuperadminUser = isSuperadmin(user);

  // Filter nav items based on user permissions
  // This will automatically re-render when user data changes via useAuth()
  const filteredNavItems = navItems.filter((item) => {
    if (item.requiresSuperadmin && !isSuperadminUser) return false;
    if (item.requiresCommander && !canAccessCommander) return false;
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
          </div>

          <UserProfile />
        </nav>
      </div>
    </div>
  );
}
