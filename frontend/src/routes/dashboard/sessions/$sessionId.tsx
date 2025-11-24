import { createFileRoute, useNavigate } from '@tanstack/react-router';
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
import { useState } from 'react';
import {
  ArrowLeft,
  Download,
  X,
  QrCode,
  Users,
  FileDown,
  FileSpreadsheet,
} from 'lucide-react';
import { toast } from 'sonner';
import { Link } from '@tanstack/react-router';
import { useAuth } from '../../../lib/auth-context';

export const Route = createFileRoute('/dashboard/sessions/$sessionId')({
  component: SessionDetailPage,
});

function SessionDetailPage() {
  const { sessionId } = Route.useParams();
  const { user } = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [qrImageUrl, setQrImageUrl] = useState<string | null>(null);

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

  const handleDownloadQR = async () => {
    try {
      const blob = await apiClient.getSessionQR(sessionId);
      const url = URL.createObjectURL(blob);
      setQrImageUrl(url);

      const a = document.createElement('a');
      a.href = url;
      a.download = `${session?.name || 'session'}_qr.png`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (error) {
      toast.error('Failed to download QR code');
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
    } catch (error) {
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
                {session.sessionType.replace('_', ' ').toUpperCase()} •{' '}
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
              {qrImageUrl ? (
                <img src={qrImageUrl} alt="QR Code" className="w-64 h-64" />
              ) : (
                <div className="w-64 h-64 bg-muted flex items-center justify-center rounded-lg">
                  <QrCode className="h-32 w-32 text-muted-foreground" />
                </div>
              )}
              <Button onClick={handleDownloadQR} variant="outline">
                <Download className="mr-2 h-4 w-4" />
                Download QR Code
              </Button>
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

        {analytics && analytics.missingUsers.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Missing Users</CardTitle>
              <CardDescription>Users who have not marked attendance</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {analytics.missingUsers.map((user) => (
                  <div
                    key={user.id}
                    className="flex items-center justify-between p-2 border rounded"
                  >
                    <div>
                      <p className="font-medium">{user.fullName || 'Unknown'}</p>
                      <p className="text-sm text-muted-foreground">
                        {user.rank || 'N/A'} • {user.battery || 'N/A'}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </DashboardLayout>
  );
}

