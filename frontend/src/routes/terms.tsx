import { createFileRoute } from '@tanstack/react-router';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card';
import { PublicFooter } from '../components/public-footer';

export const Route = createFileRoute('/terms')({
  component: TermsPage,
});

function TermsPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center px-4 py-8">
      <Card className="w-full max-w-2xl">
        <CardHeader>
          <CardTitle>Terms of Service</CardTitle>
          <CardDescription>236 Attendance</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 text-sm leading-6">
          <p>
            236 Attendance is an unofficial unit tool operated by David. You can contact the
            operator at{' '}
            <a className="underline" href="mailto:ddl.tdh@gmail.com">
              ddl.tdh@gmail.com
            </a>
            .
          </p>
          <p>
            Use this service only for unit attendance tracking. Please provide accurate
            information, keep your password private, and contact David if you need help or
            notice a problem.
          </p>
          <p>
            This service is provided as-is for unit use. It is not an official MINDEF or SAF
            system, and availability may change without notice.
          </p>
        </CardContent>
      </Card>
      <PublicFooter />
    </main>
  );
}
