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
import { ArrowLeft } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { Button } from '../../../components/ui/button';
import { useState } from 'react';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';

export const Route = createFileRoute('/dashboard/reports/batteries')({
  component: BatteryReportsPage,
});

function BatteryReportsPage() {
  const [selectedBattery, setSelectedBattery] = useState<string>('HQ');

  const { data: hqReport } = useQuery({
    queryKey: ['battery-report', 'HQ'],
    queryFn: () => apiClient.getBatteryReport('HQ'),
  });

  const { data: alphaReport } = useQuery({
    queryKey: ['battery-report', 'Alpha'],
    queryFn: () => apiClient.getBatteryReport('Alpha'),
  });

  const { data: bravoReport } = useQuery({
    queryKey: ['battery-report', 'Bravo'],
    queryFn: () => apiClient.getBatteryReport('Bravo'),
  });

  const currentReport =
    selectedBattery === 'HQ'
      ? hqReport
      : selectedBattery === 'Alpha'
        ? alphaReport
        : bravoReport;

  // Prepare comparison data
  const comparisonData =
    hqReport && alphaReport && bravoReport
      ? hqReport.sessions.map((session, index) => ({
          session: session.sessionName,
          HQ: session.attendancePercentage,
          Alpha: alphaReport.sessions[index]?.attendancePercentage || 0,
          Bravo: bravoReport.sessions[index]?.attendancePercentage || 0,
        }))
      : [];

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
            <h1 className="text-3xl font-semibold tracking-tight">Battery Reports</h1>
            <p className="text-muted-foreground">Compare attendance across batteries</p>
          </div>
        </div>

        <Tabs value={selectedBattery} onValueChange={(v) => setSelectedBattery(v)}>
          <TabsList>
            <TabsTrigger value="HQ">HQ</TabsTrigger>
            <TabsTrigger value="Alpha">Alpha</TabsTrigger>
            <TabsTrigger value="Bravo">Bravo</TabsTrigger>
            <TabsTrigger value="compare">Compare All</TabsTrigger>
          </TabsList>

          <TabsContent value={selectedBattery} className="space-y-4">
            {currentReport ? (
              <>
                <Card>
                  <CardHeader>
                    <CardTitle>{currentReport.battery} Battery Sessions</CardTitle>
                    <CardDescription>
                      Attendance statistics for {currentReport.battery} battery
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="space-y-4">
                      {currentReport.sessions.map((session) => (
                        <div
                          key={session.sessionId}
                          className="flex items-center justify-between p-4 border rounded"
                        >
                          <div>
                            <p className="font-medium">{session.sessionName}</p>
                            <p className="text-sm text-muted-foreground">
                              {new Date(session.startTime).toLocaleString()}
                            </p>
                          </div>
                          <div className="text-right">
                            <p className="text-2xl font-bold">
                              {session.attendancePercentage.toFixed(1)}%
                            </p>
                            <p className="text-sm text-muted-foreground">
                              {session.presentCount} / {session.totalUsers}
                            </p>
                          </div>
                        </div>
                      ))}
                    </div>
                  </CardContent>
                </Card>
              </>
            ) : (
              <div className="flex items-center justify-center py-12">
                <div className="text-muted-foreground">Loading...</div>
              </div>
            )}
          </TabsContent>

          <TabsContent value="compare" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle>Battery Comparison</CardTitle>
                <CardDescription>
                  Compare attendance rates across all batteries
                </CardDescription>
              </CardHeader>
              <CardContent>
                {comparisonData.length > 0 ? (
                  <ResponsiveContainer width="100%" height={400}>
                    <BarChart data={comparisonData}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="session" />
                      <YAxis />
                      <Tooltip />
                      <Legend />
                      <Bar dataKey="HQ" fill="#8884d8" />
                      <Bar dataKey="Alpha" fill="#82ca9d" />
                      <Bar dataKey="Bravo" fill="#ffc658" />
                    </BarChart>
                  </ResponsiveContainer>
                ) : (
                  <div className="flex items-center justify-center py-12">
                    <div className="text-muted-foreground">No data available</div>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  );
}

