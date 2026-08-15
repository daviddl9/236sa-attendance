import { createFileRoute } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { apiClient } from '../../../lib/api-client';
import { cn } from '../../../lib/utils';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../../components/ui/select';
import { Badge } from '../../../components/ui/badge';
import { Tabs, TabsList, TabsTrigger } from '../../../components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../../../components/ui/dialog';
import { Input } from '../../../components/ui/input';
import { Checkbox } from '../../../components/ui/checkbox';
import { Label } from '../../../components/ui/label';
import { QRCodeSVG } from 'qrcode.react';
import {
  ArrowLeft,
  Download,
  X,
  FileDown,
  FileSpreadsheet,
  Copy,
  Search,
  Trash2,
  ChevronRight,
} from 'lucide-react';
import { toast } from 'sonner';
import { Link, useNavigate } from '@tanstack/react-router';
import { useAuth } from '../../../lib/auth-context';
import { canAccessCommanderFeatures, isSuperadmin } from '../../../lib/user-utils';
import type { UserInfo } from '../../../lib/api-client';
import { UserTable } from '../../../components/users/user-table';
import { useSessionSSE } from '../../../hooks/use-session-sse';

export const Route = createFileRoute('/dashboard/sessions/$sessionId')({
  component: SessionDetailPage,
});

function SessionDetailPage() {
  const { sessionId } = Route.useParams();
  const { user } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const [batteryFilter, setBatteryFilter] = useState('');
  const [markingUserId, setMarkingUserId] = useState<string | null>(null);
  const [statsTab, setStatsTab] = useState<'All' | 'HQ' | 'Alpha' | 'Bravo'>('All');
  const [exportTab, setExportTab] = useState<'All' | 'HQ' | 'Alpha' | 'Bravo'>('All');
  const [searchQuery, setSearchQuery] = useState('');
  const [includeAbsentList, setIncludeAbsentList] = useState(true);
  const [includePresentList, setIncludePresentList] = useState(true);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [presentBatteryFilter, setPresentBatteryFilter] = useState('');
  const [presentSearchQuery, setPresentSearchQuery] = useState('');
  const [unmarkTarget, setUnmarkTarget] = useState<UserInfo | null>(null);
  const [highlightedCard, setHighlightedCard] = useState<string | null>(null);
  const scrollToCard = (id: string) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    el.focus({ preventScroll: true });
    setHighlightedCard(id);
    setTimeout(() => setHighlightedCard((c) => (c === id ? null : c)), 1200);
  };

  const isCommander = canAccessCommanderFeatures(user);
  const canMarkAttendance = isCommander;
  const canUnmarkAttendance = isCommander;

  // SSE connection for live attendance updates
  useSessionSSE({
    sessionId,
    enabled: isCommander, // Will only connect when session is active (backend validates)
  });

  const { data: session, isLoading } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => apiClient.getSessionById(sessionId),
  });

  const { data: analytics } = useQuery({
    queryKey: ['session-analytics', sessionId],
    queryFn: () => apiClient.getSessionAnalytics(sessionId),
    enabled: !!session,
  });

  const missingUsers = analytics?.missingUsers ?? [];
  const filteredMissingUsers = missingUsers.filter((u) => {
    const matchesBattery = !batteryFilter || u.battery === batteryFilter;
    const matchesSearch =
      !searchQuery ||
      u.fullName?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      u.rank?.toLowerCase().includes(searchQuery.toLowerCase());
    return matchesBattery && matchesSearch;
  });

  const presentUsers = analytics?.presentUsers ?? [];
  const filteredPresentUsers = presentUsers.filter((u) => {
    const matchesBattery = !presentBatteryFilter || u.battery === presentBatteryFilter;
    const matchesSearch =
      !presentSearchQuery ||
      u.fullName?.toLowerCase().includes(presentSearchQuery.toLowerCase()) ||
      u.rank?.toLowerCase().includes(presentSearchQuery.toLowerCase());
    return matchesBattery && matchesSearch;
  });

  const manualMarkMutation = useMutation({
    mutationFn: (userId: string) => {
      setMarkingUserId(userId);
      return apiClient.manualMarkAttendance(sessionId, { userIds: [userId] });
    },
    onSuccess: (result) => {
      const errors = result.errors ?? [];
      if (result.successCount === 0 || errors.length > 0) {
        toast.error(errors.join('; ') || 'Failed to mark attendance');
      } else {
        toast.success('User marked as present');
      }
      setMarkingUserId(null);
      queryClient.invalidateQueries({ queryKey: ['session-analytics', sessionId] });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to mark attendance');
      setMarkingUserId(null);
    },
  });

  const unmarkMutation = useMutation({
    mutationFn: (userId: string) => apiClient.removeAttendance(sessionId, userId),
    onSuccess: () => {
      toast.success('Attendance removed');
      setUnmarkTarget(null);
      queryClient.invalidateQueries({ queryKey: ['session-analytics', sessionId] });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to remove attendance');
      setUnmarkTarget(null);
    },
  });

  const closeMutation = useMutation({
    mutationFn: () => apiClient.closeSession(sessionId),
    onSuccess: () => {
      toast.success('Session closed successfully');
      queryClient.invalidateQueries({ queryKey: ['session', sessionId] });
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to close session');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => apiClient.deleteSession(sessionId),
    onSuccess: () => {
      toast.success('Session deleted successfully');
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      navigate({ to: '/dashboard/sessions' });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to delete session');
      setDeleteDialogOpen(false);
    },
  });

  const canClose = user?.isSuperadmin || session?.createdBy === user?.id;
  const canDelete = isSuperadmin(user);

  // The attendance QR points at the workers.dev dashboard so a soldier can
  // scan, sign in, and monitor their attendance immediately. The token is the
  // opaque session:secret pair, never the raw deep-link code or bot token.
  const attendanceOrigin = 'https://236sa-attendance.ddl-tdh.workers.dev';
  const qrCodeUrl = session
    ? session.qrCode
      ? (() => {
          const parts = session.qrCode.split(':');
          const secret = parts.length >= 2 ? parts[1] : '';
          return `${attendanceOrigin}/qr/${session.id}:${secret}`;
        })()
      : ''
    : '';

  const handleDownloadQR = () => {
    if (!session || !qrCodeUrl) return;

    // Get the QR code SVG element
    const svgElement = document.querySelector('svg[data-qr-code]') as SVGElement;
    if (!svgElement) {
      toast.error('QR code not found');
      return;
    }

    try {
      // Convert SVG to blob and download
      const svgData = new XMLSerializer().serializeToString(svgElement);
      const svgBlob = new Blob([svgData], { type: 'image/svg+xml;charset=utf-8' });
      const url = URL.createObjectURL(svgBlob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${session.name || 'session'}_qr.svg`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch {
      toast.error('Failed to download QR code');
    }
  };

  const handleCopyLink = async () => {
    if (!qrCodeUrl) return;
    try {
      await navigator.clipboard.writeText(qrCodeUrl);
      toast.success('Link copied to clipboard');
    } catch {
      toast.error('Failed to copy link');
    }
  };

  const handleExport = async (format: 'csv' | 'excel') => {
    try {
      const battery = exportTab === 'All' ? undefined : exportTab;
      const blob =
        format === 'csv'
          ? await apiClient.exportSessionCSV(sessionId, battery)
          : await apiClient.exportSessionExcel(sessionId, battery);

      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      const suffix = battery ? `_${battery}` : '';
      a.download = `${session?.name || 'session'}${suffix}_attendance.${format === 'csv' ? 'csv' : 'xlsx'}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      toast.success(`Exported ${exportTab} to ${format.toUpperCase()}`);
    } catch {
      toast.error(`Failed to export to ${format.toUpperCase()}`);
    }
  };

  const handleCopyText = async () => {
    if (!analytics || !session) return;

    const battery = exportTab === 'All' ? undefined : exportTab;
    const stats = battery
      ? analytics.byBattery?.[battery]
      : { total: analytics.totalUsers, present: analytics.presentCount };

    const dateStr = new Date().toLocaleString('en-SG', {
      timeZone: 'Asia/Singapore',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    });

    const absentList = battery
      ? analytics.missingUsers?.filter((u) => u.battery === battery)
      : analytics.missingUsers;

    const presentList = battery
      ? analytics.presentUsers?.filter((u) => u.battery === battery)
      : analytics.presentUsers;

    const batteries = ['HQ', 'Alpha', 'Bravo'];

    const formatUserList = (
      users: typeof absentList,
      groupByBattery: boolean
    ) => {
      if (!users || users.length === 0) return 'None\n';

      const formatUser = (user: (typeof users)[0], index: number) => {
        const name = user.fullName || 'Unknown';
        const rank = user.rank || '';
        const status = user.activeStatus ? ` (${user.activeStatus.displayName})` : '';
        return `${index + 1}. ${rank} ${name}${status}\n`;
      };

      if (groupByBattery) {
        let result = '';
        for (const batt of batteries) {
          const batteryUsers = users.filter((u) => u.battery === batt);
          if (batteryUsers.length > 0) {
            result += `_${batt}_\n`;
            batteryUsers.forEach((user, i) => {
              result += formatUser(user, i);
            });
            result += '\n';
          }
        }
        return result || 'None\n';
      } else {
        let result = '';
        users.forEach((user, i) => {
          result += formatUser(user, i);
        });
        result += '\n';
        return result;
      }
    };

    if (!includeAbsentList && !includePresentList) {
      toast.error('Please select at least one list to include');
      return;
    }

    let text = `*236 SA Attendance (${dateStr} SGT)*\n`;
    if (battery) text += `*Battery:* ${battery}\n`;
    text += `*Present:* ${stats?.present || 0} / ${stats?.total || 0}\n\n`;

    if (includeAbsentList) {
      text += `*Absent List*\n`;
      text += formatUserList([...absentList], !battery);
    }

    if (includePresentList) {
      text += `*Present List*\n`;
      text += formatUserList([...presentList], !battery);
    }

    try {
      await navigator.clipboard.writeText(text);
      toast.success('Copied to clipboard');
    } catch {
      toast.error('Failed to copy to clipboard');
    }
  };

  if (isLoading) {
    return (
      <DashboardLayout>
        <div className="flex items-center justify-center py-12">
          <div className="text-muted-foreground">Loading...</div>
        </div>
      </DashboardLayout>
    );
  }

  if (!session) {
    return (
      <DashboardLayout>
        <div className="flex items-center justify-center py-12">
          <div className="text-muted-foreground">Session not found</div>
        </div>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <Link to="/dashboard/sessions">
              <Button variant="ghost" size="icon">
                <ArrowLeft className="h-4 w-4" />
              </Button>
            </Link>
            <div>
              <h1 className="text-3xl font-semibold tracking-tight">{session.name}</h1>
              <p className="text-muted-foreground">
                {new Date(session.startTime).toLocaleString()}
              </p>
            </div>
          </div>
          <div className="flex gap-2">
            <Badge variant={session.status === 'active' ? 'default' : 'secondary'}>
              {session.status.toUpperCase()}
            </Badge>
            {session.status === 'active' && canClose && (
              <Button
                variant="destructive"
                onClick={() => closeMutation.mutate()}
                disabled={closeMutation.isPending}
              >
                <X className="mr-2 h-4 w-4" />
                Close Session
              </Button>
            )}
            {canDelete && (
              <Button
                variant="destructive"
                onClick={() => setDeleteDialogOpen(true)}
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete Session
              </Button>
            )}
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          {isCommander && (
            <Card>
              <CardHeader>
                <CardTitle>Attendance QR Code</CardTitle>
                <CardDescription>Scan this QR code to mark and monitor your attendance</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col items-center gap-4">
                {session && qrCodeUrl ? (
                  <div className="bg-white p-4 rounded-lg">
                    <QRCodeSVG value={qrCodeUrl} size={256} level="H" data-qr-code />
                  </div>
                ) : (
                  <div className="w-64 h-64 bg-muted flex items-center justify-center rounded-lg">
                    <div className="text-muted-foreground">Loading QR Code...</div>
                  </div>
                )}
                <div className="flex flex-col gap-2 w-full max-w-[200px]">
                  <Button onClick={handleDownloadQR} variant="outline" className="w-full" disabled={!session || !qrCodeUrl}>
                    <Download className="mr-2 h-4 w-4" />
                    Download QR Code
                  </Button>
                  <Button onClick={handleCopyLink} variant="outline" className="w-full" disabled={!qrCodeUrl}>
                    <Copy className="mr-2 h-4 w-4" />
                    Copy Link
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}

          {analytics && (() => {
            const getStats = () => {
              if (statsTab === 'All') {
                return {
                  total: analytics.totalUsers,
                  present: analytics.presentCount,
                  percentage: analytics.attendancePercentage,
                  missing: analytics.missingUsers?.length || 0,
                };
              }
              const batteryStats = analytics.byBattery?.[statsTab];
              const total = batteryStats?.total || 0;
              const present = batteryStats?.present || 0;
              const percentage = total > 0 ? (present / total) * 100 : 0;
              const missing = total - present;
              return { total, present, percentage, missing };
            };
            const stats = getStats();
            return (
              <Card>
                <CardHeader>
                  <CardTitle>Statistics</CardTitle>
                  <CardDescription>Session attendance overview</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  {isCommander && (
                    <Tabs value={statsTab} onValueChange={(v) => setStatsTab(v as typeof statsTab)}>
                      <TabsList>
                        <TabsTrigger value="All">All</TabsTrigger>
                        <TabsTrigger value="HQ">HQ</TabsTrigger>
                        <TabsTrigger value="Alpha">Alpha</TabsTrigger>
                        <TabsTrigger value="Bravo">Bravo</TabsTrigger>
                      </TabsList>
                    </Tabs>
                  )}
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <p className="text-sm text-muted-foreground">Total Users</p>
                      <p className="text-2xl font-bold">{stats.total}</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => scrollToCard('present-users-card')}
                      className="text-left group"
                      aria-label="View present users list"
                    >
                      <p className="text-sm text-muted-foreground flex items-center gap-1">
                        Present
                        <ChevronRight className="h-3 w-3 opacity-60 group-hover:opacity-100" />
                      </p>
                      <p className="text-2xl font-bold">{stats.present}</p>
                    </button>
                    <div className="col-span-2">
                      <p className="text-sm text-muted-foreground">Attendance Rate</p>
                      <p className="text-2xl font-bold">
                        {stats.percentage.toFixed(1)}%
                      </p>
                    </div>
                  </div>
                  <div className="pt-4 border-t">
                    <button
                      type="button"
                      onClick={() => scrollToCard('missing-users-card')}
                      className="text-left group"
                      aria-label="View missing users list"
                    >
                      <p className="text-sm text-muted-foreground mb-2 flex items-center gap-1">
                        Missing Users
                        <ChevronRight className="h-3 w-3 opacity-60 group-hover:opacity-100" />
                      </p>
                      <p className="text-lg font-semibold">{stats.missing}</p>
                    </button>
                  </div>
                </CardContent>
              </Card>
            );
          })()}
        </div>

        {isCommander && (
          <Card>
            <CardHeader>
              <CardTitle>Export Attendance</CardTitle>
              <CardDescription>Download attendance records</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <Tabs value={exportTab} onValueChange={(v) => setExportTab(v as typeof exportTab)}>
                <TabsList>
                  <TabsTrigger value="All">All</TabsTrigger>
                  <TabsTrigger value="HQ">HQ</TabsTrigger>
                  <TabsTrigger value="Alpha">Alpha</TabsTrigger>
                  <TabsTrigger value="Bravo">Bravo</TabsTrigger>
                </TabsList>
              </Tabs>
              <div className="flex flex-col gap-2">
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="include-absent"
                    checked={includeAbsentList}
                    onCheckedChange={(checked) => setIncludeAbsentList(checked === true)}
                  />
                  <Label htmlFor="include-absent">Include absent list</Label>
                </div>
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="include-present"
                    checked={includePresentList}
                    onCheckedChange={(checked) => setIncludePresentList(checked === true)}
                  />
                  <Label htmlFor="include-present">Include present list</Label>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button onClick={() => handleExport('csv')} variant="outline">
                  <FileDown className="mr-2 h-4 w-4" />
                  CSV
                </Button>
                <Button onClick={() => handleExport('excel')} variant="outline">
                  <FileSpreadsheet className="mr-2 h-4 w-4" />
                  Excel
                </Button>
                <Button onClick={handleCopyText} variant="outline">
                  <Copy className="mr-2 h-4 w-4" />
                  Copy Text
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

        {analytics && analytics.missingUsers && analytics.missingUsers.length > 0 && (
          <Card id="missing-users-card" tabIndex={-1} className={cn('outline-none', highlightedCard === 'missing-users-card' && 'ring-2 ring-primary transition-shadow')}>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle>Missing Users</CardTitle>
                  <CardDescription>Users who have not marked attendance</CardDescription>
                </div>
                {isCommander && (
                  <Select
                    value={batteryFilter || 'all'}
                    onValueChange={(value) => setBatteryFilter(value === 'all' ? '' : value)}
                  >
                    <SelectTrigger className="w-[150px]">
                      <SelectValue placeholder="All Batteries" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Batteries</SelectItem>
                      <SelectItem value="HQ">HQ</SelectItem>
                      <SelectItem value="Alpha">Alpha</SelectItem>
                      <SelectItem value="Bravo">Bravo</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search by name or rank..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-9"
                />
              </div>
              <UserTable
                users={filteredMissingUsers}
                showActions={false}
                emptyMessage={searchQuery ? 'No matching users' : 'No missing users'}
                onMark={canMarkAttendance ? (userId) => manualMarkMutation.mutate(userId) : undefined}
                markingUserId={markingUserId ?? undefined}
              />
            </CardContent>
          </Card>
        )}

        {analytics && (
          <Card id="present-users-card" tabIndex={-1} className={cn('outline-none', highlightedCard === 'present-users-card' && 'ring-2 ring-primary transition-shadow')}>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle>Present Users</CardTitle>
                  <CardDescription>Users who have marked attendance</CardDescription>
                </div>
                {isCommander && (
                  <Select
                    value={presentBatteryFilter || 'all'}
                    onValueChange={(value) => setPresentBatteryFilter(value === 'all' ? '' : value)}
                  >
                    <SelectTrigger className="w-[150px]">
                      <SelectValue placeholder="All Batteries" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Batteries</SelectItem>
                      <SelectItem value="HQ">HQ</SelectItem>
                      <SelectItem value="Alpha">Alpha</SelectItem>
                      <SelectItem value="Bravo">Bravo</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search by name or rank..."
                  value={presentSearchQuery}
                  onChange={(e) => setPresentSearchQuery(e.target.value)}
                  className="pl-9"
                />
              </div>
              <UserTable
                users={filteredPresentUsers}
                showActions={false}
                emptyMessage={presentSearchQuery ? 'No matching users' : 'No one marked yet'}
                onUnmark={
                  canUnmarkAttendance
                    ? (userId) => {
                        const target = presentUsers.find((u) => u.id === userId);
                        if (target) setUnmarkTarget(target);
                      }
                    : undefined
                }
                unmarkingUserId={unmarkMutation.isPending ? unmarkMutation.variables : undefined}
              />
            </CardContent>
          </Card>
        )}

        <Dialog
          open={!!unmarkTarget}
          onOpenChange={(open) => {
            if (!open) setUnmarkTarget(null);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Remove Attendance</DialogTitle>
              <DialogDescription>
                Are you sure you want to unmark{' '}
                {[unmarkTarget?.rank, unmarkTarget?.fullName].filter(Boolean).join(' ') || 'this user'}?
                They will move back to the missing list.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setUnmarkTarget(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => unmarkTarget && unmarkMutation.mutate(unmarkTarget.id)}
                disabled={unmarkMutation.isPending}
              >
                {unmarkMutation.isPending ? 'Removing...' : 'Unmark'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Delete Session</DialogTitle>
              <DialogDescription>
                Are you sure you want to permanently delete "{session.name}"? This will also delete{' '}
                {analytics?.presentCount ?? 0} attendance records. This action cannot be undone.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => deleteMutation.mutate()}
                disabled={deleteMutation.isPending}
              >
                {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  );
}
