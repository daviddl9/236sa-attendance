import { createFileRoute } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { Badge } from '../../../components/ui/badge';
import { QRCodeSVG } from 'qrcode.react';
import {
  ArrowLeft,
  Download,
  X,
  FileDown,
  FileSpreadsheet,
  Copy,
} from 'lucide-react';
import { toast } from 'sonner';
import { Link } from '@tanstack/react-router';
import { useAuth } from '../../../lib/auth-context';
import { UserTable } from '../../../components/users/user-table';

export const Route = createFileRoute('/dashboard/sessions/$sessionId')({
  component: SessionDetailPage,
});

function SessionDetailPage() {
  const { sessionId } = Route.useParams();
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const { data: session, isLoading } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => apiClient.getSessionById(sessionId),
  });

  const { data: analytics } = useQuery({
    queryKey: ['session-analytics', sessionId],
    queryFn: () => apiClient.getSessionAnalytics(sessionId),
    enabled: !!session,
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

  const canClose = user?.isSuperadmin || session?.createdBy === user?.id;

  // Generate QR code URL from session data
  // session.qrCode may contain "sessionID:secret" or "sessionID:secret:timestamp" format
  // Extract secret (second part) and construct URL with session.id:secret
  // Use frontend route for simpler URL that works with frontend domain
  const qrCodeUrl = session
    ? (() => {
        const parts = session.qrCode.split(':');
        const secret = parts.length >= 2 ? parts[1] : '';
        return `${window.location.origin}/qr/${session.id}:${secret}`;
      })()
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
      const blob =
        format === 'csv'
          ? await apiClient.exportSessionCSV(sessionId)
          : await apiClient.exportSessionExcel(sessionId);

      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${session?.name || 'session'}_attendance.${format === 'csv' ? 'csv' : 'xlsx'}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      toast.success(`Exported to ${format.toUpperCase()}`);
    } catch {
      toast.error(`Failed to export to ${format.toUpperCase()}`);
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
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>QR Code</CardTitle>
              <CardDescription>Scan this QR code to mark attendance</CardDescription>
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

          {analytics && (
            <Card>
              <CardHeader>
                <CardTitle>Statistics</CardTitle>
                <CardDescription>Session attendance overview</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <p className="text-sm text-muted-foreground">Total Users</p>
                    <p className="text-2xl font-bold">{analytics.totalUsers}</p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Present</p>
                    <p className="text-2xl font-bold">{analytics.presentCount}</p>
                  </div>
                  <div className="col-span-2">
                    <p className="text-sm text-muted-foreground">Attendance Rate</p>
                    <p className="text-2xl font-bold">
                      {analytics.attendancePercentage.toFixed(1)}%
                    </p>
                  </div>
                </div>
                <div className="pt-4 border-t">
                  <p className="text-sm text-muted-foreground mb-2">Missing Users</p>
                  <p className="text-lg font-semibold">{analytics.missingUsers?.length || 0}</p>
                </div>
              </CardContent>
            </Card>
          )}
        </div>

        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>Export Attendance</CardTitle>
                <CardDescription>Download attendance records</CardDescription>
              </div>
              <div className="flex gap-2">
                <Button onClick={() => handleExport('csv')} variant="outline">
                  <FileDown className="mr-2 h-4 w-4" />
                  CSV
                </Button>
                <Button onClick={() => handleExport('excel')} variant="outline">
                  <FileSpreadsheet className="mr-2 h-4 w-4" />
                  Excel
                </Button>
              </div>
            </div>
          </CardHeader>
        </Card>

        {analytics && analytics.missingUsers && analytics.missingUsers.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Missing Users</CardTitle>
              <CardDescription>Users who have not marked attendance</CardDescription>
            </CardHeader>
            <CardContent>
              <UserTable users={analytics.missingUsers} showActions={false} emptyMessage="No missing users" />
            </CardContent>
          </Card>
        )}
      </div>
    </DashboardLayout>
  );
}

