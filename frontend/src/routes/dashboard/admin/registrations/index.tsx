import { createFileRoute, Navigate } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient, type PendingRegistration } from '../../../../lib/api-client';
import DashboardLayout from '../../../../components/dashboard/layout';
import { Button } from '../../../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../../components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../../../components/ui/table';
import { Badge } from '../../../../components/ui/badge';
import { toast } from 'sonner';
import { Check, X, Clock } from 'lucide-react';
import { useAuth } from '../../../../lib/auth-context';
import { isSuperadmin } from '../../../../lib/user-utils';

export const Route = createFileRoute('/dashboard/admin/registrations/')({
  component: RegistrationsPage,
});

function RegistrationsPage() {
  const { user } = useAuth();

  if (!isSuperadmin(user)) {
    return <Navigate to="/dashboard" />;
  }

  return <RegistrationsContent />;
}

function RegistrationsContent() {
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ['pending-registrations'],
    queryFn: () => apiClient.listPendingRegistrations(),
    refetchInterval: 30_000,
  });

  const approveMutation = useMutation({
    mutationFn: (id: string) => apiClient.approveRegistration(id),
    onSuccess: (_, id) => {
      toast.success('Registration approved');
      queryClient.setQueryData(['pending-registrations'], (old: { registrations: PendingRegistration[]; total: number } | undefined) => {
        if (!old) return old;
        const registrations = old.registrations.filter((r) => r.id !== id);
        return { registrations, total: registrations.length };
      });
    },
    onError: () => toast.error('Failed to approve registration'),
  });

  const rejectMutation = useMutation({
    mutationFn: (id: string) => apiClient.rejectRegistration(id),
    onSuccess: (_, id) => {
      toast.success('Registration rejected');
      queryClient.setQueryData(['pending-registrations'], (old: { registrations: PendingRegistration[]; total: number } | undefined) => {
        if (!old) return old;
        const registrations = old.registrations.filter((r) => r.id !== id);
        return { registrations, total: registrations.length };
      });
    },
    onError: () => toast.error('Failed to reject registration'),
  });

  const registrations = data?.registrations ?? [];

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Pending Registrations</h1>
          <p className="text-muted-foreground">
            Review and approve self-registered users before they can access the system.
          </p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Awaiting Approval
              {registrations.length > 0 && (
                <Badge variant="destructive" className="ml-1">
                  {registrations.length}
                </Badge>
              )}
            </CardTitle>
            <CardDescription>
              These users registered themselves and are not on the pre-loaded personnel list.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <p className="text-sm text-muted-foreground py-4 text-center">Loading...</p>
            ) : registrations.length === 0 ? (
              <p className="text-sm text-muted-foreground py-8 text-center">
                No pending registrations.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Full Name</TableHead>
                    <TableHead>Rank</TableHead>
                    <TableHead>Battery</TableHead>
                    <TableHead>NRIC Last 5</TableHead>
                    <TableHead>Submitted</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {registrations.map((reg) => (
                    <TableRow key={reg.id}>
                      <TableCell className="font-medium">{reg.fullName ?? '—'}</TableCell>
                      <TableCell>{reg.rank ?? '—'}</TableCell>
                      <TableCell>{reg.battery ?? '—'}</TableCell>
                      <TableCell>{reg.nricLast5 ?? '—'}</TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {new Date(reg.createdAt).toLocaleDateString()}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="outline"
                            className="text-green-600 border-green-200 hover:bg-green-50 hover:border-green-300"
                            onClick={() => approveMutation.mutate(reg.id)}
                            disabled={approveMutation.isPending || rejectMutation.isPending}
                          >
                            <Check className="h-4 w-4 mr-1" />
                            Approve
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            className="text-destructive border-destructive/30 hover:bg-destructive/5"
                            onClick={() => rejectMutation.mutate(reg.id)}
                            disabled={approveMutation.isPending || rejectMutation.isPending}
                          >
                            <X className="h-4 w-4 mr-1" />
                            Reject
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}
