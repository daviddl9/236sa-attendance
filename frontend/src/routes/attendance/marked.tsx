import { createFileRoute, useSearch } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../../lib/api-client';
import DashboardLayout from '../../components/dashboard/layout';
import { Button } from '../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../components/ui/card';
import { CheckCircle2, ArrowLeft } from 'lucide-react';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/attendance/marked')({
  component: AttendanceMarkedPage,
  validateSearch: (search: Record<string, unknown>) => {
    return {
      session: (search.session as string) || undefined,
    };
  },
});

function AttendanceMarkedPage() {
  const search = useSearch({ from: '/attendance/marked' }) as { session?: string };

  // Try to fetch session details, but silently fail if user doesn't have permission
  const { data: session } = useQuery({
    queryKey: ['session', search.session],
    queryFn: () => apiClient.getSessionById(search.session!),
    enabled: !!search.session,
    retry: false,
  });

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <Card className="max-w-md mx-auto">
          <CardHeader>
            <div className="flex flex-col items-center gap-4">
              <div className="rounded-full bg-green-100 dark:bg-green-900 p-4">
                <CheckCircle2 className="h-12 w-12 text-green-600 dark:text-green-400" />
              </div>
              <div className="text-center">
                <CardTitle className="text-2xl">Attendance Marked!</CardTitle>
                <CardDescription className="mt-2">
                  Your attendance has been successfully recorded
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {session && (
              <div className="space-y-2 text-center">
                <p className="text-sm text-muted-foreground">Session</p>
                <p className="font-semibold">{session.name}</p>
                <p className="text-sm text-muted-foreground">
                  {new Date(session.startTime).toLocaleString()}
                </p>
              </div>
            )}
            <div className="flex flex-col gap-2 pt-4">
              <Link to="/dashboard">
                <Button className="w-full">
                  <ArrowLeft className="mr-2 h-4 w-4" />
                  Back to Dashboard
                </Button>
              </Link>
            </div>
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}

