import { Users, Calendar, ScanLine, BarChart3, Clock, DoorOpen } from 'lucide-react';

export interface NavItem {
  label: string;
  href: string;
  icon: React.ComponentType<React.SVGProps<SVGSVGElement>>;
  minTier?: number;
}

/**
 * The dashboard navigation, shared by the desktop sidebar and the mobile
 * menu. Both used to keep their own copy, which meant a new section could
 * appear on one and silently be missing from the other.
 */
export const navItems: NavItem[] = [
  { label: 'Sessions', href: '/dashboard/sessions', icon: Calendar, minTier: 2 },
  { label: 'Groups', href: '/dashboard/groups', icon: Users, minTier: 2 },
  { label: 'Scan QR', href: '/dashboard/attendance/scan', icon: ScanLine },
  { label: 'Out', href: '/dashboard/out', icon: DoorOpen, minTier: 2 },
  { label: 'Users', href: '/dashboard/users', icon: Users, minTier: 2 },
  { label: 'Statuses', href: '/dashboard/statuses', icon: Clock, minTier: 3 },
  { label: 'Reports', href: '/dashboard/reports', icon: BarChart3, minTier: 2 },
];
