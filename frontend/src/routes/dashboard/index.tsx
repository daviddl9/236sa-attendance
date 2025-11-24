import { createFileRoute, Navigate } from '@tanstack/react-router';
import { useAuth } from '../../lib/auth-context';
import DashboardLayout from '../../components/dashboard/layout';
import { SectionCards } from '../../components/dashboard/section-cards';
import { ChartAreaInteractive } from '../../components/dashboard/chart-interactive';
import { useEffect } from 'react';

export const Route = createFileRoute('/dashboard/')({
  component: DashboardComponent,
});

function DashboardComponent() {
  const { user, isLoading } = useAuth();

  // Check if user is superadmin (only superadmins can access overview)
  const isSuperadmin = user?.isSuperadmin || false;

  // Redirect non-superadmins (including commanders) to scan page
  useEffect(() => {
    if (!isLoading && user && !isSuperadmin) {
      window.location.href = '/dashboard/attendance/scan';
    }
  }, [isLoading, user, isSuperadmin]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-muted-foreground">Loading...</div>
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/sign-in" />;
  }

  // Show loading while redirecting non-superadmins
  if (!isSuperadmin) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-muted-foreground">Redirecting...</div>
      </div>
    );
  }

  return (
    <DashboardLayout>
      <section className="flex flex-col items-start justify-start p-6 w-full">
        <div className="w-full">
          <div className="flex flex-col items-start justify-center gap-2">
            <h1 className="text-3xl font-semibold tracking-tight">
              Interactive Chart
            </h1>
            <p className="text-muted-foreground">
              Interactive chart with data visualization and interactive elements.
            </p>
          </div>
          <div className="@container/main flex flex-1 flex-col gap-2">
            <div className="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
              <SectionCards />
              <ChartAreaInteractive />
            </div>
          </div>
        </div>
      </section>
    </DashboardLayout>
  );
}

