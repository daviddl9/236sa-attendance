import { createFileRoute } from '@tanstack/react-router';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card';
import { PublicFooter } from '../components/public-footer';

export const Route = createFileRoute('/privacy')({
  component: PrivacyPage,
});

function PrivacyPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center px-4 py-8">
      <Card className="w-full max-w-2xl">
        <CardHeader>
          <CardTitle>Privacy Policy</CardTitle>
          <CardDescription>236 Attendance</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 text-sm leading-6">
          <p>
            The operator is David. For questions about your information, email{' '}
            <a className="underline" href="mailto:ddl.tdh@gmail.com">
              ddl.tdh@gmail.com
            </a>
            .
          </p>
          <p>
            We store your name, rank, battery, and attendance records. We use them for unit
            attendance tracking.
          </p>
          <p>
            Unit commanders can see this information in the tool. It is not shown to signed-out
            visitors.
          </p>
          <p>We no longer collect NRIC or date of birth.</p>
        </CardContent>
      </Card>
      <PublicFooter />
    </main>
  );
}
