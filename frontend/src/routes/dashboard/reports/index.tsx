import { createFileRoute } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../../components/ui/tabs';
import { BarChart3, Users, TrendingUp } from 'lucide-react';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/dashboard/reports/')({
  component: ReportsPage,
});

function ReportsPage() {
  const { data: sessions } = useQuery({
    queryKey: ['sessions', 'all'],
    queryFn: () => apiClient.listSessions(),
  });

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Reports</h1>
          <p className="text-muted-foreground">View attendance reports and analytics</p>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Link to="/dashboard/reports/batteries">
            <Card className="hover:shadow-md transition-shadow cursor-pointer">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <BarChart3 className="h-5 w-5" />
                  Battery Reports
                </CardTitle>
                <CardDescription>Compare attendance across batteries</CardDescription>
              </CardHeader>
            </Card>
          </Link>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Users className="h-5 w-5" />
                User Reports
              </CardTitle>
              <CardDescription>View individual user attendance history</CardDescription>
            </CardHeader>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TrendingUp className="h-5 w-5" />
                Session Reports
              </CardTitle>
              <CardDescription>Detailed session analytics</CardDescription>
            </CardHeader>
          </Card>
        </div>

        {sessions && sessions.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Recent Sessions</CardTitle>
              <CardDescription>Click on a session to view detailed report</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {sessions.slice(0, 10).map((session) => (
                  <Link
                    key={session.id}
                    to="/dashboard/reports/sessions/$sessionId"
                    params={{ sessionId: session.id }}
                  >
                    <div className="flex items-center justify-between p-3 border rounded hover:bg-muted transition-colors cursor-pointer">
                      <div>
                        <p className="font-medium">{session.name}</p>
                        <p className="text-sm text-muted-foreground">
                          {new Date(session.startTime).toLocaleString()}
                        </p>
                      </div>
                      <span className="text-sm capitalize">{session.status}</span>
                    </div>
                  </Link>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </DashboardLayout>
  );
}

