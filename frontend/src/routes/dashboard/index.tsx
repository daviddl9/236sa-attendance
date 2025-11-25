import { createFileRoute, Navigate } from '@tanstack/react-router';
import { useAuth } from '../../lib/auth-context';

export const Route = createFileRoute('/dashboard/')({
  component: DashboardComponent,
});

function DashboardComponent() {
  const { user, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-muted-foreground">Loading...</div>
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/sign-in" search={{ redirect: undefined, qrToken: undefined }} />;
  }

  // Redirect all users to the scan page
  return <Navigate to="/dashboard/attendance/scan" />;
}

