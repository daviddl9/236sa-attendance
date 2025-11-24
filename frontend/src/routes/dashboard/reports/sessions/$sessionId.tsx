import { createFileRoute } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../../../../lib/api-client';
import DashboardLayout from '../../../../components/dashboard/layout';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../../components/ui/card';
import { ArrowLeft, Users, TrendingUp, AlertCircle } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { Button } from '../../../../components/ui/button';

export const Route = createFileRoute('/dashboard/reports/sessions/$sessionId')({
  component: SessionReportPage,
});

function SessionReportPage() {
  const { sessionId } = Route.useParams();

  const { data: analytics, isLoading } = useQuery({
    queryKey: ['session-analytics', sessionId],
    queryFn: () => apiClient.getSessionAnalytics(sessionId),
  });

  const { data: session } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => apiClient.getSessionById(sessionId),
  });

  if (isLoading) {
    return (
      <DashboardLayout>
        <div className="flex items-center justify-center py-12">
          <div className="text-muted-foreground">Loading...</div>
        </div>
      </DashboardLayout>
    );
  }

  if (!analytics || !session) {
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
        <div className="flex items-center gap-4">
          <Link to="/dashboard/reports">
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">{session.name}</h1>
            <p className="text-muted-foreground">Detailed attendance report</p>
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Users className="h-5 w-5" />
                Total Users
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-3xl font-bold">{analytics.totalUsers}</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TrendingUp className="h-5 w-5" />
                Present
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-3xl font-bold">{analytics.presentCount}</p>
              <p className="text-sm text-muted-foreground">
                {analytics.attendancePercentage.toFixed(1)}% attendance rate
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <AlertCircle className="h-5 w-5" />
                Missing
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-3xl font-bold">{analytics.missingUsers?.length || 0}</p>
            </CardContent>
          </Card>
        </div>

        {analytics.byBattery && Object.keys(analytics.byBattery).length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>By Battery</CardTitle>
              <CardDescription>Attendance breakdown by battery</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {Object.entries(analytics.byBattery).map(([battery, stats]) => {
                  const percentage =
                    stats.total > 0 ? (stats.present / stats.total) * 100 : 0;
                  return (
                    <div key={battery}>
                      <div className="flex items-center justify-between mb-2">
                        <span className="font-medium">{battery}</span>
                        <span className="text-sm text-muted-foreground">
                          {stats.present} / {stats.total} ({percentage.toFixed(1)}%)
                        </span>
                      </div>
                      <div className="w-full bg-muted rounded-full h-2">
                        <div
                          className="bg-primary h-2 rounded-full"
                          style={{ width: `${percentage}%` }}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            </CardContent>
          </Card>
        )}

        {analytics.missingUsers && analytics.missingUsers.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Missing Users</CardTitle>
              <CardDescription>
                Users who have not marked attendance for this session
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {analytics.missingUsers.map((user) => (
                  <div
                    key={user.id}
                    className="flex items-center justify-between p-3 border rounded"
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

