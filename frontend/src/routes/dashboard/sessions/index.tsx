import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../../components/ui/tabs';
import { useState } from 'react';
import { Plus, Calendar, Clock, Users } from 'lucide-react';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/dashboard/sessions/')({
  component: SessionsPage,
});

function SessionsPage() {
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<'active' | 'all'>('active');

  const { data: activeSessions, isLoading: isLoadingActive } = useQuery({
    queryKey: ['sessions', 'active'],
    queryFn: () => apiClient.getActiveSessions(),
    enabled: activeTab === 'active',
  });

  const { data: allSessions, isLoading: isLoadingAll } = useQuery({
    queryKey: ['sessions', 'all'],
    queryFn: () => apiClient.listSessions(),
    enabled: activeTab === 'all',
  });

  const sessions = activeTab === 'active' ? activeSessions : allSessions;
  const isLoading = activeTab === 'active' ? isLoadingActive : isLoadingAll;

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Sessions</h1>
            <p className="text-muted-foreground">
              Manage attendance sessions and view history
            </p>
          </div>
          <Button onClick={() => navigate({ to: '/dashboard/sessions/create' })}>
            <Plus className="mr-2 h-4 w-4" />
            Create Session
          </Button>
        </div>

        <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'active' | 'all')}>
          <TabsList>
            <TabsTrigger value="active">Active</TabsTrigger>
            <TabsTrigger value="all">All Sessions</TabsTrigger>
          </TabsList>

          <TabsContent value={activeTab} className="space-y-4">
            {isLoading ? (
              <div className="flex items-center justify-center py-12">
                <div className="text-muted-foreground">Loading...</div>
              </div>
            ) : sessions && sessions.length > 0 ? (
              <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
                {sessions.map((session) => (
                  <Link
                    key={session.id}
                    to="/dashboard/sessions/$sessionId"
                    params={{ sessionId: session.id }}
                  >
                    <Card className="hover:shadow-md transition-shadow cursor-pointer">
                      <CardHeader>
                        <CardTitle className="text-lg">{session.name}</CardTitle>
                        <CardDescription>
                          {session.sessionType.replace('_', ' ').toUpperCase()}
                        </CardDescription>
                      </CardHeader>
                      <CardContent>
                        <div className="space-y-2 text-sm">
                          <div className="flex items-center gap-2 text-muted-foreground">
                            <Calendar className="h-4 w-4" />
                            <span>
                              {new Date(session.startTime).toLocaleString()}
                            </span>
                          </div>
                          <div className="flex items-center gap-2 text-muted-foreground">
                            <Clock className="h-4 w-4" />
                            <span className="capitalize">{session.status}</span>
                          </div>
                          <div className="flex items-center gap-2 text-muted-foreground">
                            <Users className="h-4 w-4" />
                            <span className="capitalize">
                              {session.scope === 'unit_wide' ? 'Unit Wide' : session.batteries.join(', ')}
                            </span>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  </Link>
                ))}
              </div>
            ) : (
              <div className="flex items-center justify-center py-12">
                <div className="text-center">
                  <p className="text-muted-foreground">No sessions found</p>
                  {activeTab === 'active' && (
                    <Button
                      className="mt-4"
                      onClick={() => navigate({ to: '/dashboard/sessions/create' })}
                    >
                      Create First Session
                    </Button>
                  )}
                </div>
              </div>
            )}
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  );
}

