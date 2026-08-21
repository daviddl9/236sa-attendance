import { createFileRoute } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
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
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../../../components/ui/tabs';
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
  CheckCircle2,
  Download,
  X,
  FileDown,
  FileSpreadsheet,
  Copy,
  Search,
  Trash2,
  ChevronRight,
  Users,
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
  validateSearch: (search: Record<string, unknown>): { scanned?: boolean } => {
    const scanned = search.scanned === 'true' || search.scanned === true;
    return scanned ? { scanned: true } : {};
  },
});

function SessionDetailPage() {
  const { sessionId } = Route.useParams();
  const { scanned } = Route.useSearch();
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
  const [duplicateDialogOpen, setDuplicateDialogOpen] = useState(false);
  const [saveGroupDialogOpen, setSaveGroupDialogOpen] = useState(false);
  const [duplicateName, setDuplicateName] = useState('');
  const [saveGroupName, setSaveGroupName] = useState('');
  const [presentBatteryFilter, setPresentBatteryFilter] = useState('');
  const [presentSearchQuery, setPresentSearchQuery] = useState('');
  const [listTab, setListTab] = useState<'missing' | 'present' | null>(null);
  const [unmarkTarget, setUnmarkTarget] = useState<UserInfo | null>(null);
  const [highlightedCard, setHighlightedCard] = useState<string | null>(null);
  const [scannedDialogOpen, setScannedDialogOpen] = useState(!!scanned);

  // Dismiss the "Attendance Scanned" confirmation and drop the ?scanned param
  // so a refresh/back doesn't re-show it.
  const dismissScannedDialog = useCallback(() => {
    setScannedDialogOpen(false);
    navigate({
      to: '/dashboard/sessions/$sessionId',
      params: { sessionId },
      search: (prev) => ({ ...prev, scanned: false }),
      replace: true,
    });
  }, [navigate, sessionId]);

  // Auto-dismiss the confirmation after 5 seconds.
  useEffect(() => {
    if (!scannedDialogOpen) return;
    const timer = setTimeout(dismissScannedDialog, 5000);
    return () => clearTimeout(timer);
  }, [scannedDialogOpen, dismissScannedDialog]);

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

  // Tab not yet chosen by the user: default to Missing, or Present when there
  // is nobody missing (avoids opening the page on an empty list).
  const effectiveTab: 'missing' | 'present' =
    listTab ?? (missingUsers.length === 0 ? 'present' : 'missing');
  const activeSearch = effectiveTab === 'missing' ? searchQuery : presentSearchQuery;
  const setActiveSearch = effectiveTab === 'missing' ? setSearchQuery : setPresentSearchQuery;
  const activeBatteryFilter =
    effectiveTab === 'missing' ? batteryFilter : presentBatteryFilter;
  const setActiveBatteryFilter =
    effectiveTab === 'missing' ? setBatteryFilter : setPresentBatteryFilter;

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

  const duplicateMutation = useMutation({
    mutationFn: (name: string) => apiClient.duplicateSession(sessionId, { name, participantIds: [] }),
    onSuccess: (newSession) => {
      toast.success('Session duplicated');
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      setDuplicateDialogOpen(false);
      navigate({ to: '/dashboard/sessions/$sessionId', params: { sessionId: newSession.id } });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to duplicate session');
      setDuplicateDialogOpen(false);
    },
  });

  const saveGroupMutation = useMutation({
    mutationFn: (name: string) => apiClient.saveSessionAsGroup(sessionId, { name, participantIds: [] }),
    onSuccess: () => {
      toast.success('Saved as reusable group');
      queryClient.invalidateQueries({ queryKey: ['groups'] });
      setSaveGroupDialogOpen(false);
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to save group');
      setSaveGroupDialogOpen(false);
    },
  });

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

    const extras = battery ? 0 : (analytics.extraCount ?? 0);
    const extrasSuffix = extras > 0 ? ` · +${extras} walk-in${extras === 1 ? '' : 's'}` : '';

    let text = `*236 SA Attendance (${dateStr} SGT)*\n`;
    if (battery) text += `*Battery:* ${battery}\n`;
    text += `*Present:* ${stats?.present || 0} / ${stats?.total || 0}${extrasSuffix}\n\n`;

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
        <div className="flex flex-col items-center justify-center gap-4 py-12">
          <div className="text-muted-foreground">Session not found</div>
          <Link to="/dashboard/sessions">
            <Button variant="outline">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Sessions
            </Button>
          </Link>
        </div>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-4 min-w-0">
            <Link to="/dashboard/sessions">
              <Button variant="ghost" size="icon" aria-label="Back to sessions">
                <ArrowLeft className="h-4 w-4" />
              </Button>
            </Link>
            <div className="min-w-0">
              <h1 className="text-xl sm:text-3xl font-semibold tracking-tight truncate">
                {session.name}
              </h1>
              <p className="text-muted-foreground text-sm truncate">
                {new Date(session.startTime).toLocaleString()}
              </p>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={session.status === 'active' ? 'default' : 'secondary'}>
              {session.status.toUpperCase()}
            </Badge>
            {isCommander && (
              <>
                <Button
                  variant="outline"
                  className="h-9 px-3"
                  onClick={() => {
                    setDuplicateName(`${session.name} (copy)`);
                    setDuplicateDialogOpen(true);
                  }}
                  aria-label="Duplicate session"
                >
                  <Copy className="h-4 w-4" />
                  <span className="sr-only sm:not-sr-only sm:ml-2">Duplicate</span>
                </Button>
                <Button
                  variant="outline"
                  className="h-9 px-3"
                  onClick={() => {
                    setSaveGroupName(session.name);
                    setSaveGroupDialogOpen(true);
                  }}
                  aria-label="Save as group"
                >
                  <Users className="h-4 w-4" />
                  <span className="sr-only sm:not-sr-only sm:ml-2">Save as Group</span>
                </Button>
              </>
            )}
            {session.status === 'active' && canClose && (
              <Button
                variant="destructive"
                className="h-9 w-9 px-0 sm:h-auto sm:w-auto sm:px-4"
                onClick={() => closeMutation.mutate()}
                disabled={closeMutation.isPending}
                aria-label="Close session"
              >
                <X className="h-4 w-4" />
                <span className="sr-only sm:not-sr-only sm:ml-2">Close Session</span>
              </Button>
            )}
            {canDelete && (
              <Button
                variant="destructive"
                className="h-9 w-9 px-0 sm:h-auto sm:w-auto sm:px-4"
                onClick={() => setDeleteDialogOpen(true)}
                aria-label="Delete session"
              >
                <Trash2 className="h-4 w-4" />
                <span className="sr-only sm:not-sr-only sm:ml-2">Delete Session</span>
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
                  extras: analytics.extraCount ?? 0,
                };
              }
              const batteryStats = analytics.byBattery?.[statsTab];
              const total = batteryStats?.total || 0;
              const present = batteryStats?.present || 0;
              const percentage = total > 0 ? (present / total) * 100 : 0;
              const missing = total - present;
              // Per-battery rows already include that battery's walk-ins.
              return { total, present, percentage, missing, extras: 0 };
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
                    {isCommander ? (
                      <button
                        type="button"
                        onClick={() => {
                          setListTab('present');
                          scrollToCard('attendance-users-card');
                        }}
                        className="text-left group"
                        aria-label="View present users list"
                      >
                        <p className="text-sm text-muted-foreground flex items-center gap-1">
                          Present
                          <ChevronRight className="h-3 w-3 opacity-60 group-hover:opacity-100" />
                        </p>
                        <p className="text-2xl font-bold">{stats.present}</p>
                        {stats.extras > 0 && (
                          <p className="text-xs text-muted-foreground">
                            +{stats.extras} walk-in{stats.extras === 1 ? '' : 's'}
                          </p>
                        )}
                      </button>
                    ) : (
                      <div className="text-left">
                        <p className="text-sm text-muted-foreground">Present</p>
                        <p className="text-2xl font-bold">{stats.present}</p>
                        {stats.extras > 0 && (
                          <p className="text-xs text-muted-foreground">
                            +{stats.extras} walk-in{stats.extras === 1 ? '' : 's'}
                          </p>
                        )}
                      </div>
                    )}
                    <div className="col-span-2">
                      <p className="text-sm text-muted-foreground">Attendance Rate</p>
                      <p className="text-2xl font-bold">
                        {stats.percentage.toFixed(1)}%
                      </p>
                    </div>
                  </div>
                  <div className="pt-4 border-t">
                    {isCommander ? (
                      <button
                        type="button"
                        onClick={() => {
                          setListTab('missing');
                          scrollToCard('attendance-users-card');
                        }}
                        className="text-left group"
                        aria-label="View missing users list"
                      >
                        <p className="text-sm text-muted-foreground mb-2 flex items-center gap-1">
                          Missing Users
                          <ChevronRight className="h-3 w-3 opacity-60 group-hover:opacity-100" />
                        </p>
                        <p className="text-lg font-semibold">{stats.missing}</p>
                      </button>
                    ) : (
                      <div className="text-left">
                        <p className="text-sm text-muted-foreground mb-2">Missing Users</p>
                        <p className="text-lg font-semibold">{stats.missing}</p>
                      </div>
                    )}
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

        {isCommander && analytics && (
          <Card
            id="attendance-users-card"
            tabIndex={-1}
            className={cn(
              'outline-none',
              highlightedCard === 'attendance-users-card' && 'ring-2 ring-primary transition-shadow'
            )}
          >
            <CardHeader>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <CardTitle>Attendance</CardTitle>
                  <CardDescription>View who is missing or present</CardDescription>
                </div>
                {isCommander && (
                  <Select
                    value={activeBatteryFilter || 'all'}
                    onValueChange={(value) =>
                      setActiveBatteryFilter(value === 'all' ? '' : value)
                    }
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
              <Tabs
                value={effectiveTab}
                onValueChange={(value) => setListTab(value as 'missing' | 'present')}
              >
                <TabsList>
                  <TabsTrigger value="missing">Missing ({missingUsers.length})</TabsTrigger>
                  <TabsTrigger value="present">Present ({presentUsers.length})</TabsTrigger>
                </TabsList>
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder="Search by name or rank..."
                    value={activeSearch}
                    onChange={(e) => setActiveSearch(e.target.value)}
                    className="pl-9"
                  />
                </div>
                <TabsContent value="missing" className="space-y-3">
                  <UserTable
                    users={filteredMissingUsers}
                    showActions={false}
                    emptyMessage={searchQuery ? 'No matching users' : 'No missing users'}
                    onMark={canMarkAttendance ? (userId) => manualMarkMutation.mutate(userId) : undefined}
                    markingUserId={markingUserId ?? undefined}
                  />
                </TabsContent>
                <TabsContent value="present" className="space-y-3">
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
                </TabsContent>
              </Tabs>
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

        <Dialog open={duplicateDialogOpen} onOpenChange={setDuplicateDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Duplicate Session</DialogTitle>
              <DialogDescription>
                Create a new session with the same participants as "{session.name}".
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-2">
              <Label htmlFor="duplicate-name">Session name</Label>
              <Input
                id="duplicate-name"
                value={duplicateName}
                onChange={(e) => setDuplicateName(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setDuplicateDialogOpen(false)}>Cancel</Button>
              <Button
                onClick={() => duplicateMutation.mutate(duplicateName.trim())}
                disabled={duplicateMutation.isPending || !duplicateName.trim()}
              >
                <Copy className="mr-2 h-4 w-4" />
                Duplicate
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={saveGroupDialogOpen} onOpenChange={setSaveGroupDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Save as Reusable Group</DialogTitle>
              <DialogDescription>
                Save "{session.name}"'s participants as a reusable group so you can start future sessions in one click.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-2">
              <Label htmlFor="save-group-name">Group name</Label>
              <Input
                id="save-group-name"
                value={saveGroupName}
                onChange={(e) => setSaveGroupName(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setSaveGroupDialogOpen(false)}>Cancel</Button>
              <Button
                onClick={() => saveGroupMutation.mutate(saveGroupName.trim())}
                disabled={saveGroupMutation.isPending || !saveGroupName.trim()}
              >
                <Users className="mr-2 h-4 w-4" />
                Save Group
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog
          open={scannedDialogOpen}
          onOpenChange={(open) => {
            if (open) {
              setScannedDialogOpen(true);
            } else {
              dismissScannedDialog();
            }
          }}
        >
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <div className="flex justify-center mb-2">
                <div className="rounded-full bg-green-100 dark:bg-green-900 p-3">
                  <CheckCircle2 className="h-8 w-8 text-green-600 dark:text-green-400" />
                </div>
              </div>
              <DialogTitle className="text-center text-xl">Attendance Scanned</DialogTitle>
              <DialogDescription className="text-center">
                Your attendance has been recorded for{' '}
                <span className="font-semibold text-foreground">{session.name}</span>.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter className="justify-center">
              <Button onClick={dismissScannedDialog}>OK</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  );
}
