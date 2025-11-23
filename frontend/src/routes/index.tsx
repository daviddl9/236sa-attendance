import { createFileRoute } from '@tanstack/react-router';
import { useState } from 'react';
import { useAuth } from '../lib/auth-context';
import { Button } from '../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card';
import { cn } from '../lib/utils';
import { toast } from 'sonner';
import DashboardTopNav from '../components/dashboard/navbar';
import { SectionCards } from '../components/dashboard/section-cards';
import { ChartAreaInteractive } from '../components/dashboard/chart-interactive';

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

export const Route = createFileRoute('/')({
  component: IndexComponent,
});

function SignInContent() {
  const [loading, setLoading] = useState(false);

  const handleGoogleSignIn = async () => {
    try {
      setLoading(true);
      // Redirect to Go backend OAuth endpoint
      window.location.href = `${API_URL}/api/auth/oauth/google`;
    } catch (error) {
      setLoading(false);
      console.error('Authentication error:', error);
      toast.error('Oops, something went wrong', {
        duration: 5000,
      });
    }
  };

  return (
    <div className="flex flex-col justify-center items-center w-full h-screen px-4">
      <Card className="max-w-md w-full">
        <CardHeader>
          <CardTitle className="text-lg md:text-xl">
            Welcome to React Starter Kit
          </CardTitle>
          <CardDescription className="text-xs md:text-sm">
            Use your google account to login to your account
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4">
            <div
              className={cn(
                'w-full gap-2 flex items-center',
                'justify-between flex-col',
              )}
            >
              <Button
                variant="outline"
                className={cn('w-full gap-2 transition-all hover:shadow-md active:shadow-sm')}
                disabled={loading}
                onClick={handleGoogleSignIn}
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="0.98em"
                  height="1em"
                  viewBox="0 0 256 262"
                >
                  <path
                    fill="#4285F4"
                    d="M255.878 133.451c0-10.734-.871-18.567-2.756-26.69H130.55v48.448h71.947c-1.45 12.04-9.283 30.172-26.69 42.356l-.244 1.622l38.755 30.023l2.685.268c24.659-22.774 38.875-56.282 38.875-96.027"
                  ></path>
                  <path
                    fill="#34A853"
                    d="M130.55 261.1c35.248 0 64.839-11.605 86.453-31.622l-41.196-31.913c-11.024 7.688-25.82 13.055-45.257 13.055c-34.523 0-63.824-22.773-74.269-54.25l-1.531.13l-40.298 31.187l-.527 1.465C35.393 231.798 79.49 261.1 130.55 261.1"
                  ></path>
                  <path
                    fill="#FBBC05"
                    d="M56.281 156.37c-2.756-8.123-4.351-16.827-4.351-25.82c0-8.994 1.595-17.697 4.206-25.82l-.073-1.73L15.26 71.312l-1.335.635C5.077 89.644 0 109.517 0 130.55s5.077 40.905 13.925 58.602z"
                  ></path>
                  <path
                    fill="#EB4335"
                    d="M130.55 50.479c24.514 0 41.05 10.589 50.479 19.438l36.844-35.974C195.245 12.91 165.798 0 130.55 0C79.49 0 35.393 29.301 13.925 71.947l42.211 32.783c10.59-31.477 39.891-54.251 74.414-54.251"
                  ></path>
                </svg>
                Login with Google
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
      <p className="mt-6 text-xs text-center text-gray-500 dark:text-gray-400 max-w-md px-4">
        By signing in, you agree to our Terms of Service and Privacy Policy
      </p>
    </div>
  );
}

function IndexComponent() {
  try {
    const { user, isLoading } = useAuth();

    if (isLoading) {
      return (
        <div className="flex flex-col justify-center items-center w-full h-screen bg-background px-4">
          <div className="max-w-md w-full bg-gray-200 dark:bg-gray-800 animate-pulse rounded-lg h-96"></div>
        </div>
      );
    }

    if (user) {
      return (
        <DashboardTopNav>
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
        </DashboardTopNav>
      );
    }

    return <SignInContent />;
  } catch (error) {
    console.error('IndexComponent error:', error);
    return (
      <div className="flex flex-col justify-center items-center w-full h-screen bg-background px-4">
        <div className="text-red-500">Error loading page. Please refresh.</div>
      </div>
    );
  }
}

